package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoAndVault(t *testing.T, f *fixtureRepo) (string, string) {
	t.Helper()
	return physicalPath(t, f.root), f.root
}

func TestEnumerateWorkingTree_DiscoveryAndKinds(t *testing.T) {
	f := newFixtureRepo(t)
	learning := "auth-20260401T101112-alice.md"
	compiled := "authentication.md"
	f.write(filepath.Join(".uf", "dewey", "learnings", learning), "learning body\n")
	f.write(filepath.Join(".uf", "dewey", "compiled", compiled), "compiled body\n")
	f.write(filepath.Join(".uf", "dewey", "knowledge", "team", "a.md"), "knowledge body\n")

	repo, vault := repoAndVault(t, f)
	got, warnings, err := enumerateWorkingTree(repo, vault, nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if len(got) != 3 {
		t.Fatalf("candidates = %d, want 3 (%+v)", len(got), got)
	}
	byPath := make(map[string]candidate)
	for _, c := range got {
		byPath[c.Path] = c
	}
	l := byPath[".uf/dewey/learnings/"+learning]
	if l.Kind != KindLearning || l.Identity != "auth-20260401T101112-alice" || l.Tag != "auth" {
		t.Errorf("learning metadata = %+v", l)
	}
	if l.SHA256 != fixtureSHA256([]byte("learning body\n")) || l.Source != SourceWorkingTree {
		t.Errorf("learning digest/source = %+v", l)
	}
	if string(l.Content) != "learning body\n" {
		t.Errorf("learning content = %q", l.Content)
	}
	cp := byPath[".uf/dewey/compiled/"+compiled]
	if cp.Kind != KindCompiled || cp.Identity != "authentication" || cp.Tag != "authentication" {
		t.Errorf("compiled metadata = %+v", cp)
	}
	kn := byPath[".uf/dewey/knowledge/team/a.md"]
	if kn.Kind != KindKnowledge || kn.Identity != "team/a.md" || kn.Tag != "team" {
		t.Errorf("knowledge metadata = %+v", kn)
	}
}

func TestEnumerateWorkingTree_DedupVaultEqualsRepo(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(filepath.Join(".uf", "dewey", "learnings", "x-20260101T000000-a.md"), "x\n")

	repo, vault := repoAndVault(t, f)
	got, _, err := enumerateWorkingTree(repo, vault, nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want exactly 1 (vault trio must dedup against repo trio): %+v", len(got), got)
	}
	if got[0].Path != ".uf/dewey/learnings/x-20260101T000000-a.md" {
		t.Errorf("path = %q", got[0].Path)
	}
}

func TestEnumerateWorkingTree_VaultSubdirDualTrio(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(filepath.Join(".uf", "dewey", "learnings", "r-20260101T000000-amy.md"), "repo\n")
	f.write(filepath.Join("vault", ".uf", "dewey", "learnings", "v-20260101T000000-bob.md"), "vault\n")

	repo, _ := repoAndVault(t, f)
	got, _, err := enumerateWorkingTree(repo, f.path("vault"), nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	paths := make(map[string]bool)
	for _, c := range got {
		paths[c.Path] = true
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(got), got)
	}
	if !paths[".uf/dewey/learnings/r-20260101T000000-amy.md"] {
		t.Errorf("repo-anchored learning missing: %v", paths)
	}
	if !paths["vault/.uf/dewey/learnings/v-20260101T000000-bob.md"] {
		t.Errorf("vault-anchored learning missing: %v", paths)
	}
}

func TestEnumerateWorkingTree_MissingDirectoriesEmpty(t *testing.T) {
	f := newFixtureRepo(t)
	repo, vault := repoAndVault(t, f)
	got, warnings, err := enumerateWorkingTree(repo, vault, nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	if len(got) != 0 || len(warnings) != 0 {
		t.Errorf("got %d candidates, %d warnings, want empty/empty", len(got), len(warnings))
	}
}

func TestEnumerateWorkingTree_SkipsAndWarnings(t *testing.T) {
	f := newFixtureRepo(t)
	learnings := filepath.Join(".uf", "dewey", "learnings")
	f.write(filepath.Join(learnings, "auth-20260401T101112-alice.md"), "real\n")
	f.write(filepath.Join(learnings, ".DS_Store"), "noise\n")
	f.write(filepath.Join(learnings, "-weird.md"), "hyphen\n")
	f.write(filepath.Join(learnings, "co:lon.md"), "colon\n")
	outside := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "target.md"), f.path(filepath.Join(learnings, "link.md"))); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	repo, vault := repoAndVault(t, f)
	got, warnings, err := enumerateWorkingTree(repo, vault, nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want only the regular learning: %+v", len(got), got)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{
		"skipped symlink: .uf/dewey/learnings/link.md",
		"skipped unsafe basename: .uf/dewey/learnings/-weird.md",
		"skipped unsafe basename: .uf/dewey/learnings/co:lon.md",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q: %v", want, warnings)
		}
	}
	if strings.Contains(joined, ".DS_Store") {
		t.Errorf("dot-prefixed skip must be silent: %v", warnings)
	}
}

func TestEnumerateWorkingTree_IgnoresGitState(t *testing.T) {
	f := newFixtureRepo(t)
	f.stageArtifact(filepath.Join(".uf", "dewey", "learnings", "staged-20260101T000000-a.md"), "staged\n")
	f.write(filepath.Join(".uf", "dewey", "learnings", "loose-20260101T000000-b.md"), "loose\n")
	f.enableBareDeweyIgnore() // bare `dewey` pattern now ignores .uf/dewey/**

	repo, vault := repoAndVault(t, f)
	got, _, err := enumerateWorkingTree(repo, vault, nil)
	if err != nil {
		t.Fatalf("enumerateWorkingTree: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want staged+ignored+untracked all found: %+v", len(got), got)
	}
}

func TestEnumerateWorkingTree_VaultOutsideRepoFailsClosed(t *testing.T) {
	f := newFixtureRepo(t)
	outside := t.TempDir()

	repo, _ := repoAndVault(t, f)
	_, _, err := enumerateWorkingTree(repo, outside, nil)
	if err == nil {
		t.Fatalf("expected fail-closed error for vault outside the repository")
	}
	if !strings.Contains(err.Error(), "outside the git repository") {
		t.Errorf("error should name the containment failure: %v", err)
	}
}

func TestEnumerateWorkingTree_SymlinkedWalkRootFailsClosed(t *testing.T) {
	f := newFixtureRepo(t)
	if err := os.MkdirAll(f.path(filepath.Join(".uf", "dewey")), 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, f.path(filepath.Join(".uf", "dewey", "learnings"))); err != nil {
		t.Fatalf("symlink walk root: %v", err)
	}

	repo, vault := repoAndVault(t, f)
	_, _, err := enumerateWorkingTree(repo, vault, nil)
	if err == nil {
		t.Fatalf("expected fail-closed error for symlinked walk root")
	}
	if !strings.Contains(err.Error(), "symbolic-link component") || !strings.Contains(err.Error(), "learnings") {
		t.Errorf("error should name the root and offending component: %v", err)
	}
}

func TestEnumerateWorkingTree_StorePathValidation(t *testing.T) {
	f := newFixtureRepo(t)
	repo, vault := repoAndVault(t, f)

	outside, err := os.MkdirTemp("", "dewey-store")
	if err != nil {
		t.Fatalf("outside dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })

	cases := []struct {
		name    string
		store   string
		wantErr string
	}{
		{"outside repo", outside, "outside the git repository"},
		{"colon", ".uf/dewey/knowledge/te:am", "colon in path"},
		{"leading hyphen", ".uf/dewey/-store", "leading hyphen in path component"},
		{"control character", ".uf/dewey/team\x01", "control character in path"},
	}
	for _, tc := range cases {
		_, _, err := enumerateWorkingTree(repo, vault, []string{tc.store})
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}

	// A missing store is valid and empty; an existing custom store is
	// enumerated with the store name as tag.
	f.write(filepath.Join(".uf", "dewey", "team-store", "a.md"), "store body\n")
	got, _, err := enumerateWorkingTree(repo, vault, []string{".uf/dewey/team-store", ".uf/dewey/knowledge/ghost"})
	if err != nil {
		t.Fatalf("enumerateWorkingTree with stores: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want 1 from the custom store: %+v", len(got), got)
	}
	c := got[0]
	if c.Path != ".uf/dewey/team-store/a.md" || c.Kind != KindKnowledge || c.Identity != "a.md" || c.Tag != "team-store" {
		t.Errorf("custom-store candidate = %+v", c)
	}
}

func TestEnumerateWorkingTree_UnreadableFailsClosed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission checks are unreliable")
	}
	f := newFixtureRepo(t)
	lockedDir := f.path(filepath.Join(".uf", "dewey", "learnings", "locked"))
	if err := os.MkdirAll(lockedDir, 0o755); err != nil {
		t.Fatalf("mkdir locked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lockedDir, "x-20260101T000000-a.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write in locked dir: %v", err)
	}
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatalf("chmod locked dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })

	repo, vault := repoAndVault(t, f)
	_, _, err := enumerateWorkingTree(repo, vault, nil)
	if err == nil {
		t.Fatalf("expected fail-closed error for unreadable directory")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("error should name the unreadable path: %v", err)
	}
}
