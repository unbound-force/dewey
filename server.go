package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/tools"
	"github.com/unbound-force/dewey/vault"
)

// serverConfig holds optional dependencies for the MCP server.
type serverConfig struct {
	embedder   embed.Embedder
	store      *store.Store
	vaultPath  string
	indexReady *atomic.Bool
	indexMutex *sync.Mutex
}

// serverOption configures the MCP server.
type serverOption func(*serverConfig)

// WithEmbedder sets the embedding provider for semantic search tools.
func WithEmbedder(e embed.Embedder) serverOption {
	return func(c *serverConfig) { c.embedder = e }
}

// WithPersistentStore sets the SQLite store for semantic search tools.
func WithPersistentStore(s *store.Store) serverOption {
	return func(c *serverConfig) { c.store = s }
}

// WithVaultPath sets the vault root path for tools that need to locate
// configuration files (e.g., sources.yaml for indexing tools).
func WithVaultPath(p string) serverOption {
	return func(c *serverConfig) { c.vaultPath = p }
}

// WithIndexReady sets the atomic flag that tracks background indexing status.
// When non-nil, the health tool reports whether indexing is still in progress.
func WithIndexReady(ready *atomic.Bool) serverOption {
	return func(c *serverConfig) { c.indexReady = ready }
}

// WithIndexMutex sets the shared mutex for indexing mutual exclusion.
// This mutex is shared between the background startup indexing goroutine
// and the index/reindex MCP tools to prevent concurrent indexing operations.
func WithIndexMutex(mu *sync.Mutex) serverOption {
	return func(c *serverConfig) { c.indexMutex = mu }
}

// newServer creates and configures the MCP server with all tools registered.
// If readOnly is true, write tools are not registered.
// Tools requiring DataScript are only registered if the backend supports it.
// The embedder and persistent store are optional — semantic search tools
// are always registered but return clear error messages when unavailable.
// Returns the server and the total number of registered tools.
func newServer(b backend.Backend, readOnly bool, opts ...serverOption) (*mcp.Server, int) {
	var cfg serverConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "dewey",
			Version: version,
		},
		nil,
	)

	_, hasDataScript := b.(backend.HasDataScript)

	nav := tools.NewNavigate(b)
	search := tools.NewSearch(b)
	analyze := tools.NewAnalyze(b)
	journal := tools.NewJournal(b)

	var toolCount int
	toolCount += registerNavigateTools(srv, nav, hasDataScript)
	toolCount += registerSearchTools(srv, search, hasDataScript)
	toolCount += registerAnalyzeTools(srv, analyze)

	if !readOnly {
		write := tools.NewWrite(b)
		decision := tools.NewDecision(b)
		toolCount += registerWriteTools(srv, write)
		toolCount += registerDecisionTools(srv, decision)
	}

	toolCount += registerJournalTools(srv, journal)

	if hasDataScript {
		flashcard := tools.NewFlashcard(b)
		toolCount += registerFlashcardTools(srv, flashcard, readOnly)

		whiteboard := tools.NewWhiteboard(b)
		toolCount += registerWhiteboardTools(srv, whiteboard)
	}

	semantic := tools.NewSemantic(cfg.embedder, cfg.store)
	toolCount += registerSemanticTools(srv, semantic)

	if !readOnly {
		learning := tools.NewLearning(cfg.embedder, cfg.store)
		toolCount += registerLearningTools(srv, learning)

		indexing := tools.NewIndexing(cfg.store, cfg.embedder, cfg.vaultPath, cfg.indexMutex)
		toolCount += registerIndexingTools(srv, indexing)
	}

	toolCount += registerHealthTool(srv, b, readOnly, &cfg)

	// Vault management tools (Obsidian-specific).
	if vaultClient, ok := b.(*vault.Client); ok && !readOnly {
		srv.AddTool(&mcp.Tool{
			Name:        "reload",
			Description: "Force a full vault re-index. Clears all cached pages and blocks, then re-reads all .md files. Use when external changes need to be refreshed.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`),
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := vaultClient.Reload(); err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Reload failed: %v", err)}},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "Vault reloaded successfully"}},
			}, nil
		})
		toolCount++
	}

	return srv, toolCount
}

// registerNavigateTools registers page, block, link, and traversal tools.
// Returns the number of tools registered.
func registerNavigateTools(srv *mcp.Server, nav *tools.Navigate, hasDataScript bool) int {
	count := 5

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_page",
		Description: "Get a Logseq page with its full recursive block tree, properties, tags, and parsed links. Every block includes extracted [[links]], ((references)), #tags, and key:: value properties. Use maxBlocks to limit output size for large pages.",
	}, nav.GetPage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_block",
		Description: "Get a specific block by UUID with its ancestor chain (path to root page), children, and optionally siblings. Provides full context for where a block sits in the knowledge hierarchy.",
	}, nav.GetBlock)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_pages",
		Description: "List pages with filtering by namespace, property, or tag. Returns page summaries with block count and link count. Sort by name, modified, or created.",
	}, nav.ListPages)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_links",
		Description: "Get forward links (pages this page links to) and backlinks (pages that link to this page) for any page. Each link includes the specific block containing it.",
	}, nav.GetLinks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "traverse",
		Description: "Find paths between two pages through the link graph using BFS. Discovers how concepts are connected through intermediate pages. Returns all paths up to max_hops length.",
	}, nav.Traverse)

	if hasDataScript {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "get_references",
			Description: "Get all blocks that reference a specific block via ((uuid)) block references. Returns the referencing blocks with their page context.",
		}, nav.GetReferences)
		count++
	}

	return count
}

// registerSearchTools registers full-text search, property query, tag, and DataScript tools.
// Returns the number of tools registered.
func registerSearchTools(srv *mcp.Server, search *tools.Search, hasDataScript bool) int {
	count := 3

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search",
		Description: "Full-text search across all blocks in the knowledge graph. Returns matching blocks with surrounding context (parent chain and sibling blocks) so you understand where each match sits.",
	}, search.Search)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_properties",
		Description: "Find blocks and pages by property values. Search for all content with a specific property key, or filter by property value with operators (eq, contains, gt, lt).",
	}, search.QueryProperties)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "find_by_tag",
		Description: "Find all blocks and pages with a specific tag, including child tags in the tag hierarchy. Returns content grouped by page.",
	}, search.FindByTag)

	if hasDataScript {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "query_datalog",
			Description: "Execute raw DataScript/Datalog queries against the Logseq database. This is the most powerful query mechanism — can find anything. Example: [:find (pull ?b [*]) :where [?b :block/marker \"TODO\"]] finds all TODO blocks.",
		}, search.QueryDatalog)
		count++
	}

	return count
}

// registerAnalyzeTools registers graph overview, connections, gaps, orphans, and clusters tools.
// Returns the number of tools registered.
func registerAnalyzeTools(srv *mcp.Server, analyze *tools.Analyze) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "graph_overview",
		Description: "Get a high-level overview of the entire knowledge graph: total pages, blocks, links, most connected pages, orphan count, namespace breakdown. Builds an in-memory graph for analysis.",
	}, analyze.GraphOverview)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "find_connections",
		Description: "Discover how two pages are connected through the link graph. Returns whether they're directly linked, shortest paths between them, and shared connections (pages both link to or are linked from).",
	}, analyze.FindConnections)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "knowledge_gaps",
		Description: "Find sparse areas in the knowledge graph: orphan pages (no links in or out), dead-end pages (linked to but link nowhere), and weakly-linked pages. Helps identify where to add connections.",
	}, analyze.KnowledgeGaps)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_orphans",
		Description: "List orphan pages (no incoming or outgoing links). Returns page names with block counts and property status. Use for graph hygiene — find disconnected pages that need linking or cleanup.",
	}, analyze.ListOrphans)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "topic_clusters",
		Description: "Discover topic clusters by finding connected components in the knowledge graph. Returns groups of densely connected pages with their hub (most connected page in each cluster).",
	}, analyze.TopicClusters)

	return 5
}

// registerWriteTools registers page and block write/mutation tools.
// Returns the number of tools registered.
func registerWriteTools(srv *mcp.Server, write *tools.Write) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_page",
		Description: "Create a new Logseq page with optional properties and initial blocks. Use properties for metadata like type::, status::, etc.",
	}, write.CreatePage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "append_blocks",
		Description: "Append plain-text blocks to an existing page. Accepts an array of strings (same format as create_page blocks). Simpler than upsert_blocks when you just need to add content without nesting or properties.",
	}, write.AppendBlocks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_block",
		Description: "Update an existing block's content by UUID. Replaces the block's entire content with the new value. Use get_page or get_block first to find the UUID.",
	}, write.UpdateBlock)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_block",
		Description: "Delete a block by UUID. Removes the block and all its children from the graph. This is irreversible.",
	}, write.DeleteBlock)

	// upsert_blocks uses raw handler because BlockInput has recursive Children field
	// which the schema generator can't handle.
	srv.AddTool(&mcp.Tool{
		Name:        "upsert_blocks",
		Description: "Batch create blocks on a page. Supports nested children for building block hierarchies. Append or prepend to existing content.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"page":{"type":"string","description":"Page name to add blocks to"},"blocks":{"type":"array","description":"Blocks to create. Each block has content (string), optional properties (object of strings), and optional children (array of blocks).","items":{"type":"object"}},"position":{"type":"string","description":"Where to add: append or prepend. Default: append"}},"required":["page","blocks"],"additionalProperties":false}`),
	}, write.UpsertBlocksRaw)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "move_block",
		Description: "Move a block to a new location — before, after, or as a child of another block.",
	}, write.MoveBlock)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_page",
		Description: "Delete a page entirely from the graph. Removes the page and all its blocks. This is irreversible.",
	}, write.DeletePage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "rename_page",
		Description: "Rename a page and update all [[links]] across the graph that reference the old name. Preserves content and connections.",
	}, write.RenamePage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "bulk_update_properties",
		Description: "Set a property (key:: value) on multiple pages at once. Useful for backfilling metadata like type::, status::, etc. across many pages in one call.",
	}, write.BulkUpdateProperties)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "link_pages",
		Description: "Create a bidirectional connection between two pages by adding a link block to each. Optionally include context describing the relationship.",
	}, write.LinkPages)

	return 10
}

// registerDecisionTools registers decision tracking and analysis health tools.
// Returns the number of tools registered.
func registerDecisionTools(srv *mcp.Server, decision *tools.Decision) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "decision_check",
		Description: "Surface all tracked decisions in the knowledge graph. Returns open, overdue, and optionally resolved decisions with deadline status. Use at session start to check what needs attention.",
	}, decision.DecisionCheck)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "decision_create",
		Description: "Create a new DECIDE block on a page with #decision tag and deadline. Decisions live on the page where context is richest, not on a central backlog.",
	}, decision.DecisionCreate)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "decision_resolve",
		Description: "Mark a decision as DONE with today's date and an optional outcome. Changes the DECIDE marker to DONE and adds resolved:: property.",
	}, decision.DecisionResolve)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "decision_defer",
		Description: "Push a decision's deadline to a new date with a reason. Tracks deferral count and warns after 3+ deferrals.",
	}, decision.DecisionDefer)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "analysis_health",
		Description: "Audit analysis, strategy, and assessment pages for graph connectivity. A page is healthy if it has 3+ outgoing links or contains a decision. Finds isolated analyses that don't connect back to the knowledge graph.",
	}, decision.AnalysisHealth)

	return 5
}

// registerJournalTools registers journal range and search tools.
// Returns the number of tools registered.
func registerJournalTools(srv *mcp.Server, journal *tools.Journal) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "journal_range",
		Description: "Get journal entries across a date range. Returns journal pages with their full block trees. Dates in YYYY-MM-DD format.",
	}, journal.JournalRange)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "journal_search",
		Description: "Search within journal entries specifically. Optionally filter by date range. Returns matching blocks with their journal date context.",
	}, journal.JournalSearch)

	return 2
}

// registerFlashcardTools registers flashcard overview, due, and create tools.
// Returns the number of tools registered.
func registerFlashcardTools(srv *mcp.Server, flashcard *tools.Flashcard, readOnly bool) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "flashcard_overview",
		Description: "Get SRS (spaced repetition) statistics: total cards, cards due for review, new vs reviewed cards, average repetitions. Gives a snapshot of the flashcard collection health.",
	}, flashcard.FlashcardOverview)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "flashcard_due",
		Description: "Get flashcards currently due for review. Returns card content, page, and SRS properties (ease factor, interval, repeats). Prioritizes new cards and overdue reviews.",
	}, flashcard.FlashcardDue)

	count := 2
	if !readOnly {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "flashcard_create",
			Description: "Create a new flashcard on a page. Adds a block with #card tag (front/question) and a child block (back/answer). The card will appear in Logseq's flashcard review system.",
		}, flashcard.FlashcardCreate)
		count++
	}

	return count
}

// registerWhiteboardTools registers whiteboard list and detail tools.
// Returns the number of tools registered.
func registerWhiteboardTools(srv *mcp.Server, whiteboard *tools.Whiteboard) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_whiteboards",
		Description: "List all Logseq whiteboards in the graph. Whiteboards are infinite canvas spaces where concepts are visually arranged and connected.",
	}, whiteboard.ListWhiteboards)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_whiteboard",
		Description: "Get a whiteboard's content including embedded pages, block references, visual connections between elements, and any text content. Reveals how concepts are spatially organized.",
	}, whiteboard.GetWhiteboard)

	return 2
}

// registerSemanticTools registers vector-based semantic search tools.
// Returns the number of tools registered.
func registerSemanticTools(srv *mcp.Server, semantic *tools.Semantic) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "semantic_search",
		Description: "Find documents semantically similar to a natural language query, ranked by cosine similarity. Returns results with provenance metadata.",
	}, semantic.SemanticSearch)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "similar",
		Description: "Given a document (by page name or block UUID), find the most similar documents in the index.",
	}, semantic.Similar)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "semantic_search_filtered",
		Description: "Semantic search constrained by metadata filters (source type, repository, properties).",
	}, semantic.SemanticSearchFiltered)

	return 3
}

// registerLearningTools registers the store_learning MCP tool for persisting
// agent learnings into the knowledge graph with optional embeddings.
// Returns the number of tools registered.
func registerLearningTools(srv *mcp.Server, learning *tools.Learning) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "store_learning",
		Description: "Store a learning (insight, pattern, gotcha) with optional tags. The learning is persisted with embeddings and immediately searchable via semantic_search. Use to build semantic memory across sessions.",
	}, learning.StoreLearning)

	return 1
}

// registerIndexingTools registers the index and reindex MCP tools for
// triggering source re-indexing while the server is running. These are
// write operations — excluded in read-only mode (per plan D4, D8).
// Returns the number of tools registered.
func registerIndexingTools(srv *mcp.Server, indexing *tools.Indexing) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "index",
		Description: "Fetch and index configured content sources. Updates the knowledge graph with new and changed content from disk, GitHub, web, and code sources. Optionally filter to a single source by ID.",
	}, indexing.Index)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "reindex",
		Description: "Delete and rebuild all external source content in the index. Preserves local vault content and stored learnings. Use when the index is stale or corrupted.",
	}, indexing.Reindex)

	return 2
}

// registerHealthTool registers the health check tool that reports server status,
// embedding coverage, and source information.
// Returns the number of tools registered.
func registerHealthTool(srv *mcp.Server, b backend.Backend, readOnly bool, cfg *serverConfig) int {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "health",
		Description: "Check server status: version, backend type, read-only mode, page count. Use to verify the server is alive and see its configuration.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		backendType := "obsidian"
		if _, ok := b.(backend.HasDataScript); ok {
			backendType = "logseq"
		}

		pages, _ := b.GetAllPages(ctx)
		pingErr := b.Ping(ctx)

		status := "ok"
		if pingErr != nil {
			status = fmt.Sprintf("error: %v", pingErr)
		}

		result := map[string]any{
			"status":    status,
			"version":   version,
			"backend":   backendType,
			"readOnly":  readOnly,
			"pageCount": len(pages),
		}

		// Add Dewey-specific fields per contracts/mcp-tools.md.
		deweyInfo := map[string]any{}
		if cfg.store != nil {
			deweyInfo["persistent"] = true

			embeddingCount, _ := cfg.store.CountEmbeddings()
			blockCount, _ := cfg.store.CountBlocks()
			deweyInfo["embeddingCount"] = embeddingCount

			var coverage float64
			if blockCount > 0 {
				coverage = float64(embeddingCount) / float64(blockCount)
			}
			deweyInfo["embeddingCoverage"] = coverage

			// Include per-source status in health response.
			storedSources, _ := cfg.store.ListSources()
			var sourcesInfo []map[string]any
			for _, src := range storedSources {
				srcInfo := map[string]any{
					"id":     src.ID,
					"type":   src.Type,
					"status": src.Status,
				}
				pc, _ := cfg.store.CountPagesBySource(src.ID)
				srcInfo["pageCount"] = pc
				if src.LastFetchedAt > 0 {
					srcInfo["lastFetched"] = fmt.Sprintf("%d", src.LastFetchedAt)
				}
				sourcesInfo = append(sourcesInfo, srcInfo)
			}
			deweyInfo["sources"] = sourcesInfo
		} else {
			deweyInfo["persistent"] = false
			deweyInfo["embeddingCount"] = 0
			deweyInfo["embeddingCoverage"] = 0.0
			deweyInfo["sources"] = []map[string]any{}
		}

		if cfg.embedder != nil {
			deweyInfo["embeddingModel"] = cfg.embedder.ModelID()
			deweyInfo["embeddingAvailable"] = cfg.embedder.Available()
		} else {
			deweyInfo["embeddingModel"] = ""
			deweyInfo["embeddingAvailable"] = false
		}

		// Report background indexing status when the flag is available (FR-007, D2).
		if cfg.indexReady != nil {
			deweyInfo["indexing"] = !cfg.indexReady.Load()
		}

		result["dewey"] = deweyInfo

		data, _ := json.MarshalIndent(result, "", "  ")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	return 1
}
