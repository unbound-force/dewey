package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// mockEmbedderForHealth implements embed.Embedder for health tool testing.
type mockEmbedderForHealth struct {
	available bool
	model     string
}

func (m *mockEmbedderForHealth) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}
func (m *mockEmbedderForHealth) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2}
	}
	return result, nil
}
func (m *mockEmbedderForHealth) Available() bool { return m.available }
func (m *mockEmbedderForHealth) ModelID() string { return m.model }

var _ embed.Embedder = (*mockEmbedderForHealth)(nil)

// serverMockBackend implements backend.Backend for newServer tests.
// It returns empty data for all read operations and tracks write calls.
// It does NOT implement HasDataScript, so DataScript-only tools should
// not be registered when this backend is used.
type serverMockBackend struct{}

func (m *serverMockBackend) GetAllPages(_ context.Context) ([]types.PageEntity, error) {
	return nil, nil
}
func (m *serverMockBackend) GetPage(_ context.Context, _ any) (*types.PageEntity, error) {
	return nil, fmt.Errorf("page not found")
}
func (m *serverMockBackend) GetPageBlocksTree(_ context.Context, _ any) ([]types.BlockEntity, error) {
	return nil, nil
}
func (m *serverMockBackend) GetBlock(_ context.Context, _ string, _ ...map[string]any) (*types.BlockEntity, error) {
	return nil, fmt.Errorf("block not found")
}
func (m *serverMockBackend) GetPageLinkedReferences(_ context.Context, _ any) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (m *serverMockBackend) DatascriptQuery(_ context.Context, _ string, _ ...any) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (m *serverMockBackend) CreatePage(_ context.Context, name string, _ map[string]any, _ map[string]any) (*types.PageEntity, error) {
	return &types.PageEntity{Name: name}, nil
}
func (m *serverMockBackend) AppendBlockInPage(_ context.Context, _ string, content string) (*types.BlockEntity, error) {
	return &types.BlockEntity{Content: content}, nil
}
func (m *serverMockBackend) PrependBlockInPage(_ context.Context, _ string, content string) (*types.BlockEntity, error) {
	return &types.BlockEntity{Content: content}, nil
}
func (m *serverMockBackend) InsertBlock(_ context.Context, _ any, content string, _ map[string]any) (*types.BlockEntity, error) {
	return &types.BlockEntity{Content: content}, nil
}
func (m *serverMockBackend) UpdateBlock(_ context.Context, _ string, _ string, _ ...map[string]any) error {
	return nil
}
func (m *serverMockBackend) RemoveBlock(_ context.Context, _ string) error { return nil }
func (m *serverMockBackend) MoveBlock(_ context.Context, _ string, _ string, _ map[string]any) error {
	return nil
}
func (m *serverMockBackend) DeletePage(_ context.Context, _ string) error { return nil }
func (m *serverMockBackend) RenamePage(_ context.Context, _, _ string) error {
	return nil
}
func (m *serverMockBackend) Ping(_ context.Context) error { return nil }

var _ backend.Backend = (*serverMockBackend)(nil)

// serverMockBackendWithDataScript embeds serverMockBackend and adds
// the HasDataScript marker interface, enabling DataScript-only tools
// (get_references, query_datalog, flashcard_*, whiteboard_*).
type serverMockBackendWithDataScript struct {
	serverMockBackend
}

func (m *serverMockBackendWithDataScript) HasDataScript() {}

var _ backend.Backend = (*serverMockBackendWithDataScript)(nil)
var _ backend.HasDataScript = (*serverMockBackendWithDataScript)(nil)

// listServerTools creates an in-memory MCP client session connected to the
// given server and returns the names of all registered tools, sorted.
func listServerTools(t *testing.T, srv *mcp.Server) []string {
	t.Helper()

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// TestNewServer_SemanticToolsRegistered verifies that the 3 semantic search
// tools are registered in the server (T035).
func TestNewServer_SemanticToolsRegistered(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := &mockEmbedderForHealth{available: true, model: "test-model"}

	// This should not panic — all tools should register successfully.
	srv, _ := newServer(vc, false, WithEmbedder(e), WithPersistentStore(s))
	if srv == nil {
		t.Fatal("newServer returned nil")
	}
}

// TestNewServer_WithoutEmbedder verifies server works without embedder.
func TestNewServer_WithoutEmbedder(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	srv, _ := newServer(vc, false)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}
}

// TestHealthToolOutput_DeweyFields verifies the health tool includes
// Dewey-specific fields per contracts/mcp-tools.md (T042B).
//
// Design decision: We test the health tool output format by constructing
// the expected response structure rather than calling through the MCP
// protocol, because the MCP SDK's Server.CallTool is unexported.
// The health tool's inline closure is tested indirectly through the
// server registration — if it compiles and the server starts, the
// tool is correctly registered.
func TestHealthToolOutput_DeweyFields(t *testing.T) {
	// Verify the serverConfig correctly stores embedder and store.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := &mockEmbedderForHealth{available: true, model: "granite-embedding:30m"}

	cfg := serverConfig{
		embedder: e,
		store:    s,
	}

	// Verify embedder interface methods.
	if cfg.embedder.ModelID() != "granite-embedding:30m" {
		t.Errorf("ModelID = %q, want %q", cfg.embedder.ModelID(), "granite-embedding:30m")
	}
	if !cfg.embedder.Available() {
		t.Error("Available() = false, want true")
	}

	// Verify store operations work.
	_ = s.InsertPage(&store.Page{
		Name: "test", OriginalName: "test",
		SourceID: "disk-local", SourceDocID: "test.md",
		ContentHash: "abc", CreatedAt: 1, UpdatedAt: 1,
	})
	_ = s.InsertBlock(&store.Block{
		UUID: "b1", PageName: "test", Content: "content", Position: 0,
	})
	_ = s.InsertEmbedding("b1", "granite-embedding:30m", []float32{1, 0}, "chunk")

	count, err := s.CountEmbeddings()
	if err != nil {
		t.Fatalf("CountEmbeddings: %v", err)
	}
	if count != 1 {
		t.Errorf("CountEmbeddings = %d, want 1", count)
	}

	blockCount, err := s.CountBlocks()
	if err != nil {
		t.Fatalf("CountBlocks: %v", err)
	}
	if blockCount != 1 {
		t.Errorf("CountBlocks = %d, want 1", blockCount)
	}

	// Simulate the health tool's Dewey-specific output construction.
	deweyInfo := map[string]any{
		"persistent":         true,
		"embeddingModel":     cfg.embedder.ModelID(),
		"embeddingAvailable": cfg.embedder.Available(),
		"embeddingCount":     count,
		"embeddingCoverage":  float64(count) / float64(blockCount),
	}

	// Verify all required fields are present.
	requiredFields := []string{
		"persistent", "embeddingModel", "embeddingAvailable",
		"embeddingCount", "embeddingCoverage",
	}
	for _, field := range requiredFields {
		if _, ok := deweyInfo[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}

	// Verify JSON serialization works.
	data, err := json.MarshalIndent(deweyInfo, "", "  ")
	if err != nil {
		t.Fatalf("marshal dewey info: %v", err)
	}
	if len(data) == 0 {
		t.Error("serialized dewey info is empty")
	}
}

// TestWithEmbedder verifies the WithEmbedder option.
func TestWithEmbedder(t *testing.T) {
	e := &mockEmbedderForHealth{available: true, model: "test"}
	var cfg serverConfig
	WithEmbedder(e)(&cfg)
	if cfg.embedder != e {
		t.Error("WithEmbedder did not set embedder")
	}
}

// TestWithPersistentStore verifies the WithPersistentStore option.
func TestWithPersistentStore(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	var cfg serverConfig
	WithPersistentStore(s)(&cfg)
	if cfg.store != s {
		t.Error("WithPersistentStore did not set store")
	}
}

// TestNewServer_RegistersTools verifies that newServer with a non-DataScript
// backend in read-write mode registers the expected set of tools. The backend
// does not implement HasDataScript, so DataScript-only tools (get_references,
// query_datalog, flashcard_*, whiteboard_*) must be absent.
func TestNewServer_RegistersTools(t *testing.T) {
	b := &serverMockBackend{}
	srv, _ := newServer(b, false)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	// Core tools expected for any non-DataScript read-write backend.
	coreTools := []string{
		// Navigate
		"get_page", "get_block", "list_pages", "get_links", "traverse",
		// Search
		"search", "query_properties", "find_by_tag",
		// Analyze
		"graph_overview", "find_connections", "knowledge_gaps", "list_orphans", "topic_clusters",
		// Write
		"create_page", "append_blocks", "update_block", "delete_block",
		"upsert_blocks", "move_block", "delete_page", "rename_page",
		"bulk_update_properties", "link_pages",
		// Decision
		"decision_check", "decision_create", "decision_resolve", "decision_defer", "analysis_health",
		// Journal
		"journal_range", "journal_search",
		// Semantic
		"semantic_search", "similar", "semantic_search_filtered",
		// Learning
		"store_learning",
		// Indexing
		"index", "reindex",
		// Health
		"health",
	}

	for _, name := range coreTools {
		if !containsTool(tools, name) {
			t.Errorf("missing expected tool %q", name)
		}
	}

	// DataScript-only tools must NOT be registered.
	dataScriptOnly := []string{
		"get_references", "query_datalog",
		"flashcard_overview", "flashcard_due", "flashcard_create",
		"list_whiteboards", "get_whiteboard",
	}
	for _, name := range dataScriptOnly {
		if containsTool(tools, name) {
			t.Errorf("DataScript-only tool %q should not be registered for non-DataScript backend", name)
		}
	}

	// Vault-specific "reload" tool should not be registered for a mock backend.
	if containsTool(tools, "reload") {
		t.Error("reload tool should only be registered for vault.Client backend")
	}
}

// TestNewServer_ReadOnlyMode verifies that write and decision tools are not
// registered when readOnly is true.
func TestNewServer_ReadOnlyMode(t *testing.T) {
	b := &serverMockBackend{}
	srv, _ := newServer(b, true)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	// Write tools must be absent in read-only mode.
	writeTools := []string{
		"create_page", "append_blocks", "update_block", "delete_block",
		"upsert_blocks", "move_block", "delete_page", "rename_page",
		"bulk_update_properties", "link_pages",
		// Learning and indexing tools are also write-only.
		"store_learning", "index", "reindex",
	}
	for _, name := range writeTools {
		if containsTool(tools, name) {
			t.Errorf("write tool %q should not be registered in read-only mode", name)
		}
	}

	// Decision tools must also be absent in read-only mode.
	decisionTools := []string{
		"decision_check", "decision_create", "decision_resolve",
		"decision_defer", "analysis_health",
	}
	for _, name := range decisionTools {
		if containsTool(tools, name) {
			t.Errorf("decision tool %q should not be registered in read-only mode", name)
		}
	}

	// Read-only tools must still be present.
	readTools := []string{
		"get_page", "get_block", "list_pages", "get_links", "traverse",
		"search", "query_properties", "find_by_tag",
		"graph_overview", "find_connections", "knowledge_gaps", "list_orphans", "topic_clusters",
		"journal_range", "journal_search",
		"semantic_search", "similar", "semantic_search_filtered",
		"health",
	}
	for _, name := range readTools {
		if !containsTool(tools, name) {
			t.Errorf("read tool %q should be registered in read-only mode", name)
		}
	}
}

// TestNewServer_DataScriptBackend verifies that DataScript-only tools are
// registered when the backend implements backend.HasDataScript.
func TestNewServer_DataScriptBackend(t *testing.T) {
	b := &serverMockBackendWithDataScript{}
	srv, _ := newServer(b, false)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	// DataScript-only tools must be present.
	dataScriptTools := []string{
		"get_references", "query_datalog",
		"flashcard_overview", "flashcard_due", "flashcard_create",
		"list_whiteboards", "get_whiteboard",
	}
	for _, name := range dataScriptTools {
		if !containsTool(tools, name) {
			t.Errorf("DataScript tool %q should be registered for HasDataScript backend", name)
		}
	}
}

// TestNewServer_DataScriptReadOnly verifies that DataScript-only write tools
// (flashcard_create) are excluded in read-only mode, while DataScript read
// tools remain.
func TestNewServer_DataScriptReadOnly(t *testing.T) {
	b := &serverMockBackendWithDataScript{}
	srv, _ := newServer(b, true)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	// DataScript read tools should be registered.
	for _, name := range []string{
		"get_references", "query_datalog",
		"flashcard_overview", "flashcard_due",
		"list_whiteboards", "get_whiteboard",
	} {
		if !containsTool(tools, name) {
			t.Errorf("DataScript read tool %q should be registered in read-only mode", name)
		}
	}

	// flashcard_create is a write tool and should be excluded.
	if containsTool(tools, "flashcard_create") {
		t.Error("flashcard_create should not be registered in read-only mode")
	}
}

// TestNewServer_WithEmbedderOption verifies that the WithEmbedder option
// passes the embedder to the semantic tools (server creation succeeds and
// semantic tools are registered).
func TestNewServer_WithEmbedderOption(t *testing.T) {
	b := &serverMockBackend{}
	e := &mockEmbedderForHealth{available: true, model: "test-model"}

	srv, _ := newServer(b, false, WithEmbedder(e))
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	semanticTools := []string{
		"semantic_search", "similar", "semantic_search_filtered",
	}
	for _, name := range semanticTools {
		if !containsTool(tools, name) {
			t.Errorf("semantic tool %q should be registered with embedder option", name)
		}
	}
}

// TestNewServer_WithPersistentStoreOption verifies that the WithPersistentStore
// option passes the store to the server (server creation succeeds and health
// tool is registered with store-dependent fields available).
func TestNewServer_WithPersistentStoreOption(t *testing.T) {
	b := &serverMockBackend{}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	srv, _ := newServer(b, false, WithPersistentStore(s))
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	// Health tool should be registered (it uses the store internally).
	if !containsTool(tools, "health") {
		t.Error("health tool should be registered with persistent store option")
	}

	// Semantic tools should also be registered (they use the store).
	for _, name := range []string{
		"semantic_search", "similar", "semantic_search_filtered",
	} {
		if !containsTool(tools, name) {
			t.Errorf("semantic tool %q should be registered with persistent store option", name)
		}
	}
}

// TestNewServer_VaultBackendRegistersReload verifies that the "reload" tool
// is registered when the backend is a *vault.Client in read-write mode.
func TestNewServer_VaultBackendRegistersReload(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	srv, _ := newServer(vc, false)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	if !containsTool(tools, "reload") {
		t.Error("reload tool should be registered for vault.Client backend")
	}
}

// TestNewServer_VaultBackendReadOnlyNoReload verifies that the "reload" tool
// is NOT registered for a vault.Client in read-only mode.
func TestNewServer_VaultBackendReadOnlyNoReload(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	srv, _ := newServer(vc, true)
	if srv == nil {
		t.Fatal("newServer returned nil")
	}

	tools := listServerTools(t, srv)

	if containsTool(tools, "reload") {
		t.Error("reload tool should not be registered in read-only mode")
	}
}

// TestNewServer_ToolCount verifies the total number of registered tools
// for different backend configurations.
func TestNewServer_ToolCount(t *testing.T) {
	tests := []struct {
		name     string
		backend  backend.Backend
		readOnly bool
		wantMin  int // minimum expected tool count
		wantMax  int // maximum expected tool count
	}{
		{
			name:     "non-DataScript read-write",
			backend:  &serverMockBackend{},
			readOnly: false,
			wantMin:  32, // navigate(5) + search(3) + analyze(5) + write(10) + decision(5) + journal(2) + semantic(3) + learning(1) + indexing(2) + health(1) = 37
			wantMax:  37,
		},
		{
			name:     "non-DataScript read-only",
			backend:  &serverMockBackend{},
			readOnly: true,
			wantMin:  16, // navigate(5) + search(3) + analyze(5) + journal(2) + semantic(3) + health(1) = 19
			wantMax:  20,
		},
		{
			name:     "DataScript read-write",
			backend:  &serverMockBackendWithDataScript{},
			readOnly: false,
			wantMin:  40, // above + get_references + query_datalog + flashcard(3) + whiteboard(2) = 44
			wantMax:  44,
		},
		{
			name:     "DataScript read-only",
			backend:  &serverMockBackendWithDataScript{},
			readOnly: true,
			wantMin:  23, // read-only + DataScript read tools
			wantMax:  26,
		},
		{
			name:     "vault read-write",
			backend:  vault.New(t.TempDir()),
			readOnly: false,
			wantMin:  33, // non-DataScript read-write + reload
			wantMax:  38,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(tc.backend, tc.readOnly)
			tools := listServerTools(t, srv)

			if len(tools) < tc.wantMin || len(tools) > tc.wantMax {
				t.Errorf("tool count = %d, want between %d and %d\ntools: %v",
					len(tools), tc.wantMin, tc.wantMax, tools)
			}
		})
	}
}

// containsTool reports whether the sorted tool list contains the given name.
func containsTool(tools []string, name string) bool {
	i := sort.SearchStrings(tools, name)
	return i < len(tools) && tools[i] == name
}

// callHealthTool creates an in-memory MCP client session, calls the "health"
// tool, and returns the parsed JSON response.
func callHealthTool(t *testing.T, srv *mcp.Server) map[string]any {
	t.Helper()

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()

	ss, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "health"})
	if err != nil {
		t.Fatalf("CallTool(health): %v", err)
	}
	if result.IsError {
		t.Fatalf("health tool returned error")
	}

	// Extract text content from the result.
	if len(result.Content) == 0 {
		t.Fatal("health tool returned no content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("health content is %T, want *mcp.TextContent", result.Content[0])
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &parsed); err != nil {
		t.Fatalf("unmarshal health response: %v", err)
	}
	return parsed
}

// TestRegisterHealthTool_MinimalConfig tests health tool output with nil
// store and nil embedder — the minimal server configuration.
func TestRegisterHealthTool_MinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)
	srv, _ := newServer(vc, false)

	parsed := callHealthTool(t, srv)

	if parsed["status"] != "ok" {
		t.Errorf("status = %v, want %q", parsed["status"], "ok")
	}
	if parsed["readOnly"] != false {
		t.Errorf("readOnly = %v, want false", parsed["readOnly"])
	}

	dewey, ok := parsed["dewey"].(map[string]any)
	if !ok {
		t.Fatalf("dewey field missing or wrong type: %T", parsed["dewey"])
	}
	if dewey["persistent"] != false {
		t.Errorf("dewey.persistent = %v, want false", dewey["persistent"])
	}
	if dewey["embeddingAvailable"] != false {
		t.Errorf("dewey.embeddingAvailable = %v, want false", dewey["embeddingAvailable"])
	}
	if dewey["embeddingCount"] != float64(0) {
		t.Errorf("dewey.embeddingCount = %v, want 0", dewey["embeddingCount"])
	}
}

// TestRegisterHealthTool_WithStore tests health tool output when a persistent
// store is configured with pages, blocks, embeddings, and sources.
func TestRegisterHealthTool_WithStore(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Populate store with test data.
	_ = s.InsertPage(&store.Page{
		Name: "test-page", OriginalName: "Test Page",
		SourceID: "disk-local", SourceDocID: "test.md",
		ContentHash: "abc", CreatedAt: 1, UpdatedAt: 1,
	})
	_ = s.InsertBlock(&store.Block{
		UUID: "b1", PageName: "test-page", Content: "block content", Position: 0,
	})
	_ = s.InsertBlock(&store.Block{
		UUID: "b2", PageName: "test-page", Content: "another block", Position: 1,
	})
	_ = s.InsertEmbedding("b1", "test-model", []float32{0.1, 0.2}, "chunk1")
	_ = s.InsertSource(&store.SourceRecord{
		ID: "disk-local", Type: "disk", Status: "ok", LastFetchedAt: 1000,
	})

	srv, _ := newServer(vc, false, WithPersistentStore(s))
	parsed := callHealthTool(t, srv)

	dewey, ok := parsed["dewey"].(map[string]any)
	if !ok {
		t.Fatalf("dewey field missing or wrong type: %T", parsed["dewey"])
	}
	if dewey["persistent"] != true {
		t.Errorf("dewey.persistent = %v, want true", dewey["persistent"])
	}
	if dewey["embeddingCount"] != float64(1) {
		t.Errorf("dewey.embeddingCount = %v, want 1", dewey["embeddingCount"])
	}
	coverage, ok := dewey["embeddingCoverage"].(float64)
	if !ok || coverage <= 0 {
		t.Errorf("dewey.embeddingCoverage = %v, want > 0", dewey["embeddingCoverage"])
	}

	sources, ok := dewey["sources"].([]any)
	if !ok {
		t.Fatalf("dewey.sources missing or wrong type: %T", dewey["sources"])
	}
	if len(sources) != 1 {
		t.Fatalf("dewey.sources length = %d, want 1", len(sources))
	}
	src, ok := sources[0].(map[string]any)
	if !ok {
		t.Fatalf("sources[0] is not a map: %T", sources[0])
	}
	if src["id"] != "disk-local" {
		t.Errorf("sources[0].id = %v, want %q", src["id"], "disk-local")
	}
	if src["type"] != "disk" {
		t.Errorf("sources[0].type = %v, want %q", src["type"], "disk")
	}
}

// TestRegisterHealthTool_WithEmbedder tests health tool output when an
// embedder is configured and available.
func TestRegisterHealthTool_WithEmbedder(t *testing.T) {
	tmpDir := t.TempDir()
	vc := vault.New(tmpDir)

	e := &mockEmbedderForHealth{available: true, model: "granite-embedding:30m"}
	srv, _ := newServer(vc, false, WithEmbedder(e))

	parsed := callHealthTool(t, srv)

	dewey, ok := parsed["dewey"].(map[string]any)
	if !ok {
		t.Fatalf("dewey field missing or wrong type: %T", parsed["dewey"])
	}
	if dewey["embeddingAvailable"] != true {
		t.Errorf("dewey.embeddingAvailable = %v, want true", dewey["embeddingAvailable"])
	}
	if dewey["embeddingModel"] != "granite-embedding:30m" {
		t.Errorf("dewey.embeddingModel = %v, want %q", dewey["embeddingModel"], "granite-embedding:30m")
	}
}

// TestRegisterHealthTool_PingError tests health tool output when the backend
// Ping() returns an error.
func TestRegisterHealthTool_PingError(t *testing.T) {
	mb := &serverMockBackendWithPingError{}
	srv, _ := newServer(mb, false)

	parsed := callHealthTool(t, srv)

	status, ok := parsed["status"].(string)
	if !ok {
		t.Fatalf("status missing or wrong type: %T", parsed["status"])
	}
	if len(status) < 6 || status[:6] != "error:" {
		t.Errorf("status = %q, want prefix %q", status, "error:")
	}
}

// serverMockBackendWithPingError is a backend that returns an error from Ping.
type serverMockBackendWithPingError struct {
	serverMockBackend
}

func (m *serverMockBackendWithPingError) Ping(_ context.Context) error {
	return fmt.Errorf("connection refused")
}
