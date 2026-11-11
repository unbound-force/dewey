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
	"sync"
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

// writeLearningBackup writes a learning's markdown backup under vault
// and returns its path, so a test can seed the durable record and
// later assert it stayed untouched byte-for-byte.
func writeLearningBackup(t *testing.T, vault, identity, content string) string {
	t.Helper()
	dir := filepath.Join(vault, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, identity+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
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

func TestFrontmatterScalar_ScalarForms(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		key       string
		wantValue string
		wantFound bool
	}{
		{
			name:      "bare value",
			content:   "---\ntag: auth\nsuperseded_by: auth-1\n---\n\nBody.\n",
			key:       "superseded_by",
			wantValue: "auth-1",
			wantFound: true,
		},
		{
			name:      "double-quoted value",
			content:   "---\nsuperseded_by: \"auth-1\"\n---\n",
			key:       "superseded_by",
			wantValue: "auth-1",
			wantFound: true,
		},
		{
			name:      "double-quoted with escape",
			content:   "---\nsuperseded_by: \"a\\tb\"\n---\n",
			key:       "superseded_by",
			wantValue: "a\tb",
			wantFound: true,
		},
		{
			name:      "single-quoted value",
			content:   "---\nsuperseded_by: 'auth-1'\n---\n",
			key:       "superseded_by",
			wantValue: "auth-1",
			wantFound: true,
		},
		{
			name:      "single-quoted with doubled quote",
			content:   "---\nsuperseded_by: 'it''s'\n---\n",
			key:       "superseded_by",
			wantValue: "it's",
			wantFound: true,
		},
		{
			name:      "empty double-quoted value",
			content:   "---\nsuperseded_by: \"\"\n---\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: true,
		},
		{
			name:      "empty value",
			content:   "---\ntag: auth\nsuperseded_by:\ntier: draft\n---\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: true,
		},
		{
			name:      "key absent",
			content:   "---\ntag: auth\n---\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: false,
		},
		{
			name:      "nested key is not top-level",
			content:   "---\nevidence:\n  superseded_by: nested\nsuperseded_by: \"auth-1\"\n---\n",
			key:       "superseded_by",
			wantValue: "auth-1",
			wantFound: true,
		},
		{
			name:      "unparseable double-quoted returned verbatim",
			content:   "---\nsuperseded_by: \"unterminated\n---\n",
			key:       "superseded_by",
			wantValue: "\"unterminated",
			wantFound: true,
		},
		{
			name:      "unterminated single-quoted returned as junk",
			content:   "---\nsuperseded_by: 'oops\n---\n",
			key:       "superseded_by",
			wantValue: "'oops",
			wantFound: true,
		},
		{
			name:      "no frontmatter",
			content:   "just a body\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: false,
		},
		{
			name:      "unterminated frontmatter",
			content:   "---\ntag: auth\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: false,
		},
		{
			name:      "body occurrence is not read",
			content:   "---\ntag: auth\n---\nsuperseded_by: auth-1\n",
			key:       "superseded_by",
			wantValue: "",
			wantFound: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, found := frontmatterScalar(tc.content, tc.key)
			if value != tc.wantValue || found != tc.wantFound {
				t.Errorf("frontmatterScalar(content, %q) = (%q, %v), want (%q, %v)",
					tc.key, value, found, tc.wantValue, tc.wantFound)
			}
		})
	}
}

func TestFrontmatterScalar_BlockForm(t *testing.T) {
	value, found := frontmatterScalar("---\ntag: auth\nsuperseded_by:\n  - x\n---\n\nBody.\n", "superseded_by")
	if !found {
		t.Fatal("block form: found = false, want true")
	}
	if value == "" {
		t.Fatal("block form: value is empty, want non-empty junk")
	}
	if !strings.Contains(value, "- x") {
		t.Errorf("block form: value %q does not carry the block's children", value)
	}
	if value == "x" {
		t.Errorf("block form: value %q reads as a plain identity; want junk no identity equals", value)
	}

	// Whitespace after the colon must not demote the block to an unclaimed
	// scalar: setFrontmatterKeys would replace only the key line and leave
	// the children attached to it.
	value, found = frontmatterScalar("---\nsuperseded_by: \n  - x\n---\n", "superseded_by")
	if !found || value == "" {
		t.Errorf("block form with trailing space: got (%q, %v), want non-empty junk and found", value, found)
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

	// One successor per learning (LTV-FR-001, scenario "Unknown or
	// already-superseded target" — the serialize-supersession-writes
	// regression guard): a second sequential supersession of the same
	// target fails validation with the existing "already superseded by"
	// error before any write. Snapshot the durable record first so the
	// rejection can be held against byte-for-byte and count invariants.
	backupPath := filepath.Join(vault, resultString(t, old, "file_path"))
	backupBefore, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", backupPath, err)
	}
	pagesBefore, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pagesBefore) != 2 {
		t.Fatalf("learning pages before rejected supersession = %d, want 2 (target + first successor)", len(pagesBefore))
	}
	filesBefore, err := os.ReadDir(filepath.Join(vault, deweyWorkspaceDir, "learnings"))
	if err != nil {
		t.Fatalf("ReadDir learnings: %v", err)
	}
	if len(filesBefore) != 2 {
		t.Fatalf("learning files before rejected supersession = %d, want 2 (one backup per stored page)", len(filesBefore))
	}

	third, isErr := storeLearning(t, l, types.StoreLearningInput{Information: "fork", Tag: "gate-v3", Supersedes: oldIdentity})
	if !isErr {
		t.Fatalf("second supersession of the same target = %v, want an error result", third)
	}
	thirdErr := resultString(t, third, "error")
	if !strings.Contains(thirdErr, "already superseded by") ||
		!strings.Contains(thirdErr, oldIdentity) ||
		!strings.Contains(thirdErr, newIdentity) {
		t.Errorf("second supersession error = %q, want the already-superseded-by error naming target %q and first successor %q",
			thirdErr, oldIdentity, newIdentity)
	}

	// The rejection wrote nothing: no second successor page, the
	// target's store row and markdown backup still name the FIRST
	// successor, and the learnings directory gained no file.
	pagesAfter, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pagesAfter) != len(pagesBefore) {
		t.Errorf("learning pages after rejected supersession = %d, want %d (no second successor page)", len(pagesAfter), len(pagesBefore))
	}
	_, oldPropsAfter := pageProps(t, s, "learning/"+oldIdentity)
	if oldPropsAfter["superseded_by"] != newIdentity || oldPropsAfter["validity"] != "superseded" {
		t.Errorf("target properties after rejected supersession = %v, want the first successor %q with validity superseded", oldPropsAfter, newIdentity)
	}
	backupAfter, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", backupPath, err)
	}
	if string(backupAfter) != string(backupBefore) {
		t.Errorf("target backup after rejected supersession =\n%s\nwant byte-identical to\n%s", backupAfter, backupBefore)
	}
	filesAfter, err := os.ReadDir(filepath.Join(vault, deweyWorkspaceDir, "learnings"))
	if err != nil {
		t.Fatalf("ReadDir learnings: %v", err)
	}
	if len(filesAfter) != len(filesBefore) {
		t.Errorf("learning files after rejected supersession = %d, want %d (no file gained)", len(filesAfter), len(filesBefore))
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

// TestLinkSuperseded_LostClaimFastFailNamesWinner exercises the
// in-closure lost-claim fast-fail (SSW-FR-002, design D2): the target
// passed validation unclaimed, but by the time the store leg runs the
// row already records a successor. The re-read inside
// withLearningPropsLock must route that through the shared lost-claim
// outcome — a warning naming the recorded winner, never the generic
// store-leg failure — and neither leg may write: the store keeps the
// recorded successor and the backup stays byte-for-byte unchanged.
func TestLinkSuperseded_LostClaimFastFailNamesWinner(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	// The validation-time snapshot, fetched the way
	// resolveSupersedesTarget fetches it while the target is still
	// unclaimed. The snapshot supplies the target's identity only —
	// the store leg re-reads the row — so seeding a winner after this
	// fetch is the deterministic stand-in for "superseded after
	// validation".
	insertPinnedLearning(t, s, "race-fast-1", map[string]any{"tag": "race", "validity": "current"})
	target, err := s.GetPage(store.LearningPrefix + "race-fast-1")
	if err != nil || target == nil {
		t.Fatalf("GetPage: %v, %v", target, err)
	}

	// Another writer's supersession landed on the target between
	// validation and this call — store row and markdown backup alike.
	const winnerProps = `{"tag":"race","validity":"superseded","superseded_by":"winner-A"}`
	target.Properties = winnerProps
	if err := s.UpdatePage(target); err != nil {
		t.Fatalf("UpdatePage seed: %v", err)
	}
	seededBackup := "---\ntag: race\nidentity: race-fast-1\ntier: draft\nsuperseded_by: \"winner-A\"\nvalidity: \"superseded\"\n---\n\nOriginal claim body.\n"
	backup := writeLearningBackup(t, vault, "race-fast-1", seededBackup)

	warning := l.linkSuperseded(target, "successor-B")

	// The fast-fail names the recorded winner with the lost-claim
	// wording, not the generic store-leg failure.
	want := `learning "race-fast-1" was concurrently superseded by "winner-A"; this learning's supersedes claim is dangling`
	if warning != want {
		t.Errorf("warning = %q, want the lost-claim warning %q", warning, want)
	}
	if strings.Contains(warning, "failed to mark") {
		t.Errorf("fast-fail surfaced the generic store-leg failure: %q", warning)
	}

	// The store keeps the recorded winner; the losing successor never
	// landed.
	_, props := pageProps(t, s, store.LearningPrefix+"race-fast-1")
	if props["superseded_by"] != "winner-A" || props["validity"] != "superseded" {
		t.Errorf("store back-link = %v (validity %v), want the recorded winner-A untouched", props["superseded_by"], props["validity"])
	}

	// The file leg does not run on a lost claim.
	after, err := os.ReadFile(backup)
	if err != nil || string(after) != seededBackup {
		t.Errorf("backup after lost claim = %q, %v; want byte-identical to the seeded content", after, err)
	}
}

// TestLinkSuperseded_LostClaimConcurrentClaimNamesWinner exercises the
// zero-row-CAS loss scenario (SSW-FR-001, "Target superseded between
// validation and write"): the snapshot shows an unclaimed target, but
// a writer that bypasses the tools mutex records a successor before
// the store leg runs — seeded here through the guarded store claim
// itself, the write the SQL guard is authoritative over. Both loss
// routes converge on claimSuperseded's (false, winner, nil) result
// (design D2), so the call must report the recorded winner and keep
// it: the pre-seeded successor stays in the store — not this call's —
// and the backup is not overwritten.
func TestLinkSuperseded_LostClaimConcurrentClaimNamesWinner(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	insertPinnedLearning(t, s, "race-cas-1", map[string]any{"tag": "race", "validity": "current"})
	target, err := s.GetPage(store.LearningPrefix + "race-cas-1")
	if err != nil || target == nil {
		t.Fatalf("GetPage: %v, %v", target, err)
	}

	// The concurrent writer records its successor through the guarded
	// store claim, leaving exactly the row shape any production writer
	// leaves; its file leg landed on the backup too.
	const winnerProps = `{"tag":"race","validity":"superseded","superseded_by":"winner-B"}`
	claimed, _, err := s.ClaimSupersession(store.LearningPrefix+"race-cas-1", winnerProps)
	if err != nil || !claimed {
		t.Fatalf("seed claim: claimed=%v, err=%v; want the concurrent writer's claim to land", claimed, err)
	}
	seededBackup := "---\ntag: race\nidentity: race-cas-1\ntier: draft\nsuperseded_by: \"winner-B\"\nvalidity: \"superseded\"\n---\n\nOriginal claim body.\n"
	backup := writeLearningBackup(t, vault, "race-cas-1", seededBackup)

	warning := l.linkSuperseded(target, "successor-C")

	if !strings.Contains(warning, "winner-B") || !strings.Contains(warning, "concurrently superseded") {
		t.Errorf("warning = %q, want a lost-claim warning naming winner-B", warning)
	}
	if strings.Contains(warning, "failed to mark") || strings.Contains(warning, "successor-C") {
		t.Errorf("warning = %q, want neither the generic store-leg failure nor the losing successor named", warning)
	}

	// The store keeps the pre-seeded winner verbatim — a refused claim
	// writes nothing.
	page, props := pageProps(t, s, store.LearningPrefix+"race-cas-1")
	if props["superseded_by"] != "winner-B" {
		t.Errorf("store back-link = %v, want the pre-seeded winner-B", props["superseded_by"])
	}
	if page.Properties != winnerProps {
		t.Errorf("store properties = %q, want the pre-seeded %q untouched", page.Properties, winnerProps)
	}

	after, err := os.ReadFile(backup)
	if err != nil || string(after) != seededBackup {
		t.Errorf("backup after lost claim = %q, %v; want byte-identical to the seeded content", after, err)
	}
}

// hookEmbedder wraps mockEmbedder with a callback fired from inside
// EmbedBatch. StoreLearning generates embeddings after validation has
// resolved the supersession target and before the supersession store
// leg runs, so an onBatch callback lands a write in exactly that
// window — deterministically, with no goroutines or sleeps.
type hookEmbedder struct {
	*mockEmbedder
	onBatch func()
}

// EmbedBatch fires the hook, then embeds as usual.
func (h *hookEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	h.onBatch()
	return h.mockEmbedder.EmbedBatch(ctx, texts)
}

// TestStoreLearning_LostClaimWarningNamesWinner drives the lost-claim
// path end-to-end through StoreLearning (SSW-FR-002, "Concurrent
// losing claim"): the call passes validation against an unclaimed
// target, a concurrent writer claims the target during the call's
// embedding phase — after validation, before the store leg — and the
// result carries supersedes_warning naming the winner while the losing
// learning is kept with its supersedes claim intact. Without this
// test, the lost-claim cause of the warning's wiring into the result
// field is covered only by the file-leg partial failure.
func TestStoreLearning_LostClaimWarningNamesWinner(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "alice")
	s := newTestStore(t)
	vault := t.TempDir()

	insertPinnedLearning(t, s, "race-e2e-1", map[string]any{"tag": "race", "validity": "current"})
	// The concurrent writer's supersession completed on the durable
	// record before the call under test runs.
	seededBackup := "---\ntag: race\nidentity: race-e2e-1\ntier: draft\nsuperseded_by: \"winner-A\"\nvalidity: \"superseded\"\n---\n\nOriginal claim body.\n"
	backup := writeLearningBackup(t, vault, "race-e2e-1", seededBackup)

	// The hook claims the target through the guarded store call —
	// bypassing the tools mutex, the writer class the SQL guard is
	// authoritative over — while StoreLearning sits between validation
	// and its store leg.
	const winnerProps = `{"tag":"race","validity":"superseded","superseded_by":"winner-A"}`
	var hookClaimed bool
	var hookErr error
	l := NewLearning(&hookEmbedder{
		mockEmbedder: newMockEmbedder(true),
		onBatch: func() {
			hookClaimed, _, hookErr = s.ClaimSupersession(store.LearningPrefix+"race-e2e-1", winnerProps)
		},
	}, s, vault, 0).WithAsOfResolver(nil)

	result, isErr := storeLearning(t, l, types.StoreLearningInput{
		Information: "Revised claim that loses the race.",
		Tag:         "race-v2",
		Supersedes:  "race-e2e-1",
	})
	if isErr {
		t.Fatalf("unexpected error result: %v", result["error"])
	}
	if hookErr != nil || !hookClaimed {
		t.Fatalf("concurrent writer's claim: claimed=%v, err=%v; want it to land", hookClaimed, hookErr)
	}

	warning := resultString(t, result, "supersedes_warning")
	if !strings.Contains(warning, "winner-A") || !strings.Contains(warning, "concurrently superseded") {
		t.Errorf("supersedes_warning = %q, want a lost-claim warning naming winner-A", warning)
	}
	if strings.Contains(warning, "failed to mark") {
		t.Errorf("supersedes_warning = %q, want the lost-claim cause, not the generic store-leg failure", warning)
	}

	// The losing learning is kept with its supersedes claim intact.
	_, props := pageProps(t, s, resultString(t, result, "page"))
	if props["supersedes"] != "race-e2e-1" {
		t.Errorf("losing learning's supersedes = %v, want race-e2e-1", props["supersedes"])
	}

	// The target keeps the concurrent winner in the store and in the
	// backup; the losing successor never landed.
	_, targetProps := pageProps(t, s, store.LearningPrefix+"race-e2e-1")
	if targetProps["superseded_by"] != "winner-A" {
		t.Errorf("target superseded_by = %v, want the concurrent winner-A", targetProps["superseded_by"])
	}
	after, err := os.ReadFile(backup)
	if err != nil || string(after) != seededBackup {
		t.Errorf("backup = %q, %v; want byte-identical to the seeded content", after, err)
	}
}

// concurrentResult captures one goroutine's raw StoreLearning outcome so
// the main test goroutine can classify it after every call completes —
// goroutines never touch t, because t.Fatalf from a non-test goroutine
// does not stop the test.
type concurrentResult struct {
	text  string // raw result text; set for success and error results alike
	isErr bool   // result.IsError
	goErr error  // Go-level error returned by the call itself
}

// TestStoreLearning_ConcurrentSupersession is the SSW-FR-001 race
// regression for the scenario "Two concurrent supersessions of the same
// target": eight goroutines released together by closing a shared start
// channel — no sleeps — all call StoreLearning superseding the same
// target. newTestStore's :memory: SQLite runs on a single pooled
// connection (store.New sets MaxOpenConns(1)), so seeding and all eight
// goroutines share one store handle with no per-connection isolation.
//
// Determinism reasoning (iteration-3 review advisory): only the
// store-level invariant — exactly one claim lands — is deterministic,
// because Store.ClaimSupersession is a compare-and-swap guarded in the
// UPDATE's WHERE clause. Per-call classification is NOT deterministic:
// StoreLearning validates the target in prepareTemporal /
// resolveSupersedesTarget before any write, and that validation is
// itself check-then-act, so a goroutine whose validation runs after
// another goroutine's claim landed takes the existing "already
// superseded by" error path and produces neither a successor page nor a
// warning. The test therefore asserts the invariant and accepts both
// legitimate loser outcomes:
//
//   - (a) the claim winner: success with no supersedes_warning —
//     exactly one, by the CAS;
//   - (b) a write-leg loser: success with a supersedes_warning naming
//     the recorded winner (both loss routes — the zero-row CAS refusal
//     and the in-closure fast-fail — converge on the same warning);
//   - (c) a validation loser: an "already superseded by" error, writing
//     nothing.
//
// It deliberately does NOT assert that all N successors exist or that
// the other N-1 results carry a warning — those claims are unsound
// because validation races the claim. What MUST hold under every
// interleaving: exactly one class-(a) result whose identity is the
// recorded winner; store and backup both record that winner and nothing
// else; every successful call (class a or b) stored its successor page
// with its supersedes claim intact; validation losers (class c) wrote
// nothing; at least one successor page exists.
func TestStoreLearning_ConcurrentSupersession(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "racer")
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0).WithAsOfResolver(nil)

	// Seed the target through the real tool so the race exercises the
	// full flow: a stored learning page plus a markdown backup whose
	// file leg the claim winner rewrites.
	target, isErr := storeLearning(t, l, types.StoreLearningInput{
		Information: "Original concurrent-supersession claim.",
		Tag:         "race-target",
	})
	if isErr {
		t.Fatalf("store target: %v", target["error"])
	}
	targetIdentity := resultString(t, target, "identity")

	const goroutines = 8
	results := make([]concurrentResult, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// Distinct tags keep the eight identities distinct, so the
			// recorded winner can only match one successor identity.
			result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
				Information: fmt.Sprintf("Concurrent claim %d.", i),
				Tag:         fmt.Sprintf("race-v%d", i),
				Supersedes:  targetIdentity,
			})
			results[i].goErr = err
			if result != nil {
				results[i].text = resultText(result)
				results[i].isErr = result.IsError
			}
		}(i)
	}
	close(start) // release every goroutine at once
	wg.Wait()

	// Store-level invariant (SSW-FR-001): exactly one superseded_by is
	// recorded, and the target's validity flipped to superseded.
	_, targetProps := pageProps(t, s, store.LearningPrefix+targetIdentity)
	recordedWinner, ok := targetProps["superseded_by"].(string)
	if !ok || recordedWinner == "" {
		t.Fatalf("target superseded_by = %v, want exactly one recorded winner", targetProps["superseded_by"])
	}
	if targetProps["validity"] != "superseded" {
		t.Errorf("target validity = %v, want superseded", targetProps["validity"])
	}

	// The durable record agrees with the store: the backup's file leg ran
	// exactly once, for the claim winner.
	fm, _ := fileFrontmatter(t, filepath.Join(vault, resultString(t, target, "file_path")))
	if fm["superseded_by"] != recordedWinner {
		t.Errorf("backup superseded_by = %v, want the recorded winner %q", fm["superseded_by"], recordedWinner)
	}
	if fm["validity"] != "superseded" {
		t.Errorf("backup validity = %v, want superseded", fm["validity"])
	}

	// Classify every call. A write-leg loser's warning is always the
	// lost-claim warning — the file leg never runs on a lost claim — and
	// both loss routes render it identically, so exact equality both
	// proves it names the recorded winner and rules out any other name.
	wantWarning := fmt.Sprintf("learning %q was concurrently superseded by %q; this learning's supersedes claim is dangling",
		targetIdentity, recordedWinner)

	var winnerIdentity string
	successIdentities := make([]string, 0, goroutines)
	for i, res := range results {
		if res.goErr != nil {
			t.Fatalf("goroutine %d: StoreLearning Go error: %v", i, res.goErr)
		}
		if res.isErr {
			// Class (c): validation loser — it wrote nothing.
			if !strings.Contains(res.text, "already superseded by") || !strings.Contains(res.text, targetIdentity) {
				t.Errorf("goroutine %d: error result = %q, want the already-superseded validation error naming %q", i, res.text, targetIdentity)
			}
			continue
		}
		parsed := parseLearningResult(t, res.text)
		identity := resultString(t, parsed, "identity")
		if warning, present := parsed["supersedes_warning"]; present {
			// Class (b): write-leg loser.
			text, isString := warning.(string)
			if !isString {
				t.Fatalf("goroutine %d: supersedes_warning = %v, want a string", i, warning)
			}
			if text != wantWarning {
				t.Errorf("goroutine %d: supersedes_warning = %q, want the lost-claim warning %q", i, text, wantWarning)
			}
		} else {
			// Class (a): the claim winner.
			if winnerIdentity != "" {
				t.Errorf("two warning-free successes (%q, %q); the CAS must land exactly one claim", winnerIdentity, identity)
			}
			winnerIdentity = identity
		}
		successIdentities = append(successIdentities, identity)
	}

	if winnerIdentity == "" {
		t.Fatal("no warning-free success; exactly one goroutine must win the claim")
	}
	if winnerIdentity != recordedWinner {
		t.Errorf("claim winner = %q, recorded successor = %q; the store must record the winner", winnerIdentity, recordedWinner)
	}

	// The recorded winner is the identity of exactly one of the N
	// successors (distinct tags make every success identity unique, so
	// this counts identity collisions as well as forked records).
	winnerMatches := 0
	for _, identity := range successIdentities {
		if identity == recordedWinner {
			winnerMatches++
		}
	}
	if winnerMatches != 1 {
		t.Errorf("recorded winner %q matches %d successor identities, want exactly 1", recordedWinner, winnerMatches)
	}

	// At least one successor page exists — the winner's own learning.
	if len(successIdentities) == 0 {
		t.Fatal("no successor pages stored; the winner's learning must be kept")
	}

	// Every successful call — winner and write-leg losers alike — stored
	// its successor page with its own supersedes claim intact
	// (SSW-FR-002 keeps the losing learning).
	for _, identity := range successIdentities {
		_, props := pageProps(t, s, store.LearningPrefix+identity)
		if props["supersedes"] != targetIdentity {
			t.Errorf("successor %s: stored supersedes = %v, want %q", identity, props["supersedes"], targetIdentity)
		}
	}

	// Validation losers wrote nothing: the store holds exactly the target
	// plus one page per successful call, and the learnings directory one
	// markdown backup per stored page.
	pages, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pages) != 1+len(successIdentities) {
		t.Errorf("learning pages = %d, want 1 target + %d successors", len(pages), len(successIdentities))
	}
	entries, err := os.ReadDir(filepath.Join(vault, deweyWorkspaceDir, "learnings"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1+len(successIdentities) {
		t.Errorf("learning files = %d, want 1 target backup + %d successors", len(entries), len(successIdentities))
	}
}

// TestLinkSuperseded_UndecodablePropertiesFailStoreLeg verifies the
// fail-closed decode seam on the store leg's re-read: properties that
// SQLite's JSON parser accepts but Go's encoding/json rejects (the \x
// hex-escape JSON5 extension — the same document the store claim's
// disambiguating re-read fails on) surface as the store-leg failure
// warning, never as a lost claim naming an unreadable winner, and
// nothing is written.
func TestLinkSuperseded_UndecodablePropertiesFailStoreLeg(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	// The row records a successor SQLite can read — json_extract sees
	// superseded_by = "winner-A", so the SQL guard would refuse a
	// claim — but Go's decode of the document fails, so the re-read
	// inside the store leg fails closed before any write.
	const poisoned = `{"tag":"race","superseded_by":"winner-A","note":"a\x01b"}`
	target := &store.Page{
		Name:         store.LearningPrefix + "race-poison-1",
		OriginalName: store.LearningPrefix + "race-poison-1",
		SourceID:     store.SourceLearning,
		SourceDocID:  "learning-race-poison-1",
		Properties:   poisoned,
		Tier:         "draft",
	}
	if err := s.InsertPage(target); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	seededBackup := "---\ntag: race\nidentity: race-poison-1\ntier: draft\n---\n\nOriginal claim body.\n"
	backup := writeLearningBackup(t, vault, "race-poison-1", seededBackup)

	warning := l.linkSuperseded(target, "successor-E")

	if !strings.Contains(warning, "failed to mark") || !strings.Contains(warning, "unreadable stored properties") {
		t.Errorf("warning = %q, want the store-leg failure naming the unreadable properties", warning)
	}
	if strings.Contains(warning, "concurrently superseded") {
		t.Errorf("warning = %q, an undecodable row must not surface as a lost claim", warning)
	}

	// The failed leg wrote nothing: the row and the backup are
	// untouched.
	page, err := s.GetPage(store.LearningPrefix + "race-poison-1")
	if err != nil || page == nil {
		t.Fatalf("GetPage: %v, %v", page, err)
	}
	if page.Properties != poisoned {
		t.Errorf("store properties = %q, want the seeded %q untouched", page.Properties, poisoned)
	}
	after, err := os.ReadFile(backup)
	if err != nil || string(after) != seededBackup {
		t.Errorf("backup = %q, %v; want byte-identical to the seeded content", after, err)
	}
}

// TestLinkSuperseded_FileGuardRefusesDifferingRecordedValue exercises the
// supersession file leg's guard (serialize-supersession-writes design D3)
// through linkSuperseded: the store target is unclaimed, so the store leg
// claims it, but the markdown backup already records a different successor.
// The file leg must refuse — leaving the durable record byte-for-byte
// unchanged and reporting the refusal as the warning — while the store
// leg's claim stands, so the warning is the file-guard refusal naming the
// recorded value, never the generic store-leg failure.
func TestLinkSuperseded_FileGuardRefusesDifferingRecordedValue(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	insertPinnedLearning(t, s, "guard-differ-1", map[string]any{"tag": "guard", "validity": "current"})
	target, err := s.GetPage(store.LearningPrefix + "guard-differ-1")
	if err != nil || target == nil {
		t.Fatalf("GetPage: %v, %v", target, err)
	}
	seededBackup := "---\ntag: guard\nidentity: guard-differ-1\ntier: draft\nsuperseded_by: \"old-1\"\n---\n\nOriginal claim body.\n"
	backup := writeLearningBackup(t, vault, "guard-differ-1", seededBackup)

	warning := l.linkSuperseded(target, "new-1")

	// The refusal names the recorded successor with the file-leg wording,
	// not the generic store-leg failure.
	want := `learning "guard-differ-1" was not marked superseded in its markdown backup: backup already records superseded_by "old-1"`
	if warning != want {
		t.Errorf("warning = %q, want the file-guard refusal %q", warning, want)
	}

	// The store leg is not the failure: the claim landed on the unclaimed
	// target, so the store carries this call's successor.
	_, props := pageProps(t, s, store.LearningPrefix+"guard-differ-1")
	if props["superseded_by"] != "new-1" || props["validity"] != "superseded" {
		t.Errorf("store leg = superseded_by %v, validity %v; want new-1 and superseded", props["superseded_by"], props["validity"])
	}

	// The durable record keeps the differing successor byte-for-byte.
	after, err := os.ReadFile(backup)
	if err != nil || string(after) != seededBackup {
		t.Errorf("backup = %q, %v; want byte-identical to the seeded content", after, err)
	}
}

// TestMarkSupersededFile_GuardRefusesUnparseableRecordedValue covers the
// fail-closed half of the file guard: a recorded superseded_by that
// frontmatterScalar cannot decode to a comparable identity — block-form
// YAML with indented children, or an unparseable quoted scalar returned
// verbatim — is treated as claimed by another writer. The write refuses,
// the file stays byte-for-byte unchanged, and the refusal is reported.
func TestMarkSupersededFile_GuardRefusesUnparseableRecordedValue(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	cases := []struct {
		name   string
		backup string
	}{
		{
			name:   "block form with indented children",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by:\n  - old-1\n  - old-2\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "unparseable double-quoted scalar",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by: \"unterminated\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "unterminated single-quoted scalar",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by: 'oops\n---\n\nOriginal claim body.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backup := writeLearningBackup(t, vault, "guard-1", tc.backup)
			warning := l.markSupersededFile("guard-1", "new-1")
			if !strings.Contains(warning, "was not marked superseded in its markdown backup") ||
				!strings.Contains(warning, "already records superseded_by") {
				t.Errorf("warning = %q, want the file-guard refusal", warning)
			}
			after, err := os.ReadFile(backup)
			if err != nil || string(after) != tc.backup {
				t.Errorf("backup = %q, %v; want byte-identical to the seeded content", after, err)
			}
		})
	}
}

// TestMarkSupersededFile_GuardProceedsOnEqualOrUnclaimedBackup covers the
// proceeding half of the file guard: a backup whose recorded superseded_by
// is absent, empty, or equal to the successor about to be written — after
// supersedesIdentity normalization, so a recorded "learning/<identity>"
// page name equals the bare identity — does not refuse. The write records
// both keys, so an equal rewrite also repairs a missing validity, and the
// body is untouched.
func TestMarkSupersededFile_GuardProceedsOnEqualOrUnclaimedBackup(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	cases := []struct {
		name   string
		backup string
	}{
		{
			name:   "equal value repairs missing validity",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by: \"new-1\"\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "recorded learning/ prefix equals bare identity",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by: \"learning/new-1\"\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "absent key",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "empty quoted value",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by: \"\"\n---\n\nOriginal claim body.\n",
		},
		{
			name:   "empty bare value before a top-level key",
			backup: "---\ntag: guard\nidentity: guard-1\ntier: draft\nsuperseded_by:\nauthor: alice\n---\n\nOriginal claim body.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backup := writeLearningBackup(t, vault, "guard-1", tc.backup)
			if warning := l.markSupersededFile("guard-1", "new-1"); warning != "" {
				t.Fatalf("warning = %q, want empty (the write proceeds)", warning)
			}
			fm, body := fileFrontmatter(t, backup)
			if fm["superseded_by"] != "new-1" {
				t.Errorf("superseded_by = %v, want new-1", fm["superseded_by"])
			}
			if fm["validity"] != "superseded" {
				t.Errorf("validity = %v, want superseded", fm["validity"])
			}
			if fm["identity"] != "guard-1" || fm["tag"] != "guard" {
				t.Errorf("identity/tag = %v/%v, want guard-1/guard (only the guarded keys change)", fm["identity"], fm["tag"])
			}
			if !strings.Contains(body, "Original claim body.") {
				t.Errorf("body = %q, want untouched", body)
			}
		})
	}
}

// TestMarkSupersededFile_MissingBackupFileIsNoOp pins the no-op path: a
// learning with no markdown backup (store-only learning, or no vault) is
// not an error, and no file is created for it.
func TestMarkSupersededFile_MissingBackupFileIsNoOp(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	if warning := l.markSupersededFile("guard-absent-1", "new-1"); warning != "" {
		t.Errorf("warning = %q, want empty (a missing backup is a no-op)", warning)
	}
	path := filepath.Join(vault, deweyWorkspaceDir, "learnings", "guard-absent-1.md")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%s) = %v, want not-exist (no file created)", path, err)
	}
}

// TestMarkSupersededFile_UnsafeIdentityStillErrors keeps the pre-guard
// contract fixed (SC-004): an identity that fails ValidLearningIdentity
// errors exactly as before — the refusal policy never runs, and nothing
// is written anywhere under the vault.
func TestMarkSupersededFile_UnsafeIdentityStillErrors(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLearning(nil, s, vault, 0)

	warning := l.markSupersededFile("../../escape", "new-1")
	if !strings.Contains(warning, "unsafe learning identity") {
		t.Errorf("warning = %q, want the unsafe-identity error unchanged", warning)
	}
	entries, err := os.ReadDir(vault)
	if err != nil || len(entries) != 0 {
		t.Errorf("vault after the call = %v entries, %v; want none (no write)", entries, err)
	}
}

// TestMarkStaleUnverified_FileGuardRefusesSupersededBackup exercises the
// lint file leg's half of the same guard: the store still reads validity
// current, so the store leg marks the learning stale-unverified, but the
// backup already records a successor — a superseded_by value, or a
// validity of superseded. The file leg must leave the durable record
// byte-for-byte unchanged and surface the refusal as a warning, never an
// error, while the store leg's write stands.
func TestMarkStaleUnverified_FileGuardRefusesSupersededBackup(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	lint := NewLint(s, nil, vault, 0)

	cases := []struct {
		name        string
		identity    string
		backup      string
		wantRefusal string
	}{
		{
			name:        "backup records a successor",
			identity:    "guard-lint-super-1",
			backup:      "---\ntag: guard\nidentity: guard-lint-super-1\ntier: draft\nsuperseded_by: \"winner-1\"\nvalidity: \"current\"\n---\n\nOriginal claim body.\n",
			wantRefusal: `backup already records superseded_by "winner-1"`,
		},
		{
			name:        "backup records validity superseded",
			identity:    "guard-lint-validity-1",
			backup:      "---\ntag: guard\nidentity: guard-lint-validity-1\ntier: draft\nvalidity: \"superseded\"\n---\n\nOriginal claim body.\n",
			wantRefusal: "backup already records validity superseded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			insertPinnedLearning(t, s, tc.identity, map[string]any{"tag": "guard", "validity": "current"})
			backup := writeLearningBackup(t, vault, tc.identity, tc.backup)

			warning, err := lint.markStaleUnverified(store.LearningPrefix + tc.identity)
			if err != nil {
				t.Fatalf("markStaleUnverified error = %v; want the refusal reported as a warning", err)
			}
			if !strings.Contains(warning, "but not in its markdown backup") || !strings.Contains(warning, tc.wantRefusal) {
				t.Errorf("warning = %q, want the backup refusal %q", warning, tc.wantRefusal)
			}

			// The store leg runs first and stands.
			if _, props := pageProps(t, s, store.LearningPrefix+tc.identity); props["validity"] != "stale-unverified" {
				t.Errorf("store validity = %v, want stale-unverified (the store leg runs first)", props["validity"])
			}

			// The durable record is left byte-for-byte unchanged.
			after, readErr := os.ReadFile(backup)
			if readErr != nil || string(after) != tc.backup {
				t.Errorf("backup = %q, %v; want byte-identical to the seeded content", after, readErr)
			}
		})
	}
}
