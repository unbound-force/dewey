package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/parser"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// logger is the package-level structured logger for vault operations.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix: "dewey",
})

// ErrNotSupported is returned for Logseq-specific operations (DataScript queries).
var ErrNotSupported = fmt.Errorf("operation not supported by obsidian backend")

// ErrPathEscape is returned when a resolved path escapes the vault boundary.
var ErrPathEscape = fmt.Errorf("path escapes vault boundary")

// safePath resolves a relative path against the vault root and validates it
// stays within the vault boundary. Prevents path traversal attacks.
func (c *Client) safePath(relPath string) (string, error) {
	absPath := filepath.Join(c.vaultPath, filepath.FromSlash(relPath))
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	vaultAbs, err := filepath.Abs(c.vaultPath)
	if err != nil {
		return "", fmt.Errorf("resolve vault path: %w", err)
	}
	if !strings.HasPrefix(absPath, vaultAbs+string(filepath.Separator)) && absPath != vaultAbs {
		return "", fmt.Errorf("%w: %s", ErrPathEscape, relPath)
	}
	return absPath, nil
}

// generateRandomUUID generates a new random UUID.
func generateRandomUUID() string {
	return uuid.New().String()
}

// cachedPage holds a parsed markdown file in memory.
type cachedPage struct {
	entity    types.PageEntity
	lowerName string
	filePath  string
	blocks    []types.BlockEntity
}

// Client implements backend.Backend for an Obsidian vault on disk.
// It reads all .md files on initialization and serves queries from memory.
type Client struct {
	vaultPath   string
	dailyFolder string                  // e.g. "daily notes"
	pages       map[string]*cachedPage  // lowercase name → page
	backlinks   map[string][]backlink   // lowercase target → backlinks
	blockIndex  map[string]*blockLookup // uuid → block + page
	searchIndex *SearchIndex            // inverted index for full-text search
	mu          sync.RWMutex            // protects all maps above
	watcher     *fsnotify.Watcher       // file system watcher
	vaultStore  *VaultStore             // optional persistent store adapter (nil when no .dewey/)
}

// blockLookup stores a block and its page for UUID-based retrieval.
type blockLookup struct {
	block *types.BlockEntity
	page  string
}

// Option configures a vault Client.
type Option func(*Client)

// WithDailyFolder sets the daily notes subfolder (default: "daily notes").
func WithDailyFolder(folder string) Option {
	return func(c *Client) { c.dailyFolder = folder }
}

// WithStore enables persistent indexing via the given store.Store.
// When set, the vault client persists index changes to SQLite alongside
// in-memory updates. Pass nil to disable persistence (default behavior).
func WithStore(s *store.Store) Option {
	return func(c *Client) {
		if s != nil {
			c.vaultStore = NewVaultStore(s, c.vaultPath, "disk-local")
		}
	}
}

// New creates a new Obsidian vault client. Call Load() to index the vault.
func New(vaultPath string, opts ...Option) *Client {
	c := &Client{
		vaultPath:   vaultPath,
		dailyFolder: "daily notes",
		pages:       make(map[string]*cachedPage),
		backlinks:   make(map[string][]backlink),
		blockIndex:  make(map[string]*blockLookup),
		searchIndex: NewSearchIndex(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Store returns the VaultStore adapter, or nil if persistence is not configured.
func (c *Client) Store() *VaultStore {
	return c.vaultStore
}

// Load reads all .md files in the vault and builds the in-memory index.
func (c *Client) Load() error {
	return filepath.Walk(c.vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip hidden directories (.obsidian, .git, etc).
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		relPath, _ := filepath.Rel(c.vaultPath, path)
		c.indexFile(relPath, string(content), info)
		return nil
	})
}

// indexFile parses a single markdown file and adds it to the index.
// indexFile parses and indexes a file, acquiring its own locks.
// Do NOT call from within a locked context — use indexFileCore instead.
func (c *Client) indexFile(relPath, content string, info os.FileInfo) {
	page := c.parseFile(relPath, content, info)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applyPageIndex(page)
}

// indexFileCore parses and indexes a file. Caller must hold c.mu for write.
func (c *Client) indexFileCore(relPath, content string, info os.FileInfo) {
	page := c.parseFile(relPath, content, info)
	c.applyPageIndex(page)
}

// parseFile creates a cachedPage from file content (no locking needed).
func (c *Client) parseFile(relPath, content string, info os.FileInfo) *cachedPage {
	name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
	lowerName := strings.ToLower(name)

	props, body := parseFrontmatter(content)

	isJournal := false
	if c.dailyFolder != "" {
		prefix := strings.ToLower(c.dailyFolder) + "/"
		isJournal = strings.HasPrefix(lowerName, prefix)
	}

	entity := types.PageEntity{
		Name:         name,
		OriginalName: name,
		Properties:   props,
		Journal:      isJournal,
		CreatedAt:    info.ModTime().UnixMilli(),
		UpdatedAt:    info.ModTime().UnixMilli(),
	}

	blocks := parseMarkdownBlocks(relPath, body)

	return &cachedPage{
		entity:    entity,
		lowerName: lowerName,
		filePath:  relPath,
		blocks:    blocks,
	}
}

// applyPageIndex stores a parsed page into all indices. Caller must hold c.mu for write.
func (c *Client) applyPageIndex(page *cachedPage) {
	// Handle aliases.
	if page.entity.Properties != nil {
		if aliases, ok := page.entity.Properties["aliases"]; ok {
			if aliasList, ok := aliases.([]any); ok {
				for _, a := range aliasList {
					if s, ok := a.(string); ok {
						c.pages[strings.ToLower(s)] = page
					}
				}
			}
		}
	}

	c.pages[page.lowerName] = page
	c.indexBlocksLocked(page.blocks, page.entity.Name)
}

// indexBlocksLocked recursively adds blocks to the UUID index (caller must hold c.mu).
func (c *Client) indexBlocksLocked(blocks []types.BlockEntity, pageName string) {
	for i := range blocks {
		c.blockIndex[blocks[i].UUID] = &blockLookup{
			block: &blocks[i],
			page:  pageName,
		}
		if len(blocks[i].Children) > 0 {
			c.indexBlocksLocked(blocks[i].Children, pageName)
		}
	}
}

// BuildBacklinks must be called after Load() to build the reverse link index.
func (c *Client) BuildBacklinks() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rebuildLinksLocked()
}

// rebuildLinksLocked rebuilds backlinks and search index. Caller must hold c.mu.
func (c *Client) rebuildLinksLocked() {
	c.backlinks = buildBacklinks(c.pages)
	if c.searchIndex != nil {
		c.searchIndex.BuildFrom(c.pages)
	}
}

// removePageFromIndexLocked removes a page without locking. Caller must hold c.mu.
func (c *Client) removePageFromIndexLocked(lowerName string) {
	cached, ok := c.pages[lowerName]
	if !ok {
		return
	}
	c.removeBlocksFromIndexLocked(cached.blocks)
	for key, page := range c.pages {
		if key != lowerName && page.lowerName == lowerName {
			delete(c.pages, key)
		}
	}
	delete(c.pages, lowerName)
}

// Watch starts watching the vault directory for file changes and automatically
// re-indexes modified files. Runs in a goroutine and logs errors to stderr.
func (c *Client) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	c.watcher = watcher

	// Add vault root and all subdirectories (recursive).
	if err := c.addWatcherDirs(c.vaultPath); err != nil {
		_ = c.watcher.Close()
		return fmt.Errorf("add directories to watcher: %w", err)
	}

	// Start watching in a goroutine.
	go c.watchLoop()
	return nil
}

// addWatcherDirs recursively adds directories to the watcher, skipping hidden dirs.
func (c *Client) addWatcherDirs(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			return nil
		}
		// Skip hidden directories (.obsidian, .git, etc).
		if strings.HasPrefix(info.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		return c.watcher.Add(path)
	})
}

// watchLoop processes file system events.
func (c *Client) watchLoop() {
	for {
		select {
		case event, ok := <-c.watcher.Events:
			if !ok {
				return // watcher closed
			}
			c.handleEvent(event)
		case err, ok := <-c.watcher.Errors:
			if !ok {
				return // watcher closed
			}
			logger.Error("watcher error", "err", err)
		}
	}
}

// handleEvent processes a single fsnotify event by dispatching to
// per-event-type handlers after shared pre-checks.
func (c *Client) handleEvent(event fsnotify.Event) {
	// Skip non-.md files.
	if !strings.HasSuffix(event.Name, ".md") {
		return
	}

	// Skip hidden directories.
	if strings.Contains(event.Name, "/.") {
		return
	}

	relPath, err := filepath.Rel(c.vaultPath, event.Name)
	if err != nil {
		logger.Error("failed to get relative path", "file", event.Name, "err", err)
		return
	}

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create, event.Op&fsnotify.Write == fsnotify.Write:
		c.handleFileWrite(relPath, event.Name)
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		c.handleFileRemove(relPath)
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		c.handleFileRename(relPath)
	}
}

// handleFileWrite re-indexes a created or modified markdown file and
// optionally persists it to the store.
func (c *Client) handleFileWrite(relPath, absPath string) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		logger.Error("failed to read file", "file", absPath, "err", err)
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		logger.Error("failed to stat file", "file", absPath, "err", err)
		return
	}
	c.indexFile(filepath.ToSlash(relPath), string(content), info)
	c.BuildBacklinks()
	c.persistPageToStore(relPath)
	logger.Info("reindexed", "file", relPath)
}

// handleFileRemove removes a deleted markdown file from the in-memory
// index and optionally from the store.
func (c *Client) handleFileRemove(relPath string) {
	name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
	lowerName := strings.ToLower(name)
	c.removePageFromIndex(lowerName)
	c.BuildBacklinks()
	c.removePageFromStore(name, relPath)
	logger.Info("removed from index", "file", relPath)
}

// handleFileRename treats a renamed file as a removal. The new file name
// will trigger a separate Create event.
func (c *Client) handleFileRename(relPath string) {
	name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
	lowerName := strings.ToLower(name)
	c.removePageFromIndex(lowerName)
	c.BuildBacklinks()
	c.removePageFromStore(name, relPath)
	logger.Info("removed from index (rename)", "file", relPath)
}

// persistPageToStore writes the page at relPath to the persistent store
// if one is configured. This is a no-op when vaultStore is nil.
func (c *Client) persistPageToStore(relPath string) {
	if c.vaultStore == nil {
		return
	}
	pageName := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
	lowerName := strings.ToLower(pageName)
	c.mu.RLock()
	page, ok := c.pages[lowerName]
	c.mu.RUnlock()
	if ok {
		if err := c.vaultStore.PersistPage(page); err != nil {
			logger.Warn("failed to persist page to store", "file", relPath, "err", err)
		}
	}
}

// removePageFromStore removes the named page from the persistent store
// if one is configured. This is a no-op when vaultStore is nil.
func (c *Client) removePageFromStore(pageName, relPath string) {
	if c.vaultStore == nil {
		return
	}
	if err := c.vaultStore.RemovePage(pageName); err != nil {
		logger.Warn("failed to remove page from store", "file", relPath, "err", err)
	}
}

// Close stops the file watcher.
func (c *Client) Close() error {
	if c.watcher != nil {
		return c.watcher.Close()
	}
	return nil
}

// Reload forces a full vault re-index.
func (c *Client) Reload() error {
	// Clear existing indices.
	c.mu.Lock()
	c.pages = make(map[string]*cachedPage)
	c.blockIndex = make(map[string]*blockLookup)
	c.mu.Unlock()

	// Reload all files.
	if err := c.Load(); err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	c.BuildBacklinks()
	return nil
}

// --- backend.Backend implementation ---

func (c *Client) Ping(_ context.Context) error {
	info, err := os.Stat(c.vaultPath)
	if err != nil {
		return fmt.Errorf("vault path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vault path is not a directory: %s", c.vaultPath)
	}
	return nil
}

func (c *Client) GetAllPages(_ context.Context) ([]types.PageEntity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]bool)
	var pages []types.PageEntity
	for _, page := range c.pages {
		if seen[page.lowerName] {
			continue // skip alias duplicates
		}
		seen[page.lowerName] = true
		pages = append(pages, page.entity)
	}
	return pages, nil
}

func (c *Client) GetPage(_ context.Context, nameOrID any) (*types.PageEntity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	name := fmt.Sprint(nameOrID)
	page, ok := c.pages[strings.ToLower(name)]
	if !ok {
		return nil, nil
	}
	return &page.entity, nil
}

func (c *Client) GetPageBlocksTree(_ context.Context, nameOrID any) ([]types.BlockEntity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	name := fmt.Sprint(nameOrID)
	page, ok := c.pages[strings.ToLower(name)]
	if !ok {
		return nil, nil
	}
	return page.blocks, nil
}

func (c *Client) GetBlock(_ context.Context, uuid string, opts ...map[string]any) (*types.BlockEntity, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lookup, ok := c.blockIndex[uuid]
	if !ok {
		return nil, nil
	}

	// Build a copy with page reference.
	block := *lookup.block
	block.Page = &types.PageRef{Name: lookup.page}
	return &block, nil
}

func (c *Client) GetPageLinkedReferences(_ context.Context, nameOrID any) (json.RawMessage, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	name := fmt.Sprint(nameOrID)
	key := strings.ToLower(name)

	links, ok := c.backlinks[key]
	if !ok || len(links) == 0 {
		return json.Marshal([]any{})
	}

	// Group by source page.
	grouped := make(map[string][]types.BlockSummary)
	for _, bl := range links {
		grouped[bl.fromPage] = append(grouped[bl.fromPage], bl.block)
	}

	// Format as array of [PageEntity, blocks] pairs (matching Logseq format).
	var result []any
	for page, blocks := range grouped {
		// Build a PageEntity-like object so navigate.getBacklinks can unmarshal it.
		pageObj := map[string]any{
			"name":          page,
			"original-name": page,
		}
		// Look up the original-case name if available.
		if cached, ok := c.pages[page]; ok {
			pageObj["name"] = cached.entity.Name
			pageObj["original-name"] = cached.entity.OriginalName
		}
		result = append(result, []any{pageObj, blocks})
	}

	return json.Marshal(result)
}

func (c *Client) DatascriptQuery(_ context.Context, query string, inputs ...any) (json.RawMessage, error) {
	return nil, ErrNotSupported
}

// --- Write operations ---

func (c *Client) CreatePage(_ context.Context, name string, properties map[string]any, opts map[string]any) (*types.PageEntity, error) {
	if len(name) > 255 || strings.ContainsAny(name, "\x00") {
		return nil, fmt.Errorf("invalid page name: too long or contains null bytes")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	lowerName := strings.ToLower(name)
	if _, exists := c.pages[lowerName]; exists {
		return nil, fmt.Errorf("page already exists: %s", name)
	}

	relPath := name + ".md"
	absPath, err := c.safePath(relPath)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	var content string
	if len(properties) > 0 {
		content = renderFrontmatter(properties)
	}

	if err := atomicWrite(absPath, content); err != nil {
		return nil, fmt.Errorf("write page: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat new file: %w", err)
	}
	c.indexFileCore(relPath, content, info)
	c.rebuildLinksLocked()

	page := c.pages[lowerName]
	if page == nil {
		return nil, fmt.Errorf("index failed for %s", name)
	}
	return &page.entity, nil
}

func (c *Client) AppendBlockInPage(_ context.Context, page string, content string) (*types.BlockEntity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lowerName := strings.ToLower(page)
	cached, exists := c.pages[lowerName]

	var absPath string
	var relPath string
	if !exists {
		relPath = page + ".md"
		var err error
		absPath, err = c.safePath(relPath)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}
		if err := atomicWrite(absPath, ""); err != nil {
			return nil, fmt.Errorf("create page: %w", err)
		}
		info, _ := os.Stat(absPath)
		c.indexFileCore(relPath, "", info)
		cached = c.pages[lowerName]
	} else {
		relPath = cached.filePath
		var err error
		absPath, err = c.safePath(relPath)
		if err != nil {
			return nil, err
		}
	}

	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	blockUUID, cleanContent := extractUUID(content)
	if blockUUID == "" {
		blockUUID = generateRandomUUID()
		content = embedUUID(cleanContent, blockUUID)
	} else {
		content = cleanContent
	}

	newContent := string(existing)
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += content + "\n"

	if err := atomicWrite(absPath, newContent); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	fileRelPath := relPath
	if cached != nil {
		fileRelPath = cached.filePath
	}
	c.indexFileCore(fileRelPath, newContent, info)
	c.rebuildLinksLocked()

	cached = c.pages[lowerName]
	if cached != nil && len(cached.blocks) > 0 {
		last := lastBlock(cached.blocks)
		if last != nil {
			return last, nil
		}
	}

	return &types.BlockEntity{UUID: blockUUID, Content: cleanContent}, nil
}

func (c *Client) PrependBlockInPage(_ context.Context, page string, content string) (*types.BlockEntity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lowerName := strings.ToLower(page)
	cached, exists := c.pages[lowerName]

	blockUUID, cleanContent := extractUUID(content)
	if blockUUID == "" {
		blockUUID = generateRandomUUID()
		content = embedUUID(cleanContent, blockUUID)
	} else {
		content = cleanContent
	}

	var absPath string
	if !exists {
		relPath := page + ".md"
		var err error
		absPath, err = c.safePath(relPath)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}
		if err := atomicWrite(absPath, content+"\n"); err != nil {
			return nil, fmt.Errorf("create page: %w", err)
		}
		info, _ := os.Stat(absPath)
		c.indexFileCore(relPath, content+"\n", info)
		c.rebuildLinksLocked()
		cached = c.pages[lowerName]
		if cached != nil && len(cached.blocks) > 0 {
			return &cached.blocks[0], nil
		}
		return &types.BlockEntity{UUID: blockUUID, Content: cleanContent}, nil
	}

	var err error
	absPath, err = c.safePath(cached.filePath)
	if err != nil {
		return nil, err
	}

	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	props, body := parseFrontmatter(string(existing))
	var newContent string
	if props != nil {
		newContent = renderFrontmatter(props) + content + "\n" + body
	} else {
		newContent = content + "\n" + string(existing)
	}

	if err := atomicWrite(absPath, newContent); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(cached.filePath, newContent, info)
	c.rebuildLinksLocked()

	cached = c.pages[lowerName]
	if cached != nil && len(cached.blocks) > 0 {
		return &cached.blocks[0], nil
	}
	return &types.BlockEntity{UUID: blockUUID, Content: cleanContent}, nil
}

func (c *Client) InsertBlock(_ context.Context, srcBlock any, content string, opts map[string]any) (*types.BlockEntity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	parentUUID := fmt.Sprint(srcBlock)
	lookup, ok := c.blockIndex[parentUUID]
	if !ok {
		return nil, fmt.Errorf("parent block not found: %s", parentUUID)
	}

	pageName := lookup.page
	lowerName := strings.ToLower(pageName)
	cached, ok := c.pages[lowerName]
	if !ok {
		return nil, fmt.Errorf("page not found for block: %s", pageName)
	}

	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return nil, err
	}
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	parentContent := lookup.block.Content
	fileStr := string(existing)
	idx := strings.Index(fileStr, parentContent)
	if idx < 0 {
		return nil, fmt.Errorf("could not locate parent block content in file")
	}
	insertPos := idx + len(parentContent)

	childContent := content
	if lvl := headingLevel(strings.SplitN(parentContent, "\n", 2)[0]); lvl > 0 && lvl < 6 {
		if headingLevel(strings.SplitN(content, "\n", 2)[0]) == 0 {
			prefix := strings.Repeat("#", lvl+1) + " "
			childContent = prefix + content
		}
	}

	blockUUID, cleanContent := extractUUID(childContent)
	if blockUUID == "" {
		blockUUID = generateRandomUUID()
		childContent = embedUUID(cleanContent, blockUUID)
	} else {
		childContent = cleanContent
	}

	newContent := fileStr[:insertPos] + "\n" + childContent + fileStr[insertPos:]

	if err := atomicWrite(absPath, newContent); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(cached.filePath, newContent, info)
	c.rebuildLinksLocked()

	cached = c.pages[lowerName]
	if cached != nil {
		block := findBlockByContent(cached.blocks, childContent)
		if block != nil {
			return block, nil
		}
	}

	return &types.BlockEntity{UUID: blockUUID, Content: cleanContent}, nil
}

func (c *Client) UpdateBlock(_ context.Context, uuid string, content string, opts ...map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	lookup, ok := c.blockIndex[uuid]
	if !ok {
		return fmt.Errorf("block not found: %s", uuid)
	}

	pageName := lookup.page
	lowerName := strings.ToLower(pageName)
	cached, ok := c.pages[lowerName]
	if !ok {
		return fmt.Errorf("page not found: %s", pageName)
	}

	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	oldContent := lookup.block.Content
	fileStr := string(existing)

	// The file might have the UUID embedded, so we need to search for it
	// We'll look for the old content with or without UUID comment
	oldContentWithUUID := embedUUID(oldContent, uuid)

	var oldInFile, newInFile string
	if strings.Contains(fileStr, oldContentWithUUID) {
		// File has UUID embedded
		oldInFile = oldContentWithUUID
		// Check if new content has a UUID, preserve the original UUID
		providedUUID, cleanContent := extractUUID(content)
		if providedUUID == "" || providedUUID == uuid {
			// No UUID provided or same UUID, preserve the block's UUID
			newInFile = embedUUID(cleanContent, uuid)
		} else {
			// Different UUID provided, use the new content as-is
			newInFile = content
		}
	} else if strings.Contains(fileStr, oldContent) {
		// File doesn't have UUID embedded (old format)
		oldInFile = oldContent
		// Add UUID to new content
		cleanContent := content
		if _, extractedClean := extractUUID(content); extractedClean != content {
			cleanContent = extractedClean
		}
		newInFile = embedUUID(cleanContent, uuid)
	} else {
		return fmt.Errorf("block content not found in file (may have been modified externally)")
	}

	newContent := strings.Replace(fileStr, oldInFile, newInFile, 1)
	if err := atomicWrite(absPath, newContent); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(cached.filePath, newContent, info)
	c.rebuildLinksLocked()

	return nil
}

func (c *Client) RemoveBlock(_ context.Context, uuid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	lookup, ok := c.blockIndex[uuid]
	if !ok {
		return fmt.Errorf("block not found: %s", uuid)
	}

	pageName := lookup.page
	lowerName := strings.ToLower(pageName)
	cached, ok := c.pages[lowerName]
	if !ok {
		return fmt.Errorf("page not found: %s", pageName)
	}

	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	oldContent := lookup.block.Content
	fileStr := string(existing)

	oldContentWithUUID := embedUUID(oldContent, uuid)
	var newContent string

	if strings.Contains(fileStr, oldContentWithUUID+"\n") {
		newContent = strings.Replace(fileStr, oldContentWithUUID+"\n", "", 1)
	} else if strings.Contains(fileStr, oldContentWithUUID) {
		newContent = strings.Replace(fileStr, oldContentWithUUID, "", 1)
	} else if strings.Contains(fileStr, oldContent+"\n") {
		newContent = strings.Replace(fileStr, oldContent+"\n", "", 1)
	} else if strings.Contains(fileStr, oldContent) {
		newContent = strings.Replace(fileStr, oldContent, "", 1)
	} else {
		return fmt.Errorf("block content not found in file")
	}

	if err := atomicWrite(absPath, newContent); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(cached.filePath, newContent, info)
	c.rebuildLinksLocked()

	return nil
}

func (c *Client) DeletePage(_ context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	lowerName := strings.ToLower(name)
	cached, ok := c.pages[lowerName]
	if !ok {
		return fmt.Errorf("page not found: %s", name)
	}

	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	c.removePageFromIndexLocked(lowerName)
	c.rebuildLinksLocked()

	dir := filepath.Dir(absPath)
	vaultAbs, _ := filepath.Abs(c.vaultPath)
	for dir != vaultAbs {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			logger.Error("failed to remove empty dir", "dir", dir, "err", err)
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

func (c *Client) RenamePage(_ context.Context, oldName, newName string) error {
	if len(newName) > 255 || strings.ContainsAny(newName, "\x00") {
		return fmt.Errorf("invalid page name: too long or contains null bytes")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	lowerOld := strings.ToLower(oldName)
	cached, ok := c.pages[lowerOld]
	if !ok {
		return fmt.Errorf("page not found: %s", oldName)
	}

	lowerNew := strings.ToLower(newName)
	if _, exists := c.pages[lowerNew]; exists {
		return fmt.Errorf("target page already exists: %s", newName)
	}

	oldPath, err := c.safePath(cached.filePath)
	if err != nil {
		return err
	}
	newRelPath := newName + ".md"
	newAbsPath, err := c.safePath(newRelPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(newAbsPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.Rename(oldPath, newAbsPath); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}

	// Update all [[links]] across the vault — log every error.
	if errs := c.updateLinksAcrossVaultLocked(oldName, newName); len(errs) > 0 {
		for _, e := range errs {
			logger.Error("link update error during rename", "err", e)
		}
	}

	c.removePageFromIndexLocked(lowerOld)
	content, err := os.ReadFile(newAbsPath)
	if err == nil {
		info, _ := os.Stat(newAbsPath)
		c.indexFileCore(newRelPath, string(content), info)
	}
	c.rebuildLinksLocked()

	dir := filepath.Dir(oldPath)
	vaultAbs, _ := filepath.Abs(c.vaultPath)
	for dir != vaultAbs {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			logger.Error("failed to remove empty dir", "dir", dir, "err", err)
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

// removePageFromIndex removes a page and its blocks from all indices.
// Acquires its own lock — do NOT call from within a locked context.
func (c *Client) removePageFromIndex(lowerName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removePageFromIndexLocked(lowerName)
}

// removeBlocksFromIndexLocked recursively removes blocks from the UUID index (caller must hold c.mu).
func (c *Client) removeBlocksFromIndexLocked(blocks []types.BlockEntity) {
	for _, b := range blocks {
		delete(c.blockIndex, b.UUID)
		if len(b.Children) > 0 {
			c.removeBlocksFromIndexLocked(b.Children)
		}
	}
}

// updateLinksAcrossVaultLocked replaces [[oldName]] with [[newName]] in all files.
// Caller must hold c.mu. Returns errors from failed writes.
func (c *Client) updateLinksAcrossVaultLocked(oldName, newName string) []error {
	oldLink := "[[" + oldName + "]]"
	newLink := "[[" + newName + "]]"
	lowerOld := strings.ToLower(oldName)

	var errs []error
	seen := make(map[string]bool)
	for _, page := range c.pages {
		if seen[page.lowerName] {
			continue
		}
		seen[page.lowerName] = true

		// Skip the page being renamed — its file was already moved.
		if page.lowerName == lowerOld {
			continue
		}

		absPath, err := c.safePath(page.filePath)
		if err != nil {
			errs = append(errs, fmt.Errorf("skip %s: %w", page.filePath, err))
			continue
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", page.filePath, err))
			continue
		}

		fileStr := string(content)
		if !strings.Contains(fileStr, oldLink) {
			continue
		}

		updated := strings.ReplaceAll(fileStr, oldLink, newLink)
		if err := atomicWrite(absPath, updated); err != nil {
			errs = append(errs, fmt.Errorf("write %s: %w", page.filePath, err))
			continue
		}

		info, _ := os.Stat(absPath)
		c.indexFileCore(page.filePath, updated, info)
	}
	return errs
}

func (c *Client) MoveBlock(_ context.Context, uuid string, targetUUID string, opts map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	srcLookup, ok := c.blockIndex[uuid]
	if !ok {
		return fmt.Errorf("source block not found: %s", uuid)
	}
	tgtLookup, ok := c.blockIndex[targetUUID]
	if !ok {
		return fmt.Errorf("target block not found: %s", targetUUID)
	}

	srcPage := srcLookup.page
	tgtPage := tgtLookup.page
	srcContent := srcLookup.block.Content
	tgtContent := tgtLookup.block.Content
	before := parseMoveOptions(opts)

	if strings.EqualFold(srcPage, tgtPage) {
		return c.moveBlockSamePage(srcPage, srcContent, tgtContent, before)
	}
	return c.moveBlockCrossPage(srcPage, tgtPage, srcContent, tgtContent, before)
}

// parseMoveOptions extracts the "before" flag from MoveBlock options.
// Returns false if opts is nil or "before" is not set.
func parseMoveOptions(opts map[string]any) bool {
	if opts == nil {
		return false
	}
	b, ok := opts["before"]
	if !ok {
		return false
	}
	before, _ := b.(bool)
	return before
}

// moveBlockSamePage moves a block within the same page by removing its content
// and re-inserting it before or after the target block's content.
// Caller must hold c.mu for write.
func (c *Client) moveBlockSamePage(pageName, srcContent, tgtContent string, before bool) error {
	lowerName := strings.ToLower(pageName)
	cached, ok := c.pages[lowerName]
	if !ok {
		return fmt.Errorf("page not found: %s", pageName)
	}

	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	fileStr := string(existing)
	fileStr = strings.Replace(fileStr, srcContent+"\n", "", 1)

	fileStr = insertContentRelative(fileStr, srcContent, tgtContent, before)

	if err := atomicWrite(absPath, fileStr); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	info, _ := os.Stat(absPath)
	c.indexFileCore(cached.filePath, fileStr, info)
	c.rebuildLinksLocked()
	return nil
}

// moveBlockCrossPage moves a block from one page to another by removing its
// content from the source file and inserting it before or after the target
// block's content in the target file.
// Caller must hold c.mu for write.
func (c *Client) moveBlockCrossPage(srcPage, tgtPage, srcContent, tgtContent string, before bool) error {
	// Cross-page move.
	srcLower := strings.ToLower(srcPage)
	tgtLower := strings.ToLower(tgtPage)
	srcCached, ok := c.pages[srcLower]
	if !ok {
		return fmt.Errorf("source page not found: %s", srcPage)
	}
	tgtCached, ok := c.pages[tgtLower]
	if !ok {
		return fmt.Errorf("target page not found: %s", tgtPage)
	}

	srcStr, err := c.removeContentFromFile(srcCached, srcContent)
	if err != nil {
		return err
	}

	tgtStr, err := c.insertContentInFile(tgtCached, srcContent, tgtContent, before)
	if err != nil {
		return err
	}

	srcAbsPath, err := c.safePath(srcCached.filePath)
	if err != nil {
		return err
	}
	tgtAbsPath, err := c.safePath(tgtCached.filePath)
	if err != nil {
		return err
	}

	srcInfo, _ := os.Stat(srcAbsPath)
	c.indexFileCore(srcCached.filePath, srcStr, srcInfo)
	tgtInfo, _ := os.Stat(tgtAbsPath)
	c.indexFileCore(tgtCached.filePath, tgtStr, tgtInfo)
	c.rebuildLinksLocked()

	return nil
}

// removeContentFromFile removes a block's content from its page file.
// Tries removing with trailing newline first, then without.
// Caller must hold c.mu for write.
func (c *Client) removeContentFromFile(cached *cachedPage, content string) (string, error) {
	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return "", err
	}
	srcFile, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read source file: %w", err)
	}
	srcStr := strings.Replace(string(srcFile), content+"\n", "", 1)
	if srcStr == string(srcFile) {
		srcStr = strings.Replace(string(srcFile), content, "", 1)
	}
	if err := atomicWrite(absPath, srcStr); err != nil {
		return "", fmt.Errorf("write source file: %w", err)
	}
	return srcStr, nil
}

// insertContentInFile inserts block content before or after target content in a page file.
// Caller must hold c.mu for write.
func (c *Client) insertContentInFile(cached *cachedPage, srcContent, tgtContent string, before bool) (string, error) {
	absPath, err := c.safePath(cached.filePath)
	if err != nil {
		return "", err
	}
	tgtFile, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read target file: %w", err)
	}
	tgtStr := insertContentRelative(string(tgtFile), srcContent, tgtContent, before)
	if err := atomicWrite(absPath, tgtStr); err != nil {
		return "", fmt.Errorf("write target file: %w", err)
	}
	return tgtStr, nil
}

// insertContentRelative inserts srcContent before or after tgtContent in fileStr.
func insertContentRelative(fileStr, srcContent, tgtContent string, before bool) string {
	if before {
		return strings.Replace(fileStr, tgtContent, srcContent+"\n"+tgtContent, 1)
	}
	return strings.Replace(fileStr, tgtContent, tgtContent+"\n"+srcContent, 1)
}

// --- Write helpers ---

// atomicWrite writes content to a file via a temp file rename.
func atomicWrite(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// lastBlock returns the last block in a tree (depth-first).
func lastBlock(blocks []types.BlockEntity) *types.BlockEntity {
	if len(blocks) == 0 {
		return nil
	}
	last := &blocks[len(blocks)-1]
	if len(last.Children) > 0 {
		child := lastBlock(last.Children)
		if child != nil {
			return child
		}
	}
	return last
}

// findBlockByContent searches for a block with matching content.
func findBlockByContent(blocks []types.BlockEntity, content string) *types.BlockEntity {
	for i := range blocks {
		if blocks[i].Content == content {
			return &blocks[i]
		}
		if len(blocks[i].Children) > 0 {
			found := findBlockByContent(blocks[i].Children, content)
			if found != nil {
				return found
			}
		}
	}
	return nil
}

// --- Optional search interfaces ---

// FindBlocksByTag scans all pages for blocks containing the given #tag.
// Implements backend.TagSearcher.
func (c *Client) FindBlocksByTag(_ context.Context, tag string, includeChildren bool) ([]backend.TagResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tagLower := strings.ToLower(tag)
	var results []backend.TagResult

	seen := make(map[string]bool)
	for _, page := range c.pages {
		if seen[page.lowerName] {
			continue
		}
		seen[page.lowerName] = true

		var matches []types.BlockEntity
		findTagInBlocks(page.blocks, tagLower, &matches)
		if len(matches) > 0 {
			results = append(results, backend.TagResult{
				Page:   page.entity.Name,
				Blocks: matches,
			})
		}
	}

	return results, nil
}

// findTagInBlocks recursively searches blocks for a tag.
func findTagInBlocks(blocks []types.BlockEntity, tagLower string, matches *[]types.BlockEntity) {
	for _, b := range blocks {
		parsed := parser.Parse(b.Content)
		for _, t := range parsed.Tags {
			if strings.ToLower(t) == tagLower {
				*matches = append(*matches, b)
				break
			}
		}
		if len(b.Children) > 0 {
			findTagInBlocks(b.Children, tagLower, matches)
		}
	}
}

// FindByProperty scans all pages for matching frontmatter properties.
// Implements backend.PropertySearcher.
func (c *Client) FindByProperty(_ context.Context, key, value, operator string) ([]backend.PropertyResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []backend.PropertyResult

	seen := make(map[string]bool)
	for _, page := range c.pages {
		if seen[page.lowerName] {
			continue
		}
		seen[page.lowerName] = true

		if page.entity.Properties == nil {
			continue
		}

		propVal, ok := page.entity.Properties[key]
		if !ok {
			continue
		}

		if value == "" {
			// Just checking if property exists.
			results = append(results, backend.PropertyResult{
				Type:       "page",
				Name:       page.entity.Name,
				Properties: page.entity.Properties,
			})
			continue
		}

		propStr := fmt.Sprint(propVal)
		match := false
		switch operator {
		case "eq", "":
			match = strings.EqualFold(propStr, value)
		case "contains":
			match = strings.Contains(strings.ToLower(propStr), strings.ToLower(value))
		case "gt":
			match = propStr > value
		case "lt":
			match = propStr < value
		}

		if match {
			results = append(results, backend.PropertyResult{
				Type:       "page",
				Name:       page.entity.Name,
				Properties: page.entity.Properties,
			})
		}
	}

	return results, nil
}

// SearchJournals scans daily notes for matching content.
// Implements backend.JournalSearcher.
func (c *Client) SearchJournals(_ context.Context, query, from, to string) ([]backend.JournalResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	queryLower := strings.ToLower(query)
	var results []backend.JournalResult

	seen := make(map[string]bool)
	for _, page := range c.pages {
		if seen[page.lowerName] {
			continue
		}
		seen[page.lowerName] = true

		if !page.entity.Journal {
			continue
		}

		// Extract date from page name (last path segment).
		parts := strings.Split(page.entity.Name, "/")
		date := parts[len(parts)-1]

		// Filter by date range.
		if from != "" && date < from {
			continue
		}
		if to != "" && date > to {
			continue
		}

		// Search blocks for query.
		var matches []types.BlockEntity
		searchBlocksForText(page.blocks, queryLower, &matches)
		if len(matches) > 0 {
			results = append(results, backend.JournalResult{
				Date:   date,
				Page:   page.entity.Name,
				Blocks: matches,
			})
		}
	}

	return results, nil
}

// FullTextSearch uses the inverted index for efficient full-text search.
// Implements backend.FullTextSearcher.
func (c *Client) FullTextSearch(_ context.Context, query string, limit int) ([]backend.SearchHit, error) {
	if c.searchIndex == nil {
		return nil, fmt.Errorf("search index not initialized")
	}

	results := c.searchIndex.Search(query, limit)

	hits := make([]backend.SearchHit, len(results))
	for i, r := range results {
		hits[i] = backend.SearchHit{
			PageName: r.PageName,
			UUID:     r.UUID,
			Content:  r.Content,
		}
	}
	return hits, nil
}

// searchBlocksForText recursively searches blocks for text content.
func searchBlocksForText(blocks []types.BlockEntity, queryLower string, matches *[]types.BlockEntity) {
	for _, b := range blocks {
		if strings.Contains(strings.ToLower(b.Content), queryLower) {
			*matches = append(*matches, b)
		}
		if len(b.Children) > 0 {
			searchBlocksForText(b.Children, queryLower, matches)
		}
	}
}
