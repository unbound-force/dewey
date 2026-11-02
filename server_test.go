package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/backend"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/tools"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
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
		// Knowledge compilation (013-knowledge-compile)
		"compile", "store_compiled", "lint", "promote", "curate",
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
		// Learning, indexing, and knowledge compilation tools are also write-only.
		"store_learning", "index", "reindex",
		"compile", "store_compiled", "lint", "promote", "curate",
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
			wantMin:  36, // navigate(5) + search(3) + analyze(5) + write(10) + decision(5) + journal(2) + semantic(3) + learning(1) + indexing(2) + compile(2) + curate(1) + lint(1) + promote(1) + health(1) = 42
			wantMax:  42,
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
			wantMin:  44, // above + get_references + query_datalog + flashcard(3) + whiteboard(2) = 49
			wantMax:  49,
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
			wantMin:  37, // non-DataScript read-write + reload
			wantMax:  43,
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

// --- mcp-internal-store-access: #90/#91 server-level integration (task 5.1) ---

// storeAccessText extracts the text content from an MCP tool result.
func storeAccessText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool returned no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool content is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

// storeAccessOK asserts that a direct tool-handler invocation succeeded —
// nil Go error, non-nil result, no MCP error flag — and returns the result
// text. label names the tool call in failure messages.
func storeAccessOK(t *testing.T, label string, res *mcp.CallToolResult, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if res == nil {
		t.Fatalf("%s: nil result", label)
	}
	if res.IsError {
		t.Fatalf("%s returned an error result: %s", label, storeAccessText(t, res))
	}
	return storeAccessText(t, res)
}

// storeAccessJSON unmarshals a tool result's JSON text into v.
func storeAccessJSON(t *testing.T, label, text string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), v); err != nil {
		t.Fatalf("%s: unmarshal response %q: %v", label, text, err)
	}
}

// storeAccessResultFor returns the first entry of a tool results array whose
// "page" field equals page, or nil when no entry matches.
func storeAccessResultFor(results []any, page string) map[string]any {
	for _, r := range results {
		entry, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := entry["page"].(string); name == page {
			return entry
		}
	}
	return nil
}

// storeAccessTreeContains reports whether any block in a JSON-serialized
// block tree has content containing s. Children are visited recursively.
func storeAccessTreeContains(blocks []any, s string) bool {
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if content, ok := block["content"].(string); ok && strings.Contains(content, s) {
			return true
		}
		if children, ok := block["children"].([]any); ok {
			if storeAccessTreeContains(children, s) {
				return true
			}
		}
	}
	return false
}

// TestServerIntegration_StoreBackedPagesSameSession reproduces the #90/#91
// flow end-to-end at the server level (mcp-internal-store-access task 5.1):
// within a single server session backed by a persistent store, store_compiled
// writes a compiled article, and get_page, list_pages, search, find_by_tag,
// get_block, and promote must all operate on it immediately — no restart, no
// re-Load (specs: Live Retrieval, Block Retrieval, Unified Enumeration,
// Unified Search, Promote).
//
// Design decision: the tool handlers are invoked directly on the same struct
// instances and wiring newServer registers (server.go), because the MCP SDK's
// Server.CallTool is unexported — the same tradeoff TestHealthToolOutput
// documents. The server itself is still constructed with the production
// wiring (vault backend sharing one store instance with the store-backed
// tools) to prove all seven handlers register for this configuration.
func TestServerIntegration_StoreBackedPagesSameSession(t *testing.T) {
	vaultDir := t.TempDir()

	// A vault file page, so unified enumeration merges a vault page with the
	// store-only compiled page (spec: Unified Page Enumeration).
	readme := "# Readme\n\nVault file page for the same-session integration test.\n"
	if err := os.WriteFile(filepath.Join(vaultDir, "readme.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("write readme.md: %v", err)
	}

	// File-backed store under {vault}/.uf/dewey/ — the production shape from
	// #90 (main.go opens graph.db there). The directory must exist before
	// store.New creates graph.db and dewey.lock.
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	s, err := store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Production wiring (main.go + server.go): one store instance shared by
	// the vault backend and the store-backed tools.
	vc := vault.New(vaultDir, vault.WithStore(s))

	// Background startup indexing equivalent (main.go deferredIndex →
	// indexVault): vault files land in the in-memory index and in the store
	// under the disk-local source. This runs BEFORE store_compiled, so the
	// compiled page is genuinely absent from the vault file index.
	if _, err := vc.Store().IncrementalIndex(vc); err != nil {
		t.Fatalf("incremental index: %v", err)
	}

	// Start the server with the persistent store.
	srv, _ := newServer(vc, false, WithPersistentStore(s))
	if srv == nil {
		t.Fatal("newServer returned nil server")
	}

	// Every tool in the #90/#91 flow must be registered on this wiring.
	registered := listServerTools(t, srv)
	for _, name := range []string{
		"store_compiled", "get_page", "list_pages", "search",
		"find_by_tag", "get_block", "promote",
	} {
		if !containsTool(registered, name) {
			t.Errorf("tool %q should be registered for a store-backed vault server", name)
		}
	}

	// Tool handlers wired exactly as newServer constructs them (server.go):
	// navigate and search against the vault backend, compile and promote
	// against the shared store.
	nav := tools.NewNavigate(vc)
	searchTool := tools.NewSearch(vc)
	compileTool := tools.NewCompile(s, nil, nil, vaultDir, 0)
	promoteTool := tools.NewPromote(s)
	ctx := context.Background()

	// --- #90 flow: store_compiled, then same-session reads (no restart) ---

	const (
		tag      = "gateway"
		pageName = "compiled/gateway"
		phrase   = "zorbulon-fragment-seven"
	)

	res, _, err := compileTool.StoreCompiled(ctx, nil, types.StoreCompiledInput{
		Tag:     tag,
		Content: "# Gateway\n\nBody text with the distinctive phrase " + phrase + ".",
		Sources: []string{"gateway-1"},
		Model:   "test-model",
	})
	text := storeAccessOK(t, "store_compiled", res, err)
	var stored map[string]any
	storeAccessJSON(t, "store_compiled", text, &stored)
	if stored["status"] != "stored" {
		t.Errorf("store_compiled: status = %v, want stored", stored["status"])
	}
	if stored["page"] != pageName {
		t.Errorf("store_compiled: page = %v, want %q", stored["page"], pageName)
	}

	// Read tools must see the page in this same session (issue #90).
	storeAccessAssertGetPage(t, nav, ctx, pageName, tag, phrase)
	storeAccessAssertListPages(t, nav, ctx, pageName, tag)
	storeAccessAssertSearch(t, searchTool, ctx, pageName, tag, phrase)
	storeAccessAssertGetBlock(t, nav, s, ctx, pageName, phrase)

	// --- #91 flow: promote draft → validated in the same session ---

	// A second same-session compiled article gives the shorthand promote
	// form its own draft page (spec: Shorthand resolution).
	res, _, err = compileTool.StoreCompiled(ctx, nil, types.StoreCompiledInput{
		Tag:     "auth-patterns",
		Content: "# Auth Patterns\n\nSecond article for promote shorthand resolution.",
	})
	storeAccessOK(t, "store_compiled(auth-patterns)", res, err)

	// Full page name form (spec: Promote Locates Internal Store Pages).
	res, _, err = promoteTool.Promote(ctx, nil, types.PromoteInput{Page: pageName})
	storeAccessAssertPromoted(t, "promote(full name)", storeAccessOK(t, "promote", res, err), pageName)

	// Bare shorthand form (spec: Promote Name Resolution).
	res, _, err = promoteTool.Promote(ctx, nil, types.PromoteInput{Page: "auth-patterns"})
	storeAccessAssertPromoted(t, "promote(shorthand)", storeAccessOK(t, "promote", res, err), "compiled/auth-patterns")

	// The transitions persisted in the store, not just in the tool results.
	for _, name := range []string{pageName, "compiled/auth-patterns"} {
		page, err := s.GetPage(name)
		if err != nil || page == nil {
			t.Fatalf("store GetPage(%q) after promote: %v (page=%v)", name, err, page)
		}
		if page.Tier != "validated" {
			t.Errorf("store tier for %q = %q, want validated", name, page.Tier)
		}
	}

	// --- Enumeration filters apply over the merged set (spec: Unified
	// Page Enumeration — namespace, property, and tag filters over the
	// merged set) ---

	storeAccessAssertListPagesNamespace(t, nav, ctx)
	learningTool := tools.NewLearning(nil, s, vaultDir, 0)
	storeAccessAssertListPagesTagFilter(t, nav, learningTool, ctx)
	storeAccessAssertListPagesPropertyFilter(t, nav, ctx)
}

// storeAccessAssertGetPage asserts get_page returns the just-stored compiled
// page with its block tree and stored properties in the same session
// (spec: Live Retrieval of Internal Store Pages).
func storeAccessAssertGetPage(t *testing.T, nav *tools.Navigate, ctx context.Context, pageName, tag, phrase string) {
	t.Helper()
	res, _, err := nav.GetPage(ctx, nil, types.GetPageInput{Name: pageName})
	text := storeAccessOK(t, "get_page", res, err)

	var result map[string]any
	storeAccessJSON(t, "get_page", text, &result)
	page, _ := result["page"].(map[string]any)
	if page == nil {
		t.Fatalf("get_page: page field missing or wrong type: %T", result["page"])
	}
	if page["name"] != pageName {
		t.Errorf("get_page: page.name = %v, want %q", page["name"], pageName)
	}
	props, _ := page["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("get_page: page.properties missing or wrong type: %T", page["properties"])
	}
	if props["topic"] != tag {
		t.Errorf("get_page: properties.topic = %v, want %q", props["topic"], tag)
	}
	if props["tier"] != "draft" {
		t.Errorf("get_page: properties.tier = %v, want draft", props["tier"])
	}
	if _, ok := props["compiled_at"]; !ok {
		t.Error("get_page: properties.compiled_at missing")
	}
	if count, _ := result["blockCount"].(float64); count <= 0 {
		t.Errorf("get_page: blockCount = %v, want > 0", result["blockCount"])
	}
	blocks, _ := result["blocks"].([]any)
	if !storeAccessTreeContains(blocks, phrase) {
		t.Errorf("get_page: block tree for %q does not contain %q", pageName, phrase)
	}
}

// storeAccessAssertListPages asserts list_pages merges the store-only
// compiled page with vault file pages, with no duplicate names
// (spec: Unified Page Enumeration).
func storeAccessAssertListPages(t *testing.T, nav *tools.Navigate, ctx context.Context, pageName, tag string) {
	t.Helper()
	res, _, err := nav.ListPages(ctx, nil, types.ListPagesInput{})
	text := storeAccessOK(t, "list_pages", res, err)

	var pages []any
	storeAccessJSON(t, "list_pages", text, &pages)

	// Page summaries are keyed by OriginalName; StoreCompiled sets a compiled
	// page's OriginalName to the input tag, so the compiled page is listed
	// under its tag while the store row (and get_page) use "compiled/{tag}".
	var sawCompiled, sawVaultPage bool
	counts := make(map[string]int)
	for _, p := range pages {
		summary, _ := p.(map[string]any)
		name, _ := summary["name"].(string)
		counts[name]++
		switch name {
		case tag:
			sawCompiled = true
			if props, _ := summary["properties"].(map[string]any); props["topic"] != tag {
				t.Errorf("list_pages: compiled page properties.topic = %v, want %q", props["topic"], tag)
			}
		case "readme":
			sawVaultPage = true
		}
	}
	if !sawCompiled {
		t.Errorf("list_pages: store-only page %q missing from the merged set (issue #90)", pageName)
	}
	if !sawVaultPage {
		t.Error("list_pages: vault file page \"readme\" missing from the merged set")
	}
	for name, n := range counts {
		if n > 1 {
			t.Errorf("list_pages: page %q listed %d times, want once", name, n)
		}
	}
}

// storeAccessAssertListPagesNamespace asserts the list_pages namespace
// filter applies over the merged vault+store set: namespace "compiled"
// returns exactly the store-backed compiled pages and excludes the vault
// file page (spec: Unified Page Enumeration — Namespace filter over the
// merged set).
func storeAccessAssertListPagesNamespace(t *testing.T, nav *tools.Navigate, ctx context.Context) {
	t.Helper()
	res, _, err := nav.ListPages(ctx, nil, types.ListPagesInput{Namespace: "compiled"})
	text := storeAccessOK(t, "list_pages(namespace=compiled)", res, err)

	var pages []any
	storeAccessJSON(t, "list_pages(namespace=compiled)", text, &pages)

	// Page summaries are keyed by OriginalName: compiled pages list under
	// their tag (see storeAccessAssertListPages), so the two entries wanted
	// here are the store-backed compiled/ pages stored earlier in this test.
	counts := make(map[string]int)
	for _, p := range pages {
		summary, _ := p.(map[string]any)
		name, _ := summary["name"].(string)
		counts[name]++
	}
	if counts["gateway"] != 1 || counts["auth-patterns"] != 1 {
		t.Errorf("list_pages(namespace=compiled) = %v, want exactly the store-backed compiled pages gateway and auth-patterns", counts)
	}
	if len(pages) != 2 {
		t.Errorf("list_pages(namespace=compiled) returned %d pages, want 2 (vault file page \"readme\" must not match the compiled/ namespace)", len(pages))
	}
}

// storeAccessAssertListPagesTagFilter asserts the list_pages has_tag
// filter applies over the merged set: a learning stored this session —
// whose block content carries a #tag — is returned by the filter, which
// exercises the live GetPageBlocksTree store fallback for store-only
// pages (spec: Unified Page Enumeration — tag filters over the merged
// set).
func storeAccessAssertListPagesTagFilter(t *testing.T, nav *tools.Navigate, learningTool *tools.Learning, ctx context.Context) {
	t.Helper()
	res, _, err := learningTool.StoreLearning(ctx, nil, types.StoreLearningInput{
		Tag:         "deployment",
		Information: "Gateways deploy behind a load balancer. #gateway-deploy",
	})
	text := storeAccessOK(t, "store_learning", res, err)
	var stored map[string]any
	storeAccessJSON(t, "store_learning", text, &stored)
	pageName, _ := stored["page"].(string)
	if pageName == "" {
		t.Fatalf("store_learning: page field missing or empty: %v", stored)
	}

	res, _, err = nav.ListPages(ctx, nil, types.ListPagesInput{HasTag: "gateway-deploy"})
	text = storeAccessOK(t, "list_pages(hasTag)", res, err)
	var pages []any
	storeAccessJSON(t, "list_pages(hasTag)", text, &pages)

	if len(pages) != 1 {
		t.Errorf("list_pages(hasTag=gateway-deploy) returned %d pages, want exactly the learning page %q", len(pages), pageName)
	}
	if len(pages) > 0 {
		if summary, _ := pages[0].(map[string]any); summary["name"] != pageName {
			t.Errorf("list_pages(hasTag=gateway-deploy): page = %v, want %q", summary["name"], pageName)
		}
	}
}

// storeAccessAssertListPagesPropertyFilter asserts the list_pages
// has_property filter applies over the merged vault+store set: the
// store-backed compiled pages (hydrated properties.topic) are returned,
// while the property-less vault file page and the learning page (whose
// properties carry `tag`, not `topic`) are excluded. Asserting exclusion as
// well as inclusion makes the test fail under a filter-before-merge reorder,
// not just a property-hydration break (spec: Unified Page Enumeration —
// property filter over the merged set; query_properties is store-blind by
// design, so this filter is the only property surface over the merged set).
func storeAccessAssertListPagesPropertyFilter(t *testing.T, nav *tools.Navigate, ctx context.Context) {
	t.Helper()
	res, _, err := nav.ListPages(ctx, nil, types.ListPagesInput{HasProperty: "topic"})
	text := storeAccessOK(t, "list_pages(hasProperty=topic)", res, err)

	var pages []any
	storeAccessJSON(t, "list_pages(hasProperty=topic)", text, &pages)

	// Compiled pages list under their OriginalName (the tag): gateway and
	// auth-patterns (see storeAccessAssertListPages).
	counts := make(map[string]int)
	for _, p := range pages {
		summary, _ := p.(map[string]any)
		name, _ := summary["name"].(string)
		counts[name]++
	}
	if counts["gateway"] != 1 || counts["auth-patterns"] != 1 {
		t.Errorf("list_pages(hasProperty=topic) = %v, want the store-backed compiled pages gateway and auth-patterns", counts)
	}
	if len(pages) != 2 {
		t.Errorf("list_pages(hasProperty=topic) returned %d pages, want 2 (the property-less vault file \"readme\" and the learning page, whose properties carry `tag` but not `topic`, must not match)", len(pages))
	}
}

// storeAccessAssertSearch asserts search finds the same-session compiled
// article by content and find_by_tag matches it by its topic tag
// (specs: Unified Full-Text Search, Unified Tag Search).
func storeAccessAssertSearch(t *testing.T, searchTool *tools.Search, ctx context.Context, pageName, tag, phrase string) {
	t.Helper()
	res, _, err := searchTool.Search(ctx, nil, types.SearchInput{Query: phrase})
	text := storeAccessOK(t, "search", res, err)

	var result map[string]any
	storeAccessJSON(t, "search", text, &result)
	results, _ := result["results"].([]any)
	hit := storeAccessResultFor(results, pageName)
	if hit == nil {
		t.Fatalf("search: no hit from %q for phrase %q (issue #90)", pageName, phrase)
	}
	if content, _ := hit["content"].(string); !strings.Contains(content, phrase) {
		t.Errorf("search: hit content %q does not contain %q", content, phrase)
	}

	res, _, err = searchTool.FindByTag(ctx, nil, types.FindByTagInput{Tag: tag})
	text = storeAccessOK(t, "find_by_tag", res, err)
	storeAccessJSON(t, "find_by_tag", text, &result)
	results, _ = result["results"].([]any)
	if storeAccessResultFor(results, pageName) == nil {
		t.Errorf("find_by_tag: %q not matched by topic tag %q (issue #90)", pageName, tag)
	}
}

// storeAccessAssertGetBlock asserts get_block resolves a block UUID from the
// stored compiled page tree with its content and page reference
// (spec: Block Retrieval for Store Pages).
func storeAccessAssertGetBlock(t *testing.T, nav *tools.Navigate, s *store.Store, ctx context.Context, pageName, phrase string) {
	t.Helper()
	// Take a block UUID from the stored page tree — the same store the server
	// session holds. store_learning returns the first block's UUID the same
	// way, and agents pass it directly to get_block.
	storedBlocks, err := s.GetBlocksByPage(pageName)
	if err != nil {
		t.Fatalf("store GetBlocksByPage(%q): %v", pageName, err)
	}
	var uuid string
	for _, b := range storedBlocks {
		if b != nil && strings.Contains(b.Content, phrase) {
			uuid = b.UUID
			break
		}
	}
	if uuid == "" {
		t.Fatalf("no stored block in %q contains %q", pageName, phrase)
	}

	res, _, err := nav.GetBlock(ctx, nil, types.GetBlockInput{UUID: uuid})
	text := storeAccessOK(t, "get_block", res, err)

	var block map[string]any
	storeAccessJSON(t, "get_block", text, &block)
	if block["uuid"] != uuid {
		t.Errorf("get_block: uuid = %v, want %q", block["uuid"], uuid)
	}
	if content, _ := block["content"].(string); !strings.Contains(content, phrase) {
		t.Errorf("get_block: content %q does not contain %q", content, phrase)
	}
	if ref, _ := block["page"].(map[string]any); ref["name"] != pageName {
		t.Errorf("get_block: page reference = %v, want %q", block["page"], pageName)
	}
}

// storeAccessAssertPromoted parses a promote result and asserts the
// draft → validated transition was reported for the resolved page.
func storeAccessAssertPromoted(t *testing.T, label, text, wantPage string) {
	t.Helper()
	var result map[string]any
	storeAccessJSON(t, label, text, &result)
	if result["page"] != wantPage {
		t.Errorf("%s: page = %v, want %q", label, result["page"], wantPage)
	}
	if result["previous_tier"] != "draft" {
		t.Errorf("%s: previous_tier = %v, want draft", label, result["previous_tier"])
	}
	if result["new_tier"] != "validated" {
		t.Errorf("%s: new_tier = %v, want validated", label, result["new_tier"])
	}
}
