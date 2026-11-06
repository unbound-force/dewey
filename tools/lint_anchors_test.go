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

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// anchorRepo creates a git repository with pkg/a.go and pkg/b.go at a
// first commit, then changes pkg/a.go in a second commit. Returns the
// repository path and the first commit.
func anchorRepo(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	write("pkg/a.go", "package pkg // v1\n")
	write("pkg/b.go", "package pkg\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	first, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	write("pkg/a.go", "package pkg // v2\n")
	git("commit", "-q", "-am", "second")
	return dir, first
}

// insertPinnedLearning stores a learning page with the given properties.
func insertPinnedLearning(t *testing.T, s *store.Store, identity string, props map[string]any) {
	t.Helper()
	data, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	page := &store.Page{
		Name:         store.LearningPrefix + identity,
		OriginalName: store.LearningPrefix + identity,
		SourceID:     store.SourceLearning,
		SourceDocID:  "learning-" + identity,
		Properties:   string(data),
		Tier:         "draft",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
}

// runLint runs the lint tool and returns its parsed JSON result.
func runLint(t *testing.T, l *Lint, fix bool) map[string]any {
	t.Helper()
	result, _, err := l.Lint(context.Background(), nil, types.LintInput{Fix: fix})
	if err != nil || result.IsError {
		t.Fatalf("Lint = %v, %v", resultText(result), err)
	}
	return parseLearningResult(t, resultText(result))
}

// anchorFindingsOf returns "type identity" strings for anchor findings.
func anchorFindingsOf(t *testing.T, parsed map[string]any) []string {
	t.Helper()
	findings, _ := parsed["findings"].([]any)
	var out []string
	for _, f := range findings {
		m, ok := f.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["type"].(string)
		if strings.HasPrefix(kind, "anchor_") {
			desc, _ := m["description"].(string)
			out = append(out, kind+" "+desc)
		}
	}
	return out
}

func TestLint_AnchorChecks(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{
		"tag":   "gate",
		"as_of": "kb@" + first,
		"evidence": []string{
			"kb:pkg/a.go",       // changed since the pin → drift
			"kb:pkg/b.go:10-20", // unchanged; line suffix stripped
			"kb:nope.go",        // never existed → missing
			"other:pkg/a.go",    // another repository → skipped
			"kb#12",             // issue number → skipped
		},
	})
	insertPinnedLearning(t, s, "old-1", map[string]any{"tag": "old", "as_of": "kb@" + first, "validity": "historical", "evidence": []string{"kb:pkg/a.go"}})
	insertPinnedLearning(t, s, "gone-1", map[string]any{"tag": "gone", "as_of": "kb@" + first, "validity": "superseded", "evidence": []string{"kb:pkg/a.go"}})
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{"tag": "unpinned", "evidence": []string{"kb:pkg/a.go"}})
	insertPinnedLearning(t, s, "lost-1", map[string]any{"tag": "lost", "as_of": "kb@" + strings.Repeat("0", 40), "evidence": []string{"kb:pkg/a.go"}})

	parsed := runLint(t, NewLint(s, nil, repo, 0), false)
	got := anchorFindingsOf(t, parsed)
	if len(got) != 3 {
		t.Fatalf("anchor findings = %d, want 3:\n%s", len(got), strings.Join(got, "\n"))
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"anchor_drift Learning 'gate-1' cites pkg/a.go, which changed",
		"anchor_missing Learning 'gate-1' cites nope.go, which does not exist",
		"anchor_missing Learning 'lost-1' is pinned to commit 0000000",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing finding %q in:\n%s", want, joined)
		}
	}
	summary, _ := parsed["summary"].(map[string]any)
	if summary["anchor_drift"] != float64(1) || summary["anchor_missing"] != float64(2) {
		t.Errorf("summary anchor counts = %v / %v", summary["anchor_drift"], summary["anchor_missing"])
	}
}

func TestLint_AnchorFixMarksDriftedLearnings(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/a.go"}})
	insertPinnedLearning(t, s, "steady-1", map[string]any{"tag": "steady", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/b.go"}})

	dir := filepath.Join(repo, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	backup := filepath.Join(dir, "gate-1.md")
	if err := os.WriteFile(backup, []byte("---\ntag: gate\ntier: draft\nvalidity: \"current\"\n---\n\nGate body.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	parsed := runLint(t, NewLint(s, nil, repo, 0), true)
	fixed, _ := parsed["fixed"].(map[string]any)
	if fixed["anchor_drift"] != float64(1) {
		t.Errorf("fixed.anchor_drift = %v, want 1", fixed["anchor_drift"])
	}

	page, props := pageProps(t, s, "learning/gate-1")
	if props["validity"] != "stale-unverified" || page.Tier != "draft" {
		t.Errorf("drifted learning validity/tier = %v / %q", props["validity"], page.Tier)
	}
	if _, steady := pageProps(t, s, "learning/steady-1"); steady["validity"] != "current" {
		t.Errorf("unchanged learning validity = %v, want current", steady["validity"])
	}
	fm, body := fileFrontmatter(t, backup)
	if fm["validity"] != "stale-unverified" || !strings.Contains(body, "Gate body.") {
		t.Errorf("backup frontmatter/body = %v / %q", fm, body)
	}
}

func TestLint_AnchorChecksSkippedOutsideGit(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"}})

	for name, l := range map[string]*Lint{
		"no vault":       NewLint(s, nil, "", 0),
		"not a git repo": NewLint(s, nil, t.TempDir(), 0),
	} {
		if got := anchorFindingsOf(t, runLint(t, l, false)); len(got) != 0 {
			t.Errorf("%s: anchor findings = %v, want none", name, got)
		}
	}
}

// fakeAnchors is an anchorChecker with scripted failures.
type fakeAnchors struct {
	commitErr, pathErr, diffErr error
}

func (f fakeAnchors) Head() (string, error)                   { return "head", nil }
func (f fakeAnchors) CommitExists(string) (bool, error)       { return true, f.commitErr }
func (f fakeAnchors) PathExists(string, string) (bool, error) { return true, f.pathErr }
func (f fakeAnchors) PathChanged(string, string, string) (bool, error) {
	return false, f.diffErr
}

func TestCheckAnchors_CheckerErrors(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"}})
	boom := errors.New("boom")

	for name, checker := range map[string]fakeAnchors{
		"commit": {commitErr: boom},
		"path":   {pathErr: boom},
		"diff":   {diffErr: boom},
	} {
		l := NewLint(s, nil, "", 0)
		l.anchors = checker
		if _, err := l.checkAnchors(); !errors.Is(err, boom) {
			t.Errorf("%s: checkAnchors error = %v, want boom", name, err)
		}
		// The lint tool logs the failure and still completes.
		if got := anchorFindingsOf(t, runLint(t, l, false)); len(got) != 0 {
			t.Errorf("%s: anchor findings = %v, want none", name, got)
		}
	}
}

func TestGitAnchorChecker_Errors(t *testing.T) {
	repo, first := anchorRepo(t)
	g := gitAnchorChecker{dir: repo}
	// A bad revision makes git diff exit 128, which is an error, not a change.
	if _, err := g.PathChanged("not-a-rev", first, "pkg/a.go"); err == nil {
		t.Error("PathChanged with a bad revision: expected error")
	}
	// A missing directory cannot run git at all.
	if code, err := (gitAnchorChecker{dir: filepath.Join(repo, "missing")}).exitCode("status"); err != nil || code == 0 {
		t.Errorf("exitCode in missing dir = %d, %v; want non-zero exit, nil error", code, err)
	}
}

func TestSplitAsOfAndParseAnchor(t *testing.T) {
	for in, want := range map[string]string{
		"kb@abc":     "kb abc true",
		"my@repo@ab": "my@repo ab true",
		"@abc":       "  false",
		"kb@":        "  false",
		"":           "  false",
	} {
		repo, sha, ok := splitAsOf(in)
		got := repo + " " + sha + " "
		if ok {
			got += "true"
		} else {
			got += "false"
		}
		if got != want {
			t.Errorf("splitAsOf(%q) = %q, want %q", in, got, want)
		}
	}
	for locator, want := range map[string]string{
		"kb:pkg/a.go":       "pkg/a.go",
		"kb:pkg/a.go:12":    "pkg/a.go",
		"kb:pkg/a.go:12-40": "pkg/a.go",
		"kb:./pkg/a.go":     "pkg/a.go",
		"other:pkg/a.go":    "",
		"kb#12":             "",
	} {
		if got := parseAnchor(locator, "kb"); got != want {
			t.Errorf("parseAnchor(%q) = %q, want %q", locator, got, want)
		}
	}
	if got := parseAnchor("kb:x", ""); got != "" {
		t.Errorf("parseAnchor with empty repo = %q, want empty", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA(abc) = %q", got)
	}
}

// insertRawLearning stores a learning page with verbatim properties text.
func insertRawLearning(t *testing.T, s *store.Store, identity, properties string) {
	t.Helper()
	name := store.LearningPrefix + identity
	if err := s.InsertPage(&store.Page{Name: name, OriginalName: name, SourceID: store.SourceLearning, SourceDocID: name, Properties: properties, Tier: "draft"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
}

func TestMarkStaleUnverified_FailsClosed(t *testing.T) {
	s := newTestStore(t)
	l := NewLint(s, nil, t.TempDir(), 0)

	// Corrupt properties are reported and left intact, never replaced.
	insertRawLearning(t, s, "corrupt-1", "{not json")
	if err := l.markStaleUnverified("learning/corrupt-1"); err == nil {
		t.Error("corrupt properties: expected error")
	}
	page, err := s.GetPage("learning/corrupt-1")
	if err != nil || page == nil {
		t.Fatalf("GetPage = %v, %v", page, err)
	}
	if page.Properties != "{not json" {
		t.Errorf("corrupt properties were rewritten: %q", page.Properties)
	}

	// A literal JSON null and empty properties decode to an empty map.
	for identity, raw := range map[string]string{"null-1": "null", "empty-1": ""} {
		insertRawLearning(t, s, identity, raw)
		if err := l.markStaleUnverified("learning/" + identity); err != nil {
			t.Errorf("%s: %v", identity, err)
		}
		if _, props := pageProps(t, s, "learning/"+identity); props["validity"] != "stale-unverified" {
			t.Errorf("%s properties = %v", identity, props)
		}
	}
}

func TestMarkStaleUnverified_RejectsUnsafeIdentity(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLint(s, nil, vault, 0)
	insertRawLearning(t, s, "../../escape", `{"tag":"x"}`)
	outside := filepath.Join(vault, deweyWorkspaceDir, "..", "..", "escape.md")
	original := "---\ntag: x\n---\n\nOutside.\n"
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(outside, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := l.markStaleUnverified("learning/../../escape"); err == nil {
		t.Fatal("unsafe identity: expected error")
	}
	if _, props := pageProps(t, s, "learning/../../escape"); props["validity"] != nil {
		t.Errorf("store was written despite the unsafe identity: %v", props)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != original {
		t.Errorf("file outside the learnings directory changed: %q, %v", content, err)
	}
}

func TestLearningAnchors_IgnoresNonHexPin(t *testing.T) {
	for _, asOf := range []string{"kb@--output=/tmp/x", "kb@HEAD", "kb@main"} {
		data, err := json.Marshal(map[string]any{"as_of": asOf, "evidence": []string{"kb:pkg/a.go"}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, _, ok := learningAnchors(&store.Page{Properties: string(data)}); ok {
			t.Errorf("learningAnchors accepted pin %q", asOf)
		}
	}
	data, err := json.Marshal(map[string]any{"as_of": "kb@0123abcDEF", "evidence": []string{"kb:pkg/a.go"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if sha, paths, ok := learningAnchors(&store.Page{Properties: string(data)}); !ok || sha != "0123abcDEF" || len(paths) != 1 {
		t.Errorf("learningAnchors(hex pin) = %q, %v, %v", sha, paths, ok)
	}
}

func TestMarkStaleUnverified_Errors(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLint(s, nil, vault, 0)
	if err := l.markStaleUnverified("learning/absent-1"); err == nil {
		t.Error("absent page: expected error")
	}
	insertPinnedLearning(t, s, "bare-1", map[string]any{"tag": "bare"})
	dir := filepath.Join(vault, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bare-1.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := l.markStaleUnverified("learning/bare-1"); err == nil {
		t.Error("backup without frontmatter: expected error")
	}
	// fixAnchorDrift skips failures and counts only successes.
	if got := l.fixAnchorDrift([]Finding{{Type: "anchor_drift", Page: "learning/bare-1"}, {Type: "anchor_missing", Page: "learning/x"}}); got != 0 {
		t.Errorf("fixAnchorDrift = %d, want 0", got)
	}
}
