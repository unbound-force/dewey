package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Temporal-validity metadata for learnings (issue #146).
//
// Design decision: every field lives in the page's properties JSON and
// the learning's markdown frontmatter — no schema change. Validity is an
// attribute, not a trust tier: tiers say who vouched for a learning,
// validity says whether it is still true as the code evolves.

// validValidities is the closed set of validity statuses a learning may
// carry. Absent validity means unknown.
var validValidities = map[string]bool{
	"current":          true,
	"stale-unverified": true,
	"superseded":       true,
	"historical":       true,
}

const (
	// maxEvidenceEntries bounds the evidence list on one learning.
	maxEvidenceEntries = 50
	// maxEvidenceLen bounds one evidence locator, in characters
	// (runes), matching the spec's "at most 512 characters".
	maxEvidenceLen = 512
)

// temporalFields holds validated temporal metadata for one learning.
// Empty fields are omitted from properties and frontmatter.
type temporalFields struct {
	AsOf       string
	Evidence   []string
	Validity   string
	Supersedes string // identity of the superseded learning
}

// validateValidity returns an error when v is non-empty and not one of
// the valid statuses.
func validateValidity(v string) error {
	if v != "" && !validValidities[v] {
		return fmt.Errorf("invalid validity %q. Valid values: current, stale-unverified, superseded, historical", v)
	}
	return nil
}

// normalizeEvidence trims each locator, drops exact duplicates while
// preserving order, and enforces the entry-count and length bounds.
// Returns an error for an empty or over-long locator, or too many.
func normalizeEvidence(raw []string) ([]string, error) {
	if len(raw) > maxEvidenceEntries {
		return nil, fmt.Errorf("evidence has %d entries; at most %d are allowed", len(raw), maxEvidenceEntries)
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for i, entry := range raw {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			return nil, fmt.Errorf("evidence entry %d is empty", i+1)
		}
		if runes := utf8.RuneCountInString(trimmed); runes > maxEvidenceLen {
			return nil, fmt.Errorf("evidence entry %d is %d characters; at most %d are allowed", i+1, runes, maxEvidenceLen)
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out, nil
}

// supersedesIdentity strips an optional "learning/" page-name prefix so
// callers may pass either an identity or a page name.
func supersedesIdentity(ref string) string {
	return strings.TrimPrefix(strings.TrimSpace(ref), "learning/")
}

// ValidLearningIdentity reports whether identity is safe to embed in a
// learning path: non-empty, free of path separators, and with no leading
// dot or ".." traversal segment, so filepath.Join cannot escape the
// learnings directory (SC-004). Tool-created identities
// ("{tag}-{timestamp}-{author}") always pass; identities forged through
// re-ingested, untrusted frontmatter (for example "../../escape") fail.
// Characters beyond [a-z0-9-] are allowed because identities embed the
// author, which git config may render with spaces or capitals.
func ValidLearningIdentity(identity string) bool {
	if identity == "" || strings.HasPrefix(identity, ".") {
		return false
	}
	if strings.ContainsAny(identity, `/\`) {
		return false
	}
	return !strings.Contains(identity, "..")
}

// properties returns the non-empty temporal fields as page properties.
func (t temporalFields) properties() map[string]any {
	props := map[string]any{}
	if t.AsOf != "" {
		props["as_of"] = t.AsOf
	}
	if len(t.Evidence) > 0 {
		props["evidence"] = t.Evidence
	}
	if t.Validity != "" {
		props["validity"] = t.Validity
	}
	if t.Supersedes != "" {
		props["supersedes"] = t.Supersedes
	}
	return props
}

// frontmatter renders the non-empty temporal fields as YAML frontmatter
// lines (each newline-terminated). Strings are double-quoted with %q,
// which is valid YAML for these values; evidence is a block sequence.
func (t temporalFields) frontmatter() string {
	var buf strings.Builder
	if t.AsOf != "" {
		fmt.Fprintf(&buf, "as_of: %q\n", t.AsOf)
	}
	if len(t.Evidence) > 0 {
		buf.WriteString("evidence:\n")
		for _, e := range t.Evidence {
			fmt.Fprintf(&buf, "  - %q\n", e)
		}
	}
	if t.Validity != "" {
		fmt.Fprintf(&buf, "validity: %q\n", t.Validity)
	}
	if t.Supersedes != "" {
		fmt.Fprintf(&buf, "supersedes: %q\n", t.Supersedes)
	}
	return buf.String()
}

// prepareTemporal validates the temporal inputs, resolves the supersession
// target, and resolves the as_of pin — all before anything is written, so
// an invalid call writes nothing. The returned page is nil when the input
// supersedes nothing.
func (l *Learning) prepareTemporal(input types.StoreLearningInput) (temporalFields, *store.Page, error) {
	if err := validateValidity(input.Validity); err != nil {
		return temporalFields{}, nil, err
	}
	evidence, err := normalizeEvidence(input.Evidence)
	if err != nil {
		return temporalFields{}, nil, err
	}
	target, err := l.resolveSupersedesTarget(input.Supersedes)
	if err != nil {
		return temporalFields{}, nil, err
	}
	fields := temporalFields{
		AsOf:       resolveAsOf(l.asOfResolver),
		Evidence:   evidence,
		Validity:   input.Validity,
		Supersedes: supersedesIdentity(input.Supersedes),
	}
	return fields, target, nil
}

// resolveSupersedesTarget returns the learning page that ref names, nil
// when ref is empty, or an error when ref names no learning or one that
// is already superseded (one successor per learning: chains, not forks).
func (l *Learning) resolveSupersedesTarget(ref string) (*store.Page, error) {
	identity := supersedesIdentity(ref)
	if identity == "" {
		return nil, nil
	}
	if !ValidLearningIdentity(identity) {
		return nil, fmt.Errorf("supersedes: learning %q not found", identity)
	}
	page, err := l.store.GetPage(store.LearningPrefix + identity)
	if err != nil {
		return nil, fmt.Errorf("supersedes: failed to look up learning %q: %v", identity, err)
	}
	if page == nil || page.SourceID != store.SourceLearning {
		return nil, fmt.Errorf("supersedes: learning %q not found", identity)
	}
	props, err := decodeLearningProperties(page.Properties)
	if err != nil {
		return nil, fmt.Errorf("supersedes: learning %q has unreadable properties: %w", identity, err)
	}
	if successor, ok := props["superseded_by"].(string); ok && successor != "" {
		return nil, fmt.Errorf("supersedes: learning %q is already superseded by %q", identity, successor)
	}
	return page, nil
}

// linkSuperseded records successor as the target's superseded_by and sets
// its validity to superseded, in the store and in its markdown backup when
// one exists. The target's tier and content are untouched. Returns a
// warning describing a failure, or "" on success or a nil target.
func (l *Learning) linkSuperseded(target *store.Page, successor string) string {
	if target == nil {
		return ""
	}
	props, err := decodeLearningProperties(target.Properties)
	if err != nil {
		return fmt.Sprintf("failed to mark %s superseded: unreadable stored properties: %v", target.Name, err)
	}
	props["superseded_by"] = successor
	props["validity"] = "superseded"
	data, _ := json.Marshal(props) // a map of JSON-decoded values always marshals
	target.Properties = string(data)
	if err := l.store.UpdatePage(target); err != nil {
		return fmt.Sprintf("failed to mark %s superseded in the store: %v", target.Name, err)
	}
	return l.markSupersededFile(supersedesIdentity(target.Name), successor)
}

// markSupersededFile sets superseded_by and validity in the frontmatter of
// a learning's markdown backup. A missing file (no vault, or a store-only
// learning) is not an error. Returns a warning on failure, else "".
func (l *Learning) markSupersededFile(identity, successor string) string {
	if err := updateLearningBackup(l.vaultPath, identity, [][2]string{{"superseded_by", successor}, {"validity", "superseded"}}); err != nil {
		return fmt.Sprintf("marked %s superseded in the store, but not in its markdown backup: %v", identity, err)
	}
	return ""
}

// decodeLearningProperties decodes a page's properties JSON into a non-nil
// map: empty text and a literal JSON null both yield an empty map. It is
// the one decoder for learning properties that are read to be rewritten.
func decodeLearningProperties(raw string) (map[string]any, error) {
	props := map[string]any{}
	if raw == "" {
		return props, nil
	}
	if err := json.Unmarshal([]byte(raw), &props); err != nil {
		return nil, err
	}
	if props == nil {
		props = map[string]any{}
	}
	return props, nil
}

// updateLearningBackup sets top-level frontmatter keys in a learning's
// markdown backup under vaultPath. It is the shared write path for every
// flow that updates a learning's store properties and then its backup
// (#146 supersession, #147 anchor fix): callers write the store first and
// report an error from here as a warning, since the store write stands.
// No vault and a missing file are not errors; an identity that is unsafe
// to embed in a path is, and nothing is written.
func updateLearningBackup(vaultPath, identity string, keys [][2]string) error {
	if vaultPath == "" {
		return nil
	}
	if !ValidLearningIdentity(identity) {
		return fmt.Errorf("unsafe learning identity %q", identity)
	}
	path := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings", identity+".md")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	updated, err := setFrontmatterKeys(string(content), keys)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

// resolveAsOf returns the resolver's `<repo>@<sha>` pin, or "" when the
// resolver is nil or fails. Storing a learning never fails because git
// is unavailable.
func resolveAsOf(resolver func() (string, error)) string {
	if resolver == nil {
		return ""
	}
	asOf, err := resolver()
	if err != nil {
		learningLogger.Debug("as_of unavailable", "err", err)
		return ""
	}
	return asOf
}

// productionAsOfResolver returns a resolver that pins the vault's git
// repository as `<basename of top-level>@<HEAD sha>`, using git
// subprocesses bounded by a 2-second timeout. An empty vaultPath always
// fails, so tool handlers without a vault never shell out.
func productionAsOfResolver(vaultPath string) func() (string, error) {
	return func() (string, error) {
		if vaultPath == "" {
			return "", fmt.Errorf("no vault path")
		}
		top, err := gitOutput(vaultPath, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", err
		}
		sha, err := gitOutput(vaultPath, "rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		return filepath.Base(top) + "@" + sha, nil
	}
}

// gitOutput runs `git -C dir args...` with a 2-second timeout and returns
// its trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// setFrontmatterKeys sets top-level scalar keys in a markdown file's YAML
// frontmatter, replacing an existing `key:` line in place or inserting
// new keys before the closing delimiter, in the given order. Values are
// written double-quoted. The body is untouched. Returns an error when the
// content has no frontmatter.
func setFrontmatterKeys(content string, keys [][2]string) (string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", fmt.Errorf("no YAML frontmatter found")
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return "", fmt.Errorf("unterminated YAML frontmatter")
	}
	lines := strings.Split(content[4:4+end], "\n")
	for _, kv := range keys {
		line := fmt.Sprintf("%s: %q", kv[0], kv[1])
		lines = replaceOrAppendKey(lines, kv[0], line)
	}
	return "---\n" + strings.Join(lines, "\n") + content[4+end:], nil
}

// replaceOrAppendKey replaces the first top-level `key:` line with line,
// or appends line when the key is absent.
func replaceOrAppendKey(lines []string, key, line string) []string {
	prefix := key + ":"
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			lines[i] = line
			return lines
		}
	}
	return append(lines, line)
}
