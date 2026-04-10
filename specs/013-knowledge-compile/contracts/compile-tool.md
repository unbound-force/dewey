# Contract: Compile MCP Tool & CLI Command

**Package**: `tools`
**File**: `tools/compile.go`

## MCP Tool: `compile`

### Input Type

```go
// CompileInput is the input for the dewey_compile MCP tool.
type CompileInput struct {
    // Incremental limits compilation to specific learning identities.
    // When empty, all learnings are compiled (full rebuild).
    Incremental []string `json:"incremental,omitempty" jsonschema:"Optional list of learning identities to compile incrementally (e.g., ['authentication-3']). When empty, performs full rebuild."`
}
```

### Tool Registration

```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "compile",
    Description: "Compile stored learnings into synthesized knowledge articles. Clusters learnings by topic, applies category-aware resolution (decisions: temporal merge, patterns: accumulate, gotchas: dedup), and produces markdown articles in .uf/dewey/compiled/. Returns clusters with synthesis prompts when no LLM is configured.",
}, compile.Compile)
```

## Compile Handler

```go
// Compile implements the dewey_compile MCP tool and CLI command.
// Dependencies are injected for testability.
type Compile struct {
    store       *store.Store
    embedder    embed.Embedder
    synthesizer llm.Synthesizer // nil = return prompts only
    vaultPath   string          // root path for .uf/dewey/compiled/
}

// NewCompile creates a new Compile tool handler.
func NewCompile(s *store.Store, e embed.Embedder, synth llm.Synthesizer, vaultPath string) *Compile
```

### Compile Method

```go
// Compile handles the dewey_compile MCP tool. Reads learnings from the
// store, clusters by tag + semantic similarity, and produces compiled
// articles.
//
// When synthesizer is available: writes compiled articles to
// .uf/dewey/compiled/ and indexes them in the store.
//
// When synthesizer is nil: returns clusters with synthesis prompts
// as structured output for the calling agent to perform synthesis.
//
// Full rebuild (incremental empty): deletes existing compiled articles
// and rebuilds from all learnings.
//
// Incremental (identities provided): merges specified learnings into
// existing compiled articles.
func (c *Compile) Compile(ctx context.Context, req *mcp.CallToolRequest, input types.CompileInput) (*mcp.CallToolResult, any, error)
```

### Return Value — With Synthesizer

```json
{
    "status": "compiled",
    "articles": [
        {
            "path": ".uf/dewey/compiled/authentication.md",
            "topic": "authentication",
            "sources": ["authentication-1", "authentication-2", "authentication-3"],
            "learning_count": 3
        }
    ],
    "index": ".uf/dewey/compiled/_index.md",
    "total_learnings": 10,
    "total_articles": 3,
    "message": "Compiled 10 learnings into 3 articles."
}
```

### Return Value — Without Synthesizer (Prompt Mode)

```json
{
    "status": "prompts_ready",
    "clusters": [
        {
            "topic": "authentication",
            "dominant_tag": "authentication",
            "learnings": [
                {
                    "identity": "authentication-1",
                    "category": "decision",
                    "created_at": "2026-03-15T10:00:00Z",
                    "content": "Use Option A for auth. Timeout 30s."
                },
                {
                    "identity": "authentication-2",
                    "category": "decision",
                    "created_at": "2026-03-20T14:00:00Z",
                    "content": "Switch to Option B due to rate limiting."
                }
            ],
            "synthesis_prompt": "You are compiling learnings into a knowledge article...",
            "category_instructions": "For 'decision' learnings: newer decisions supersede older ones..."
        }
    ],
    "total_learnings": 10,
    "total_clusters": 3,
    "message": "Clustered 10 learnings into 3 topics. Synthesis prompts ready for agent execution."
}
```

## Clustering Algorithm

```go
// clusterLearnings groups learnings by tag, then refines by semantic
// similarity. Returns one cluster per topic. Pure function — no side effects.
func clusterLearnings(learnings []LearningEntry, embeddings map[string][]float32) []Cluster

// LearningEntry represents a learning with its metadata for clustering.
type LearningEntry struct {
    Identity  string // e.g., "authentication-3"
    Tag       string
    Category  string
    CreatedAt time.Time
    Content   string
}

// Cluster represents a group of related learnings for compilation.
type Cluster struct {
    Topic       string          // Auto-generated topic name from dominant tag
    DominantTag string          // Most common tag in the cluster
    Learnings   []LearningEntry // Ordered by created_at ascending
}
```

### Clustering Steps

1. **Group by tag**: All learnings with the same `tag` → same initial group
2. **Semantic refinement**: Within each tag group, if any pair has cosine similarity < 0.3, split into sub-clusters (the tag covers multiple distinct topics)
3. **Cross-tag merge**: If two different-tag groups have average pairwise similarity > 0.8, merge them (different tags, same topic)
4. **Topic naming**: Use the dominant tag (most frequent) as the topic name

## Compiled Article Format

```markdown
---
tier: draft
compiled_at: 2026-04-10T14:30:00Z
sources:
  - authentication-1
  - authentication-2
  - authentication-3
topic: authentication
---

# Authentication

## Current State

[LLM-synthesized current truth. Contradictions resolved by recency.
Non-contradicted facts carried forward.]

## History

| Learning | Date | Category | Summary |
|----------|------|----------|---------|
| authentication-1 | 2026-03-15 | decision | Use Option A for auth. Timeout 30s. |
| authentication-2 | 2026-03-20 | decision | Switch to Option B due to rate limiting. |
| authentication-3 | 2026-04-01 | context | Increase timeout to 60s per user feedback. |
```

## CLI Command: `dewey compile`

```go
// newCompileCmd creates the `dewey compile` subcommand.
func newCompileCmd() *cobra.Command
```

**Usage**: `dewey compile [--incremental IDENTITY...]`

**Flags**:
- `--incremental, -i`: Learning identities to compile incrementally (repeatable)

**Behavior**: Opens the store, creates an Ollama synthesizer (from config), runs compilation, prints results.

## Invariants

1. Full rebuild MUST delete existing `.uf/dewey/compiled/` before writing new articles
2. Compiled articles MUST be indexed in the store with `source_id = "compiled"` and `tier = "draft"`
3. The `_index.md` file MUST list all compiled articles with links
4. Clustering MUST be deterministic given the same input learnings
5. Category-aware instructions MUST be included in synthesis prompts
6. When no learnings exist, produce an empty `_index.md` with "No learnings to compile" (no error)
7. Compilation failure MUST NOT corrupt existing compiled articles (atomic write)
8. Full rebuild MUST produce the same output as equivalent incremental compiles (FR-028)
