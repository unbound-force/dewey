# Contract: Promote MCP Tool & CLI Command

**Package**: `tools`
**File**: `tools/promote.go`

## MCP Tool: `promote`

### Input Type

```go
// PromoteInput is the input for the dewey_promote MCP tool.
type PromoteInput struct {
    // Page is the page name to promote from draft to validated.
    Page string `json:"page" jsonschema:"Page name to promote from draft to validated tier (e.g., 'learning/authentication-3')."`
}
```

### Tool Registration

```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "promote",
    Description: "Promote a draft page to validated tier. Changes the trust level from 'draft' (unreviewed agent content) to 'validated' (human-reviewed). Only draft pages can be promoted.",
}, promote.Promote)
```

## Promote Handler

```go
// Promote implements the dewey_promote MCP tool and CLI command.
type Promote struct {
    store *store.Store
}

// NewPromote creates a new Promote tool handler.
func NewPromote(s *store.Store) *Promote
```

### Promote Method

```go
// Promote handles the dewey_promote MCP tool. Changes a page's trust
// tier from "draft" to "validated". Only pages with tier "draft" can
// be promoted. Returns an error if the page doesn't exist, is not
// draft tier, or the store is unavailable.
func (p *Promote) Promote(ctx context.Context, req *mcp.CallToolRequest, input types.PromoteInput) (*mcp.CallToolResult, any, error)
```

### Return Value — Success

```json
{
    "page": "learning/authentication-3",
    "previous_tier": "draft",
    "new_tier": "validated",
    "message": "Page 'learning/authentication-3' promoted to validated tier."
}
```

### Return Value — Error Cases

Page not found:
```json
{
    "isError": true,
    "message": "Page 'learning/nonexistent-1' not found."
}
```

Page not draft:
```json
{
    "isError": true,
    "message": "Page 'specs/001-core' has tier 'authored' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'."
}
```

Store unavailable:
```json
{
    "isError": true,
    "message": "promote requires persistent storage. Configure --vault with a .uf/dewey/ directory."
}
```

## New Store Method

```go
// UpdatePageTier updates a page's tier column. Returns an error if the
// page doesn't exist or the update fails.
func (s *Store) UpdatePageTier(name, tier string) error
```

```sql
UPDATE pages SET tier = ?, updated_at = ? WHERE name = ?
```

## CLI Command: `dewey promote`

```go
// newPromoteCmd creates the `dewey promote` subcommand.
func newPromoteCmd() *cobra.Command
```

**Usage**: `dewey promote PAGE_NAME`

**Arguments**:
- `PAGE_NAME`: The page name to promote (required, positional)

**Output**: Success message or error.

## Invariants

1. Only `draft` → `validated` transition is supported
2. `authored` pages MUST NOT be promotable (return error)
3. `validated` pages MUST NOT be re-promoted (return error: already validated)
4. The `updated_at` timestamp MUST be set to current time on promotion
5. Promote MUST NOT modify page content, blocks, links, or embeddings
6. Promote MUST work on both learning pages and compiled article pages
