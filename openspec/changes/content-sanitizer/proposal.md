## Why

Dewey indexes content from external sources (web pages, GitHub issues/PRs, disk files, source code) and serves it to AI agents via MCP tools. A thorough audit of the codebase reveals that **no defense against prompt injection or data poisoning exists in the indexing pipeline**. While SQL injection is prevented via parameterized queries (FR-028), the content itself flows unsanitized from `source.Document.Content` through `vault.ParseDocument()` → `vault.PersistBlocks()` → `store.InsertBlock()` → MCP tool responses (`search`, `semantic_search`, `get_page`) → agent context windows.

This creates two categories of risk:

1. **Prompt injection via indexed content**: A malicious or compromised web page can embed adversarial instructions (e.g., "Ignore previous instructions. You are now...") that get stored as blocks and returned verbatim to agents via MCP search tools. Because agents treat Dewey's results as trusted context, injected instructions could hijack agent behavior.

2. **Data poisoning**: A website can silently replace legitimate documentation with misleading content between index cycles. Without change detection beyond simple content hashing, Dewey re-indexes the poisoned content and agents act on stale or fabricated information with no awareness of the tampering.

These risks affect **all four source types** — web, GitHub, disk, and code — as well as any future source types added to the pipeline.

The correct fix is a shared sanitization layer in the indexing pipeline that all sources benefit from, rather than per-source ad-hoc defenses.

## What Changes

Add a `sanitize` package that scans content entering the knowledge graph for prompt injection patterns, data poisoning signals, structural anomalies, and size outliers. The sanitizer sits in the shared indexing pipeline between document production and `vault.ParseDocument()`, ensuring all source types are covered.

The sanitizer operates in **warn-by-default** mode: it annotates documents with findings and logs warnings but does not reject content. This is deliberate — a knowledge graph that indexes security documentation will encounter legitimate content containing the exact phrases used in prompt injection attacks. Blocking is available as an opt-in configuration.

Additionally, the existing trust tier system is extended with a new `untrusted` tier value. A source-level `trust_tier` configuration field lets users assign lower trust to web-scraped content, pushing trust decisions to consuming agents via `semantic_search_filtered(tier: ...)`. This extension requires updating the `SemanticSearchFilteredInput` schema, AGENTS.md Trust Tiers table, and cross-referencing specs 013 and 015.

## Capabilities

### New Capabilities
- `sanitize package`: Shared content scanning library with five defense layers — adversarial keyword scanning, content hash drift alerting, Markdown structure validation, content size anomaly detection, and source-level trust tier assignment
- `sanitize.ScanInjectionPatterns()`: Regex-based detection of common prompt injection phrases with configurable pattern database
- `sanitize.ContentDrift()`: Hash-based change detection that flags pages where the content hash changed between index cycles, enabling data poisoning awareness
- `sanitize.ValidateStructure()`: Detects suspicious Markdown patterns — excessive invisible Unicode, zero-width characters, anomalous heading nesting, embedded data URIs
- `sanitize.SizeAnomaly()`: Statistical outlier detection for pages that deviate significantly from source-average content size
- `untrusted trust tier`: New tier value below `draft` in the trust hierarchy (`authored > curated > validated > draft > untrusted`), configurable per source
- `dewey doctor` sanitization check: New diagnostic check category reporting content warning counts per source
- `dewey lint` sanitization rule: New lint rule surfacing pages with active sanitization warnings

### Modified Capabilities
- `cli.go indexDocuments()`: Inserts `sanitize.Scan()` call before `vault.ParseDocument()`, merges findings into page properties after parsing
- `tools/indexing.go indexDocuments()`: Same sanitization insertion in the MCP tool indexing path
- `source/config.go`: Extended with optional `trust_tier` and `sanitize_mode` field validation
- `tools/lint.go`: Extended with sanitization warning lint check
- `types/tools.go`: Updated `SemanticSearchFilteredInput` tier schema to include `untrusted`
- `tools/semantic.go`: Updated tier documentation to include `untrusted` in ordering

### Removed Capabilities
- None

## Impact

**Files added:**
- `sanitize/sanitize.go` — Core scanning orchestrator, `ScanInput`/`ScanResult`/`Finding` types
- `sanitize/patterns.go` — Adversarial keyword pattern database
- `sanitize/drift.go` — Content hash drift detection
- `sanitize/structure.go` — Markdown structure validation
- `sanitize/anomaly.go` — Content size anomaly detection
- `sanitize/sanitize_test.go` — Comprehensive test suite with known injection payloads

**Files modified:**
- `cli.go` — Insert `sanitize.Scan()` in `indexDocuments()`, merge findings into properties
- `tools/indexing.go` — Insert `sanitize.Scan()` in MCP tool `indexDocuments()`, merge findings
- `source/config.go` — Add `trust_tier` and `sanitize_mode` validation
- `source/manager.go` — Propagate `trust_tier` from config to source metadata
- `tools/lint.go` — Add sanitization warning lint check
- `tools/doctor.go` — Add sanitization diagnostics
- `types/tools.go` — Update `SemanticSearchFilteredInput` tier schema description
- `tools/semantic.go` — Update tier ordering documentation comment
- `store/store.go` — Add `GetPagesWithProperty()` query method using `json_extract()`

**No changes to:**
- Any existing MCP tool signatures or response schemas (only schema description text updated)
- Store schema (no new tables or columns — findings merged into existing `properties` JSON column)
- Any of the 50 existing MCP tools' behavior
- Backward compatibility of existing indexed content

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Autonomous Collaboration

**Assessment**: PASS

The sanitizer operates within Dewey's indexing pipeline and produces structured findings that are merged into page properties. No runtime coupling with other tools is introduced. Agents discover sanitization warnings through the same MCP tool registry — no new communication channels or shared memory.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable. The sanitizer is a pure Go library with no external dependencies — no cloud APIs, no LLM calls, no third-party services. It runs entirely locally using regex patterns and statistical methods. The `trust_tier` configuration defaults to `authored` for backward compatibility, so existing deployments are unaffected.

### III. Observable Quality

**Assessment**: PASS

This change directly strengthens observable quality. Sanitization findings include the pattern matched, line number, severity level, and source context. Findings are persisted in page properties and queryable via `json_extract()`. The `dewey doctor` command reports per-source warning counts. The `dewey lint` command surfaces pages with active warnings. All findings are machine-parseable and auditable.

### IV. Testability

**Assessment**: PASS

The `sanitize` package is designed as a pure-function library operating on strings and statistics. All exported functions (`Scan`, `ScanInjectionPatterns`, `ContentDrift`, `ValidateStructure`, `SizeAnomaly`, `ComputeStats`) are contract-critical and testable in isolation without external services. Coverage target: >= 80% line coverage for the `sanitize` package. Integration tests verify pipeline behavior using in-memory SQLite.
