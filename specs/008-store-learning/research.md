# Research: Store Learning MCP Tool

**Branch**: `008-store-learning` | **Date**: 2026-04-03

## R1: Existing Store Pipeline (Page → Block → Embedding)

**Question**: How does Dewey currently persist content and generate embeddings?

**Finding**: The pipeline is well-established across three layers:

1. **Page insertion** (`store.InsertPage`): Creates a page record with `Name`, `OriginalName`, `SourceID`, `SourceDocID`, `Properties` (JSON string), `ContentHash`, timestamps. The `SourceID` field uses a convention of `"type-name"` (e.g., `"disk-local"`, `"github-gaze"`, `"web-docs"`).

2. **Block insertion** (`vault.PersistBlocks`): Recursively inserts blocks with UUID, page name, parent UUID, content, heading level, and position. Block UUIDs are generated deterministically from a seed (the document ID).

3. **Embedding generation** (`vault.GenerateEmbeddings`): Iterates blocks, prepares chunks via `embed.PrepareChunk()` (which prepends page name and heading path), calls `embedder.Embed()`, and persists via `store.InsertEmbedding()`. Failures are logged but don't block the pipeline (graceful degradation).

**Implication**: The learning tool can reuse all three layers directly. No new store methods are needed.

## R2: Source Type Filtering Mechanism

**Question**: How does `dewey_semantic_search_filtered` filter by source type?

**Finding**: The `SearchSimilarFiltered()` method in `store/embeddings.go` (line 204-209) filters by source type using a SQL `LIKE` prefix match on `pages.source_id`:

```sql
AND p.source_id LIKE ? ESCAPE '\'
```

With the argument `escapeLike(filters.SourceType) + "%"`. The `inferSourceType()` function (line 363-371) extracts the type by splitting on the first `-` character.

**Implication**: If we set `source_id = "learning"` (no dash suffix), `inferSourceType("learning")` returns `"learning"`. Filtering with `source_type: "learning"` will match `LIKE 'learning%'`. This works correctly. However, for consistency with the existing convention (`type-name`), we should use `source_id = "learning-agent"` or similar. But the spec says `source_id = "learning"` — let's use that since it's simpler and `inferSourceType` handles it correctly (returns the full string when no `-` is found).

**Decision**: Use `source_id = "learning"` as specified. The `inferSourceType()` function returns `"learning"` for this ID, and the `LIKE 'learning%'` filter matches it. Both `source_type: "learning"` and `source_id: "learning"` filters will work.

## R3: Tag Storage Mechanism

**Question**: How are tags stored so `dewey_semantic_search_filtered(has_tag: "gotcha")` works?

**Finding**: The `SearchSimilarFiltered()` method checks for tags in two places (line 221-228):
1. Page properties JSON: `p.properties LIKE '%tag%'`
2. Block content: `b.content LIKE '%#tag%'`

Page properties are stored as a JSON string in `pages.properties`. The spec says tags should be stored as page properties.

**Implication**: We need to store tags in the page's `Properties` field as a JSON object. The format `{"tags": "tag1, tag2, tag3"}` would be matched by the `LIKE` check. Alternatively, we can store individual tag properties. The simplest approach that works with the existing filter: store `{"tags": "006-unified-ignore, gotcha, vault-walker"}` as the properties JSON. The `has_tag: "gotcha"` filter does `LIKE '%gotcha%'` on properties, which matches.

**Decision**: Store tags as `{"tags": "tag1, tag2, ..."}` in the page properties JSON. This is the simplest approach that works with the existing `has_tag` filter without any changes to the search infrastructure.

## R4: MCP Tool Registration Pattern

**Question**: What pattern do existing tools follow for registration?

**Finding**: Two patterns exist in `server.go`:

1. **Typed handler** (preferred, used by most tools): `mcp.AddTool(srv, &mcp.Tool{Name, Description}, handler.Method)` where the method signature is `func(ctx, *mcp.CallToolRequest, InputType) (*mcp.CallToolResult, any, error)`. The MCP SDK auto-generates the JSON schema from the input struct's `json` and `jsonschema` tags.

2. **Raw handler** (used by `upsert_blocks` and `reload`): `srv.AddTool(&mcp.Tool{Name, Description, InputSchema}, rawHandler)` where the handler manually parses JSON. Used when the input has recursive types the schema generator can't handle.

**Implication**: The learning tool has a simple, flat input (two string fields). Use the typed handler pattern (pattern 1).

## R5: Tool Handler Struct Pattern

**Question**: How are tool handler structs organized?

**Finding**: Each tool category has its own struct in the `tools` package:
- `tools.Semantic` — holds `embedder` and `store`, created via `NewSemantic(embedder, store)`
- `tools.Write` — holds `backend`, created via `NewWrite(backend)`
- `tools.Navigate` — holds `backend`, created via `NewNavigate(backend)`

Each struct has methods matching the MCP handler signature. The struct is created in `newServer()` and its methods are registered as tool handlers.

**Implication**: Create a `tools.Learning` struct with `embedder` and `store` fields (same as `Semantic`), plus a `NewLearning()` constructor. Register via `registerLearningTools()` in `server.go`.

## R6: Block UUID Generation

**Question**: How are block UUIDs generated for programmatically created content?

**Finding**: `vault.ParseDocument(docID, content)` calls `parseMarkdownBlocks(docID, body)` which generates deterministic UUIDs from the document ID seed. For learnings, we need unique UUIDs. The simplest approach: use a UUID based on a combination of timestamp and content hash, or use the `crypto/rand` package for random UUIDs.

Looking at the existing code, `parseMarkdownBlocks` uses a deterministic seed. For learnings, since each is unique, we can either:
1. Use `ParseDocument()` with a unique docID (e.g., `"learning-" + timestamp`)
2. Generate UUIDs manually

**Decision**: Use `vault.ParseDocument()` with a unique document ID (`"learning-" + timestamp`). This reuses the existing parsing pipeline and generates deterministic UUIDs from the document ID, which is unique per learning.

## R7: Reindex Protection

**Question**: How do we ensure learnings survive `dewey reindex`?

**Finding**: The `reindex` command in `cli.go` deletes pages by source before re-indexing. It operates on configured sources (disk, GitHub, web). Since learnings use `source_id = "learning"` which is not a configured source in `sources.yaml`, the reindex command will not touch them.

Specifically, `DeletePagesBySource(sourceID)` is called per configured source. The learning source is never configured as a source, so its pages are never deleted during reindex.

**Implication**: No special protection needed. Learnings are naturally preserved because they use a source ID that is not part of the configured source pipeline.

## R8: Page Name Uniqueness

**Question**: How do we ensure learning page names don't collide with existing pages?

**Finding**: The `pages` table has a unique constraint on `name`. We need a naming scheme that avoids collisions with vault pages, GitHub pages, and other learnings.

**Decision**: Use the format `learning/{timestamp}` (e.g., `learning/1712150400000`). The `learning/` namespace prefix avoids collisions with all other content. The Unix millisecond timestamp ensures uniqueness across learnings (sub-millisecond collisions are astronomically unlikely for agent-generated learnings).
