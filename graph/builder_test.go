package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbound-force/dewey/types"
)

// --- Mock backend for builder tests ---

// builderMockBackend is a minimal backend.Backend for testing Build and Cache.
// It records how many times GetAllPages is called so tests can verify caching.
type builderMockBackend struct {
	pages     []types.PageEntity
	blocks    map[string][]types.BlockEntity // page name → blocks
	callCount atomic.Int64
	pagesErr  error
	blocksErr map[string]error // per-page errors for GetPageBlocksTree
}

func newBuilderMock() *builderMockBackend {
	return &builderMockBackend{
		blocks:    make(map[string][]types.BlockEntity),
		blocksErr: make(map[string]error),
	}
}

func (m *builderMockBackend) addPage(name string, blocks ...types.BlockEntity) {
	m.pages = append(m.pages, types.PageEntity{
		Name:         name,
		OriginalName: name,
	})
	if len(blocks) > 0 {
		m.blocks[name] = blocks
	}
}

func (m *builderMockBackend) addJournalPage(name string) {
	m.pages = append(m.pages, types.PageEntity{
		Name:         name,
		OriginalName: name,
		Journal:      true,
	})
}

func (m *builderMockBackend) GetAllPages(_ context.Context) ([]types.PageEntity, error) {
	m.callCount.Add(1)
	if m.pagesErr != nil {
		return nil, m.pagesErr
	}
	return m.pages, nil
}

func (m *builderMockBackend) GetPageBlocksTree(_ context.Context, nameOrID any) ([]types.BlockEntity, error) {
	name := fmt.Sprintf("%v", nameOrID)
	if err, ok := m.blocksErr[name]; ok {
		return nil, err
	}
	return m.blocks[name], nil
}

// --- Unused backend.Backend methods (required to satisfy interface) ---

func (m *builderMockBackend) GetPage(context.Context, any) (*types.PageEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) GetBlock(context.Context, string, ...map[string]any) (*types.BlockEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) GetPageLinkedReferences(context.Context, any) (json.RawMessage, error) {
	return nil, nil
}
func (m *builderMockBackend) DatascriptQuery(context.Context, string, ...any) (json.RawMessage, error) {
	return nil, nil
}
func (m *builderMockBackend) CreatePage(context.Context, string, map[string]any, map[string]any) (*types.PageEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) AppendBlockInPage(context.Context, string, string) (*types.BlockEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) PrependBlockInPage(context.Context, string, string) (*types.BlockEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) InsertBlock(context.Context, any, string, map[string]any) (*types.BlockEntity, error) {
	return nil, nil
}
func (m *builderMockBackend) UpdateBlock(context.Context, string, string, ...map[string]any) error {
	return nil
}
func (m *builderMockBackend) RemoveBlock(context.Context, string) error { return nil }
func (m *builderMockBackend) MoveBlock(context.Context, string, string, map[string]any) error {
	return nil
}
func (m *builderMockBackend) DeletePage(context.Context, string) error         { return nil }
func (m *builderMockBackend) RenamePage(context.Context, string, string) error { return nil }
func (m *builderMockBackend) Ping(context.Context) error                       { return nil }

// --- Build tests ---

func TestBuild_PagesWithLinks(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Go", types.BlockEntity{Content: "Go is great. See [[Rust]] and [[Python]]."})
	mb.addPage("Rust", types.BlockEntity{Content: "Rust is fast. See [[Go]]."})
	mb.addPage("Python", types.BlockEntity{Content: "Python is easy."})

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify pages are indexed (lowercased keys)
	if len(g.Pages) != 3 {
		t.Errorf("Pages count = %d, want 3", len(g.Pages))
	}
	for _, name := range []string{"go", "rust", "python"} {
		if _, ok := g.Pages[name]; !ok {
			t.Errorf("Pages[%q] missing", name)
		}
	}

	// Verify forward links from "go" → Rust, Python
	goForward := g.Forward["go"]
	if !goForward["Rust"] {
		t.Errorf("Forward[go] missing Rust link, got %v", goForward)
	}
	if !goForward["Python"] {
		t.Errorf("Forward[go] missing Python link, got %v", goForward)
	}
	if len(goForward) != 2 {
		t.Errorf("Forward[go] count = %d, want 2", len(goForward))
	}

	// Verify forward links from "rust" → Go
	rustForward := g.Forward["rust"]
	if !rustForward["Go"] {
		t.Errorf("Forward[rust] missing Go link, got %v", rustForward)
	}
	if len(rustForward) != 1 {
		t.Errorf("Forward[rust] count = %d, want 1", len(rustForward))
	}

	// Verify python has no forward links
	if len(g.Forward["python"]) != 0 {
		t.Errorf("Forward[python] count = %d, want 0", len(g.Forward["python"]))
	}

	// Verify backward links
	if !g.Backward["rust"]["go"] {
		t.Errorf("Backward[rust] should contain 'go'")
	}
	if !g.Backward["python"]["go"] {
		t.Errorf("Backward[python] should contain 'go'")
	}
	if !g.Backward["go"]["rust"] {
		t.Errorf("Backward[go] should contain 'rust'")
	}

	// Verify block counts
	if g.BlockCounts["go"] != 1 {
		t.Errorf("BlockCounts[go] = %d, want 1", g.BlockCounts["go"])
	}
}

func TestBuild_EmptyPages(t *testing.T) {
	mb := newBuilderMock()
	// No pages added

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(g.Pages) != 0 {
		t.Errorf("Pages count = %d, want 0", len(g.Pages))
	}
	if len(g.Forward) != 0 {
		t.Errorf("Forward count = %d, want 0", len(g.Forward))
	}
	if len(g.Backward) != 0 {
		t.Errorf("Backward count = %d, want 0", len(g.Backward))
	}
	if len(g.BlockCounts) != 0 {
		t.Errorf("BlockCounts count = %d, want 0", len(g.BlockCounts))
	}
}

func TestBuild_SelfReferencingLinks(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("SelfRef", types.BlockEntity{Content: "Links to [[SelfRef]] itself."})

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Forward: selfref → SelfRef
	if !g.Forward["selfref"]["SelfRef"] {
		t.Errorf("Forward[selfref] missing self-link, got %v", g.Forward["selfref"])
	}

	// Backward: selfref ← selfref
	if !g.Backward["selfref"]["selfref"] {
		t.Errorf("Backward[selfref] missing self-backlink, got %v", g.Backward["selfref"])
	}
}

func TestBuild_SkipsEmptyNamePages(t *testing.T) {
	mb := newBuilderMock()
	// Add a page with empty name (should be skipped)
	mb.pages = append(mb.pages, types.PageEntity{Name: ""})
	mb.addPage("Valid", types.BlockEntity{Content: "content"})

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(g.Pages) != 1 {
		t.Errorf("Pages count = %d, want 1 (empty name skipped)", len(g.Pages))
	}
	if _, ok := g.Pages["valid"]; !ok {
		t.Error("Pages[valid] missing")
	}
}

func TestBuild_GetAllPagesError(t *testing.T) {
	mb := newBuilderMock()
	mb.pagesErr = fmt.Errorf("connection refused")

	_, err := Build(context.Background(), mb)
	if err == nil {
		t.Fatal("Build() expected error, got nil")
	}
	if err.Error() != "connection refused" {
		t.Errorf("Build() error = %q, want 'connection refused'", err.Error())
	}
}

func TestBuild_BlockTreeErrorSkipsPage(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Good", types.BlockEntity{Content: "[[Link]]"})
	mb.addPage("Bad")
	mb.blocksErr["Bad"] = fmt.Errorf("block tree unavailable")

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v (should skip bad page, not fail)", err)
	}

	// Both pages should exist in the graph (they were returned by GetAllPages)
	if len(g.Pages) != 2 {
		t.Errorf("Pages count = %d, want 2", len(g.Pages))
	}

	// Good page should have its links processed
	if !g.Forward["good"]["Link"] {
		t.Errorf("Forward[good] missing Link, got %v", g.Forward["good"])
	}

	// Bad page should have empty forward links (block tree failed)
	if len(g.Forward["bad"]) != 0 {
		t.Errorf("Forward[bad] = %v, want empty (block tree error)", g.Forward["bad"])
	}

	// Bad page should have zero block count
	if g.BlockCounts["bad"] != 0 {
		t.Errorf("BlockCounts[bad] = %d, want 0", g.BlockCounts["bad"])
	}
}

func TestBuild_NestedBlocks(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Parent", types.BlockEntity{
		Content: "[[TopLink]]",
		Children: []types.BlockEntity{
			{Content: "[[ChildLink]]"},
			{
				Content: "no links here",
				Children: []types.BlockEntity{
					{Content: "[[DeepLink]]"},
				},
			},
		},
	})

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// All links from nested blocks should be in forward links
	fwd := g.Forward["parent"]
	for _, want := range []string{"TopLink", "ChildLink", "DeepLink"} {
		if !fwd[want] {
			t.Errorf("Forward[parent] missing %q, got %v", want, fwd)
		}
	}

	// Block count: 1 parent + 2 children + 1 grandchild = 4
	if g.BlockCounts["parent"] != 4 {
		t.Errorf("BlockCounts[parent] = %d, want 4", g.BlockCounts["parent"])
	}
}

func TestBuild_CaseInsensitiveKeys(t *testing.T) {
	mb := newBuilderMock()
	mb.pages = append(mb.pages, types.PageEntity{
		Name:         "MyPage",
		OriginalName: "MyPage",
	})
	mb.blocks["MyPage"] = []types.BlockEntity{
		{Content: "Link to [[AnotherPage]]"},
	}

	g, err := Build(context.Background(), mb)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Page key should be lowercase
	if _, ok := g.Pages["mypage"]; !ok {
		t.Error("Pages should use lowercase key 'mypage'")
	}
	if _, ok := g.Pages["MyPage"]; ok {
		t.Error("Pages should NOT have mixed-case key 'MyPage'")
	}

	// Forward link key should be lowercase
	if _, ok := g.Forward["mypage"]; !ok {
		t.Error("Forward should use lowercase key 'mypage'")
	}

	// But link values preserve original case
	if !g.Forward["mypage"]["AnotherPage"] {
		t.Errorf("Forward[mypage] should contain 'AnotherPage' (original case), got %v", g.Forward["mypage"])
	}

	// Backward link key is lowercase of link target
	if !g.Backward["anotherpage"]["mypage"] {
		t.Errorf("Backward[anotherpage] should contain 'mypage', got %v", g.Backward["anotherpage"])
	}
}

// --- Cache tests ---

func TestCacheGet_BuildsGraphOnFirstCall(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Alpha", types.BlockEntity{Content: "content"})

	cache := NewCache(mb, 5*time.Minute)

	g, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Cache.Get() error = %v", err)
	}

	if g == nil {
		t.Fatal("Cache.Get() returned nil graph")
	}
	if len(g.Pages) != 1 {
		t.Errorf("Pages count = %d, want 1", len(g.Pages))
	}
	if _, ok := g.Pages["alpha"]; !ok {
		t.Error("Pages[alpha] missing")
	}

	// Backend should have been called exactly once
	if calls := mb.callCount.Load(); calls != 1 {
		t.Errorf("GetAllPages called %d times, want 1", calls)
	}
}

func TestCacheGet_ReturnsCachedGraph(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Alpha", types.BlockEntity{Content: "content"})

	cache := NewCache(mb, 5*time.Minute)

	g1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	g2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}

	// Same pointer — cached, not rebuilt
	if g1 != g2 {
		t.Error("second Get() returned different graph pointer, want same (cached)")
	}

	// Backend should have been called only once (second call used cache)
	if calls := mb.callCount.Load(); calls != 1 {
		t.Errorf("GetAllPages called %d times, want 1", calls)
	}
}

func TestCacheGet_InvalidationCausesRebuild(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Alpha", types.BlockEntity{Content: "content"})

	cache := NewCache(mb, 5*time.Minute)

	g1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	cache.Invalidate()

	g2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() after Invalidate() error = %v", err)
	}

	// Different pointer — rebuilt after invalidation
	if g1 == g2 {
		t.Error("Get() after Invalidate() returned same graph pointer, want different (rebuilt)")
	}

	// Backend should have been called twice (initial + after invalidation)
	if calls := mb.callCount.Load(); calls != 2 {
		t.Errorf("GetAllPages called %d times, want 2", calls)
	}
}

func TestCacheGet_TTLExpiryCausesRebuild(t *testing.T) {
	mb := newBuilderMock()
	mb.addPage("Alpha", types.BlockEntity{Content: "content"})

	// Use a very short TTL so it expires quickly
	cache := NewCache(mb, 1*time.Millisecond)

	g1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	g2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get() after TTL error = %v", err)
	}

	// Different pointer — rebuilt after TTL expiry
	if g1 == g2 {
		t.Error("Get() after TTL expiry returned same graph pointer, want different (rebuilt)")
	}

	// Backend should have been called twice
	if calls := mb.callCount.Load(); calls != 2 {
		t.Errorf("GetAllPages called %d times, want 2", calls)
	}
}

func TestCacheGet_BuildError(t *testing.T) {
	mb := newBuilderMock()
	mb.pagesErr = fmt.Errorf("backend unavailable")

	cache := NewCache(mb, 5*time.Minute)

	g, err := cache.Get(context.Background())
	if err == nil {
		t.Fatal("Cache.Get() expected error, got nil")
	}
	if g != nil {
		t.Errorf("Cache.Get() returned non-nil graph on error")
	}
	if err.Error() != "backend unavailable" {
		t.Errorf("Cache.Get() error = %q, want 'backend unavailable'", err.Error())
	}
}

func TestCacheGet_ErrorDoesNotCache(t *testing.T) {
	mb := newBuilderMock()
	mb.pagesErr = fmt.Errorf("temporary failure")

	cache := NewCache(mb, 5*time.Minute)

	// First call fails
	_, err := cache.Get(context.Background())
	if err == nil {
		t.Fatal("first Get() expected error")
	}

	// Fix the backend
	mb.pagesErr = nil
	mb.addPage("Recovery", types.BlockEntity{Content: "back online"})

	// Second call should retry (not return cached error)
	g, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("second Get() error = %v, want nil (recovered)", err)
	}
	if len(g.Pages) != 1 {
		t.Errorf("Pages count = %d, want 1", len(g.Pages))
	}
}

func TestNewCache_Fields(t *testing.T) {
	mb := newBuilderMock()
	ttl := 10 * time.Minute

	cache := NewCache(mb, ttl)

	if cache.ttl != ttl {
		t.Errorf("cache.ttl = %v, want %v", cache.ttl, ttl)
	}
	if cache.graph != nil {
		t.Error("cache.graph should be nil initially")
	}
	if cache.backend == nil {
		t.Error("cache.backend should not be nil")
	}
}
