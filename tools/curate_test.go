package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/log"

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

// TestAutoIndex_PersistsFrontmatterProperties verifies that knowledge-store
// pages carry their frontmatter as page properties, so validity-aware
// semantic search sees a curated page's validity and as_of.
func TestAutoIndex_PersistsFrontmatterProperties(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "claim-1",
		"tier: curated\nvalidity: stale-unverified\nas_of: org/repo@abc123\n")

	cfg := curate.StoreConfig{Name: "test-store", Path: storePath}
	if indexed, _ := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger); indexed != 1 {
		t.Fatalf("indexed = %d, want 1", indexed)
	}

	page, err := s.GetPage("knowledge/test-store/claim-1")
	if err != nil || page == nil {
		t.Fatalf("GetPage: page=%v err=%v", page, err)
	}
	validity, asOf := pageValidity(page)
	if validity != "stale-unverified" {
		t.Errorf("validity = %q, want stale-unverified (properties %q)", validity, page.Properties)
	}
	if asOf != "org/repo@abc123" {
		t.Errorf("as_of = %q, want org/repo@abc123", asOf)
	}
	if page.Tier != "curated" {
		t.Errorf("tier = %q, want curated", page.Tier)
	}
}

// TestAutoIndex_UnchangedContentBackfillsProperties verifies that a page
// indexed before properties were persisted (empty Properties, same content
// hash) gets its properties backfilled without re-indexing its blocks.
func TestAutoIndex_UnchangedContentBackfillsProperties(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "claim-1", "tier: curated\nvalidity: superseded\n")

	cfg := curate.StoreConfig{Name: "test-store", Path: storePath}
	if indexed, _ := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger); indexed != 1 {
		t.Fatalf("first indexing = %d, want 1", indexed)
	}

	// Simulate a row written by an older binary that dropped properties.
	old, err := s.GetPage("knowledge/test-store/claim-1")
	if err != nil || old == nil {
		t.Fatalf("GetPage: page=%v err=%v", old, err)
	}
	old.Properties = ""
	if err := s.UpdatePage(old); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	blocksBefore, err := s.GetBlocksByPage("knowledge/test-store/claim-1")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}

	if indexed, _ := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger); indexed != 0 {
		t.Errorf("second indexing = %d, want 0 (content unchanged)", indexed)
	}

	after, err := s.GetPage("knowledge/test-store/claim-1")
	if err != nil || after == nil {
		t.Fatalf("GetPage after: page=%v err=%v", after, err)
	}
	if validity, _ := pageValidity(after); validity != "superseded" {
		t.Errorf("validity after backfill = %q, want superseded (properties %q)", validity, after.Properties)
	}
	blocksAfter, err := s.GetBlocksByPage("knowledge/test-store/claim-1")
	if err != nil {
		t.Fatalf("GetBlocksByPage after: %v", err)
	}
	if len(blocksAfter) != len(blocksBefore) || len(blocksAfter) == 0 ||
		blocksAfter[0].UUID != blocksBefore[0].UUID {
		t.Errorf("blocks changed on unchanged-content backfill: before %d, after %d", len(blocksBefore), len(blocksAfter))
	}
}

// TestIndexKnowledgeStore_PrunesDeletedPage pins CKS-FR-003's "a store file
// removed by a pull disappears after restart" scenario: after a store file is
// deleted, the next IndexKnowledgeStore pass removes that file's
// knowledge/<store>/<name> page, cascades the deletion to its blocks and
// embeddings, and leaves a surviving file's page in place unchanged. The scan
// filters hold (_index.md and dot-files never produce pages), the pruned
// count reports exactly the one deletion, and the indexed count still covers
// only files actually (re)indexed — the unchanged survivor is skipped and a
// pruned page is not a re-index.
func TestIndexKnowledgeStore_PrunesDeletedPage(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "team-kb")
	// Fixture per the CKS-FR-003 scenario: two live files plus the two
	// entries the scan must skip — no page may ever exist for either.
	writeRawKnowledgeFile(t, storePath, "decision-a", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "decision-b", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "_index", "")
	writeRawKnowledgeFile(t, storePath, ".hidden", "")

	cfg := curate.StoreConfig{Name: "team-kb", Path: storePath}
	survivorPage := "knowledge/team-kb/decision-a"
	doomedPage := "knowledge/team-kb/decision-b"

	// The hook embedder (an existing mockEmbedder wrapper whose callback
	// fires from inside EmbedBatch) counts embedding calls so the prune pass
	// can prove it does no embedding work (design D3); its rows power the
	// cascade assertions below (one embedding row per block).
	embedBatches := 0
	embedder := &hookEmbedder{
		mockEmbedder: newMockEmbedder(true),
		onBatch:      func() { embedBatches++ },
	}

	indexed, pruned := IndexKnowledgeStore(s, embedder, cfg, vaultDir, 0, curateLogger)
	if indexed != 2 || pruned != 0 {
		t.Fatalf("first pass = (%d, %d), want (2, 0)", indexed, pruned)
	}
	if embedBatches != 2 {
		t.Fatalf("embed batches after first pass = %d, want 2 (one per page)", embedBatches)
	}

	// Premises: both live pages hold exactly one block and one embedding row
	// each — the state a deletion converges from — and the skipped entries
	// never produced pages.
	for _, name := range []string{survivorPage, doomedPage} {
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", name, err)
		}
		if len(blocks) != 1 {
			t.Fatalf("blocks for %q = %d, want 1", name, len(blocks))
		}
	}
	for _, name := range []string{"knowledge/team-kb/_index", "knowledge/team-kb/.hidden"} {
		page, err := s.GetPage(name)
		if err != nil {
			t.Fatalf("GetPage(%q): %v", name, err)
		}
		if page != nil {
			t.Errorf("page %q must never exist (_index.md and dot-files are skipped by the scan)", name)
		}
	}
	embeddingsBefore, err := s.GetAllEmbeddings(embedder.ModelID())
	if err != nil {
		t.Fatalf("GetAllEmbeddings (before): %v", err)
	}
	if len(embeddingsBefore) != 2 {
		t.Fatalf("embedding rows before = %d, want 2 (one per block)", len(embeddingsBefore))
	}
	survivorBefore, err := s.GetPage(survivorPage)
	if err != nil || survivorBefore == nil {
		t.Fatalf("GetPage(survivor): %v, %v", survivorBefore, err)
	}
	survivorBlocks, err := s.GetBlocksByPage(survivorPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(survivor): %v", err)
	}

	// The deletion — a pulled removal of one store file.
	if err := os.Remove(filepath.Join(storePath, "decision-b.md")); err != nil {
		t.Fatalf("Remove decision-b.md: %v", err)
	}

	indexed, pruned = IndexKnowledgeStore(s, embedder, cfg, vaultDir, 0, curateLogger)
	// The survivor's content is unchanged (skipped) and the deleted file was
	// never scanned, so a pass that pruned one page indexes nothing.
	if indexed != 0 {
		t.Errorf("indexed = %d after the prune pass, want 0 (a pruned page is not a re-index)", indexed)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d after the prune pass, want 1", pruned)
	}

	// The deleted file's page is gone from the store.
	doomed, err := s.GetPage(doomedPage)
	if err != nil {
		t.Fatalf("GetPage(doomed) after prune: %v", err)
	}
	if doomed != nil {
		t.Errorf("page %q still exists after its file was deleted", doomedPage)
	}

	// The cascade reached blocks: zero block rows remain for the deleted page.
	doomedBlocks, err := s.GetBlocksByPage(doomedPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(doomed) after prune: %v", err)
	}
	if len(doomedBlocks) != 0 {
		t.Errorf("blocks for %q = %d after prune, want 0 (DeletePage must cascade)", doomedPage, len(doomedBlocks))
	}

	// The cascade reached embeddings: every remaining row belongs to the
	// survivor's block.
	embeddingsAfter, err := s.GetAllEmbeddings(embedder.ModelID())
	if err != nil {
		t.Fatalf("GetAllEmbeddings (after): %v", err)
	}
	if len(embeddingsAfter) != len(survivorBlocks) {
		t.Errorf("embedding rows after = %d, want %d (the survivor's block only)", len(embeddingsAfter), len(survivorBlocks))
	}
	for _, e := range embeddingsAfter {
		if e.BlockUUID != survivorBlocks[0].UUID {
			t.Errorf("embedding row for block %q lingers after its page was pruned", e.BlockUUID)
		}
	}

	// Design D3: pruning never re-embeds — the embedder saw no new work.
	if embedBatches != 2 {
		t.Errorf("embed batches = %d after the prune pass, want the first-pass 2 (pruning does no embedding work)", embedBatches)
	}

	// The survivor remains and is unchanged: same page row, same block rows.
	survivorAfter, err := s.GetPage(survivorPage)
	if err != nil || survivorAfter == nil {
		t.Fatalf("GetPage(survivor) after prune: %v, %v", survivorAfter, err)
	}
	if survivorAfter.ContentHash != survivorBefore.ContentHash {
		t.Errorf("survivor content hash = %q after prune, want the pre-prune %q", survivorAfter.ContentHash, survivorBefore.ContentHash)
	}
	survivorBlocksAfter, err := s.GetBlocksByPage(survivorPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(survivor) after prune: %v", err)
	}
	if len(survivorBlocksAfter) != len(survivorBlocks) {
		t.Fatalf("survivor block count = %d after prune, want %d", len(survivorBlocksAfter), len(survivorBlocks))
	}
	for i, b := range survivorBlocks {
		if survivorBlocksAfter[i].UUID != b.UUID || survivorBlocksAfter[i].Content != b.Content {
			t.Errorf("survivor block %d changed across the prune pass: (%q, %q) -> (%q, %q); want identical UUID and content",
				i, b.UUID, b.Content, survivorBlocksAfter[i].UUID, survivorBlocksAfter[i].Content)
		}
	}

	// The store's source holds exactly the survivor.
	pages, err := s.ListPagesBySource("knowledge-team-kb")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != survivorPage {
		t.Errorf("store pages after prune = %v, want exactly [%s]", pages, survivorPage)
	}
}

// TestIndexKnowledgeStore_MissingDirPrunesAll pins CKS-FR-003's "a store
// directory removed entirely" scenario: a store still configured whose
// directory is gone from disk counts as an empty file set, so every page of
// that store's source is pruned with its blocks — and the deletion stays
// scoped to that store's source, leaving another store's pages untouched.
func TestIndexKnowledgeStore_MissingDirPrunesAll(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "obsolete")
	writeRawKnowledgeFile(t, storePath, "claim-a", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "claim-b", "tier: curated\n")

	cfg := curate.StoreConfig{Name: "obsolete", Path: storePath}

	indexed, pruned := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger)
	if indexed != 2 || pruned != 0 {
		t.Fatalf("first pass = (%d, %d), want (2, 0)", indexed, pruned)
	}

	// A page from another store's source: the missing-dir prune must stay
	// scoped to the obsolete store's source and leave it untouched.
	otherPage := &store.Page{
		Name:         "knowledge/live-store/claim-x",
		OriginalName: "claim-x",
		SourceID:     "knowledge-live-store",
		SourceDocID:  "knowledge-live-store-claim-x",
		ContentHash:  "deadbeefdeadbeef",
		Tier:         "curated",
	}
	if err := s.InsertPage(otherPage); err != nil {
		t.Fatalf("InsertPage(other): %v", err)
	}

	// The operator deletes the store directory; the config entry remains.
	if err := os.RemoveAll(storePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	indexed, pruned = IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger)
	if indexed != 0 {
		t.Errorf("indexed = %d, want 0 (an empty file set indexes nothing)", indexed)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2 (every page of the store is an orphan)", pruned)
	}

	// Every page of the obsolete store and its blocks are pruned.
	pages, err := s.ListPagesBySource("knowledge-obsolete")
	if err != nil {
		t.Fatalf("ListPagesBySource(knowledge-obsolete): %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("obsolete store pages = %d after the directory vanished, want 0 (all pruned)", len(pages))
	}
	for _, name := range []string{"knowledge/obsolete/claim-a", "knowledge/obsolete/claim-b"} {
		page, err := s.GetPage(name)
		if err != nil {
			t.Fatalf("GetPage(%q): %v", name, err)
		}
		if page != nil {
			t.Errorf("page %q must be pruned when the store directory is gone", name)
		}
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", name, err)
		}
		if len(blocks) != 0 {
			t.Errorf("blocks for %q = %d after prune, want 0", name, len(blocks))
		}
	}

	// The other store's page survived the prune.
	survivor, err := s.GetPage(otherPage.Name)
	if err != nil {
		t.Fatalf("GetPage(other): %v", err)
	}
	if survivor == nil {
		t.Error("page from another store's source must survive the missing-dir prune")
	}
}

// TestIndexKnowledgeStore_TransientScanErrorSkipsPrune pins CKS-FR-003's
// fail-closed guarantee: a store directory that cannot be scanned for any
// reason other than not-exist prunes nothing. IndexKnowledgeStore returns
// counts only (no error, design D4's warn-and-continue shape), so the pass
// completes with (0, 0) after surfacing the scan failure as a warning, and
// every page of the store stays in place. The scan failure is simulated by
// replacing the store directory with a regular file — os.ReadDir on a
// non-directory fails with ENOTDIR, which is not IsNotExist, on every
// supported platform (a chmod 000 directory is not a deterministic failure
// on macOS, where the owning user can still read it).
func TestIndexKnowledgeStore_TransientScanErrorSkipsPrune(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "claim-a", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "claim-b", "tier: curated\n")

	cfg := curate.StoreConfig{Name: "test-store", Path: storePath}

	indexed, pruned := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger)
	if indexed != 2 || pruned != 0 {
		t.Fatalf("first pass = (%d, %d), want (2, 0)", indexed, pruned)
	}

	// Replace the store directory with a regular file so ReadDir fails with
	// a non-not-exist error (ENOTDIR) — the deterministic stand-in for a
	// permissions or I/O failure.
	if err := os.RemoveAll(storePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if err := os.WriteFile(storePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile (store as file): %v", err)
	}

	// A buffer-backed logger captures the warning the fail-closed path must
	// surface (CKS-FR-003: "the scan failure is surfaced").
	var logBuf bytes.Buffer
	scanLogger := log.NewWithOptions(&logBuf, log.Options{ReportTimestamp: false})

	indexed, pruned = IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, scanLogger)
	if indexed != 0 || pruned != 0 {
		t.Errorf("pass over unreadable store dir = (%d, %d), want (0, 0) (fail closed)", indexed, pruned)
	}
	if !strings.Contains(logBuf.String(), "failed to read knowledge store") {
		t.Errorf("log should surface the scan failure, got: %q", logBuf.String())
	}

	// Fail closed: every page of the store and its blocks stay in place.
	pages, err := s.ListPagesBySource("knowledge-test-store")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 2 {
		t.Errorf("store pages = %d after the failed scan, want 2 (a transient fault must not prune)", len(pages))
	}
	for _, name := range []string{"knowledge/test-store/claim-a", "knowledge/test-store/claim-b"} {
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", name, err)
		}
		if len(blocks) != 1 {
			t.Errorf("blocks for %q = %d after the failed scan, want 1", name, len(blocks))
		}
	}
}

// TestIndexKnowledgeStore_ListedButUnreadableFileKeepsPage pins the per-file
// fail-closed ordering inside IndexKnowledgeStore's scan loop: the base name
// is recorded into the prune file set from the directory entry BEFORE the
// os.ReadFile — the loop's first fallible I/O. Because of that ordering, a
// store file that ReadDir lists but ReadFile fails on (dangling symlink,
// transient I/O) still lands in the file set, so its page survives the prune
// even while a true orphan is pruned alongside it — the same invariant
// TestReIngestLearnings_PerFileFailureSkipsPrune pins on the learning path.
// The ordering is load-bearing: moving the recording back below the ReadFile
// error check (its pre-change position) would drop the unreadable file from
// the file set and prune its live page on a transient read fault, silently
// reintroducing the hazard this test exists to catch. The read failure is
// simulated by replacing the file with a dangling symlink — os.ReadDir lists
// a dangling symlink as a non-directory .md entry while os.ReadFile fails on
// it with ENOENT, deterministically on every supported platform (a chmod
// 0000 file is not a deterministic failure on macOS, where the owning user
// can still read it).
func TestIndexKnowledgeStore_ListedButUnreadableFileKeepsPage(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "team-kb")
	writeRawKnowledgeFile(t, storePath, "decision-a", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "decision-b", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "decision-c", "tier: curated\n")

	cfg := curate.StoreConfig{Name: "team-kb", Path: storePath}
	listedPage := "knowledge/team-kb/decision-a" // becomes the dangling symlink
	intactPage := "knowledge/team-kb/decision-b" // stays on disk, unchanged
	orphanPage := "knowledge/team-kb/decision-c" // deleted — the true orphan
	listedFile := filepath.Join(storePath, "decision-a.md")
	orphanFile := filepath.Join(storePath, "decision-c.md")

	indexed, pruned := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger)
	if indexed != 3 || pruned != 0 {
		t.Fatalf("first pass = (%d, %d), want (3, 0)", indexed, pruned)
	}

	// Premise: every page holds exactly one block — the state the fault pass
	// below must leave byte-identical for the listed and intact files and
	// cascade away for the orphan.
	pagesBefore := make(map[string]*store.Page)
	blocksBefore := make(map[string][]*store.Block)
	for _, name := range []string{listedPage, intactPage, orphanPage} {
		page, err := s.GetPage(name)
		if err != nil || page == nil {
			t.Fatalf("GetPage(%q) after first pass: %v, %v", name, page, err)
		}
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q) after first pass: %v", name, err)
		}
		if len(blocks) != 1 {
			t.Fatalf("blocks for %q = %d after first pass, want 1", name, len(blocks))
		}
		pagesBefore[name] = page
		blocksBefore[name] = blocks
	}

	// The fault: decision-a's file becomes a dangling symlink — listed by
	// ReadDir, unreadable by ReadFile — while decision-c leaves the disk
	// entirely (the true orphan proving the pass still prunes).
	if err := os.Remove(listedFile); err != nil {
		t.Fatalf("Remove decision-a.md: %v", err)
	}
	if err := os.Symlink(filepath.Join(vaultDir, "never-created-target.md"), listedFile); err != nil {
		t.Fatalf("Symlink (dangling decision-a): %v", err)
	}
	if err := os.Remove(orphanFile); err != nil {
		t.Fatalf("Remove decision-c.md: %v", err)
	}

	// A buffer-backed logger captures the read-failure warning the scan must
	// surface for the listed-but-unreadable file — the same capture pattern
	// TestIndexKnowledgeStore_TransientScanErrorSkipsPrune uses, with no
	// package-logger mutation.
	var logBuf bytes.Buffer
	passLogger := log.NewWithOptions(&logBuf, log.Options{ReportTimestamp: false})

	indexed, pruned = IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, passLogger)
	// decision-a failed to read (not a re-index), decision-b is unchanged,
	// and decision-c was never scanned, so the pass (re)indexes nothing; the
	// prune deleted exactly the true orphan.
	if indexed != 0 {
		t.Errorf("indexed = %d after the fault pass, want 0 (the unreadable file is not a re-index; the survivor is unchanged)", indexed)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d after the fault pass, want 1 (the true orphan only — successes only)", pruned)
	}

	// assertSurvives pins the fail-closed outcome for a page the pass must
	// not touch: the page row and its block rows byte-identical to the
	// first-pass snapshot.
	assertSurvives := func(name, why string) {
		t.Helper()
		page, err := s.GetPage(name)
		if err != nil {
			t.Fatalf("GetPage(%q) after the fault pass: %v", name, err)
		}
		if page == nil {
			t.Fatalf("page %q must survive the fault pass: %s", name, why)
		}
		if page.ContentHash != pagesBefore[name].ContentHash {
			t.Errorf("content hash for %q = %q, want the first-pass %q",
				name, page.ContentHash, pagesBefore[name].ContentHash)
		}
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q) after the fault pass: %v", name, err)
		}
		if len(blocks) != len(blocksBefore[name]) {
			t.Fatalf("blocks for %q = %d after the fault pass, want the first-pass %d",
				name, len(blocks), len(blocksBefore[name]))
		}
		for i, b := range blocksBefore[name] {
			if blocks[i].UUID != b.UUID || blocks[i].Content != b.Content {
				t.Errorf("block %d for %q changed: (%q, %q) -> (%q, %q); want identical UUID and content",
					i, name, b.UUID, b.Content, blocks[i].UUID, blocks[i].Content)
			}
		}
	}

	// The listed-but-unreadable file's page remains: the directory entry
	// recorded its base name into the file set before the read failed, so
	// the prune skips it — the record-before-read ordering this test pins.
	assertSurvives(listedPage, "the file is listed, so record-before-read keeps it in the prune file set")
	// The intact survivor is untouched.
	assertSurvives(intactPage, "its file is intact and unchanged")

	// The true orphan is pruned — cascade included.
	orphan, err := s.GetPage(orphanPage)
	if err != nil {
		t.Fatalf("GetPage(orphan) after the fault pass: %v", err)
	}
	if orphan != nil {
		t.Errorf("page %q must be pruned after its file was deleted", orphanPage)
	}
	orphanBlocks, err := s.GetBlocksByPage(orphanPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(orphan) after the fault pass: %v", err)
	}
	if len(orphanBlocks) != 0 {
		t.Errorf("blocks for %q = %d after prune, want 0 (DeletePage must cascade)",
			orphanPage, len(orphanBlocks))
	}

	// The store's source holds exactly the two survivors.
	pages, err := s.ListPagesBySource("knowledge-team-kb")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 2 || pages[0].Name != listedPage || pages[1].Name != intactPage {
		t.Errorf("store pages after the fault pass = %v, want exactly [%s, %s]",
			pages, listedPage, intactPage)
	}

	// The read failure is surfaced as a warning naming the file.
	logged := logBuf.String()
	if !strings.Contains(logged, "failed to read knowledge file for indexing") {
		t.Errorf("log should name the per-file read failure, got: %q", logged)
	}
	if !strings.Contains(logged, "decision-a.md") {
		t.Errorf("the read warning should name the unreadable file, got: %q", logged)
	}
	if strings.Contains(logged, "failed to prune knowledge page") {
		t.Errorf("no prune failure may be reported, got: %q", logged)
	}
}

// TestAutoIndex_LogsPrunedCount pins CKS-FR-003's logging contract on the
// background-curation path (design D4): autoIndexKnowledgeStore reports the
// pruned count with an info line when it deleted something, and stays silent
// when a pass deletes nothing.
func TestAutoIndex_LogsPrunedCount(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "test-store")
	writeRawKnowledgeFile(t, storePath, "auth-1", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "vault-2", "tier: curated\n")

	cfg := curate.StoreConfig{Name: "test-store", Sources: []string{"disk-local"}, Path: storePath}
	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 2 {
		t.Fatalf("first indexing = %d, want 2", indexed)
	}

	// The deletion — one store file removed.
	if err := os.Remove(filepath.Join(storePath, "vault-2.md")); err != nil {
		t.Fatalf("Remove vault-2.md: %v", err)
	}

	// Capture the package logger the auto-index path writes to; only the
	// prune pass writes to this buffer. The capture mutates the
	// package-global logger — the reason these tests must stay serial
	// (this file uses no t.Parallel()) and why every capture defers
	// restore.
	var logBuf bytes.Buffer
	curateLogger.SetOutput(&logBuf)
	defer curateLogger.SetOutput(os.Stderr)

	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 0 {
		t.Errorf("second indexing = %d, want 0 (survivor unchanged, a pruned page is not an index)", indexed)
	}

	// The pruned page is really gone — the log line reports a deletion that
	// happened.
	page, err := s.GetPage("knowledge/test-store/vault-2")
	if err != nil || page != nil {
		t.Errorf("page knowledge/test-store/vault-2 = %v, %v after prune, want gone", page, err)
	}
	if !strings.Contains(logBuf.String(), "pruned knowledge store pages") {
		t.Errorf("log should report the prune count, got: %q", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "count=1") {
		t.Errorf("prune log line should carry the count, got: %q", logBuf.String())
	}

	// Nothing is deleted when nothing was deleted (CKS-FR-003): a pass over
	// intact files emits no pruning log line.
	var quietBuf bytes.Buffer
	curateLogger.SetOutput(&quietBuf)
	if indexed := curateTool.autoIndexKnowledgeStore(cfg); indexed != 0 {
		t.Errorf("third indexing = %d, want 0 (files unchanged)", indexed)
	}
	if strings.Contains(quietBuf.String(), "pruned knowledge store pages") {
		t.Errorf("no prune log line may be emitted when no file was deleted, got: %q", quietBuf.String())
	}
}

// TestPruneOrphanedKnowledgePages_MidPruneDeleteFailureKeepsFailedPage pins
// CKS-FR-003's "a deletion failure mid-prune keeps the failed page" scenario:
// when one orphan's DeletePage fails with a store error, the failed page stays
// in place for the next indexing pass to retry while the remaining orphan is
// still deleted, the failure is logged as a warning naming the page, and the
// helper's returned count includes successful deletions only — this helper
// returns the count rather than logging it (design D4), so the return value
// carries the assertion. The failure is injected at the database level — a
// scoped SQLite trigger that aborts the delete of exactly one page — so the
// test exercises the production warn-and-continue path rather than mocking
// the store.
func TestPruneOrphanedKnowledgePages_MidPruneDeleteFailureKeepsFailedPage(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	storePath := filepath.Join(vaultDir, ".uf", "dewey", "knowledge", "team-kb")
	writeRawKnowledgeFile(t, storePath, "decision-a", "tier: curated\n")
	writeRawKnowledgeFile(t, storePath, "decision-b", "tier: curated\n")

	cfg := curate.StoreConfig{Name: "team-kb", Path: storePath}
	failedPage := "knowledge/team-kb/decision-a"
	doomedPage := "knowledge/team-kb/decision-b"

	indexed, pruned := IndexKnowledgeStore(s, nil, cfg, vaultDir, 0, curateLogger)
	if indexed != 2 || pruned != 0 {
		t.Fatalf("first pass = (%d, %d), want (2, 0)", indexed, pruned)
	}

	// Premise: both pages hold exactly one block — the state whose survival
	// (decision-a) and cascade (decision-b) the prune below must produce.
	// ListPagesBySource orders by name, so decision-a is pruned first: the
	// trigger below fails the FIRST deletion, exactly the spec scenario's
	// shape.
	failedBlocks, err := s.GetBlocksByPage(failedPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(decision-a): %v", err)
	}
	if len(failedBlocks) != 1 {
		t.Fatalf("decision-a block count = %d, want 1", len(failedBlocks))
	}
	doomedBlocks, err := s.GetBlocksByPage(doomedPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(decision-b): %v", err)
	}
	if len(doomedBlocks) != 1 {
		t.Fatalf("decision-b block count = %d, want 1", len(doomedBlocks))
	}

	// Both files leave the disk, so every page of the store is an orphan.
	for _, name := range []string{"decision-a.md", "decision-b.md"} {
		if err := os.Remove(filepath.Join(storePath, name)); err != nil {
			t.Fatalf("Remove %s: %v", name, err)
		}
	}

	// Fault injection: a BEFORE DELETE trigger makes exactly decision-a's
	// page delete fail at the database level (store.Store exposes the raw
	// handle through DB()), so DeletePage returns an error and the prune
	// helper takes its warn-and-continue path. The abort fires before the
	// row is removed, so the blocks cascade never runs either. The trigger
	// is scoped to the one page and dropped afterward so the store stays
	// usable for the assertions and the t.Cleanup close.
	if _, err := s.DB().Exec(`CREATE TRIGGER fail_delete_decision_a BEFORE DELETE ON pages
WHEN OLD.name = 'knowledge/team-kb/decision-a'
BEGIN
	SELECT RAISE(ABORT, 'injected delete failure');
END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	defer func() {
		if _, err := s.DB().Exec(`DROP TRIGGER IF EXISTS fail_delete_decision_a`); err != nil {
			t.Errorf("drop trigger: %v", err)
		}
	}()

	// A buffer-backed logger makes the warnings capturable without touching
	// the package logger, the same capture pattern
	// TestIndexKnowledgeStore_TransientScanErrorSkipsPrune uses.
	var logBuf bytes.Buffer
	pruneLogger := log.NewWithOptions(&logBuf, log.Options{ReportTimestamp: false})

	// The prune helper runs directly with a nil file set (both files are
	// gone): both pages are orphans, decision-a's delete fails,
	// decision-b's succeeds.
	prunedCount := pruneOrphanedKnowledgePages(s, cfg, nil, pruneLogger)
	if prunedCount != 1 {
		t.Errorf("pruneOrphanedKnowledgePages = %d, want 1 (successes only; the failed delete is not counted)", prunedCount)
	}

	// The failed page remains — it is retried on the next indexing pass.
	failed, err := s.GetPage(failedPage)
	if err != nil {
		t.Fatalf("GetPage(decision-a) after prune: %v", err)
	}
	if failed == nil {
		t.Error("the failed page must remain in the store after its delete failed")
	}

	// Its blocks remain with it, byte-identical.
	failedBlocksAfter, err := s.GetBlocksByPage(failedPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(decision-a) after prune: %v", err)
	}
	if len(failedBlocksAfter) != len(failedBlocks) {
		t.Fatalf("failed page block count = %d after prune, want the pre-prune %d",
			len(failedBlocksAfter), len(failedBlocks))
	}
	for i, b := range failedBlocks {
		if failedBlocksAfter[i].UUID != b.UUID || failedBlocksAfter[i].Content != b.Content {
			t.Errorf("failed page block %d changed: (%q, %q) -> (%q, %q); want identical UUID and content",
				i, b.UUID, b.Content, failedBlocksAfter[i].UUID, failedBlocksAfter[i].Content)
		}
	}

	// The remaining orphan is still deleted — cascade included.
	doomed, err := s.GetPage(doomedPage)
	if err != nil {
		t.Fatalf("GetPage(decision-b) after prune: %v", err)
	}
	if doomed != nil {
		t.Errorf("page %q must still be pruned when the sibling delete failed", doomedPage)
	}
	doomedBlocksAfter, err := s.GetBlocksByPage(doomedPage)
	if err != nil {
		t.Fatalf("GetBlocksByPage(decision-b) after prune: %v", err)
	}
	if len(doomedBlocksAfter) != 0 {
		t.Errorf("blocks for %q = %d after prune, want 0 (the remaining orphan's cascade is not blocked)",
			doomedPage, len(doomedBlocksAfter))
	}

	// The failure is logged as a warning naming the failed page.
	logged := logBuf.String()
	if !strings.Contains(logged, "failed to prune knowledge page") {
		t.Errorf("log should warn about the failed prune, got: %q", logged)
	}
	if !strings.Contains(logged, failedPage) {
		t.Errorf("the prune warning should name the failed page %q, got: %q", failedPage, logged)
	}
}
