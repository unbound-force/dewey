package vault

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
)

// These tests cover the IP-FR-001 wiring of the optional TierBlobResolver
// into the ingestion call sites (PersistPage via the vault scan,
// indexSourceDocuments via IndexDocuments): a compiled page whose blob equals
// the fetched default-branch ref ingests as validated, a working-tree edit
// downgrades to draft on the next index, and a nil resolver keeps the
// pre-wiring behavior (empty SHAs, grant withheld per GTS-FR-004).

// tierGitFixture initializes a git repository in dir with a deterministic
// identity so trust-tier tests can commit and move remote-tracking refs
// without network access (mirroring gitref.gitrefRepo).
func tierGitFixture(t *testing.T, dir string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tierGitRun(t, dir, "init", "-q", "-b", "main")
	tierGitRun(t, dir, "config", "user.name", "t")
	tierGitRun(t, dir, "config", "user.email", "t@example.com")
}

// tierGitRun runs one git command in dir, failing the test on error.
func tierGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// tierFixtureVault builds a git-backed vault with one committed compiled
// page (blob equal to the fetched default-branch ref) and per-vault config
// pinning the comparison ref — the highest-precedence IP-FR-004 leg, keeping
// the fixture independent of machine-wide dewey or git configuration.
func tierFixtureVault(t *testing.T) string {
	return tierCommittedVault(t, true)
}

// tierCommittedVault builds a git-backed vault with one committed compiled
// page claiming validated and per-vault config pinning the comparison ref.
// fetchRef selects the remote-tracking state: true points
// refs/remotes/origin/main at the commit (the local state a fetch leaves
// behind — blob equality holds); false leaves the ref absent entirely (a
// fresh clone before any fetch — GTS-FR-004's missing-ref fallback).
func tierCommittedVault(t *testing.T, fetchRef bool) string {
	t.Helper()
	dir := t.TempDir()
	tierGitFixture(t, dir)
	writeTierVaultConfig(t, dir, "origin/main")

	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	page := filepath.Join(dir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n\n# Auth\n\ncompiled body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tierGitRun(t, dir, "add", ".")
	tierGitRun(t, dir, "commit", "-q", "-m", "compiled pages")
	if fetchRef {
		tierGitRun(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	}
	return dir
}

// writeTierVaultConfig writes a per-vault config.yaml pinning the
// default-branch comparison ref.
func writeTierVaultConfig(t *testing.T, vaultDir, ref string) {
	t.Helper()
	cfgDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := "default_branch_ref: " + ref + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// tierResolver builds the production gitref resolver for a fixture vault.
func tierResolver(t *testing.T, vaultDir string) TierBlobResolver {
	t.Helper()
	return gitref.NewPageBlobResolver(context.Background(),
		gitref.Options{VaultDir: vaultDir}, gitref.TrustPathspecs())
}

// TestVaultStore_FullIndex_ValidatedViaBlobMatch covers IP-FR-001's "Ingest
// validated when blob matches" scenario through the real vault scan:
// PersistPage derives validated only when the frontmatter requests it and the
// working-tree blob equals the fetched default-branch blob.
func TestVaultStore_FullIndex_ValidatedViaBlobMatch(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	vs, s := newTestVaultStore(t, vaultDir)
	vs.SetTierBlobResolver(tierResolver(t, vaultDir))

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	page, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if page.Tier != "validated" {
		t.Errorf("pages.tier = %q, want validated (blob equals origin/main)", page.Tier)
	}
}

// TestVaultStore_FullIndex_StaleValidityNotValidated covers GTS-FR-003's
// "Stale validated page is not validated" scenario through the real vault
// scan: a compiled page whose file claims tier: validated with validity:
// stale-unverified ingests as draft even though its blob equals the fetched
// default-branch ref (the downgrade outranks the GTS-FR-001 grant), and the
// stale-unverified validity is preserved in the page's properties.
func TestVaultStore_FullIndex_StaleValidityNotValidated(t *testing.T) {
	dir := t.TempDir()
	tierGitFixture(t, dir)
	writeTierVaultConfig(t, dir, "origin/main")

	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	page := filepath.Join(dir, "compiled", "auth.md")
	content := "---\ntier: validated\nvalidity: stale-unverified\n---\n\n# Auth\n\ncompiled body\n"
	if err := os.WriteFile(page, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tierGitRun(t, dir, "add", ".")
	tierGitRun(t, dir, "commit", "-q", "-m", "stale validated page")
	tierGitRun(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")

	vs, s := newTestVaultStore(t, dir)
	vs.SetTierBlobResolver(tierResolver(t, dir))

	c := New(dir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft (stale-unverified validity downgrades validated despite blob match)", got.Tier)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(got.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["validity"] != "stale-unverified" {
		t.Errorf("properties.validity = %v, want stale-unverified (preserved)", props["validity"])
	}
}

// TestVaultStore_IncrementalIndex_EditDowngradesToDraft covers IP-FR-001's
// "Ingest draft when blob diverges" scenario: after a working-tree edit, the
// next index re-derives the cached tier from the moved blob (GTS-FR-005).
func TestVaultStore_IncrementalIndex_EditDowngradesToDraft(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	vs, s := newTestVaultStore(t, vaultDir)
	vs.SetTierBlobResolver(tierResolver(t, vaultDir))

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Working-tree edit: the file blob diverges from the default branch.
	page := filepath.Join(vaultDir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n\n# Auth\n\nregenerated body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c2 := New(vaultDir, WithStore(vs.store))
	if _, err := vs.IncrementalIndex(c2); err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft (blob diverges from origin/main)", got.Tier)
	}
}

// TestVaultStore_NilResolverWithholdsGrant pins the backward-compatible
// default: with no resolver installed, a validated-claiming compiled page
// ingests as draft (empty blob SHAs, GTS-FR-004) — identical to the
// pre-wiring behavior.
func TestVaultStore_NilResolverWithholdsGrant(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	vs, s := newTestVaultStore(t, vaultDir)
	// Deliberately no SetTierBlobResolver call.

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft without a resolver (grant withheld)", got.Tier)
	}
}

// TestIndexDocuments_TierBlobResolverWiring covers the source-indexing call
// site: IndexDocuments derives the tier for a disk-source document through
// the optional trailing resolver (IP-FR-001), and without one keeps the
// pre-wiring behavior.
func TestIndexDocuments_TierBlobResolverWiring(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	content, err := os.ReadFile(filepath.Join(vaultDir, "compiled", "auth.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	docs := map[string][]source.Document{
		"disk-local": {{
			ID:          "compiled/auth.md",
			Title:       "compiled/auth",
			Content:     string(content),
			ContentHash: "fixture",
			SourceID:    "disk-local",
		}},
	}

	t.Run("with resolver ingests validated on blob match", func(t *testing.T) {
		s, err := store.New(":memory:")
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		defer func() { _ = s.Close() }()

		if _, err := IndexDocuments(s, docs, nil, nil, 0, tierResolver(t, vaultDir)); err != nil {
			t.Fatalf("IndexDocuments: %v", err)
		}
		page, err := s.GetPage("disk-local/compiled/auth.md")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page == nil {
			t.Fatal("disk-local/compiled/auth not persisted")
		}
		if page.Tier != "validated" {
			t.Errorf("pages.tier = %q, want validated (blob equals origin/main)", page.Tier)
		}
	})

	t.Run("without resolver withholds the grant", func(t *testing.T) {
		s, err := store.New(":memory:")
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		defer func() { _ = s.Close() }()

		if _, err := IndexDocuments(s, docs, nil, nil, 0); err != nil {
			t.Fatalf("IndexDocuments: %v", err)
		}
		page, err := s.GetPage("disk-local/compiled/auth.md")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page == nil {
			t.Fatal("disk-local/compiled/auth not persisted")
		}
		if page.Tier != "draft" {
			t.Errorf("pages.tier = %q, want draft without a resolver (grant withheld)", page.Tier)
		}
	})
}

// TestVaultStore_FullIndex_BranchOnlyCommitStaysDraft covers PW-FR-003's
// branch-only scenario at the ingestion seam: a compiled page committed on a
// NON-default branch leaves a clean working tree whose blob differs from the
// fetched origin/main blob, so the validated claim in its frontmatter cannot
// self-promote (GTS-FR-002) and the page ingests as draft — only a merge plus
// fetch (the blob-match test above) flips it to validated.
func TestVaultStore_FullIndex_BranchOnlyCommitStaysDraft(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	// Commit a divergent v2 on a non-default branch; the working tree is
	// clean afterwards, so only the branch — not a local edit — diverges.
	tierGitRun(t, vaultDir, "checkout", "-q", "-b", "feature/compiled")
	page := filepath.Join(vaultDir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n\n# Auth\n\nbranch-only body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tierGitRun(t, vaultDir, "add", ".")
	tierGitRun(t, vaultDir, "commit", "-q", "-m", "branch-only page")

	vs, s := newTestVaultStore(t, vaultDir)
	vs.SetTierBlobResolver(tierResolver(t, vaultDir))

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft (branch-only commit diverges from origin/main)", got.Tier)
	}
}

// TestVaultStore_FullIndex_RegeneratedCommitAheadOfFetchedRefStaysDraft
// covers #204's committed variant at the ingestion seam: a regenerated
// article (blob B) committed to the local default branch while the fetched
// origin/main still holds the merged original (blob A). The page claims
// validated in its frontmatter, but blob comparison must withhold the grant —
// frontmatter alone cannot validate (GTS-FR-002). The uncommitted worktree
// variant is already covered by the IncrementalIndex edit test.
func TestVaultStore_FullIndex_RegeneratedCommitAheadOfFetchedRefStaysDraft(t *testing.T) {
	vaultDir := tierFixtureVault(t)

	// Regenerate and commit on the local default branch; origin/main still
	// points at the merged commit, and the working tree is clean afterwards.
	page := filepath.Join(vaultDir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n\n# Auth\n\nregenerated body\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	tierGitRun(t, vaultDir, "add", ".")
	tierGitRun(t, vaultDir, "commit", "-q", "-m", "regenerate article")

	vs, s := newTestVaultStore(t, vaultDir)
	vs.SetTierBlobResolver(tierResolver(t, vaultDir))

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft (regenerated blob B vs default-branch blob A, #204)", got.Tier)
	}
}

// TestVaultStore_FullIndex_MissingRefFallsBackToDraft covers GTS-FR-004's
// source-level fallback through the real resolver: a git vault whose
// refs/remotes/origin/main was never fetched (a fresh clone before any fetch)
// resolves to missing-ref, every PageBlobs lookup withholds both SHAs, and a
// validated-claiming compiled page ingests as draft — ingestion succeeds and
// never errors on the unresolvable ref.
func TestVaultStore_FullIndex_MissingRefFallsBackToDraft(t *testing.T) {
	vaultDir := tierCommittedVault(t, false)

	vs, s := newTestVaultStore(t, vaultDir)
	vs.SetTierBlobResolver(tierResolver(t, vaultDir))

	c := New(vaultDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("compiled/auth not persisted")
	}
	if got.Tier != "draft" {
		t.Errorf("pages.tier = %q, want draft (missing ref withholds the validated grant)", got.Tier)
	}
}
