package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// Promote implements the dewey_promote MCP tool and CLI command.
// Transitions a page from draft tier to validated tier after human review.
//
// Design decision: Only the draft → validated transition is supported
// (invariant 1). Authored pages cannot be promoted because they represent
// original vault content, not agent-generated material. Validated pages
// cannot be re-promoted because they are already at the highest
// agent-achievable trust level.
type Promote struct {
	store *store.Store
}

// NewPromote creates a new Promote tool handler with the given store.
// The store must be non-nil for the tool to function; a clear error is
// returned at call time if it is nil.
func NewPromote(s *store.Store) *Promote {
	return &Promote{store: s}
}

// Promote handles the dewey_promote MCP tool. Changes a page's trust
// tier from "draft" to "validated". Only pages with tier "draft" can
// be promoted. Returns an error if the page doesn't exist, is not
// draft tier, or the store is unavailable.
func (p *Promote) Promote(ctx context.Context, req *mcp.CallToolRequest, input types.PromoteInput) (*mcp.CallToolResult, any, error) {
	if p.store == nil {
		return errorResult("promote requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	if input.Page == "" {
		return errorResult("page parameter is required"), nil, nil
	}

	// Look up the page to validate it exists and check its current tier.
	page, err := p.store.GetPage(input.Page)
	if err != nil {
		return errorResult(fmt.Sprintf("Page '%s' not found.", input.Page)), nil, nil
	}
	if page == nil {
		return errorResult(fmt.Sprintf("Page '%s' not found.", input.Page)), nil, nil
	}

	// Validate current tier is draft (invariant 1: only draft → validated).
	if page.Tier != "draft" {
		return errorResult(fmt.Sprintf(
			"Page '%s' has tier '%s' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'.",
			input.Page, page.Tier,
		)), nil, nil
	}

	// Promote: update tier from draft to validated.
	// UpdatePageTier also refreshes updated_at (invariant 4).
	if err := p.store.UpdatePageTier(input.Page, "validated"); err != nil {
		return errorResult(fmt.Sprintf("failed to promote page: %v", err)), nil, nil
	}

	result := map[string]any{
		"page":          input.Page,
		"previous_tier": "draft",
		"new_tier":      "validated",
		"message":       fmt.Sprintf("Page '%s' promoted to validated tier.", input.Page),
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}
