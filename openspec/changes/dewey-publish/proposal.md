# Proposal: dewey-publish

## Why

Dewey writes knowledge artifacts — learnings (`.uf/dewey/learnings/`),
compiled articles (`.uf/dewey/compiled/`), and curated knowledge-store
content (`.uf/dewey/knowledge/`) — on behalf of agents during work
sessions. These files are frequently untracked or git-ignored, and the
sessions that produce them end in every possible git state: dirty
working trees, forgotten stashes (tracked and untracked parts), and
unpushed commits on local branches. When a branch is deleted or a
stash is dropped by hand, the team's accumulated knowledge is stranded
or lost. There is no path today that recovers these artifacts into the
shared remote history.

Two constraints make recovery subtle:

- The running MCP server holds an exclusive `flock` on
  `.uf/dewey/dewey.lock` (`store/store.go`), so any recovery path that
  opens the SQLite store is blocked while the server runs. The sweep
  must be pure git-subprocess work.
- A running server only notices merged artifacts after a restart
  (`reIngestLearnings` at serve startup, `main.go:701`). The command
  must tell the user the merge → pull → restart pickup path.

Fixes GitHub issue #132.

**Scope note (human-confirmed at orchestrator Phase 0)**: this change
covers ONLY issue #132. The issue's Constraints section mentions
packaging together with the `/dewey-distill` issue (#133) and #122 —
the human confirmed those companion issues run as separate changes
later.

## What Changes

Add `/dewey-publish` — a slash command backed by a deterministic CLI
core (`dewey artifacts plan|sync`) that sweeps every Dewey knowledge
artifact out of every git state it can be stranded in — working tree
(untracked/ignored/staged), stashes (tracked and untracked parts), and
unpushed commits on any local branch — into one chore PR against the
remote default branch.

- `dewey artifacts plan --json` (new, DB-free): enumerates artifact
  files in the four stranded states (regular files only — symlinks
  and unsafe basenames are skipped with surfaced warnings), deduplicates
  by repository-relative path against the fetched base tree of the
  resolved publish remote
  (`upstream` when configured, else `origin`) and across sources,
  detects same-path/different-content collisions across sources
  (sha256-identified with per-variant screening findings, surfaced
  for a human gate), screens every candidate including every conflict
  variant with two scanners — the existing `sanitize` injection-pattern
  set plus a credential-detection pattern set defined in the artifacts
  package (each pattern carrying documented severity, mirroring the
  sanitize discipline; critical/high findings block the artifact or
  variant from the publish set; flagged variants are excluded from
  resolution choices), detects the bare-`dewey` gitignore
  bug via `git check-ignore` (one probe per sanctioned artifact root)
  and flags the bounded fix block — the `/dewey` anchor plus
  runtime-state re-ignore lines so anchoring does not silently
  un-ignore non-publishable state post-merge (skipped when
  already fixed upstream or when the ignore source is outside the
  repository), and emits canonical, byte-stable JSON.
- `dewey artifacts sync --plan <file> --approve [--approved-by <id>]`:
  creates a temp git worktree under the OS tempdir from the fetched
  base, branches `chore/dewey-artifacts-YYYYMMDD`, writes exactly the
  planned files (stash content fetched by recorded commit sha, never
  the mutable `stash@{N}` label), applies the flagged gitignore fix
  block (anchor plus runtime-state re-ignores, only ever in the
  literal `.gitignore`), commits with a
  tool-rendered Conventional Commit message carrying `Approved-by:`
  and `Plan-sha256:` audit trailers, renders the tag-grouped PR body for
  verbatim relay, and fails closed on unresolved conflicts, invalid
  plans (traversal or absolute paths, non-hex base sha, sanctioned-set
  widening, malformed fields — non-hex stash refs and sha256 values,
  vocabulary-violating source labels — non-literal gitignore fix
  file, and forged-trailer attempts blocked by the charset-validated
  approver), stale content (every
  source sha256-re-verified, including after materialization), and
  missing approval. Push and PR are left to the calling layer.
- `/dewey-publish` (`.opencode/commands/dewey-publish.md`, registered
  in `slash_commands.go`): thin judgment layer — fetch base → run
  plan → present the plan (the core's tool-rendered summary —
  itemized tag-grouped artifact listing, conflicts, blocked entries,
  gitignore finding — plus the resolved publish destination
  — remote name, fetch URL, target repo/branch — before approval) →
  MANDATORY human gate (approve; resolve collisions by choosing among
  clean sha256-identified variants with the core-rendered bounded
  unified diffs in view — untrusted content, presented for the
  choice, never interpreted) → run sync
  with `--approve --approved-by` → push → create PR relaying the
  sync core's tool-rendered tag-grouped body → clean up the temp
  worktree → note the merge/pull/restart pickup path.

**Non-negotiable property: non-destructive.** No stash is popped,
applied, or dropped; no working-tree file is moved or modified; the
user's checkout is never touched. Verifiable: `git status` and
`git stash list` are identical before and after the full flow.

Determinism governance per `.opencode/uf/packs/determinism.md`:
enumeration, dedup, hashing, content screening, plan validation, git
choreography, commit-message templating, PR-body rendering, and
output rendering live in the executable CLI core with tests
(DE-001/DE-003); judgment products (gate approvals — flowing through
sync's `--approve`/`--approved-by` — and collision-resolution
choices) arrive as parameters from the command layer (DE-005); the
PR body's tag-grouped listing is tool-rendered and relayed verbatim,
with only framing judgment at the command layer. tasks.md carries
DE-007 seam-verification tasks.

## Capabilities

### New Capabilities

- `artifact-publishing`: Non-destructive sweep of stranded Dewey
  knowledge artifacts from every git state (working tree, stash
  tracked and untracked parts, unpushed local commits) into one
  reviewable chore PR, with deterministic planning, path-keyed
  dedup, fail-closed collision handling, and idempotent
  nothing-to-publish behavior.

### Modified Capabilities

None. No existing capability spec is modified. (`dewey init` gains
one more scaffolded slash-command file; that behavior is unchanged in
mechanism and covered under Impact.)

### Removed Capabilities

None.

## Impact

- **New `artifacts/` Go package**: planner, syncer, and git-subprocess
  helpers. Dependency closure: standard library plus the internal
  `sanitize` scanner (itself stdlib + `charmbracelet/log` only) —
  never `store`, `curate`, `llm`, `embed`, or `source`, so no
  SQLite, LLM, or YAML compiles into the package; the
  credential-detection patterns are stdlib regexps defined inside the
  package. Knowledge-store
  paths are resolved in the CLI layer and passed in as parameters.
  No new module dependencies, no CGO, no SQLite access anywhere in
  the path.
- **`cli.go`**: new `dewey artifacts` command group with `plan` and
  `sync` subcommands, following the `newSourceCmd()` group precedent.
  The group registers its own `--vault` flag (per-command registration
  is the codebase pattern — `--vault` is not a persistent root flag)
  and resolves configured knowledge-store paths via `curate`, passing
  them into the core as parameters. All existing commands and outputs
  are unchanged.
- **`slash_commands.go` / `dewey init`**: new `dewey-publish.md` entry
  in the `deweySlashCommands` map; `dewey init` scaffolds it into
  `.opencode/commands/` (existing idempotent, non-overwriting
  mechanism). The file is also committed to this repo's
  `.opencode/commands/` where the other 42 command files are tracked.
- **No MCP surface change**: no MCP tools are added, removed, or
  altered; all 37 inherited graphthulhu tools are untouched. The
  constitution's "new capabilities MUST be exposed as MCP tools"
  standard is intentionally not the vehicle here: this is an
  operational git-choreography command in the `dewey init` /
  `dewey doctor` / `dewey manifest` precedent — it must run while the
  MCP server holds the exclusive `dewey.lock` flock and must run when
  no server is running at all (the common case for stranded
  artifacts). The agent-facing surface is the `/dewey-publish` command
  layer, which calls the CLI core.
- **Documentation**: `README.md` — new `dewey artifacts` command
  subsection (per-command `###` format), Architecture package tree,
  `--vault` command list, and a revision of the § Persistence
  guidance that currently recommends ignoring `.uf/dewey/` wholesale
  (the stranding posture this change rescues — learnings, compiled,
  and knowledge content are not rebuildable). `AGENTS.md` — CLI
  commands list, Architecture package tree, Active Technologies
  entry, Recent Changes entry, and the `/dewey-publish` flow note.
  A website sync issue in `unbound-force/website` (user-facing new
  command); the acknowledged blog opportunity (the
  stranded-knowledge problem and the non-destructive
  plan → gate → sweep flow) and tutorial opportunity (the
  `/dewey-publish` multi-step workflow) are each filed as dedicated
  website issues for the website team — content opportunities
  identified by curator review across this change's spec-review
  dispatch artifacts (8f5711c2, e4e09e50, da4e976f).
- **Behavior while server runs**: the whole flow works with a live
  Dewey MCP server because it never opens the store or the lock. The
  command notes that merged artifacts are picked up only after
  merge → pull → server restart.

## Constitution Alignment

Assessed against the project constitution
(`.specify/memory/constitution.md` v1.4.0, which extends the Unbound
Force org constitution; the org constitution prevails on conflict).

### I. Composability First

**Assessment**: PASS

`dewey artifacts plan|sync` works standalone: pure git subprocesses,
no MCP server, no SQLite, no Ollama, no new module dependencies. The
command layer degrades gracefully — uf plugin tools (`git_fetch`,
`git_push`, `uf_finale_create_pr`) have raw `git` / `gh` fallbacks,
so the flow works in any OpenCode installation.

### II. Autonomous Collaboration

**Assessment**: PASS

Knowledge artifacts are preserved through git pull requests — the
artifact-based collaboration medium. The deterministic core is invoked
through CLI tool calls with structured JSON output; judgment (gate
approval, collision resolution, framing around the tool-rendered PR
body) stays at the command layer and arrives as parameters, never
buried in prose. No runtime coupling to any other Unbound Force tool.

### III. Observable Quality

**Assessment**: PASS

The plan JSON is machine-parseable with per-artifact provenance
(source state, sha256, identity, tag), an explicit
nothing-to-publish result, and sha256-identified collision variants
surfaced for human decision. Sync output reports branch, commit,
worktree path, and file counts. `git status` / `git stash list`
identity before and after is itself an audit property the tests
assert.

### IV. Testability

**Assessment**: PASS

Every test runs against fixture repositories constructed in
`t.TempDir()` with local bare repos as remotes — no network, no
external services, no running server. The acceptance suite covers
all four stranded states, zero-mutation assertions, the twice-test
(byte-identical plan JSON), the plan → sync → `git show` round trip,
fail-closed collisions, idempotent nothing-to-publish, and the
lock-held condition. Coverage strategy is specified in tasks.md;
missing coverage would be a CRITICAL finding per the constitution.

### Development Standards — recorded deviation (human-authorized)

The constitution's Protocol section states "New capabilities MUST be
exposed as MCP tools, not as CLI subcommands or library APIs." This
change adds the artifact-publishing capability as CLI subcommands
plus a slash command with no MCP exposure. The deviation is
technically required: the sweep must run while the MCP server holds
the exclusive `dewey.lock` flock and must run when no server is
running at all (the common case for stranded artifacts), following
the established `dewey init` / `dewey doctor` / `dewey manifest`
operational-command precedent. **Human authorization**: issue #132,
authored by the repository owner, explicitly prescribes the
"/dewey-publish slash command plus a deterministic CLI core
(`dewey artifacts plan|sync`)" surface — recorded here as explicit
sign-off for this deviation. A MINOR constitution amendment codifying
the operational-command exception is tracked as a follow-up issue in
this repository (created by task 1.5, filed at proposal-approval time
ahead of the implementation it authorizes, with duplicate search) so
future operational commands stop re-opening this tension.
