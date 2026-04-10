package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// Semantic implements the 3 semantic search MCP tools:
// dewey_semantic_search, dewey_similar, and dewey_semantic_search_filtered.
//
// Design decision: The embedder and store are injected as dependencies
// (Dependency Inversion Principle) to enable testing with mocks and to
// support graceful degradation when Ollama is unavailable.
type Semantic struct {
	embedder embed.Embedder
	store    *store.Store
}

// NewSemantic creates a new Semantic tool handler with the given embedder
// and store. Both embedder and store may be nil — the tools return clear
// MCP error messages when unavailable, enabling graceful degradation when
// Ollama is not running or no persistent store is configured.
//
// Returns a ready-to-use handler for the three semantic search MCP tools.
func NewSemantic(e embed.Embedder, s *store.Store) *Semantic {
	return &Semantic{embedder: e, store: s}
}

// SemanticSearch handles the dewey_semantic_search MCP tool. Embeds the
// query text via the configured embedder, then searches for similar blocks
// via cosine similarity. Returns a JSON array of [types.SemanticSearchResult]
// with provenance metadata (page, source, similarity score, indexed
// timestamp). Defaults limit to 10 and threshold to 0.3 if not specified.
//
// Returns an MCP error result (not a Go error) if the embedder is
// unavailable, the store is nil, embedding fails, or the search fails.
func (s *Semantic) SemanticSearch(ctx context.Context, req *mcp.CallToolRequest, input types.SemanticSearchInput) (*mcp.CallToolResult, any, error) {
	if s.embedder == nil || !s.embedder.Available() {
		return errorResult("Semantic search unavailable: embedding model not loaded. Ensure Ollama is running with the configured model."), nil, nil
	}
	if s.store == nil {
		return errorResult("Semantic search unavailable: no persistent store configured."), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	threshold := input.Threshold
	if threshold <= 0 {
		threshold = 0.3
	}

	// Embed the query text.
	queryVec, err := s.embedder.Embed(ctx, input.Query)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to embed query: %v", err)), nil, nil
	}

	// Search for similar blocks.
	results, err := s.store.SearchSimilar(s.embedder.ModelID(), queryVec, limit, threshold)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err)), nil, nil
	}

	// Convert to output format with provenance metadata.
	output := toSemanticResults(results, s.store)

	res, err := jsonTextResult(output)
	return res, nil, err
}

// Similar handles the dewey_similar MCP tool. Finds documents similar to
// a given page or block by looking up its existing embedding and searching
// for similar vectors. At least one of input.Page or input.UUID must be
// provided. Returns a JSON array of [types.SemanticSearchResult] excluding
// the query document itself. Defaults limit to 10.
//
// Returns an MCP error result (not a Go error) if neither page nor UUID
// is provided, the embedder is unavailable, no embeddings exist in the
// index, or the referenced page/block has no embedding.
func (s *Semantic) Similar(ctx context.Context, req *mcp.CallToolRequest, input types.SimilarInput) (*mcp.CallToolResult, any, error) {
	if errResult := s.validateSimilarInput(input); errResult != nil {
		return errResult, nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	modelID := s.embedder.ModelID()

	queryVec, errResult := s.resolveQueryVector(ctx, input, modelID)
	if errResult != nil {
		return errResult, nil, nil
	}

	results, err := s.store.SearchSimilar(modelID, queryVec, limit+1, 0.0)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err)), nil, nil
	}

	filtered := filterSimilarResults(results, input.UUID, limit)
	output := toSemanticResults(filtered, s.store)

	res, err := jsonTextResult(output)
	return res, nil, err
}

// validateSimilarInput checks that the input has at least one identifier
// and that the embedder and store are available. Returns an error result
// or nil if validation passes.
func (s *Semantic) validateSimilarInput(input types.SimilarInput) *mcp.CallToolResult {
	if input.Page == "" && input.UUID == "" {
		return errorResult("At least one of 'page' or 'uuid' must be provided.")
	}
	if s.embedder == nil || !s.embedder.Available() {
		return errorResult("Semantic search unavailable: embedding model not loaded. Ensure Ollama is running with the configured model.")
	}
	if s.store == nil {
		return errorResult("Semantic search unavailable: no persistent store configured.")
	}
	return nil
}

// resolveQueryVector looks up the embedding vector for the given input,
// either by block UUID or by finding the first embedded block for a page.
// Returns the vector or an error result.
func (s *Semantic) resolveQueryVector(_ context.Context, input types.SimilarInput, modelID string) ([]float32, *mcp.CallToolResult) {
	// Check if any embeddings exist at all.
	count, err := s.store.CountEmbeddings()
	if err != nil {
		return nil, errorResult(fmt.Sprintf("Failed to check embeddings: %v", err))
	}
	if count == 0 {
		return nil, errorResult("No embeddings in index. Run `dewey index` to generate embeddings.")
	}

	if input.UUID != "" {
		emb, err := s.store.GetEmbedding(input.UUID, modelID)
		if err != nil {
			return nil, errorResult(fmt.Sprintf("Failed to look up embedding: %v", err))
		}
		if emb == nil {
			return nil, errorResult(fmt.Sprintf("No embedding found for %s. Run `dewey index` to generate embeddings.", input.UUID))
		}
		return emb.Vector, nil
	}

	// Look up by page name — find the first block's embedding.
	blocks, err := s.store.GetBlocksByPage(input.Page)
	if err != nil || len(blocks) == 0 {
		return nil, errorResult(fmt.Sprintf("Page/block not found: %s", input.Page))
	}

	for _, block := range blocks {
		emb, err := s.store.GetEmbedding(block.UUID, modelID)
		if err == nil && emb != nil {
			return emb.Vector, nil
		}
	}
	return nil, errorResult(fmt.Sprintf("No embedding found for %s. Run `dewey index` to generate embeddings.", input.Page))
}

// filterSimilarResults removes the query document from results and enforces
// the limit. This is a pure function with no side effects.
func filterSimilarResults(results []store.SimilarityResult, excludeUUID string, limit int) []store.SimilarityResult {
	filtered := make([]store.SimilarityResult, 0, len(results))
	for _, r := range results {
		if r.BlockUUID == excludeUUID {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

// SemanticSearchFiltered handles the dewey_semantic_search_filtered MCP
// tool. Combines semantic search with metadata filters (source type,
// source ID, property, tag) to narrow results. Filters are applied at the
// SQL level before vector comparison for efficiency. Returns a JSON array
// of [types.SemanticSearchResult]. Defaults limit to 10 and threshold to 0.3.
//
// Returns an MCP error result (not a Go error) if the embedder is
// unavailable, the store is nil, embedding fails, or the filtered search fails.
func (s *Semantic) SemanticSearchFiltered(ctx context.Context, req *mcp.CallToolRequest, input types.SemanticSearchFilteredInput) (*mcp.CallToolResult, any, error) {
	if s.embedder == nil || !s.embedder.Available() {
		return errorResult("Semantic search unavailable: embedding model not loaded. Ensure Ollama is running with the configured model."), nil, nil
	}
	if s.store == nil {
		return errorResult("Semantic search unavailable: no persistent store configured."), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	threshold := input.Threshold
	if threshold <= 0 {
		threshold = 0.3
	}

	// Embed the query text.
	queryVec, err := s.embedder.Embed(ctx, input.Query)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to embed query: %v", err)), nil, nil
	}

	// Build filters.
	filters := store.SearchFilters{
		SourceType:  input.SourceType,
		SourceID:    input.SourceID,
		HasProperty: input.HasProperty,
		HasTag:      input.HasTag,
	}

	// When tier filtering is requested, fetch extra results to account for
	// post-query filtering. The tier filter is applied after similarity ranking
	// because the store's SearchFilters does not support tier natively.
	// Design decision: Post-query filter chosen over modifying store/ to keep
	// this change scoped to the tools layer (Single Responsibility Principle).
	fetchLimit := limit
	if input.Tier != "" {
		fetchLimit = limit * 3 // Over-fetch to compensate for post-filter reduction.
		if fetchLimit < 30 {
			fetchLimit = 30
		}
	}

	// Search with filters.
	results, err := s.store.SearchSimilarFiltered(s.embedder.ModelID(), queryVec, filters, fetchLimit, threshold)
	if err != nil {
		return errorResult(fmt.Sprintf("Search failed: %v", err)), nil, nil
	}

	// Apply tier post-filter when requested (FR-024).
	if input.Tier != "" {
		results = filterResultsByTier(results, input.Tier, s.store)
	}

	// Enforce the original limit after tier filtering.
	if len(results) > limit {
		results = results[:limit]
	}

	output := toSemanticResults(results, s.store)

	res, err := jsonTextResult(output)
	return res, nil, err
}

// toSemanticResults converts store.SimilarityResult to types.SemanticSearchResult
// with ISO 8601 timestamps and page-level metadata (tier, category, created_at)
// for the MCP response. The store parameter is used to look up page metadata
// that isn't carried by SimilarityResult (FR-004, 013-knowledge-compile).
//
// Design decision: Page metadata is looked up per unique page name (not per
// result) to avoid N+1 queries when multiple results reference the same page.
// A nil store is handled gracefully — metadata fields are left empty.
func toSemanticResults(results []store.SimilarityResult, st *store.Store) []types.SemanticSearchResult {
	// Build a cache of page metadata keyed by page name to avoid N+1 lookups.
	pageCache := make(map[string]*store.Page)
	if st != nil {
		for _, r := range results {
			if r.PageName != "" {
				if _, ok := pageCache[r.PageName]; !ok {
					page, err := st.GetPage(r.PageName)
					if err == nil && page != nil {
						pageCache[r.PageName] = page
					}
				}
			}
		}
	}

	output := make([]types.SemanticSearchResult, len(results))
	for i, r := range results {
		indexedAt := ""
		if r.IndexedAt > 0 {
			indexedAt = time.UnixMilli(r.IndexedAt).UTC().Format(time.RFC3339)
		}

		result := types.SemanticSearchResult{
			DocumentID: r.BlockUUID,
			Page:       r.PageName,
			Content:    r.Content,
			Similarity: r.Similarity,
			Source:     r.Source,
			SourceID:   r.SourceID,
			OriginURL:  r.OriginURL,
			IndexedAt:  indexedAt,
		}

		// Enrich with page-level metadata when available.
		if page, ok := pageCache[r.PageName]; ok {
			result.Tier = page.Tier
			result.Category = page.Category
			if page.CreatedAt > 0 {
				result.CreatedAt = time.UnixMilli(page.CreatedAt).UTC().Format(time.RFC3339)
			}
		}

		output[i] = result
	}
	return output
}

// filterResultsByTier removes results whose page tier does not match the
// requested tier. Uses the store to look up page metadata. Results with
// unknown pages (no page metadata) are excluded when a tier filter is active.
// This is a post-query filter applied after similarity ranking (FR-024).
func filterResultsByTier(results []store.SimilarityResult, tier string, st *store.Store) []store.SimilarityResult {
	if st == nil {
		return results
	}

	// Build page cache for tier lookups.
	pageCache := make(map[string]*store.Page)
	for _, r := range results {
		if r.PageName != "" {
			if _, ok := pageCache[r.PageName]; !ok {
				page, err := st.GetPage(r.PageName)
				if err == nil && page != nil {
					pageCache[r.PageName] = page
				}
			}
		}
	}

	filtered := make([]store.SimilarityResult, 0, len(results))
	for _, r := range results {
		page, ok := pageCache[r.PageName]
		if !ok {
			continue // Unknown page — exclude when tier filter is active.
		}
		if page.Tier == tier {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
