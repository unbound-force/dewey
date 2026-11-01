package artifacts

import (
	"os"
	"strings"
	"testing"
)

func TestEnumerateCommits_UnpushedCommitOnBranch(t *testing.T) {
	f := newFixtureRepo(t)
	f.commitArtifactOnBranch("feature", ".uf/dewey/learnings/auth-20260401T101112-alice.md", "learning body\n", "add learning")
	before := f.capture()

	cands, warnings, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	f.assertUnchanged(before)
	if len(cands) != 1 || len(warnings) != 0 {
		t.Fatalf("got %d candidates, %d warnings; want 1, 0", len(cands), len(warnings))
	}
	sha := strings.TrimSpace(f.git("rev-parse", "feature"))
	c := cands[0]
	if c.Path != ".uf/dewey/learnings/auth-20260401T101112-alice.md" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.Identity != "auth-20260401T101112-alice" {
		t.Errorf("Identity = %q", c.Identity)
	}
	if c.Tag != "auth" {
		t.Errorf("Tag = %q", c.Tag)
	}
	if c.Kind != KindLearning {
		t.Errorf("Kind = %q", c.Kind)
	}
	if want := fixtureSHA256([]byte("learning body\n")); c.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", c.SHA256, want)
	}
	if c.Source != "commit:"+sha {
		t.Errorf("Source = %q, want %q", c.Source, "commit:"+sha)
	}
	if string(c.Content) != "learning body\n" {
		t.Errorf("Content = %q", string(c.Content))
	}
}

func TestEnumerateCommits_NewestTouchingCommitWins(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	f.git("checkout", "-b", "feature")
	f.write(rel, "v1 body\n")
	f.git("add", rel)
	f.git("commit", "-m", "v1")
	f.write(rel, "v2 body\n")
	f.git("add", rel)
	f.git("commit", "-m", "v2")
	f.git("checkout", "main")

	cands, _, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("got %d candidates; want 1", len(cands))
	}
	if want := fixtureSHA256([]byte("v2 body\n")); cands[0].SHA256 != want {
		t.Errorf("SHA256 = %q, want newest %q", cands[0].SHA256, want)
	}
	if string(cands[0].Content) != "v2 body\n" {
		t.Errorf("Content = %q, want newest content", string(cands[0].Content))
	}
	sha := strings.TrimSpace(f.git("rev-parse", "feature"))
	if cands[0].Source != "commit:"+sha {
		t.Errorf("Source = %q, want %q", cands[0].Source, "commit:"+sha)
	}
}

func TestEnumerateCommits_NoUnpushedCommits(t *testing.T) {
	f := newFixtureRepo(t)
	cands, warnings, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	if len(cands) != 0 || len(warnings) != 0 {
		t.Fatalf("got %d candidates, %d warnings; want 0, 0", len(cands), len(warnings))
	}
}

func TestEnumerateCommits_PushedBranchExcluded(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	f.commitArtifactOnBranch("feature", rel, "pushed body\n", "add learning")

	cands, _, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits before push: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("before push: got %d candidates; want 1", len(cands))
	}

	f.git("push", "origin", "feature")
	cands, _, err = enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits after push: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("after push: got %d candidates; want 0", len(cands))
	}
}

func TestEnumerateCommits_TrioAtAnyDepth(t *testing.T) {
	f := newFixtureRepo(t)
	f.commitArtifactOnBranch("feature", "sub/.uf/dewey/learnings/x-tag-20260401T101112-alice.md", "nested body\n", "nested")

	cands, _, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("got %d candidates; want 1", len(cands))
	}
	if cands[0].Kind != KindLearning {
		t.Errorf("Kind = %q, want %q", cands[0].Kind, KindLearning)
	}
	if cands[0].Tag != "x" {
		t.Errorf("Tag = %q, want x", cands[0].Tag)
	}
}

func TestEnumerateCommits_CustomStorePath(t *testing.T) {
	f := newFixtureRepo(t)
	f.commitArtifactOnBranch("feature", ".uf/dewey/team-store/a.md", "store body\n", "store")

	cands, _, err := enumerateCommits(physicalPath(t, f.root), []string{".uf/dewey/team-store"})
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("got %d candidates; want 1", len(cands))
	}
	if cands[0].Kind != KindKnowledge {
		t.Errorf("Kind = %q, want %q", cands[0].Kind, KindKnowledge)
	}
	if cands[0].Identity != "a.md" {
		t.Errorf("Identity = %q, want a.md", cands[0].Identity)
	}
	if cands[0].Tag != "team-store" {
		t.Errorf("Tag = %q, want team-store", cands[0].Tag)
	}
}

func TestEnumerateCommits_SymlinkInCommit(t *testing.T) {
	f := newFixtureRepo(t)
	f.git("checkout", "-b", "linker")
	if err := os.MkdirAll(f.path(".uf/dewey/learnings"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(f.path("target.txt"), f.path(".uf/dewey/learnings/link.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	f.git("add", ".uf/dewey/learnings/link.md")
	f.git("commit", "-m", "add symlink")
	f.git("checkout", "main")

	before := f.capture()
	cands, warnings, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	f.assertUnchanged(before)
	if len(cands) != 0 {
		t.Fatalf("got %d candidates; want 0", len(cands))
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings; want 1: %v", len(warnings), warnings)
	}
	if want := "skipped symlink: .uf/dewey/learnings/link.md (in "; !strings.HasPrefix(warnings[0], want) {
		t.Errorf("warning = %q, want prefix %q", warnings[0], want)
	}
}

func TestEnumerateCommits_NewestSymlinkSkipsOlderBlob(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/m-20260401T101112-alice.md"
	f.git("checkout", "-b", "mixed")
	f.write(rel, "blob body\n")
	f.git("add", rel)
	f.git("commit", "-m", "blob")
	if err := os.Remove(f.path(rel)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Symlink(f.path("target.txt"), f.path(rel)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	f.git("add", rel)
	f.git("commit", "-m", "to symlink")
	f.git("checkout", "main")

	cands, warnings, err := enumerateCommits(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateCommits: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("got %d candidates; want 0 (newest touching state is a symlink)", len(cands))
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings; want 1: %v", len(warnings), warnings)
	}
	if want := "skipped symlink: " + rel + " (in "; !strings.HasPrefix(warnings[0], want) {
		t.Errorf("warning = %q, want prefix %q", warnings[0], want)
	}
}
