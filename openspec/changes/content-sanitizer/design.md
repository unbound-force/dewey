## Context

Dewey's indexing pipeline processes content from four source types (`disk`, `github`, `web`, `code`) through a shared path:

```
source.List() → Document.Content
                      ↓
              vault.ParseDocument()     ← parses Markdown into block tree
                      ↓
              vault.PersistBlocks()     ← stores blocks in SQLite
                      ↓
              vault.PersistLinks()      ← extracts and persists wikilinks
                      ↓
              vault.GenerateEmbeddings() ← creates vector embeddings
```

Content enters this pipeline as raw strings and flows through every stage without content-level inspection. While SQL injection is prevented via parameterized queries (FR-028), there is no defense against prompt injection patterns or data poisoning in the content itself. `ParseDocument` splits content structurally, `PersistBlocks` stores it as-is, and MCP tools return block content verbatim to agents.

This design introduces a `sanitize` package that inspects content at the pipeline chokepoint, producing structured findings that annotate documents without blocking indexing by default. Per the proposal's constitution alignment, the design uses pure Go functions (Composability First, Testability), produces machine-parseable findings (Observable Quality), and communicates via existing MCP tool responses (Autonomous Collaboration).

## Goals / Non-Goals

### Goals
- Detect common prompt injection patterns in content before it enters the knowledge graph
- Alert on content hash changes between index cycles as a data poisoning signal
- Validate Markdown structural integrity (invisible characters, anomalous patterns)
- Flag content size outliers that may indicate injected payload
- Extend the trust tier system with an `untrusted` value for source-level trust assignment
- Surface all findings via `dewey doctor` and `dewey lint`
- Operate in warn-by-default mode with opt-in blocking
- Achieve >= 80% line coverage for the `sanitize` package

### Non-Goals
- LLM-based content evaluation (violates Composability First — requires external service)
- Cross-modal verification (screenshot comparison — Dewey does not process images)
- Real-time scanning of MCP tool output (sanitization happens at ingest, not query time)
- Full trigram-based content drift comparison (requires storing raw content; deferred to future iteration — see D4)
- Scanning content from `disk` or `code` sources by default (user-authored/controlled content is trusted)
- Content rewriting or censoring (the sanitizer reports findings, never modifies content)

## Decisions

### D1: Pipeline Insertion Point

**Decision**: Insert `sanitize.Scan()` between `source.Document` production and `vault.ParseDocument()` in both `cli.go:indexDocuments()` and `tools/indexing.go:indexDocuments()`. After `ParseDocument()` returns frontmatter properties, merge `sanitize_findings` into the properties map before persisting.

**Rationale**: This is the single chokepoint where all source content passes. Inserting here means every source type (current and future) benefits from sanitization without per-source implementation. Findings are merged into the parsed properties map (not stored separately in `Document.Properties`) to avoid the silent overwrite issue where frontmatter properties would discard document-level properties during persistence.

### D2: Warn, Don't Block

**Decision**: The sanitizer logs findings at WARN level and merges them into page properties but does not reject content by default. Blocking is opt-in via `sanitize_mode: strict` in source config.

**Rationale**: A knowledge graph indexing security documentation, penetration testing guides, or AI safety research will contain legitimate content with the exact phrases used in prompt injection attacks. A page explaining "how to defend against 'ignore previous instructions' attacks" would be falsely blocked. Warn-by-default preserves index completeness while making threats visible. Users who index only trusted documentation can enable strict mode.

### D3: Pure Regex Pattern Scanning (No LLM)

**Decision**: Adversarial keyword detection uses compiled regex patterns, not LLM-based evaluation.

**Rationale**: Adding an LLM call to evaluate every indexed page would violate the constitution's Composability First principle (mandatory cloud dependency), add significant latency to indexing, and cost money per page. Regex patterns are fast, deterministic, testable, and operate locally. The pattern database is extensible — new patterns can be added without architectural changes. Each pattern includes a `PatternVersion` field (monotonic integer) to enable staleness detection when patterns are updated in future releases.

### D4: Hash-Only Content Drift Detection

**Decision**: Content drift detection uses the existing `ContentHash` (SHA-256) to detect binary changed/unchanged state. When a page's hash differs from the previously indexed version, a finding is produced flagging the page as changed. No trigram similarity comparison is performed.

**Rationale**: Full trigram-based drift comparison (as originally proposed) requires access to the previous version's raw content. The store does not persist raw `Document.Content` — it stores parsed blocks. Adding a content snapshot mechanism would require either a schema change or a file cache, both of which add complexity disproportionate to the value for this iteration. Hash-only detection still provides the core signal: "this page changed since last index." Future iterations can add trigram comparison once a content snapshot mechanism exists.

**Alternative considered**: Storing raw content in a file cache (`basePath/cache/<sourceID>/<hash>.md`) for trigram comparison. Rejected for this iteration because it adds disk usage proportional to total indexed content size and introduces a new persistence path with its own consistency concerns.

### D5: Statistical Anomaly Detection via ScanInput

**Decision**: Size anomaly detection receives per-source statistics via an optional `SourceStats *SourceStats` field in the `ScanInput` struct. The pipeline computes stats from all documents in the current batch before per-document scanning. If `SourceStats` is nil, anomaly detection is skipped.

**Rationale**: This keeps `Scan()` as a pure function — the caller (pipeline) controls the stats lifecycle. Stats are computed from the current batch of documents, which provides meaningful baselines for sources with 5+ pages. During incremental indexing with fewer than 5 changed documents, anomaly detection is naturally skipped (insufficient sample size).

### D6: Formal Trust Tier Extension

**Decision**: Extend the trust tier system from 4 to 5 values: `authored > curated > validated > draft > untrusted`. Add an optional `trust_tier` field to source configuration in `sources.yaml`. Default: `authored` (backward compatible). The extension requires updating `types/tools.go` (`SemanticSearchFilteredInput` schema description), `tools/semantic.go` (tier ordering comment), AGENTS.md (Trust Tiers table), and cross-referencing specs 013-knowledge-compile and 015-curated-knowledge-stores.

**Rationale**: A separate `source_trust` property was considered but rejected because it fragments the trust model — agents would need to check both `tier` and `source_trust` to assess content quality. The tier system is the established mechanism for trust-based filtering via `semantic_search_filtered`. Adding `untrusted` below `draft` maintains a single trust axis. The `promote` tool only allows `draft → validated` transitions; `untrusted` pages cannot be promoted (they must be re-indexed with a different `trust_tier` source config). The `compile` tool operates on `draft` tier learnings and will not compile `untrusted` content.

### D7: Findings Merged into Page Properties

**Decision**: After `vault.ParseDocument()` extracts frontmatter properties, merge `sanitize_findings` into the properties map before passing to `store.InsertPage()`. Findings are queryable via a new `store.GetPagesWithProperty()` method using SQLite `json_extract()`.

**Rationale**: `Document.Properties` are overwritten by frontmatter properties during indexing (`cli.go:1018-1025`). Storing findings in `Document.Properties` would cause silent data loss for any document with frontmatter. Merging findings into the parsed properties map after `ParseDocument()` ensures they survive persistence. SQLite's built-in `json_extract()` function enables efficient property-based queries without schema changes.

### D8: Configurable Pattern Database with Versioning

**Decision**: Injection patterns are defined as a `[]PatternRule` slice in `sanitize/patterns.go`, each with a compiled regex, severity, description, and `Version int` field. The `ScanResult` includes a `PatternVersion int` reflecting the current pattern database version. Users cannot add custom patterns in this iteration.

**Rationale**: Centralizing patterns in a single file makes the detection rules auditable and testable. Pattern versioning enables `dewey lint` to detect stale findings from older pattern versions and suggest re-scanning via `dewey reindex`. Each pattern has explicit documentation of what it detects and why.

### D9: Unified Sanitization Mode (No Separate Boolean)

**Decision**: All source types use the `sanitize_mode` field (`warn`/`strict`/`off`). Disk and code sources default to `sanitize_mode: off`. External sources (`web`, `github`) default to `sanitize_mode: warn`. There is no separate `sanitize: true` boolean field.

**Rationale**: Having both a boolean toggle and a mode enum creates ambiguous interactions (what if `sanitize: true` AND `sanitize_mode: off`?). A unified field eliminates this ambiguity. Disk and code sources default to `off` because they represent user-controlled content — the user authored the vault content and the source code. Scanning local content for prompt injection would generate noise without security benefit.

### D10: Scan() Follows AP-001 Options/Result Pattern

**Decision**: `Scan()` accepts a `ScanInput` struct and `ScanConfig`, returning `(*ScanResult, error)`. The `ScanInput` struct contains: `Content string`, `SourceID string`, `DocumentID string`, `SourceType string`, `PreviousHash string`, `CurrentHash string`, `SourceStats *SourceStats`.

**Rationale**: Per convention pack AP-001, functions should accept an Options struct for configuration and return a Result struct. This future-proofs the API for additional scan parameters without breaking callers.

## Coverage Strategy

The `sanitize` package MUST achieve >= 80% line coverage. All exported functions are contract-critical:
- `Scan(input ScanInput, config ScanConfig) (*ScanResult, error)`
- `ScanInjectionPatterns(content string, patterns []PatternRule) []Finding`
- `ContentDrift(previousHash, currentHash string) *Finding`
- `ValidateStructure(content string, invisibleThreshold int) []Finding`
- `SizeAnomaly(contentLen int, stats SourceStats) (bool, *Finding)`
- `ComputeStats(lengths []int) SourceStats`

Pipeline integration is tested via integration tests using in-memory SQLite (`store.New(":memory:")`), verifying that findings survive the full `indexDocuments()` → `store` → `GetPagesWithProperty()` path.

## Risks / Trade-offs

### R1: False Positives on Security Documentation (Accepted)

Pages about prompt injection defense, AI safety, or penetration testing will trigger adversarial keyword patterns. Mitigation: warn-by-default mode ensures these pages are still indexed. The `dewey lint` output distinguishes between finding counts, letting users assess whether warnings are legitimate threats or false positives from expected content.

### R2: Hash-Only Drift Detection is Coarse (Accepted)

Hash-based drift detection only provides a binary changed/unchanged signal, not a similarity score. A minor typo fix and a complete content replacement both show as "changed." Mitigation: the hash change finding is informational (`severity: info`), not blocking. Future iterations can add trigram comparison once a content snapshot mechanism exists.

### R3: Size Anomaly Requires Sufficient Sample Size (Accepted)

The statistical anomaly detector needs at least 5 pages per source to compute meaningful standard deviations. Sources with fewer pages skip anomaly detection. This is documented in the function contract.

### R4: No Protection at Query Time (Accepted)

The sanitizer runs at ingest time, not when MCP tools return results. A page indexed before the sanitizer was deployed will not have findings. Mitigation: `dewey reindex` re-processes all content through the sanitizer. A future enhancement could add query-time scanning, but the ingest-time approach covers the primary threat vector.

### R5: Pattern Evasion (Accepted)

Sophisticated attackers can evade regex patterns using unicode substitutions, whitespace manipulation, or encoding tricks. The structure validator catches some of these (zero-width characters, invisible Unicode), but determined evasion is possible. This is a fundamental limitation of pattern-based detection. The sanitizer raises the bar for casual attacks without claiming to stop all threats.

### R6: Trust Tier Extension Scope (Accepted)

Adding `untrusted` as a 5th tier requires updating the `SemanticSearchFilteredInput` schema description, AGENTS.md, and cross-referencing specs 013/015. This is larger than a typical OpenSpec change but remains within scope because the tier system extension is narrowly defined (one new value, no new tables, no workflow changes) and the implementation touches only documentation strings and one config validation function.

### R7: Duplicated Pipeline Integration (Accepted)

The sanitization logic must be inserted in two `indexDocuments()` functions (`cli.go` and `tools/indexing.go`) because the CLI version lives in package `main` and cannot be imported. Both paths delegate to the same `sanitize.Scan()` function, which is the single source of truth. A cross-reference comment (`// SYNC: identical sanitization logic in tools/indexing.go`) documents the duplication. An integration test verifies behavioral equivalence.
