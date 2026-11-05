package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// fixedAsOf returns an as_of resolver that always yields pin.
func fixedAsOf(pin string) func() (string, error) {
	return func() (string, error) { return pin, nil }
}

// pageProps decodes a stored page's properties JSON.
func pageProps(t *testing.T, s *store.Store, name string) (*store.Page, map[string]any) {
	t.Helper()
	page, err := s.GetPage(name)
	if err != nil || page == nil {
		t.Fatalf("GetPage(%q) = %v, %v", name, page, err)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(page.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties %q: %v", page.Properties, err)
	}
	return page, props
}

// fileFrontmatter parses the YAML frontmatter of a learning file and
// returns it with the file's body.
func fileFrontmatter(t *testing.T, path string) (map[string]any, string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		t.Fatalf("no frontmatter in %s:\n%s", path, content)
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		t.Fatalf("parse frontmatter of %s: %v\n%s", path, err, parts[1])
	}
	return fm, parts[2]
}

// resultString returns m[key] as a string, failing the test when it is not one.
func resultString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, ok := m[key].(string)
	if !ok {
		t.Fatalf("%s = %v, want a string", key, m[key])
	}
	return s
}

// storeLearning calls StoreLearning and fails the test on a Go error.
func storeLearning(t *testing.T, l *Learning, input types.StoreLearningInput) (map[string]any, bool) {
	t.Helper()
	result, _, err := l.StoreLearning(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		return map[string]any{"error": resultText(result)}, true
	}
	return parseLearningResult(t, resultText(result)), false
}

func TestValidateValidity(t *testing.T) {
	for _, v := range []string{"", "current", "stale-unverified", "superseded", "historical"} {
		if err := validateValidity(v); err != nil {
			t.Errorf("validateValidity(%q) = %v, want nil", v, err)
		}
	}
	err := validateValidity("fresh")
	if err == nil || !strings.Contains(err.Error(), "stale-unverified") {
		t.Errorf("validateValidity(fresh) = %v, want error naming valid values", err)
	}
}

func TestNormalizeEvidence(t *testing.T) {
	got, err := normalizeEvidence([]string{" dewey:tools/lint.go ", "dewey#147", "dewey:tools/lint.go"})
	if err != nil {
		t.Fatalf("normalizeEvidence error: %v", err)
	}
	if want := []string{"dewey:tools/lint.go", "dewey#147"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("normalizeEvidence = %v, want %v", got, want)
	}

	if got, err := normalizeEvidence(nil); err != nil || len(got) != 0 {
		t.Errorf("normalizeEvidence(nil) = %v, %v; want empty, nil", got, err)
	}

	tooMany := make([]string, maxEvidenceEntries+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	for name, input := range map[string][]string{
		"empty entry": {"ok", "  "},
		"too long":    {strings.Repeat("a", maxEvidenceLen+1)},
		"too many":    tooMany,
	} {
		if _, err := normalizeEvidence(input); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}

	// The bound counts characters, not bytes: a multi-byte locator within
	// 512 characters passes even though it exceeds 512 bytes, and one
	// character over the bound still fails.
	multiByte := strings.Repeat("世", maxEvidenceLen)
	if got, err := normalizeEvidence([]string{multiByte}); err != nil || len(got) != 1 {
		t.Errorf("normalizeEvidence(%d multi-byte chars) = %v, %v; want accepted", maxEvidenceLen, got, err)
	}
	if _, err := normalizeEvidence([]string{strings.Repeat("世", maxEvidenceLen+1)}); err == nil {
		t.Errorf("normalizeEvidence(%d multi-byte chars): expected error", maxEvidenceLen+1)
	}
}

func TestTemporalFields_Empty(t *testing.T) {
	var fields temporalFields
	if props := fields.properties(); len(props) != 0 {
		t.Errorf("properties() = %v, want empty", props)
	}
	if fm := fields.frontmatter(); fm != "" {
		t.Errorf("frontmatter() = %q, want empty", fm)
	}
}

func TestTemporalFields_FrontmatterRoundTripsThroughYAML(t *testing.T) {
	fields := temporalFields{
		AsOf:       "dewey@0123abc",
		Evidence:   []string{"dewey:tools/lint.go", `quote "and" # hash: colon`},
		Validity:   "current",
		Supersedes: "auth-20261101T000000-alice",
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(fields.frontmatter()), &parsed); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v\n%s", err, fields.frontmatter())
	}
	props := fields.properties()
	for _, key := range []string{"as_of", "validity", "supersedes"} {
		if parsed[key] != props[key] {
			t.Errorf("%s: YAML %v, properties %v", key, parsed[key], props[key])
		}
	}
	evidence, _ := parsed["evidence"].([]any)
	if len(evidence) != 2 || evidence[1] != fields.Evidence[1] {
		t.Errorf("evidence round trip = %v, want %v", evidence, fields.Evidence)
	}
}

func TestSupersedesIdentity(t *testing.T) {
	for in, want := range map[string]string{
		"auth-1":            "auth-1",
		"learning/auth-1":   "auth-1",
		"  learning/auth-1": "auth-1",
		"":                  "",
	} {
		if got := supersedesIdentity(in); got != want {
			t.Errorf("supersedesIdentity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidLearningIdentity(t *testing.T) {
	for in, want := range map[string]bool{
		"auth-1":                            true,
		"auth-20260502T143022-alice":        true,
		"auth-20260502T143022-John Flowers": true, // authors come from git config
		"":                                  false,
		".":                                 false,
		"..":                                false,
		"../../etc/passwd":                  false,
		"learning/../../x":                  false,
		"a..b":                              false,
		".hidden":                           false,
		`back\slash`:                        false,
	} {
		if got := ValidLearningIdentity(in); got != want {
			t.Errorf("ValidLearningIdentity(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetFrontmatterKeys(t *testing.T) {
	content := "---\ntag: auth\nvalidity: \"current\"\ntier: draft\n---\n\nBody with validity: inline text.\n"
	got, err := setFrontmatterKeys(content, [][2]string{{"superseded_by", "auth-2"}, {"validity", "superseded"}})
	if err != nil {
		t.Fatalf("setFrontmatterKeys error: %v", err)
	}
	want := "---\ntag: auth\nvalidity: \"superseded\"\ntier: draft\nsuperseded_by: \"auth-2\"\n---\n\nBody with validity: inline text.\n"
	if got != want {
		t.Errorf("setFrontmatterKeys =\n%s\nwant\n%s", got, want)
	}

	for name, bad := range map[string]string{
		"no frontmatter": "just a body\n",
		"unterminated":   "---\ntag: auth\n",
	} {
		if _, err := setFrontmatterKeys(bad, [][2]string{{"k", "v"}}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestResolveAsOf(t *testing.T) {
	if got := resolveAsOf(nil); got != "" {
		t.Errorf("resolveAsOf(nil) = %q, want empty", got)
	}
	if got := resolveAsOf(func() (string, error) { return "", errors.New("no git") }); got != "" {
		t.Errorf("resolveAsOf(failing) = %q, want empty", got)
	}
	if got := resolveAsOf(fixedAsOf("dewey@abc")); got != "dewey@abc" {
		t.Errorf("resolveAsOf(fixed) = %q, want dewey@abc", got)
	}
}

func TestProductionAsOfResolver(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	if _, err := productionAsOfResolver("")(); err == nil {
		t.Error("empty vault path: expected error")
	}
	if _, err := productionAsOfResolver(t.TempDir())(); err == nil {
		t.Error("non-repository directory: expected error")
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "myrepo")
	sub := filepath.Join(repo, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	sha, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}

	got, err := productionAsOfResolver(sub)()
	if err != nil {
		t.Fatalf("resolver error: %v", err)
	}
	if want := "myrepo@" + sha; got != want {
		t.Errorf("as_of = %q, want %q", got, want)
	}
}

func TestStoreLearning_TemporalFields(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "alice")
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0).WithAsOfResolver(fixedAsOf("dewey@abc123"))

	result, isErr := storeLearning(t, l, types.StoreLearningInput{
		Information: "Lint staleness should follow code drift.",
		Tag:         "lint",
		Evidence:    []string{"dewey:tools/lint.go", "dewey#147"},
		Validity:    "current",
	})
	if isErr {
		t.Fatalf("unexpected error result: %v", result["error"])
	}
	if result["as_of"] != "dewey@abc123" || result["validity"] != "current" {
		t.Errorf("result as_of/validity = %v/%v", result["as_of"], result["validity"])
	}
	if evidence, _ := result["evidence"].([]any); len(evidence) != 2 || evidence[0] != "dewey:tools/lint.go" || evidence[1] != "dewey#147" {
		t.Errorf("result evidence = %v, want [dewey:tools/lint.go dewey#147]", result["evidence"])
	}

	page, props := pageProps(t, s, resultString(t, result, "page"))
	if page.Tier != "draft" {
		t.Errorf("tier = %q, want draft", page.Tier)
	}
	if props["as_of"] != "dewey@abc123" || props["validity"] != "current" {
		t.Errorf("properties as_of/validity = %v/%v", props["as_of"], props["validity"])
	}
	if evidence, _ := props["evidence"].([]any); len(evidence) != 2 || evidence[0] != "dewey:tools/lint.go" || evidence[1] != "dewey#147" {
		t.Errorf("properties evidence = %v, want [dewey:tools/lint.go dewey#147]", props["evidence"])
	}

	fm, body := fileFrontmatter(t, filepath.Join(vault, resultString(t, result, "file_path")))
	if fm["as_of"] != "dewey@abc123" || fm["validity"] != "current" || fm["tier"] != "draft" {
		t.Errorf("frontmatter = %v", fm)
	}
	if evidence, _ := fm["evidence"].([]any); len(evidence) != 2 || evidence[0] != "dewey:tools/lint.go" || evidence[1] != "dewey#147" {
		t.Errorf("frontmatter evidence = %v, want [dewey:tools/lint.go dewey#147]", fm["evidence"])
	}
	if !strings.Contains(body, "Lint staleness") {
		t.Errorf("body = %q", body)
	}
}

func TestStoreLearning_NoTemporalFieldsWithoutInputOrGit(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "", 0)

	result, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "plain", Tag: "plain"})
	if isErr {
		t.Fatalf("unexpected error result: %v", result["error"])
	}
	_, props := pageProps(t, s, resultString(t, result, "page"))
	for _, key := range []string{"as_of", "evidence", "validity", "supersedes"} {
		if _, ok := props[key]; ok {
			t.Errorf("unexpected %s in properties: %v", key, props)
		}
	}
}

func TestStoreLearning_InvalidTemporalInputWritesNothing(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0).WithAsOfResolver(nil)

	// Seed a non-learning page under the learning/ namespace so the
	// SourceID rejection branch (page exists but is not a stored learning)
	// actually executes — against an empty store the same input would exit
	// through the not-found branch. ListLearningPages filters on
	// source_id='learning', so this disk-sourced seed does not affect the
	// zero-learning-pages assertion below.
	seed := &store.Page{
		Name:         "learning/README",
		OriginalName: "learning/README",
		SourceID:     "disk",
		SourceDocID:  "readme",
		Properties:   `{"tag":"readme"}`,
		Tier:         "authored",
	}
	if err := s.InsertPage(seed); err != nil {
		t.Fatalf("InsertPage seed: %v", err)
	}

	for name, input := range map[string]types.StoreLearningInput{
		"bad validity":     {Information: "x", Tag: "x", Validity: "fresh"},
		"empty evidence":   {Information: "x", Tag: "x", Evidence: []string{" "}},
		"unknown target":   {Information: "x", Tag: "x", Supersedes: "nope-20260101T000000-bob"},
		"non-learning tgt": {Information: "x", Tag: "x", Supersedes: "learning/README"},
		"traversal target": {Information: "x", Tag: "x", Supersedes: "../../etc/passwd"},
	} {
		if result, isErr := storeLearning(t, l, input); !isErr {
			t.Errorf("%s: expected error result, got %v", name, result)
		}
	}
	pages, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected no learning pages, got %d", len(pages))
	}
	if _, err := os.Stat(filepath.Join(vault, deweyWorkspaceDir, "learnings")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(vault, deweyWorkspaceDir, "learnings"))
		if len(entries) != 0 {
			t.Errorf("expected no learning files, got %d", len(entries))
		}
	}

	// The seeded non-learning page is untouched by the rejected calls.
	after, err := s.GetPage("learning/README")
	if err != nil || after == nil {
		t.Fatalf("seeded page missing after invalid calls: %v, %v", after, err)
	}
	if after.SourceID != "disk" || after.Properties != `{"tag":"readme"}` {
		t.Errorf("seeded page changed: source %q, properties %s", after.SourceID, after.Properties)
	}
}

func TestStoreLearning_SupersedesLinksBothWays(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "alice")
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0).WithAsOfResolver(nil)

	old, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "Gate uses three layers.", Tag: "gate"})
	if isErr {
		t.Fatalf("store old: %v", old["error"])
	}
	oldIdentity := resultString(t, old, "identity")

	// Page-name form is accepted as well as the bare identity.
	newer, isErr := storeLearning(t, l, types.StoreLearningInput{
		Information: "Gate uses two layers.",
		Tag:         "gate-v2",
		Supersedes:  "learning/" + oldIdentity,
	})
	if isErr {
		t.Fatalf("store newer: %v", newer["error"])
	}
	if newer["supersedes"] != oldIdentity {
		t.Errorf("result supersedes = %v, want %q", newer["supersedes"], oldIdentity)
	}
	if _, ok := newer["supersedes_warning"]; ok {
		t.Errorf("unexpected warning: %v", newer["supersedes_warning"])
	}
	newIdentity := resultString(t, newer, "identity")

	oldPage, oldProps := pageProps(t, s, "learning/"+oldIdentity)
	if oldProps["superseded_by"] != newIdentity || oldProps["validity"] != "superseded" {
		t.Errorf("old properties = %v", oldProps)
	}
	if oldPage.Tier != "draft" || oldProps["tag"] != "gate" {
		t.Errorf("old page tier/tag changed: %q / %v", oldPage.Tier, oldProps["tag"])
	}

	fm, body := fileFrontmatter(t, filepath.Join(vault, resultString(t, old, "file_path")))
	if fm["superseded_by"] != newIdentity || fm["validity"] != "superseded" || fm["identity"] != oldIdentity {
		t.Errorf("old frontmatter = %v", fm)
	}
	if !strings.Contains(body, "Gate uses three layers.") {
		t.Errorf("old body changed: %q", body)
	}

	// One successor per learning.
	third, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "fork", Tag: "gate-v3", Supersedes: oldIdentity})
	if !isErr || !strings.Contains(resultString(t, third, "error"), "already superseded") {
		t.Errorf("second supersession of the same target = %v, want already-superseded error", third)
	}
}

// TestStoreLearning_SupersedesWarningOnPartialFailure exercises the
// supersedes_warning response field through StoreLearning: when the store
// leg of the back-link succeeds but the markdown backup cannot be
// rewritten, the learning is still stored and the result carries a
// non-empty warning naming the target and its file. Every other branch of
// the warning wiring (success, nil target) is covered elsewhere; without
// this test, renaming or dropping the result key keeps the suite green.
func TestStoreLearning_SupersedesWarningOnPartialFailure(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "alice")
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0).WithAsOfResolver(nil)

	old, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "Original claim.", Tag: "claim"})
	if isErr {
		t.Fatalf("store old: %v", old["error"])
	}
	oldIdentity := resultString(t, old, "identity")

	// Corrupt the markdown backup so the file leg of the back-link fails
	// while the store leg succeeds: the target page is updated in the
	// store, but rewriting the backup cannot parse its frontmatter.
	backup := filepath.Join(vault, deweyWorkspaceDir, "learnings", oldIdentity+".md")
	if err := os.WriteFile(backup, []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	newer, isErr := storeLearning(t, l, types.StoreLearningInput{
		Information: "Revised claim.",
		Tag:         "claim-v2",
		Supersedes:  oldIdentity,
	})
	if isErr {
		t.Fatalf("store newer: %v", newer["error"])
	}
	warning := resultString(t, newer, "supersedes_warning")
	if !strings.Contains(warning, oldIdentity) || !strings.Contains(warning, "frontmatter") {
		t.Errorf("supersedes_warning = %q, want %q and a file-failure marker", warning, oldIdentity)
	}
	newIdentity := resultString(t, newer, "identity")

	// The store leg succeeded despite the file-leg warning.
	_, oldProps := pageProps(t, s, "learning/"+oldIdentity)
	if oldProps["superseded_by"] != newIdentity || oldProps["validity"] != "superseded" {
		t.Errorf("store back-link missing after partial failure: %v", oldProps["superseded_by"])
	}
}

// TestStoreLearning_SupersedesTargetWithCorruptProperties verifies that a
// supersession target whose stored properties are unreadable fails closed:
// the call returns an error result and writes nothing, instead of silently
// proceeding with an empty properties map (which would skip the
// already-superseded guard and fork the chain).
func TestStoreLearning_SupersedesTargetWithCorruptProperties(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, t.TempDir(), 0).WithAsOfResolver(nil)

	corrupt := &store.Page{
		Name:         "learning/corrupt-1",
		OriginalName: "learning/corrupt-1",
		SourceID:     store.SourceLearning,
		SourceDocID:  "learning-corrupt-1",
		Properties:   "{not json",
		Tier:         "draft",
	}
	if err := s.InsertPage(corrupt); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "x", Tag: "x", Supersedes: "corrupt-1"})
	if !isErr || !strings.Contains(resultString(t, result, "error"), "unreadable") {
		t.Errorf("corrupt target = %v, want unreadable-properties error", result)
	}
	if pages, err := s.ListLearningPages(); err != nil || len(pages) != 1 {
		t.Errorf("learning pages = %d, %v; want only the seeded corrupt page", len(pages), err)
	}
}

func TestLinkSuperseded_Warnings(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	if got := l.linkSuperseded(nil, "x"); got != "" {
		t.Errorf("nil target: %q, want empty", got)
	}

	// A page that is not in the store cannot be updated.
	ghost := &store.Page{Name: "learning/ghost-1", SourceID: store.SourceLearning, Properties: "{}"}
	if got := l.linkSuperseded(ghost, "x"); !strings.Contains(got, "in the store") {
		t.Errorf("missing page: %q, want store warning", got)
	}

	// A backup file without frontmatter cannot be rewritten.
	page := &store.Page{Name: "learning/bare-1", OriginalName: "learning/bare-1", SourceID: store.SourceLearning, Properties: "{}", Tier: "draft"}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	dir := filepath.Join(vault, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bare-1.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := l.linkSuperseded(page, "x"); !strings.Contains(got, "not in") {
		t.Errorf("bad backup: %q, want file warning", got)
	}

	// Unreadable stored properties fail the link with a warning instead of
	// silently replacing the target's properties wholesale (CS-006).
	corrupt := &store.Page{Name: "learning/corrupt-1", OriginalName: "learning/corrupt-1", SourceID: store.SourceLearning, SourceDocID: "learning-corrupt-1", Properties: "{not json", Tier: "draft"}
	if err := s.InsertPage(corrupt); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if got := l.linkSuperseded(corrupt, "x"); !strings.Contains(got, "unreadable") {
		t.Errorf("corrupt properties: %q, want unreadable-properties warning", got)
	}

	// A literal JSON null decodes to a nil map; the link must still succeed
	// rather than panic on the superseded_by assignment.
	nullProps := &store.Page{Name: "learning/null-1", OriginalName: "learning/null-1", SourceID: store.SourceLearning, SourceDocID: "learning-null-1", Properties: "null", Tier: "draft"}
	if err := s.InsertPage(nullProps); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if got := l.linkSuperseded(nullProps, "x"); got != "" {
		t.Errorf("null properties: %q, want empty (no warning)", got)
	}
	if _, props := pageProps(t, s, "learning/null-1"); props["superseded_by"] != "x" || props["validity"] != "superseded" {
		t.Errorf("null-props back-link = %v", props)
	}

	// An unsafe identity never reaches a file write (SC-004).
	if got := l.markSupersededFile("../../escape", "x"); !strings.Contains(got, "unsafe") {
		t.Errorf("unsafe identity: %q, want unsafe-identity warning", got)
	}

	// No vault, or no backup file: nothing to rewrite, no warning.
	if got := NewLearning(nil, s, "", 0).markSupersededFile("bare-1", "x"); got != "" {
		t.Errorf("no vault: %q, want empty", got)
	}
	if got := l.markSupersededFile("absent-1", "x"); got != "" {
		t.Errorf("no file: %q, want empty", got)
	}
}
