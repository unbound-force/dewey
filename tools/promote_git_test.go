package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
)

// These tests cover the promote-driven trust lifecycle (PW-FR-003) and the
// two-server convergence contract (GTS-FR-005) end-to-end through the real
// Promote handler and the real gitref resolver: promoting on a feature
// branch edits the file but keeps every index at draft until the branch
// merges to the default branch and the ref is fetched — the merge is the
// vouch. The blob-equality, edit-downgrade, nil-resolver, branch-only, and
// missing-ref ingestion seams are covered by vault's tier wiring tests;
// these tests add the promote act itself.

// promoteGitSeed is the committed on-disk shape of the fixture's draft
// compiled page.
const promoteGitSeed = "---\ntier: draft\n---\n\n# Auth\n\nseed body\n"

// promoteGitFixture initializes a git repository in dir with a deterministic
// identity so promote lifecycle tests can commit and move remote-tracking
// refs without network access (mirroring vault.tierGitFixture).
func promoteGitFixture(t *testing.T, dir string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	promoteGitRun(t, dir, "init", "-q", "-b", "main")
	promoteGitRun(t, dir, "config", "user.name", "t")
	promoteGitRun(t, dir, "config", "user.email", "t@example.com")
}

// promoteGitRun runs one git command in dir, failing the test on error.
func promoteGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// promoteGitVault builds a git-backed vault whose compiled page is a draft
// committed on main, with refs/remotes/origin/main pointing at the commit
// (the local state a fetch leaves behind) and per-vault config pinning the
// comparison ref — keeping the fixture independent of machine-wide dewey
// or git configuration.
func promoteGitVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	promoteGitFixture(t, dir)

	cfgDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := "default_branch_ref: origin/main\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compiled", "auth.md"), []byte(promoteGitSeed), 0o644); err != nil {
		t.Fatalf("WriteFile page: %v", err)
	}
	promoteGitRun(t, dir, "add", ".")
	promoteGitRun(t, dir, "commit", "-q", "-m", "seed compiled page")
	promoteGitRun(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	return dir
}

// promoteGitIndex runs a full vault index of vaultDir against s with the
// production gitref tier resolver installed — the same wiring dewey serve
// uses (main.go indexVault), so tier re-derivation here matches a running
// server's next index.
func promoteGitIndex(t *testing.T, s *store.Store, vaultDir string) {
	t.Helper()
	vs := vault.NewVaultStore(s, vaultDir, "disk-local")
	vs.SetTierBlobResolver(gitref.NewPageBlobResolver(context.Background(),
		gitref.Options{VaultDir: vaultDir}, gitref.TrustPathspecs()))
	c := vault.New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}
}

// promoteGitTier returns the stored tier of a page, failing on lookup
// problems.
func promoteGitTier(t *testing.T, s *store.Store, page string) string {
	t.Helper()
	p, err := s.GetPage(page)
	if err != nil {
		t.Fatalf("GetPage(%s): %v", page, err)
	}
	if p == nil {
		t.Fatalf("page %s missing from the store", page)
	}
	return p.Tier
}

// TestPromote_GitTrustLifecycle covers PW-FR-003 end-to-end through the
// real Promote handler: promoting a draft compiled page on a feature branch
// edits the file (and only the file), every index while the branch is
// unmerged — uncommitted or committed — re-derives draft, and only after
// the merge reaches the default branch and the ref is fetched does the
// next index grant validated. The merge is the vouch; promote itself never
// touches the database tier (PW-FR-002).
func TestPromote_GitTrustLifecycle(t *testing.T) {
	vaultDir := promoteGitVault(t)

	s := newTestStore(t)
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("baseline tier = %q, want draft (seed claims draft)", got)
	}

	// Promote on a feature branch: the handler edits the file frontmatter.
	promoteGitRun(t, vaultDir, "checkout", "-q", "-b", "feature/promote-auth")
	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/auth",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("Promote returned error: %s", resultText(result))
	}

	promoted := "---\ntier: validated\n---\n\n# Auth\n\nseed body\n"
	if got := readVaultFile(t, vaultDir, "compiled/auth.md"); got != promoted {
		t.Fatalf("file after promote = %q, want %q", got, promoted)
	}
	// PW-FR-002: the database tier is untouched by the promote itself.
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("tier after promote = %q, want draft (promote never writes the DB tier)", got)
	}

	// Uncommitted promotion on the branch: the working-tree blob diverges
	// from origin/main, so the next index stays draft.
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("tier after uncommitted promote = %q, want draft (working tree diverges)", got)
	}

	// Committed promotion on the branch: the working tree is clean, but the
	// branch is not the fetched default branch — still draft (PW-FR-003).
	promoteGitRun(t, vaultDir, "add", ".")
	promoteGitRun(t, vaultDir, "commit", "-q", "-m", "promote compiled/auth")
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("tier after branch-only commit = %q, want draft (merge is the vouch)", got)
	}

	// Merge to the default branch and fetch: the file blob now equals the
	// fetched ref's blob, so the next index grants validated.
	promoteGitRun(t, vaultDir, "checkout", "-q", "main")
	promoteGitRun(t, vaultDir, "merge", "-q", "--ff-only", "feature/promote-auth")
	promoteGitRun(t, vaultDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "validated" {
		t.Fatalf("tier after merge+fetch = %q, want validated (blob equals origin/main)", got)
	}

	// The merged file is byte-identical to what promote wrote — the merge
	// carried the promotion, nothing re-edited it.
	if got := readVaultFile(t, vaultDir, "compiled/auth.md"); got != promoted {
		t.Errorf("file after merge = %q, want %q", got, promoted)
	}
}

// TestPromote_TwoServersConvergeOnSameRef covers GTS-FR-005: the database
// tier is a cache, so two servers with independent databases indexing the
// same files against the same fetched default-branch ref derive the same
// tiers — including a server that never saw the draft state (a fresh
// database converges on its first index after the merge).
func TestPromote_TwoServersConvergeOnSameRef(t *testing.T) {
	vaultDir := promoteGitVault(t)

	// Server one: indexes the draft, promotes on a branch, merges, fetches.
	s1 := newTestStore(t)
	promoteGitIndex(t, s1, vaultDir)
	if got := promoteGitTier(t, s1, "compiled/auth"); got != "draft" {
		t.Fatalf("server one baseline tier = %q, want draft", got)
	}

	promoteGitRun(t, vaultDir, "checkout", "-q", "-b", "feature/promote-auth")
	result, _, err := NewPromote(s1, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/auth",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("Promote returned error: %s", resultText(result))
	}
	promoteGitRun(t, vaultDir, "add", ".")
	promoteGitRun(t, vaultDir, "commit", "-q", "-m", "promote compiled/auth")
	promoteGitRun(t, vaultDir, "checkout", "-q", "main")
	promoteGitRun(t, vaultDir, "merge", "-q", "--ff-only", "feature/promote-auth")
	promoteGitRun(t, vaultDir, "update-ref", "refs/remotes/origin/main", "HEAD")

	// Server one re-indexes after the merge: validated.
	promoteGitIndex(t, s1, vaultDir)
	if got := promoteGitTier(t, s1, "compiled/auth"); got != "validated" {
		t.Fatalf("server one tier after merge = %q, want validated", got)
	}

	// Server two — a fresh database that never saw the draft state, the
	// same working tree and fetched ref — derives the same tier.
	s2 := newTestStore(t)
	promoteGitIndex(t, s2, vaultDir)
	if got := promoteGitTier(t, s2, "compiled/auth"); got != "validated" {
		t.Fatalf("server two tier = %q, want validated (same files and ref derive the same tier)", got)
	}

	// Both servers also agree the seed's stale pre-merge state is gone: no
	// database carries a tier the git rule does not re-derive.
	for name, s := range map[string]*store.Store{"server one": s1, "server two": s2} {
		if got := promoteGitTier(t, s, "compiled/auth"); got != "validated" {
			t.Errorf("%s tier = %q, want validated", name, got)
		}
	}
}

// TestPromote_WorkingTreeClaimAloneNeverValidates pins the negative space
// of the lifecycle (GTS-FR-002): hand-editing `tier: validated` into the
// file without committing it to the fetched default branch — the shape a
// compromised agent would produce by bypassing promote — never grants
// validated, on any index, because the working-tree blob differs from the
// default-branch blob.
func TestPromote_WorkingTreeClaimAloneNeverValidates(t *testing.T) {
	vaultDir := promoteGitVault(t)

	// Forge the claim directly on the default branch, uncommitted.
	if err := os.WriteFile(filepath.Join(vaultDir, "compiled", "auth.md"),
		[]byte("---\ntier: validated\n---\n\n# Auth\n\nforged body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := newTestStore(t)
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("tier = %q, want draft (frontmatter alone cannot validate)", got)
	}

	// Even committing the forged claim locally leaves the fetched ref
	// behind, so the grant stays withheld (#204 regenerated-commit shape).
	promoteGitRun(t, vaultDir, "add", ".")
	promoteGitRun(t, vaultDir, "commit", "-q", "-m", "forge claim")
	promoteGitIndex(t, s, vaultDir)
	if got := promoteGitTier(t, s, "compiled/auth"); got != "draft" {
		t.Fatalf("tier after local commit = %q, want draft (fetched ref still holds the draft blob)", got)
	}

	// The file never became promotable-through-forgery: its stored tier
	// stayed draft throughout, so a later promote attempt would still be
	// a legitimate draft→validated transition.
	if !strings.Contains(readVaultFile(t, vaultDir, "compiled/auth.md"), "tier: validated") {
		t.Fatal("fixture sanity: forged claim should still be on disk")
	}
}
