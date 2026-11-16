// Unit tests for the scaffold marker and ownership-classification core
// built by tasks 1.1-1.2 of the tool-owned-scaffold-updates change
// (openspec/changes/tool-owned-scaffold-updates, design D1-D3):
// canonical-body normalization, marker rendering in both comment
// syntaxes, marker parsing and round-trips, digest verifiability against
// an independently computed SHA-256, the full classifyScaffold decision
// table (DIO-FR-004), version-independence of classification
// (DIO-FR-003), and YAML resilience of the marked real workflow
// template (DIO-FR-002). The task 2.3 tests below cover the shared
// writer (scaffoldToolOwnedFiles) and its factored create arm over
// t.TempDir() trees: every decision-table outcome, the write-failure
// discipline (DI-FR-001), and the input-acquisition edges of DIO-FR-004
// (symlink, unreadable) plus the EEXIST concurrent-create race.
//
// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel(). The
// writer tests mutate process-global state: logger (for log-output
// assertions, via captureLoggerOutput). Running them in parallel with
// other package tests would cause data races and flaky failures.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// captureLoggerOutput redirects the package logger into a fresh buffer for
// the duration of t and returns the buffer, so writer tests can assert the
// outcome events scaffoldToolOwnedFiles emits (DIO-FR-006: `scaffolded`,
// `updated`, `skip (customized)`, `skip (concurrent)`, each naming its
// path). This wraps the established cli_test.go pattern (logger.SetOutput
// plus restore) once, so every writer test restores the logger even on
// t.Fatal paths; the logger is process-global state, which is why this file
// forbids t.Parallel().
func captureLoggerOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

// TestScaffoldToolOwnedFiles_CreateWritesExactPayload verifies the create
// outcome of the shared writer (DIO-FR-004 "Missing" row): a map entry whose
// path is absent is written as exactly scaffoldPayload — canonical body plus
// the kind's marker — and the run logs `scaffolded <label>` naming the path.
// The target directory is a fresh subdirectory, matching how newInitCmd
// calls the writer with .opencode/commands/ before that directory exists.
func TestScaffoldToolOwnedFiles_CreateWritesExactPayload(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		body  string
		kind  scaffoldKind
		label string
	}{
		{
			name:  "markdown kind writes the HTML-comment marker",
			file:  "dewey-new.md",
			body:  "---\ndescription: test\n---\n\ncommand body\n",
			kind:  scaffoldMarkdown,
			label: "slash command",
		},
		{
			name:  "yaml kind writes the hash-comment marker",
			file:  "dewey-distill.yml.example",
			body:  "name: dewey-distill\non:\n  schedule:\n    - cron: '0 8 * * 1'\n",
			kind:  scaffoldYAML,
			label: "workflow template example",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "scaffold")
			logBuf := captureLoggerOutput(t)

			if err := scaffoldToolOwnedFiles(dir, map[string]string{tt.file: tt.body}, tt.kind, tt.label); err != nil {
				t.Fatalf("scaffoldToolOwnedFiles(create) failed: %v", err)
			}

			path := filepath.Join(dir, tt.file)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read scaffolded file: %v", err)
			}
			want := scaffoldPayload([]byte(tt.body), tt.kind)
			if !bytes.Equal(got, want) {
				t.Errorf("created file = %q, want the exact scaffoldPayload %q", got, want)
			}

			logs := logBuf.String()
			if !strings.Contains(logs, "scaffolded "+tt.label) {
				t.Errorf("create must log `scaffolded %s`, got:\n%s", tt.label, logs)
			}
			if !strings.Contains(logs, path) {
				t.Errorf("scaffolded event must name the path %s, got:\n%s", path, logs)
			}
		})
	}
}

// TestScaffoldToolOwnedFiles_UpdateReplacesContentAndRefreshesMarker
// verifies the DIO-FR-004 "Untouched, drifted" row end to end: a
// marker-valid scaffold of an older embedded body is rewritten to the
// current embedded payload, and the rewrite carries a fresh marker whose
// digest matches the new payload — not the old one (DIO-FR-002 "Rewritten
// file carries a fresh marker").
func TestScaffoldToolOwnedFiles_UpdateReplacesContentAndRefreshesMarker(t *testing.T) {
	dir := t.TempDir()
	older := "---\ndescription: test\n---\n\nbody of an older embedded version\n"
	current := "---\ndescription: test\n---\n\nbody of the current embedded version\n"
	path := filepath.Join(dir, "dewey-drifted.md")

	// The fixture is what an untouched scaffold of an older Dewey looks
	// like: the marker digest still matches the payload it was written
	// with, but that payload differs from the current embedded body.
	drifted := scaffoldPayload([]byte(older), scaffoldMarkdown)
	if err := os.WriteFile(path, drifted, 0o644); err != nil {
		t.Fatalf("write drifted scaffold: %v", err)
	}
	oldDigest, _, ok := parseScaffoldMarker(drifted)
	if !ok || oldDigest != sha256Hex(canonicalBody(older)) {
		t.Fatal("fixture precondition: drifted scaffold must carry a marker valid for the older body")
	}

	logBuf := captureLoggerOutput(t)
	if err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-drifted.md": current}, scaffoldMarkdown, "slash command"); err != nil {
		t.Fatalf("scaffoldToolOwnedFiles(update) failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if want := scaffoldPayload([]byte(current), scaffoldMarkdown); !bytes.Equal(got, want) {
		t.Errorf("updated file = %q, want the current embedded scaffoldPayload %q", got, want)
	}

	// The marker was refreshed with the rewrite: it now verifies against
	// the new payload and no longer carries the old body's digest.
	newDigest, newPayload, ok := parseScaffoldMarker(got)
	if !ok {
		t.Fatalf("updated file does not end in a parseable marker:\n%s", got)
	}
	if want := sha256Hex(canonicalBody(current)); newDigest != want {
		t.Errorf("refreshed marker digest = %q, want %q (the new payload's digest)", newDigest, want)
	}
	if newDigest == oldDigest {
		t.Error("marker digest was not refreshed: the rewrite kept the old body's digest")
	}
	if !bytes.Equal(newPayload, canonicalBody(current)) {
		t.Errorf("updated marker payload = %q, want the current canonical body %q", newPayload, canonicalBody(current))
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "updated slash command") {
		t.Errorf("update must log `updated slash command`, got:\n%s", logs)
	}
	if !strings.Contains(logs, path) {
		t.Errorf("updated event must name the path %s, got:\n%s", path, logs)
	}
}

// TestScaffoldToolOwnedFiles_CurrentLeavesFileUntouched verifies the
// DIO-FR-004 "Untouched (digest matches), current" row: a scaffold already
// equal to the embedded payload is neither rewritten (bytes and mtime are
// preserved) nor logged (idempotent silence, design D4).
func TestScaffoldToolOwnedFiles_CurrentLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	body := "---\ndescription: test\n---\n\ncurrent embedded body\n"
	path := filepath.Join(dir, "dewey-current.md")
	want := scaffoldPayload([]byte(body), scaffoldMarkdown)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write current scaffold: %v", err)
	}

	// Pin a recognizable old mtime, then re-stat it: the filesystem may
	// round the stored timestamp, so the comparison baseline is what the
	// filesystem actually kept, not what was requested. Any rewrite —
	// create or temp-and-rename — would set a fresh mtime hours away.
	forced := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, forced, forced); err != nil {
		t.Fatalf("pin mtime: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before run: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	if err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-current.md": body}, scaffoldMarkdown, "slash command"); err != nil {
		t.Fatalf("scaffoldToolOwnedFiles(current) failed: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after run: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("current scaffold was rewritten: mtime changed from %v to %v", before.ModTime(), after.ModTime())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current scaffold: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("current scaffold bytes changed: got %q, want %q", got, want)
	}

	// Idempotent silence (D4): no outcome event for a current scaffold.
	logs := logBuf.String()
	for _, event := range []string{"scaffolded", "updated", "skip"} {
		if strings.Contains(logs, event) {
			t.Errorf("current scaffold must log nothing; found %q in:\n%s", event, logs)
		}
	}
}

// TestScaffoldToolOwnedFiles_SkipLeavesCustomizedFileByteIdentical verifies
// the DIO-FR-004 "Otherwise" row: a marker-less customized file is preserved
// byte-for-byte and the run logs `skip (customized)` naming the path.
func TestScaffoldToolOwnedFiles_SkipLeavesCustomizedFileByteIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dewey-custom.md")
	custom := []byte("# my customized command\n\nlocal edits the user made\n")
	if err := os.WriteFile(path, custom, 0o644); err != nil {
		t.Fatalf("write customized scaffold: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	if err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-custom.md": "embedded body\n"}, scaffoldMarkdown, "slash command"); err != nil {
		t.Fatalf("scaffoldToolOwnedFiles(skip) failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read customized scaffold: %v", err)
	}
	if !bytes.Equal(got, custom) {
		t.Errorf("customized file was modified: got %q, want byte-identical %q", got, custom)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "skip (customized)") {
		t.Errorf("customized scaffold must log `skip (customized)`, got:\n%s", logs)
	}
	if !strings.Contains(logs, path) {
		t.Errorf("skip event must name the path %s, got:\n%s", path, logs)
	}
	if strings.Contains(logs, "updated") {
		t.Errorf("customized scaffold must not be rewritten; `updated` logged:\n%s", logs)
	}
}

// TestScaffoldToolOwnedFiles_FailedCreateFailsNamingPathWithoutResidue
// verifies the DI-FR-001 write-failure discipline for the create arm over a
// read-only directory: the run fails naming the target path and leaves no
// partial file behind, so a later run cannot mistake residue for a
// user-owned file. With a read-only directory the exclusive open itself
// fails, so the no-residue postcondition is what the rollback discipline
// guarantees here; the remove-on-write-failure branch inside the arm
// enforces the same postcondition when a write fails mid-file.
func TestScaffoldToolOwnedFiles_FailedCreateFailsNamingPathWithoutResidue(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: read-only directory does not reject writes")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("make directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	path := filepath.Join(dir, "dewey-new.md")
	err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-new.md": "embedded body\n"}, scaffoldMarkdown, "slash command")
	if err == nil {
		t.Fatal("expected create to fail on a read-only directory")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("create failure %q does not name the target path %s", err.Error(), path)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("failed create left a partial file behind")
	} else if !os.IsNotExist(statErr) {
		t.Errorf("stat after failed create: %v", statErr)
	}
}

// TestScaffoldToolOwnedFiles_FailedUpdateLeavesOriginalIntact verifies the
// DI-FR-001 overwrite-failure discipline: when a tool-owned file classified
// for update cannot be replaced — a read-only directory blocks the temp
// file the atomic rename needs — the run fails naming the path, the
// pre-existing file's content survives byte-for-byte, and no
// `.dewey-scaffold-*` temp residue remains in the directory.
func TestScaffoldToolOwnedFiles_FailedUpdateLeavesOriginalIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: read-only directory does not reject writes")
	}
	dir := t.TempDir()
	older := "body of an older embedded version\n"
	current := "body of the current embedded version\n"
	path := filepath.Join(dir, "dewey-drifted.md")

	// A marker-valid drifted scaffold classifies for update; the file
	// itself stays readable (0o644) because only the directory is
	// read-only.
	drifted := scaffoldPayload([]byte(older), scaffoldMarkdown)
	if err := os.WriteFile(path, drifted, 0o644); err != nil {
		t.Fatalf("write drifted scaffold: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("make directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-drifted.md": current}, scaffoldMarkdown, "slash command")
	if err == nil {
		t.Fatal("expected update to fail on a read-only directory")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("update failure %q does not name the target path %s", err.Error(), path)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read original after failed update: %v", readErr)
	}
	if !bytes.Equal(got, drifted) {
		t.Errorf("failed update modified the original file: got %q, want %q", got, drifted)
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read directory after failed update: %v", readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".dewey-scaffold-") {
			t.Errorf("failed update left temp residue %s behind", e.Name())
		}
	}
}

// TestScaffoldToolOwnedFiles_SymlinkedPathIsPreserved verifies the DIO-FR-004
// symlink row: a tool-owned path that is a symlink — here to a marker-valid
// drifted payload, the sharpest fixture, because content classification
// alone would choose update — is treated as customized. The symlink is not
// replaced by a regular file, nothing is written through it to the referent,
// and `skip (customized)` is logged naming the path.
func TestScaffoldToolOwnedFiles_SymlinkedPathIsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	older := "body of an older shared curator\n"
	current := "body of the current embedded curator\n"

	// The referent is the canonical copy the user shares across repos.
	referent := filepath.Join(tmpDir, "shared", "curator.md")
	if err := os.MkdirAll(filepath.Dir(referent), 0o755); err != nil {
		t.Fatalf("create shared dir: %v", err)
	}
	drifted := scaffoldPayload([]byte(older), scaffoldMarkdown)
	if err := os.WriteFile(referent, drifted, 0o644); err != nil {
		t.Fatalf("write referent: %v", err)
	}

	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	linkPath := filepath.Join(agentsDir, "curator.md")
	if err := os.Symlink(referent, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	if err := scaffoldToolOwnedFiles(agentsDir, map[string]string{"curator.md": current}, scaffoldMarkdown, "agent"); err != nil {
		t.Fatalf("symlinked tool-owned path must not fail the run: %v", err)
	}

	// The path is still a symlink pointing at the same referent: neither
	// a temp-and-rename replace (which swaps the link itself for a
	// regular file) nor an open-for-write (which writes through the link)
	// happened.
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was replaced by a regular file (mode %s)", info.Mode())
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != referent {
		t.Errorf("symlink target = %q, want unchanged %q", target, referent)
	}

	got, err := os.ReadFile(referent)
	if err != nil {
		t.Fatalf("read referent: %v", err)
	}
	if !bytes.Equal(got, drifted) {
		t.Errorf("referent was written through the symlink: got %q, want %q", got, drifted)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "skip (customized)") {
		t.Errorf("symlinked path must log `skip (customized)`, got:\n%s", logs)
	}
	if !strings.Contains(logs, linkPath) {
		t.Errorf("skip event must name the path %s, got:\n%s", linkPath, logs)
	}
	if strings.Contains(logs, "updated") {
		t.Errorf("symlinked path must not be rewritten; `updated` logged:\n%s", logs)
	}
}

// TestScaffoldToolOwnedFiles_UnreadableExistingFileFailsNamingPath verifies
// the DIO-FR-004 unreadable row: an existing tool-owned path whose content
// cannot be read fails the run naming the path, and the failure is never
// mislabeled as a customization — no `skip (customized)` event is logged —
// because an environment failure masked as a user edit would hide the
// problem from the operator.
func TestScaffoldToolOwnedFiles_UnreadableExistingFileFailsNamingPath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0o000 does not prevent reading")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dewey-unreadable.md")
	custom := []byte("# customized command the run cannot read\n")
	if err := os.WriteFile(path, custom, 0o644); err != nil {
		t.Fatalf("write unreadable fixture: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("make fixture unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	logBuf := captureLoggerOutput(t)
	err := scaffoldToolOwnedFiles(dir, map[string]string{"dewey-unreadable.md": "embedded body\n"}, scaffoldMarkdown, "slash command")
	if err == nil {
		t.Fatal("expected unreadable existing file to fail the run")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("read failure %q does not name the path %s", err.Error(), path)
	}

	logs := logBuf.String()
	if strings.Contains(logs, "skip (customized)") {
		t.Errorf("read failure was mislabeled as a customization; got:\n%s", logs)
	}

	// The file was not modified: restore readability and compare bytes.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("restore readability: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read fixture after run: %v", readErr)
	}
	if !bytes.Equal(got, custom) {
		t.Errorf("unreadable file was modified: got %q, want %q", got, custom)
	}
}

// TestCreateScaffoldFile_ConcurrentCreateIsBenignRace verifies the DI-FR-001
// EEXIST exception: invoking the factored create arm against an
// already-present path — what losing the create race to a concurrent
// `dewey init` looks like — does not fail, does not replace the concurrent
// writer's file, and logs `skip (concurrent)` naming the path.
func TestCreateScaffoldFile_ConcurrentCreateIsBenignRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dewey-store.md")
	concurrent := []byte("file the concurrent init run wrote\n")
	if err := os.WriteFile(path, concurrent, 0o644); err != nil {
		t.Fatalf("write concurrent winner's file: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	payload := scaffoldPayload([]byte("embedded body\n"), scaffoldMarkdown)
	if err := createScaffoldFile(path, payload, "slash command"); err != nil {
		t.Fatalf("EEXIST is a benign race and must not fail: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file after lost race: %v", err)
	}
	if !bytes.Equal(got, concurrent) {
		t.Errorf("lost race replaced the concurrent writer's file: got %q, want %q", got, concurrent)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "skip (concurrent)") {
		t.Errorf("lost race must log `skip (concurrent)`, got:\n%s", logs)
	}
	if !strings.Contains(logs, path) {
		t.Errorf("skip (concurrent) event must name the path %s, got:\n%s", path, logs)
	}
	if strings.Contains(logs, "scaffolded") {
		t.Errorf("lost race must not log a `scaffolded` event; got:\n%s", logs)
	}
}
