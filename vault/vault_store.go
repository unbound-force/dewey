package vault

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/v3/backend"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/ignore"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// VaultStore bridges the vault.Client (in-memory index) with the store.Store
// (SQLite persistence). It converts between types.PageEntity/types.BlockEntity
// and store operations, enabling persistent indexing across sessions.
//
// Design decision: The adapter pattern (VaultStore) was chosen over embedding
// store.Store directly in vault.Client to preserve separation of concerns
// (SOLID Single Responsibility). The vault.Client owns in-memory indexing;
// VaultStore owns the persistence bridge.
type VaultStore struct {
	store         *store.Store
	embedder      embed.Embedder // optional — nil when Ollama is unavailable
	vaultPath     string
	sourceID      string // e.g., "disk-local"
	maxChunkChars int    // max chars per embedding chunk; 0 uses embed.DefaultMaxChunkChars
}

// NewVaultStore creates a VaultStore adapter for bridging vault and store.
// The sourceID identifies this vault in the store (typically "disk-local").
func NewVaultStore(s *store.Store, vaultPath, sourceID string) *VaultStore {
	return &VaultStore{
		store:     s,
		vaultPath: vaultPath,
		sourceID:  sourceID,
	}
}

// SetEmbedder configures the embedding provider for the vault store.
// When set, embedding generation is integrated into the indexing pipeline.
// Pass nil to disable embedding generation (graceful degradation).
func (vs *VaultStore) SetEmbedder(e embed.Embedder) {
	vs.embedder = e
}

// SetMaxChunkChars configures the maximum character count per embedding chunk.
// When set to a positive value, overrides embed.DefaultMaxChunkChars for all
// embedding generation in this vault store. Pass 0 to use the default.
func (vs *VaultStore) SetMaxChunkChars(n int) {
	vs.maxChunkChars = n
}

// effectiveMaxChunkChars returns the configured max chunk chars, falling back
// to embed.DefaultMaxChunkChars when no override is set.
func (vs *VaultStore) effectiveMaxChunkChars() int {
	if vs.maxChunkChars > 0 {
		return vs.maxChunkChars
	}
	return embed.DefaultMaxChunkChars
}

// PersistPage writes a single page and its blocks/links to the store.
// It replaces any existing data for the page (delete + re-insert).
// Uses parameterized queries throughout (FR-028).
func (vs *VaultStore) PersistPage(page *cachedPage) error {
	if vs.store == nil {
		return nil
	}

	pageName := page.entity.Name
	contentHash := vs.computeContentHash(page)

	// Convert properties to JSON for storage.
	propsJSON := ""
	if page.entity.Properties != nil {
		data, err := json.Marshal(page.entity.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties for %q: %w", pageName, err)
		}
		propsJSON = string(data)
	}

	// Check if page already exists in store.
	existing, err := vs.store.GetPage(pageName)
	if err != nil {
		return fmt.Errorf("check existing page %q: %w", pageName, err)
	}

	if existing != nil {
		// Update existing page.
		existing.OriginalName = page.entity.OriginalName
		existing.SourceID = vs.sourceID
		existing.SourceDocID = page.filePath
		existing.Properties = propsJSON
		existing.ContentHash = contentHash
		existing.IsJournal = page.entity.Journal
		if err := vs.store.UpdatePage(existing); err != nil {
			return fmt.Errorf("update page %q: %w", pageName, err)
		}

		// Replace blocks and links for this page.
		if err := vs.store.DeleteBlocksByPage(pageName); err != nil {
			return fmt.Errorf("delete blocks for %q: %w", pageName, err)
		}
		if err := vs.store.DeleteLinksByPage(pageName); err != nil {
			return fmt.Errorf("delete links for %q: %w", pageName, err)
		}
	} else {
		// Insert new page.
		sp := &store.Page{
			Name:         pageName,
			OriginalName: page.entity.OriginalName,
			SourceID:     vs.sourceID,
			SourceDocID:  page.filePath,
			Properties:   propsJSON,
			ContentHash:  contentHash,
			IsJournal:    page.entity.Journal,
			CreatedAt:    page.entity.CreatedAt,
			UpdatedAt:    page.entity.UpdatedAt,
		}
		if err := vs.store.InsertPage(sp); err != nil {
			return fmt.Errorf("insert page %q: %w", pageName, err)
		}
	}

	// Persist blocks.
	if err := vs.persistBlocks(pageName, page.blocks, sql.NullString{}, 0); err != nil {
		return fmt.Errorf("persist blocks for %q: %w", pageName, err)
	}

	// Persist links extracted from blocks.
	if err := vs.persistLinks(pageName, page.blocks); err != nil {
		return fmt.Errorf("persist links for %q: %w", pageName, err)
	}

	// Generate embeddings for blocks if embedder is available.
	// This is a best-effort operation — embedding failures don't block indexing.
	vs.generateEmbeddings(pageName, page.blocks, nil)

	return nil
}

// RemovePage deletes a page and its associated blocks/links from the store.
func (vs *VaultStore) RemovePage(pageName string) error {
	if vs.store == nil {
		return nil
	}

	// CASCADE handles blocks and links via foreign keys.
	err := vs.store.DeletePage(pageName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("remove page %q from store: %w", pageName, err)
	}
	return nil
}

// LoadPages loads all persisted pages from the store and returns them as
// a map of content hashes keyed by page name. This is used for incremental
// indexing — comparing stored hashes against current file hashes.
//
// IMPORTANT: Only loads pages from this vault's own source (vs.sourceID,
// typically "disk-local"). External-source pages (from dewey index) are
// excluded so that IncrementalIndex() does not treat them as deleted
// (they don't have corresponding .md files on disk).
func (vs *VaultStore) LoadPages() (map[string]string, error) {
	if vs.store == nil {
		return nil, nil
	}

	pages, err := vs.store.ListPagesBySource(vs.sourceID)
	if err != nil {
		return nil, fmt.Errorf("list pages from store: %w", err)
	}

	hashes := make(map[string]string, len(pages))
	for _, p := range pages {
		hashes[p.Name] = p.ContentHash
	}
	logger.Debug("loaded stored hashes for incremental index",
		"source", vs.sourceID, "pages", len(hashes))
	return hashes, nil
}

// SyncToStore persists the entire in-memory index to the store.
// Used for first-run full-index and corruption recovery.
func (vs *VaultStore) SyncToStore(pages map[string]*cachedPage) error {
	if vs.store == nil {
		return nil
	}

	seen := make(map[string]bool)
	for _, page := range pages {
		if seen[page.lowerName] {
			continue // skip alias duplicates
		}
		seen[page.lowerName] = true

		if err := vs.PersistPage(page); err != nil {
			return fmt.Errorf("sync page %q: %w", page.entity.Name, err)
		}
	}

	// Update metadata.
	now := time.Now().UnixMilli()
	if err := vs.store.SetMeta("last_full_index_at", fmt.Sprintf("%d", now)); err != nil {
		return fmt.Errorf("set last_full_index_at: %w", err)
	}
	if err := vs.store.SetMeta("page_count", fmt.Sprintf("%d", len(seen))); err != nil {
		return fmt.Errorf("set page_count: %w", err)
	}

	return nil
}

// fileEntry holds metadata for a single vault file discovered during a walk.
type fileEntry struct {
	relPath string
	content string
	info    os.FileInfo
}

// pageDiff categorizes pages by comparing current file hashes against stored hashes.
type pageDiff struct {
	newPages     []string // pages on disk but not in store
	changedPages []string // pages on disk whose hash differs from store
	deletedPages []string // pages in store but not on disk
	unchanged    []string // pages on disk whose hash matches store
}

// walkVault scans the vault directory and returns a map of page names to content
// hashes, and a map of page names to file metadata. The provided matcher is
// used to skip directories and files matching gitignore patterns.
func walkVault(vaultPath string, matcher *ignore.Matcher) (currentFiles map[string]string, fileContents map[string]fileEntry, err error) {
	currentFiles = make(map[string]string)
	fileContents = make(map[string]fileEntry)

	walkErr := filepath.Walk(vaultPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			logger.Debug("skipping path", "path", path, "err", walkErr)
			return nil // skip errors
		}
		// Use the ignore matcher to skip directories and files matching
		// gitignore patterns. This replaces the previous inline
		// strings.HasPrefix(info.Name(), ".") check.
		if matcher.ShouldSkip(info.Name(), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Debug("skipping unreadable file", "path", path, "err", readErr)
			return nil // skip unreadable files
		}

		relPath, _ := filepath.Rel(vaultPath, path)
		relPath = filepath.ToSlash(relPath)
		pageName := strings.TrimSuffix(relPath, ".md")
		hash := computeHash(string(content))

		currentFiles[pageName] = hash
		fileContents[pageName] = fileEntry{relPath, string(content), info}

		return nil
	})
	if walkErr != nil {
		return nil, nil, fmt.Errorf("walk vault: %w", walkErr)
	}

	return currentFiles, fileContents, nil
}

// diffPages compares current file hashes against stored hashes and categorizes
// each page as new, changed, deleted, or unchanged. This is a pure function
// with no side effects.
func diffPages(currentFiles map[string]string, storedHashes map[string]string) pageDiff {
	var diff pageDiff
	seen := make(map[string]bool, len(storedHashes))
	for k := range storedHashes {
		seen[k] = true
	}

	for pageName, currentHash := range currentFiles {
		storedHash, exists := storedHashes[pageName]
		if !exists {
			diff.newPages = append(diff.newPages, pageName)
		} else if storedHash != currentHash {
			diff.changedPages = append(diff.changedPages, pageName)
		} else {
			diff.unchanged = append(diff.unchanged, pageName)
		}
		delete(seen, pageName)
	}

	for pageName := range seen {
		diff.deletedPages = append(diff.deletedPages, pageName)
	}

	return diff
}

// IncrementalIndex performs incremental indexing by comparing file content
// hashes against stored hashes. It returns counts of new, changed, deleted,
// and unchanged files.
//
// Algorithm:
//  1. Load stored content hashes from the store
//  2. Walk the vault directory, computing content hashes for each .md file
//  3. Compare: new files (not in store), changed files (hash differs),
//     unchanged files (hash matches), deleted files (in store but not on disk)
//  4. Re-index only new and changed files
//  5. Remove deleted files from store
func (vs *VaultStore) IncrementalIndex(c *Client) (stats IndexStats, err error) {
	if vs.store == nil {
		return stats, nil
	}

	storedHashes, err := vs.LoadPages()
	if err != nil {
		return stats, fmt.Errorf("load stored hashes: %w", err)
	}

	// Use the client's matcher for ignore filtering. The matcher is always
	// initialized by vault.New(), so it is guaranteed to be non-nil.
	matcher := c.matcher

	currentFiles, fileContents, err := walkVault(c.vaultPath, matcher)
	if err != nil {
		return stats, err
	}

	diff := diffPages(currentFiles, storedHashes)

	// Index new files.
	changedPages := make(map[string]bool)
	for _, pageName := range diff.newPages {
		fc := fileContents[pageName]
		c.indexFile(fc.relPath, fc.content, fc.info)
		changedPages[pageName] = true
		stats.New++
	}

	// Index changed files.
	for _, pageName := range diff.changedPages {
		fc := fileContents[pageName]
		c.indexFile(fc.relPath, fc.content, fc.info)
		changedPages[pageName] = true
		stats.Changed++
	}

	// Load unchanged files into memory (needed for serving).
	for _, pageName := range diff.unchanged {
		fc := fileContents[pageName]
		c.indexFile(fc.relPath, fc.content, fc.info)
		stats.Unchanged++
	}

	// Remove deleted files from index and store.
	for _, pageName := range diff.deletedPages {
		lowerName := strings.ToLower(pageName)
		c.removePageFromIndex(lowerName)
		if err := vs.RemovePage(pageName); err != nil {
			logger.Warn("failed to remove deleted page from store",
				"page", pageName, "err", err)
		}
		stats.Deleted++
	}

	c.BuildBacklinks()

	// Persist only new/changed pages to store.
	c.mu.RLock()
	for pageName := range changedPages {
		lowerName := strings.ToLower(pageName)
		if page, ok := c.pages[lowerName]; ok {
			if err := vs.PersistPage(page); err != nil {
				logger.Warn("failed to persist page to store",
					"page", pageName, "err", err)
			}
		}
	}
	c.mu.RUnlock()

	total := stats.New + stats.Changed + stats.Unchanged
	if err := vs.store.SetMeta("page_count", fmt.Sprintf("%d", total)); err != nil {
		logger.Warn("failed to update page_count metadata", "err", err)
	}

	return stats, nil
}

// FullIndex performs a complete index of all vault files and persists to store.
// Used on first run when no .uf/dewey/graph.db exists, or after corruption recovery.
func (vs *VaultStore) FullIndex(c *Client) error {
	// Load all files into memory (existing vault.Client behavior).
	if err := c.Load(); err != nil {
		return fmt.Errorf("load vault: %w", err)
	}
	c.BuildBacklinks()

	// Persist entire in-memory index to store.
	c.mu.RLock()
	defer c.mu.RUnlock()
	return vs.SyncToStore(c.pages)
}

// ValidateStore checks the store's schema version and integrity.
// Returns nil if the store is valid, or an error describing the issue.
// Used for corruption detection (T018).
func (vs *VaultStore) ValidateStore() error {
	if vs.store == nil {
		return fmt.Errorf("store is nil")
	}

	version, err := vs.store.GetMeta("schema_version")
	if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}
	if version == "" {
		return fmt.Errorf("missing schema_version metadata")
	}

	// Validate by attempting a simple query.
	_, err = vs.store.ListPages()
	if err != nil {
		return fmt.Errorf("store integrity check failed: %w", err)
	}

	return nil
}

// IndexStats reports the results of an incremental index operation.
type IndexStats struct {
	New       int
	Changed   int
	Deleted   int
	Unchanged int
}

// Total returns the total number of files processed.
func (s IndexStats) Total() int {
	return s.New + s.Changed + s.Deleted + s.Unchanged
}

// generateEmbeddings creates vector embeddings for blocks in a page.
// Skips silently if the embedder is nil or unavailable. Embedding failures
// are logged but don't block indexing (graceful degradation per Constitution I).
// Delegates to the shared GenerateEmbeddings function.
func (vs *VaultStore) generateEmbeddings(pageName string, blocks []types.BlockEntity, headingPath []string) {
	if vs.embedder == nil || !vs.embedder.Available() {
		return
	}
	if vs.store == nil {
		return
	}
	GenerateEmbeddings(vs.store, vs.embedder, pageName, blocks, headingPath, vs.effectiveMaxChunkChars())
}

// --- Store read helpers (mcp-internal-store-access) ---

// GetStorePage looks up a page by name in the persistent store and returns the
// page entity together with its reconstructed block tree — the same shape
// LoadExternalPages registers for external pages. It backs the vault client's
// live read-through fallbacks so store-backed pages are retrievable without a
// server restart (mcp-internal-store-access, Live Retrieval of Internal Store
// Pages). Results are not cached; every call queries the store fresh (design
// D2 — live lookups, no mid-session caching of store reads).
//
// Returns (nil, nil, false, nil) when no page with the given name exists, and
// a non-nil error when a store query fails. A nil store behaves as an
// always-miss lookup, preserving no-store parity (Constitution II).
func (vs *VaultStore) GetStorePage(name string) (*types.PageEntity, []types.BlockEntity, bool, error) {
	if vs.store == nil {
		return nil, nil, false, nil
	}

	sp, err := vs.store.GetPage(name)
	if err != nil {
		return nil, nil, false, fmt.Errorf("look up store page %q: %w", name, err)
	}
	if sp == nil {
		return nil, nil, false, nil
	}

	entity, blocks, err := vs.hydrateStorePage(sp)
	if err != nil {
		return nil, nil, false, err
	}
	return &entity, blocks, true, nil
}

// GetStoreBlock resolves a block by UUID in the persistent store and returns
// it with a reference to its page, matching the shape the vault client's
// in-memory GetBlock serves (content, UUID, and page reference). It backs the
// get_block fallback for pages that live only in the store
// (mcp-internal-store-access, Block Retrieval for Store Pages).
//
// Returns (nil, false, nil) when no block with the given UUID exists, and a
// non-nil error when the store query fails. A nil store behaves as an
// always-miss lookup, preserving no-store parity (Constitution II).
func (vs *VaultStore) GetStoreBlock(uuid string) (*types.BlockEntity, bool, error) {
	if vs.store == nil {
		return nil, false, nil
	}

	sb, err := vs.store.GetBlock(uuid)
	if err != nil {
		return nil, false, fmt.Errorf("look up store block %q: %w", uuid, err)
	}
	if sb == nil {
		return nil, false, nil
	}

	return &types.BlockEntity{
		UUID:    sb.UUID,
		Content: sb.Content,
		Page:    &types.PageRef{Name: sb.PageName},
	}, true, nil
}

// ListStorePages returns the entities of all store pages that are not
// persisted mirrors of local vault files — the same
// ListPagesExcludingSource("disk-local") predicate LoadExternalPages uses —
// converted via the shared storePageEntity conversion. It backs the vault
// client's GetAllPages unified enumeration merge (mcp-internal-store-access,
// spec: Unified Page Enumeration). Page rows only: no blocks are loaded,
// keeping the merge cost at one store scan (design D2/D3).
//
// Results are not cached; every call queries the store fresh (design D2). A
// nil store behaves as an empty list, preserving no-store parity
// (Constitution II).
func (vs *VaultStore) ListStorePages() ([]types.PageEntity, error) {
	if vs.store == nil {
		return nil, nil
	}

	rows, err := vs.store.ListPagesExcludingSource("disk-local")
	if err != nil {
		return nil, fmt.Errorf("list store pages: %w", err)
	}

	entities := make([]types.PageEntity, 0, len(rows))
	for _, sp := range rows {
		entities = append(entities, storePageEntity(sp))
	}
	return entities, nil
}

// SearchStoreBlocks searches block content in the persistent store via
// store.SearchBlocksByContent (case-insensitive containment match) and returns
// the hits converted to backend.SearchHit — page name, UUID, content — the
// same shape the vault's in-memory search index produces. It backs the vault
// client's FullTextSearch store merge (mcp-internal-store-access, design D4;
// spec: Unified Full-Text Search).
//
// Results are not cached; every call queries the store fresh (design D2 —
// live lookups, no mid-session caching of store reads). A nil store behaves
// as an empty result, preserving no-store parity (Constitution II). A limit
// greater than 0 caps the returned hits; limit <= 0 returns all matches,
// following the SearchBlocksByContent convention.
func (vs *VaultStore) SearchStoreBlocks(query string, limit int) ([]backend.SearchHit, error) {
	if vs.store == nil {
		return nil, nil
	}

	blocks, err := vs.store.SearchBlocksByContent(query, limit)
	if err != nil {
		return nil, fmt.Errorf("search store blocks for %q: %w", query, err)
	}

	hits := make([]backend.SearchHit, 0, len(blocks))
	for _, b := range blocks {
		if b == nil {
			continue
		}
		hits = append(hits, backend.SearchHit{
			PageName: b.PageName,
			UUID:     b.UUID,
			Content:  b.Content,
		})
	}
	return hits, nil
}

// hydrateStorePage converts a persisted store.Page row into the
// types.PageEntity and reconstructed block-tree shape the vault serves for
// store-backed pages. It is the single conversion shared by GetStorePage and
// LoadExternalPages (DRY), so live lookups and the startup load produce
// identical results.
//
// Properties are parsed from the stored JSON blob; an unparseable blob leaves
// properties nil rather than failing the lookup, matching the historical
// LoadExternalPages behavior. Returns an error only when the block query fails.
func (vs *VaultStore) hydrateStorePage(sp *store.Page) (types.PageEntity, []types.BlockEntity, error) {
	entity := storePageEntity(sp)

	storeBlocks, err := vs.store.GetBlocksByPage(sp.Name)
	if err != nil {
		return types.PageEntity{}, nil, fmt.Errorf("load blocks for store page %q: %w", sp.Name, err)
	}

	return entity, reconstructBlockTree(storeBlocks), nil
}

// storePageEntity converts the metadata half of a persisted store.Page row
// into a types.PageEntity: name, original name, journal flag, timestamps,
// and properties. It is the single entity conversion shared by
// hydrateStorePage and ListStorePages (DRY), so block-loading lookups
// (GetStorePage, LoadExternalPages) and the GetAllPages merge serve
// identical entity shapes.
//
// Properties are parsed from the stored JSON blob; an unparseable blob leaves
// properties nil rather than failing the read, matching the historical
// LoadExternalPages behavior.
func storePageEntity(sp *store.Page) types.PageEntity {
	entity := types.PageEntity{
		Name:         sp.Name,
		OriginalName: sp.OriginalName,
		Journal:      sp.IsJournal,
		CreatedAt:    sp.CreatedAt,
		UpdatedAt:    sp.UpdatedAt,
	}

	if sp.Properties != "" {
		var props map[string]any
		if err := json.Unmarshal([]byte(sp.Properties), &props); err == nil {
			entity.Properties = props
		}
	}

	return entity
}

// LoadExternalPages loads all non-local pages from the persistent store into
// the vault client's in-memory index. This makes external-source content
// (GitHub, web crawl) queryable via all MCP tools alongside local vault pages.
//
// For each external page: the shared hydrateStorePage conversion (also used by
// GetStorePage) produces the types.PageEntity and the reconstructed block
// tree, then a cachedPage with sourceID and readOnly=true is registered via
// applyPageIndex(). Pages whose blocks fail to load are skipped with a
// warning. Returns the number of pages loaded.
//
// Design decision: External pages are marked readOnly=true to prevent write
// operations from modifying content that doesn't exist on disk (per research R7).
func (vs *VaultStore) LoadExternalPages(c *Client) (int, error) {
	if vs.store == nil {
		return 0, nil
	}

	start := time.Now()
	logger.Info("loading external pages from store")

	pages, err := vs.store.ListPagesExcludingSource("disk-local")
	if err != nil {
		return 0, fmt.Errorf("list external pages: %w", err)
	}

	totalBlocks := 0
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, sp := range pages {
		// Convert store.Page → entity + reconstructed block tree via the
		// shared conversion, so live lookups (GetStorePage) and this startup
		// load produce identical shapes.
		entity, blocks, err := vs.hydrateStorePage(sp)
		if err != nil {
			logger.Warn("failed to load blocks for external page",
				"page", sp.Name, "err", err)
			continue
		}
		totalBlocks += countBlocks(blocks)

		// Build cachedPage with external source metadata.
		page := &cachedPage{
			entity:    entity,
			lowerName: strings.ToLower(sp.Name),
			filePath:  sp.SourceDocID,
			blocks:    blocks,
			sourceID:  sp.SourceID,
			readOnly:  true,
		}

		// Register in vault's in-memory index.
		c.applyPageIndex(page)
	}

	elapsed := time.Since(start)
	logger.Info("external pages loaded",
		"pages", len(pages),
		"blocks", totalBlocks,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	return len(pages), nil
}

// reconstructBlockTree converts a flat slice of store.Block records (with
// ParentUUID and Position fields) into a nested []types.BlockEntity tree
// with Children populated. Root blocks (those with no parent) form the
// returned slice. Children are ordered by Position within each parent.
//
// Design decision: This is the inverse of persistBlocks() — round-trip
// fidelity is guaranteed by the schema (per research R5). The algorithm
// processes blocks bottom-up (leaves first) to ensure children are fully
// constructed before being attached to their parents.
func reconstructBlockTree(flat []*store.Block) []types.BlockEntity {
	if len(flat) == 0 {
		return nil
	}

	// Build a map of UUID → index for lookups, and track parent-child relationships.
	type blockInfo struct {
		block    types.BlockEntity
		parentID string // empty string for roots
	}

	infos := make([]blockInfo, len(flat))
	childrenOf := make(map[string][]int) // parentUUID → child indices

	// First pass: create all BlockEntity instances.
	for i, sb := range flat {
		infos[i] = blockInfo{
			block: types.BlockEntity{
				UUID:    sb.UUID,
				Content: sb.Content,
			},
		}
		if sb.ParentUUID.Valid && sb.ParentUUID.String != "" {
			infos[i].parentID = sb.ParentUUID.String
		}
	}

	// Build children map.
	for i, info := range infos {
		if info.parentID != "" {
			childrenOf[info.parentID] = append(childrenOf[info.parentID], i)
		}
	}

	// Recursive function to build a block with its children fully resolved.
	var buildBlock func(idx int) types.BlockEntity
	buildBlock = func(idx int) types.BlockEntity {
		be := infos[idx].block
		if childIndices, ok := childrenOf[be.UUID]; ok {
			be.Children = make([]types.BlockEntity, len(childIndices))
			for i, ci := range childIndices {
				be.Children[i] = buildBlock(ci)
			}
		}
		return be
	}

	// Build the tree starting from roots.
	var roots []types.BlockEntity
	for i, info := range infos {
		if info.parentID == "" {
			roots = append(roots, buildBlock(i))
		}
	}

	return roots
}

// --- Internal helpers ---

// persistBlocks recursively inserts blocks into the store.
// Delegates to the shared PersistBlocks function.
func (vs *VaultStore) persistBlocks(pageName string, blocks []types.BlockEntity, parentUUID sql.NullString, startPos int) error {
	return PersistBlocks(vs.store, pageName, blocks, parentUUID, startPos)
}

// persistLinks extracts wikilinks from blocks and persists them to the store.
// Delegates to the shared PersistLinks function.
func (vs *VaultStore) persistLinks(pageName string, blocks []types.BlockEntity) error {
	return PersistLinks(vs.store, pageName, blocks)
}

// computeContentHash generates a SHA-256 hash of a page's file content.
func (vs *VaultStore) computeContentHash(page *cachedPage) string {
	// Read the actual file content for hashing.
	absPath := filepath.Join(vs.vaultPath, page.filePath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		// Fallback: hash the in-memory content.
		return computeHash(page.entity.Name)
	}
	return computeHash(string(content))
}

// computeHash generates a SHA-256 hex digest of the given content.
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}
