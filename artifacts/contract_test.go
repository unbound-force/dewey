package artifacts

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

// sortedMapKeys returns the keys of m in sorted order.
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isHex reports whether s is exactly n lowercase hexadecimal characters.
func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// TestPlanJSONContract pins the documented plan JSON structure and its full
// vocabulary (task 6.2): the nine top-level keys, the repo block, the four
// source label formats, the three kind values, the findings shape (no
// context excerpt), the summary arithmetic, and the rendered summary's
// tag-grouped markers. The gitignore fix block is pinned separately in
// TestPlanGitignoreJSONContract because the bare-dewey-ignore fixture
// commits (and would sweep) staged artifacts.
func TestPlanJSONContract(t *testing.T) {
	f := newFixtureRepo(t)
	f.stashTrackedArtifact(".uf/dewey/compiled/compiled-x.md", "# compiled\n\ncompiled article body.\n")
	f.stashUntrackedArtifact(".uf/dewey/knowledge/store-a/item.md", "# knowledge\n\ncurated knowledge item.\n")
	f.commitArtifactOnBranch("feature-a", ".uf/dewey/learnings/other-20260101T000000-b.md", "# learning\n\nfrom a commit.\n", "add learning")

	// Collision: the same path held by two sources with distinct clean content.
	dup := ".uf/dewey/learnings/dup-20260101T000000-c.md"
	f.commitArtifactOnBranch("feature-b", dup, "# learning\n\nvariant one.\n", "first variant")
	f.stageArtifact(dup, "# learning\n\nvariant two.\n")

	// Working-tree-only artifact, staged AFTER the branch commits: the
	// commit helper commits everything staged, so staging earlier would
	// sweep this file into a branch commit.
	f.stageArtifact(".uf/dewey/learnings/auth-20260101T000000-a.md", "# learning\n\ntier:: draft\n\nclean working tree copy.\n")

	// Blocked: credential content (high-severity scanner finding).
	f.stageArtifact(".uf/dewey/learnings/secret-20260101T000000-d.md",
		"# learning\n\nline two has a token ghp_"+strings.Repeat("a", 36)+" in it\n")

	plan, err := BuildPlan(physicalPath(t, f.root), f.path(""), nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	// Top-level keys.
	if got := sortedMapKeys(m); !slices.Equal(got, []string{
		"artifacts", "blocked", "conflicts", "gitignore",
		"rendered_summary", "repo", "schema_version", "summary", "warnings",
	}) {
		t.Fatalf("plan top-level keys = %v, want the nine documented keys", got)
	}

	// Repo block.
	repo, ok := m["repo"].(map[string]any)
	if !ok {
		t.Fatalf("repo is %T, want object", m["repo"])
	}
	if got := sortedMapKeys(repo); !slices.Equal(got, []string{
		"artifact_roots", "base_branch", "base_sha", "publish_remote", "push_remote", "stash_refs",
	}) {
		t.Errorf("repo keys = %v", got)
	}
	if baseSHA, _ := repo["base_sha"].(string); !isHex(baseSHA, 40) {
		t.Errorf("repo.base_sha = %q, want 40-hex", repo["base_sha"])
	}
	if pr, _ := repo["publish_remote"].(string); pr == "" {
		t.Error("repo.publish_remote is empty")
	}
	if pr, _ := repo["push_remote"].(string); pr == "" {
		t.Error("repo.push_remote is empty")
	}
	if roots, _ := repo["artifact_roots"].([]any); len(roots) == 0 {
		t.Error("repo.artifact_roots is empty")
	}
	stashRefs, _ := repo["stash_refs"].(map[string]any)
	if len(stashRefs) == 0 {
		t.Error("repo.stash_refs is empty")
	}
	for label := range stashRefs {
		if !strings.HasPrefix(label, "stash@{") {
			t.Errorf("stash_refs label %q does not use the stash@{N} format", label)
		}
	}

	// Artifacts: key shape, kind enum, sha256, and the four source label
	// formats.
	artifacts, _ := m["artifacts"].([]any)
	if len(artifacts) == 0 {
		t.Fatal("artifacts array is empty")
	}
	sawKind := map[string]bool{}
	sawWorkingTree, sawStash, sawStashUntracked, sawCommit := false, false, false, false
	for i, raw := range artifacts {
		art, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("artifacts[%d] is %T, want object", i, raw)
		}
		if got := sortedMapKeys(art); !slices.Equal(got, []string{
			"findings", "identity", "kind", "path", "sha256", "sources", "status", "tag",
		}) {
			t.Errorf("artifacts[%d] keys = %v", i, got)
		}
		if sha, _ := art["sha256"].(string); !isHex(sha, 64) {
			t.Errorf("artifacts[%d].sha256 = %q, want 64-hex", i, art["sha256"])
		}
		kind, _ := art["kind"].(string)
		switch kind {
		case KindLearning, KindCompiled, KindKnowledge:
			sawKind[kind] = true
		default:
			t.Errorf("artifacts[%d].kind = %q, not in the documented enum", i, kind)
		}
		sources, _ := art["sources"].([]any)
		for _, s := range sources {
			label, _ := s.(string)
			switch {
			case label == SourceWorkingTree:
				sawWorkingTree = true
			case strings.HasPrefix(label, SourceStashPrefix+"stash@{"):
				sawStash = true
			case strings.HasPrefix(label, SourceStashUntrackedPrefix+"stash@{"):
				sawStashUntracked = true
			case strings.HasPrefix(label, SourceCommitPrefix):
				sawCommit = true
			default:
				t.Errorf("artifacts[%d] has undocumented source label %q", i, label)
			}
		}
	}
	for _, kind := range []string{KindLearning, KindCompiled, KindKnowledge} {
		if !sawKind[kind] {
			t.Errorf("kind %q not observed across artifacts", kind)
		}
	}
	if !sawWorkingTree || !sawStash || !sawStashUntracked || !sawCommit {
		t.Errorf("source label formats not all observed: working-tree=%v stash=%v stash-untracked=%v commit=%v",
			sawWorkingTree, sawStash, sawStashUntracked, sawCommit)
	}

	// Conflicts: key shape and sha256-identified variants.
	conflicts, _ := m["conflicts"].([]any)
	if len(conflicts) == 0 {
		t.Fatal("conflicts array is empty")
	}
	for i, raw := range conflicts {
		conf, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("conflicts[%d] is %T, want object", i, raw)
		}
		if got := sortedMapKeys(conf); !slices.Equal(got, []string{"identity", "path", "variants"}) {
			t.Errorf("conflicts[%d] keys = %v", i, got)
		}
		variants, _ := conf["variants"].([]any)
		if len(variants) < 2 {
			t.Errorf("conflicts[%d] has %d variants, want at least 2", i, len(variants))
		}
		for j, rawV := range variants {
			variant, ok := rawV.(map[string]any)
			if !ok {
				t.Fatalf("conflicts[%d].variants[%d] is %T, want object", i, j, rawV)
			}
			if got := sortedMapKeys(variant); !slices.Equal(got, []string{"findings", "sha256", "sources"}) {
				t.Errorf("conflicts[%d].variants[%d] keys = %v", i, j, got)
			}
			if sha, _ := variant["sha256"].(string); !isHex(sha, 64) {
				t.Errorf("conflicts[%d].variants[%d].sha256 = %q, want 64-hex", i, j, variant["sha256"])
			}
		}
	}

	// Blocked: key shape and findings with exactly the five documented
	// fields — no context excerpt.
	blocked, _ := m["blocked"].([]any)
	if len(blocked) == 0 {
		t.Fatal("blocked array is empty")
	}
	for i, raw := range blocked {
		blk, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("blocked[%d] is %T, want object", i, raw)
		}
		if got := sortedMapKeys(blk); !slices.Equal(got, []string{"findings", "identity", "path"}) {
			t.Errorf("blocked[%d] keys = %v", i, got)
		}
		findings, _ := blk["findings"].([]any)
		if len(findings) == 0 {
			t.Errorf("blocked[%d].findings is empty", i)
		}
		for j, rawF := range findings {
			finding, ok := rawF.(map[string]any)
			if !ok {
				t.Fatalf("blocked[%d].findings[%d] is %T, want object", i, j, rawF)
			}
			if got := sortedMapKeys(finding); !slices.Equal(got, []string{"line", "message", "pattern", "scanner", "severity"}) {
				t.Errorf("blocked[%d].findings[%d] keys = %v, want exactly scanner/pattern/severity/line/message", i, j, got)
			}
		}
	}

	// Gitignore status is in the documented enum (the fix block under
	// bare-dewey-fix is pinned in TestPlanGitignoreJSONContract).
	gitignore, ok := m["gitignore"].(map[string]any)
	if !ok {
		t.Fatalf("gitignore is %T, want object", m["gitignore"])
	}
	status, _ := gitignore["status"].(string)
	switch status {
	case GitignoreStatusOK, GitignoreStatusBareDeweyFix, GitignoreStatusOtherIgnoreWarning, GitignoreStatusIgnoreSourceExternal:
	default:
		t.Errorf("gitignore.status = %q, not in the documented enum", status)
	}

	// Summary arithmetic.
	summary, ok := m["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is %T, want object", m["summary"])
	}
	if got := sortedMapKeys(summary); !slices.Equal(got, []string{"blocked", "conflicts", "new", "total", "unchanged"}) {
		t.Errorf("summary keys = %v", got)
	}
	total, _ := summary["total"].(float64)
	newCount, _ := summary["new"].(float64)
	unchanged, _ := summary["unchanged"].(float64)
	conflictCount, _ := summary["conflicts"].(float64)
	blockedCount, _ := summary["blocked"].(float64)
	if total != float64(len(artifacts)) {
		t.Errorf("summary.total = %v, want len(artifacts) = %d", total, len(artifacts))
	}
	if newCount+unchanged != float64(len(artifacts)) {
		t.Errorf("summary.new+unchanged = %v, want len(artifacts) = %d", newCount+unchanged, len(artifacts))
	}
	if conflictCount != float64(len(conflicts)) {
		t.Errorf("summary.conflicts = %v, want len(conflicts) = %d", conflictCount, len(conflicts))
	}
	if blockedCount != float64(len(blocked)) {
		t.Errorf("summary.blocked = %v, want len(blocked) = %d", blockedCount, len(blocked))
	}

	// Rendered summary: the tool-rendered, tag-grouped listing.
	rendered, _ := m["rendered_summary"].(string)
	if rendered == "" {
		t.Fatal("rendered_summary is empty")
	}
	if !strings.Contains(rendered, "## Artifacts by tag") {
		t.Error("rendered_summary missing the tag-grouped section header")
	}
	if !strings.Contains(rendered, "### tag: `") {
		t.Error("rendered_summary missing a tag subsection marker")
	}
}

// TestPlanGitignoreJSONContract pins the gitignore block of the plan JSON
// under the bare-dewey-ignore posture (task 6.2): the bare-dewey-fix status
// and its bounded fix block (anchor plus runtime-state re-ignore list) with
// the file pinned to the literal .gitignore. The ignore pattern is committed
// before any artifact exists, so working-tree artifacts are stranded on disk
// (unstaged) rather than staged.
func TestPlanGitignoreJSONContract(t *testing.T) {
	f := newFixtureRepo(t)
	f.enableBareDeweyIgnore()
	f.write(".uf/dewey/learnings/wt-20260101T000000-f.md", "# learning\n\nstranded on disk under the ignore bug.\n")

	plan, err := BuildPlan(physicalPath(t, f.root), f.path(""), nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	gitignore, ok := m["gitignore"].(map[string]any)
	if !ok {
		t.Fatalf("gitignore is %T, want object", m["gitignore"])
	}
	if status, _ := gitignore["status"].(string); status != GitignoreStatusBareDeweyFix {
		t.Fatalf("gitignore.status = %q, want %q", gitignore["status"], GitignoreStatusBareDeweyFix)
	}
	fix, ok := gitignore["fix"].(map[string]any)
	if !ok {
		t.Fatal("gitignore.fix is absent under bare-dewey-fix (fix must be present iff status is bare-dewey-fix)")
	}
	if got := sortedMapKeys(fix); !slices.Equal(got, []string{"anchor", "file", "reignore"}) {
		t.Errorf("gitignore.fix keys = %v", got)
	}
	if file, _ := fix["file"].(string); file != ".gitignore" {
		t.Errorf("gitignore.fix.file = %q, want .gitignore", fix["file"])
	}
	anchor, _ := fix["anchor"].(map[string]any)
	if anchor == nil {
		t.Fatalf("gitignore.fix.anchor is %T, want object", fix["anchor"])
	}
	if got := sortedMapKeys(anchor); !slices.Equal(got, []string{"from", "to"}) {
		t.Errorf("gitignore.fix.anchor keys = %v", got)
	}
	reignore, _ := fix["reignore"].([]any)
	for _, rawR := range reignore {
		entry, _ := rawR.(string)
		if !strings.HasPrefix(entry, ".uf/dewey/") {
			t.Errorf("gitignore.fix.reignore entry %q is not .uf/dewey/-prefixed", entry)
		}
	}

	// The stranded-on-disk artifact is still planned from the working tree.
	artifacts, _ := m["artifacts"].([]any)
	found := false
	for _, raw := range artifacts {
		art, _ := raw.(map[string]any)
		if path, _ := art["path"].(string); path == ".uf/dewey/learnings/wt-20260101T000000-f.md" {
			found = true
			sources, _ := art["sources"].([]any)
			if len(sources) == 0 || sources[0] != SourceWorkingTree {
				t.Errorf("artifact sources = %v, want working-tree first", sources)
			}
		}
	}
	if !found {
		t.Error("disk-stranded artifact under the ignore bug was not planned")
	}
}

// TestSyncJSONContract pins the documented sync JSON structure (task 6.2):
// the nine result keys, the chore branch naming, the plan digest, and the
// tool-rendered PR body's tag-grouped rendering.
func TestSyncJSONContract(t *testing.T) {
	f := newFixtureRepo(t)
	f.stageArtifact(".uf/dewey/learnings/sync-20260101T000000-e.md", "# learning\n\nsync contract.\n")

	_, planPath := buildPlanFile(t, f)
	result, err := runSync(t, f, planPath, "", true, "Contract Tester")
	if err != nil {
		t.Fatalf("runSync: %v", err)
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal sync result: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal sync result: %v", err)
	}

	if got := sortedMapKeys(m); !slices.Equal(got, []string{
		"approved_by", "branch", "commit", "files_written", "gitignore_fix_applied",
		"nothing_to_publish", "plan_sha256", "pr_body", "worktree",
	}) {
		t.Fatalf("sync result keys = %v, want the nine documented keys", got)
	}

	if branch, _ := m["branch"].(string); !strings.HasPrefix(branch, "chore/dewey-artifacts-") {
		t.Errorf("branch = %q, want chore/dewey-artifacts- prefix", m["branch"])
	}
	if commit, _ := m["commit"].(string); !isHex(commit, 40) {
		t.Errorf("commit = %q, want 40-hex", m["commit"])
	}
	if files, _ := m["files_written"].([]any); len(files) == 0 {
		t.Error("files_written is empty")
	}
	if np, _ := m["nothing_to_publish"].(bool); np {
		t.Error("nothing_to_publish = true, want false")
	}
	if ab, _ := m["approved_by"].(string); ab != "Contract Tester" {
		t.Errorf("approved_by = %q, want Contract Tester", m["approved_by"])
	}

	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	if digest, _ := m["plan_sha256"].(string); digest != fixtureSHA256(planBytes) {
		t.Errorf("plan_sha256 = %q, want sha256 of the plan file bytes %q", m["plan_sha256"], fixtureSHA256(planBytes))
	}

	prBody, _ := m["pr_body"].(string)
	if !strings.Contains(prBody, "## Published knowledge artifacts") {
		t.Error("pr_body missing the published-artifacts header")
	}
	if !strings.Contains(prBody, "### tag: `") {
		t.Error("pr_body missing the tag-grouped rendering")
	}
	if !strings.Contains(prBody, "Approved by `Contract Tester`") {
		t.Error("pr_body missing the approver line")
	}
}

// TestGitignoreStatusEnum pins the documented gitignore status vocabulary.
func TestGitignoreStatusEnum(t *testing.T) {
	want := map[string]string{
		GitignoreStatusOK:                   "ok",
		GitignoreStatusBareDeweyFix:         "bare-dewey-fix",
		GitignoreStatusOtherIgnoreWarning:   "other-ignore-warning",
		GitignoreStatusIgnoreSourceExternal: "ignore-source-external",
	}
	for constant, value := range want {
		if constant != value {
			t.Errorf("gitignore status constant = %q, want %q", constant, value)
		}
	}
}
