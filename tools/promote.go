package tools

import (
	"context"
	"fmt"
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
// "draft" to "validated". Page names are resolved through store.FindPage, so
// bare tags resolve against the learning/compiled namespaces and matching is
// case-insensitive. Returns an error result if any requested page cannot be
// promoted.
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

	type target struct {
		requested string
		page      *store.Page
	}
	targets := make([]target, 0, len(requested))
	resolvedNames := make(map[string]struct{}, len(requested))
	failures := make([]map[string]string, 0)

	for _, name := range requested {
		if strings.TrimSpace(name) == "" {
			failures = append(failures, map[string]string{"page": name, "error": "page name is required"})
			continue
		}

		page, err := p.store.FindPage(name)
		if err != nil {
			if !batch {
				return errorResult(fmt.Sprintf("failed to resolve page '%s': %s", name, err)), nil, nil
			}
			failures = append(failures, map[string]string{"page": name, "error": err.Error()})
			continue
		}
		if page == nil {
			if !batch {
				return errorResult(fmt.Sprintf("Page '%s' not found.", name)), nil, nil
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
				return errorResult(fmt.Sprintf(
					"Page '%s' has tier '%s' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'.",
					page.Name, page.Tier,
				)), nil, nil
			}
			failures = append(failures, map[string]string{
				"page":  page.Name,
				"error": fmt.Sprintf("tier is '%s'; only 'draft' pages can be promoted to 'validated'", page.Tier),
			})
			continue
		}

		targets = append(targets, target{requested: name, page: page})
	}

	if len(failures) > 0 {
		payload := map[string]any{
			"status":       "failed",
			"failed":       failures,
			"total_failed": len(failures),
			"message":      fmt.Sprintf("%d page(s) could not be promoted; no tiers were changed.", len(failures)),
		}
		res, err := jsonTextResult(payload)
		if err != nil {
			return nil, nil, err
		}
		res.IsError = true
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
		failed := make([]map[string]string, 0, len(targets))
		for _, target := range targets {
			failed = append(failed, map[string]string{
				"page":  target.page.Name,
				"error": fmt.Sprintf("transaction rolled back: %v", err),
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
			return nil, nil, jsonErr
		}
		res.IsError = true
		return res, nil, nil
	}
	promoted := make([]map[string]string, 0, len(targets))
	for _, t := range targets {
		promoted = append(promoted, map[string]string{
			"page":          t.page.Name,
			"previous_tier": "draft",
			"new_tier":      "validated",
		})
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
		"promoted":       promoted,
		"total_promoted": len(promoted),
		"message":        fmt.Sprintf("%d page(s) promoted to validated tier.", len(promoted)),
	}
	res, err := jsonTextResult(result)
	return res, nil, err
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
