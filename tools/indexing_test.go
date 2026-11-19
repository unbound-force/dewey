package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// TestIndexing_Index_NilStore verifies that calling Index with a nil store
// returns an error result mentioning persistent storage (FR-008).
func TestIndexing_Index_NilStore(t *testing.T) {
	ix := NewIndexing(nil, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

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
	ix := NewIndexing(nil, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

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

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{})

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
	ix := NewIndexing(s, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

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
	ix := NewIndexing(s, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

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

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{})

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
	ix := NewIndexing(s, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

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

// --- External mutex injection tests (T007, spec 012) ---

// TestIndexing_ExternalMutex_SharedLock verifies that when an external mutex
// is injected via NewIndexing(), locking it externally prevents Index() from
// proceeding — it returns "already in progress". This is the mechanism that
// enables mutual exclusion between background startup indexing and the
// index/reindex MCP tools (spec 012, D1).
func TestIndexing_ExternalMutex_SharedLock(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()

	// Create the .uf/dewey/ directory with sources.yaml so Index() can
	// proceed past the config loading step (when the mutex is unlocked).
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create an external mutex and lock it — simulating background indexing
	// holding the lock during startup.
	mu := &sync.Mutex{}
	mu.Lock()

	// Pass the locked external mutex to NewIndexing.
	ix := NewIndexing(s, nil, tmpDir, mu, 0, IndexBuildInfo{})

	// Call Index() — should return "already in progress" because the
	// external mutex is locked.
	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result when external mutex is locked")
	}

	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}

	// Unlock the external mutex — simulating background indexing completion.
	mu.Unlock()

	// Call Index() again — should now proceed past the mutex check.
	// It will return "no sources configured" because sources.yaml doesn't
	// exist, which is fine — the point is it got past the mutex.
	result2, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index (after unlock) returned Go error: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected non-nil result after unlock")
	}

	// The result should be an error about no sources, NOT "already in progress".
	text2 := resultText(result2)
	if strings.Contains(text2, "already in progress") {
		t.Errorf("after unlock, should not get 'already in progress', got: %q", text2)
	}
}

// TestIndexing_ExternalMutex_NilFallback verifies that passing nil for the
// mutex parameter creates an internal mutex — backward compatible behavior.
// The internal mutex still provides mutual exclusion between Index and Reindex.
func TestIndexing_ExternalMutex_NilFallback(t *testing.T) {
	s := newTestStore(t)

	// Pass nil mutex — NewIndexing should create an internal one.
	ix := NewIndexing(s, nil, t.TempDir(), nil, 0, IndexBuildInfo{})

	// Verify the internal mutex works by locking it and checking rejection.
	// PARALLEL SAFETY: We access ix.mu directly because this is a package-
	// internal test (tools_test package has access to unexported fields).
	ix.mu.Lock()

	// Index should be rejected because the internal mutex is locked.
	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when internal mutex is locked")
	}

	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}

	// Also verify Reindex is blocked by the same internal mutex.
	result2, _, err := ix.Reindex(context.Background(), nil, types.ReindexInput{})
	if err != nil {
		t.Fatalf("Reindex returned Go error: %v", err)
	}
	if !result2.IsError {
		t.Fatal("expected error result for Reindex when internal mutex is locked")
	}

	text2 := resultText(result2)
	if !strings.Contains(text2, "already in progress") {
		t.Errorf("Reindex error message = %q, should mention 'already in progress'", text2)
	}

	ix.mu.Unlock()
}

// TestIndexing_ReportSourceErrors verifies that indexing reports per-source
// fetch errors to the store: failing sources present in the store get an
// error status, while unknown sources and clean summaries are left alone.
func TestIndexing_ReportSourceErrors(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertSource(&store.SourceRecord{
		ID:     "existing-src",
		Type:   "disk",
		Name:   "Existing",
		Status: "ok",
	}); err != nil {
		t.Fatalf("InsertSource failed: %v", err)
	}

	ix := NewIndexing(s, nil, t.TempDir(), nil, 0, IndexBuildInfo{})
	ix.reportSourceErrors(&source.FetchResult{Summaries: []source.FetchSummary{
		{SourceID: "clean-src", SourceType: "disk", Documents: 3},
		{SourceID: "missing-src", SourceType: "disk", Error: "fetch failed"},
		{SourceID: "existing-src", SourceType: "disk", Error: "fetch failed"},
	}})

	rec, err := s.GetSource("existing-src")
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if rec == nil || rec.Status != "error" || rec.ErrorMessage != "fetch failed" {
		t.Errorf("existing source after report = %+v, want status error with message fetch failed", rec)
	}

	missing, _ := s.GetSource("missing-src")
	if missing != nil {
		t.Errorf("reportSourceErrors must not create missing sources, got %+v", missing)
	}
}

// --- Index build provenance stamping tests (IBP-FR-001, design D4) ---

// writeIndexableVault prepares a minimal vault root that a successful
// Index/Reindex pass can run over: a .uf/dewey/sources.yaml configuring
// one disk source rooted at the vault itself, plus one markdown note for
// the pass to index. The .uf directory is hidden, so the disk source's
// ignore matcher skips it and the walk finds only the note.
func writeIndexableVault(t *testing.T, tmpDir string) {
	t.Helper()
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sourcesYAML := "sources:\n  - id: test-disk\n    type: disk\n    name: Test Disk\n    config:\n      path: \".\"\n"
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatalf("WriteFile sources.yaml: %v", err)
	}
	note := "# Note\n\nContent for the indexing pass to index.\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "note.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("WriteFile note.md: %v", err)
	}
}

// assertBuildStamp reads the two IBP-FR-001 metadata keys back from the
// store and asserts their exact values. An empty want value asserts the
// key is absent — GetMeta returns "" when the key was never written.
func assertBuildStamp(t *testing.T, s *store.Store, wantVersion, wantFormat string) {
	t.Helper()
	version, err := s.GetMeta("dewey_version")
	if err != nil {
		t.Fatalf("GetMeta(dewey_version): %v", err)
	}
	if version != wantVersion {
		t.Errorf("dewey_version = %q, want %q", version, wantVersion)
	}
	format, err := s.GetMeta("index_format")
	if err != nil {
		t.Fatalf("GetMeta(index_format): %v", err)
	}
	if format != wantFormat {
		t.Errorf("index_format = %q, want %q", format, wantFormat)
	}
}

// assertSummaryPagesIndexed unmarshals an Index/Reindex result summary and
// asserts the pass indexed exactly want pages, proving the handler ran a
// real indexing pass — the precondition for stamping (design D4).
func assertSummaryPagesIndexed(t *testing.T, text string, want float64) {
	t.Helper()
	var summary map[string]any
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if pages, ok := summary["pages_indexed"].(float64); !ok || pages != want {
		t.Errorf("pages_indexed = %v, want %v", summary["pages_indexed"], want)
	}
}

// TestIndexing_Index_StampBuildInfo verifies that a successful Index pass
// stamps the injected IndexBuildInfo into the store's metadata (IBP-FR-001,
// design D4): dewey_version recorded verbatim and index_format as a decimal
// string.
func TestIndexing_Index_StampBuildInfo(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	writeIndexableVault(t, tmpDir)

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{DeweyVersion: "test-version", IndexFormat: 42})

	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("Index returned error result: %s", resultText(result))
	}

	assertSummaryPagesIndexed(t, resultText(result), 1)
	assertBuildStamp(t, s, "test-version", "42")
}

// TestIndexing_Reindex_StampBuildInfo verifies that a successful Reindex
// rebuild stamps the injected IndexBuildInfo into the store's metadata
// (IBP-FR-001, design D4) with the same semantics as Index.
func TestIndexing_Reindex_StampBuildInfo(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	writeIndexableVault(t, tmpDir)

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{DeweyVersion: "test-version", IndexFormat: 42})

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

	assertSummaryPagesIndexed(t, resultText(result), 1)
	assertBuildStamp(t, s, "test-version", "42")
}

// TestIndexing_Index_ZeroBuildInfoSkipsStamp verifies the zero-value no-op:
// a handler constructed with IndexBuildInfo{} completes a successful pass
// but leaves both metadata keys unwritten, preserving pre-stamping behavior.
func TestIndexing_Index_ZeroBuildInfoSkipsStamp(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	writeIndexableVault(t, tmpDir)

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{})

	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("Index returned error result: %s", resultText(result))
	}

	assertSummaryPagesIndexed(t, resultText(result), 1)
	assertBuildStamp(t, s, "", "")
}

// TestIndexing_Reindex_ZeroBuildInfoSkipsStamp verifies the zero-value
// no-op for Reindex: a handler constructed with IndexBuildInfo{} completes
// a successful rebuild without writing either metadata key.
func TestIndexing_Reindex_ZeroBuildInfoSkipsStamp(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	writeIndexableVault(t, tmpDir)

	ix := NewIndexing(s, nil, tmpDir, nil, 0, IndexBuildInfo{})

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

	assertSummaryPagesIndexed(t, resultText(result), 1)
	assertBuildStamp(t, s, "", "")
}
