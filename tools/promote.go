package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Promote implements the dewey_promote MCP tool and CLI command.
// Transitions pages from draft tier to validated tier after human review.
//
// Design decision: Only the draft → validated transition is supported
// (invariant 1). Authored pages cannot be promoted because they represent
// original vault content, not agent-generated material. Validated pages
// cannot be re-promoted because they are already at the highest
// agent-achievable trust level.
//
// Promotion accepts either a single page name (Page) or a batch (Pages).
// Batch promotion validates every page before mutating any of them: if one
// page is missing, ambiguous, or not draft, no tier changes and the result
// lists each failure. This keeps a human review decision all-or-nothing.
type Promote struct {
	store *store.Store
}

// NewPromote creates a new Promote tool handler with the given store.
// The store must be non-nil for the tool to function; a clear error is
// returned at call time if it is nil.
func NewPromote(s *store.Store) *Promote {
	return &Promote{store: s}
}

// Promote handles the dewey_promote MCP tool. Changes page trust tiers from
// "draft" to "validated". Page names are resolved through resolvePromotePage,
// a tools-side ladder over narrow index-backed store queries: exact name,
// then case-insensitive name, then the learning/compiled/knowledge namespaces
// for bare tags, then source doc IDs. Returns an error result if any
// requested page cannot be promoted.
func (p *Promote) Promote(_ context.Context, _ *mcp.CallToolRequest, input types.PromoteInput) (*mcp.CallToolResult, any, error) {
	if p.store == nil {
		return errorResult(persistentStorageError("promote").Error()), nil, nil
	}

	batch := input.Pages != nil
	if batch && input.Page != "" {
		return errorResult("provide either page or pages, not both"), nil, nil
	}
	if !batch && strings.TrimSpace(input.Page) == "" {
		return errorResult("page or pages parameter is required"), nil, nil
	}

	requested := []string{input.Page}
	if batch {
		if len(input.Pages) == 0 {
			return errorResult("pages must contain at least one page name"), nil, nil
		}
		requested = dedupePageNames(input.Pages)
	}

	targets, failures, singleErr := p.resolvePromoteTargets(requested, batch)
	if singleErr != nil {
		return singleErr, nil, nil
	}

	if len(failures) > 0 {
		res, err := promoteFailureResult(failures)
		if err != nil {
			return nil, nil, err
		}
		return res, nil, nil
	}

	// All requested pages resolved and are drafts. Promote them in one
	// transaction so a mid-batch failure changes nothing.
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.page.Name)
	}
	if err := p.store.PromotePagesTiers(names); err != nil {
		if !batch {
			return errorResult(fmt.Sprintf("failed to promote page: %v", err)), nil, nil
		}
		res, renderErr := rollbackFailureResult(targets, err)
		if renderErr != nil {
			return nil, nil, renderErr
		}
		return res, nil, nil
	}
	if !batch {
		page := targets[0].page
		result := map[string]any{
			"page":          page.Name,
			"previous_tier": "draft",
			"new_tier":      "validated",
			"message":       fmt.Sprintf("Page '%s' promoted to validated tier.", page.Name),
		}
		res, err := jsonTextResult(result)
		return res, nil, err
	}

	result := map[string]any{
		"status":         "promoted",
		"promoted":       promotedEntries(targets),
		"total_promoted": len(targets),
		"message":        fmt.Sprintf("%d page(s) promoted to validated tier.", len(targets)),
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// promoteTarget pairs a requested input name with its resolved store page.
type promoteTarget struct {
	requested string
	page      *store.Page
}

// resolvePromoteTargets resolves every requested name through the
// resolution ladder and keeps only pages that are safe to promote. In
// single-page mode the first problem aborts with a rendered error result;
// in batch mode problems accumulate as per-page failure entries. The error
// strings are the handler's contract and must not change (PR 145 review
// amendment).
func (p *Promote) resolvePromoteTargets(requested []string, batch bool) ([]promoteTarget, []map[string]string, *mcp.CallToolResult) {
	targets := make([]promoteTarget, 0, len(requested))
	resolvedNames := make(map[string]struct{}, len(requested))
	failures := make([]map[string]string, 0)

	for _, name := range requested {
		if strings.TrimSpace(name) == "" {
			failures = append(failures, map[string]string{"page": name, "error": "page name is required"})
			continue
		}

		page, err := resolvePromotePage(p.store, name)
		if err != nil {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf("failed to look up page '%s': %s", name, err))
			}
			failures = append(failures, map[string]string{"page": name, "error": err.Error()})
			continue
		}
		if page == nil {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf("Page '%s' not found.", name))
			}
			failures = append(failures, map[string]string{"page": name, "error": "page not found"})
			continue
		}

		// Two different inputs (e.g. "one" and "compiled/one") can resolve
		// to the same page. Validate and report it once.
		if _, seen := resolvedNames[page.Name]; seen {
			continue
		}
		resolvedNames[page.Name] = struct{}{}

		if page.Tier != "draft" {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf(
					"Page '%s' has tier '%s' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'.",
					page.Name, page.Tier,
				))
			}
			failures = append(failures, map[string]string{
				"page":  page.Name,
				"error": fmt.Sprintf("tier is '%s'; only 'draft' pages can be promoted to 'validated'", page.Tier),
			})
			continue
		}

		targets = append(targets, promoteTarget{requested: name, page: page})
	}
	return targets, failures, nil
}

// promoteFailureResult renders the pre-flight failure result: some pages
// could not be resolved, so no tiers were changed.
func promoteFailureResult(failures []map[string]string) (*mcp.CallToolResult, error) {
	payload := map[string]any{
		"status":       "failed",
		"failed":       failures,
		"total_failed": len(failures),
		"message":      fmt.Sprintf("%d page(s) could not be promoted; no tiers were changed.", len(failures)),
	}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, err
	}
	res.IsError = true
	return res, nil
}

// rollbackFailureResult renders the mid-transaction failure result: the
// batch rolled back and no tiers were changed.
func rollbackFailureResult(targets []promoteTarget, promoteErr error) (*mcp.CallToolResult, error) {
	failed := make([]map[string]string, 0, len(targets))
	for _, target := range targets {
		failed = append(failed, map[string]string{
			"page":  target.page.Name,
			"error": fmt.Sprintf("transaction rolled back: %v", promoteErr),
		})
	}
	payload := map[string]any{
		"status":         "failed",
		"promoted":       []any{},
		"failed":         failed,
		"total_promoted": 0,
		"total_failed":   len(failed),
		"message":        "promotion transaction failed; no tiers were changed.",
	}
	res, jsonErr := jsonTextResult(payload)
	if jsonErr != nil {
		return nil, jsonErr
	}
	res.IsError = true
	return res, nil
}

// promotedEntries renders the per-page success entries for the batch result.
func promotedEntries(targets []promoteTarget) []map[string]string {
	promoted := make([]map[string]string, 0, len(targets))
	for _, t := range targets {
		promoted = append(promoted, map[string]string{
			"page":          t.page.Name,
			"previous_tier": "draft",
			"new_tier":      "validated",
		})
	}
	return promoted
}

// dedupePageNames removes duplicate entries while preserving request order.
// Blank entries are retained so batch validation can report them instead of
// silently ignoring part of the request.
func dedupePageNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// resolvePromotePage resolves a promote page reference to a stored page
// (dewey-distill feedback item 10). The ladder runs narrow, index-backed
// store queries and keeps the resolution policy in the tools layer:
//
//  1. exact page name (GetPage)
//  2. case-insensitive page name (ListPagesByNameFoldCase)
//  3. bare tags only: compiled/{name}, learning/{name}, and knowledge
//     pages whose name ends with "/{name}"
//  4. source doc IDs: "source/doc" pairs, then the bare tag against the
//     compiled and learning sources
//
// A unique match returns the page; multiple matches return an ambiguity
// error naming the candidates; no match returns (nil, nil) so callers can
// distinguish not-found from a store failure.
func resolvePromotePage(s *store.Store, name string) (*store.Page, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("page name is required")
	}

	// Tier 1: exact name.
	page, err := s.GetPage(trimmed)
	if err != nil {
		return nil, err
	}
	if page != nil {
		return page, nil
	}

	// Tier 2: case-insensitive exact name.
	matches, err := s.ListPagesByNameFoldCase(trimmed)
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, ambiguousPageError(trimmed, matches)
	}

	// Tier 3: bare-tag namespaces. Slash-containing references skip this
	// tier; they address pages by name or source doc directly.
	if !strings.Contains(trimmed, "/") {
		var candidates []*store.Page
		for _, prefix := range []string{store.CompiledPrefix, store.LearningPrefix} {
			ns, err := s.ListPagesByNameFoldCase(prefix + trimmed)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, ns...)
		}
		knowledge, err := s.ListKnowledgePagesFoldCase()
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filterPagesBySuffix(knowledge, trimmed)...)
		candidates = dedupePages(candidates)
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		if len(candidates) > 1 {
			return nil, ambiguousPageError(trimmed, candidates)
		}
		// No namespace candidate: fall through to tier 4.
	}

	// Tier 4: source doc IDs. "source/doc" pairs address the doc directly;
	// bare tags search the compiled and learning sources.
	if source, doc, found := strings.Cut(trimmed, "/"); found {
		docMatches, err := s.GetPageBySourceDoc(source, doc)
		if err != nil {
			return nil, err
		}
		if len(docMatches) == 1 {
			return docMatches[0], nil
		}
		if len(docMatches) > 1 {
			return nil, ambiguousPageError(trimmed, docMatches)
		}
		return nil, nil
	}
	var candidates []*store.Page
	for _, source := range []string{store.SourceCompiled, store.SourceLearning} {
		docMatches, err := s.GetPageBySourceDoc(source, trimmed)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, docMatches...)
	}
	candidates = dedupePages(candidates)
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return nil, ambiguousPageError(trimmed, candidates)
	}
	return nil, nil
}

// filterPagesBySuffix returns pages whose name ends with "/suffix",
// case-insensitively (e.g. knowledge store pages for a bare tag).
func filterPagesBySuffix(pages []*store.Page, suffix string) []*store.Page {
	needle := "/" + strings.ToLower(suffix)
	out := make([]*store.Page, 0, len(pages))
	for _, p := range pages {
		if strings.HasSuffix(strings.ToLower(p.Name), needle) {
			out = append(out, p)
		}
	}
	return out
}

// dedupePages removes pages with duplicate names, preserving order.
func dedupePages(pages []*store.Page) []*store.Page {
	seen := make(map[string]struct{}, len(pages))
	out := make([]*store.Page, 0, len(pages))
	for _, p := range pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		out = append(out, p)
	}
	return out
}

// ambiguousPageError reports that a page reference matched multiple pages.
// The candidate list is sorted and capped so pathological matches cannot
// flood the error message.
func ambiguousPageError(name string, pages []*store.Page) error {
	names := make([]string, 0, len(pages))
	for _, p := range pages {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	const maxMatches = 10
	if len(names) > maxMatches {
		return fmt.Errorf("page name %q is ambiguous; matches: %s (and %d more)",
			name, strings.Join(names[:maxMatches], ", "), len(names)-maxMatches)
	}
	return fmt.Errorf("page name %q is ambiguous; matches: %s", name, strings.Join(names, ", "))
}
