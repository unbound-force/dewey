package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/types"
)

// mockBackend is a configurable test double that implements backend.Backend
// and optional capability interfaces (TagSearcher, PropertySearcher, etc.).
// Use it across all tool tests to avoid coupling to real Logseq/Obsidian.
type mockBackend struct {
	// Page data
	pages      []types.PageEntity
	pageBlocks map[string][]types.BlockEntity // page name → blocks
	pageByName map[string]*types.PageEntity

	// Block data
	blocks map[string]*types.BlockEntity // UUID → block

	// Linked references (raw JSON per page)
	linkedRefs map[string]json.RawMessage

	// DataScript query responses (query string → response)
	queryResults map[string]json.RawMessage

	// Write operation tracking
	createdPages    []string
	appendedBlocks  []struct{ page, content string }
	prependedBlocks []struct{ page, content string }
	insertedBlocks  []struct{ parent, content string }
	updatedBlocks   []struct{ uuid, content string }
	removedBlocks   []string
	movedBlocks     []struct{ uuid, target string }
	deletedPages    []string
	renamedPages    []struct{ old, new string }

	// Write return values
	createPageResult   *types.PageEntity
	appendBlockResult  *types.BlockEntity
	prependBlockResult *types.BlockEntity
	insertBlockResult  *types.BlockEntity

	// Error injection
	getPageErr       error
	getBlocksErr     error
	getBlockErr      error
	getLinkedRefsErr error
	getAllPagesErr   error
	queryErr         error
	createPageErr    error
	appendBlockErr   error
	prependBlockErr  error
	insertBlockErr   error
	updateBlockErr   error
	removeBlockErr   error
	moveBlockErr     error
	deletePageErr    error
	renamePageErr    error
	pingErr          error

	// Capability flags
	hasDataScript    bool
	tagSearcher      *mockTagSearcher
	propertySearcher *mockPropertySearcher
	journalSearcher  *mockJournalSearcher
	fullTextSearcher *mockFullTextSearcher
}

// newMockBackend creates a new mockBackend with initialized maps.
func newMockBackend() *mockBackend {
	return &mockBackend{
		pageBlocks:   make(map[string][]types.BlockEntity),
		pageByName:   make(map[string]*types.PageEntity),
		blocks:       make(map[string]*types.BlockEntity),
		linkedRefs:   make(map[string]json.RawMessage),
		queryResults: make(map[string]json.RawMessage),
	}
}

// addPage adds a page and optionally its blocks to the mock.
func (m *mockBackend) addPage(p types.PageEntity, blocks ...types.BlockEntity) {
	m.pages = append(m.pages, p)
	name := p.Name
	m.pageByName[name] = &p
	if len(blocks) > 0 {
		m.pageBlocks[name] = blocks
	}
}

// addBlock registers a block by UUID.
func (m *mockBackend) addBlock(b types.BlockEntity) {
	m.blocks[b.UUID] = &b
}

// --- backend.Backend implementation ---

func (m *mockBackend) GetAllPages(_ context.Context) ([]types.PageEntity, error) {
	if m.getAllPagesErr != nil {
		return nil, m.getAllPagesErr
	}
	return m.pages, nil
}

func (m *mockBackend) GetPage(_ context.Context, nameOrID any) (*types.PageEntity, error) {
	if m.getPageErr != nil {
		return nil, m.getPageErr
	}
	name := fmt.Sprint(nameOrID)
	if p, ok := m.pageByName[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("page not found: %s", name)
}

func (m *mockBackend) GetPageBlocksTree(_ context.Context, nameOrID any) ([]types.BlockEntity, error) {
	if m.getBlocksErr != nil {
		return nil, m.getBlocksErr
	}
	name := fmt.Sprint(nameOrID)
	if blocks, ok := m.pageBlocks[name]; ok {
		return blocks, nil
	}
	return nil, nil
}

func (m *mockBackend) GetBlock(_ context.Context, uuid string, opts ...map[string]any) (*types.BlockEntity, error) {
	if m.getBlockErr != nil {
		return nil, m.getBlockErr
	}
	if b, ok := m.blocks[uuid]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("block not found: %s", uuid)
}

func (m *mockBackend) GetPageLinkedReferences(_ context.Context, nameOrID any) (json.RawMessage, error) {
	if m.getLinkedRefsErr != nil {
		return nil, m.getLinkedRefsErr
	}
	name := fmt.Sprint(nameOrID)
	if raw, ok := m.linkedRefs[name]; ok {
		return raw, nil
	}
	return json.RawMessage(`[]`), nil
}

func (m *mockBackend) DatascriptQuery(_ context.Context, query string, inputs ...any) (json.RawMessage, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if raw, ok := m.queryResults[query]; ok {
		return raw, nil
	}
	return json.RawMessage(`[]`), nil
}

func (m *mockBackend) CreatePage(_ context.Context, name string, properties map[string]any, opts map[string]any) (*types.PageEntity, error) {
	if m.createPageErr != nil {
		return nil, m.createPageErr
	}
	m.createdPages = append(m.createdPages, name)
	if m.createPageResult != nil {
		return m.createPageResult, nil
	}
	return &types.PageEntity{Name: name, OriginalName: name}, nil
}

func (m *mockBackend) AppendBlockInPage(_ context.Context, page string, content string) (*types.BlockEntity, error) {
	if m.appendBlockErr != nil {
		return nil, m.appendBlockErr
	}
	m.appendedBlocks = append(m.appendedBlocks, struct{ page, content string }{page, content})
	if m.appendBlockResult != nil {
		return m.appendBlockResult, nil
	}
	return &types.BlockEntity{UUID: "mock-uuid-append", Content: content}, nil
}

func (m *mockBackend) PrependBlockInPage(_ context.Context, page string, content string) (*types.BlockEntity, error) {
	if m.prependBlockErr != nil {
		return nil, m.prependBlockErr
	}
	m.prependedBlocks = append(m.prependedBlocks, struct{ page, content string }{page, content})
	if m.prependBlockResult != nil {
		return m.prependBlockResult, nil
	}
	return &types.BlockEntity{UUID: "mock-uuid-prepend", Content: content}, nil
}

func (m *mockBackend) InsertBlock(_ context.Context, srcBlock any, content string, opts map[string]any) (*types.BlockEntity, error) {
	if m.insertBlockErr != nil {
		return nil, m.insertBlockErr
	}
	m.insertedBlocks = append(m.insertedBlocks, struct{ parent, content string }{fmt.Sprint(srcBlock), content})
	if m.insertBlockResult != nil {
		return m.insertBlockResult, nil
	}
	return &types.BlockEntity{UUID: "mock-uuid-insert", Content: content}, nil
}

func (m *mockBackend) UpdateBlock(_ context.Context, uuid string, content string, opts ...map[string]any) error {
	if m.updateBlockErr != nil {
		return m.updateBlockErr
	}
	m.updatedBlocks = append(m.updatedBlocks, struct{ uuid, content string }{uuid, content})
	return nil
}

func (m *mockBackend) RemoveBlock(_ context.Context, uuid string) error {
	if m.removeBlockErr != nil {
		return m.removeBlockErr
	}
	m.removedBlocks = append(m.removedBlocks, uuid)
	return nil
}

func (m *mockBackend) MoveBlock(_ context.Context, uuid string, targetUUID string, opts map[string]any) error {
	if m.moveBlockErr != nil {
		return m.moveBlockErr
	}
	m.movedBlocks = append(m.movedBlocks, struct{ uuid, target string }{uuid, targetUUID})
	return nil
}

func (m *mockBackend) DeletePage(_ context.Context, name string) error {
	if m.deletePageErr != nil {
		return m.deletePageErr
	}
	m.deletedPages = append(m.deletedPages, name)
	return nil
}

func (m *mockBackend) RenamePage(_ context.Context, oldName, newName string) error {
	if m.renamePageErr != nil {
		return m.renamePageErr
	}
	m.renamedPages = append(m.renamedPages, struct{ old, new string }{oldName, newName})
	return nil
}

func (m *mockBackend) Ping(_ context.Context) error {
	return m.pingErr
}

// --- Optional capability interfaces ---

// HasDataScript marks the mock as supporting DataScript.
func (m *mockBackend) HasDataScript() {}

// Verify interface compliance at compile time.
var _ backend.Backend = (*mockBackend)(nil)
var _ backend.HasDataScript = (*mockBackend)(nil)

// --- Mock capability implementations ---

type mockTagSearcher struct {
	results []backend.TagResult
	err     error
}

func (m *mockTagSearcher) FindBlocksByTag(_ context.Context, tag string, includeChildren bool) ([]backend.TagResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

type mockPropertySearcher struct {
	results map[string][]backend.PropertyResult // "key:value:op" → results
	err     error
}

func (m *mockPropertySearcher) FindByProperty(_ context.Context, key, value, operator string) ([]backend.PropertyResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	lookup := key + ":" + value + ":" + operator
	return m.results[lookup], nil
}

type mockJournalSearcher struct {
	results []backend.JournalResult
	err     error
}

func (m *mockJournalSearcher) SearchJournals(_ context.Context, query string, from, to string) ([]backend.JournalResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

type mockFullTextSearcher struct {
	results []backend.SearchHit
	err     error
}

func (m *mockFullTextSearcher) FullTextSearch(_ context.Context, query string, limit int) ([]backend.SearchHit, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && len(m.results) > limit {
		return m.results[:limit], nil
	}
	return m.results, nil
}

// --- mockBackendWithTagSearch combines mockBackend + TagSearcher ---

type mockBackendWithTagSearch struct {
	*mockBackend
	*mockTagSearcher
}

func (m *mockBackendWithTagSearch) FindBlocksByTag(ctx context.Context, tag string, includeChildren bool) ([]backend.TagResult, error) {
	return m.mockTagSearcher.FindBlocksByTag(ctx, tag, includeChildren)
}

var _ backend.TagSearcher = (*mockBackendWithTagSearch)(nil)

// --- mockBackendWithPropertySearch combines mockBackend + PropertySearcher ---

type mockBackendWithPropertySearch struct {
	*mockBackend
	*mockPropertySearcher
}

func (m *mockBackendWithPropertySearch) FindByProperty(ctx context.Context, key, value, operator string) ([]backend.PropertyResult, error) {
	return m.mockPropertySearcher.FindByProperty(ctx, key, value, operator)
}

var _ backend.PropertySearcher = (*mockBackendWithPropertySearch)(nil)

// --- mockBackendWithJournalSearch combines mockBackend + JournalSearcher ---

type mockBackendWithJournalSearch struct {
	*mockBackend
	*mockJournalSearcher
}

func (m *mockBackendWithJournalSearch) SearchJournals(ctx context.Context, query string, from, to string) ([]backend.JournalResult, error) {
	return m.mockJournalSearcher.SearchJournals(ctx, query, from, to)
}

var _ backend.JournalSearcher = (*mockBackendWithJournalSearch)(nil)

// --- mockBackendWithFullTextSearch combines mockBackend + FullTextSearcher ---

type mockBackendWithFullTextSearch struct {
	*mockBackend
	*mockFullTextSearcher
}

func (m *mockBackendWithFullTextSearch) FullTextSearch(ctx context.Context, query string, limit int) ([]backend.SearchHit, error) {
	return m.mockFullTextSearcher.FullTextSearch(ctx, query, limit)
}

var _ backend.FullTextSearcher = (*mockBackendWithFullTextSearch)(nil)
