# Contract: MCP Tools

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21

## New Tool: `curate`

### Registration

```go
// In server.go, within the !readOnly block:
curateTool := tools.NewCurate(cfg.store, cfg.embedder, nil, cfg.vaultPath, cfg.indexMutex)
toolCount += registerCurateTools(srv, curateTool)
```

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "store": {
      "type": "string",
      "description": "Name of the knowledge store to curate. If omitted, curates all configured stores."
    },
    "incremental": {
      "type": "boolean",
      "description": "Only process new/changed documents since last curation. Default: true"
    }
  },
  "additionalProperties": false
}
```

### Input Type

```go
// types/curate.go (or in types/types.go)
type CurateInput struct {
    Store       string `json:"store"`
    Incremental *bool  `json:"incremental,omitempty"`
}
```

### Output (with synthesizer — CLI mode)

```json
{
  "status": "complete",
  "results": [
    {
      "store_name": "team-decisions",
      "files_created": 5,
      "docs_processed": 12,
      "docs_skipped": 3,
      "error": ""
    }
  ],
  "message": "Curated 5 knowledge files from 12 documents in 1 store."
}
```

### Output (without synthesizer — MCP mode)

```json
{
  "status": "prompt_ready",
  "stores": [
    {
      "store_name": "team-decisions",
      "docs_to_process": 12,
      "extraction_prompt": "You are a knowledge curator. Analyze the following..."
    }
  ],
  "message": "Extraction prompts ready for 1 store. Call with synthesized results to complete curation."
}
```

### Error Cases

| Condition | Response |
|-----------|----------|
| No persistent store | `"curate requires persistent storage. Configure --vault with a .uf/dewey/ directory."` |
| No knowledge-stores.yaml | `"No knowledge stores configured. Create .uf/dewey/knowledge-stores.yaml or run 'dewey init'."` |
| Named store not found | `"Knowledge store '{name}' not found in configuration."` |
| Indexing in progress | `"Curation cannot run while indexing is in progress. Try again later."` |
| LLM unavailable (CLI) | `"LLM unavailable. Ensure Ollama is running with a generation model."` |

## Modified Tool: `store_learning`

### Behavioral Change

The `store_learning` tool now dual-writes to both SQLite and a markdown file. The output schema is unchanged — the response includes a new `file_path` field:

```json
{
  "uuid": "abc123",
  "identity": "authentication-3",
  "page": "learning/authentication-3",
  "tag": "authentication",
  "category": "decision",
  "created_at": "2026-04-21T10:30:00Z",
  "file_path": ".uf/dewey/learnings/authentication-3.md",
  "message": "Learning stored successfully."
}
```

### Backward Compatibility

- The `file_path` field is additive — existing consumers ignore unknown fields.
- If the file write fails, the learning is still stored in SQLite. A warning is logged but the tool returns success.
- The constructor signature changes: `NewLearning(e, s)` → `NewLearning(e, s, vaultPath)`. This is an internal API change, not an MCP contract change.

## Modified Tool: `lint`

### New Findings

The lint tool adds two new check types:

```json
{
  "type": "knowledge_quality",
  "severity": "info",
  "description": "Knowledge store 'team-decisions': 3 low-confidence facts, 1 incongruent flag",
  "remediation": "Review low-confidence facts and resolve contradictions in .uf/dewey/knowledge/team-decisions/"
}
```

```json
{
  "type": "stale_knowledge",
  "severity": "warning",
  "description": "Knowledge store 'team-decisions' has 5 unprocessed documents (sources updated since last curation)",
  "remediation": "Run 'dewey curate --store team-decisions' to process new content."
}
```

### Summary Extension

The lint summary gains new fields:

```json
{
  "summary": {
    "stale_decisions": 0,
    "uncompiled_learnings": 2,
    "embedding_gaps": 0,
    "contradictions": 0,
    "knowledge_quality_issues": 4,
    "stale_knowledge_stores": 1,
    "total_issues": 7
  }
}
```

## Modified Tool: `semantic_search_filtered`

### No Schema Change

The `tier` parameter already accepts any string. Passing `tier: "curated"` works immediately. No input schema change needed.

### Behavioral Verification

Curated pages have `tier = "curated"` in the `pages` table. The `filterResultsByTier()` function in `tools/semantic.go` does string equality comparison — works for `curated` without code changes.

## Modified Tool: `health`

### New Field

The health response gains a `knowledgeStores` field in the `dewey` section:

```json
{
  "dewey": {
    "persistent": true,
    "knowledgeStores": {
      "configured": 2,
      "totalCuratedFiles": 45,
      "lastCuration": "2026-04-21T10:30:00Z"
    }
  }
}
```
