# MCP Tool Contracts: Dewey Semantic Search

**Branch**: `001-core-implementation` | **Date**: 2026-03-22

Three new MCP tools are added alongside the existing 37 tools. All existing tools remain unchanged.

## dewey_semantic_search

**Purpose**: Find documents semantically similar to a natural language query, ranked by cosine similarity.

**Input Schema**:

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Natural language search query"
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of results. Default: 10"
    },
    "threshold": {
      "type": "number",
      "description": "Minimum similarity score (0.0-1.0). Default: 0.3"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}
```

**Output**: Array of search results, each containing:

```json
{
  "document_id": "string (block UUID)",
  "page": "string (page name)",
  "content": "string (block content)",
  "similarity": 0.87,
  "source": "string (source type: disk, github, web)",
  "source_id": "string (source identifier)",
  "origin_url": "string (original URL for external sources, null for disk)",
  "indexed_at": "string (ISO 8601 timestamp, corresponds to page updated_at)"
}
```

**Error cases**:
- Embedding model unavailable: Returns error text "Semantic search unavailable: embedding model not loaded. Ensure Ollama is running with the configured model."
- Empty index: Returns empty results array with no error.

---

## dewey_similar

**Purpose**: Given a document (by page name or block UUID), find the most similar documents in the index.

**Input Schema**:

```json
{
  "type": "object",
  "properties": {
    "page": {
      "type": "string",
      "description": "Page name to find similar documents for"
    },
    "uuid": {
      "type": "string",
      "description": "Block UUID to find similar blocks for. Takes precedence over page."
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of results. Default: 10"
    }
  },
  "additionalProperties": false
}
```

**Constraint**: At least one of `page` or `uuid` MUST be provided. Validated at runtime (JSON Schema cannot express "at least one of" cleanly). The tool handler MUST return an error if neither is provided.

**Output**: Same format as `dewey_semantic_search` results.

**Error cases**:
- Neither `page` nor `uuid` provided: Returns error text "At least one of 'page' or 'uuid' must be provided."
- Page/block not found: Returns error text "Page/block not found: {identifier}"
- No embedding for the specified document: Returns error text "No embedding found for {identifier}. Run `dewey index` to generate embeddings."
- No embeddings exist in the index: Returns error text "No embeddings in index. Run `dewey index` to generate embeddings."

---

## dewey_semantic_search_filtered

**Purpose**: Semantic search constrained by metadata filters (source type, repository, properties).

**Input Schema**:

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Natural language search query"
    },
    "source_type": {
      "type": "string",
      "description": "Filter by source type: disk, github, web"
    },
    "source_id": {
      "type": "string",
      "description": "Filter by specific source identifier (e.g., github-gaze)"
    },
    "has_property": {
      "type": "string",
      "description": "Filter to pages with this frontmatter property key"
    },
    "has_tag": {
      "type": "string",
      "description": "Filter to pages with this tag"
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of results. Default: 10"
    },
    "threshold": {
      "type": "number",
      "description": "Minimum similarity score (0.0-1.0). Default: 0.3"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}
```

**Output**: Same format as `dewey_semantic_search` results, with results filtered to match all specified criteria.

**Error cases**: Same as `dewey_semantic_search`.

---

## Updated health Tool

The existing `health` tool is extended (not replaced) to include Dewey-specific information.

**Additional output fields** (merged into existing health response):

```json
{
  "status": "ok",
  "version": "0.2.0",
  "backend": "obsidian",
  "readOnly": false,
  "pageCount": 188,
  "dewey": {
    "indexPath": ".dewey/",
    "persistent": true,
    "embeddingModel": "granite-embedding:30m",
    "embeddingAvailable": true,
    "embeddingCount": 1523,
    "embeddingCoverage": 0.95,
    "sources": [
      {
        "id": "disk-local",
        "type": "disk",
        "status": "active",
        "pageCount": 188,
        "lastFetched": "2026-03-22T14:30:00Z"
      },
      {
        "id": "github-gaze",
        "type": "github",
        "status": "active",
        "pageCount": 47,
        "lastFetched": "2026-03-22T10:00:00Z"
      }
    ]
  }
}
```
