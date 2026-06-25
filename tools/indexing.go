package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
)

// deweyWorkspaceDir is the workspace directory name relative to the vault root.
// Duplicated from main.go because tools/ cannot import package main.
const deweyWorkspaceDir = ".uf/dewey"

// protectedSourceIDs lists source IDs that must NOT be deleted during reindex.
// "disk-local" is the local vault content loaded at serve startup.
// "learning" is agent learnings stored via the store_learning MCP tool.
// Deleting these would destroy user content that cannot be re-fetched from
// external sources (per FR-009, R5).
var protectedSourceIDs = map[string]bool{
	"disk-local": true,
	"learning":   true,
}

// indexingLogger is the package-level structured logger for indexing tool operations.
var indexingLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/tools/indexing",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Indexing provides MCP tools for triggering source indexing while
// the server is running. It shares a mutex between Index and Reindex
// to ensure mutual exclusion — only one indexing operation can run at
// a time (per FR-005).
//
// Design decision: The store, embedder, and vaultPath are injected as
// dependencies (Dependency Inversion Principle) following the Learning
// struct pattern. This enables testing with in-memory stores and nil
// embedders.
type Indexing struct {
	mu            *sync.Mutex
	store         *store.Store
	embedder      embed.Embedder
	vaultPath     string
	maxChunkChars int // max chars per embedding chunk; 0 uses embed.DefaultMaxChunkChars
}

// NewIndexing creates a new Indexing tool handler with the given store,
// embedder, vault path, optional shared mutex, and max chunk chars for
// embedding generation. The embedder may be nil — indexing proceeds
// without embedding generation when unavailable (graceful degradation).
// The store must be non-nil for the tools to function; a clear error is
// returned at call time if it is nil. The vaultPath is the vault root
// directory (not the .uf/dewey/ workspace).
//
// The mu parameter enables shared mutual exclusion with background startup
// indexing (per D1, spec 012). When mu is non-nil, it replaces the internal
// mutex. When mu is nil, an internal mutex is created (backward compatible).
//
// The maxChunkChars parameter controls the maximum character length per
// embedding chunk. Pass 0 to use embed.DefaultMaxChunkChars.
func NewIndexing(s *store.Store, e embed.Embedder, vaultPath string, mu *sync.Mutex, maxChunkChars int) *Indexing {
	if mu == nil {
		mu = &sync.Mutex{}
	}
	return &Indexing{store: s, embedder: e, vaultPath: vaultPath, mu: mu, maxChunkChars: maxChunkChars}
}

// indexSummary is the structured JSON response returned by both Index
// and Reindex handlers. It provides agents with actionable information
// about the indexing operation's outcome.
type indexSummary struct {
	Status              string          `json:"status"`
	SourcesProcessed    int             `json:"sources_processed"`
	PagesIndexed        int             `json:"pages_indexed"`
	EmbeddingsGenerated int             `json:"embeddings_generated"`
	EmbeddingsSkipped   int             `json:"embeddings_skipped"`
	PagesDeleted        int64           `json:"pages_deleted,omitempty"`
	ElapsedMs           int64           `json:"elapsed_ms"`
	Sources             []sourceSummary `json:"sources"`
}

// sourceSummary reports per-source results within an indexing operation.
type sourceSummary struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Documents int    `json:"documents"`
	Error     string `json:"error,omitempty"`
}

// Index handles the index MCP tool. Fetches and indexes configured sources
// while the server is running. Supports optional source_id filtering to
// re-index only a specific source (per FR-002).
//
// Returns a structured JSON summary with sources processed, pages indexed,
// embeddings generated, and elapsed time (per FR-004). Returns an MCP error
// result (not a Go error) if the store is unavailable or another indexing
// operation is already in progress.
func (ix *Indexing) Index(ctx context.Context, req *mcp.CallToolRequest, input types.IndexInput) (*mcp.CallToolResult, any, error) {
	if ix.store == nil {
		return errorResult("index requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	// Non-blocking mutual exclusion: if another Index or Reindex is running,
	// return immediately with an error rather than queuing (per FR-005).
	if !ix.mu.TryLock() {
		return errorResult("indexing operation already in progress"), nil, nil
	}
	defer ix.mu.Unlock()

	start := time.Now()
	indexingLogger.Info("index starting", "source_id", input.SourceID)

	// Load sources configuration from the workspace directory.
	deweyDir := filepath.Join(ix.vaultPath, deweyWorkspaceDir)
	sourcesPath := filepath.Join(deweyDir, "sources.yaml")
	configs, err := source.LoadSourcesConfig(sourcesPath)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to load sources config: %v", err)), nil, nil
	}
	if len(configs) == 0 {
		return errorResult("no sources configured. Add sources via 'dewey source add'."), nil, nil
	}

	// Build last-fetched times from the store for refresh interval checks.
	lastFetchedTimes := make(map[string]time.Time)
	storedSources, _ := ix.store.ListSources()
	for _, src := range storedSources {
		if src.LastFetchedAt > 0 {
			lastFetchedTimes[src.ID] = time.UnixMilli(src.LastFetchedAt)
		}
	}

	// Create source manager and fetch documents.
	cacheDir := filepath.Join(deweyDir, "cache")
	mgr := source.NewManager(configs, ix.vaultPath, cacheDir)

	// When a specific source_id is requested, pass it as the filter.
	// Force=true to bypass refresh intervals for explicit index requests.
	result, allDocs := mgr.FetchAll(input.SourceID, true, lastFetchedTimes)

	// Index fetched documents through the shared pipeline.
	indexResult, indexErr := vault.IndexDocuments(ix.store, allDocs, configs, ix.embedder, ix.maxChunkChars)
	if indexErr != nil {
		indexingLogger.Warn("index had errors", "err", indexErr)
	}

	// Report source errors to the store.
	ix.reportSourceErrors(result)

	// Build per-source summaries for the response.
	var sources []sourceSummary
	for _, s := range result.Summaries {
		sources = append(sources, sourceSummary{
			ID:        s.SourceID,
			Type:      s.SourceType,
			Documents: s.Documents,
			Error:     s.Error,
		})
	}

	elapsed := time.Since(start)
	indexingLogger.Info("index complete",
		"sources", len(result.Summaries),
		"pages", indexResult.TotalIndexed,
		"embeddings", indexResult.TotalEmbeddings,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	summary := indexSummary{
		Status:              "completed",
		SourcesProcessed:    len(result.Summaries),
		PagesIndexed:        indexResult.TotalIndexed,
		EmbeddingsGenerated: indexResult.TotalEmbeddings,
		EmbeddingsSkipped:   indexResult.TotalIndexed - indexResult.TotalEmbeddings,
		ElapsedMs:           elapsed.Milliseconds(),
		Sources:             sources,
	}

	res, err := jsonTextResult(summary)
	return res, nil, err
}

// Reindex handles the reindex MCP tool. Deletes all external source content
// from the store and rebuilds the index from scratch. Protected sources
// (disk-local, learning) are preserved (per FR-009, R5).
//
// Unlike the CLI `dewey reindex` which deletes graph.db entirely, this
// tool uses per-source DeletePagesBySource() because the server is actively
// using the database. This is a safer, more targeted approach.
//
// Returns a structured JSON summary including delete counts and new index
// counts. Returns an MCP error result if the store is unavailable or
// another indexing operation is already in progress.
func (ix *Indexing) Reindex(ctx context.Context, req *mcp.CallToolRequest, input types.ReindexInput) (*mcp.CallToolResult, any, error) {
	if ix.store == nil {
		return errorResult("reindex requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	// Non-blocking mutual exclusion: shared mutex with Index (per FR-005).
	if !ix.mu.TryLock() {
		return errorResult("indexing operation already in progress"), nil, nil
	}
	defer ix.mu.Unlock()

	start := time.Now()
	indexingLogger.Info("reindex starting")

	// Load sources configuration.
	deweyDir := filepath.Join(ix.vaultPath, deweyWorkspaceDir)
	sourcesPath := filepath.Join(deweyDir, "sources.yaml")
	configs, err := source.LoadSourcesConfig(sourcesPath)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to load sources config: %v", err)), nil, nil
	}

	// Delete pages for each non-protected source. Protected sources
	// (disk-local, learning) are skipped to preserve user content that
	// cannot be re-fetched from external sources (per FR-009, R5).
	var totalDeleted int64
	storedSources, _ := ix.store.ListSources()
	for _, src := range storedSources {
		if protectedSourceIDs[src.ID] {
			indexingLogger.Info("preserving protected source", "source", src.ID)
			continue
		}
		deleted, err := ix.store.DeletePagesBySource(src.ID)
		if err != nil {
			indexingLogger.Warn("failed to delete pages for source",
				"source", src.ID, "err", err)
			continue
		}
		if deleted > 0 {
			indexingLogger.Info("deleted source pages",
				"source", src.ID, "pages", deleted)
		}
		totalDeleted += deleted
	}

	// Also delete pages for sources in config that might not have a source
	// record yet but have pages in the store.
	for _, cfg := range configs {
		if protectedSourceIDs[cfg.ID] {
			continue
		}
		// Check if we already deleted this source above.
		alreadyHandled := false
		for _, src := range storedSources {
			if src.ID == cfg.ID {
				alreadyHandled = true
				break
			}
		}
		if alreadyHandled {
			continue
		}
		deleted, err := ix.store.DeletePagesBySource(cfg.ID)
		if err != nil {
			indexingLogger.Warn("failed to delete pages for configured source",
				"source", cfg.ID, "err", err)
			continue
		}
		totalDeleted += deleted
	}

	// Re-index all sources with force=true (ignore refresh intervals).
	cacheDir := filepath.Join(deweyDir, "cache")
	mgr := source.NewManager(configs, ix.vaultPath, cacheDir)
	lastFetchedTimes := make(map[string]time.Time) // empty — force fetch all
	result, allDocs := mgr.FetchAll("", true, lastFetchedTimes)

	// Index fetched documents.
	indexResult, indexErr := vault.IndexDocuments(ix.store, allDocs, configs, ix.embedder, ix.maxChunkChars)
	if indexErr != nil {
		indexingLogger.Warn("reindex had errors", "err", indexErr)
	}

	// Report source errors.
	ix.reportSourceErrors(result)

	// Build per-source summaries.
	var sources []sourceSummary
	for _, s := range result.Summaries {
		sources = append(sources, sourceSummary{
			ID:        s.SourceID,
			Type:      s.SourceType,
			Documents: s.Documents,
			Error:     s.Error,
		})
	}

	elapsed := time.Since(start)
	indexingLogger.Info("reindex complete",
		"deleted", totalDeleted,
		"sources", len(result.Summaries),
		"pages", indexResult.TotalIndexed,
		"embeddings", indexResult.TotalEmbeddings,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	summary := indexSummary{
		Status:              "completed",
		SourcesProcessed:    len(result.Summaries),
		PagesIndexed:        indexResult.TotalIndexed,
		EmbeddingsGenerated: indexResult.TotalEmbeddings,
		EmbeddingsSkipped:   indexResult.TotalIndexed - indexResult.TotalEmbeddings,
		PagesDeleted:        totalDeleted,
		ElapsedMs:           elapsed.Milliseconds(),
		Sources:             sources,
	}

	res, err := jsonTextResult(summary)
	return res, nil, err
}

// reportSourceErrors updates source status for any sources that failed
// during the fetch phase. Mirrors cli.go's reportSourceErrors().
func (ix *Indexing) reportSourceErrors(result *source.FetchResult) {
	for _, summary := range result.Summaries {
		if summary.Error != "" {
			existingSrc, _ := ix.store.GetSource(summary.SourceID)
			if existingSrc != nil {
				if err := ix.store.UpdateSourceStatus(summary.SourceID, "error", summary.Error); err != nil {
					indexingLogger.Warn("failed to update source error status",
						"source", summary.SourceID, "err", err)
				}
			}
		}
	}
}
