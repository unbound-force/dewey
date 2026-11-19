package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/backend"
	"github.com/unbound-force/dewey/v3/parser"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Search implements search and query MCP tools.
type Search struct {
	client backend.Backend
	// store resolves page sources for the optional source_id / has_tag
	// scope. Nil disables scoping: a scoped call returns an error result.
	store *store.Store
}

// NewSearch creates a new Search tool handler.
func NewSearch(c backend.Backend) *Search {
	return &Search{client: c}
}

// WithStore sets the persistent store used to resolve the optional
// source_id / has_tag scope on search and find_by_tag, and returns the
// handler. A nil store leaves scoping unavailable; unscoped calls are
// unaffected either way.
func (s *Search) WithStore(st *store.Store) *Search {
	s.store = st
	return s
}

// scopeMatcherFor returns the matcher for scope, or an error result when
// the scope is active and no store is configured to resolve it.
func (s *Search) scopeMatcherFor(scope pageScope) (*scopeMatcher, *mcp.CallToolResult) {
	if scope.active() && s.store == nil {
		return nil, errorResult(scopeUnavailableMessage)
	}
	return newScopeMatcher(scope, s.store), nil
}

// Search performs full-text search across all blocks with context.
// Uses FullTextSearcher (indexed) when available, falls back to brute-force scan.
// Optional source_id / has_tag narrow results with the same semantics as
// semantic_search_filtered; they require a persistent store (WithStore).
func (s *Search) Search(ctx context.Context, req *mcp.CallToolRequest, input types.SearchInput) (*mcp.CallToolResult, any, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	scope := pageScope{SourceID: input.SourceID, HasTag: input.HasTag}
	matcher, errResult := s.scopeMatcherFor(scope)
	if errResult != nil {
		return errResult, nil, nil
	}

	// Use indexed search if the backend supports it (Obsidian with SearchIndex).
	if searcher, ok := s.client.(backend.FullTextSearcher); ok {
		return s.searchIndexed(ctx, searcher, input, limit, matcher)
	}

	// Fall back to brute-force scan (Logseq or backends without index).
	return s.searchBruteForce(ctx, input, limit, matcher)
}

// searchIndexed uses the backend's inverted index for fast search. A
// scoped search over-fetches (scopedFetchLimit), drops hits outside the
// scope, then trims to limit.
func (s *Search) searchIndexed(ctx context.Context, searcher backend.FullTextSearcher, input types.SearchInput, limit int, matcher *scopeMatcher) (*mcp.CallToolResult, any, error) {
	hits, err := searcher.FullTextSearch(ctx, input.Query, scopedFetchLimit(limit, matcher.scope))
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}
	hits = filterHitsByScope(hits, matcher, limit)

	if len(hits) == 0 {
		return textResult(fmt.Sprintf("No results found for '%s'.", input.Query)), nil, nil
	}

	var results []map[string]any
	for _, hit := range hits {
		if input.Compact {
			results = append(results, map[string]any{
				"page":    hit.PageName,
				"uuid":    hit.UUID,
				"content": hit.Content,
			})
		} else {
			parsed := parser.Parse(hit.Content)
			results = append(results, map[string]any{
				"page":    hit.PageName,
				"uuid":    hit.UUID,
				"content": hit.Content,
				"parsed":  parsed,
			})
		}
	}

	res, err := jsonTextResult(map[string]any{
		"query":   input.Query,
		"count":   len(results),
		"results": results,
	})
	return res, nil, err
}

// filterHitsByScope keeps the hits matcher accepts, up to limit. With an
// inactive scope it only enforces limit, which the backend already did.
func filterHitsByScope(hits []backend.SearchHit, matcher *scopeMatcher, limit int) []backend.SearchHit {
	kept := make([]backend.SearchHit, 0, min(len(hits), limit))
	for _, hit := range hits {
		if len(kept) >= limit {
			break
		}
		if matcher.matches(hit.PageName, hit.Content) {
			kept = append(kept, hit)
		}
	}
	return kept
}

// searchBruteForce scans all pages sequentially (original implementation).
// Matches outside the optional scope are skipped before they count toward
// limit.
func (s *Search) searchBruteForce(ctx context.Context, input types.SearchInput, limit int, matcher *scopeMatcher) (*mcp.CallToolResult, any, error) {
	query := strings.ToLower(input.Query)

	pages, err := s.client.GetAllPages(ctx)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list pages: %v", err)), nil, nil
	}

	var results []map[string]any
	for _, page := range pages {
		if len(results) >= limit {
			break
		}
		if page.Name == "" {
			continue
		}

		blocks, err := s.client.GetPageBlocksTree(ctx, page.Name)
		if err != nil {
			continue
		}

		matches := searchBlockTree(blocks, query, page.OriginalName)
		for _, m := range matches {
			if len(results) >= limit {
				break
			}
			if content, _ := m["content"].(string); !matcher.matches(page.Name, content) {
				continue
			}
			if input.Compact {
				compact := map[string]any{
					"page":    m["page"],
					"uuid":    m["uuid"],
					"content": m["content"],
				}
				results = append(results, compact)
			} else {
				results = append(results, m)
			}
		}
	}

	if len(results) == 0 {
		return textResult(fmt.Sprintf("No results found for '%s'.", input.Query)), nil, nil
	}

	res, err := jsonTextResult(map[string]any{
		"query":   input.Query,
		"count":   len(results),
		"results": results,
	})
	return res, nil, err
}

// QueryProperties finds blocks/pages by property values.
func (s *Search) QueryProperties(ctx context.Context, req *mcp.CallToolRequest, input types.QueryPropertiesInput) (*mcp.CallToolResult, any, error) {
	operator := input.Operator
	if operator == "" {
		operator = "eq"
	}

	// Use native property search if the backend supports it (e.g. Obsidian).
	if searcher, ok := s.client.(backend.PropertySearcher); ok {
		results, err := searcher.FindByProperty(ctx, input.Property, input.Value, operator)
		if err != nil {
			return errorResult(fmt.Sprintf("property search failed: %v", err)), nil, nil
		}
		res, err := jsonTextResult(map[string]any{
			"property": input.Property,
			"value":    input.Value,
			"operator": operator,
			"count":    len(results),
			"results":  results,
		})
		return res, nil, err
	}

	// Fall back to DataScript (Logseq).
	var query string
	if input.Value == "" {
		query = fmt.Sprintf(`[:find (pull ?b [:block/uuid :block/content :block/properties {:block/page [:block/name :block/original-name]}])
			:where
			[?b :block/properties ?props]
			[(get ?props :%s)]]`, input.Property)
	} else {
		query = fmt.Sprintf(`[:find (pull ?b [:block/uuid :block/content :block/properties {:block/page [:block/name :block/original-name]}])
			:where
			[?b :block/properties ?props]
			[(get ?props :%s) ?v]
			[(str ?v) ?vs]
			[(clojure.string/includes? ?vs "%s")]]`, input.Property, input.Value)
	}

	raw, err := s.client.DatascriptQuery(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("property query failed: %v", err)), nil, nil
	}

	res, err := jsonRawTextResult(raw)
	return res, nil, err
}

// QueryDatalog executes raw DataScript queries.
func (s *Search) QueryDatalog(ctx context.Context, req *mcp.CallToolRequest, input types.QueryDatalogInput) (*mcp.CallToolResult, any, error) {
	raw, err := s.client.DatascriptQuery(ctx, input.Query, input.Inputs...)
	if err != nil {
		return errorResult(fmt.Sprintf("Datalog query failed: %v", err)), nil, nil
	}

	res, err := jsonRawTextResult(raw)
	return res, nil, err
}

// FindByTag finds content by tag, including child tags. An optional
// source_id keeps only pages of that source (requires a persistent store,
// see WithStore).
func (s *Search) FindByTag(ctx context.Context, req *mcp.CallToolRequest, input types.FindByTagInput) (*mcp.CallToolResult, any, error) {
	matcher, errResult := s.scopeMatcherFor(pageScope{SourceID: input.SourceID})
	if errResult != nil {
		return errResult, nil, nil
	}

	// Use native tag search if the backend supports it (e.g. Obsidian).
	if searcher, ok := s.client.(backend.TagSearcher); ok {
		results, err := searcher.FindBlocksByTag(ctx, input.Tag, input.IncludeChildren)
		if err != nil {
			return errorResult(fmt.Sprintf("tag search failed: %v", err)), nil, nil
		}

		var enriched []map[string]any
		for _, r := range results {
			if !matcher.matches(r.Page, "") {
				continue
			}
			for _, block := range r.Blocks {
				parsed := parser.Parse(block.Content)
				enriched = append(enriched, map[string]any{
					"uuid":    block.UUID,
					"content": block.Content,
					"parsed":  parsed,
					"page":    r.Page,
				})
			}
		}

		res, err := jsonTextResult(map[string]any{
			"tag":     input.Tag,
			"count":   len(enriched),
			"results": enriched,
		})
		return res, nil, err
	}

	// Fall back to DataScript (Logseq).
	query := fmt.Sprintf(`[:find (pull ?b [:block/uuid :block/content {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "%s"]]`, strings.ToLower(input.Tag))

	raw, err := s.client.DatascriptQuery(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("tag query failed: %v", err)), nil, nil
	}

	var results [][]json.RawMessage
	if err := json.Unmarshal(raw, &results); err != nil {
		res, err := jsonRawTextResult(raw)
		return res, nil, err
	}

	var enriched []map[string]any
	for _, r := range results {
		if len(r) == 0 {
			continue
		}
		var block types.BlockEntity
		if err := json.Unmarshal(r[0], &block); err != nil {
			continue
		}
		pageName := ""
		if block.Page != nil {
			pageName = block.Page.Name
		}
		if !matcher.matches(pageName, block.Content) {
			continue
		}
		parsed := parser.Parse(block.Content)
		entry := map[string]any{
			"uuid":    block.UUID,
			"content": block.Content,
			"parsed":  parsed,
		}
		if block.Page != nil {
			entry["page"] = block.Page.Name
		}
		enriched = append(enriched, entry)
	}

	res, err := jsonTextResult(map[string]any{
		"tag":     input.Tag,
		"count":   len(enriched),
		"results": enriched,
	})
	return res, nil, err
}

// --- Internal helpers ---

func searchBlockTree(blocks []types.BlockEntity, query, pageName string) []map[string]any {
	var results []map[string]any

	// Split query into terms for multi-word matching.
	// A block matches if it contains ALL terms (in any order).
	terms := splitSearchTerms(query)

	searchBlocksRecursive(blocks, terms, pageName, nil, &results)
	return results
}

// splitSearchTerms splits a query into individual lowercase terms,
// filtering out empty strings.
func splitSearchTerms(query string) []string {
	parts := strings.Fields(query)
	terms := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			terms = append(terms, p)
		}
	}
	return terms
}

// blockMatchesTerms returns true if the block content contains all given terms.
func blockMatchesTerms(content string, terms []string) bool {
	lower := strings.ToLower(content)
	for _, t := range terms {
		if !strings.Contains(lower, t) {
			return false
		}
	}
	return len(terms) > 0
}

func searchBlocksRecursive(blocks []types.BlockEntity, terms []string, pageName string, parentChain []types.BlockSummary, results *[]map[string]any) {
	for i, b := range blocks {
		if blockMatchesTerms(b.Content, terms) {
			var siblings []types.BlockSummary
			start := i - 1
			if start < 0 {
				start = 0
			}
			end := i + 2
			if end > len(blocks) {
				end = len(blocks)
			}
			for j := start; j < end; j++ {
				if j != i {
					siblings = append(siblings, types.BlockSummary{
						UUID:    blocks[j].UUID,
						Content: blocks[j].Content,
					})
				}
			}

			parsed := parser.Parse(b.Content)
			match := map[string]any{
				"page":        pageName,
				"uuid":        b.UUID,
				"content":     b.Content,
				"parsed":      parsed,
				"parentChain": parentChain,
				"siblings":    siblings,
			}
			*results = append(*results, match)
		}

		if len(b.Children) > 0 {
			chain := append(append([]types.BlockSummary{}, parentChain...), types.BlockSummary{
				UUID:    b.UUID,
				Content: b.Content,
			})
			searchBlocksRecursive(b.Children, terms, pageName, chain, results)
		}
	}
}
