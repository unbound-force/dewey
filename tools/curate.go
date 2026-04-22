package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/curate"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// curateLogger is the package-level structured logger for curate tool operations.
var curateLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/tools/curate",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Curate implements the dewey_curate MCP tool for running the curation
// pipeline to extract structured knowledge from indexed sources.
//
// Design decision: Follows the same pattern as Compile — dependencies are
// injected, synth may be nil (returns prompts), and a mutex prevents
// concurrent curation. The indexMutex is shared with indexing tools to
// prevent curation during indexing (per FR-020).
type Curate struct {
	mu        *sync.Mutex
	store     *store.Store
	embedder  embed.Embedder
	synth     llm.Synthesizer
	vaultPath string
}

// NewCurate creates a new Curate tool handler with the given dependencies.
// The store must be non-nil. The synth may be nil — the tool returns
// extraction prompts when no synthesizer is available (MCP mode).
// The mu parameter is the shared index mutex; when non-nil, curation
// acquires it to prevent concurrent indexing operations.
func NewCurate(s *store.Store, e embed.Embedder, synth llm.Synthesizer, vaultPath string, mu *sync.Mutex) *Curate {
	if mu == nil {
		mu = &sync.Mutex{}
	}
	return &Curate{store: s, embedder: e, synth: synth, vaultPath: vaultPath, mu: mu}
}

// Curate handles the dewey_curate MCP tool. Reads knowledge-stores.yaml,
// runs the curation pipeline for each configured store (or a named store),
// and returns results.
//
// When synthesizer is available: runs the full pipeline and returns results.
// When synthesizer is nil: returns extraction prompts for external synthesis.
//
// Returns an MCP error result (not a Go error) for configuration issues,
// missing stores, or concurrent operation conflicts.
func (c *Curate) Curate(ctx context.Context, req *mcp.CallToolRequest, input types.CurateInput) (*mcp.CallToolResult, any, error) {
	if c.store == nil {
		return errorResult("curate requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	// Non-blocking mutual exclusion: if indexing is in progress,
	// return immediately with an error rather than queuing.
	if !c.mu.TryLock() {
		return errorResult("Curation cannot run while indexing is in progress. Try again later."), nil, nil
	}
	defer c.mu.Unlock()

	// Load knowledge stores config.
	deweyDir := filepath.Join(c.vaultPath, deweyWorkspaceDir)
	ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	stores, err := curate.LoadKnowledgeStoresConfig(ksPath)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to load knowledge stores config: %v", err)), nil, nil
	}
	if len(stores) == 0 {
		return errorResult("No knowledge stores configured. Create .uf/dewey/knowledge-stores.yaml or run 'dewey init'."), nil, nil
	}

	// Filter to named store if specified.
	if input.Store != "" {
		var found *curate.StoreConfig
		for i := range stores {
			if stores[i].Name == input.Store {
				found = &stores[i]
				break
			}
		}
		if found == nil {
			return errorResult(fmt.Sprintf("Knowledge store %q not found in configuration.", input.Store)), nil, nil
		}
		stores = []curate.StoreConfig{*found}
	}

	// Determine incremental mode (default: true).
	incremental := true
	if input.Incremental != nil {
		incremental = *input.Incremental
	}

	// Create pipeline.
	pipeline := curate.NewPipeline(c.store, c.synth, c.embedder, c.vaultPath)

	// If no synthesizer, return prompts for all stores.
	if c.synth == nil || !c.synth.Available() {
		return c.returnPromptMode(ctx, pipeline, stores)
	}

	// Run curation for each store.
	var results []curate.CurationResult
	totalFiles := 0

	for _, cfg := range stores {
		curateLogger.Info("curating store", "store", cfg.Name, "sources", len(cfg.Sources))

		var filesCreated int
		var curateErr error
		if incremental {
			filesCreated, curateErr = pipeline.CurateStoreIncremental(ctx, cfg)
		} else {
			filesCreated, curateErr = pipeline.CurateStore(ctx, cfg)
		}

		result := curate.CurationResult{
			StoreName:    cfg.Name,
			FilesCreated: filesCreated,
		}
		if curateErr != nil {
			result.Error = curateErr.Error()
		}
		results = append(results, result)
		totalFiles += filesCreated

		// Auto-index curated files so they're immediately searchable
		// (FR-027, FR-028). Uses the same PersistBlocks/GenerateEmbeddings
		// pipeline as store_learning for consistency.
		if filesCreated > 0 {
			indexed := c.autoIndexKnowledgeStore(cfg)
			curateLogger.Info("auto-indexed curated files",
				"store", cfg.Name, "indexed", indexed)
		}
	}

	response := map[string]any{
		"status":  "complete",
		"results": results,
		"message": fmt.Sprintf("Curated %d knowledge files across %d store(s).", totalFiles, len(stores)),
	}

	res, err := jsonTextResult(response)
	return res, nil, err
}

// returnPromptMode returns extraction prompts for all stores when no
// synthesizer is available (MCP mode). The calling agent can use these
// prompts to perform synthesis externally.
func (c *Curate) returnPromptMode(ctx context.Context, pipeline *curate.Pipeline, stores []curate.StoreConfig) (*mcp.CallToolResult, any, error) {
	var storePrompts []map[string]any

	for _, cfg := range stores {
		// Load documents to build the prompt.
		pages, err := c.loadStoreDocCount(cfg)
		if err != nil {
			curateLogger.Warn("failed to count docs for store", "store", cfg.Name, "err", err)
			continue
		}

		if pages == 0 {
			continue
		}

		// Build a prompt by loading content and calling BuildExtractionPrompt.
		var documents []curate.DocumentContent
		for _, srcID := range cfg.Sources {
			srcPages, _ := c.store.ListPagesBySource(srcID)
			for _, p := range srcPages {
				blocks, _ := c.store.GetBlocksByPage(p.Name)
				var parts []string
				for _, b := range blocks {
					if b.Content != "" {
						parts = append(parts, b.Content)
					}
				}
				if len(parts) > 0 {
					documents = append(documents, curate.DocumentContent{
						SourceID: srcID,
						PageName: p.Name,
						Content:  joinStrings(parts, "\n"),
					})
				}
			}
		}

		prompt := pipeline.BuildExtractionPrompt(documents)
		storePrompts = append(storePrompts, map[string]any{
			"store_name":        cfg.Name,
			"docs_to_process":   len(documents),
			"extraction_prompt": prompt,
		})
	}

	response := map[string]any{
		"status":  "prompt_ready",
		"stores":  storePrompts,
		"message": fmt.Sprintf("Extraction prompts ready for %d store(s). Call with synthesized results to complete curation.", len(storePrompts)),
	}

	res, err := jsonTextResult(response)
	return res, nil, err
}

// loadStoreDocCount returns the total number of indexed documents across
// all sources for a store configuration.
func (c *Curate) loadStoreDocCount(cfg curate.StoreConfig) (int, error) {
	total := 0
	for _, srcID := range cfg.Sources {
		count, err := c.store.CountPagesBySource(srcID)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// autoIndexKnowledgeStore reads curated markdown files from a knowledge
// store directory and persists them into the SQLite store with the source
// ID "knowledge-{store-name}". This makes curated content immediately
// searchable via semantic_search and other MCP tools (FR-027, FR-028).
//
// Design decision: Follows the same PersistBlocks/GenerateEmbeddings
// pipeline as store_learning and reIngestLearnings for consistency.
// Pages are upserted — if a page already exists (from a previous curation),
// its blocks and embeddings are replaced.
func (c *Curate) autoIndexKnowledgeStore(cfg curate.StoreConfig) int {
	storePath := curate.ResolveStorePath(cfg, c.vaultPath)
	sourceID := knowledgeSourceID(cfg.Name)

	entries, err := os.ReadDir(storePath)
	if err != nil {
		curateLogger.Warn("failed to read knowledge store for auto-indexing",
			"store", cfg.Name, "path", storePath, "err", err)
		return 0
	}

	indexed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// Skip index and state files.
		if entry.Name() == "_index.md" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		filePath := filepath.Join(storePath, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			curateLogger.Warn("failed to read knowledge file for indexing",
				"path", filePath, "err", err)
			continue
		}

		// Compute content hash for deduplication.
		hash := sha256.Sum256(content)
		contentHash := fmt.Sprintf("%x", hash[:8])

		// Page name follows the pattern: knowledge/{store-name}/{filename-without-ext}
		baseName := strings.TrimSuffix(entry.Name(), ".md")
		pageName := fmt.Sprintf("knowledge/%s/%s", cfg.Name, baseName)

		// Check if page already exists and content hasn't changed.
		existing, err := c.store.GetPage(pageName)
		if err != nil {
			curateLogger.Warn("failed to check existing page",
				"page", pageName, "err", err)
			continue
		}

		if existing != nil {
			if existing.ContentHash == contentHash {
				// Content unchanged — skip re-indexing.
				continue
			}
			// Content changed — delete old blocks and re-index.
			if err := c.store.DeleteBlocksByPage(pageName); err != nil {
				curateLogger.Warn("failed to delete old blocks",
					"page", pageName, "err", err)
			}
			// Update existing page.
			existing.ContentHash = contentHash
			existing.Tier = "curated"
			existing.SourceID = sourceID
			if err := c.store.UpdatePage(existing); err != nil {
				curateLogger.Warn("failed to update page",
					"page", pageName, "err", err)
				continue
			}
		} else {
			// Insert new page.
			page := &store.Page{
				Name:         pageName,
				OriginalName: baseName,
				SourceID:     sourceID,
				SourceDocID:  entry.Name(),
				ContentHash:  contentHash,
				Tier:         "curated",
			}
			if err := c.store.InsertPage(page); err != nil {
				curateLogger.Warn("failed to insert knowledge page",
					"page", pageName, "err", err)
				continue
			}
		}

		// Parse the document into blocks.
		docID := fmt.Sprintf("%s-%s", sourceID, baseName)
		_, blocks := vault.ParseDocument(docID, string(content))

		// Persist blocks.
		if err := vault.PersistBlocks(c.store, pageName, blocks, sql.NullString{}, 0); err != nil {
			curateLogger.Warn("failed to persist knowledge blocks",
				"page", pageName, "err", err)
			continue
		}

		// Generate embeddings if available.
		if c.embedder != nil && c.embedder.Available() {
			vault.GenerateEmbeddings(c.store, c.embedder, pageName, blocks, nil)
		}

		indexed++
	}

	return indexed
}

// knowledgeSourceID returns the source ID for a knowledge store.
// Format: "knowledge-{store-name}" per FR-028.
func knowledgeSourceID(storeName string) string {
	return "knowledge-" + storeName
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}
