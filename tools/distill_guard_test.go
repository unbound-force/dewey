// Distiller tier guard tests (specs/018-weekly-distill-automation, T009;
// FR-012/FR-017; contracts/distiller-allowlist.md Denied).
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/v3/types"
)

func TestGuardDistillTier_RejectsValidated(t *testing.T) {
	err := GuardDistillTier(tierValidated)
	if err == nil {
		t.Fatal("GuardDistillTier(validated) = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "human-only") {
		t.Errorf("error = %v, want the human-only rationale", err)
	}
}

func TestGuardDistillTier_AllowsDraft(t *testing.T) {
	if err := GuardDistillTier(DistillTierDraft); err != nil {
		t.Fatalf("GuardDistillTier(draft) = %v, want nil", err)
	}
	// An empty tier (a path with no tier notion) is not a validated
	// write; the guard only polices the human-only tier.
	if err := GuardDistillTier(""); err != nil {
		t.Fatalf("GuardDistillTier(\"\") = %v, want nil", err)
	}
}

// TestStoreCompiled_AlwaysWritesDraftFrontmatter pins the distiller's
// primary article-write path to tier draft end to end: the page row, the
// properties JSON, and the article file frontmatter all carry draft
// (FR-012; data-model.md RollingPR.tier_written). A refactor that lets a
// validated tier reach this path fails here before it fails in
// production.
func TestStoreCompiled_AlwaysWritesDraftFrontmatter(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	storeLearningWithProps(t, s, "auth-1", `{"tag":"auth","created_at":"2026-03-15T10:00:01Z"}`, "first", time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))
	vaultPath := t.TempDir()
	c := NewCompile(s, nil, nil, vaultPath, nil, 0)

	result, _, err := c.StoreCompiled(context.Background(), nil, types.StoreCompiledInput{
		Tag:     "authentication",
		Content: "# Authentication\n\nBody.",
		Sources: []string{"auth-1"},
	})
	if err != nil {
		t.Fatalf("StoreCompiled: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreCompiled returned error: %s", resultText(result))
	}

	data, err := os.ReadFile(filepath.Join(vaultPath, ".uf", "dewey", "compiled", "authentication.md"))
	if err != nil {
		t.Fatalf("read article: %v", err)
	}
	if !strings.Contains(string(data), "tier: draft") {
		t.Errorf("article frontmatter missing 'tier: draft':\n%s", data)
	}
	if strings.Contains(string(data), "tier: validated") {
		t.Errorf("article frontmatter carries tier: validated:\n%s", data)
	}

	page, err := s.GetPage("compiled/authentication")
	if err != nil || page == nil {
		t.Fatalf("GetPage: %v, %v", page, err)
	}
	if page.Tier != DistillTierDraft {
		t.Errorf("page tier = %q, want %q", page.Tier, DistillTierDraft)
	}
}
