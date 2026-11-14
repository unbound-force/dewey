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

// TestAutoIndex_OverwritesValidatedTierCache pins the DB-tier-as-cache
// semantics on the knowledge-store update path (git-is-the-source-of-record,
// design D4): re-indexing a store whose file derives a non-validated tier
// overwrites a stale stored validated tier, because the DB tier is a cache
// recomputed from the file rule (GTS-FR-005). The seed content hash is stale
// by construction (the on-disk file is rewritten with different content
// before the second indexing), so the hash-skip short-circuit cannot produce
// a vacuous pass.
func TestAutoIndex_OverwritesValidatedTierCache(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	storePath := filepath.Join(deweyDir, "knowledge", "test-store")
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)
	writeKnowledgeFile(t, storePath, "vault", 2, "high", nil)

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 2 {
		t.Fatalf("first indexing = %d, want 2", indexed)
	}

	// Seed a stale validated cache row, as a pre-change DB (or an
	// old promote-era write) would hold: promote no longer writes
	// pages.tier under this change.
	promoted, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || promoted == nil {
		t.Fatalf("get auth page: %v", err)
	}
	promoted.Tier = "validated"
	if err := s.UpdatePage(promoted); err != nil {
		t.Fatalf("seed stale validated tier: %v", err)
	}
	before, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || before == nil {
		t.Fatalf("get auth page: %v", err)
	}
	if before.Tier != "validated" {
		t.Fatalf("seed tier = %q, want validated before reindex", before.Tier)
	}

	// Rewrite both files with changed content so the next indexing takes
	// the update path (the stored content hashes no longer match).
	writeKnowledgeFile(t, storePath, "auth", 1, "medium", []string{"missing_rationale"})
	writeKnowledgeFile(t, storePath, "vault", 2, "medium", nil)

	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 2 {
		t.Fatalf("second indexing = %d, want 2 (content changed)", indexed)
	}

	// The stale validated cache must converge to the file-derived tier
	// while the content hash is refreshed by the update path.
	after, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || after == nil {
		t.Fatalf("get auth page after reindex: %v", err)
	}
	if after.Tier != "curated" {
		t.Errorf("validated page tier = %q, want 'curated' (recomputed from the file, overwriting the stale cache)", after.Tier)
	}
	if after.ContentHash == before.ContentHash {
		t.Errorf("content hash unchanged (%q); update path should refresh it", after.ContentHash)
	}

	// Complementary branch: a non-validated page is (re)set to curated on
	// the update path.
	other, err := s.GetPage("knowledge/test-store/vault-2")
	if err != nil || other == nil {
		t.Fatalf("get vault page: %v", err)
	}
	if other.Tier != "curated" {
		t.Errorf("non-validated page tier = %q, want 'curated'", other.Tier)
	}
}

// writeRawKnowledgeFile writes a knowledge file with arbitrary frontmatter
// so tier-derivation tests can vary the tier claim (writeKnowledgeFile
// hardcodes tier: curated). extra is inserted verbatim between the tag
// line and the frontmatter terminator; empty means no tier claim.
func writeRawKnowledgeFile(t *testing.T, storePath, baseName, extra string) {
	t.Helper()
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatalf("create store dir: %v", err)
	}
	content := "---\ntag: " + baseName + "\n" + extra + "---\n\nTest knowledge content.\n"
	path := filepath.Join(storePath, baseName+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write knowledge file: %v", err)
	}
}

// TestAutoIndex_DerivesTierFromFrontmatter pins the IP-FR-001 wiring on
// the knowledge-store path: the tier is derived from the file's frontmatter
// rather than hardcoded — a canonical curated claim stays curated, a
// validated claim without blob proof reads as draft (GTS-FR-002), an
// absent tier reads as draft (GTS-FR-001), and a non-canonical case-variant
// claim reads as unknown → draft (GTS-FR-003) rather than being normalized
// into the vocabulary.
func TestAutoIndex_DerivesTierFromFrontmatter(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "curated-1", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "validated-1", "tier: validated\n")
	writeRawKnowledgeFile(t, storePath, "untiered-1", "")
	writeRawKnowledgeFile(t, storePath, "case-variant-1", "tier: Validated\n")

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 4 {
		t.Fatalf("indexing = %d, want 4", indexed)
	}

	tests := []struct {
		page string
		want string
	}{
		{page: "knowledge/test-store/curated-1", want: "curated"},
		{page: "knowledge/test-store/validated-1", want: "draft"},
		{page: "knowledge/test-store/untiered-1", want: "draft"},
		{page: "knowledge/test-store/case-variant-1", want: "draft"},
	}
	for _, tt := range tests {
		p, err := s.GetPage(tt.page)
		if err != nil || p == nil {
			t.Fatalf("GetPage(%q): %v (nil=%v)", tt.page, err, p == nil)
		}
		if p.Tier != tt.want {
			t.Errorf("page %q tier = %q, want %q", tt.page, p.Tier, tt.want)
		}
	}
}

// TestAutoIndex_UnchangedContentRecomputesTier pins GTS-FR-005 on the
// knowledge-store path: when content is unchanged the blocks are not
// re-indexed (the indexed count stays 0), but a drifted cached tier is
// still recomputed from the file's frontmatter.
func TestAutoIndex_UnchangedContentRecomputesTier(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "auth-1", "tier: curated\n")

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 1 {
		t.Fatalf("first indexing = %d, want 1", indexed)
	}

	// Simulate a stale cached tier — validated specifically, the hardest
	// case for the cache semantics (design D4): the unchanged-content
	// refresh must converge it back down to the file-derived tier.
	drifted, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || drifted == nil {
		t.Fatalf("GetPage: %v", err)
	}
	drifted.Tier = "validated"
	if err := s.UpdatePage(drifted); err != nil {
		t.Fatalf("UpdatePage drift: %v", err)
	}

	// Re-index with unchanged content: no block re-indexing, but the tier
	// cache is refreshed from the file.
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 0 {
		t.Errorf("second indexing = %d, want 0 (content unchanged)", indexed)
	}

	after, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil || after == nil {
		t.Fatalf("GetPage after: %v", err)
	}
	if after.Tier != "curated" {
		t.Errorf("tier after unchanged re-index = %q, want curated (recomputed)", after.Tier)
	}
}
