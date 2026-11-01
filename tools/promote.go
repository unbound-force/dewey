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
// be promoted. The page is located by its exact name first; when that
// name is absent and the input contains no "/", the shorthand is
// resolved by trying "compiled/{name}" then "learning/{name}". Returns
// an error if the page doesn't exist, is not draft tier, or the store
// is unavailable; a failed store lookup surfaces the underlying error
// so it is not mistaken for absence.
func (p *Promote) Promote(ctx context.Context, req *mcp.CallToolRequest, input types.PromoteInput) (*mcp.CallToolResult, any, error) {
	if p.store == nil {
		return errorResult(persistentStorageError("promote").Error()), nil, nil
	}

	if input.Page == "" {
		return errorResult("page parameter is required"), nil, nil
	}

	// Resolve the page: exact name first, then the compiled/ and
	// learning/ shorthand prefixes when the input contains no "/"
	// (spec: Promote Name Resolution). Absence and store failure are
	// kept distinct so store failures stay diagnosable instead of
	// masked as "not found" (issue #91).
	page, name, err := p.resolvePage(input.Page)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if page == nil {
		return errorResult(fmt.Sprintf("Page '%s' not found.", input.Page)), nil, nil
	}

	// Validate current tier is draft (invariant 1: only draft → validated).
	if page.Tier != "draft" {
		return errorResult(fmt.Sprintf(
			"Page '%s' has tier '%s' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'.",
			name, page.Tier,
		)), nil, nil
	}

	// Promote: update tier from draft to validated.
	// UpdatePageTier also refreshes updated_at (invariant 4).
	if err := p.store.UpdatePageTier(name, "validated"); err != nil {
		return errorResult(fmt.Sprintf("failed to promote page: %v", err)), nil, nil
	}

	result := map[string]any{
		"page":          name,
		"previous_tier": "draft",
		"new_tier":      "validated",
		"message":       fmt.Sprintf("Page '%s' promoted to validated tier.", name),
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}

// resolvePage locates the page to promote. The exact name is tried
// first; when it is genuinely absent and the input contains no "/", the
// "compiled/{name}" and "learning/{name}" shorthand prefixes are tried
// in that order — #91 records agents calling promote with the bare tag,
// and the first existing candidate wins. GetPage's contract: a missing
// page is (nil, nil); a failed lookup is (nil, err). Accordingly, this
// returns the page and the name it was found under, or (nil, name, nil)
// when nothing exists (so the caller reports the original input as not
// found), and a lookup failure returns an error naming the page that
// was queried — store errors are never masked as absence (issue #91).
func (p *Promote) resolvePage(name string) (*store.Page, string, error) {
	page, err := p.store.GetPage(name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to look up page '%s': %w", name, err)
	}
	if page != nil || strings.Contains(name, "/") {
		return page, name, nil
	}
	for _, prefix := range []string{"compiled/", "learning/"} {
		candidate := prefix + name
		candidatePage, err := p.store.GetPage(candidate)
		if err != nil {
			return nil, "", fmt.Errorf("failed to look up page '%s': %w", candidate, err)
		}
		if candidatePage != nil {
			return candidatePage, candidate, nil
		}
	}
	return nil, name, nil
}
