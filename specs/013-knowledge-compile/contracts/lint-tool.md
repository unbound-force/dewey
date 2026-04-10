# Contract: Lint MCP Tool & CLI Command

**Package**: `tools`
**File**: `tools/lint.go`

## MCP Tool: `lint`

### Input Type

```go
// LintInput is the input for the dewey_lint MCP tool.
type LintInput struct {
    // Fix enables auto-repair of mechanical issues (e.g., regenerate
    // missing embeddings). Does not fix semantic issues.
    Fix bool `json:"fix,omitempty" jsonschema:"Auto-repair mechanical issues (missing embeddings). Default: false"`
}
```

### Tool Registration

```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "lint",
    Description: "Scan the knowledge index for quality issues: stale decisions (>30 days), uncompiled learnings, embedding gaps, and semantic contradictions. Use --fix to auto-repair mechanical issues.",
}, lint.Lint)
```

## Lint Handler

```go
// Lint implements the dewey_lint MCP tool and CLI command.
type Lint struct {
    store    *store.Store
    embedder embed.Embedder
}

// NewLint creates a new Lint tool handler.
func NewLint(s *store.Store, e embed.Embedder) *Lint
```

### Lint Method

```go
// Lint handles the dewey_lint MCP tool. Scans the index for knowledge
// quality issues and optionally auto-repairs mechanical problems.
//
// Checks performed:
// 1. Stale decisions: learnings with category "decision" older than 30 days
//    and not validated
// 2. Uncompiled learnings: learnings not referenced by any compiled article
// 3. Embedding gaps: pages with blocks but no embeddings
// 4. Semantic contradictions: learning pairs with high similarity but
//    potentially different conclusions (same tag, different content)
//
// When fix=true, auto-repairs embedding gaps by regenerating embeddings.
// Semantic issues (contradictions, staleness) require human/agent judgment.
func (l *Lint) Lint(ctx context.Context, req *mcp.CallToolRequest, input types.LintInput) (*mcp.CallToolResult, any, error)
```

### Return Value

```json
{
    "status": "issues_found",
    "summary": {
        "stale_decisions": 3,
        "uncompiled_learnings": 12,
        "embedding_gaps": 5,
        "contradictions": 2,
        "total_issues": 22
    },
    "findings": [
        {
            "type": "stale_decision",
            "severity": "warning",
            "identity": "auth-config-1",
            "description": "Decision learning 'auth-config-1' is 45 days old and not validated.",
            "remediation": "Review and either validate with `dewey promote auth-config-1` or store an updated decision."
        },
        {
            "type": "uncompiled",
            "severity": "info",
            "identity": "deployment-2",
            "description": "Learning 'deployment-2' has not been compiled into any article.",
            "remediation": "Run `dewey compile` to compile all learnings."
        },
        {
            "type": "embedding_gap",
            "severity": "warning",
            "page": "learning/cache-strategy-1",
            "description": "Page 'learning/cache-strategy-1' has 3 blocks but no embeddings.",
            "remediation": "Run `dewey lint --fix` to regenerate embeddings, or `dewey index` to re-index."
        },
        {
            "type": "contradiction",
            "severity": "warning",
            "identities": ["auth-timeout-1", "auth-timeout-3"],
            "similarity": 0.92,
            "description": "Learnings 'auth-timeout-1' and 'auth-timeout-3' are highly similar (0.92) but may contain contradicting information.",
            "remediation": "Run `dewey compile` to resolve contradictions via temporal merge, or review manually."
        }
    ],
    "fixed": {
        "embedding_gaps": 5
    },
    "message": "Found 22 issues. Fixed 5 embedding gaps."
}
```

### Return Value — No Issues

```json
{
    "status": "clean",
    "summary": {
        "stale_decisions": 0,
        "uncompiled_learnings": 0,
        "embedding_gaps": 0,
        "contradictions": 0,
        "total_issues": 0
    },
    "findings": [],
    "message": "Knowledge index is clean. No issues found."
}
```

## Check Functions

```go
// checkStaleDecisions finds decision learnings older than the staleness
// threshold (30 days) that have not been validated.
func (l *Lint) checkStaleDecisions() ([]Finding, error)

// checkUncompiledLearnings finds learnings not referenced by any
// compiled article's sources list.
func (l *Lint) checkUncompiledLearnings() ([]Finding, error)

// checkEmbeddingGaps finds pages with blocks but no embeddings.
func (l *Lint) checkEmbeddingGaps() ([]Finding, error)

// checkContradictions finds learning pairs with high semantic similarity
// (>0.8) within the same tag namespace, suggesting potential contradictions.
func (l *Lint) checkContradictions() ([]Finding, error)

// fixEmbeddingGaps regenerates embeddings for pages that have blocks
// but no embeddings. Requires an available embedder.
func (l *Lint) fixEmbeddingGaps(ctx context.Context, gaps []Finding) (int, error)
```

## Finding Type

```go
// Finding represents a single lint issue.
type Finding struct {
    Type        string   `json:"type"`        // stale_decision, uncompiled, embedding_gap, contradiction
    Severity    string   `json:"severity"`    // info, warning, error
    Identity    string   `json:"identity,omitempty"`
    Identities  []string `json:"identities,omitempty"` // for contradictions (pair)
    Page        string   `json:"page,omitempty"`
    Similarity  float64  `json:"similarity,omitempty"` // for contradictions
    Description string   `json:"description"`
    Remediation string   `json:"remediation"`
}
```

## CLI Command: `dewey lint`

```go
// newLintCmd creates the `dewey lint` subcommand.
func newLintCmd() *cobra.Command
```

**Usage**: `dewey lint [--fix]`

**Flags**:
- `--fix`: Auto-repair mechanical issues (embedding gaps)

**Output**: Structured report printed to stdout. Exit code 0 if clean, 1 if issues found.

## New Store Methods

```go
// ListLearningPages returns all pages with source_id = 'learning',
// ordered by name. Used by lint to enumerate all learnings.
func (s *Store) ListLearningPages() ([]*Page, error)

// PagesWithoutEmbeddings returns pages that have blocks but no
// embeddings. Used by lint to detect embedding gaps.
func (s *Store) PagesWithoutEmbeddings() ([]*Page, error)
```

## Invariants

1. Lint MUST NOT modify any data unless `fix=true`
2. `--fix` MUST only repair mechanical issues (embeddings), never semantic issues
3. Stale decision threshold MUST be 30 days (hardcoded, not configurable in v1)
4. Contradiction detection MUST use cosine similarity > 0.8 threshold
5. Each finding MUST include an actionable remediation suggestion
6. Lint MUST work without an embedder (skip contradiction check, report embedder unavailable)
7. Lint MUST work without a store (return error: "lint requires persistent storage")
