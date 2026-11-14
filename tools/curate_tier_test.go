package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/unbound-force/dewey/v3/curate"
	"github.com/unbound-force/dewey/v3/store"
)

// TestIndexKnowledgeStore_DerivesTierFromGitBlobs covers the third IP-FR-001
// call site: the knowledge-store indexer derives each file's tier from
// frontmatter plus default-branch blob equality, using the vault's repository
// as the trust root (design D2 — one ls-tree scoped to the store directory).
// A merged file (blob equal to origin/main) ingests as validated; a
// regenerated file (working-tree edit after the merge) ingests as draft.
func TestIndexKnowledgeStore_DerivesTierFromGitBlobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	vaultDir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", vaultDir}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.name", "t")
	git("config", "user.email", "t@example.com")

	// Per-vault config pins the comparison ref — the highest-precedence
	// IP-FR-004 leg — so the fixture does not depend on machine-wide dewey
	// or git configuration.
	cfgDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(filepath.Join(cfgDir, "knowledge", "store1"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("default_branch_ref: origin/main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	merged := filepath.Join(cfgDir, "knowledge", "store1", "merged.md")
	regenerated := filepath.Join(cfgDir, "knowledge", "store1", "regenerated.md")
	if err := os.WriteFile(merged, []byte("---\ntier: validated\n---\n\nmerged knowledge\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(regenerated, []byte("---\ntier: validated\n---\n\noriginal knowledge\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "knowledge store")
	git("update-ref", "refs/remotes/origin/main", "HEAD")

	// Regenerate one article after the merge: its working-tree blob diverges
	// from the default branch, so it must read as draft (GTS-FR-002).
	if err := os.WriteFile(regenerated, []byte("---\ntier: validated\n---\n\nregenerated knowledge\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	cfg := curate.StoreConfig{Name: "store1"}
	if indexed := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger); indexed != 2 {
		t.Fatalf("IndexKnowledgeStore indexed %d files, want 2", indexed)
	}

	mergedPage, err := s.GetPage("knowledge/store1/merged")
	if err != nil {
		t.Fatalf("GetPage(merged): %v", err)
	}
	if mergedPage == nil {
		t.Fatal("knowledge/store1/merged not persisted")
	}
	if mergedPage.Tier != "validated" {
		t.Errorf("merged page tier = %q, want validated (blob equals origin/main)", mergedPage.Tier)
	}

	regenPage, err := s.GetPage("knowledge/store1/regenerated")
	if err != nil {
		t.Fatalf("GetPage(regenerated): %v", err)
	}
	if regenPage == nil {
		t.Fatal("knowledge/store1/regenerated not persisted")
	}
	if regenPage.Tier != "draft" {
		t.Errorf("regenerated page tier = %q, want draft (blob diverges from origin/main)", regenPage.Tier)
	}
}
