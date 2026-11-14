package gitref

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// blobSHAEmpty is git's object ID for the empty blob (a well-known constant,
// assertable without invoking git).
const blobSHAEmpty = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"

// blobSHAOther is a second syntactically valid 40-hex object ID.
const blobSHAOther = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

// resolvedAt builds a successfully resolved RefResolution for fullRef.
func resolvedAt(fullRef string) RefResolution {
	return RefResolution{
		Ref:     "origin/main",
		FullRef: fullRef,
		SHA:     testSHA,
		Status:  StatusResolved,
		Leg:     LegDefault,
	}
}

// TestMapDefaultBranchBlobs_BatchRecords covers IP-FR-001's "Batch ref
// resolution prefers ls-tree" scenario: one invocation maps every blob entry
// for the requested pathspecs, skipping non-blob records, with the argv shape
// (argv array, fully qualified ref, `--` before pathspecs) pinned exactly.
func TestMapDefaultBranchBlobs_BatchRecords(t *testing.T) {
	out := "100644 blob " + testSHA + "\tcompiled/auth.md\x00" +
		"100644 blob " + blobSHAOther + "\tknowledge/team/item.md\x00" +
		"040000 tree " + testSHA + "\tcompiled\x00" +
		"160000 commit " + testSHA + "\tvendor/sub\x00"
	runner := &fakeRunner{responses: []fakeResponse{{stdout: out}}}

	got := MapDefaultBranchBlobs(context.Background(), runner, "/repo",
		resolvedAt("refs/remotes/origin/main"), []string{"compiled", "knowledge"})

	want := map[string]string{
		"compiled/auth.md":       testSHA,
		"knowledge/team/item.md": blobSHAOther,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MapDefaultBranchBlobs() = %v, want %v", got, want)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected exactly one git invocation, got %d: %v", len(runner.calls), runner.calls)
	}
	wantArgs := []string{"ls-tree", "-r", "-z", "refs/remotes/origin/main", "--", "compiled", "knowledge"}
	if !reflect.DeepEqual(runner.calls[0], wantArgs) {
		t.Errorf("argv = %v, want %v", runner.calls[0], wantArgs)
	}
	if runner.dirs[0] != "/repo" {
		t.Errorf("working dir = %q, want /repo", runner.dirs[0])
	}
}

// TestMapDefaultBranchBlobs_GuardsSkipGit covers the fail-closed guards: an
// unresolvable resolution, an empty full ref, or no pathspecs must return an
// empty map without invoking git at all (a fakeRunner with no scripted
// responses fails the test if called).
func TestMapDefaultBranchBlobs_GuardsSkipGit(t *testing.T) {
	cases := []struct {
		name  string
		res   RefResolution
		paths []string
	}{
		{"no paths", resolvedAt("refs/remotes/origin/main"), nil},
		{"unresolved missing ref", RefResolution{Ref: "origin/main", FullRef: "refs/remotes/origin/main", Status: StatusMissingRef}, []string{"compiled"}},
		{"unresolved not a repo", RefResolution{Ref: "origin/main", Status: StatusNotARepo}, []string{"compiled"}},
		{"unresolved invalid config", RefResolution{Ref: "--upload-pack=evil", Status: StatusInvalidConfig}, []string{"compiled"}},
		{"resolved but empty full ref", RefResolution{Ref: "origin/main", Status: StatusResolved}, []string{"compiled"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{}
			got := MapDefaultBranchBlobs(context.Background(), runner, "/repo", tt.res, tt.paths)
			if len(got) != 0 {
				t.Errorf("MapDefaultBranchBlobs() = %v, want empty map", got)
			}
			if len(runner.calls) != 0 {
				t.Errorf("expected no git invocation, got %v", runner.calls)
			}
		})
	}
}

// TestMapDefaultBranchBlobs_FailuresReturnEmptyMap covers the GTS-FR-004
// contract: every failure (non-zero exit, git could not run, malformed or
// truncated output) returns an empty map — grant withheld, never an error.
func TestMapDefaultBranchBlobs_FailuresReturnEmptyMap(t *testing.T) {
	res := resolvedAt("refs/remotes/origin/main")
	cases := []struct {
		name string
		run  Runner
	}{
		{"non-zero exit", &fakeRunner{responses: []fakeResponse{{stdout: "", stderr: "fatal: bad object", code: 128}}}},
		{"nil runner", nil},
		{"run error (timeout shape)", &fakeRunner{responses: []fakeResponse{{err: context.DeadlineExceeded}}}},
		{"malformed record without tab", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob " + testSHA + " compiled/auth.md\x00"}}}},
		{"malformed metadata fields", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob\x00compiled/auth.md\x00"}}}},
		{"malformed object id", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob nothexsha\tcompiled/auth.md\x00"}}}},
		{"truncated record", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob " + testSHA + "\tcompiled/auth.md\x00100644 blob"}}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := MapDefaultBranchBlobs(context.Background(), tt.run, "/repo", res, []string{"compiled"})
			if len(got) != 0 {
				t.Errorf("MapDefaultBranchBlobs() = %v, want empty map", got)
			}
		})
	}
}

// TestMapDefaultBranchBlobsResult_BatchFailureSignal covers the distinction
// IP-FR-001's per-page fallback depends on: a failed batch invocation
// (non-zero exit, git could not run, malformed output) reports BatchFailed
// true, while a successful invocation over a tree that simply lacks the
// requested paths — and every guard that skips git — reports an authoritative
// empty map with BatchFailed false (withholding is correct there: no possible
// equality).
func TestMapDefaultBranchBlobsResult_BatchFailureSignal(t *testing.T) {
	res := resolvedAt("refs/remotes/origin/main")

	got := MapDefaultBranchBlobsResult(context.Background(),
		&fakeRunner{responses: []fakeResponse{{stdout: ""}}}, "/repo", res, []string{"compiled"})
	if got.BatchFailed {
		t.Error("BatchFailed = true for a successful invocation over a tree lacking the paths, want false")
	}
	if len(got.Blobs) != 0 {
		t.Errorf("Blobs = %v, want empty map", got.Blobs)
	}

	failures := []struct {
		name string
		run  Runner
	}{
		{"non-zero exit", &fakeRunner{responses: []fakeResponse{{stderr: "fatal: bad object", code: 128}}}},
		{"run error (timeout shape)", &fakeRunner{responses: []fakeResponse{{err: context.DeadlineExceeded}}}},
		{"malformed record without tab", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob " + testSHA + " compiled/auth.md\x00"}}}},
		{"malformed metadata fields", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob\x00compiled/auth.md\x00"}}}},
		{"malformed object id", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob nothexsha\tcompiled/auth.md\x00"}}}},
		{"truncated record", &fakeRunner{responses: []fakeResponse{{stdout: "100644 blob " + testSHA + "\tcompiled/auth.md\x00100644 blob"}}}},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			got := MapDefaultBranchBlobsResult(context.Background(), tt.run, "/repo", res, []string{"compiled"})
			if !got.BatchFailed {
				t.Error("BatchFailed = false for a failed batch invocation, want true")
			}
			if len(got.Blobs) != 0 {
				t.Errorf("Blobs = %v, want empty map alongside BatchFailed", got.Blobs)
			}
		})
	}

	guards := []struct {
		name  string
		run   Runner
		res   RefResolution
		paths []string
	}{
		{"nil runner", nil, res, []string{"compiled"}},
		{"no paths", &fakeRunner{}, res, nil},
		{"unresolved ref", &fakeRunner{}, RefResolution{Ref: "origin/main", Status: StatusMissingRef}, []string{"compiled"}},
	}
	for _, tt := range guards {
		t.Run(tt.name, func(t *testing.T) {
			got := MapDefaultBranchBlobsResult(context.Background(), tt.run, "/repo", tt.res, tt.paths)
			if got.BatchFailed {
				t.Error("BatchFailed = true for a guard that skipped git, want false (authoritative empty map)")
			}
			if len(got.Blobs) != 0 {
				t.Errorf("Blobs = %v, want empty map", got.Blobs)
			}
		})
	}
}

// TestFileBlobSHA_MatchesGitHashObject pins FileBlobSHA against the real
// `git hash-object` on a temp file, plus the empty-blob constant and the
// read-error path (TC-011: guarded for missing git / short mode).
func TestFileBlobSHA_MatchesGitHashObject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.md")
	content := "---\ntier: validated\n---\n\nbody with unicode: café\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := FileBlobSHA(file)
	if err != nil {
		t.Fatalf("FileBlobSHA: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if _, gitErr := lookGit(); gitErr == nil {
		want := strings.TrimSpace(gitOut(t, dir, "hash-object", file))
		if got != want {
			t.Errorf("FileBlobSHA = %q, want git hash-object %q", got, want)
		}
	}

	// The empty blob's object ID is a well-known constant, assertable
	// without git: SHA-1 over "blob 0\x00".
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	emptySHA, err := FileBlobSHA(empty)
	if err != nil {
		t.Fatalf("FileBlobSHA(empty): %v", err)
	}
	if emptySHA != blobSHAEmpty {
		t.Errorf("FileBlobSHA(empty) = %q, want %q", emptySHA, blobSHAEmpty)
	}

	// A missing file is an error; callers treat it as an absent blob SHA.
	if _, err := FileBlobSHA(filepath.Join(dir, "missing.md")); err == nil {
		t.Error("FileBlobSHA(missing) expected an error, got nil")
	}
}

// lookGit reports whether the git binary is available.
func lookGit() (string, error) {
	return exec.LookPath("git")
}

// TestNewPageBlobResolver_EndToEndFixture covers the full per-index-run
// derivation against a real fixture repository (design D2, IP-FR-001): one
// Resolve plus one batch ls-tree, then per-page lookups — blob equality on
// the committed file, divergence after a working-tree edit, and withholding
// for a path absent from the default-branch tree.
func TestNewPageBlobResolver_EndToEndFixture(t *testing.T) {
	dir, _ := gitrefRepo(t, true)

	resolver := NewPageBlobResolver(context.Background(),
		Options{VaultDir: dir, Env: mapEnv(nil)}, TrustPathspecs())

	fileSHA, refSHA := resolver.PageBlobs("compiled/auth.md")
	if fileSHA == "" || refSHA == "" {
		t.Fatalf("PageBlobs(compiled/auth.md) = (%q, %q), want non-empty SHAs", fileSHA, refSHA)
	}
	if fileSHA != refSHA {
		t.Errorf("committed file: file SHA %q != default-branch SHA %q", fileSHA, refSHA)
	}

	// A working-tree edit changes the file blob but not the default-branch
	// blob: the grant is withheld (GTS-FR-002 working-tree edits cannot
	// self-promote).
	page := filepath.Join(dir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n# Auth\n\nedited\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fileSHA2, refSHA2 := resolver.PageBlobs("compiled/auth.md")
	if fileSHA2 == "" || refSHA2 == "" {
		t.Fatalf("PageBlobs after edit = (%q, %q), want non-empty SHAs", fileSHA2, refSHA2)
	}
	if fileSHA2 == fileSHA {
		t.Errorf("edited file SHA = unchanged %q; the working-tree hash must move", fileSHA2)
	}
	if refSHA2 != refSHA {
		t.Errorf("default-branch SHA moved from %q to %q; the map is a per-run snapshot", refSHA, refSHA2)
	}
	if fileSHA2 == refSHA2 {
		t.Error("edited file blob equals the default-branch blob; want divergence")
	}

	// A path with no default-branch blob withholds both SHAs — the file
	// hash is not even computed (untracked file, no possible equality).
	if f, r := resolver.PageBlobs("compiled/untracked.md"); f != "" || r != "" {
		t.Errorf("PageBlobs(untracked) = (%q, %q), want empty SHAs", f, r)
	}
}

// gitRun runs one mutating git command in dir, failing the test on error.
// Like gitrefRepo's runner it carries a deterministic identity via -c flags
// so commits work on machines without a global git identity (CI runners).
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestNewPageBlobResolver_CommittedDivergenceWithholdsEquality covers the two
// committed-divergence shapes of task 2.3 against real fixture repositories:
// a page committed on a NON-default branch (PW-FR-003's branch-only promotion
// staying draft, at the resolver level) and a regenerated page committed on
// the local default branch while the fetched origin/main still holds the
// merged original (#204). Both leave a clean working tree holding blob B
// against a default-branch map holding blob A, so the resolved SHAs must
// diverge and withhold the validated grant — the file side is answered by the
// per-page FileBlobSHA fallback, pinned here by exact object ID.
func TestNewPageBlobResolver_CommittedDivergenceWithholdsEquality(t *testing.T) {
	const v1 = "---\ntier: validated\n---\n# Auth\n"
	const v2 = "---\ntier: validated\n---\n# Auth\n\nregenerated body\n"
	cases := []struct {
		name   string
		branch string // empty: commit on the current (default) branch
	}{
		{"branch-only commit diverges from default branch", "feature/compiled"},
		{"regenerated commit ahead of fetched ref (#204)", ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := gitrefRepo(t, true)
			if tt.branch != "" {
				gitRun(t, dir, "checkout", "-q", "-b", tt.branch)
			}
			page := filepath.Join(dir, "compiled", "auth.md")
			if err := os.WriteFile(page, []byte(v2), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			gitRun(t, dir, "add", ".")
			gitRun(t, dir, "commit", "-q", "-m", "divergent page")

			resolver := NewPageBlobResolver(context.Background(),
				Options{VaultDir: dir, Env: mapEnv(nil)}, TrustPathspecs())

			fileSHA, refSHA := resolver.PageBlobs("compiled/auth.md")
			if fileSHA == "" || refSHA == "" {
				t.Fatalf("PageBlobs(compiled/auth.md) = (%q, %q), want non-empty SHAs", fileSHA, refSHA)
			}
			if want := BlobSHA([]byte(v2)); fileSHA != want {
				t.Errorf("file SHA = %q, want the committed blob B %q", fileSHA, want)
			}
			if want := BlobSHA([]byte(v1)); refSHA != want {
				t.Errorf("default-branch SHA = %q, want the fetched blob A %q", refSHA, want)
			}
			if fileSHA == refSHA {
				t.Error("committed divergence: file blob equals default-branch blob; want blob B != blob A")
			}
		})
	}
}

// TestNewPageBlobResolver_MissingRefWithholdsLookups covers the GTS-FR-004
// fallback end to end against a real fixture repository whose
// refs/remotes/origin/main was never created (a fresh clone before any
// fetch): the resolution reports missing-ref, the batch ls-tree is skipped
// (the guard in MapDefaultBranchBlobs), and every per-page lookup withholds
// both SHAs — the grant is withheld without an error and without hashing the
// working tree.
func TestNewPageBlobResolver_MissingRefWithholdsLookups(t *testing.T) {
	dir, _ := gitrefRepo(t, false)

	// Precondition: the fixture really is the missing-ref shape, so the
	// empty lookups below pin the fallback, not an unrelated failure.
	res := New(Options{VaultDir: dir, Env: mapEnv(nil)}).Resolve(context.Background())
	if res.Status != StatusMissingRef {
		t.Fatalf("precondition: Status = %q, want %q (reason: %q)", res.Status, StatusMissingRef, res.Reason)
	}

	resolver := NewPageBlobResolver(context.Background(),
		Options{VaultDir: dir, Env: mapEnv(nil)}, TrustPathspecs())

	if f, r := resolver.PageBlobs("compiled/auth.md"); f != "" || r != "" {
		t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want empty SHAs on missing ref", f, r)
	}
	if f, r := resolver.PageBlobs("compiled/untracked.md"); f != "" || r != "" {
		t.Errorf("PageBlobs(compiled/untracked.md) = (%q, %q), want empty SHAs on missing ref", f, r)
	}
}

// TestBlobResolver_PerPageFallbackRecoversAfterBatchFailure covers the
// IP-FR-001 per-page fallback at the resolver level: when armed (batch
// invocation failed, ref resolved), a map miss runs ONE single-path ls-tree —
// argv, working dir, and invocation count pinned exactly — and the equality
// grant becomes possible again. Repeat lookups are memoized: still exactly
// one invocation per path.
func TestBlobResolver_PerPageFallbackRecoversAfterBatchFailure(t *testing.T) {
	dir := t.TempDir()
	content := []byte("---\ntier: validated\n---\n# Auth\n")
	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compiled", "auth.md"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runner := &fakeRunner{responses: []fakeResponse{
		{stdout: "100644 blob " + BlobSHA(content) + "\tcompiled/auth.md\x00"},
	}}
	res := newFallbackBlobResolver(dir, map[string]string{}, runner, "refs/remotes/origin/main")

	want := BlobSHA(content)
	fileSHA, refSHA := res.PageBlobs("compiled/auth.md")
	if fileSHA != want || refSHA != want {
		t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want recovered equality grant on %q", fileSHA, refSHA, want)
	}

	// The memo serves repeat lookups: still exactly one single-path invocation.
	if f2, r2 := res.PageBlobs("compiled/auth.md"); f2 != want || r2 != want {
		t.Errorf("repeat PageBlobs = (%q, %q), want memoized (%q, %q)", f2, r2, want, want)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected exactly one git invocation, got %d: %v", len(runner.calls), runner.calls)
	}
	wantArgs := []string{"ls-tree", "-z", "refs/remotes/origin/main", "--", "compiled/auth.md"}
	if !reflect.DeepEqual(runner.calls[0], wantArgs) {
		t.Errorf("argv = %v, want %v", runner.calls[0], wantArgs)
	}
	if runner.dirs[0] != dir {
		t.Errorf("working dir = %q, want %q", runner.dirs[0], dir)
	}
}

// TestBlobResolver_PerPageFallbackWithholdsOnFailureOrAbsence covers the
// fallback's own withholding: a failed single-path invocation, malformed
// output, or a path absent from the default-branch tree returns two empty
// strings — and the working-tree file exists here, so a wrongly-returned file
// SHA would be caught. The file side is never hashed without a ref-side SHA.
func TestBlobResolver_PerPageFallbackWithholdsOnFailureOrAbsence(t *testing.T) {
	cases := []struct {
		name string
		resp fakeResponse
	}{
		{"single-path also fails", fakeResponse{stderr: "fatal: bad object", code: 128}},
		{"single-path run error", fakeResponse{err: context.DeadlineExceeded}},
		{"path absent from tree", fakeResponse{stdout: ""}},
		{"malformed single-path output", fakeResponse{stdout: "100644 blob " + testSHA + " no-tab\x00"}},
		{"record for a different path", fakeResponse{stdout: "100644 blob " + testSHA + "\tknowledge/other.md\x00"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "compiled", "auth.md"), []byte("any working-tree content\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			res := newFallbackBlobResolver(dir, map[string]string{},
				&fakeRunner{responses: []fakeResponse{tt.resp}}, "refs/remotes/origin/main")
			if f, r := res.PageBlobs("compiled/auth.md"); f != "" || r != "" {
				t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want empty SHAs (withheld)", f, r)
			}
		})
	}
}

// TestBlobResolver_PerPageFallbackConcurrentLookups pins the concurrency
// contract under -race: concurrent PageBlobs calls on an armed resolver stay
// safe and still run exactly one single-path invocation per distinct path
// (the fallback lock serializes runner access, so even the non-thread-safe
// test fake is safe here).
func TestBlobResolver_PerPageFallbackConcurrentLookups(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contentA := []byte("# A\n")
	contentB := []byte("# B\n")
	if err := os.WriteFile(filepath.Join(dir, "compiled", "a.md"), contentA, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compiled", "b.md"), contentB, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Both scripted responses carry both records, so either pop order
	// answers either path.
	both := "100644 blob " + BlobSHA(contentA) + "\tcompiled/a.md\x00" +
		"100644 blob " + BlobSHA(contentB) + "\tcompiled/b.md\x00"
	runner := &fakeRunner{responses: []fakeResponse{{stdout: both}, {stdout: both}}}
	res := newFallbackBlobResolver(dir, map[string]string{}, runner, "refs/remotes/origin/main")

	wantA, wantB := BlobSHA(contentA), BlobSHA(contentB)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if f, r := res.PageBlobs("compiled/a.md"); f != wantA || r != wantA {
				t.Errorf("PageBlobs(a.md) = (%q, %q), want (%q, %q)", f, r, wantA, wantA)
			}
		}()
		go func() {
			defer wg.Done()
			if f, r := res.PageBlobs("compiled/b.md"); f != wantB || r != wantB {
				t.Errorf("PageBlobs(b.md) = (%q, %q), want (%q, %q)", f, r, wantB, wantB)
			}
		}()
	}
	wg.Wait()
	if len(runner.calls) != 2 {
		t.Fatalf("expected exactly two git invocations (one per distinct path), got %d: %v", len(runner.calls), runner.calls)
	}
}

// TestBlobResolver_PerPageFallbackRealGit pins the fallback's single-path
// invocation against real git (TC-011: gitrefRepo skips in short mode and
// without git): the non-recursive `ls-tree -z <ref> -- <path>` argv answers
// the committed blob for the fixture's fetched origin/main, and an untracked
// path withholds both SHAs.
func TestBlobResolver_PerPageFallbackRealGit(t *testing.T) {
	dir, _ := gitrefRepo(t, true)

	res := newFallbackBlobResolver(dir, map[string]string{}, ExecRunner{}, "refs/remotes/origin/main")

	want := BlobSHA([]byte("---\ntier: validated\n---\n# Auth\n"))
	if fileSHA, refSHA := res.PageBlobs("compiled/auth.md"); fileSHA != want || refSHA != want {
		t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want equality grant on %q", fileSHA, refSHA, want)
	}
	if f, r := res.PageBlobs("compiled/untracked.md"); f != "" || r != "" {
		t.Errorf("PageBlobs(compiled/untracked.md) = (%q, %q), want empty SHAs", f, r)
	}
}

// resolveThenBatchScript scripts NewPageBlobResolver's construction path
// through a fake runner with the default origin/main leg: the git-config leg
// unset, the repository probe OK, the ref verify resolved, then the given
// batch ls-tree outcome.
func resolveThenBatchScript(batch fakeResponse) *fakeRunner {
	return &fakeRunner{responses: []fakeResponse{
		{code: 1},                // git config --get dewey.defaultBranchRef: unset
		{stdout: ".git\n"},       // rev-parse --git-dir: inside a repository
		{stdout: testSHA + "\n"}, // rev-parse --verify: refs/remotes/origin/main resolved
		batch,                    // ls-tree -r -z: the batch map outcome
	}}
}

// TestNewPageBlobResolver_BatchFailureArmsPerPageFallback covers the
// construction-path wiring of IP-FR-001's per-page fallback: the precedence
// chain resolves origin/main, the batch ls-tree fails, and the armed resolver
// recovers the ref-side SHA for a map miss via one single-path ls-tree —
// making the equality grant possible again.
func TestNewPageBlobResolver_BatchFailureArmsPerPageFallback(t *testing.T) {
	dir := t.TempDir()
	content := []byte("---\ntier: validated\n---\n# Auth\n")
	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compiled", "auth.md"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runner := resolveThenBatchScript(fakeResponse{stderr: "fatal: bad object", code: 128})
	runner.responses = append(runner.responses,
		fakeResponse{stdout: "100644 blob " + BlobSHA(content) + "\tcompiled/auth.md\x00"})

	res := NewPageBlobResolver(context.Background(),
		Options{VaultDir: dir, Runner: runner, Env: mapEnv(nil)}, TrustPathspecs())

	want := BlobSHA(content)
	fileSHA, refSHA := res.PageBlobs("compiled/auth.md")
	if fileSHA != want || refSHA != want {
		t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want recovered equality grant on %q", fileSHA, refSHA, want)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("expected 5 git invocations (3 resolve + 1 batch + 1 single-path), got %d: %v",
			len(runner.calls), runner.calls)
	}
	wantBatch := []string{"ls-tree", "-r", "-z", "refs/remotes/origin/main", "--", "compiled", "knowledge"}
	if !reflect.DeepEqual(runner.calls[3], wantBatch) {
		t.Errorf("batch argv = %v, want %v", runner.calls[3], wantBatch)
	}
	wantSingle := []string{"ls-tree", "-z", "refs/remotes/origin/main", "--", "compiled/auth.md"}
	if !reflect.DeepEqual(runner.calls[4], wantSingle) {
		t.Errorf("single-path argv = %v, want %v", runner.calls[4], wantSingle)
	}
}

// TestNewPageBlobResolver_BatchHealthyNeverInvokesPerPage covers the other
// side of IP-FR-001's fallback contract: when the batch ls-tree succeeds, the
// resolver serves map hits with NO further git invocation (the fake runner
// returns an error — classified as a git failure — if called past its
// script), and a map miss withholds without any invocation.
func TestNewPageBlobResolver_BatchHealthyNeverInvokesPerPage(t *testing.T) {
	dir := t.TempDir()
	runner := resolveThenBatchScript(fakeResponse{
		stdout: "100644 blob " + testSHA + "\tcompiled/auth.md\x00"})

	res := NewPageBlobResolver(context.Background(),
		Options{VaultDir: dir, Runner: runner, Env: mapEnv(nil)}, TrustPathspecs())

	// Map hit: served from the snapshot with no per-page invocation and no
	// working-tree file (the file side degrades to empty).
	fileSHA, refSHA := res.PageBlobs("compiled/auth.md")
	if fileSHA != "" || refSHA != testSHA {
		t.Errorf("PageBlobs(compiled/auth.md) = (%q, %q), want (%q, %q)", fileSHA, refSHA, "", testSHA)
	}
	// Map miss on a healthy map: withheld without any invocation.
	if f, r := res.PageBlobs("compiled/untracked.md"); f != "" || r != "" {
		t.Errorf("PageBlobs(compiled/untracked.md) = (%q, %q), want empty SHAs", f, r)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("expected 4 git invocations (3 resolve + 1 batch), got %d: %v",
			len(runner.calls), runner.calls)
	}
	wantBatch := []string{"ls-tree", "-r", "-z", "refs/remotes/origin/main", "--", "compiled", "knowledge"}
	if !reflect.DeepEqual(runner.calls[3], wantBatch) {
		t.Errorf("batch argv = %v, want %v", runner.calls[3], wantBatch)
	}
}
