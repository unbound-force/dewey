// Unit tests for the scaffold marker and ownership-classification core
// built by tasks 1.1-1.2 of the tool-owned-scaffold-updates change
// (openspec/changes/tool-owned-scaffold-updates, design D1-D3):
// canonical-body normalization, marker rendering in both comment
// syntaxes, marker parsing and round-trips, digest verifiability against
// an independently computed SHA-256, the full classifyScaffold decision
// table (DIO-FR-004), version-independence of classification
// (DIO-FR-003), and YAML resilience of the marked real workflow
// template (DIO-FR-002). The filesystem writer and its outcome events
// are tasks 2.1/2.3 and are tested there.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sha256Hex returns the lowercase hex SHA-256 of b. Tests compute
// digests through this helper rather than through scaffold.go's own
// hashing, so marker assertions verify the digest against the
// crypto/sha256 definition instead of against the code under test.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// markdownMarkerLine and yamlMarkerLine hand-build final-line markers
// per the D1 grammar, pinning an explicit version string: the version
// package var is global state (CS-007) and cannot be varied per test
// without mutating it.
func markdownMarkerLine(ver, digest string) string {
	return fmt.Sprintf("<!-- dewey:scaffold %s sha256:%s -->", ver, digest)
}

// yamlMarkerLine builds the `#`-comment marker form (design D1) for
// YAML payloads, where an HTML comment would corrupt the document.
func yamlMarkerLine(ver, digest string) string {
	return fmt.Sprintf("# dewey:scaffold %s sha256:%s", ver, digest)
}

// withFinalLine builds file content whose final line is line: payload,
// then line, then the line's terminating newline. payload must end with
// a newline for parseScaffoldMarker to return it verbatim as the
// payload preceding the marker.
func withFinalLine(payload []byte, line string) []byte {
	out := make([]byte, 0, len(payload)+len(line)+1)
	out = append(out, payload...)
	out = append(out, line...)
	return append(out, '\n')
}

// decisionName renders a scaffoldDecision for assertion messages. The
// type carries no String method (scaffold.go holds only production
// code), so the test file owns the rendering.
func decisionName(d scaffoldDecision) string {
	switch d {
	case scaffoldCreate:
		return "scaffoldCreate"
	case scaffoldUpdate:
		return "scaffoldUpdate"
	case scaffoldCurrent:
		return "scaffoldCurrent"
	case scaffoldSkip:
		return "scaffoldSkip"
	}
	return fmt.Sprintf("scaffoldDecision(%d)", int(d))
}

// TestCanonicalBody_TrailingNewlineNormalization verifies design D2's
// canonical payload form (DIO-FR-003): the map value normalized to end
// with exactly one trailing newline, so the write side and the classify
// side agree on one payload shape regardless of how the raw value ends.
func TestCanonicalBody_TrailingNewlineNormalization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"value without trailing newline gains one", "command body", "command body\n"},
		{"value ending in exactly one newline is unchanged", "command body\n", "command body\n"},
		{"value ending in several newlines keeps exactly one", "command body\n\n\n", "command body\n"},
		{"multi-line value keeps interior newlines", "first\nsecond", "first\nsecond\n"},
		{"empty value normalizes to a single newline", "", "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalBody(tt.in)
			if string(got) != tt.want {
				t.Errorf("canonicalBody(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// The normalization is idempotent (D2): canonicalizing its
			// own output is a no-op, or marker digests would depend on
			// how many times a value had been normalized.
			if again := canonicalBody(string(got)); string(again) != tt.want {
				t.Errorf("canonicalBody not idempotent: canonicalBody(canonicalBody(%q)) = %q, want %q", tt.in, again, tt.want)
			}
		})
	}
}

// TestScaffoldMarker_DigestMatchesIndependentSHA256 verifies marker
// digests against a SHA-256 computed in the test (DIO-FR-002: the
// marker is self-verifying against the payload it is appended to). The
// body is chosen already canonical, so the expected digest is the plain
// hash of those bytes, and each kind's marker must be the exact D1 form
// carrying that digest.
func TestScaffoldMarker_DigestMatchesIndependentSHA256(t *testing.T) {
	// Already canonical — ends with exactly one newline — so
	// canonicalBody is the identity over these bytes and the digest can
	// be computed without going through the code under test.
	body := []byte("---\nagent: curator\n---\n\n# Curator\n\nwatches the review gate.\n")
	digest := sha256Hex(body)

	t.Run("markdown kind renders the HTML-comment form", func(t *testing.T) {
		want := markdownMarkerLine(version, digest)
		if got := scaffoldMarker(body, scaffoldMarkdown); got != want {
			t.Errorf("scaffoldMarker(markdown) = %q, want %q", got, want)
		}
	})
	t.Run("yaml kind renders the hash-comment form", func(t *testing.T) {
		want := yamlMarkerLine(version, digest)
		if got := scaffoldMarker(body, scaffoldYAML); got != want {
			t.Errorf("scaffoldMarker(yaml) = %q, want %q", got, want)
		}
	})
	t.Run("non-canonical body hashes its canonical form", func(t *testing.T) {
		// The marker hashes canonicalBody(body); a body lacking (or
		// carrying extra) trailing newlines must therefore render the
		// same marker as the canonical form.
		trimmed := bytes.TrimSuffix(body, []byte("\n"))
		if got, want := scaffoldMarker(trimmed, scaffoldMarkdown), scaffoldMarker(body, scaffoldMarkdown); got != want {
			t.Errorf("scaffoldMarker(body without trailing newline) = %q, want %q (the canonical form's marker)", got, want)
		}
		padded := append(bytes.Clone(body), '\n', '\n')
		if got, want := scaffoldMarker(padded, scaffoldMarkdown), scaffoldMarker(body, scaffoldMarkdown); got != want {
			t.Errorf("scaffoldMarker(body with extra trailing newlines) = %q, want %q (the canonical form's marker)", got, want)
		}
	})
}

// TestParseScaffoldMarker_BothCommentSyntaxes verifies the read side of
// the D1 grammar: a final-line marker in either comment form — an HTML
// comment for markdown payloads, a `#` comment for YAML payloads —
// parses back to the recorded digest and the exact payload bytes
// preceding it.
func TestParseScaffoldMarker_BothCommentSyntaxes(t *testing.T) {
	payload := []byte("---\nagent: curator\n---\n\nbody the user never edited\n")
	digest := sha256Hex(payload)

	tests := []struct {
		name         string
		marker       string
		finalNewline bool
	}{
		{"HTML comment (markdown payload)", markdownMarkerLine("v1.2.3", digest), true},
		{"hash comment (YAML payload)", yamlMarkerLine("v1.2.3", digest), true},
		{"HTML comment with no trailing newline", markdownMarkerLine("v1.2.3", digest), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := withFinalLine(payload, tt.marker)
			if !tt.finalNewline {
				content = content[:len(content)-1]
			}
			gotDigest, gotPayload, ok := parseScaffoldMarker(content)
			if !ok {
				t.Fatalf("parseScaffoldMarker rejected marker %q", tt.marker)
			}
			if gotDigest != digest {
				t.Errorf("digest = %q, want %q", gotDigest, digest)
			}
			if !bytes.Equal(gotPayload, payload) {
				t.Errorf("payload = %q, want %q", gotPayload, payload)
			}
		})
	}
}

// TestParseScaffoldMarker_RejectsNonMarkerFinalLines pins the fail-safe
// side of the D1 grammar: only a final line that fully matches the
// marker grammar yields ok, so anything else falls through to the
// decision table's customized arm instead of being mistaken for a
// valid marker.
func TestParseScaffoldMarker_RejectsNonMarkerFinalLines(t *testing.T) {
	payload := []byte("command body\n")
	digest := sha256Hex(payload)
	// 64 characters of valid hex, but uppercase — the grammar requires
	// lowercase so a digest can never round-trip through hex.EncodeToString
	// and come back different.
	upperHex := strings.ToUpper(strings.Repeat("ab", 32))

	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "plain final line with no marker",
			content: withFinalLine(payload, "an ordinary closing line"),
		},
		{
			name:    "digest shorter than 64 hex characters",
			content: withFinalLine(payload, markdownMarkerLine("dev", "deadbeef")),
		},
		{
			name:    "digest with non-hex characters",
			content: withFinalLine(payload, markdownMarkerLine("dev", strings.Repeat("z", 64))),
		},
		{
			name:    "uppercase hex digest",
			content: withFinalLine(payload, markdownMarkerLine("dev", upperHex)),
		},
		{
			name:    "wrong marker token",
			content: withFinalLine(payload, fmt.Sprintf("<!-- dewey:scaffolded %s sha256:%s -->", "dev", digest)),
		},
		{
			name: "marker buried before a trailing content line",
			content: append(
				withFinalLine(payload, markdownMarkerLine("dev", digest)),
				[]byte("content after the marker\n")...,
			),
		},
		{
			name:    "empty file",
			content: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := parseScaffoldMarker(tt.content); ok {
				t.Errorf("parseScaffoldMarker accepted a non-marker final line; content:\n%s", tt.content)
			}
		})
	}
}

// TestScaffoldPayload_RoundTrip verifies design D2's write shape —
// canonical body + marker + "\n" — from the read side: scaffoldPayload
// output must end in a parseable marker whose digest matches the
// canonical body and whose preceding bytes are exactly that body, so
// writes round-trip through parseScaffoldMarker without loss.
func TestScaffoldPayload_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind scaffoldKind
	}{
		{"markdown body already canonical", "---\nagent: curator\n---\n\nbody\n", scaffoldMarkdown},
		{"markdown body without trailing newline", "---\nagent: curator\n---\n\nbody", scaffoldMarkdown},
		{"markdown body with extra trailing newlines", "---\nagent: curator\n---\n\nbody\n\n\n", scaffoldMarkdown},
		{"yaml body", "name: dewey-distill\njobs:\n  distill-trigger:\n    runs-on: ubuntu-latest\n", scaffoldYAML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := scaffoldPayload([]byte(tt.body), tt.kind)
			canonical := canonicalBody(tt.body)

			digest, payload, ok := parseScaffoldMarker(content)
			if !ok {
				t.Fatalf("scaffoldPayload output does not end in a parseable marker:\n%s", content)
			}
			if !bytes.Equal(payload, canonical) {
				t.Errorf("round-trip payload = %q, want canonical body %q", payload, canonical)
			}
			if want := sha256Hex(canonical); digest != want {
				t.Errorf("round-trip digest = %q, want independently computed %q", digest, want)
			}

			// The exact write shape (D2): everything after the payload
			// is the marker line plus one final newline — the marker is
			// the file's last line, and nothing else trails it.
			var wantMarker string
			if tt.kind == scaffoldYAML {
				wantMarker = yamlMarkerLine(version, digest)
			} else {
				wantMarker = markdownMarkerLine(version, digest)
			}
			if rest := string(content[len(payload):]); rest != wantMarker+"\n" {
				t.Errorf("scaffoldPayload trailing bytes = %q, want %q", rest, wantMarker+"\n")
			}
		})
	}
}

// TestClassifyScaffold_DecisionTable exercises every row of the D3
// decision table (DIO-FR-004) against the pure classifier — bytes in,
// decision out, no filesystem.
func TestClassifyScaffold_DecisionTable(t *testing.T) {
	currentCommand := "---\nagent: curator\n---\n\nbody of the current embedded version\n"
	olderCommand := "---\nagent: curator\n---\n\nbody of an older embedded version\n"
	currentWorkflow := "name: dewey-distill\njobs:\n  distill-trigger:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n"
	olderWorkflow := "name: dewey-distill\njobs:\n  distill-trigger:\n    runs-on: ubuntu-latest\n"

	// A post-write body edit: scaffolded from olderCommand, then the
	// payload was edited while the marker kept the digest of the
	// original payload — the spec's customized-scaffold scenario.
	editedBody := bytes.Replace(
		scaffoldPayload([]byte(olderCommand), scaffoldMarkdown),
		[]byte("older embedded"),
		[]byte("user-edited embedded"),
		1,
	)

	tests := []struct {
		name     string
		existing []byte
		exists   bool
		embedded string
		want     scaffoldDecision
	}{
		{
			name:     "missing path creates",
			existing: nil,
			exists:   false,
			embedded: currentCommand,
			want:     scaffoldCreate,
		},
		{
			name:     "untouched and current stays current",
			existing: scaffoldPayload([]byte(currentCommand), scaffoldMarkdown),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldCurrent,
		},
		{
			name:     "untouched but drifted updates",
			existing: scaffoldPayload([]byte(olderCommand), scaffoldMarkdown),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldUpdate,
		},
		{
			name:     "untouched but drifted yaml workflow updates",
			existing: scaffoldPayload([]byte(olderWorkflow), scaffoldYAML),
			exists:   true,
			embedded: currentWorkflow,
			want:     scaffoldUpdate,
		},
		{
			name:     "marker-less byte-identical bootstraps the marker",
			existing: canonicalBody(currentCommand),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldUpdate,
		},
		{
			name:     "marker-less differing content is customized",
			existing: []byte("# user-customized command\n\nlocal edits\n"),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldSkip,
		},
		{
			name:     "marker-less content differing only by the trailing newline is customized",
			existing: []byte(strings.TrimSuffix(currentCommand, "\n")),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldSkip,
		},
		{
			name:     "existing empty file is customized",
			existing: nil,
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldSkip,
		},
		{
			name:     "malformed digest is customized",
			existing: withFinalLine(canonicalBody(olderCommand), markdownMarkerLine("dev", "deadbeef")),
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldSkip,
		},
		{
			name:     "digest mismatch after a body edit is customized",
			existing: editedBody,
			exists:   true,
			embedded: currentCommand,
			want:     scaffoldSkip,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyScaffold(tt.existing, tt.exists, []byte(tt.embedded))
			if got != tt.want {
				t.Errorf("classifyScaffold() = %s, want %s", decisionName(got), decisionName(tt.want))
			}
		})
	}
}

// TestClassifyScaffold_VersionIndependence enforces DIO-FR-003 (design
// D8): classification depends only on the digest, never the recorded
// version. Markers are hand-built per the D1 grammar so the same
// payload carries several different version strings (and both comment
// forms); every one must yield the same decision.
func TestClassifyScaffold_VersionIndependence(t *testing.T) {
	currentBody := "---\nagent: distiller\n---\n\nbody text\n"
	driftedBody := "---\nagent: distiller\n---\n\nnewer body text\n"
	canonical := canonicalBody(currentBody)
	digest := sha256Hex(canonical)

	markers := []string{
		markdownMarkerLine("dev", digest),
		markdownMarkerLine("v1.0.0", digest),
		markdownMarkerLine("v3.6.0-rc.1+build.42", digest),
		yamlMarkerLine("v99.0.0", digest),
	}

	scenarios := []struct {
		name     string
		embedded string
		want     scaffoldDecision
	}{
		{"current embedded body stays current", currentBody, scaffoldCurrent},
		{"drifted embedded body updates", driftedBody, scaffoldUpdate},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, marker := range markers {
				existing := withFinalLine(canonical, marker)
				got := classifyScaffold(existing, true, []byte(sc.embedded))
				if got != sc.want {
					t.Errorf("classification under marker %q = %s, want %s (the version string must not affect the decision)", marker, decisionName(got), decisionName(sc.want))
				}
			}
		})
	}
}

// TestScaffoldPayload_WorkflowTemplateRemainsYAML enforces the
// DIO-FR-002 scenario "Workflow template marker is a YAML comment": the
// scaffolded form of the real deweyWorkflowTemplates entry —
// scaffoldPayload with the yaml kind — must remain parseable YAML after
// the marker append, with the workflow's structure intact, because
// GitHub Actions consumes the scaffolded example directly.
func TestScaffoldPayload_WorkflowTemplateRemainsYAML(t *testing.T) {
	entry, ok := deweyWorkflowTemplates["dewey-distill.yml.example"]
	if !ok {
		t.Fatal("deweyWorkflowTemplates missing dewey-distill.yml.example entry")
	}

	content := scaffoldPayload([]byte(entry), scaffoldYAML)

	// The scaffolded form ends with the `#`-comment marker as its final
	// line, and that marker verifies against the canonical map value.
	digest, payload, ok := parseScaffoldMarker(content)
	if !ok {
		t.Fatalf("scaffolded workflow template does not end in a parseable marker:\n%s", content)
	}
	canonical := canonicalBody(entry)
	if !bytes.Equal(payload, canonical) {
		t.Error("workflow template round-trip payload differs from the canonical map value")
	}
	if want := sha256Hex(canonical); digest != want {
		t.Errorf("workflow template marker digest = %q, want independently computed %q", digest, want)
	}
	if want := yamlMarkerLine(version, digest) + "\n"; !strings.HasSuffix(string(content), want) {
		t.Errorf("scaffolded workflow template must end with the yaml-form marker %q", want)
	}

	// The marked file remains parseable YAML with its structure intact:
	// the trailing `#` comment is inert to the parser.
	var wf distillWorkflow
	if err := yaml.Unmarshal(content, &wf); err != nil {
		t.Fatalf("scaffolded workflow template is no longer parseable YAML: %v", err)
	}
	if wf.Name != "dewey-distill" {
		t.Errorf("parsed workflow name = %q, want %q", wf.Name, "dewey-distill")
	}
	job, ok := wf.Jobs["distill-trigger"]
	if !ok {
		t.Fatal("scaffolded workflow template lost the distill-trigger job")
	}
	if len(job.Steps) == 0 {
		t.Error("scaffolded workflow template lost the distill-trigger steps")
	}
	// The template's last content lives inside the `run: |` block
	// scalar; a wrongly indented append would be swallowed into the
	// script. The marker must stay outside it.
	if script := job.Steps[0].Run; strings.Contains(script, "dewey:scaffold") {
		t.Errorf("marker leaked into the run block scalar:\n%s", script)
	}
}

// TestScaffoldDecision_DistinctValues guards the decision enum against
// constants collapsing onto a shared value: the decision table compares
// decisions by value, and two names sharing a value would silently make
// distinct rows (for example update and current) interchangeable.
func TestScaffoldDecision_DistinctValues(t *testing.T) {
	all := []struct {
		name string
		val  scaffoldDecision
	}{
		{"scaffoldCreate", scaffoldCreate},
		{"scaffoldUpdate", scaffoldUpdate},
		{"scaffoldCurrent", scaffoldCurrent},
		{"scaffoldSkip", scaffoldSkip},
	}
	seen := make(map[scaffoldDecision]string, len(all))
	for _, d := range all {
		if other, dup := seen[d.val]; dup {
			t.Errorf("decisions %s and %s share value %d", other, d.name, int(d.val))
		}
		seen[d.val] = d.name
	}
}
