## Context

`dewey init` scaffolds three kinds of files from embedded maps in the binary:

| Kind | Map | Target | Current write semantics (cli.go `newInitCmd`) |
|---|---|---|---|
| Slash commands | `deweySlashCommands` (slash_commands.go) | `.opencode/commands/<name>` | create only when `os.Stat` fails (~414-423); map-order iteration; write failures silently ignored |
| Agent definitions | `deweyAgentDefinitions` (agents.go) | `.opencode/agents/<name>` | `O_CREATE\|O_EXCL` (~451); existing file → `continue`; sorted iteration; rollback-on-partial-write; write failure fails init naming the path |
| Workflow template | `deweyWorkflowTemplates` (workflow_templates.go) | `.github/workflows/<name>` | `fileExists` guard + `continue` (~519-520); sorted iteration; MkdirAll and write failures warn-and-continue |

None of the three ever updates an existing file, so scaffolded content drifts forever: the `dewey` repo shows 19/130/11 lines of drift across `/dewey-compile`, `/dewey-store`, `/dewey-lint` vs the v3.6.0 embedded versions, and `snake-eyes` shows 26/2/2 (proposal, verified against issue #213). Operators must manually diff and copy after every Dewey upgrade.

User-owned scaffolds — `.uf/dewey/config.yaml`, `sources.yaml`, `knowledge-stores.yaml` (write-once under `!alreadyInitialized`) and `.opencode/dcp.jsonc` (three-way `protectTags` logic) — must keep their current semantics.

The `cli_test.go` suite pins the current behavior: skip-tests write marker-less custom content and assert preservation (`TestInitCmd_SkipsExistingSlashCommands`, `TestInitCmd_ScaffoldingSkipsExistingCuratorAgent`, `TestInitCmd_ScaffoldingSkipsExistingDeweyPublish`), and byte-parity tests hold scaffolded files and tracked repo copies to the embedded map values (`TestInitCmd_ScaffoldsDeweyPublishFromMap` and siblings).

This change introduces an ownership split modeled on Gaze's `isToolOwned` (per the proposal's description: a version marker plus content-equality detection; the Gaze source is not locally available — see A1).

## Goals / Non-Goals

### Goals

- Tool-owned scaffolds (the three embedded-map kinds) are refreshed to the current embedded content on every `dewey init`, with no new flag, whenever the on-disk file is verifiably untouched.
- Customized tool-owned files are never clobbered; the run logs `skip (customized)` naming the path.
- Every tool-owned write carries a self-describing marker (version + payload digest) enabling both the ownership check and provenance.
- User-owned files keep today's write-once / three-way semantics exactly.
- One shared write discipline (sorted iteration, rollback, surfaced errors) across all three scaffold kinds, preserving the existing agent-path discipline through the refactor.

### Non-Goals

- No new CLI flags, no `--force`, no migration command (proposal Out of scope).
- No changes to trust tiers, promotion, `store_learning`, index paths, or MCP tools.
- No change to user-owned scaffolds, the `.gitignore` append logic, or the `alreadyInitialized` gating of `.uf/dewey/` config files.
- No auto-repair of pre-marker legacy files whose content differs from the current embedded payload (safe default; see R1).
- No concurrency control between simultaneous `dewey init` runs (unchanged from today).

## Decisions

### D1 Marker format: final line, per-kind comment syntax, digest-functional

Every tool-owned write appends exactly one marker line as the **final line** of the file:

- Markdown payloads (slash commands, agents): `<!-- dewey:scaffold <version> sha256:<64 lowercase hex> -->`
- YAML payloads (workflow template): `# dewey:scaffold <version> sha256:<64 lowercase hex>`

Grammar (read side accepts either comment form on the final line):

```
^\s*(?:<!--\s*|#\s*)dewey:scaffold\s+(\S+)\s+sha256:([0-9a-f]{64})\s*(?:-->)?\s*$
```

Rationale:

- **Last-line placement** avoids frontmatter-aware parsing entirely (OpenCode command/agent frontmatter must remain the first block of the file, and YAML has no trailing-comment ambiguity), and gives a uniform rule for both file kinds.
- **Per-kind comment syntax** keeps the marker invisible to the host parsers: an HTML comment in markdown, a `#` comment in YAML. A shared `<!-- -->` syntax would corrupt the workflow YAML; a shared `#` syntax would render as a heading in markdown.
- **The digest is the ownership signal; the version is informational provenance only.** The version comes from the existing `var version` in main.go (ldflags-injected; `dev` in non-release builds). Classification depends solely on the digest, so version-string churn never changes ownership outcomes (D8).

Alternatives rejected: a frontmatter `dewey_version` field (requires frontmatter parsing/serialization and perturbs the byte-identical two-copy discipline), and first-line placement (breaks frontmatter-first requirements).

**Convention deviation, recorded:** go-pack AP-006 (SHOULD) prescribes inserting version markers after YAML frontmatter in scaffolded Markdown files. This design deviates deliberately with final-line placement: frontmatter must stay byte-stable for the two-copy parity discipline (D6) and the frontmatter shape tests, the workflow template carries no frontmatter at all, and a uniform placement rule for both file kinds avoids frontmatter-aware insertion logic entirely. AP-006 is SHOULD-level; the deviation and its rationale are recorded here so future reviewers need not rediscover the conflict.

### D2 Marker is derived at runtime; maps stay pure content

The embedded map values remain the raw scaffold bodies. The marker is computed at write time:

- `canonicalBody(v)` = the map value normalized to end with exactly one trailing newline.
- `digest` = `sha256(canonicalBody)`.
- Written payload = `canonicalBody + markerLine + "\n"`.

A hand-maintained marker inside the map values was rejected: every content edit would require recomputing and duplicating a digest by hand — an error-prone second source of truth. Runtime derivation keeps the maps the single editable source and makes a wrong marker impossible to ship. The read side strips the final line and hashes the remainder, so round-trips are exact (D3).

### D3 Classification is a pure function

```go
type scaffoldDecision int // scaffoldCreate | scaffoldUpdate | scaffoldCurrent | scaffoldSkip

func classifyScaffold(existing []byte, exists bool, embeddedBody []byte) scaffoldDecision
```

Decision table (mirrors the `dewey-init-ownership` spec):

1. `!exists` → `create`.
2. Final line parses as a marker and `sha256(payload-before-marker) == marker digest` → tool-owned/untouched:
   - payload == `canonicalBody(embedded)` → `current` (no write — byte equality, not hash equality, so no collision reasoning is ever load-bearing);
   - otherwise → `update`.
3. No marker and file content == `canonicalBody(embedded)` (byte-identical) → `update` (marker bootstrap: the only rewrite this path performs re-adds the marker to content we would write anyway).
4. Otherwise — marker absent with differing content, malformed marker (digest not 64 lowercase hex), digest mismatch → `skip` (customized).

Case 4 is deliberately fail-safe: a file scaffolded before markers existed and since left untouched by the user is indistinguishable from a user edit, so it is preserved (R1). The function is pure (content in, decision out), satisfying the proposal's testability assessment; I/O is a thin wrapper in D4. This refines the proposal's sketch `func(path string, embedded string) bool` by separating the predicate from file reading — same guarantee, directly table-testable without temp files.

**Input acquisition (the I/O wrapper, specified).** The pure function receives `(existing, exists, embeddedBody)`; the thin wrapper in D4 acquires them and owns the failure classes the content table above does not enumerate:

- `os.Lstat(path)` reports a symlink → decision `skip` (customized) without reading content. The classify read must never lead to a rename that silently replaces the user's symlink with a regular file (a natural pattern for this feature's audience: `.opencode/agents/curator.md` symlinked to a canonical copy shared across repos). Posture matches promote's symlink guard (PW-FR-004 in git-is-the-source-of-record-for-page-trust-tiers): symlink escape rejected, TOCTOU not followed — a path that becomes a symlink between check and write is not chased.
- The path exists but `os.ReadFile` fails (EACCES on the file, a directory sitting at the path, transient I/O error) → init fails naming the path. A read failure is never mapped to the `skip` row: mislabeling an environment failure as a user customization would mask it from the operator.
- `os.Lstat` fails with anything other than not-exist → init fails naming the path.

### D4 One shared scaffold routine, strictest-existing discipline

A single helper in `scaffold.go` (package main — where the maps already live; an `internal/scaffold` package would force exporting the maps for no gain) drives all three kinds:

```go
func scaffoldToolOwnedFiles(dir string, files map[string]string, kind scaffoldKind, label string) error
```

- Sorted key iteration (agents already do this; commands gain it).
- `os.MkdirAll(dir)` failure fails init naming the directory (agents already do this; commands currently skip silently, templates warn-and-continue).
- Read-and-classify per D3's input-acquisition rules: `os.Lstat` then `os.ReadFile` — a symlinked path is skipped as customized without reading or writing through it; a read or stat failure other than not-exist fails init naming the path and is never labeled customized.
- `create`: `os.OpenFile(path, O_CREATE|O_EXCL|O_WRONLY, 0o644)` → write → close; on write/close failure remove the partial file and fail init naming the path — the existing agent discipline, verbatim. One exception: an `EEXIST` from the exclusive open — a concurrent `dewey init` created the path between classification and open — is the benign race today's code already tolerates (agents: O_EXCL-existing → continue; commands: last-writer-wins). The run logs `skip (concurrent)` at info naming the path, does not replace the concurrent writer's file, and continues; the concurrent run's own `scaffolded` event covers observability for that path.
- `update`: write the payload to `os.CreateTemp(dir, ...)` (chmod 0o644), then `os.Rename(temp, path)`. Rename is atomic on the supported platforms, so a failed update leaves the original file intact and no partial residue; temp-file failure cleans up the temp and fails init naming the path. The rename never executes against a symlinked path — the Lstat gate in the read-and-classify step precedes it — so a user's symlink is never silently replaced by a regular file.
- `skip`: `logger.Info("skip (customized)", "path", path)` — no filesystem mutation.
- Outcome events: `scaffolded <label>` / `updated <label>` / `skip (customized)`, each at info with the path (label keeps the existing kind context: "slash command", "agent", "workflow template example").
- `current`: no write, no event (idempotent silence).

This is the proposal's "deterministic iteration and rollback-on-partial-write semantics preserved" line read as *the shared routine carries the agents' discipline*. The behavior change it forces on the two lenient kinds is deliberate and is called out as R2.

### D5 Unchanged surfaces (explicit)

- `.opencode/` composability guards for commands, agents, and DCP config — untouched; init still never creates `.opencode/`.
- `.opencode/command/` → `.opencode/commands/` migration — still runs first; migrated files are then classified like any other existing file.
- User-owned files — `config.yaml`, `sources.yaml`, `knowledge-stores.yaml` write-once; `dcp.jsonc` three-way logic verbatim.
- `.gitignore` append logic and `alreadyInitialized` gating — untouched. `.gitignore` is user-owned but not write-once: it keeps its append-if-missing-patterns semantics, which the ownership spec's preservation requirement (DIO-FR-005) now states explicitly.
- `dewey doctor` DCP check (cli.go ~1845) — read-only, untouched.

### D6 Parity tests compare through the payload constructor

The byte-parity tests (`TestInitCmd_ScaffoldsDeweyPublishFromMap` and siblings, the curator/distiller agent parity tests, `TestInitCmd_ScaffoldsDistillWorkflowExample`) currently assert scaffolded-file == map-entry == tracked-copy. After D2 the scaffolded form is `marker + canonicalBody`, so these tests compare through the same construction helper the writer uses. The tracked copies in this repo (`.opencode/commands/dewey-*.md`, `.opencode/agents/*.md`, `.github/workflows/dewey-distill.yml.example`) are regenerated by running the newly built `dewey init` at the repo root — the marker-bootstrap path rewrites precisely those byte-identical files — keeping the two-copy discipline intact. Existing skip-tests need no changes: their fixtures are marker-less custom content, which the decision table preserves (A2).

### D7 Code placement

New `scaffold.go` + `scaffold_test.go` in package main: `scaffoldKind`, `canonicalBody`, `scaffoldMarker`, `scaffoldPayload`, `parseScaffoldMarker`, `classifyScaffold`, `scaffoldToolOwnedFiles`. `newInitCmd`'s three scaffold blocks collapse to three `scaffoldToolOwnedFiles` calls with their existing guards; the migration block stays inline ahead of the commands call. GoDoc on every exported… (unexported, package-internal) — on every package-level function per house style.

### D8 Update comparison is body-only

`update` fires when the *payload* differs, not the marker: a rebuild with a different `version` string but identical body leaves `current` files untouched (no churn), because case 2 compares the on-disk payload to the embedded canonical body directly. A rewritten file always receives a fresh marker matching its new payload.

## Risks / Trade-offs

1. **Legacy drifted files never auto-refresh (D3 case 4).** A marker-less scaffold of an older embedded version is preserved as if customized. Accepted per the proposal's safe default; the operator path is delete-and-re-init, and files become marker-managed from the first post-change scaffold. Documented in README.
2. **Stricter write discipline surfaces previously-silent failures (D4).** Slash-command write/dir failures were ignored and template failures warned; both now fail init. This converts silent scaffold loss into a loud error, matching the agent precedent (`TestInitCmd_CuratorAgentWriteFailureFailsInit`). A vault with an unwritable `.opencode/commands/` that previously "succeeded" now errors — the correct trade, noted as a behavior change.
3. **Marker line in consumed files.** OpenCode parses command/agent markdown (marker is a trailing HTML comment — inert); GitHub Actions parses the workflow example (marker is a trailing `#` comment — inert). Frontmatter stays first because the marker is last (D1).
4. **Rewritten files lose non-default file modes.** An update rewrites 0o644 regardless of prior mode. Trivial and consistent with create; not user data.
5. **Concurrent init runs remain unserialized** — same benign-race posture as today, now specified: the EEXIST window between classification and the exclusive open is benign (D4 — log `skip (concurrent)`, leave the concurrent writer's file, continue) rather than an init-fatal error, so a vault that previously initialized cleanly under concurrent runs still does. No broader serialization is attempted; out of scope.

## Constitution Alignment

The proposal records all four principles as PASS; the design keeps them:

- **Autonomous Collaboration (I):** scaffolds remain plain filesystem artifacts; the marker adds self-describing provenance with zero runtime coupling.
- **Composability First (II):** `.opencode/` guards unchanged; the ownership predicate is local content comparison — no network, no new dependency, no mandatory tool.
- **Observable Quality (III):** `scaffolded` / `updated` / `skip (customized)` events with paths, deterministic order, and digest-based provenance in every tool-owned file; no gate is weakened — parity coverage extends to the new predicate.
- **Testability (IV):** `classifyScaffold` is pure and table-tested; the writer is exercised through `newInitCmd()` + `t.TempDir()`; no test needs a server, network, or external service.

## Assumptions

### A1 Gaze `isToolOwned` pattern is as the proposal describes

The Gaze repository (`internal/scaffold/scaffold.go`) is not available locally. This design implements the proposal's description — version marker plus content-equality detection — rather than verified Gaze internals; the digest-in-marker construction (D1/D2) satisfies that description without shipping prior embedded versions.

### A2 Existing skip-tests encode the legacy default

Every current skip-test writes marker-less content that differs from the embedded payload, which case 4 of the decision table preserves; the suite passing unmodified is the backward-compatibility gate for the safe default.

### A3 `version` is informational

`var version` is `dev` unless ldflags inject a release string. Markers from dev builds carry `dev`; this never affects classification (D1, D8), only the provenance note.

### A4 All scaffold map keys are the intended tool-owned set

Verified: the nine `deweySlashCommands` keys, two `deweyAgentDefinitions` keys, and single `deweyWorkflowTemplates` key are exactly the files `dewey init` writes today; ownership by map membership (not filename) makes the agent files without a `dewey-` prefix correctly tool-owned.
