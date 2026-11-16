package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// scaffoldKind selects the comment syntax of the scaffold marker for a
// scaffolded file kind, so the marker stays invisible to the host parser
// of each kind (design D1): an HTML comment for markdown payloads, a `#`
// comment for YAML payloads.
type scaffoldKind int

const (
	// scaffoldMarkdown is the marker kind for markdown payloads (slash
	// commands, agent definitions). It is the zero value of scaffoldKind
	// and the default form when rendering markers.
	scaffoldMarkdown scaffoldKind = iota

	// scaffoldYAML is the marker kind for YAML payloads (workflow
	// templates); an HTML comment would corrupt YAML, so these files
	// carry a `#` comment instead.
	scaffoldYAML
)

// canonicalBody normalizes an embedded scaffold map value to the canonical
// payload form: bytes ending with exactly one trailing newline (design D2,
// DIO-FR-003). A value lacking a trailing newline gains one; a value ending
// in several keeps exactly one. Marker digests are always computed over
// this form, so the write side and the classify side agree on one payload
// shape. The normalization is idempotent: canonicalizing its own output
// returns equal bytes.
func canonicalBody(v string) []byte {
	return []byte(strings.TrimRight(v, "\n") + "\n")
}

// scaffoldMarker renders the final-line ownership marker for a scaffold
// body (design D1/D2): "<!-- dewey:scaffold <version> sha256:<hex> -->"
// for markdown payloads and "# dewey:scaffold <version> sha256:<hex>" for
// YAML payloads. The version comes from the version var (ldflags-injected;
// informational provenance only — it never affects classification, D8) and
// the digest is the lowercase hex SHA-256 of the canonical body, making
// the marker self-verifying against the payload it is appended to
// (DIO-FR-002). body is canonicalized before hashing; for canonicalBody
// output that is a no-op, so callers may pass either the raw map value or
// its canonical form and get the same marker.
func scaffoldMarker(body []byte, kind scaffoldKind) string {
	sum := sha256.Sum256(canonicalBody(string(body)))
	digest := hex.EncodeToString(sum[:])
	if kind == scaffoldYAML {
		return fmt.Sprintf("# dewey:scaffold %s sha256:%s", version, digest)
	}
	// scaffoldMarkdown (and the zero value) renders the HTML-comment form.
	return fmt.Sprintf("<!-- dewey:scaffold %s sha256:%s -->", version, digest)
}

// scaffoldPayload renders the complete bytes of a tool-owned write: the
// canonical body, then the marker line, then a final newline (design D2:
// canonicalBody + marker + "\n"). Because the marker is the file's final
// line, feeding the result back through parseScaffoldMarker yields the
// same canonical body and the marker's digest — writes round-trip exactly.
func scaffoldPayload(body []byte, kind scaffoldKind) []byte {
	canonical := canonicalBody(string(body))
	marker := scaffoldMarker(canonical, kind)
	payload := make([]byte, 0, len(canonical)+len(marker)+1)
	payload = append(payload, canonical...)
	payload = append(payload, marker...)
	return append(payload, '\n')
}

// scaffoldMarkerRe is the read-side marker grammar from design D1. It
// accepts either comment form on the final line of a file:
//
//	^\s*(?:<!--\s*|#\s*)dewey:scaffold\s+(\S+)\s+sha256:([0-9a-f]{64})\s*(?:-->)?\s*$
//
// Group 1 captures the writing binary's version (informational only);
// group 2 captures the 64-lowercase-hex payload digest. The regexp is
// compiled once and never mutated, and regexp.Regexp is safe for
// concurrent use, keeping convention CS-007's no-mutable-state intent.
var scaffoldMarkerRe = regexp.MustCompile(`^\s*(?:<!--\s*|#\s*)dewey:scaffold\s+(\S+)\s+sha256:([0-9a-f]{64})\s*(?:-->)?\s*$`)

// parseScaffoldMarker extracts the scaffold marker from the final line of
// content, accepting either comment form per the D1 grammar. When the
// final line is a well-formed marker it returns the marker's digest (64
// lowercase hex characters) and the exact payload bytes preceding the
// marker line, with ok true; otherwise it returns ("", nil, false). The
// marker's version is deliberately not returned — classification depends
// only on the digest (D8, DIO-FR-003). The returned payload aliases
// content and must be treated as read-only.
func parseScaffoldMarker(content []byte) (digest string, payload []byte, ok bool) {
	// The marker is the file's final line. Drop that line's terminating
	// newline, then take everything after the last remaining newline as
	// the candidate marker line; the bytes before it are the payload.
	body := bytes.TrimSuffix(content, []byte("\n"))
	markerStart := 0
	lastLine := body
	if idx := bytes.LastIndexByte(body, '\n'); idx >= 0 {
		markerStart = idx + 1
		lastLine = body[markerStart:]
	}
	m := scaffoldMarkerRe.FindSubmatch(lastLine)
	if m == nil {
		return "", nil, false
	}
	return string(m[2]), content[:markerStart], true
}

// scaffoldDecision is the refresh outcome classifyScaffold selects for one
// tool-owned scaffold path (design D3, DIO-FR-004). The shared writer
// (scaffoldToolOwnedFiles, design D4) maps each value to its filesystem
// action and its observability event.
type scaffoldDecision int

const (
	// scaffoldCreate reports a missing path (D3 case 1): the writer
	// scaffolds the canonical payload plus marker and logs `scaffolded`.
	scaffoldCreate scaffoldDecision = iota

	// scaffoldUpdate reports a verifiably tool-owned path that needs a
	// rewrite (D3 cases 2 and 3): either the marker digest matches but the
	// payload differs from the embedded body (untouched, drifted), or the
	// file has no marker and is byte-identical to the embedded body (marker
	// bootstrap). The writer rewrites the canonical payload with a fresh
	// marker and logs `updated`.
	scaffoldUpdate

	// scaffoldCurrent reports a verifiably tool-owned path already equal to
	// the embedded body (D3 case 2): the writer performs no write and logs
	// nothing (idempotent silence).
	scaffoldCurrent

	// scaffoldSkip reports a customized path (D3 case 4): marker absent
	// with differing content, a malformed marker, or a digest mismatch. The
	// writer leaves the file untouched and logs `skip (customized)`.
	scaffoldSkip
)

// classifyScaffold decides how a `dewey init` run refreshes one tool-owned
// scaffold path from its content alone (design D3, DIO-FR-004). It is a pure
// function — bytes in, decision out — so every decision-table row is directly
// table-testable without filesystem fixtures; the D4 writer owns input
// acquisition and the failure classes this table does not enumerate (symlink,
// unreadable, stat errors).
//
// The decision table:
//
//  1. !exists → scaffoldCreate.
//  2. The final line parses as a marker and the SHA-256 of the payload
//     preceding it equals the marker digest → verifiably untouched:
//     scaffoldCurrent when that payload is byte-equal to
//     canonicalBody(embeddedBody), else scaffoldUpdate.
//  3. No parseable marker and existing is byte-equal to
//     canonicalBody(embeddedBody) → scaffoldUpdate (marker bootstrap: the
//     only rewrite this arm performs re-adds the marker to content the run
//     would write anyway).
//  4. Otherwise → scaffoldSkip (customized): marker absent with differing
//     content, malformed marker, or digest mismatch.
//
// Classification never reads the marker's version field (D8, DIO-FR-003):
// parseScaffoldMarker discards it, so version-string churn cannot change an
// ownership outcome. The digest comparison hashes the exact payload bytes
// preceding the marker — deliberately not recanonicalized — so any post-write
// body edit, including trailing whitespace, breaks the comparison and reads
// as customized. The currency check in case 2 is byte equality, not hash
// equality, so no collision reasoning is ever load-bearing.
func classifyScaffold(existing []byte, exists bool, embeddedBody []byte) scaffoldDecision {
	if !exists {
		return scaffoldCreate
	}
	canonical := canonicalBody(string(embeddedBody))
	if digest, payload, ok := parseScaffoldMarker(existing); ok {
		sum := sha256.Sum256(payload)
		if hex.EncodeToString(sum[:]) != digest {
			// The body was edited after scaffolding: customized (case 4).
			return scaffoldSkip
		}
		if bytes.Equal(payload, canonical) {
			return scaffoldCurrent
		}
		return scaffoldUpdate
	}
	if bytes.Equal(existing, canonical) {
		return scaffoldUpdate
	}
	return scaffoldSkip
}

// scaffoldToolOwnedFiles drives one embedded scaffold map through the shared
// tool-owned write discipline of design D4 (DI-FR-001, DIO-FR-004,
// DIO-FR-006): for every name-to-body entry in files it manages dir/name —
// creating it with the kind's marker plus canonical payload when missing,
// refreshing it when verifiably untouched but drifted, and preserving it
// when customized. Entries are visited in sorted key order so outcome
// events, and any failure the run reports, are deterministic across runs.
// dir is created (0o755) when absent; a creation failure fails the run
// naming the directory.
//
// Per path, classification inputs are acquired per design D3's
// input-acquisition rules (classifyScaffoldPath): os.Lstat first — a
// symlinked path is skipped as customized without reading or writing
// through it — then os.ReadFile, whose failure fails the run naming the
// path rather than masquerading as a customization. The selected decision
// maps to its filesystem action and observability event:
//
//   - scaffoldCreate: exclusive create via createScaffoldFile (logs
//     `scaffolded <label>`, or `skip (concurrent)` when a concurrent init
//     wins the create race);
//   - scaffoldUpdate: atomic temp-and-rename replace via updateScaffoldFile
//     (logs `updated <label>`);
//   - scaffoldSkip: no filesystem mutation, `skip (customized)` logged;
//   - scaffoldCurrent: no write, no event (idempotent silence).
//
// label carries the kind context for outcome events ("slash command",
// "agent", "workflow template example"), keeping the log lines the init
// command already emits for each kind.
func scaffoldToolOwnedFiles(dir string, files map[string]string, kind scaffoldKind, label string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	// Iterate in sorted name order: outcomes and any failure the run
	// reports stay deterministic — map order would randomize which path a
	// write failure names.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		body := []byte(files[name])
		decision, err := classifyScaffoldPath(path, body)
		if err != nil {
			return err
		}
		switch decision {
		case scaffoldCreate:
			if err := createScaffoldFile(path, scaffoldPayload(body, kind), label); err != nil {
				return err
			}
		case scaffoldUpdate:
			if err := updateScaffoldFile(dir, path, scaffoldPayload(body, kind), label); err != nil {
				return err
			}
		case scaffoldSkip:
			logger.Info("skip (customized)", "path", path)
		case scaffoldCurrent:
			// Idempotent silence (D4): a verifiably current scaffold
			// is neither written nor logged.
		}
	}
	return nil
}

// classifyScaffoldPath acquires the classification inputs for one
// tool-owned path from the filesystem, wrapping the pure classifyScaffold
// decision table with the input-acquisition rules of design D3 that the
// table does not enumerate (DIO-FR-004's symlink and unreadable rows). The
// decision value is meaningful only when the returned error is nil; a
// non-nil error fails the run naming the path.
//
// os.Lstat is deliberate — never os.Stat — so a symlinked tool-owned path
// is classified as customized without reading content and without writing
// through the link: the symlink is never replaced by a regular file and
// its referent is never touched, matching the TOCTOU posture of promote's
// symlink guard (PW-FR-004). A path that becomes a symlink between this
// check and a later write is likewise not followed — no write ever reaches
// the referent.
//
// A missing path (Lstat reports not-exist) classifies as scaffoldCreate.
// Any other Lstat failure, or a ReadFile failure on an existing path — a
// permission error, a directory sitting at the path, a transient I/O error
// — fails the run naming the path: an environment failure must never be
// mislabeled as the customized outcome, which would mask it from the
// operator.
func classifyScaffoldPath(path string, embeddedBody []byte) (scaffoldDecision, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return scaffoldCreate, nil
		}
		return scaffoldSkip, fmt.Errorf("stat scaffold %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// A symlink is a user statement about the path: treat it as
		// customized without touching the link or its referent.
		return scaffoldSkip, nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return scaffoldSkip, fmt.Errorf("read scaffold %s: %w", path, err)
	}
	return classifyScaffold(existing, true, embeddedBody), nil
}

// createScaffoldFile performs the create arm of the shared scaffold
// discipline (design D4, DI-FR-001). It creates path exclusively
// (O_CREATE|O_EXCL, 0o644) holding payload, preserving the existing agent
// write discipline verbatim: a failed or partial write is rolled back — the
// partial file is removed so a later run does not mistake the residue for a
// user-owned file — and the failure fails the run naming the path.
//
// One exception is specified for the benign concurrency race: an EEXIST
// from the exclusive open means a concurrent `dewey init` created the path
// between classification and the open. The run does not fail and does not
// replace the concurrent writer's file; it logs `skip (concurrent)` at info
// naming the path and continues (the concurrent run's own `scaffolded`
// event covers observability for that path). On success it logs
// `scaffolded <label>` naming the path.
//
// The EEXIST branch is why this arm is factored out of
// scaffoldToolOwnedFiles: invoking the function against an already-present
// path makes the concurrent-create behavior directly testable without
// reproducing the race itself.
func createScaffoldFile(path string, payload []byte, label string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			logger.Info("skip (concurrent)", "path", path)
			return nil
		}
		return fmt.Errorf("scaffold %s: %w", path, err)
	}
	if _, writeErr := f.Write(payload); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("scaffold %s: %w", path, writeErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("scaffold %s: %w", path, closeErr)
	}
	logger.Info("scaffolded "+label, "path", path)
	return nil
}

// updateScaffoldFile performs the update arm of the shared scaffold
// discipline (design D4, DI-FR-001). It replaces path with payload through
// a temp file created in the target directory followed by an atomic
// os.Rename, so a failed update leaves the original file intact and no
// partial residue: either the rename completes or the original stands.
// Creating the temp in dir keeps the rename off any filesystem boundary
// where rename would degrade to copy-and-unlink. The temp uses a hidden
// `.dewey-scaffold-*` name — crash residue is never mistaken for user
// content — and is chmod'd to 0o644 to match the create arm (CreateTemp
// defaults to 0o600). Any failure — temp creation, write, chmod, close, or
// rename — removes the temp file and fails the run naming the path, so the
// original file is never lost. On success it logs `updated <label>` naming
// the path. The rename never executes against a path classified as a
// symlink: the Lstat gate in classifyScaffoldPath precedes it.
func updateScaffoldFile(dir, path string, payload []byte, label string) error {
	tmp, err := os.CreateTemp(dir, ".dewey-scaffold-*")
	if err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("update %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("update %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("update %s: %w", path, err)
	}
	// Atomic on the supported platforms: path ends up fully the new
	// payload or fully the original file.
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("update %s: %w", path, err)
	}
	logger.Info("updated "+label, "path", path)
	return nil
}
