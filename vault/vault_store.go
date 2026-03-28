package vault

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
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
	store     *store.Store
	embedder  embed.Embedder // optional — nil when Ollama is unavailable
	vaultPath string
	sourceID  string // e.g., "disk-local"
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
func (vs *VaultStore) LoadPages() (map[string]string, error) {
	if vs.store == nil {
		return nil, nil
	}

	pages, err := vs.store.ListPages()
	if err != nil {
		return nil, fmt.Errorf("list pages from store: %w", err)
	}

	hashes := make(map[string]string, len(pages))
	for _, p := range pages {
		hashes[p.Name] = p.ContentHash
	}
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

// LoadFromStore loads persisted pages from the store into the vault client's
// in-memory index. Returns the number of pages loaded.
//
// Design decision: We load page metadata and content hashes from the store
// but still re-parse .md files from disk. The store serves as a change-detection
// cache (via content_hash), not as the authoritative content source. This ensures
// the in-memory index always reflects the actual file content.
func (vs *VaultStore) LoadFromStore() (int, error) {
	if vs.store == nil {
		return 0, nil
	}

	pages, err := vs.store.ListPages()
	if err != nil {
		return 0, fmt.Errorf("load pages from store: %w", err)
	}

	return len(pages), nil
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
// hashes, and a map of page names to file metadata. Hidden directories are skipped.
func walkVault(vaultPath string) (currentFiles map[string]string, fileContents map[string]fileEntry, err error) {
	currentFiles = make(map[string]string)
	fileContents = make(map[string]fileEntry)

	walkErr := filepath.Walk(vaultPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
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

	currentFiles, fileContents, err := walkVault(c.vaultPath)
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
// Used on first run when no .dewey/graph.db exists, or after corruption recovery.
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
func (vs *VaultStore) generateEmbeddings(pageName string, blocks []types.BlockEntity, headingPath []string) {
	if vs.embedder == nil || !vs.embedder.Available() {
		return
	}
	if vs.store == nil {
		return
	}

	ctx := context.Background()

	for _, b := range blocks {
		if strings.TrimSpace(b.Content) == "" {
			continue
		}

		// Build heading path for context.
		currentPath := headingPath
		heading := extractHeading(b.Content)
		if heading != "" {
			currentPath = append(append([]string{}, headingPath...), heading)
		}

		// Prepare chunk with heading hierarchy context.
		chunk := embed.PrepareChunk(pageName, currentPath, b.Content)

		// Generate embedding.
		vec, err := vs.embedder.Embed(ctx, chunk)
		if err != nil {
			logger.Warn("failed to generate embedding",
				"page", pageName, "block", b.UUID, "err", err)
			continue
		}

		// Persist embedding.
		if err := vs.store.InsertEmbedding(b.UUID, vs.embedder.ModelID(), vec, chunk); err != nil {
			logger.Warn("failed to persist embedding",
				"page", pageName, "block", b.UUID, "err", err)
		}

		// Recurse into children with updated heading path.
		if len(b.Children) > 0 {
			vs.generateEmbeddings(pageName, b.Children, currentPath)
		}
	}
}

// extractHeading returns the heading text from a block's content line,
// stripping the leading # characters. Returns empty string for non-heading content.
func extractHeading(content string) string {
	firstLine := content
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !strings.HasPrefix(firstLine, "#") {
		return ""
	}
	// Strip leading # and space.
	trimmed := strings.TrimLeft(firstLine, "#")
	return strings.TrimSpace(trimmed)
}

// LoadExternalPages loads all non-local pages from the persistent store into
// the vault client's in-memory index. This makes external-source content
// (GitHub, web crawl) queryable via all MCP tools alongside local vault pages.
//
// For each external page: converts store.Page → types.PageEntity, loads blocks
// via store.GetBlocksByPage(), reconstructs the block tree via reconstructBlockTree(),
// builds a cachedPage with sourceID and readOnly=true, and registers it via
// applyPageIndex(). Returns the number of pages loaded.
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
		// Convert store.Page → types.PageEntity.
		entity := types.PageEntity{
			Name:         sp.Name,
			OriginalName: sp.OriginalName,
			Journal:      sp.IsJournal,
			CreatedAt:    sp.CreatedAt,
			UpdatedAt:    sp.UpdatedAt,
		}

		// Parse properties JSON if present.
		if sp.Properties != "" {
			var props map[string]any
			if err := json.Unmarshal([]byte(sp.Properties), &props); err == nil {
				entity.Properties = props
			}
		}

		// Load blocks from store and reconstruct tree.
		storeBlocks, err := vs.store.GetBlocksByPage(sp.Name)
		if err != nil {
			logger.Warn("failed to load blocks for external page",
				"page", sp.Name, "err", err)
			continue
		}
		blocks := reconstructBlockTree(storeBlocks)
		totalBlocks += len(storeBlocks)

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
