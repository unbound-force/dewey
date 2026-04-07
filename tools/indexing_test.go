package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// TestIndexing_Index_NilStore verifies that calling Index with a nil store
// returns an error result mentioning persistent storage (FR-008).
func TestIndexing_Index_NilStore(t *testing.T) {
	ix := NewIndexing(nil, nil, t.TempDir())

	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestIndexing_Reindex_NilStore verifies that calling Reindex with a nil store
// returns an error result mentioning persistent storage (FR-008).
func TestIndexing_Reindex_NilStore(t *testing.T) {
	ix := NewIndexing(nil, nil, t.TempDir())

	result, _, err := ix.Reindex(context.Background(), nil, types.ReindexInput{})
	if err != nil {
		t.Fatalf("Reindex returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestIndexing_Index_NoSources verifies that Index with a valid store but
// no sources.yaml file returns an error result about no sources configured.
// When sources.yaml doesn't exist, LoadSourcesConfig returns (nil, nil),
// which the handler treats as "no sources configured".
func TestIndexing_Index_NoSources(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()

	// Create the .uf/dewey/ directory but do NOT create sources.yaml.
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ix := NewIndexing(s, nil, tmpDir)

	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when no sources are configured")
	}

	text := resultText(result)
	if !strings.Contains(text, "no sources configured") && !strings.Contains(text, "No sources") {
		t.Errorf("error message = %q, should mention no sources configured", text)
	}
}

// TestIndexing_Index_ConcurrentCallRejected verifies that a second Index call
// while one is already in progress returns an "already in progress" error
// result (FR-005). We manually lock the mutex to simulate an in-progress
// operation.
func TestIndexing_Index_ConcurrentCallRejected(t *testing.T) {
	s := newTestStore(t)
	ix := NewIndexing(s, nil, t.TempDir())

	// Simulate an in-progress operation by locking the mutex.
	ix.mu.Lock()
	defer ix.mu.Unlock()

	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when another operation is in progress")
	}

	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}
}

// TestIndexing_Reindex_ConcurrentCallRejected verifies that a second Reindex
// call while one is already in progress returns an "already in progress"
// error result (FR-005). The mutex is shared between Index and Reindex.
func TestIndexing_Reindex_ConcurrentCallRejected(t *testing.T) {
	s := newTestStore(t)
	ix := NewIndexing(s, nil, t.TempDir())

	// Simulate an in-progress operation by locking the mutex.
	ix.mu.Lock()
	defer ix.mu.Unlock()

	result, _, err := ix.Reindex(context.Background(), nil, types.ReindexInput{})
	if err != nil {
		t.Fatalf("Reindex returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when another operation is in progress")
	}

	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}
}

// TestIndexing_Reindex_PreservesProtectedSources verifies that reindex does
// NOT delete pages belonging to protected sources ("disk-local" and
// "learning"). These sources contain user content that cannot be re-fetched
// from external sources (FR-009, R5).
func TestIndexing_Reindex_PreservesProtectedSources(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()

	// Create the .uf/dewey/ directory with an empty sources.yaml.
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sourcesYAML := "sources: []\n"
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatalf("WriteFile sources.yaml: %v", err)
	}

	// Insert pages with protected source IDs.
	if err := s.InsertPage(&store.Page{
		Name:         "vault-page",
		OriginalName: "Vault Page",
		SourceID:     "disk-local",
		SourceDocID:  "vault.md",
		ContentHash:  "abc",
		CreatedAt:    1,
		UpdatedAt:    1,
	}); err != nil {
		t.Fatalf("InsertPage(disk-local): %v", err)
	}

	if err := s.InsertPage(&store.Page{
		Name:         "learning/test-insight",
		OriginalName: "Test Insight",
		SourceID:     "learning",
		SourceDocID:  "test-insight",
		ContentHash:  "def",
		CreatedAt:    1,
		UpdatedAt:    1,
	}); err != nil {
		t.Fatalf("InsertPage(learning): %v", err)
	}

	// Insert a page with a non-protected source ID to verify it gets deleted.
	if err := s.InsertPage(&store.Page{
		Name:         "github/issue-1",
		OriginalName: "Issue 1",
		SourceID:     "github-org",
		SourceDocID:  "issue-1",
		ContentHash:  "ghi",
		CreatedAt:    1,
		UpdatedAt:    1,
	}); err != nil {
		t.Fatalf("InsertPage(github-org): %v", err)
	}

	// Insert source records so ListSources returns them for deletion.
	if err := s.InsertSource(&store.SourceRecord{
		ID: "disk-local", Type: "disk", Status: "active",
	}); err != nil {
		t.Fatalf("InsertSource(disk-local): %v", err)
	}
	if err := s.InsertSource(&store.SourceRecord{
		ID: "learning", Type: "learning", Status: "active",
	}); err != nil {
		t.Fatalf("InsertSource(learning): %v", err)
	}
	if err := s.InsertSource(&store.SourceRecord{
		ID: "github-org", Type: "github", Status: "active",
	}); err != nil {
		t.Fatalf("InsertSource(github-org): %v", err)
	}

	ix := NewIndexing(s, nil, tmpDir)

	result, _, err := ix.Reindex(context.Background(), nil, types.ReindexInput{})
	if err != nil {
		t.Fatalf("Reindex returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("Reindex returned error result: %s", resultText(result))
	}

	// Parse the summary to verify pages were deleted.
	text := resultText(result)
	var summary map[string]any
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Verify pages_deleted includes the github page.
	pagesDeleted, ok := summary["pages_deleted"].(float64)
	if !ok {
		t.Fatalf("pages_deleted missing or wrong type: %T", summary["pages_deleted"])
	}
	if pagesDeleted != 1 {
		t.Errorf("pages_deleted = %v, want 1 (only github-org page)", pagesDeleted)
	}

	// Verify protected pages still exist in the store.
	vaultPage, err := s.GetPage("vault-page")
	if err != nil {
		t.Fatalf("GetPage(vault-page): %v", err)
	}
	if vaultPage == nil {
		t.Error("disk-local page 'vault-page' was deleted — should be preserved")
	}

	learningPage, err := s.GetPage("learning/test-insight")
	if err != nil {
		t.Fatalf("GetPage(learning/test-insight): %v", err)
	}
	if learningPage == nil {
		t.Error("learning page 'learning/test-insight' was deleted — should be preserved")
	}

	// Verify the non-protected page was deleted.
	githubPage, err := s.GetPage("github/issue-1")
	if err != nil {
		t.Fatalf("GetPage(github/issue-1): %v", err)
	}
	if githubPage != nil {
		t.Error("github-org page 'github/issue-1' should have been deleted during reindex")
	}
}

// TestIndexing_Index_CrossMutexRejection verifies that the mutex is shared
// between Index and Reindex — locking via one blocks the other (FR-005).
func TestIndexing_Index_CrossMutexRejection(t *testing.T) {
	s := newTestStore(t)
	ix := NewIndexing(s, nil, t.TempDir())

	// Lock the mutex as if Reindex is running.
	ix.mu.Lock()
	defer ix.mu.Unlock()

	// Index should be rejected because Reindex holds the lock.
	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when Reindex holds the mutex")
	}

	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}
}
