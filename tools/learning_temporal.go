package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// isLearningIdentity reports whether s matches the learning-identity
// grammar: the current "{tag}-{YYYYMMDDTHHMMSS}-{author}" form or the
// legacy pre-#146 "{tag}-{sequence}" form (a non-empty tag and a numeric
// sequence, for example "auth-1", "vault-walker-2"). Both formats the
// store holds are first-class: reIngestLearnings still re-ingests
// legacy-identity files, so a declared link to "auth-1" is valid, not
// malformed. Every hyphen-separated segment must be a non-empty run of
// lowercase ASCII letters and digits — the character set normalizeTag
// produces for tags and authors — except the timestamp segment, which
// carries the one uppercase letter the grammar allows. The timestamp form
// requires a tag segment before the timestamp and an author segment after
// it; the legacy form requires a numeric final segment after the tag. This
// is the ES-FR-003 prompt-injection boundary: a value that fails the
// grammar is treated as absent rather than rendered, so spaces, newlines,
// brackets, and other prompt-bearing characters never pass. Unlike
// ValidLearningIdentity, which guards filesystem paths, this is a shape
// check for decoded link values and bracketed citations.
func isLearningIdentity(s string) bool {
	segs := strings.Split(s, "-")
	timestampAt := -1
	for i, seg := range segs {
		switch {
		case isTimestampSegment(seg):
			if timestampAt < 0 {
				timestampAt = i
			}
		case !isLowerAlnumRun(seg):
			return false
		}
	}
	if timestampAt >= 0 {
		// {tag}-{timestamp}-{author}: the timestamp must sit strictly
		// between a non-empty tag and a non-empty author.
		return timestampAt > 0 && timestampAt < len(segs)-1
	}
	// Legacy {tag}-{sequence}: a non-empty tag and a numeric sequence.
	return len(segs) >= 2 && isDigitRun(segs[len(segs)-1])
}

// isTimestampSegment reports whether s has the YYYYMMDDTHHMMSS shape —
// eight digits, an uppercase T, six digits — matching the identity
// generator's "20060102T150405" layout. It is a shape match, not a
// calendar validation: the grammar discriminates identity-shaped strings,
// it does not parse dates.
func isTimestampSegment(s string) bool {
	if len(s) != 15 || s[8] != 'T' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i != 8 && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return true
}

// isLowerAlnumRun reports whether s is non-empty and consists solely of
// lowercase ASCII letters and digits — the character set normalizeTag
// produces for identity tags and authors. Non-ASCII bytes always fail the
// range checks, so multi-byte runes never pass.
func isLowerAlnumRun(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// isDigitRun reports whether s is non-empty and consists solely of ASCII
// digits — the shape of a legacy identity's numeric sequence. Digit runs
// of any length pass: the grammar says "numeric sequence", not a bounded
// integer.
func isDigitRun(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
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
// one exists. The store leg is a guarded claim (Store.ClaimSupersession):
// when another writer recorded a successor between validation and this
// write, nothing is written, the file leg does not run, and the returned
// warning names the recorded winner rather than the generic store-leg
// failure. The file leg is guarded by refuseSupersededBackup: a backup
// that already records a different successor is left byte-for-byte
// unchanged and the refusal is returned as the warning, so a forked
// durable record is surfaced rather than silently rewritten. Both legs
// run under a single withLearningPropsLock hold, so a concurrent lint fix
// cannot interleave between them. The target's tier and content are
// untouched. Returns a warning describing a failure, a refusal, or a lost
// claim, or "" on success or a nil target.
func (l *Learning) linkSuperseded(target *store.Page, successor string) string {
	if target == nil {
		return ""
	}
	var warning string
	err := withLearningPropsLock(func() error {
		claimed, winner, claimErr := l.claimSuperseded(target, successor)
		if claimErr != nil {
			return claimErr
		}
		if !claimed {
			// Lost claim. Both loss routes — the in-closure re-read
			// fast-fail and the zero-row CAS refusal — converge on
			// claimSuperseded's (false, winner, nil) result, so the
			// warning names the recorded winner rather than surfacing
			// as the generic store-leg error (SSW-FR-002, design D2).
			warning = lostClaimWarning(target.Name, winner)
			return nil
		}
		warning = l.markSupersededFile(supersedesIdentity(target.Name), successor)
		return nil
	})
	if err != nil {
		return fmt.Sprintf("failed to mark %s superseded in the store: %v", target.Name, err)
	}
	return warning
}

// lostClaimWarning renders the supersedes_warning for a supersession
// claim that lost to a concurrent writer: targetIdentity now records
// winner as its successor, so this call's learning is kept with a
// dangling supersedes claim (SSW-FR-002, design D2). Both arguments
// are normalized with supersedesIdentity — the target arrives as a
// page name carrying the "learning/" prefix, and a winner recorded
// through re-ingested frontmatter may carry one too — so the warning
// always names bare identities.
func lostClaimWarning(targetIdentity, winner string) string {
	return fmt.Sprintf("learning %q was concurrently superseded by %q; this learning's supersedes claim is dangling",
		supersedesIdentity(targetIdentity), supersedesIdentity(winner))
}

// claimSuperseded records successor as the target's superseded_by and sets
// its validity to superseded through the guarded store claim. It MUST be
// called only inside a withLearningPropsLock section: it re-reads the
// target page and decodes its properties — the validation-time snapshot
// supplies the target's identity only, never its properties — so keys
// preserved by other mutex-held writers survive the write. The re-read
// carries the in-closure superseded_by fast-fail (the markStaleUnverified
// precedent for validity): a recorded non-empty string successor routes
// through the lost-claim outcome (false, recorded winner, nil), never a
// generic store-leg error, while a present non-string value is an error.
// Otherwise the mutated properties are marshaled and handed to
// Store.ClaimSupersession, whose (claimed, winner, err) result is returned
// unchanged — the SQL guard remains the authority over writers that
// bypass the mutex.
func (l *Learning) claimSuperseded(target *store.Page, successor string) (claimed bool, winner string, err error) {
	page, err := l.store.GetPage(target.Name)
	if err != nil {
		return false, "", err
	}
	if page == nil {
		return false, "", fmt.Errorf("page not found: %s", target.Name)
	}
	props, err := decodeLearningProperties(page.Properties)
	if err != nil {
		return false, "", fmt.Errorf("unreadable stored properties on %s: %w", target.Name, err)
	}
	if recorded, present := props["superseded_by"]; present {
		recordedWinner, isString := recorded.(string)
		if !isString {
			return false, "", fmt.Errorf("learning %q records a non-string superseded_by value", target.Name)
		}
		if recordedWinner != "" {
			return false, recordedWinner, nil
		}
	}
	props["superseded_by"] = successor
	props["validity"] = "superseded"
	data, _ := json.Marshal(props) // a map of JSON-decoded values always marshals
	return l.store.ClaimSupersession(target.Name, string(data))
}

// refuseSupersededBackup returns the supersession file leg's refusal
// policy (serialize-supersession-writes design D3): the backup's recorded
// superseded_by, read with frontmatterScalar, is compared against the
// successor about to be written, both normalized with supersedesIdentity
// so a recorded "learning/<identity>" page name equals the bare identity.
// An absent or empty value is unclaimed and proceeds; an equal value
// proceeds, so an idempotent rewrite also repairs a missing validity key.
// Any other value — a differing successor, or an unparseable or block-form
// scalar frontmatterScalar returns verbatim — yields a reason naming it,
// so updateLearningBackup leaves the file byte-for-byte unchanged rather
// than clobbering the durable record.
func refuseSupersededBackup(successor string) func(content string) string {
	return func(content string) string {
		recorded, found := frontmatterScalar(content, "superseded_by")
		if !found || recorded == "" {
			return ""
		}
		if supersedesIdentity(recorded) == supersedesIdentity(successor) {
			return ""
		}
		return fmt.Sprintf("backup already records superseded_by %q", recorded)
	}
}

// markSupersededFile sets superseded_by and validity in the frontmatter of
// a learning's markdown backup. A missing file (no vault, or a store-only
// learning) is not an error. The write is guarded by
// refuseSupersededBackup: a backup that already records a different
// successor is left byte-for-byte unchanged and the refusal is returned as
// a warning naming the recorded value — the file is the durable record
// after graph.db loss, so a fork is surfaced to the operator rather than
// silently rewritten. Returns a warning on failure or refusal, else "".
func (l *Learning) markSupersededFile(identity, successor string) string {
	err := updateLearningBackup(l.vaultPath, identity,
		[][2]string{{"superseded_by", successor}, {"validity", "superseded"}},
		refuseSupersededBackup(successor))
	if err == nil {
		return ""
	}
	var refusal *backupRefusalError
	if errors.As(err, &refusal) {
		return fmt.Sprintf("learning %q was not marked superseded in its markdown backup: %s", identity, refusal)
	}
	return fmt.Sprintf("marked %s superseded in the store, but not in its markdown backup: %v", identity, err)
}

// learningPropsMu serializes read-modify-write updates of stored learning
// properties within this process, across each caller's store leg and file
// leg. MCP calls run concurrently, and lint's fix and store_learning's
// supersession link both rewrite whole property sets, so without it one
// could silently drop the other's change. The lock is acquired only
// through withLearningPropsLock; updateLearningProperties requires its
// caller to already hold it.
var learningPropsMu sync.Mutex

// withLearningPropsLock runs fn while holding learningPropsMu and returns
// fn's result once the section ends. Callers wrap their store leg and file
// leg in one section — linkSuperseded's claim plus backup write,
// markStaleUnverified's property update plus backup write — so a
// concurrent learning-property writer cannot interleave between the legs
// and drop either leg's changes. Code inside the section (notably
// updateLearningProperties) relies on the caller holding the lock and must
// not acquire it again.
func withLearningPropsLock(fn func() error) error {
	learningPropsMu.Lock()
	defer learningPropsMu.Unlock()
	return fn()
}

// updateLearningProperties re-reads a learning page, applies mutate to its
// decoded properties, and writes the page back. The caller MUST already
// hold learningPropsMu via withLearningPropsLock: this function is
// lock-free so its callers can span their store leg and file leg with a
// single hold. Nothing is written when the page is missing, its properties
// are unreadable, or mutate returns an error.
func updateLearningProperties(s *store.Store, pageName string, mutate func(props map[string]any) error) error {
	page, err := s.GetPage(pageName)
	if err != nil {
		return err
	}
	if page == nil {
		return fmt.Errorf("page not found: %s", pageName)
	}
	props, err := decodeLearningProperties(page.Properties)
	if err != nil {
		return fmt.Errorf("unreadable stored properties on %s: %w", pageName, err)
	}
	if err := mutate(props); err != nil {
		return err
	}
	data, _ := json.Marshal(props) // a map of JSON-decoded values always marshals
	page.Properties = string(data)
	return s.UpdatePage(page)
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

// backupRefusalError reports that a guarded backup write left the file
// unchanged because its recorded frontmatter already carried a successor
// (or a superseded validity). Callers treat it as a reported refusal.
type backupRefusalError struct{ reason string }

// Error returns the refusal reason, which names the recorded value the
// guard refused to clobber.
func (e *backupRefusalError) Error() string { return e.reason }

// updateLearningBackup sets top-level frontmatter keys in a learning's
// markdown backup under vaultPath, after the caller's refusal policy has
// approved the write. It is the shared write path for every flow that
// updates a learning's store properties and then its backup (#146
// supersession, #147 anchor fix): callers write the store first, run both
// legs inside one withLearningPropsLock hold, and report an error from
// here as a warning, since the store write stands. After the file is read,
// refuse is consulted with its content; a non-empty reason returns a
// *backupRefusalError and nothing is written, so a backup whose recorded
// frontmatter must survive keeps its value byte-for-byte. A nil policy
// never refuses. No vault and a missing file are not errors; an identity
// that is unsafe to embed in a path is, and nothing is written.
func updateLearningBackup(vaultPath, identity string, keys [][2]string, refuse func(content string) string) error {
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
	if refuse != nil {
		if reason := refuse(string(content)); reason != "" {
			return &backupRefusalError{reason: reason}
		}
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
// its trimmed stdout. A non-zero exit is an error. It shares the anchor
// checker's runner so the package has one git subprocess path.
func gitOutput(dir string, args ...string) (string, error) {
	out, code, err := gitAnchorChecker{dir: dir, timeout: 2 * time.Second}.run(context.Background(), args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git %s: exit status %d", strings.Join(args, " "), code)
	}
	return strings.TrimSpace(out), nil
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

// frontmatterScalar reads a top-level scalar key from a markdown file's
// YAML frontmatter — the read-side counterpart of setFrontmatterKeys, so
// the guarded file leg re-reads exactly what the writer emits
// (serialize-supersession-writes design D3, step 2). It returns
// ("", false) when the content has no frontmatter or the key is absent,
// and (value, true) when a top-level `key:` line exists. A key line with
// nothing after it is an empty (unclaimed) value. A key line with nothing
// but whitespace after the colon whose immediately following line is
// indented is BLOCK FORM: the text after the colon plus the indented
// children is returned as non-empty junk no valid learning identity can
// equal, so the caller's comparison refuses it — reading a block as
// unclaimed would let setFrontmatterKeys replace only the key line and
// leave the children attached to a scalar, producing invalid YAML. A
// non-empty scalar is decoded to value semantics by decodeScalarValue.
// There is no error return; the caller owns the fail-closed decision.
func frontmatterScalar(content, key string) (value string, found bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", false
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return "", false
	}
	lines := strings.Split(content[4:4+end], "\n")
	prefix := key + ":"
	for i, line := range lines {
		// Top-level only: an indented line cannot start with the prefix,
		// the same scan replaceOrAppendKey uses.
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := line[len(prefix):]
		if strings.TrimSpace(rest) != "" {
			raw := strings.TrimPrefix(rest, " ") // the writer emits one space
			return decodeScalarValue(raw)
		}
		// Nothing after the colon: block form when indented children
		// follow, else an unclaimed empty value.
		block := []string{rest}
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "" || (lines[j][0] != ' ' && lines[j][0] != '\t') {
				break
			}
			block = append(block, lines[j])
		}
		if len(block) > 1 {
			return strings.Join(block, "\n"), true
		}
		return "", true
	}
	return "", false
}

// decodeScalarValue decodes the raw text following `key: ` on a scalar
// frontmatter line into value semantics — the read-side counterpart of
// setFrontmatterKeys's double-quoted writes: double-quoted values go
// through strconv.Unquote (%q escapes, matching the writer), values in
// single quotes strip the quotes and collapse each doubled quote into
// one, and bare text is trimmed. A double-quoted value Unquote cannot
// parse is returned verbatim so the caller treats it as claimed — fail
// closed rather than clobber — which is why ok is always true: every
// raw form yields a claimed value, never an absence, and the return
// pair matches frontmatterScalar's (value, found) so the caller
// forwards it directly. Split out of frontmatterScalar to keep each
// function's cyclomatic complexity below the Gaze CRAP flag threshold
// the CI ratchet enforces. There is no error return; the caller owns
// the fail-closed decision.
func decodeScalarValue(raw string) (decoded string, ok bool) {
	switch {
	case strings.HasPrefix(raw, `"`):
		if decoded, err := strconv.Unquote(raw); err == nil {
			return decoded, true
		}
		return raw, true // unparseable: verbatim, the caller treats it as claimed
	case len(raw) >= 2 && strings.HasPrefix(raw, `'`) && strings.HasSuffix(raw, `'`):
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", `'`), true
	default:
		return strings.TrimSpace(raw), true
	}
}
