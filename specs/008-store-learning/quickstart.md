# Quickstart: Store Learning MCP Tool

**Branch**: `008-store-learning` | **Date**: 2026-04-03

## Overview

This document provides the implementation blueprint for the `dewey_store_learning` MCP tool. It maps each file change to the spec requirements and provides pseudocode for the key implementation.

## File-by-File Implementation Guide

### 1. `types/tools.go` — Add Input Struct

**What**: Add `StoreLearningInput` struct alongside existing tool input types.

**Where**: After the `SemanticSearchFilteredInput` section (line ~231), before the Journal section.

```go
// --- Learning tool inputs ---

// StoreLearningInput is the input for the dewey_store_learning MCP tool.
type StoreLearningInput struct {
    Information string `json:"information" jsonschema:"The learning text to store. Required."`
    Tags        string `json:"tags,omitempty" jsonschema:"Optional comma-separated tags for filtering (e.g. 'gotcha, vault-walker, 006-unified-ignore')"`
}
```

**Maps to**: FR-001 (information required, tags optional)

---

### 2. `tools/learning.go` — Tool Implementation (NEW FILE)

**What**: Create the `Learning` struct and `StoreLearning` handler method.

**Dependencies**: `embed.Embedder` (for embedding generation), `store.Store` (for persistence)

**Handler pseudocode**:

```
func (l *Learning) StoreLearning(ctx, req, input StoreLearningInput):
    // 1. Validate input (FR-007)
    if input.Information is empty:
        return errorResult("information parameter is required and must not be empty")
    
    if l.store is nil:
        return errorResult("store_learning requires persistent storage. Configure --vault with a .dewey/ directory.")
    
    // 2. Generate unique page name and document ID
    timestamp = time.Now().UnixMilli()
    pageName = fmt.Sprintf("learning/%d", timestamp)
    docID = fmt.Sprintf("learning-%d", timestamp)
    
    // 3. Build properties JSON (FR-004 — tags for filtering)
    properties = "{}"
    if input.Tags != "":
        properties = json.Marshal(map{"tags": input.Tags})
    
    // 4. Insert page (FR-002, FR-003, FR-008)
    page = &store.Page{
        Name:         pageName,
        OriginalName: pageName,
        SourceID:     "learning",       // FR-003: distinguishes from other sources
        SourceDocID:  docID,
        Properties:   properties,
        ContentHash:  sha256(input.Information)[:16],
    }
    store.InsertPage(page)
    
    // 5. Parse content into blocks and persist (FR-002)
    _, blocks = vault.ParseDocument(docID, input.Information)
    vault.PersistBlocks(store, pageName, blocks, sql.NullString{}, 0)
    
    // 6. Generate embeddings if embedder available (FR-005, FR-009)
    embeddingMsg = ""
    if l.embedder != nil && l.embedder.Available():
        count = vault.GenerateEmbeddings(store, embedder, pageName, blocks, nil)
        // count logged but not returned
    else:
        embeddingMsg = "Note: Embeddings were not generated (Ollama unavailable). The learning is stored and searchable via keyword search. Semantic search will be available after embeddings are generated."
    
    // 7. Return success with UUID (FR-006)
    // Use the first block's UUID as the learning's identifier
    result = map{
        "uuid": blocks[0].UUID,
        "page": pageName,
        "message": "Learning stored successfully." + embeddingMsg,
    }
    return jsonTextResult(result)
```

**Key design decisions**:
- Uses `vault.ParseDocument()` to parse the learning text into blocks, reusing the existing parsing pipeline (DRY)
- Uses `vault.PersistBlocks()` and `vault.GenerateEmbeddings()` — the same exported functions used by the CLI indexing pipeline (DRY)
- Graceful degradation when embedder is unavailable: stores text without embeddings, returns informational message (FR-009)
- Returns first block UUID as the learning identifier (FR-006)

---

### 3. `tools/learning_test.go` — Tests (NEW FILE)

**Test cases** (maps to coverage strategy in plan.md):

| Test | Validates | Priority |
|------|-----------|----------|
| `TestStoreLearning_Basic` | Store learning, verify page/block created, verify UUID returned | P1 (FR-001, FR-002, FR-006) |
| `TestStoreLearning_Searchable` | Store learning, query via semantic search, verify it appears | P1 (FR-002, SC-001) |
| `TestStoreLearning_WithTags` | Store with tags, verify properties contain tags | P1 (FR-004) |
| `TestStoreLearning_EmptyInformation` | Empty input returns error | P1 (FR-007) |
| `TestStoreLearning_NilStore` | Nil store returns clear error | P1 (FR-007) |
| `TestStoreLearning_EmbedderUnavailable` | Store succeeds without embeddings, returns informational message | P1 (FR-009) |
| `TestStoreLearning_NilEmbedder` | Nil embedder stores text, returns informational message | P2 (FR-009) |
| `TestStoreLearning_FilterBySourceType` | Store learning, filter via `source_type: "learning"`, verify only learnings returned | P2 (FR-003, SC-004) |

**Test infrastructure**: Reuses `mockEmbedder` and `newTestStoreWithData()` from `semantic_test.go` (already in the same package).

---

### 4. `server.go` — Register the Tool

**What**: Add `registerLearningTools()` function and call it from `newServer()`.

**Where in `newServer()`**: After `registerSemanticTools()` (line ~83), before `registerHealthTool()`. The learning tool is a write operation (creates content), so it should be gated by `!readOnly` like the write and decision tools.

```go
// In newServer(), after registerSemanticTools:
if !readOnly {
    learning := tools.NewLearning(cfg.embedder, cfg.store)
    registerLearningTools(srv, learning)
}
```

```go
// registerLearningTools registers the store_learning MCP tool.
func registerLearningTools(srv *mcp.Server, learning *tools.Learning) {
    mcp.AddTool(srv, &mcp.Tool{
        Name:        "dewey_store_learning",
        Description: "Store a learning (insight, pattern, gotcha) with optional tags. The learning is persisted with embeddings and immediately searchable via dewey_semantic_search. Use to build semantic memory across sessions.",
    }, learning.StoreLearning)
}
```

**Maps to**: FR-010 (tool #41, registered alongside existing tools)

---

## Dependency Graph

```
types/tools.go (StoreLearningInput)
    ↓ used by
tools/learning.go (Learning struct + StoreLearning handler)
    ↓ used by
server.go (registerLearningTools)
```

No circular dependencies. The `tools/learning.go` file depends on:
- `types` package (input struct)
- `store` package (persistence)
- `embed` package (embedder interface)
- `vault` package (ParseDocument, PersistBlocks, GenerateEmbeddings)

All are existing dependencies already used by `tools/semantic.go`.

## Existing Infrastructure Reuse

| Component | Existing Location | Reused For |
|-----------|------------------|------------|
| `store.InsertPage()` | `store/store.go` | Persisting learning page |
| `vault.ParseDocument()` | `vault/parse_export.go` | Parsing learning text into blocks |
| `vault.PersistBlocks()` | `vault/parse_export.go` | Persisting blocks to store |
| `vault.GenerateEmbeddings()` | `vault/parse_export.go` | Creating embeddings for learning |
| `store.SearchSimilar()` | `store/embeddings.go` | Searching learnings (no changes) |
| `store.SearchSimilarFiltered()` | `store/embeddings.go` | Filtering by source_type "learning" (no changes) |
| `inferSourceType()` | `store/embeddings.go` | Extracting "learning" from source_id (no changes) |
| `mockEmbedder` | `tools/semantic_test.go` | Test double for embedder |
| `errorResult()` / `jsonTextResult()` | `tools/helpers.go` | MCP response formatting |

**Zero new dependencies** — everything uses existing packages.
