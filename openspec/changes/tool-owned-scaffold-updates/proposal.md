# Proposal: Tool-Owned Scaffold Updates for `dewey init`

## Why

`dewey init` currently only creates missing files — it never updates scaffolded commands, agents, or workflow templates on re-init. Verified behavior:

- Slash commands (`deweySlashCommands` → `.opencode/commands/*.md`): write only when `os.Stat` fails (cli.go:414-423, missing-file-only).
- Agents (`.opencode/agents/*.md` via `deweyAgentDefinitions` in `agents.go`): `O_CREATE|O_EXCL` (cli.go:451) — existing file is always preserved.
- Workflow template (`.github/workflows/dewey-distill.yml.example` via `deweyWorkflowTemplates`): `fileExists` guard + continue (cli.go:519-520).
- No `--force` flag exists.

This causes drift. Repo `dewey` shows 19/130/11 lines of drift across `/dewey-compile`, `/dewey-store`, `/dewey-lint` commands vs v3.6.0 embedded versions; `snake-eyes` shows 26/2/2 lines. Each `dewey init` from v3.6.0 adds 6–13 files per repo (~850–1100 lines) but never refreshes them. Operators must manually diff and copy to stay current.

Downstream `snake-eyes` PR #16 (2026-05-04) and Dewey PR #185 (2026-05-08) demonstrate the cost: slash commands evolve frequently and stale copies miss capability updates (e.g., compile/curate/verify flags).

We need an ownership split modeled on Gaze `isToolOwned`: tool-owned scaffolds auto-update when still tool-owned; user-owned config is write-once; customized tool-owned files are never overwritten.

## What Changes

**Ownership classification:**

- **Tool-owned** — generated from embedded maps in the Dewey binary: slash commands (`.opencode/commands/dewey-*.md`), agents (`.opencode/agents/*.md` from `agents.go`), and `.github/workflows/dewey-distill.yml.example` (from `deweyWorkflowTemplates`). Every tool-owned write appends a final-line ownership marker — `<!-- dewey:scaffold <version> sha256:<hex> -->` in markdown, `# dewey:scaffold <version> sha256:<hex>` in YAML — carrying the writing binary's version and the SHA-256 digest of the file's payload. Ownership detection is digest-functional: a file is verifiably untouched when its marker digest equals the SHA-256 of its own payload. Classification never consults prior embedded versions (the binary ships none) and never reads the version field; marker-less files are tool-owned only when byte-identical to the current embedded payload (marker bootstrap).
- **User-owned** — vault configuration: `.uf/dewey/config.yaml`, `.uf/dewey/sources.yaml`, `.uf/dewey/knowledge-stores.yaml`, and `.opencode/dcp.jsonc` (existing three-way logic preserved). Write-once; never overwritten.

**New `dewey init` behavior:**

- On re-init, for each tool-owned path: if file missing → create (existing behavior). If file exists and the marker digest matches the file's own payload (untouched) and the payload differs from the embedded payload → overwrite with current embedded content plus a fresh marker (no flag required). If untouched and the payload equals the embedded payload → no write. If marker-less and byte-identical to the embedded payload → rewrite to add the marker. Otherwise (marker absent with differing content, malformed marker, digest mismatch, symlinked path) → leave untouched, log `skip (customized)` at info level. An existing path that cannot be read fails init naming the path — a read failure is never labeled a user customization.
- User-owned files: retain current idempotency — create only when absent. Existing 3-way `dcp.jsonc` logic (skip if `protectTags` present, warn if present without it, else scaffold) remains unchanged.
- Deterministic iteration (sorted map keys) and rollback-on-partial-write (remove + error) semantics preserved. One shared strict write discipline now applies to all three scaffold kinds: a failed or partial create leaves no residue and fails init naming the path, and a failed overwrite leaves the original file intact (atomic replace) — so slash-command and workflow-template write failures that previously succeeded silently or warned now fail init (design R2; operator-visible behavior change).
- Logging: `scaffolded` for new files, `updated` for tool-owned overwrites, `skip (customized)` for preserved divergences — enabling observable quality and audit.

**Out of scope:** no new CLI flags, no migration command, no changes to promotion/trust-tier logic, no changes to `store_learning` or index paths.

## Capabilities

### New Capabilities
- `dewey-init-ownership`: Ownership-aware init that distinguishes tool-owned scaffolds from user-owned config and auto-updates tool-owned files when content differs.
- `dewey-init`: A new consolidating capability for `dewey init` scaffolding behavior — the shared write discipline and the ownership-aware refresh. It consolidates init-scaffolding requirements previously carried by three pending, unarchived deltas under other capability names (fix-init-scaffolding's `init-flow`, command-dir-plural's `init-command-dir`, dewey-init-dcp-scaffold's `init-dcp-scaffold`); the preservation-only re-init contract those deltas state is superseded here and recorded as historical context inside the delta.

### Modified Capabilities
- (none — `dewey-init` is new; see New Capabilities above)

### Removed Capabilities
- (none)

## Impact

- **Files:** `cli.go` (`newInitCmd` scaffold blocks for `deweySlashCommands`, `deweyAgentDefinitions`, `deweyWorkflowTemplates` collapse to shared-writer calls), new `scaffold.go` + `scaffold_test.go` in package main (marker primitives, classification, shared writer — design D7), `cli_test.go` (parity assertions extended). The embedded maps in `agents.go` and `workflow_templates.go` stay pure content — the marker is derived at write time (design D2).
- **Behavior:** Re-running `dewey init` after upgrading Dewey brings marker-managed (untouched) scaffolds current without manual diffs; user customizations are never clobbered. Pre-marker drifted files are preserved, not auto-refreshed (see below). Init now hard-fails on scaffold write failures that previously succeeded silently (see What Changes).
- **Backward compatible:** Marker-less files created before this change are treated as tool-owned only when byte-identical to the current embedded payload, and otherwise preserved as customized — the binary ships no prior embedded versions, so pre-marker drift cannot be auto-cleared. Refreshing a drifted pre-marker scaffold (the 19/130/11-line drift motivating this change) requires a one-time delete-and-re-init; files become marker-managed from the first post-change scaffold.
- **Dependents:** All repos using `dewey init` (dewey itself, snake-eyes, future adopters). No MCP tool contract change.
- **Content:** Operator-facing trust-boundary change per the org convention — the three-issue content pipeline (docs sync + blog + tutorial) is filed in `unbound-force/website` alongside the change (task 4.3 records the issue numbers).

## Constitution Alignment

Assessed against the Unbound Force org constitution (.specify/memory/constitution.md).

### I. Autonomous Collaboration

**Assessment**: PASS

Artifacts remain the collaboration surface: `dewey init` scaffolds produce version-marked markdown files that agents and commands consume via filesystem, not via runtime coupling. No new inter-hero RPC or shared memory is introduced. Self-describing outputs (scaffolded markdown with embedded version provenance) improve artifact contracts.

### II. Composability First

**Assessment**: PASS

`dewey init` remains independently installable; `.opencode/` scaffolding stays guarded by `fileExists(.opencode)` composability check. No mandatory dependency is added — the ownership predicate is local file-content comparison, not a network call. Vaults without OpenCode continue to initialize without agent/command scaffolds.

### III. Observable Quality

**Assessment**: PASS

Init already emits structured `logger.Info` events. This change adds `updated` vs `skip (customized)` outcomes, preserving machine-parseability (log levels, path fields) and adding provenance via the `dewey:scaffold` marker embedded in each scaffolded file. No quality gate is weakened; existing parity tests in `cli_test.go` gain coverage for the new predicate.

### IV. Testability

**Assessment**: PASS

`isToolOwned` is a pure function `func(path string, embedded string) bool` testable with temp files and no external services. Scaffold blocks remain parameterized over `vaultPath`, keeping `newInitCmd` testable with `t.TempDir()`. Verification tasks below require side-effect-isolated tests; no test requires a running Dewey MCP server.

<!-- scaffolded by uf vdev -->
