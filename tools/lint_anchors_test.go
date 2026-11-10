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
	"time"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// anchorRepo creates a git repository with pkg/a.go, pkg/b.go, and
// pkg/c.go at a first commit, then changes pkg/a.go and deletes pkg/c.go in
// a second commit. Returns the repository path (named kb) and the first commit.
func anchorRepo(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// The directory's base name is the repository name as_of pins use.
	dir := filepath.Join(t.TempDir(), "kb")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
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
	write("pkg/c.go", "package pkg // removed later\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	first, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	write("pkg/a.go", "package pkg // v2\n")
	git("rm", "-q", "pkg/c.go")
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

// anchorChecksOf returns the status and reason of the result's anchor_checks.
func anchorChecksOf(t *testing.T, parsed map[string]any) (string, string) {
	t.Helper()
	checks, ok := parsed["anchor_checks"].(map[string]any)
	if !ok {
		t.Fatalf("anchor_checks missing from result: %v", parsed)
	}
	status, _ := checks["status"].(string)
	reason, _ := checks["reason"].(string)
	return status, reason
}

// firstString returns the first element of list as a string.
func firstString(list []any) (string, bool) {
	if len(list) == 0 {
		return "", false
	}
	str, ok := list[0].(string)
	return str, ok
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
			"kb:pkg/a.go:3",     // same path again → no second finding
			"kb:./pkg/a.go",     // same path again → no second finding
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
	// An unknown pin is one finding per learning, not one per anchor.
	insertPinnedLearning(t, s, "lost-2", map[string]any{"tag": "lost", "as_of": "kb@" + strings.Repeat("1", 40), "evidence": []string{"kb:pkg/a.go", "kb:pkg/b.go"}})
	// Present at the pin, deleted since: drift, not missing.
	insertPinnedLearning(t, s, "deleted-1", map[string]any{"tag": "deleted", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/c.go"}})
	// Pinned to another repository: not checked against this one, even
	// though its commit is not here and its paths exist here.
	insertPinnedLearning(t, s, "foreign-1", map[string]any{"tag": "foreign", "as_of": "other@" + strings.Repeat("2", 40), "evidence": []string{"other:pkg/a.go", "kb#3"}})
	insertPinnedLearning(t, s, "foreign-2", map[string]any{"tag": "foreign", "as_of": "other@" + first, "evidence": []string{"other:pkg/a.go"}})
	// Pinned here, but no path evidence in this repository: nothing to
	// compare, so not checked, even with an unknown pin.
	insertPinnedLearning(t, s, "issues-1", map[string]any{"tag": "issues", "as_of": "kb@" + strings.Repeat("3", 40), "evidence": []string{"kb#3", "other:pkg/a.go"}})
	// Unreadable properties are skipped by the check, not reported.
	insertRawLearning(t, s, "corrupt-1", "{not json")
	// Evidence over store_learning's limits (re-ingested) is not checked.
	flood := make([]string, maxEvidenceEntries+1)
	for i := range flood {
		flood[i] = "kb:pkg/a.go"
	}
	insertPinnedLearning(t, s, "flood-1", map[string]any{"tag": "flood", "as_of": "kb@" + first, "evidence": flood})
	insertPinnedLearning(t, s, "long-1", map[string]any{"tag": "long", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go", "kb:" + strings.Repeat("x", maxEvidenceLen)}})
	// Exactly at the limits is still checked: 50 entries, and a locator of
	// exactly 512 runes (multi-byte, so runes are counted, not bytes).
	atLimit := make([]string, maxEvidenceEntries)
	for i := range atLimit {
		atLimit[i] = "kb:pkg/b.go"
	}
	atLimit[0] = "kb:pkg/a.go"
	insertPinnedLearning(t, s, "edge-1", map[string]any{"tag": "edge", "as_of": "kb@" + first, "evidence": atLimit})
	insertPinnedLearning(t, s, "edge-2", map[string]any{"tag": "edge", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go", "kb:" + strings.Repeat("é", maxEvidenceLen-len("kb:"))}})

	parsed := runLint(t, NewLint(s, nil, repo, 0), false)
	if status, reason := anchorChecksOf(t, parsed); status != anchorChecked || reason != "" {
		t.Errorf("anchor_checks = %q (%q), want checked", status, reason)
	}
	got := anchorFindingsOf(t, parsed)
	if len(got) != 7 {
		t.Fatalf("anchor findings = %d, want 7:\n%s", len(got), strings.Join(got, "\n"))
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"anchor_drift Learning 'gate-1' cites pkg/a.go, which changed",
		"anchor_missing Learning 'gate-1' cites nope.go, which does not exist",
		"anchor_missing Learning 'lost-1' is pinned to commit 0000000",
		"anchor_missing Learning 'lost-2' is pinned to commit 1111111",
		"anchor_drift Learning 'deleted-1' cites pkg/c.go, which changed or was removed",
		"anchor_drift Learning 'edge-1' cites pkg/a.go, which changed",
		"anchor_missing Learning 'edge-2' cites éé",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing finding %q in:\n%s", want, joined)
		}
	}
	summary, _ := parsed["summary"].(map[string]any)
	if summary["anchor_drift"] != float64(3) || summary["anchor_missing"] != float64(4) {
		t.Errorf("summary anchor counts = %v / %v, want 3 / 4", summary["anchor_drift"], summary["anchor_missing"])
	}
}

func TestLint_AnchorFixMarksDriftedLearnings(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/a.go"}})
	insertPinnedLearning(t, s, "steady-1", map[string]any{"tag": "steady", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/b.go"}})
	// Two drifted anchors on one learning: two findings, one learning marked.
	insertPinnedLearning(t, s, "multi-1", map[string]any{"tag": "multi", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/a.go", "kb:pkg/c.go"}})
	// Only an anchor_missing finding: fix must leave it alone.
	insertPinnedLearning(t, s, "missing-1", map[string]any{"tag": "missing", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:nope.go"}})
	// Drifted, with a backup that cannot be updated: marked in the store,
	// counted, and reported as a warning.
	insertPinnedLearning(t, s, "nofm-1", map[string]any{"tag": "nofm", "as_of": "kb@" + first, "validity": "current", "evidence": []string{"kb:pkg/a.go"}})

	dir := filepath.Join(repo, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	backup := filepath.Join(dir, "gate-1.md")
	if err := os.WriteFile(backup, []byte("---\ntag: gate\ntier: draft\nvalidity: \"current\"\n---\n\nGate body.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	nofm := filepath.Join(dir, "nofm-1.md")
	if err := os.WriteFile(nofm, []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	parsed := runLint(t, NewLint(s, nil, repo, 0), true)
	summary, _ := parsed["summary"].(map[string]any)
	if summary["anchor_drift"] != float64(4) || summary["anchor_missing"] != float64(1) {
		t.Errorf("summary anchor counts = %v / %v, want 4 / 1", summary["anchor_drift"], summary["anchor_missing"])
	}
	fixed, _ := parsed["fixed"].(map[string]any)
	// gate-1, multi-1 (two findings, one learning), and nofm-1.
	if fixed["anchor_drift"] != float64(3) {
		t.Errorf("fixed.anchor_drift = %v, want 3", fixed["anchor_drift"])
	}
	warnings, _ := parsed["anchor_fix_warnings"].([]any)
	if first, _ := firstString(warnings); len(warnings) != 1 || !strings.Contains(first, "nofm-1") {
		t.Errorf("anchor_fix_warnings = %v, want one for nofm-1", warnings)
	}
	for _, identity := range []string{"multi-1", "nofm-1"} {
		if _, props := pageProps(t, s, "learning/"+identity); props["validity"] != "stale-unverified" {
			t.Errorf("%s validity = %v, want stale-unverified", identity, props["validity"])
		}
	}
	if _, props := pageProps(t, s, "learning/missing-1"); props["validity"] != "current" {
		t.Errorf("anchor_missing-only learning validity = %v, want current", props["validity"])
	}
	if content, err := os.ReadFile(nofm); err != nil || string(content) != "no frontmatter\n" {
		t.Errorf("unwritable backup changed: %q, %v", content, err)
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

	for name, tc := range map[string]struct {
		lint   *Lint
		reason string
	}{
		"no vault":       {NewLint(s, nil, "", 0), "no vault path"},
		"not a git repo": {NewLint(s, nil, t.TempDir(), 0), errAnchorNoRepo.Error()},
	} {
		parsed := runLint(t, tc.lint, false)
		if got := anchorFindingsOf(t, parsed); len(got) != 0 {
			t.Errorf("%s: anchor findings = %v, want none", name, got)
		}
		if status, reason := anchorChecksOf(t, parsed); status != anchorSkipped || reason != tc.reason {
			t.Errorf("%s: anchor_checks = %q (%q), want skipped (%q)", name, status, reason, tc.reason)
		}
	}
}

// fakeAnchors is an anchorChecker with scripted failures.
type fakeAnchors struct {
	headErr, commitErr, pathErr, diffErr error
}

func (f fakeAnchors) Head(context.Context) (string, error) { return "head", f.headErr }
func (f fakeAnchors) Repo(context.Context) (string, error) { return "kb", nil }
func (f fakeAnchors) CommitExists(context.Context, string) (bool, error) {
	return true, f.commitErr
}
func (f fakeAnchors) PathExists(context.Context, string, string) (bool, error) {
	return true, f.pathErr
}
func (f fakeAnchors) PathChanged(context.Context, string, string, string) (bool, error) {
	return false, f.diffErr
}

func TestCheckAnchors_CheckerErrors(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"}})
	boom := errors.New("boom")

	for name, checker := range map[string]fakeAnchors{
		"head":   {headErr: boom},
		"commit": {commitErr: boom},
		"path":   {pathErr: boom},
		"diff":   {diffErr: boom},
	} {
		l := NewLint(s, nil, "", 0)
		l.anchors = checker
		if _, report, err := l.checkAnchors(context.Background()); !errors.Is(err, boom) || report.Status != anchorFailed {
			t.Errorf("%s: checkAnchors = %v, %v; want failed, boom", name, report, err)
		}
		// The lint tool still completes, and reports the failed pass.
		parsed := runLint(t, l, false)
		if got := anchorFindingsOf(t, parsed); len(got) != 0 {
			t.Errorf("%s: anchor findings = %v, want none", name, got)
		}
		if status, reason := anchorChecksOf(t, parsed); status != anchorFailed || !strings.Contains(reason, "boom") {
			t.Errorf("%s: anchor_checks = %q (%q), want failed (boom)", name, status, reason)
		}
	}
}

func TestCheckAnchors_CancelledContextFails(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{"tag": "gate", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	findings, report, err := NewLint(s, nil, repo, 0).checkAnchors(ctx)
	if err == nil || report.Status != anchorFailed || len(findings) != 0 {
		t.Errorf("checkAnchors(cancelled) = %v, %v, %v; want failed with no findings", findings, report, err)
	}
}

func TestGitAnchorChecker_IgnoresRepositoryDiffDrivers(t *testing.T) {
	repo, first := anchorRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	driver := filepath.Join(t.TempDir(), "driver.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\ntouch "+marker+"\ncat \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for _, kv := range [][2]string{{"diff.external", driver}, {"diff.hostile.textconv", driver}} {
		if out, err := exec.Command("git", "-C", repo, "config", kv[0], kv[1]).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.go diff=hostile\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	g := gitAnchorChecker{dir: repo}
	ctx := context.Background()
	head, err := g.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if changed, err := g.PathChanged(ctx, first, head, "pkg/a.go"); err != nil || !changed {
		t.Errorf("PathChanged(a.go) = %v, %v; want true", changed, err)
	}
	if changed, err := g.PathChanged(ctx, first, head, "pkg/b.go"); err != nil || changed {
		t.Errorf("PathChanged(b.go) = %v, %v; want false", changed, err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a repository-configured diff driver ran")
	}
}

func TestGitAnchorChecker_Errors(t *testing.T) {
	repo, first := anchorRepo(t)
	g := gitAnchorChecker{dir: repo}
	ctx := context.Background()
	// A bad revision makes git diff exit 128, which is an error, not a change.
	if _, err := g.PathChanged(ctx, "not-a-rev", first, "pkg/a.go"); err == nil {
		t.Error("PathChanged with a bad revision: expected error")
	}
	// A missing directory runs git, which exits non-zero: no repository.
	missing := gitAnchorChecker{dir: filepath.Join(repo, "missing")}
	if _, code, err := missing.run(ctx, "status"); err != nil || code == 0 {
		t.Errorf("run in missing dir = %d, %v; want non-zero exit, nil error", code, err)
	}
	if _, err := missing.Head(ctx); !errors.Is(err, errAnchorNoRepo) {
		t.Errorf("Head in missing dir = %v, want errAnchorNoRepo", err)
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
	if _, err := l.markStaleUnverified("learning/corrupt-1"); err == nil {
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
		if _, err := l.markStaleUnverified("learning/" + identity); err != nil {
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
	// The backup path is vault/.uf/dewey/learnings/<identity>.md, so the
	// identity's two ".." land on vault/.uf/escape.md.
	outside := filepath.Join(vault, deweyWorkspaceDir, "..", "escape.md")
	original := "---\ntag: x\n---\n\nOutside.\n"
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(outside, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := l.markStaleUnverified("learning/../../escape"); err == nil {
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
		if _, _, ok := learningAnchors(&store.Page{Properties: string(data)}, "kb"); ok {
			t.Errorf("learningAnchors accepted pin %q", asOf)
		}
	}
	data, err := json.Marshal(map[string]any{"as_of": "kb@0123abcDEF", "evidence": []string{"kb:pkg/a.go"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if sha, paths, ok := learningAnchors(&store.Page{Properties: string(data)}, "kb"); !ok || sha != "0123abcDEF" || len(paths) != 1 {
		t.Errorf("learningAnchors(hex pin) = %q, %v, %v", sha, paths, ok)
	}
}

func TestMarkStaleUnverified_Errors(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	l := NewLint(s, nil, vault, 0)
	if _, err := l.markStaleUnverified("learning/absent-1"); err == nil {
		t.Error("absent page: expected error")
	}
	insertPinnedLearning(t, s, "bare-1", map[string]any{"tag": "bare", "validity": "current"})
	dir := filepath.Join(vault, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	backup := filepath.Join(dir, "bare-1.md")
	if err := os.WriteFile(backup, []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The store write stands; the backup failure is a warning.
	warning, err := l.markStaleUnverified("learning/bare-1")
	if err != nil || !strings.Contains(warning, "but not in its markdown backup") {
		t.Errorf("backup without frontmatter = %q, %v; want a backup warning", warning, err)
	}
	page, props := pageProps(t, s, "learning/bare-1")
	if props["validity"] != "stale-unverified" || props["tag"] != "bare" || len(props) != 2 || page.Tier != "draft" {
		t.Errorf("stored learning after the backup failure = %v (tier %q)", props, page.Tier)
	}
	if content, err := os.ReadFile(backup); err != nil || string(content) != "no frontmatter\n" {
		t.Errorf("backup after the failure = %q, %v; want unchanged", content, err)
	}

	// fixAnchorDrift counts store writes, skips non-drift findings, and
	// reports both a failed learning and a backup-only failure.
	marked, warnings := l.fixAnchorDrift([]Finding{
		{Type: "anchor_drift", Page: "learning/bare-1"},
		{Type: "anchor_drift", Page: "learning/absent-1"},
		{Type: "anchor_missing", Page: "learning/x"},
	})
	if marked != 1 || len(warnings) != 2 {
		t.Fatalf("fixAnchorDrift = %d, %v; want 1 marked and 2 warnings", marked, warnings)
	}
	// Each warning names its own learning and says what happened to it.
	if !strings.Contains(warnings[0], "marked bare-1 stale-unverified in the store, but not in its markdown backup") {
		t.Errorf("backup warning = %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "failed to mark learning/absent-1 stale-unverified: page not found") {
		t.Errorf("failure warning = %q", warnings[1])
	}

	// A learning superseded after the check ran keeps its validity.
	insertPinnedLearning(t, s, "late-1", map[string]any{"tag": "late", "validity": "superseded", "superseded_by": "late-2"})
	if _, err := l.markStaleUnverified("learning/late-1"); err == nil || !strings.Contains(err.Error(), "now superseded") {
		t.Errorf("superseded learning: err = %v, want a now-superseded error", err)
	}
	if _, props := pageProps(t, s, "learning/late-1"); props["validity"] != "superseded" || props["superseded_by"] != "late-2" {
		t.Errorf("superseded learning after fix = %v", props)
	}
}

func TestLearningPropertyUpdates_ConcurrentFixAndSupersede(t *testing.T) {
	s := newTestStore(t)
	vault := t.TempDir()
	lint := NewLint(s, nil, vault, 0)
	learning := NewLearning(nil, s, vault, 0)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("race-%d", i)
		insertPinnedLearning(t, s, id, map[string]any{"tag": "race", "validity": "current"})
		target, err := s.GetPage(store.LearningPrefix + id)
		if err != nil || target == nil {
			t.Fatalf("GetPage = %v, %v", target, err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = lint.markStaleUnverified(store.LearningPrefix + id)
		}()
		go func() {
			defer wg.Done()
			_ = learning.linkSuperseded(target, "next-"+id)
		}()
		wg.Wait()
		// In either order the supersession survives: the fix never drops
		// the back-link, and never overwrites superseded validity.
		if _, props := pageProps(t, s, store.LearningPrefix+id); props["superseded_by"] != "next-"+id || props["validity"] != "superseded" {
			t.Errorf("%s properties = %v, want superseded by next-%s", id, props, id)
		}
	}
}

// blockingAnchors is an anchorChecker whose Head waits for its context.
type blockingAnchors struct{ fakeAnchors }

func (blockingAnchors) Head(ctx context.Context) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestCheckAnchors_BudgetBoundsThePass(t *testing.T) {
	l := NewLint(newTestStore(t), nil, "", 0)
	l.anchors = blockingAnchors{}
	l.anchorBudget = 10 * time.Millisecond
	type outcome struct {
		report anchorReport
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		_, report, err := l.checkAnchors(context.Background())
		done <- outcome{report, err}
	}()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.DeadlineExceeded) || got.report.Status != anchorFailed {
			t.Errorf("checkAnchors past its budget = %v, %v; want failed, deadline exceeded", got.report, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("anchor-check pass was not bounded by its budget")
	}
}

func TestGitAnchorChecker_TimeoutBoundsEachCall(t *testing.T) {
	repo, _ := anchorRepo(t)
	g := gitAnchorChecker{dir: repo, timeout: time.Nanosecond}
	if _, err := g.Head(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Head with an expired per-call timeout = %v, want deadline exceeded", err)
	}
}

func TestGitAnchorChecker_PathIsLiteral(t *testing.T) {
	repo, _ := anchorRepo(t)
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// pkg/[ab].go is a real file; as a glob it would also match pkg/a.go.
	if err := os.WriteFile(filepath.Join(repo, "pkg", "[ab].go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "glob-named file")
	pin, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pkg", "a.go"), []byte("package pkg // v3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git("commit", "-q", "-am", "change a.go only")
	g := gitAnchorChecker{dir: repo}
	ctx := context.Background()
	head, err := g.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	for _, path := range []string{"pkg/[ab].go", ":(exclude)pkg/b.go", "pkg/*.go"} {
		if changed, err := g.PathChanged(ctx, pin, head, path); err != nil || changed {
			t.Errorf("PathChanged(%q) = %v, %v; want false (only pkg/a.go changed)", path, changed, err)
		}
	}
	if changed, err := g.PathChanged(ctx, pin, head, "pkg/a.go"); err != nil || !changed {
		t.Errorf("PathChanged(pkg/a.go) = %v, %v; want true", changed, err)
	}
}
