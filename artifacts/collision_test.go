package artifacts

import "testing"

// conflictCand builds a minimal learning candidate for conflict-entry tests.
func conflictCand(path, sha, source string) candidate {
	return candidate{
		Path:     path,
		SHA256:   sha,
		Source:   source,
		Identity: "auth-20260401T101112-alice",
		Tag:      "auth",
		Kind:     KindLearning,
	}
}

func TestBuildConflictEntry_TwoVariantsFromTwoSources(t *testing.T) {
	p := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	group := pathGroup{path: p, cands: []candidate{
		conflictCand(p, "bbbb1111bbbb1111bbbb1111bbbb1111bbbb1111", SourceWorkingTree),
		conflictCand(p, "aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222", SourceStashPrefix+"stash@{0}"),
	}}

	conflict, ok := buildConflictEntry(group)
	if !ok {
		t.Fatalf("buildConflictEntry(two distinct sha256) = ok false, want true")
	}
	if conflict.Path != p {
		t.Errorf("Path = %q, want %q", conflict.Path, p)
	}
	if conflict.Identity != "auth-20260401T101112-alice" {
		t.Errorf("Identity = %q, want auth-20260401T101112-alice", conflict.Identity)
	}
	if len(conflict.Variants) != 2 {
		t.Fatalf("len(Variants) = %d, want 2", len(conflict.Variants))
	}
	// Variants are sorted by sha256: the aaaa... variant comes first.
	if conflict.Variants[0].SHA256 != "aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222" {
		t.Errorf("Variants[0].SHA256 = %q, want the aaaa... digest", conflict.Variants[0].SHA256)
	}
	if got := conflict.Variants[0].Sources; len(got) != 1 || got[0] != "stash:stash@{0}" {
		t.Errorf("Variants[0].Sources = %v, want [stash:stash@{0}]", got)
	}
	if got := conflict.Variants[1].Sources; len(got) != 1 || got[0] != SourceWorkingTree {
		t.Errorf("Variants[1].Sources = %v, want [working-tree]", got)
	}
	for i, v := range conflict.Variants {
		if v.Findings == nil {
			t.Errorf("Variants[%d].Findings = nil, want empty non-nil (screening fills later)", i)
		}
	}
}

func TestBuildConflictEntry_SameShaNotConflict(t *testing.T) {
	p := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	group := pathGroup{path: p, cands: []candidate{
		conflictCand(p, "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", SourceWorkingTree),
		conflictCand(p, "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", SourceStashPrefix+"stash@{0}"),
	}}

	if _, ok := buildConflictEntry(group); ok {
		t.Fatalf("buildConflictEntry(same sha256 across sources) = ok true, want false (plain dedup case)")
	}
}

func TestBuildConflictEntry_ThreeVariantsAndSourceMerging(t *testing.T) {
	p := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	group := pathGroup{path: p, cands: []candidate{
		conflictCand(p, "cccc3333cccc3333cccc3333cccc3333cccc3333", SourceWorkingTree),
		conflictCand(p, "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", SourceStashPrefix+"stash@{0}"),
		conflictCand(p, "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", SourceStashPrefix+"stash@{1}"),
		conflictCand(p, "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222", SourceCommitPrefix+"0123456789abcdef0123456789abcdef01234567"),
	}}

	conflict, ok := buildConflictEntry(group)
	if !ok {
		t.Fatalf("buildConflictEntry(three distinct sha256) = ok false, want true")
	}
	if len(conflict.Variants) != 3 {
		t.Fatalf("len(Variants) = %d, want 3", len(conflict.Variants))
	}
	// Sorted by sha256: aaaa, bbbb, cccc.
	wantSHA := []string{
		"aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
		"cccc3333cccc3333cccc3333cccc3333cccc3333",
	}
	for i, want := range wantSHA {
		if conflict.Variants[i].SHA256 != want {
			t.Errorf("Variants[%d].SHA256 = %q, want %q", i, conflict.Variants[i].SHA256, want)
		}
	}
	// The aaaa variant carries both stash labels, sorted.
	got := conflict.Variants[0].Sources
	wantSources := []string{"stash:stash@{0}", "stash:stash@{1}"}
	if len(got) != 2 || got[0] != wantSources[0] || got[1] != wantSources[1] {
		t.Errorf("Variants[0].Sources = %v, want %v", got, wantSources)
	}
	// The bbbb variant carries the commit source.
	if got := conflict.Variants[1].Sources; len(got) != 1 || got[0] != "commit:0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("Variants[1].Sources = %v, want [commit:0123456789abcdef0123456789abcdef01234567]", got)
	}
}

func TestBuildConflictEntry_EmptyAndSingleNotConflict(t *testing.T) {
	if _, ok := buildConflictEntry(pathGroup{path: "x"}); ok {
		t.Errorf("buildConflictEntry(empty group) = ok true, want false")
	}
	p := ".uf/dewey/learnings/auth-20260401T101112-alice.md"
	if _, ok := buildConflictEntry(pathGroup{path: p, cands: []candidate{conflictCand(p, "aaaa", SourceWorkingTree)}}); ok {
		t.Errorf("buildConflictEntry(single candidate) = ok true, want false")
	}
}
