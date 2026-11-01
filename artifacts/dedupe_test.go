package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRepoForPlan_DefaultOriginRemotes(t *testing.T) {
	f := newFixtureRepo(t)

	repo, baseTree, err := resolveRepoForPlan(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("resolveRepoForPlan: %v", err)
	}
	if repo.PublishRemote != "origin" {
		t.Errorf("publish remote = %q, want origin", repo.PublishRemote)
	}
	if repo.PushRemote != "origin" {
		t.Errorf("push remote = %q, want origin", repo.PushRemote)
	}
	if repo.BaseBranch != "main" {
		t.Errorf("base branch = %q, want main (origin/HEAD is set by the fixture)", repo.BaseBranch)
	}
	wantSHA := strings.TrimSpace(f.git("rev-parse", "origin/main"))
	if repo.BaseSHA != wantSHA {
		t.Errorf("base sha = %q, want %q", repo.BaseSHA, wantSHA)
	}
	if !isHex40(repo.BaseSHA) {
		t.Errorf("base sha %q is not 40-hex", repo.BaseSHA)
	}
	wantRoots := []string{".uf/dewey/compiled/", ".uf/dewey/knowledge/", ".uf/dewey/learnings/"}
	if len(repo.ArtifactRoots) != len(wantRoots) {
		t.Fatalf("artifact roots = %v, want %v", repo.ArtifactRoots, wantRoots)
	}
	for i, r := range wantRoots {
		if repo.ArtifactRoots[i] != r {
			t.Errorf("artifact roots = %v, want %v", repo.ArtifactRoots, wantRoots)
			break
		}
	}
	if len(baseTree) != 0 {
		t.Errorf("base tree = %v, want empty (no artifacts committed)", baseTree)
	}
}

func TestResolveRepoForPlan_ForkLayoutResolvesViaUpstreamMain(t *testing.T) {
	f := newFixtureRepo(t)
	f.addUpstreamRemote() // upstream/HEAD deliberately unset — chain must fall through to upstream/main

	repo, _, err := resolveRepoForPlan(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("resolveRepoForPlan: %v", err)
	}
	if repo.PublishRemote != "upstream" {
		t.Errorf("publish remote = %q, want upstream", repo.PublishRemote)
	}
	if repo.PushRemote != "origin" {
		t.Errorf("push remote = %q, want origin", repo.PushRemote)
	}
	if repo.BaseBranch != "main" {
		t.Errorf("base branch = %q, want main (resolved via upstream/main, not upstream/HEAD)", repo.BaseBranch)
	}
	wantSHA := strings.TrimSpace(f.git("rev-parse", "upstream/main"))
	if repo.BaseSHA != wantSHA {
		t.Errorf("base sha = %q, want %q", repo.BaseSHA, wantSHA)
	}
}

func TestResolveRepoForPlan_UpstreamWithoutOriginFailsClosed(t *testing.T) {
	f := newFixtureRepo(t)
	f.addUpstreamRemote()
	f.git("remote", "remove", "origin")

	_, _, err := resolveRepoForPlan(physicalPath(t, f.root), nil)
	if err == nil {
		t.Fatalf("expected fail-closed error for upstream publish remote without origin push remote")
	}
	if !strings.Contains(err.Error(), "upstream") || !strings.Contains(err.Error(), "origin") {
		t.Errorf("error must name both remotes, got: %v", err)
	}
}

func TestResolveRepoForPlan_MasterFallback(t *testing.T) {
	tmp := t.TempDir()
	work := filepath.Join(tmp, "work")
	origin := filepath.Join(tmp, "origin.git")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		if _, err := run(work, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	git("init", "-b", "master")
	git("config", "user.email", "fixture@example.invalid")
	git("config", "user.name", "Fixture Tester")
	git("config", "commit.gpgsign", "false")
	if _, err := run(tmp, "init", "--bare", "-b", "master", origin); err != nil {
		t.Fatalf("git init bare origin: %v", err)
	}
	git("remote", "add", "origin", origin)
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	git("add", "README.md")
	git("commit", "-m", "root")
	git("push", "-u", "origin", "master")

	repo, _, err := resolveRepoForPlan(physicalPath(t, work), nil)
	if err != nil {
		t.Fatalf("resolveRepoForPlan: %v", err)
	}
	if repo.BaseBranch != "master" {
		t.Errorf("base branch = %q, want master (chain fallback past absent origin/HEAD and origin/main)", repo.BaseBranch)
	}
}

func TestResolveRepoForPlan_UnresolvableBaseFailsClosed(t *testing.T) {
	f := newFixtureRepo(t)
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "empty-upstream.git")
	f.git("init", "--bare", "-b", "main", empty)
	f.git("remote", "add", "upstream", empty)

	_, _, err := resolveRepoForPlan(physicalPath(t, f.root), nil)
	if err == nil {
		t.Fatalf("expected fail-closed error for unresolvable base ref")
	}
	if !strings.Contains(err.Error(), "upstream") {
		t.Errorf("error must name the remote, got: %v", err)
	}
	if !strings.Contains(err.Error(), "git fetch") {
		t.Errorf("error must direct the user to fetch, got: %v", err)
	}
}

func TestGroupCandidatesAndBuildEntry_MultiSourceProvenance(t *testing.T) {
	rel := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	sha := fixtureSHA256([]byte("shared body\n"))
	cands := []candidate{
		{Path: rel, SHA256: sha, Source: SourceWorkingTree, Identity: "auth-20260401T101112-alice", Tag: "auth", Kind: KindLearning},
		{Path: rel, SHA256: sha, Source: SourceStashPrefix + "stash@{0}", Identity: "auth-20260401T101112-alice", Tag: "auth", Kind: KindLearning},
	}
	groups := groupCandidatesByPath(cands)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	art, isConflict := buildArtifactEntry(groups[0], nil)
	if isConflict {
		t.Fatalf("same-content group must not be a conflict")
	}
	if art.Path != rel || art.SHA256 != sha || art.Identity != "auth-20260401T101112-alice" || art.Tag != "auth" || art.Kind != KindLearning {
		t.Errorf("artifact = %+v", art)
	}
	if len(art.Sources) != 2 || art.Sources[0] != "stash:stash@{0}" || art.Sources[1] != "working-tree" {
		t.Errorf("sources = %v, want [stash:stash@{0} working-tree] (sorted)", art.Sources)
	}
	if art.Status != StatusNew {
		t.Errorf("status = %q, want new (empty base tree)", art.Status)
	}
	if art.Findings == nil || len(art.Findings) != 0 {
		t.Errorf("findings = %v, want empty-not-nil", art.Findings)
	}
}

func TestBuildArtifactEntry_StatusUnchangedVsBase(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	content := "committed learning\n"
	f.write(rel, content)
	f.git("add", rel)
	f.git("commit", "-m", "add learning")
	f.git("push", "origin", "main")

	_, baseTree, err := resolveRepoForPlan(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("resolveRepoForPlan: %v", err)
	}
	if got, ok := baseTree[rel]; !ok || got != fixtureSHA256([]byte(content)) {
		t.Fatalf("base tree[%s] = %q (present=%t), want %q", rel, got, ok, fixtureSHA256([]byte(content)))
	}

	same := candidate{Path: rel, SHA256: fixtureSHA256([]byte(content)), Source: SourceWorkingTree, Identity: "auth-20260401T101112-alice", Tag: "auth", Kind: KindLearning}
	art, isConflict := buildArtifactEntry(groupCandidatesByPath([]candidate{same})[0], baseTree)
	if isConflict {
		t.Fatalf("same-content candidate vs base must not be a conflict")
	}
	if art.Status != StatusUnchanged {
		t.Errorf("status = %q, want unchanged (content already on the base branch)", art.Status)
	}

	changed := candidate{Path: rel, SHA256: fixtureSHA256([]byte("updated body\n")), Source: SourceWorkingTree, Identity: "auth-20260401T101112-alice", Tag: "auth", Kind: KindLearning}
	art2, isConflict2 := buildArtifactEntry(groupCandidatesByPath([]candidate{changed})[0], baseTree)
	if isConflict2 {
		t.Fatalf("divergent-from-base candidate is a plain new entry, never a conflict")
	}
	if art2.Status != StatusNew {
		t.Errorf("status = %q, want new (updated content republished)", art2.Status)
	}
}

func TestBuildArtifactEntry_ConflictFlag(t *testing.T) {
	rel := ".uf/dewey/compiled/authentication.md"
	cands := []candidate{
		{Path: rel, SHA256: fixtureSHA256([]byte("v1\n")), Source: SourceWorkingTree, Identity: "authentication", Tag: "authentication", Kind: KindCompiled},
		{Path: rel, SHA256: fixtureSHA256([]byte("v2\n")), Source: SourceStashPrefix + "stash@{0}", Identity: "authentication", Tag: "authentication", Kind: KindCompiled},
	}
	art, isConflict := buildArtifactEntry(groupCandidatesByPath(cands)[0], nil)
	if !isConflict {
		t.Fatalf("same path with distinct sha256 across sources must flag a conflict")
	}
	if art.Path != "" || art.SHA256 != "" || len(art.Sources) != 0 || len(art.Findings) != 0 {
		t.Errorf("conflicted group must return the zero artifact, got %+v", art)
	}
}

func TestSanctionedRoots_WithStores(t *testing.T) {
	f := newFixtureRepo(t)
	resolvedRepo := physicalPath(t, f.root)
	wr, err := validateStorePath(f.root, resolvedRepo, ".uf/dewey/team-store")
	if err != nil {
		t.Fatalf("validateStorePath: %v", err)
	}
	repo, _, err := resolveRepoForPlan(resolvedRepo, []walkRoot{wr})
	if err != nil {
		t.Fatalf("resolveRepoForPlan: %v", err)
	}
	want := []string{".uf/dewey/compiled/", ".uf/dewey/knowledge/", ".uf/dewey/learnings/", ".uf/dewey/team-store/"}
	if len(repo.ArtifactRoots) != len(want) {
		t.Fatalf("artifact roots = %v, want %v", repo.ArtifactRoots, want)
	}
	for i, r := range want {
		if repo.ArtifactRoots[i] != r {
			t.Errorf("artifact roots = %v, want %v", repo.ArtifactRoots, want)
			break
		}
	}
}
