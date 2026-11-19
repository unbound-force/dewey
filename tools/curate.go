package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/curate"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/llm"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
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
	mu            *sync.Mutex
	store         *store.Store
	embedder      embed.Embedder
	synth         llm.Synthesizer
	vaultPath     string
	maxChunkChars int // max chars per embedding chunk; 0 uses embed.DefaultMaxChunkChars
}

// NewCurate creates a new Curate tool handler with the given dependencies.
// The store must be non-nil. The synth may be nil — the tool returns
// extraction prompts when no synthesizer is available (MCP mode).
// The mu parameter is the shared index mutex; when non-nil, curation
// acquires it to prevent concurrent indexing operations. The maxChunkChars
// parameter controls the maximum character length per embedding chunk;
// pass 0 to use embed.DefaultMaxChunkChars.
func NewCurate(s *store.Store, e embed.Embedder, synth llm.Synthesizer, vaultPath string, mu *sync.Mutex, maxChunkChars int) *Curate {
	if mu == nil {
		mu = &sync.Mutex{}
	}
	return &Curate{store: s, embedder: e, synth: synth, vaultPath: vaultPath, mu: mu, maxChunkChars: maxChunkChars}
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
		return errorResult(persistentStorageError("curate").Error()), nil, nil
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
		return c.returnPromptMode(ctx, pipeline, stores, synthesisUnavailableReason(c.synth))
	}

	// Run curation for each store.
	var results []curate.CurationResult
	totalFiles := 0
	failedStores := 0

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
			failedStores++
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

	status := "complete"
	response := map[string]any{
		"status":  status,
		"results": results,
		"message": fmt.Sprintf("Curated %d knowledge files across %d store(s).", totalFiles, len(stores)),
	}
	if failedStores > 0 {
		response["status"] = "partial"
		response["total_failed"] = failedStores
		response["message"] = fmt.Sprintf("Curated %d knowledge files across %d store(s); %d store(s) failed.", totalFiles, len(stores), failedStores)
	}

	res, err := jsonTextResult(response)
	if err != nil {
		return nil, nil, err
	}
	if failedStores > 0 {
		res.IsError = true
	}
	return res, nil, nil
}

// returnPromptMode returns extraction prompts for all stores when no
// synthesizer is available (MCP mode). The calling agent can use these
// prompts to perform synthesis externally.
func (c *Curate) returnPromptMode(ctx context.Context, pipeline *curate.Pipeline, stores []curate.StoreConfig, reason string) (*mcp.CallToolResult, any, error) {
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
		"status":             "prompt_ready",
		"stores":             storePrompts,
		"unavailable_reason": reason,
		"message": fmt.Sprintf(
			"Extraction prompts ready for %d store(s). Call with synthesized results to complete curation. Synthesis unavailable: %s.",
			len(storePrompts), reason,
		),
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

// IndexKnowledgeStore reads curated markdown files from a knowledge store
// directory and persists them into the SQLite store with the source ID
// "knowledge-{store-name}". This makes curated content immediately
// searchable via semantic_search and other MCP tools (FR-027, FR-028).
//
// It is the single implementation shared by the curate MCP tool
// (Curate.autoIndexKnowledgeStore) and background curation (main.go).
// Design decision: Follows the same PersistBlocks/GenerateEmbeddings
// pipeline as store_learning and reIngestLearnings for consistency.
// Pages are upserted — if a page already exists (from a previous curation),
// its blocks and embeddings are replaced. Trust tiers are derived from the
// file's frontmatter per the git-is-the-source-of-record change (IP-FR-001)
// and overwrite any stale cached tier — including a human-promoted validated
// row — so the DB never outlives the file rule (GTS-FR-005). The file's
// frontmatter is persisted as the page's properties so property-driven
// readers (semantic_search validity ranking and filtering) see it; rows
// written without properties are backfilled even when content is unchanged.
//
// After the scan, the function prunes this store's pages whose backing
// files no longer exist (CKS-FR-003, design D1: re-index first, prune
// second) by delegating to pruneOrphanedKnowledgePages with the base names
// collected during the scan. A store directory that does not exist counts
// as an empty file set, so every page of the store is an orphan; any other
// directory scan error fails closed and prunes nothing. The pruning path
// performs no embedding work (design D3).
//
// Returns the number of pages (re)indexed and the number of this store's
// pages deleted because their files no longer exist (successful deletions
// only); logging the pruned count is the callers' job (design D4).
func IndexKnowledgeStore(s *store.Store, embedder embed.Embedder, cfg curate.StoreConfig, vaultPath string, maxChunkChars int, logger *log.Logger) (indexed, pruned int) {
	storePath := curate.ResolveStorePath(cfg, vaultPath)
	sourceID := knowledgeSourceID(cfg.Name)

	// Trust-tier derivation needs the file's blob object IDs (IP-FR-001).
	// The knowledge store lives under the vault, so the vault's repository
	// is the trust root: resolve the default-branch ref and batch blob map
	// once per indexing pass (design D2 — one ls-tree scoped to the store
	// directory, not one per file). A store outside the vault, a non-git
	// vault, or an unresolvable ref leaves the resolver nil, which withholds
	// the validated grant (GTS-FR-004) and behaves exactly as before the
	// wiring.
	storeRel := ""
	if rel, err := filepath.Rel(vaultPath, storePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		storeRel = filepath.ToSlash(rel)
	}
	var tierBlobs vault.TierBlobResolver
	if storeRel != "" {
		tierBlobs = gitref.NewPageBlobResolver(context.Background(),
			gitref.Options{VaultDir: vaultPath}, []string{storeRel})
	}

	entries, err := os.ReadDir(storePath)
	if err != nil {
		logger.Warn("failed to read knowledge store for auto-indexing",
			"store", cfg.Name, "path", storePath, "err", err)
		if !os.IsNotExist(err) {
			// Any other scan error fails closed (CKS-FR-003): the file set
			// is unknown, so pruning is skipped for this store entirely.
			return 0, 0
		}
		// A store directory that does not exist is an empty file set
		// (CKS-FR-003): every page of this store's source is an orphan.
		return 0, pruneOrphanedKnowledgePages(s, cfg, nil, logger)
	}

	// Base names of the store files on disk, collected as the scan below
	// processes each entry under the same filters the scan applies (design
	// D2: the file set comes from the scan just performed, with no reads
	// beyond it). The pruning tail at the end of this function deletes
	// store pages whose base name is absent here.
	fileNames := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// Skip index and state files.
		if entry.Name() == "_index.md" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// The file exists and passed every scan filter, so its page is not
		// an orphan (CKS-FR-003). The base name is recorded from the
		// directory entry before the I/O and store operations below — a
		// read or store failure must never prune a page whose file is on
		// disk — and doubles as the page-name component below.
		baseName := strings.TrimSuffix(entry.Name(), ".md")
		fileNames[baseName] = struct{}{}

		filePath := filepath.Join(storePath, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("failed to read knowledge file for indexing",
				"path", filePath, "err", err)
			continue
		}

		// Compute content hash for deduplication.
		hash := sha256.Sum256(content)
		contentHash := fmt.Sprintf("%x", hash[:8])

		// Page name follows the pattern: knowledge/{store-name}/{filename-without-ext}
		pageName := fmt.Sprintf("knowledge/%s/%s", cfg.Name, baseName)

		// Parse the document once for both the tier derivation and the
		// block persistence (IP-FR-001). The docID seeds deterministic
		// block UUIDs.
		docID := fmt.Sprintf("%s-%s", sourceID, baseName)
		props, blocks := vault.ParseDocument(docID, string(content))

		// Derive the trust tier from frontmatter plus default-branch blob
		// equality per IP-FR-001: a canonical curated claim stays curated, a
		// validated claim without blob proof reads as draft (GTS-FR-002),
		// and an absent tier reads as draft (GTS-FR-001). The blob map key
		// is the file's vault-relative path (store directory + file name),
		// matching the ls-tree pathspec above.
		var fileSHA, refSHA string
		if tierBlobs != nil {
			fileSHA, refSHA = tierBlobs.PageBlobs(path.Join(storeRel, entry.Name()))
		}
		pageTier := vault.DeriveTierFromProps(props, fileSHA, refSHA)

		// Persist the frontmatter as the page's properties, the same JSON
		// encoding the vault and source indexers use, so property-driven
		// readers (semantic_search validity weighting and filtering,
		// property queries) see curated pages' frontmatter.
		propsJSON := ""
		if len(props) > 0 {
			if data, err := json.Marshal(props); err == nil {
				propsJSON = string(data)
			}
		}

		// Check if page already exists and content hasn't changed.
		existing, err := s.GetPage(pageName)
		if err != nil {
			logger.Warn("failed to check existing page",
				"page", pageName, "err", err)
			continue
		}

		if existing != nil {
			if existing.ContentHash == contentHash {
				// Content unchanged — skip re-indexing blocks, but still
				// recompute the cached tier so the DB never outlives the
				// file rule (GTS-FR-005), and backfill properties on rows
				// written before properties were persisted.
				if existing.Tier != pageTier || existing.Properties != propsJSON {
					existing.Tier = pageTier
					existing.Properties = propsJSON
					if err := s.UpdatePage(existing); err != nil {
						logger.Warn("failed to refresh knowledge page metadata",
							"page", pageName, "err", err)
					}
				}
				continue
			}
			// Content changed — delete old blocks and re-index.
			if err := s.DeleteBlocksByPage(pageName); err != nil {
				logger.Warn("failed to delete old blocks",
					"page", pageName, "err", err)
			}
			// Update existing page. The tier is recomputed from the file
			// (GTS-FR-005) and overwrites any stale cached value.
			existing.ContentHash = contentHash
			existing.Properties = propsJSON
			existing.Tier = pageTier
			existing.SourceID = sourceID
			if err := s.UpdatePage(existing); err != nil {
				logger.Warn("failed to update page",
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
				Properties:   propsJSON,
				ContentHash:  contentHash,
				Tier:         pageTier,
			}
			if err := s.InsertPage(page); err != nil {
				logger.Warn("failed to insert knowledge page",
					"page", pageName, "err", err)
				continue
			}
		}

		// Persist blocks.
		if err := vault.PersistBlocks(s, pageName, blocks, sql.NullString{}, 0); err != nil {
			logger.Warn("failed to persist knowledge blocks",
				"page", pageName, "err", err)
			continue
		}

		// Generate embeddings if available.
		if embedder != nil && embedder.Available() {
			maxChars := maxChunkChars
			if maxChars <= 0 {
				maxChars = embed.DefaultMaxChunkChars
			}
			vault.GenerateEmbeddings(s, embedder, pageName, blocks, nil, maxChars)
		}

		indexed++
	}

	// Pruning tail (CKS-FR-003, design D1): after the scan, delete this
	// store's pages whose files no longer exist. Pruning runs last so it
	// can never remove a page the loop above just created or refreshed
	// from a file still on disk, and the pruning path itself performs no
	// embedding work (design D3).
	pruned = pruneOrphanedKnowledgePages(s, cfg, fileNames, logger)

	return indexed, pruned
}

// pruneOrphanedKnowledgePages deletes a knowledge store's pages whose backing
// files no longer exist (CKS-FR-003). The page set is every page listed for
// the store's "knowledge-{store}" source; the file set is fileNames, the base
// names the IndexKnowledgeStore scan just collected from the store directory
// under the same filters the scan applies (design D2). A page is an orphan
// when its name minus the "knowledge/{store}/" prefix is absent from the file
// set; a nil map is an empty file set (the store directory does not exist),
// making every page of the store an orphan.
//
// Pruning fails closed: when the page listing fails, nothing is deleted and
// every page of the store stays in place, so a transient fault can never
// mass-delete pages. Each deletion goes through DeletePage, which cascades to
// the page's blocks and embeddings through the schema's foreign keys — no
// embedding work happens here. A per-page delete failure warns and continues
// (the failed page is retried on the next indexing pass), and the returned
// count includes successful deletions only; logging the count is the callers'
// job (design D4).
func pruneOrphanedKnowledgePages(s *store.Store, cfg curate.StoreConfig, fileNames map[string]struct{}, logger *log.Logger) int {
	pages, err := s.ListPagesBySource(knowledgeSourceID(cfg.Name))
	if err != nil {
		// Fail closed (CKS-FR-003): without the listing the orphan set is
		// unknown, so prune nothing rather than risk deleting live pages.
		logger.Warn("knowledge page pruning skipped; failed to list knowledge store pages",
			"store", cfg.Name, "err", err)
		return 0
	}

	prefix := fmt.Sprintf("knowledge/%s/", cfg.Name)
	pruned := 0
	for _, page := range pages {
		if _, ok := fileNames[strings.TrimPrefix(page.Name, prefix)]; ok {
			continue
		}
		if err := s.DeletePage(page.Name); err != nil {
			// The failed page stays and is retried on the next indexing
			// pass; only successful deletions are counted (CKS-FR-003).
			logger.Warn("failed to prune knowledge page", "page", page.Name, "err", err)
			continue
		}
		pruned++
	}
	return pruned
}

// autoIndexKnowledgeStore indexes a knowledge store's curated files into the
// persistent store, delegating to the shared IndexKnowledgeStore
// implementation that background curation (main.go) also uses. The pruned
// count is logged here rather than returned — pruning reports only when it
// deleted something (CKS-FR-003, design D4) — so the indexed-count contract
// this method's callers already log stays unchanged.
func (c *Curate) autoIndexKnowledgeStore(cfg curate.StoreConfig) int {
	indexed, pruned := IndexKnowledgeStore(c.store, c.embedder, cfg, c.vaultPath, c.maxChunkChars, curateLogger)
	if pruned > 0 {
		curateLogger.Info("pruned knowledge store pages (files no longer exist)",
			"store", cfg.Name, "count", pruned)
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
