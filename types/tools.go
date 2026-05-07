package types

// --- Navigate tool inputs ---

type GetPageInput struct {
	Name      string `json:"name" jsonschema:"Page name to retrieve"`
	Depth     int    `json:"depth,omitempty" jsonschema:"Max block tree depth (-1 for unlimited). Default: -1"`
	MaxBlocks int    `json:"maxBlocks,omitempty" jsonschema:"Max total blocks to return. Truncates with a flag when exceeded. Default: unlimited"`
	Compact   bool   `json:"compact,omitempty" jsonschema:"Return blocks as plain strings instead of enriched objects. Saves ~60%% tokens. Default: false"`
}

type GetBlockInput struct {
	UUID             string `json:"uuid" jsonschema:"Block UUID to retrieve"`
	IncludeAncestors bool   `json:"includeAncestors,omitempty" jsonschema:"Include ancestor chain from root. Default: true"`
	IncludeSiblings  bool   `json:"includeSiblings,omitempty" jsonschema:"Include sibling blocks. Default: false"`
}

type ListPagesInput struct {
	Namespace   string `json:"namespace,omitempty" jsonschema:"Filter by namespace prefix (e.g. projects)"`
	HasProperty string `json:"hasProperty,omitempty" jsonschema:"Filter to pages with this property key"`
	HasTag      string `json:"hasTag,omitempty" jsonschema:"Filter to pages with this tag"`
	SortBy      string `json:"sortBy,omitempty" jsonschema:"Sort by: name or modified or created. Default: name"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Max results. Default: 50"`
}

type GetLinksInput struct {
	Name      string `json:"name" jsonschema:"Page name to get links for"`
	Direction string `json:"direction,omitempty" jsonschema:"Link direction: forward or backward or both. Default: both"`
}

type GetReferencesInput struct {
	UUID string `json:"uuid" jsonschema:"Block UUID to find references for"`
}

type TraverseInput struct {
	From    string `json:"from" jsonschema:"Starting page name"`
	To      string `json:"to" jsonschema:"Target page name"`
	MaxHops int    `json:"maxHops,omitempty" jsonschema:"Maximum traversal depth. Default: 4"`
}

// --- Search tool inputs ---

type SearchInput struct {
	Query        string `json:"query" jsonschema:"Search text to find across all blocks"`
	ContextLines int    `json:"contextLines,omitempty" jsonschema:"Number of parent/sibling blocks for context. Default: 2"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results. Default: 20"`
	Compact      bool   `json:"compact,omitempty" jsonschema:"Return minimal results (uuid, content, page) without parsed metadata. Saves ~50%% tokens. Default: false"`
}

type QueryPropertiesInput struct {
	Property string `json:"property" jsonschema:"Property key to search for"`
	Value    string `json:"value,omitempty" jsonschema:"Property value to match (omit to find all with this property)"`
	Operator string `json:"operator,omitempty" jsonschema:"Comparison: eq or contains or gt or lt. Default: eq"`
}

type QueryDatalogInput struct {
	Query  string `json:"query" jsonschema:"Datalog/DataScript query string"`
	Inputs []any  `json:"inputs,omitempty" jsonschema:"Query input bindings"`
}

type FindByTagInput struct {
	Tag             string `json:"tag" jsonschema:"Tag name to search for"`
	IncludeChildren bool   `json:"includeChildren,omitempty" jsonschema:"Include child tags in hierarchy. Default: true"`
}

// --- Analyze tool inputs ---

// GraphOverviewInput has no required params — returns global stats.
type GraphOverviewInput struct{}

type FindConnectionsInput struct {
	From     string `json:"from" jsonschema:"Starting page name"`
	To       string `json:"to" jsonschema:"Target page name"`
	MaxDepth int    `json:"maxDepth,omitempty" jsonschema:"Max search depth. Default: 5"`
}

// KnowledgeGapsInput controls knowledge gap analysis filtering.
type KnowledgeGapsInput struct {
	MinBlockCount  int  `json:"minBlockCount,omitempty" jsonschema:"Minimum block count for orphan pages. Filters out stray/empty pages. Default: 0"`
	ExcludeNumeric bool `json:"excludeNumeric,omitempty" jsonschema:"Exclude pages with purely numeric names (stray block refs). Default: false"`
}

// TopicClustersInput has no required params — returns community clusters.
type TopicClustersInput struct{}

// ListOrphansInput controls orphan page listing.
type ListOrphansInput struct {
	MinBlockCount  int  `json:"minBlockCount,omitempty" jsonschema:"Minimum block count to include. Filters stray/empty pages. Default: 0"`
	ExcludeNumeric bool `json:"excludeNumeric,omitempty" jsonschema:"Exclude pages with purely numeric names (stray block refs). Default: false"`
	Limit          int  `json:"limit,omitempty" jsonschema:"Max orphans to return. Default: 50"`
}

// --- Write tool inputs ---

type AppendBlocksInput struct {
	Page   string   `json:"page" jsonschema:"Page name to append blocks to"`
	Blocks []string `json:"blocks" jsonschema:"Block contents as plain strings (same format as create_page blocks)"`
}

type CreatePageInput struct {
	Name       string         `json:"name" jsonschema:"Page name to create"`
	Properties map[string]any `json:"properties,omitempty" jsonschema:"Page properties as key-value pairs"`
	Blocks     []string       `json:"blocks,omitempty" jsonschema:"Initial block contents to add to the page"`
}

type UpsertBlocksInput struct {
	Page     string       `json:"page" jsonschema:"Page name to add blocks to"`
	Blocks   []BlockInput `json:"blocks" jsonschema:"Blocks to create or update"`
	Position string       `json:"position,omitempty" jsonschema:"Where to add: append or prepend. Default: append"`
}

type BlockInput struct {
	Content    string            `json:"content" jsonschema:"Block text content"`
	Properties map[string]string `json:"properties,omitempty" jsonschema:"Block properties"`
	Children   []BlockInput      `json:"children,omitempty" jsonschema:"Nested child blocks"`
}

type UpdateBlockInput struct {
	UUID    string `json:"uuid" jsonschema:"UUID of block to update"`
	Content string `json:"content" jsonschema:"New content for the block (replaces existing content entirely)"`
}

type DeleteBlockInput struct {
	UUID string `json:"uuid" jsonschema:"UUID of block to delete"`
}

type MoveBlockInput struct {
	UUID       string `json:"uuid" jsonschema:"UUID of block to move"`
	TargetUUID string `json:"targetUuid" jsonschema:"UUID of target block"`
	Position   string `json:"position,omitempty" jsonschema:"Placement: before or after or child. Default: child"`
}

type DeletePageInput struct {
	Name string `json:"name" jsonschema:"Page name to delete"`
}

type RenamePageInput struct {
	OldName string `json:"oldName" jsonschema:"Current page name"`
	NewName string `json:"newName" jsonschema:"New page name"`
}

type BulkUpdatePropertiesInput struct {
	Pages    []string `json:"pages" jsonschema:"List of page names to update"`
	Property string   `json:"property" jsonschema:"Property key to set"`
	Value    string   `json:"value" jsonschema:"Property value to set"`
}

type LinkPagesInput struct {
	From    string `json:"from" jsonschema:"Source page name"`
	To      string `json:"to" jsonschema:"Target page name"`
	Context string `json:"context,omitempty" jsonschema:"Description of the relationship between pages"`
}

// --- Flashcard tool inputs ---

// FlashcardOverviewInput has no required params — returns SRS statistics.
type FlashcardOverviewInput struct{}

type FlashcardDueInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Max cards to return. Default: 20"`
}

type FlashcardCreateInput struct {
	Page  string `json:"page" jsonschema:"Page to create the card on"`
	Front string `json:"front" jsonschema:"Front of the card (question)"`
	Back  string `json:"back" jsonschema:"Back of the card (answer)"`
}

// --- Whiteboard tool inputs ---

// ListWhiteboardsInput has no required params.
type ListWhiteboardsInput struct{}

type GetWhiteboardInput struct {
	Name string `json:"name" jsonschema:"Whiteboard name"`
}

// --- Decision tool inputs ---

type DecisionCheckInput struct {
	IncludeResolved bool `json:"includeResolved,omitempty" jsonschema:"Include resolved (DONE) decisions. Default: false"`
}

type DecisionCreateInput struct {
	Page     string   `json:"page" jsonschema:"Page to create the decision on (where context is richest)"`
	Question string   `json:"question" jsonschema:"The decision question"`
	Deadline string   `json:"deadline" jsonschema:"Decision deadline (YYYY-MM-DD)"`
	Options  []string `json:"options,omitempty" jsonschema:"Available choices"`
	Context  string   `json:"context,omitempty" jsonschema:"Brief context for the decision"`
}

type DecisionResolveInput struct {
	UUID    string `json:"uuid" jsonschema:"UUID of the decision block to resolve"`
	Outcome string `json:"outcome,omitempty" jsonschema:"What was decided"`
}

type DecisionDeferInput struct {
	UUID        string `json:"uuid" jsonschema:"UUID of the decision block to defer"`
	NewDeadline string `json:"newDeadline" jsonschema:"New deadline (YYYY-MM-DD)"`
	Reason      string `json:"reason,omitempty" jsonschema:"Why the decision is being deferred"`
}

// AnalysisHealthInput has no required params — audits all analysis/strategy pages.
type AnalysisHealthInput struct{}

// --- Semantic search tool inputs ---

// SemanticSearchInput is the input for the dewey_semantic_search MCP tool.
type SemanticSearchInput struct {
	Query     string  `json:"query" jsonschema:"Natural language search query"`
	Limit     int     `json:"limit,omitempty" jsonschema:"Maximum number of results. Default: 10"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"Minimum similarity score (0.0-1.0). Default: 0.3"`
}

// SimilarInput is the input for the dewey_similar MCP tool.
type SimilarInput struct {
	Page  string `json:"page,omitempty" jsonschema:"Page name to find similar documents for"`
	UUID  string `json:"uuid,omitempty" jsonschema:"Block UUID to find similar blocks for. Takes precedence over page."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results. Default: 10"`
}

// SemanticSearchFilteredInput is the input for the dewey_semantic_search_filtered MCP tool.
type SemanticSearchFilteredInput struct {
	Query       string  `json:"query" jsonschema:"Natural language search query"`
	SourceType  string  `json:"source_type,omitempty" jsonschema:"Filter by source type: disk, github, web"`
	SourceID    string  `json:"source_id,omitempty" jsonschema:"Filter by specific source identifier (e.g., github-gaze)"`
	HasProperty string  `json:"has_property,omitempty" jsonschema:"Filter to pages with this frontmatter property key"`
	HasTag      string  `json:"has_tag,omitempty" jsonschema:"Filter to pages with this tag"`
	Tier        string  `json:"tier,omitempty" jsonschema:"Filter by trust tier: authored, curated, validated, or draft"`
	Limit       int     `json:"limit,omitempty" jsonschema:"Maximum number of results. Default: 10"`
	Threshold   float64 `json:"threshold,omitempty" jsonschema:"Minimum similarity score (0.0-1.0). Default: 0.3"`
}

// SemanticSearchResult represents a single result from semantic search.
// Includes provenance metadata per Constitution III (Observable Quality).
// CreatedAt, Tier, and Category provide temporal and trust metadata for
// knowledge compilation and contamination separation (013-knowledge-compile FR-004, FR-024).
// Tier values: authored > curated > validated > draft (015-curated-knowledge-stores).
type SemanticSearchResult struct {
	DocumentID string  `json:"document_id"`
	Page       string  `json:"page"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
	Source     string  `json:"source"`
	SourceID   string  `json:"source_id"`
	OriginURL  string  `json:"origin_url,omitempty"`
	IndexedAt  string  `json:"indexed_at"`
	CreatedAt  string  `json:"created_at,omitempty"`
	Tier       string  `json:"tier,omitempty"`
	Category   string  `json:"category,omitempty"`
}

// --- Indexing tool inputs ---

// IndexInput defines the parameters for the index MCP tool.
type IndexInput struct {
	SourceID string `json:"source_id,omitempty" jsonschema:"Optional source ID to re-index only that source. If omitted all configured sources are indexed."`
}

// ReindexInput defines the parameters for the reindex MCP tool.
type ReindexInput struct{}

// --- Learning tool inputs ---

// StoreLearningInput is the input for the dewey_store_learning MCP tool.
// BREAKING CHANGE: `tags` (plural, optional) replaced by `tag` (singular, required).
type StoreLearningInput struct {
	Information string `json:"information" jsonschema:"The learning text to store. Required."`
	Tag         string `json:"tag" jsonschema:"Required topic tag for this learning (e.g., authentication, vault-walker)"`
	Category    string `json:"category,omitempty" jsonschema:"Optional category: decision, pattern, gotcha, context, reference"`
	// Deprecated: Use Tag instead. If provided and Tag is empty, the first
	// tag from the comma-separated list is used for backward compatibility.
	Tags string `json:"tags,omitempty" jsonschema:"DEPRECATED: Use 'tag' instead. If provided and 'tag' is empty, first tag is used."`
}

// --- Journal tool inputs ---

type JournalRangeInput struct {
	From          string `json:"from" jsonschema:"Start date (YYYY-MM-DD)"`
	To            string `json:"to" jsonschema:"End date (YYYY-MM-DD)"`
	IncludeBlocks bool   `json:"includeBlocks,omitempty" jsonschema:"Include full block trees. Default: true"`
}

type JournalSearchInput struct {
	Query string `json:"query" jsonschema:"Text to search for in journal entries"`
	From  string `json:"from,omitempty" jsonschema:"Start date filter (YYYY-MM-DD)"`
	To    string `json:"to,omitempty" jsonschema:"End date filter (YYYY-MM-DD)"`
}

// --- Compile tool inputs ---

// CompileInput is the input for the dewey_compile MCP tool.
type CompileInput struct {
	// Incremental limits compilation to specific learning identities.
	// When empty, all learnings are compiled (full rebuild).
	Incremental []string `json:"incremental,omitempty" jsonschema:"Optional list of learning identities to compile incrementally (e.g., ['authentication-20260502T143022-alice']). When empty, performs full rebuild."`
}

// StoreCompiledInput is the input for the store_compiled MCP tool.
// Allows an agent to persist a compiled article it synthesized from
// compilation prompts returned by the compile tool.
type StoreCompiledInput struct {
	// Tag is the topic tag for the compiled article (e.g., "authentication").
	Tag string `json:"tag" jsonschema:"Topic tag for the compiled article (e.g., authentication)"`

	// Content is the compiled article content in markdown format.
	Content string `json:"content" jsonschema:"The compiled article content (markdown)"`

	// Sources lists the learning identities used to produce this article
	// (e.g., ["auth-1", "auth-3", "auth-4"]).
	Sources []string `json:"sources" jsonschema:"Learning identities that were compiled (e.g., auth-1, auth-3)"`

	// Model identifies the model that performed the synthesis, for provenance.
	Model string `json:"model,omitempty" jsonschema:"Model that performed synthesis (for provenance tracking)"`
}

// --- Curate tool inputs ---

// CurateInput is the input for the dewey_curate MCP tool.
type CurateInput struct {
	// Store is the name of the knowledge store to curate.
	// If omitted, curates all configured stores.
	Store string `json:"store,omitempty" jsonschema:"Name of the knowledge store to curate. If omitted curates all configured stores."`
	// Incremental limits curation to new/changed documents since last run.
	// Default: true.
	Incremental *bool `json:"incremental,omitempty" jsonschema:"Only process new/changed documents since last curation. Default: true"`
}

// --- Lint tool inputs ---

// LintInput is the input for the dewey_lint MCP tool.
type LintInput struct {
	// Fix enables auto-repair of mechanical issues (e.g., regenerate
	// missing embeddings). Does not fix semantic issues.
	Fix bool `json:"fix,omitempty" jsonschema:"Auto-repair mechanical issues (missing embeddings). Default: false"`
}

// --- Promote tool inputs ---

// PromoteInput is the input for the dewey_promote MCP tool.
type PromoteInput struct {
	// Page is the page name to promote from draft to validated.
	Page string `json:"page" jsonschema:"Page name to promote from draft to validated tier (e.g., 'learning/authentication-20260502T143022-alice')."`
}
