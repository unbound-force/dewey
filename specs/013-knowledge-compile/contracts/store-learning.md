# Contract: Modified `store_learning` MCP Tool

**Package**: `tools`
**File**: `tools/learning.go`

## Modified Input Type

```go
// StoreLearningInput is the input for the dewey_store_learning MCP tool.
// BREAKING CHANGE: `tags` (plural, optional) replaced by `tag` (singular, required).
type StoreLearningInput struct {
    Information string `json:"information" jsonschema:"The learning text to store. Required."`
    Tag         string `json:"tag" jsonschema:"Topic tag for the learning (e.g., 'authentication', 'deployment'). Required. Used for {tag}-{sequence} identity and topic clustering."`
    Category    string `json:"category,omitempty" jsonschema:"Learning category: decision, pattern, gotcha, context, or reference. Optional. Affects compilation merge strategy."`
    // Deprecated: Use Tag instead. If provided and Tag is empty, the first
    // tag from the comma-separated list is used for backward compatibility.
    Tags        string `json:"tags,omitempty" jsonschema:"DEPRECATED: Use 'tag' instead. If provided and 'tag' is empty, first tag is used."`
}
```

## Modified Function: StoreLearning

```go
// StoreLearning handles the dewey_store_learning MCP tool. Persists a
// learning into the knowledge graph with a required topic tag and optional
// category. The learning receives a {tag}-{sequence} identity (e.g.,
// "authentication-3") and is stored with tier "draft".
//
// Returns a JSON result with the learning's identity, page name, and
// status message. Returns an MCP error result if input is invalid or
// the store is unavailable.
//
// BREAKING CHANGE from spec 008: The `tags` parameter (plural, optional,
// comma-separated) is replaced by `tag` (singular, required). For backward
// compatibility, if `tags` is provided but `tag` is not, the first tag
// from the comma-separated list is used. If neither is provided, defaults
// to "general".
func (l *Learning) StoreLearning(ctx context.Context, req *mcp.CallToolRequest, input types.StoreLearningInput) (*mcp.CallToolResult, any, error)
```

### Behavior Changes

1. **Tag resolution**: `tag` field takes priority. If empty, fall back to first value from `tags` (comma-separated). If both empty, default to `"general"`.

2. **Tag normalization**: Lowercase, trim whitespace, replace spaces with hyphens, strip non-alphanumeric characters (except hyphens). Example: `"My Tag Name"` → `"my-tag-name"`.

3. **Sequence determination**: Query existing learning pages with the same tag prefix to determine the next sequence number.

4. **Page naming**: `learning/{normalized-tag}-{sequence}` (e.g., `learning/authentication-3`).

5. **Properties JSON**: Includes `tag`, `category` (if provided), and `created_at` (ISO 8601).

6. **Page fields**:
   - `Tier`: `"draft"` (always for learnings)
   - `Category`: from input, or empty string
   - `SourceID`: `"learning"` (unchanged)

### Return Value

```json
{
    "identity": "authentication-3",
    "page": "learning/authentication-3",
    "tag": "authentication",
    "category": "decision",
    "created_at": "2026-04-10T14:30:00Z",
    "message": "Learning stored successfully."
}
```

### New Store Method: NextLearningSequence

```go
// NextLearningSequence returns the next sequence number for a learning
// with the given tag. Counts existing learning pages with the same tag
// prefix and returns count + 1.
func (s *Store) NextLearningSequence(tag string) (int, error)
```

```sql
SELECT COUNT(*) FROM pages 
WHERE source_id = 'learning' 
AND name LIKE 'learning/' || ? || '-%'
```

## Category Validation

Valid categories: `decision`, `pattern`, `gotcha`, `context`, `reference`.

If an invalid category is provided, return an MCP error result:
```
"invalid category 'foo'. Valid categories: decision, pattern, gotcha, context, reference"
```

If no category is provided, the field is stored as empty string (not NULL). During compilation, learnings without a category are treated as `context`.

## Invariants

1. Every stored learning MUST have a `tag` (never empty after resolution)
2. Every stored learning MUST have a unique `{tag}-{sequence}` identity
3. The sequence MUST be monotonically increasing within a tag namespace
4. `tier` MUST be `"draft"` for all learnings
5. `created_at` in properties MUST be ISO 8601 format
6. The learning MUST be immediately searchable via keyword search after storage
7. Backward compatibility: old `tags` parameter MUST still work (first tag extracted)
