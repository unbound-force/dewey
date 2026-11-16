package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
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
