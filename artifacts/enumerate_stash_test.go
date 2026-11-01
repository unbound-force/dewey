package artifacts

import (
	"os"
	"strings"
	"testing"
)

func TestEnumerateStashes_TrackedAndUntrackedParts(t *testing.T) {
	f := newFixtureRepo(t)
	auth := "learning content"
	comp := "compiled content"
	f.write(".uf/dewey/learnings/auth-20260401T101112-alice.md", auth)
	f.write(".uf/dewey/compiled/authentication.md", comp)
	f.git("add", ".uf/dewey/learnings/auth-20260401T101112-alice.md")
	f.git("stash", "push", "-u", "--",
		".uf/dewey/learnings/auth-20260401T101112-alice.md",
		".uf/dewey/compiled/authentication.md")

	cands, stashRefs, warnings, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(cands), cands)
	}

	learning := cands[1]
	if learning.Path != ".uf/dewey/learnings/auth-20260401T101112-alice.md" {
		t.Errorf("learning path = %q", learning.Path)
	}
	if learning.Source != "stash:stash@{0}" {
		t.Errorf("learning source = %q, want stash:stash@{0}", learning.Source)
	}
	if learning.Identity != "auth-20260401T101112-alice" || learning.Tag != "auth" || learning.Kind != KindLearning {
		t.Errorf("learning identity/tag/kind = %q/%q/%q", learning.Identity, learning.Tag, learning.Kind)
	}
	if learning.SHA256 != fixtureSHA256([]byte(auth)) {
		t.Errorf("learning sha256 = %q", learning.SHA256)
	}
	if string(learning.Content) != auth {
		t.Errorf("learning content = %q", learning.Content)
	}

	compiled := cands[0]
	if compiled.Path != ".uf/dewey/compiled/authentication.md" {
		t.Errorf("compiled path = %q", compiled.Path)
	}
	if compiled.Source != "stash-untracked:stash@{0}" {
		t.Errorf("compiled source = %q, want stash-untracked:stash@{0}", compiled.Source)
	}
	if compiled.Identity != "authentication" || compiled.Tag != "authentication" || compiled.Kind != KindCompiled {
		t.Errorf("compiled identity/tag/kind = %q/%q/%q", compiled.Identity, compiled.Tag, compiled.Kind)
	}
	if compiled.SHA256 != fixtureSHA256([]byte(comp)) {
		t.Errorf("compiled sha256 = %q", compiled.SHA256)
	}

	if len(stashRefs) != 2 {
		t.Fatalf("stash_refs = %v, want stash@{0} and stash@{0}^3", stashRefs)
	}
	for _, key := range []string{"stash@{0}", "stash@{0}^3"} {
		sha, ok := stashRefs[key]
		if !ok {
			t.Errorf("stash_refs missing %q", key)
			continue
		}
		if !isHex40(sha) {
			t.Errorf("stash_refs[%q] = %q, not 40-hex", key, sha)
		}
	}
}

func TestEnumerateStashes_NoStashes(t *testing.T) {
	f := newFixtureRepo(t)
	cands, stashRefs, warnings, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("candidates = %d, want 0", len(cands))
	}
	if len(stashRefs) != 0 {
		t.Errorf("stash_refs = %v, want empty", stashRefs)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

func TestEnumerateStashes_TrioAtAnyDepth(t *testing.T) {
	f := newFixtureRepo(t)
	f.stashTrackedArtifact("sub/.uf/dewey/learnings/x-tag-20260401T101112-alice.md", "nested")

	cands, _, _, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidates = %d, want 1: %+v", len(cands), cands)
	}
	c := cands[0]
	if c.Path != "sub/.uf/dewey/learnings/x-tag-20260401T101112-alice.md" {
		t.Errorf("path = %q", c.Path)
	}
	if c.Kind != KindLearning || c.Tag != "x" || c.Identity != "x-tag-20260401T101112-alice" {
		t.Errorf("kind/tag/identity = %q/%q/%q", c.Kind, c.Tag, c.Identity)
	}
	if c.Source != "stash:stash@{0}" {
		t.Errorf("source = %q", c.Source)
	}
}

func TestEnumerateStashes_SymlinkInStash(t *testing.T) {
	f := newFixtureRepo(t)
	f.write("target.txt", "target content")
	if err := os.MkdirAll(f.path(".uf/dewey/learnings"), 0o755); err != nil {
		t.Fatalf("mkdir learnings: %v", err)
	}
	if err := os.Symlink(f.path("target.txt"), f.path(".uf/dewey/learnings/link.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	f.git("add", ".uf/dewey/learnings/link.md")
	f.git("stash", "push", "--", ".uf/dewey/learnings/link.md")

	cands, _, warnings, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("candidates = %d, want 0 (symlink never read)", len(cands))
	}
	want := "skipped symlink: .uf/dewey/learnings/link.md (in stash@{0})"
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %v, want [%q]", warnings, want)
	}
}

func TestEnumerateStashes_DotPrefixedAndUnsafeBasenames(t *testing.T) {
	f := newFixtureRepo(t)
	f.stashTrackedArtifact(".uf/dewey/learnings/.hidden.md", "hidden")
	f.stashTrackedArtifact(".uf/dewey/learnings/we:ird.md", "weird")

	cands, _, warnings, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("candidates = %d, want 0", len(cands))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one (unsafe basename; dot-prefixed silent)", warnings)
	}
	if !strings.Contains(warnings[0], "skipped unsafe basename: .uf/dewey/learnings/we:ird.md") {
		t.Errorf("warning = %q, want unsafe-basename warning for we:ird.md", warnings[0])
	}
}

func TestEnumerateStashes_ZeroMutation(t *testing.T) {
	f := newFixtureRepo(t)
	f.stashTrackedArtifact(".uf/dewey/learnings/auth-20260401T101112-alice.md", "content")
	before := f.capture()

	if _, _, _, err := enumerateStashes(physicalPath(t, f.root), nil); err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	f.assertUnchanged(before)
}

func TestEnumerateStashes_CustomStorePath(t *testing.T) {
	f := newFixtureRepo(t)
	f.stashTrackedArtifact(".uf/dewey/team-store/a.md", "store content")

	cands, _, _, err := enumerateStashes(physicalPath(t, f.root), []string{".uf/dewey/team-store"})
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidates = %d, want 1: %+v", len(cands), cands)
	}
	c := cands[0]
	if c.Path != ".uf/dewey/team-store/a.md" {
		t.Errorf("path = %q", c.Path)
	}
	if c.Kind != KindKnowledge || c.Identity != "a.md" || c.Tag != "team-store" {
		t.Errorf("kind/identity/tag = %q/%q/%q", c.Kind, c.Identity, c.Tag)
	}
}

func TestEnumerateStashes_MultipleStashes(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/multi-20260401T101112-alice.md"
	f.write(rel, "version one")
	f.git("add", rel)
	f.git("stash", "push", "--", rel)
	f.write(rel, "version two")
	f.git("add", rel)
	f.git("stash", "push", "--", rel)

	cands, stashRefs, _, err := enumerateStashes(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("enumerateStashes: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(cands), cands)
	}
	// stash@{0} is the NEWEST stash (version two); sorted by Path then Source.
	if cands[0].Path != rel || cands[1].Path != rel {
		t.Fatalf("paths = %q, %q, want %q twice", cands[0].Path, cands[1].Path, rel)
	}
	if cands[0].Source != "stash:stash@{0}" || cands[0].SHA256 != fixtureSHA256([]byte("version two")) {
		t.Errorf("newest candidate = %q/%q, want stash:stash@{0}/sha(version two)", cands[0].Source, cands[0].SHA256)
	}
	if cands[1].Source != "stash:stash@{1}" || cands[1].SHA256 != fixtureSHA256([]byte("version one")) {
		t.Errorf("older candidate = %q/%q, want stash:stash@{1}/sha(version one)", cands[1].Source, cands[1].SHA256)
	}
	for _, label := range []string{"stash@{0}", "stash@{1}"} {
		sha, ok := stashRefs[label]
		if !ok || !isHex40(sha) {
			t.Errorf("stash_refs[%q] = %q, ok=%v", label, sha, ok)
		}
	}
	if _, hasUntracked := stashRefs["stash@{0}^3"]; hasUntracked {
		t.Errorf("stash_refs has unexpected ^3 entry (no -u stash)")
	}
}
