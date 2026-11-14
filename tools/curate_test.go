package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unbound-force/dewey/v3/curate"
	"github.com/unbound-force/dewey/v3/llm"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

func TestCurateTool_NilStore(t *testing.T) {
	curateTool := NewCurate(nil, nil, nil, "", nil, 0)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for nil store")
	}
	text := resultText(result)
	if !strings.Contains(text, "curate requires persistent storage") {
		t.Errorf("error message = %q, should mention 'curate requires persistent storage'", text)
	}
	if !strings.Contains(text, "may hold the database lock") {
		t.Errorf("error message = %q, should mention the lock hint 'may hold the database lock'", text)
	}
}

func TestCurateTool_NoConfig(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Use a temp dir with no knowledge-stores.yaml.
	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	curateTool := NewCurate(s, nil, nil, dir, nil, 0)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for missing config")
	}
}

func TestCurateTool_StoreNotFound(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create a config with one store.
	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: team-decisions
    sources: [disk-local]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	curateTool := NewCurate(s, nil, nil, dir, nil, 0)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{
		Store: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for store not found")
	}
}

func TestCurateTool_ConcurrentCallRejected(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: test
    sources: [disk-local]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	mu := &sync.Mutex{}
	curateTool := NewCurate(s, nil, nil, dir, mu, 0)

	// Lock the mutex to simulate an indexing operation in progress.
	mu.Lock()

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for concurrent call")
	}

	mu.Unlock()
}

// setupCurateFixture creates an in-memory store, a knowledge-stores.yaml, and
// one indexed source page for curation tests.
func setupCurateFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	s := newTestStore(t)

	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: team-knowledge
    sources: [disk-meetings]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := s.InsertPage(&store.Page{
		Name:         "meetings/kickoff",
		OriginalName: "kickoff",
		SourceID:     "disk-meetings",
		SourceDocID:  "kickoff",
		ContentHash:  "hash-1",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&store.Block{
		UUID:     "block-kickoff",
		PageName: "meetings/kickoff",
		Content:  "We decided to use Option A for authentication.",
		Position: 0,
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	return s, dir
}

// TestCurateTool_PromptMode verifies the prompt-mode contract: status
// prompt_ready, one extraction prompt per store, and no files written.
func TestCurateTool_PromptMode(t *testing.T) {
	s, dir := setupCurateFixture(t)
	curateTool := NewCurate(s, nil, nil, dir, nil, 0)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected prompt mode, got: %s", resultText(result))
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(resultText(result)), &parsed); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if parsed["status"] != "prompt_ready" {
		t.Errorf("status = %v, want prompt_ready", parsed["status"])
	}
	stores, ok := parsed["stores"].([]any)
	if !ok || len(stores) != 1 {
		t.Fatalf("stores = %v, want one entry", parsed["stores"])
	}
	storeEntry, _ := stores[0].(map[string]any)
	if prompt, _ := storeEntry["extraction_prompt"].(string); strings.TrimSpace(prompt) == "" {
		t.Errorf("extraction_prompt is empty: %v", storeEntry)
	}
	if _, err := os.Stat(filepath.Join(dir, ".uf", "dewey", "knowledge")); !os.IsNotExist(err) {
		t.Errorf("prompt mode wrote knowledge files (stat err = %v)", err)
	}
}

// TestCurateTool_SynthesisUnavailableReasonReported verifies that prompt
// mode reports the gateway's unavailable reason while keeping every
// extraction prompt (FR-015).
func TestCurateTool_SynthesisUnavailableReasonReported(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.InsertPage(&store.Page{
		Name:       "disk-page",
		SourceID:   "disk-local",
		Properties: "{}",
	}); err != nil {
		t.Fatalf("insert page: %v", err)
	}
	if err := s.InsertBlock(&store.Block{
		UUID:     "block-1",
		PageName: "disk-page",
		Content:  "A decision was made to use gateway synthesis.",
	}); err != nil {
		t.Fatalf("insert block: %v", err)
	}

	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: team-knowledge
    sources: [disk-local]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	reason := "openai-compatible synthesis endpoint is unreachable; check the endpoint and that the gateway is running"
	synth := &reasonSynthesizer{
		NoopSynthesizer: llm.NoopSynthesizer{Avail: false},
		reason:          reason,
	}
	curateTool := NewCurate(s, nil, synth, dir, nil, 0)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected prompt mode, got error: %s", resultText(result))
	}

	text := resultText(result)
	if !strings.Contains(text, "prompt_ready") {
		t.Errorf("expected prompt_ready status, got %s", text)
	}
	if !strings.Contains(text, reason) {
		t.Errorf("prompt mode must report the unavailable reason, got %s", text)
	}
	if !strings.Contains(text, "extraction_prompt") {
		t.Errorf("prompt mode must preserve extraction prompts, got %s", text)
	}
}

// TestCurateTool_Synthesizes verifies that a synthesizer produces status
// complete, writes knowledge files, and indexes them as curated.
func TestCurateTool_Synthesizes(t *testing.T) {
	s, dir := setupCurateFixture(t)
	synth := &llm.NoopSynthesizer{
		Avail:    true,
		Model:    "fake-model",
		Response: `[{"tag":"auth","category":"decision","confidence":"high","sources":[{"source_id":"disk-meetings","document":"kickoff"}],"content":"Use Option A for authentication."}]`,
	}
	curateTool := NewCurate(s, nil, synth, dir, nil, 0)

	force := false
	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{Incremental: &force})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(resultText(result)), &parsed); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if parsed["status"] != "complete" {
		t.Errorf("status = %v, want complete", parsed["status"])
	}
	results, ok := parsed["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %v, want one store", parsed["results"])
	}
	storeResult, _ := results[0].(map[string]any)
	if files, _ := storeResult["files_created"].(float64); files < 1 {
		t.Errorf("files_created = %v, want >= 1", storeResult["files_created"])
	}

	knowledgePages, err := s.ListPagesBySource("knowledge-team-knowledge")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(knowledgePages) == 0 {
		t.Error("expected curated pages to be indexed")
	}
	for _, p := range knowledgePages {
		if p.Tier != "curated" {
			t.Errorf("page %q tier = %q, want curated", p.Name, p.Tier)
		}
	}

	// The curated file must exist on disk with curated frontmatter.
	knowledgeDir := filepath.Join(dir, ".uf", "dewey", "knowledge", "team-knowledge")
	entries, err := os.ReadDir(knowledgeDir)
	if err != nil {
		t.Fatalf("read knowledge dir: %v", err)
	}
	var found bool
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "_index.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(knowledgeDir, entry.Name()))
		if err != nil {
			t.Fatalf("read curated file: %v", err)
		}
		if !strings.Contains(string(data), "tier: \"curated\"") {
			t.Errorf("curated file missing tier frontmatter: %s", data)
		}
		found = true
	}
	if !found {
		t.Error("no curated markdown file written")
	}
}

// sentinelSynth fails synthesis when the prompt contains a sentinel string.
type sentinelSynth struct {
	failOn string
}

func (s *sentinelSynth) Synthesize(_ context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, s.failOn) {
		return "", fmt.Errorf("provider failure for sentinel")
	}
	return `[{"tag":"auth","category":"decision","confidence":"high","sources":[{"source_id":"src-a","document":"doc-a"}],"content":"Use Option A."}]`, nil
}
func (s *sentinelSynth) Available() bool { return true }
func (s *sentinelSynth) ModelID() string { return "fake-model" }

// TestCurateTool_PartialStoreFailure verifies that one failing store yields
// status partial with per-store results and an error result.
func TestCurateTool_PartialStoreFailure(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir dewey: %v", err)
	}
	ksContent := `stores:
  - name: store-a
    sources: [src-a]
  - name: store-b
    sources: [src-b]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	for _, doc := range []struct{ page, source, block, body string }{
		{"docs/a", "src-a", "block-a", "SENTINEL-OK content for store a"},
		{"docs/b", "src-b", "block-b", "SENTINEL-FAIL content for store b"},
	} {
		if err := s.InsertPage(&store.Page{
			Name: doc.page, OriginalName: doc.page, SourceID: doc.source,
			SourceDocID: doc.page, ContentHash: "hash-" + doc.source,
		}); err != nil {
			t.Fatalf("InsertPage(%s): %v", doc.page, err)
		}
		if err := s.InsertBlock(&store.Block{
			UUID: doc.block, PageName: doc.page, Content: doc.body, Position: 0,
		}); err != nil {
			t.Fatalf("InsertBlock(%s): %v", doc.block, err)
		}
	}

	curateTool := NewCurate(s, nil, &sentinelSynth{failOn: "SENTINEL-FAIL"}, dir, nil, 0)
	force := false
	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{Incremental: &force})
	if err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for partial curation, got: %s", resultText(result))
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(resultText(result)), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed["status"] != "partial" {
		t.Errorf("status = %v, want partial", parsed["status"])
	}
	results, ok := parsed["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %v, want two stores", parsed["results"])
	}
	var failed, succeeded int
	for _, raw := range results {
		entry, _ := raw.(map[string]any)
		if msg, _ := entry["error"].(string); msg != "" {
			failed++
		} else if files, _ := entry["files_created"].(float64); files >= 1 {
			succeeded++
		}
	}
	if failed != 1 || succeeded != 1 {
		t.Errorf("failed=%d succeeded=%d, want 1 and 1 (results: %v)", failed, succeeded, results)
	}
}

// TestAutoIndex_ValidatedTierIsContentBound repurposes the pre-VTB-FR-004
// preserve-on-change test for the content-bound contract: re-indexing a
// knowledge file whose content hash changed lands its page at tier
// "curated" — the same tier a fresh insert receives — so unattended
// curation never carries a human vouch onto new content, while a knowledge
// file whose content hash is unchanged is skipped without any page write,
// leaving a validated page's tier and row exactly as the promotion left
// them. The non-validated sibling still resets to "curated" on change.
//
// The no-write assertions are load-bearing: removing the hash-equality skip
// must fail the indexed-count and updated_at assertions — on an equal-hash
// rewrite the store's content-bound CASE keeps the tier, so only the
// row-level signals expose the rewrite — and reverting the unconditional
// "curated" assignment on the update path must fail the changed-file tier
// assertion. The changed files' stored hashes are stale by construction
// (their on-disk files are rewritten with different content before the
// second indexing), so the skip cannot produce a vacuous pass there.
func TestAutoIndex_ValidatedTierIsContentBound(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	storePath := filepath.Join(deweyDir, "knowledge", "test-store")
	// auth-1 and vault-2 are promoted to validated below; deploy-3 stays
	// curated as the non-validated sibling.
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)
	writeKnowledgeFile(t, storePath, "vault", 2, "high", nil)
	writeKnowledgeFile(t, storePath, "deploy", 3, "high", nil)

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 3 {
		t.Fatalf("first indexing = %d, want 3", indexed)
	}

	// promoteToValidated seeds the human promotion vouch on a knowledge
	// page. The real promote gate elevates only drafts (VTB-FR-005) and
	// knowledge pages insert at "curated", so the test writes the vouch
	// through the store directly; the write carries the page's unchanged
	// content hash, and the machine paths under test never perform it.
	promoteToValidated := func(name string) *store.Page {
		t.Helper()
		p, err := s.GetPage(name)
		if err != nil || p == nil {
			t.Fatalf("get %s: %v", name, err)
		}
		p.Tier = "validated"
		if err := s.UpdatePage(p); err != nil {
			t.Fatalf("promote %s: %v", name, err)
		}
		vouched, err := s.GetPage(name)
		if err != nil || vouched == nil {
			t.Fatalf("re-read %s: %v", name, err)
		}
		if vouched.Tier != "validated" {
			t.Fatalf("seed tier for %s = %q, want validated before reindex", name, vouched.Tier)
		}
		return vouched
	}
	changedBefore := promoteToValidated("knowledge/test-store/auth-1")
	unchangedBefore := promoteToValidated("knowledge/test-store/vault-2")

	// updated_at has millisecond resolution; guarantee the clock advances
	// past the seed writes so an erroneous write during the reindex below
	// would observably change the skipped page's updated_at.
	time.Sleep(2 * time.Millisecond)

	// Rewrite auth-1 (validated) and deploy-3 (curated) with changed content
	// so both take the update path; vault-2's file stays byte-identical so
	// its stored content hash still matches.
	writeKnowledgeFile(t, storePath, "auth", 1, "medium", []string{"missing_rationale"})
	writeKnowledgeFile(t, storePath, "deploy", 3, "medium", nil)

	// The unchanged file is skipped without a write, so only the two
	// changed files count as indexed (VTB-FR-004 scenario 2).
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 2 {
		t.Fatalf("second indexing = %d, want 2 (unchanged file skipped)", indexed)
	}

	// Changed validated file lands at curated — the same tier a fresh
	// insert receives — and the update path refreshed its content hash
	// (VTB-FR-004 scenario 1).
	changed, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || changed == nil {
		t.Fatalf("get auth page after reindex: %v", err)
	}
	if changed.Tier != "curated" {
		t.Errorf("changed validated page tier = %q, want 'curated' (a vouch must not carry onto new content)", changed.Tier)
	}
	if changed.ContentHash == changedBefore.ContentHash {
		t.Errorf("content hash unchanged (%q); update path should refresh it", changed.ContentHash)
	}

	// Unchanged validated file: skipped without any page write — the tier
	// keeps the human vouch and the row is untouched.
	unchanged, err := s.GetPage("knowledge/test-store/vault-2")
	if err != nil || unchanged == nil {
		t.Fatalf("get vault page after reindex: %v", err)
	}
	if unchanged.Tier != "validated" {
		t.Errorf("unchanged validated page tier = %q, want 'validated'", unchanged.Tier)
	}
	if unchanged.ContentHash != unchangedBefore.ContentHash {
		t.Errorf("unchanged page content hash = %q, want %q (skip path must not rewrite the row)", unchanged.ContentHash, unchangedBefore.ContentHash)
	}
	if unchanged.UpdatedAt != unchangedBefore.UpdatedAt {
		t.Errorf("unchanged page updated_at = %d, want %d (skip path must not rewrite the row)", unchanged.UpdatedAt, unchangedBefore.UpdatedAt)
	}

	// Complementary branch: a non-validated page is (re)set to curated on
	// the update path.
	other, err := s.GetPage("knowledge/test-store/deploy-3")
	if err != nil || other == nil {
		t.Fatalf("get deploy page: %v", err)
	}
	if other.Tier != "curated" {
		t.Errorf("non-validated page tier = %q, want 'curated'", other.Tier)
	}
}
