# Design: dewey-publish

## Context

Dewey knowledge artifacts (learnings, compiled articles, curated
knowledge-store content) are written under `.uf/dewey/` by agents and
end up stranded in every git state: untracked/ignored/staged working
trees, stashes (tracked part and `^3` untracked part), and unpushed
commits on local branches. The running MCP server holds an exclusive
`flock` on `.uf/dewey/dewey.lock` (`store/store.go:105-112`), so the
sweep must never open the store. A running server only re-ingests
merged learnings at restart (`reIngestLearnings`, `main.go:701`).
Artifact layout (verified in code): learnings at
`.uf/dewey/learnings/{tag}-{YYYYMMDDTHHMMSS}-{author}.md`
(`tools/learning.go:335`), compiled at `.uf/dewey/compiled/{tag}.md`
(`tools/compile.go:827`), knowledge stores at
`.uf/dewey/knowledge/{store}/` with custom paths resolved from
`knowledge-stores.yaml` (`curate/config.go:106`).

This repo's own `.gitignore` anchors the binary pattern as `/dewey`
(fixed in `3399cce`); the bare-`dewey` form of that bug still exists
in the wild and ignores `.uf/dewey/` artifact paths at any depth, so
the sweep detects it via `git check-ignore` and carries the one-line
fix into the chore PR when needed.

Constitution alignment (PASS x4) is assessed in the proposal. This
design reinforces it: the deterministic core is independently usable
(II), communicates through structured JSON with provenance (III), and
is tested against fixture repos with local bare remotes (IV).

## Goals / Non-Goals

### Goals

- Every knowledge artifact in the repository's truly-local git universe —
  every commit reachable from local branches or detached HEAD but not
  from any remote-tracking ref — ends up
  preserved on the remote default branch via one reviewable chore PR.
- Non-destructive: no stash pop/apply/drop, no working-tree mutation;
  `git status` and `git stash list` identical before/after.
- Deterministic planning: byte-identical plan JSON on an unchanged
  repo (twice-test), strictly local reads (no network in plan).
- Fail-closed conflicts, idempotent nothing-to-publish, DB-free
  operation that works while the server holds `dewey.lock`.
- Mechanical work extracted to the executable CLI core with tests;
  judgment (gate approval, collision choices) at the command layer,
  arriving as parameters; the PR-body listing tool-rendered by the
  core and relayed verbatim.

### Non-Goals

- Multi-repo sweep via `.uf/sibling-repos.yaml` (issue #106).
- Publishing user-owned configs (`sources.yaml`,
  `knowledge-stores.yaml`, `config.yaml`) — those are intentionally
  private runtime configuration.
- Anything requiring DB or MCP-server access.
- Modifying `dewey init`'s existing scaffolding mechanism (it gains
  one more command file, unchanged in behavior).
- Companion issues #133 (`/dewey-distill`) and #122 — separate
  changes (human-confirmed at orchestrator Phase 0).

## Decisions

### D1: Two-layer architecture — deterministic CLI core + judgment command layer

- `dewey artifacts plan|sync` (new `artifacts/` Go package): all
  mechanical work — enumeration, dedup, hashing, git choreography,
  commit-message templating, output rendering (DE-001, DE-003).
- `/dewey-publish` (`.opencode/commands/dewey-publish.md`, entry in
  the `deweySlashCommands` map in `slash_commands.go`): thin judgment
  layer. It owns the MANDATORY human gate between plan and sync, and
  passes judgment products as parameters: gate approval (proceed/
  abort, flowing through sync's `--approve`/`--approved-by`) and
  collision resolutions (chosen sha256 per path). The PR body's
  tag-grouped listing is deterministic and therefore tool-rendered by
  sync (`pr_body`), relayed verbatim per DE-003 with optional
  judgment framing around it (DE-005). The gate is the reason the
  sequence is NOT
  collapsed into a single intent-level tool: judgment sits between
  plan and sync, so the seam is plan → gate → sync-with-parameters.

Rejected: an MCP tool as the vehicle. The sweep must run while the
server holds the exclusive flock and when no server is running at all
(the common case for stranded artifacts). This follows the
`dewey init` / `dewey doctor` / `dewey manifest` operational-command
precedent; the agent-facing surface is the slash command.

### D2: Pure git-subprocess core, no store access

The `artifacts` package imports only standard-library packages plus
one internal dependency: the `sanitize` package (itself stdlib +
`charmbracelet/log` only) for injection-pattern screening;
credential detection is a stdlib-regexp pattern set defined inside
the `artifacts` package itself (no new internal or module
dependencies). It never
imports `store`, `curate`, `llm`, `embed`, or `source`; never opens
`graph.db`; never touches `dewey.lock`. Go compiles whole packages
(there are no per-file imports), so knowledge-store path resolution
deliberately does NOT reuse `curate`'s config loading inside
`artifacts` — `curate` transitively imports `store`, `llm`, and
`yaml.v3`, which would pull SQLite and the LLM clients into this
package's dependency closure. Instead the CLI layer (`cli.go`,
already linking the full dependency graph) resolves configured store
paths via `curate` and passes them into `artifacts` as explicit
parameters. No new module dependencies; no CGO. Rejected: libgit2
bindings (CGO / dependency rule).

### D3: Enumeration mechanics per stranded state

All commands run with the repo root resolved via
`git rev-parse --show-toplevel` (from cwd, or from `--vault`; the
flag is registered per-command on the artifacts group — `--vault`
is not a persistent root flag in this codebase, every existing
command registers its own). Fail closed when not a git repo or when
neither an `upstream` nor an `origin` remote exists. The vault root
(`--vault`, else `OBSIDIAN_VAULT_PATH`, else cwd) MUST be inside
the git repository — a vault outside the repo fails closed with an
actionable error naming both paths — because Dewey writes the
default artifact trio at `<vaultPath>/.uf/dewey/` (tools/learning.go),
which can diverge from the repository root when the vault is a
subdirectory.

1. **Working tree** — `filepath.WalkDir` over the three artifact
   directories resolved under BOTH the vault root and the repository
   root (deduplicated by repository-relative path when they overlap;
   the vault-anchored trio is where Dewey writes, the repo-root trio
   covers stray layouts) plus knowledge-store paths passed in as
   parameters from the CLI layer, validated to resolve inside the
   repository — a store path outside the repo, or carrying control
   characters, a colon, or a leading-hyphen component, fails closed
   with an actionable error — regardless of git tracked/ignored/staged
   state; skip dot-prefixed files (`.DS_Store` noise); regular files
   only — symbolic links and other non-regular types are skipped and
   surfaced as plan warnings, their target content never read or
   hashed; basenames containing control characters, `:` (ambiguous
   in `<rev>:<path>` revision syntax), or a leading `-` are skipped
   with warnings; hash content with sha256 — a read error during
   the walk or hashing (EACCES, EIO) fails closed with a wrapped
   error naming the path, never silently skipped. Missing
   directories are empty.
2. **Stash tracked parts** — `git stash list --format=%gd` for refs;
   `git ls-tree -r -z stash@{N}` (full format, NOT `--name-only` —
   entry mode must survive parsing) parsed as
   `<mode> <type> <sha>\t<path>` records, filtered to artifact
   prefixes matched at any depth; entries whose type is not `blob`
   or whose mode is `120000` (symbolic link) are skipped and warned
   — the regular-files-only rule enforced in tree sources, matching
   the working-tree walk; content via `git show stash@{N}:<path>`
   (byte-exact). Dot-prefixed and unsafe basenames (control
   characters, `:`, leading `-`) are skipped here and in steps 3-4 —
   the skip rule binds every enumeration source.
3. **Stash untracked parts** — probe `git rev-parse --verify --quiet
   stash@{N}^3` (present only for stashes created with `-u`/`-a`);
   then enumerate `stash@{N}^3` like (2). Never pop, apply, or drop.
4. **Unpushed commits** — `git rev-list --branches HEAD --not
   --remotes` (newest-first; `--branches` covers local branches and
   the explicit `HEAD` catches detached-HEAD-only work; stash refs
   are excluded here because stashes are enumerated by their own
   mechanism in steps 2-3, avoiding duplicate provenance, and
   tag-reachable unpushed content is out of scope — see Risks);
   for each commit, `git ls-tree -r -z <sha>` (full format, the same
   mode-aware parsing and skip rules as step 2) filtered to
   artifact prefixes; per path, take content from the newest touching
   commit via `git show <sha>:<path>`. Using per-commit `ls-tree`
   (rather than `diff-tree`) handles merge commits uniformly.

### D4: Identity, dedup, and base-tree comparison

- Dedup key: repository-relative path — path is the dedup,
  collision, conflict, and resolution key. Identity (filename for
  learnings and compiled articles; store path for knowledge-store
  content) is retained per entry as display and PR-grouping
  metadata only.
- Content equality: sha256 of content bytes.
- Base tree: `git ls-tree -r -z <base>` (mode-aware parsing, same
  skip rules as D3 step 2) filtered to the sanctioned root set (the
  default trio glob at any depth plus each configured store-path
  prefix); per-path content sha256 via `git show <base>:<path>`
  (batch through `git cat-file --batch` when profiling justifies it).
- Artifacts matching the base tree (same path, same sha256) are
  listed with status `unchanged` (observability) but excluded from
  the publish set. Same path + same content across sources
  collapses to one entry carrying all sources in provenance. A path
  whose content differs from its base tree entry is a plain `new`
  entry (updated content republished), never a collision —
  collisions are across stranded-state sources only.
- Publish remote resolution (deterministic, from local config): if a
  git remote named `upstream` exists it is the publish remote (base
  tree source and PR target — the fork layout this repository itself
  uses), else `origin`. The push remote is `origin` when the publish
  remote is `upstream` (contributors typically lack upstream push
  rights; the PR carries the fork head), else the publish remote.
  When `upstream` is the resolved publish remote, an `origin` remote
  MUST also exist (it is the push remote in fork layouts); its
  absence fails closed with an actionable error naming both remotes.
  Both names are recorded in the plan JSON (`repo.publish_remote`,
  `repo.push_remote`).
- Base ref resolution, strictly from local refs (plan is offline and
  deterministic): `git symbolic-ref --short
  refs/remotes/<publish-remote>/HEAD` → `<publish-remote>/main` →
  `<publish-remote>/master` (`git rev-parse --verify`) → fail closed
  with a fetch-directed error naming the resolved remote. The command
  layer runs `git remote set-head <publish-remote> --auto` after its
  fetch so `<publish-remote>/HEAD` is fresh.

### D5: Plan JSON contract and canonical serialization

Plan output is a versioned JSON document marshaled from ordered
structs (Go struct field order is deterministic); all slices sorted
(path order, source order); no timestamps, no temp paths, no
run-volatile data — the branch date is resolved at sync time, not in
the plan. Sketch (authoritative shape for the golden test):

```json
{
  "schema_version": 1,
  "repo": { "base_branch": "main", "base_sha": "<40-hex>",
            "publish_remote": "upstream", "push_remote": "origin",
            "artifact_roots": [".uf/dewey/compiled/", ".uf/dewey/knowledge/",
                               ".uf/dewey/learnings/", "<sorted store paths>"],
            "stash_refs": { "stash@{0}": "<40-hex>", "stash@{0}^3": "<40-hex>" } },
  "gitignore": { "status": "ok|bare-dewey-fix|other-ignore-warning|ignore-source-external",
                 "fix": { "file": ".gitignore",
                          "anchor": { "from": "dewey", "to": "/dewey" },
                          "reignore": [".uf/dewey/graph.db", ".uf/dewey/graph.db-shm",
                                       ".uf/dewey/graph.db-wal", ".uf/dewey/dewey.log",
                                       ".uf/dewey/dewey.lock", ".uf/dewey/cache/",
                                       ".uf/dewey/config.yaml", ".uf/dewey/sources.yaml",
                                       ".uf/dewey/knowledge-stores.yaml"] } },
  "artifacts": [
    { "path": ".uf/dewey/learnings/auth-20260401T101112-alice.md",
      "identity": "auth-20260401T101112-alice", "tag": "auth", "kind": "learning",
      "sha256": "<64-hex>", "sources": ["working-tree", "stash:stash@{0}"],
      "status": "new", "findings": [] }
  ],
  "conflicts": [
    { "path": "...", "identity": "...",
      "variants": [ { "sha256": "<64-hex>", "sources": ["working-tree"], "findings": [] },
                    { "sha256": "<64-hex>", "sources": ["stash:stash@{1}"], "findings": [] } ] }
  ],
  "blocked": [
    { "path": "...", "identity": "...",
      "findings": [ { "scanner": "injection|credential", "pattern": "...",
                      "severity": "critical", "line": 3, "message": "..." } ] }
  ],
  "warnings": [ "skipped symlink: .uf/dewey/learnings/link.md" ],
  "summary": { "total": 12, "new": 10, "unchanged": 2, "conflicts": 1, "blocked": 0 },
  "rendered_summary": "<tool-rendered human summary, relayed verbatim by the command layer>"
}
```

Vocabulary (authoritative, pinned by the golden test):

- `sources[]` labels: `working-tree`; `stash:stash@{N}` (tracked
  part); `stash-untracked:stash@{N}` (the `^3` untracked part);
  `commit:<40-hex-sha>` (unpushed commit).
- `kind`: `learning` | `compiled` | `knowledge`.
- `status`: `new` | `unchanged`.
- `gitignore.status`: `ok` | `bare-dewey-fix` |
  `other-ignore-warning` | `ignore-source-external` (bare `dewey`
  matched, but the pattern's source file is outside the repository —
  fix skipped, warning surfaced). The `gitignore.fix` object is
  present if and only if status is `bare-dewey-fix`, omitted
  otherwise (never null), and its `file` value is always the literal
  `.gitignore`. The fix is a bounded block: the `anchor` rewrites the
  exact bare `dewey` line to `/dewey`, and the `reignore` list
  appends explicit re-ignore lines for non-publishable runtime state
  and private configuration, so anchoring never silently un-ignores
  `.uf/dewey/graph.db`, `.uf/dewey/graph.db-shm`,
  `.uf/dewey/graph.db-wal`, `.uf/dewey/dewey.log`,
  `.uf/dewey/dewey.lock`, `.uf/dewey/cache/`,
  `.uf/dewey/config.yaml`, `.uf/dewey/sources.yaml`, or
  `.uf/dewey/knowledge-stores.yaml` post-merge.
- `repo.artifact_roots`: the sanctioned artifact roots (three
  defaults plus store paths), sorted — informational; sync re-derives
  the set and cross-checks it (D7 step 1).
- `repo.stash_refs`: every enumerated stash label mapped to its
  resolved 40-hex commit sha (`stash@{N}` and `stash@{N}^3`) —
  recorded because labels renumber when stashes are pushed or
  dropped; sync fetches stash content by sha.
- Screening findings (in `blocked[]`, per-variant in
  `conflicts[].variants[].findings`, and non-blocking
  lower-severity findings on `artifacts[].findings`):
  `{scanner, pattern, severity, line, message}` — `scanner` is
  `injection` or `credential`; the
  sanitize `Context` field (matched-content excerpt) is deliberately
  excluded so blocked findings never re-deliver payload into the
  plan JSON or the gate. Variants carrying `critical`/`high`
  findings are blocked variants — never offered as or accepted as
  resolution choices; a path whose every variant is blocked appears
  only in `blocked`. Artifacts carrying only lower-severity findings
  remain publishable, with the findings surfaced in the summary.
- `summary` arithmetic: `total == len(artifacts) == new + unchanged`;
  `conflicts == len(conflicts)`; `blocked == len(blocked)`.
  Conflicted and blocked paths appear only in their own arrays,
  never in `artifacts`; the conflicts array holds only paths with at
  least one clean variant.
- `rendered_summary`: the tool-rendered human summary string —
  itemized tag-grouped listing (path, kind, sha256), conflicts with
  per-variant findings and bounded diffs, blocked entries, gitignore
  finding, warnings — rendered by the core, carried in the plan
  JSON, and relayed verbatim by the command layer (DE-003);
  distinct from the arithmetic `summary` object.

The plan JSON carries `rendered_summary` — the tool-rendered human
summary (itemized tag-grouped artifact listing with path, kind, and
sha256; conflicts with per-variant findings and bounded unified
diffs; blocked entries; gitignore finding; warnings) — rendered by
the core so the command layer relays it verbatim rather than
re-rendering (DE-003); `dewey artifacts plan` (without `--json`)
prints the same string to stdout. Sync output is similarly structured
JSON: `{ "branch", "worktree", "commit", "files_written",
"gitignore_fix_applied", "nothing_to_publish", "pr_body",
"approved_by", "plan_sha256" }` (`plan_sha256` binds the sync result
to the exact plan content executed). `pr_body` is the tool-rendered
PR-body markdown —
artifact identities grouped under sorted tags, per-kind counts, the
non-destructive note, and the optional gitignore fix — pinned by
golden test and relayed verbatim by the command layer (DE-003),
which may surround it with judgment framing.

### D6: Collision handling — fail closed, resolutions as parameters

Conflicts list every variant keyed by sha256 + source, each variant
carrying its screening findings (blocked variants are excluded from
resolution choices — a resolution naming one fails closed). Sync
refuses (exit non-zero, no branch/commit/worktree) while any
conflict lacks a resolution. Resolutions arrive via `--resolutions
file>` — a JSON object keyed by repository-relative path
(path-keying is the canonical schema, strict by construction —
path is the dedup/collision/conflict key throughout; the spec, this
design, and task 6.1's input-schema parity all use it):

```json
{ ".uf/dewey/learnings/auth-20260401T101112-alice.md": "<chosen-sha256>" }
```

Sync validates each resolution against the plan's conflict variants
(an unknown path, a sha256 that is not a variant of that path's
conflict, or a sha256 naming a blocked variant fails closed) and
publishes the chosen variant's content,
fetched from its recorded source.

### D7: Sync mechanics — OS-tempdir git worktree

1. **Validate and approve (fail closed, before any effect)** —
   refuse without `--approve` (exit non-zero; no branch, commit, or
   worktree); structurally validate the plan: supported
   `schema_version`, `repo.base_sha` a 40-hex sha, every
   `repo.stash_refs` value 40-hex, every `sources[]` label matching
   the vocabulary (`working-tree`, `stash:stash@{N}`,
   `stash-untracked:stash@{N}`, `commit:<40-hex>`), every sha256
   field (artifact content, conflict variant, resolution target)
   64-hex, `--approved-by` printable-only with no newlines or
   control characters (a newline could forge additional commit
   trailers, e.g. a second `Plan-sha256:`), and every
   artifact/conflict/blocked path clean, repository-relative,
   `..`-free, free of control characters, colons, and
   leading-hyphen components, and confined to the sanctioned
   artifact directories —
   a set sync RE-DERIVES at execution time (the three defaults
   under the vault root and the repository root, plus store paths
   re-resolved from `knowledge-stores.yaml` via the CLI layer);
   the re-derived set is the confinement authority, and the plan's
   recorded `repo.artifact_roots` are informational, cross-checked
   against it — mismatch fails closed. A recorded
   `gitignore.fix.file` other than the literal `.gitignore` fails
   closed. Sync also computes the consumed plan file's sha256
   (`plan_sha256`) for the audit trail. Any violation exits
   non-zero before any worktree, branch, or file exists.
2. `os.MkdirTemp("", "dewey-artifacts-")`, then
   `git worktree add --detach <dir> <base_sha>`. The worktree lives
   under the OS tempdir — outside the vault and outside every
   configured disk source path (a live server's watcher never sees
   it). Worktree registration does not affect `git status` or the
   stash stack.
3. `git checkout -b chore/dewey-artifacts-YYYYMMDD` (sync-time date;
   first free numeric suffix `-2`, `-3`, … on same-day collision).
4. Write exactly the planned files. BEFORE writing, scan each
   planned destination path and its ancestor directories within the
   checked-out worktree for symbolic links — the remote base tree
   can carry them, and a write through a symlinked destination
   escapes the worktree; a symlink found fails closed naming the
   path. Then write (mkdir -p + bytes from the
   recorded source: filesystem copy for working-tree entries,
   `git show <sha>:<path>` for stash/commit entries — stash content
   fetched by the recorded commit sha from `repo.stash_refs`, never
   the mutable `stash@{N}` label; a recorded sha missing from the
   object store (`git cat-file -e <sha>^{commit}`) fails closed
   with "re-run plan"). Before writing working-tree-sourced
   content, re-verify the source is still a regular file (a symlink
   swapped in since plan time fails closed) and that its sha256
   still matches the plan. After writing EVERY planned file — any
   source — re-compute its sha256 in the worktree and fail closed
   on any mismatch with the plan (stale plan).
5. Apply the flagged gitignore fix block in the worktree — replace
   the exact bare `dewey` line with `/dewey` AND append the
   runtime-state re-ignore lines from the plan's `reignore` list
   (graph.db, graph.db-shm, graph.db-wal, dewey.log, dewey.lock,
   cache/, config.yaml, sources.yaml, knowledge-stores.yaml) so the
   anchor does not
   silently un-ignore non-publishable state post-merge — only when
   the `git check-ignore`-identified source file is present in the
   worktree (a tracked, in-repo `.gitignore`); when the pattern's
   source is outside the repository, skip the fix and keep the
   warning (D8).
6. `git add -f -- <artifact paths>` (+ the `.gitignore` when fixed).
   `-f` is load-bearing, not defensive: repos that ignore `.uf/`
   wholesale are the most common stranded state, and tracked files
   legitimately override gitignore. The plan's `other-ignore-warning`
   surfaces this at the gate for the human to see. Then verify the
   staged set (`git diff --cached --name-only`) equals the planned
   set exactly — mismatch fails closed.
7. Commit with a tool-rendered Conventional Commit message
   (deterministic template: `chore(dewey): publish N knowledge
   artifacts` + body with per-kind counts and sorted tag list +
   non-destructive note + `Approved-by: <identity>` trailer and a
   `Plan-sha256: <hex>` trailer binding the commit to the exact
   plan content executed; the approver comes from `--approved-by`,
   falling back to `git config user.name`). Rendered strings —
   identity, tag, approver — are control-character-stripped and
   markdown-escaped wherever they flow into rendered output
   (commit message, pr_body, summary).
8. Leave the worktree in place and report its path, branch, and
   commit in the sync JSON (push/PR belong to the command layer; the
   branch ref is shared with the main repo through the worktree
   mechanism, so `git push <push-remote> <branch>` works from the
   repo). The command layer removes the worktree (`git worktree
   remove <path>`) after the PR; failure paths report the path for
   manual cleanup (`git worktree prune` recovers registrations).
9. Empty publish set: sync exits 0 with `nothing_to_publish: true`,
   creating nothing (idempotence).

Rejected: committing in the user's checkout (mutates the working
tree — violates the core property). Rejected: pure plumbing
(`hash-object`/`commit-tree` without a checkout) — fewer moving parts
in theory, but harder to inspect and test, and the worktree gives the
round-trip verification surface (`git show` in the temp worktree) the
acceptance criteria ask for.

### D8: Gitignore bug detection

`git check-ignore -v <probe-path>` with one synthetic probe per
sanctioned artifact root — a path under `.uf/dewey/learnings/`,
`.uf/dewey/compiled/`, `.uf/dewey/knowledge/`, and each configured
store path (the paths need not exist). All probes exit 1 → `ok`.
A probe exiting 0 → parse its `<source>:<line>:<pattern>`: pattern
exactly `dewey`
(unanchored) → `bare-dewey-fix` flagged with the bounded fix block
(`dewey` → `/dewey` anchor plus the runtime-state re-ignore list,
in the named file; anchoring alone would un-ignore the whole
`.uf/dewey/` tree post-merge — the block keeps runtime state and
private configs ignored, a consequence the plan surfaces); any
other pattern →
`other-ignore-warning` surfaced at the gate (sync's `git add -f`
still functions; the human decides). Two input-domain facts are
specified rather than left to the implementer: (a) `check-ignore -v`
reports only the LAST matching pattern, so a wholesale `.uf/` ignore
coexisting with the bare `dewey` line reports `.uf/` and classifies
as `other-ignore-warning` — documented degradation, acceptable
because `git add -f` is load-bearing either way; (b) the source file
can lie outside the repository — the user's global excludesfile
(`~/.config/git/ignore`) or `.git/info/exclude`. In that case the
status is `ignore-source-external`: the fix is skipped (there is no
in-repo file to edit in the chore branch), the plan surfaces a
warning naming the external source, and the sweep does not error.

### D9: Command-layer flow (`/dewey-publish`)

`.opencode/commands/dewey-publish.md` (entry in `deweySlashCommands`,
`<protect>`-wrapped Instructions per the DCP precedent), committed
also to this repo's `.opencode/commands/`:

1. **Fetch base** — resolve the publish remote (`upstream` when
   configured, else `origin`; the plan JSON records it), fetch it
   via the uf `git_fetch` tool (fallback: `git fetch <remote>`),
   then `git remote set-head <publish-remote> --auto` to refresh its
   `HEAD`.
2. **Plan** — run `dewey artifacts plan --json` via the shell; save
   the JSON to a temp file created with 0600 permissions (it carries
   artifact paths, identities, and sha256 digests — repository
   layout metadata worth keeping private, though no content
   previews); relay the plan JSON's `rendered_summary` verbatim.
3. **Present + MANDATORY GATE** — the resolved publish destination
   (publish remote name, fetch URL, target repository and branch)
   BEFORE approval is requested; then the plan's tool-rendered
   human summary relayed verbatim: the itemized tag-grouped
   artifact listing (path, kind, sha256 — not aggregates alone),
   the conflict list with sha256-identified variants and their
   per-variant screening findings, each conflict's core-rendered
   bounded unified diff between clean variants (deterministic,
   capped at 100 lines with lines truncated at 4 KiB and the total
   at 16 KiB, each bound with an explicit elision or truncation
   marker — untrusted content rendered for the resolution choice,
   never interpreted), blocked
   entries with their findings, and the gitignore finding (rendered
   strings — paths, identities, tags, store names —
   control-character-stripped and markdown-escaped by the core
   renderers). Wait for explicit approval. Collisions require the user to choose
   among clean variants only; write the path-keyed resolutions
   file from their choices. Abort on decline (nothing runs).
4. **Sync** — `dewey artifacts sync --plan <file> --approve
   --approved-by <user> [--resolutions <file>]`; relay the sync JSON
   (branch, commit, worktree, pr_body).
5. **Push** — push the chore branch to the push remote (`origin` in
   fork layouts, else the publish remote) via uf `git_push`
   (fallback: `git push -u <push-remote> <branch>`).
6. **PR** — `uf_finale_create_pr` targeting the publish remote's
   default branch (fallback: `gh pr create --repo <publish-remote>
   --base <default>`, with `--head <fork>:<branch>` in fork
   layouts). Title: `chore(dewey): publish knowledge artifacts`.
   Body: relay the sync core's tool-rendered `pr_body` verbatim
   (tag-grouped identities, per-kind counts, non-destructive note,
   optional gitignore fix — DE-003, pinned by golden test); judgment
   framing may surround the relayed fragment.
7. **Cleanup** — `git worktree remove <worktree path>` via shell.
8. **Pickup note** — merged artifacts are picked up after merge →
   pull → Dewey server restart (learnings re-ingestion runs at serve
   startup via `reIngestLearnings`).

### D10: Test strategy — fixture repos, local bare remotes

All tests construct fixture repositories in `t.TempDir()` with a
local bare repo as `origin` (init → bare clone → push base), so
base-tree dedup and `--not --remotes` behave exactly as against a
real remote, offline. The suite covers: the four stranded states
(each with a dedicated fixture); zero-mutation assertions
(`git status --porcelain` + `git stash list` captured before/after
the full flow); the twice-test (byte-compare two plan runs); the
round trip (plan → sync → `git show` of every artifact in the temp
worktree, content matching planned sha256; commit file set ==
planned set + optional fix); collisions (flagged; sync refuses;
resolutions unblock); idempotence (nothing-to-publish, nothing
created); gitignore fixtures (bare `dewey` → fix applied; `/dewey`
→ not flagged); missing artifact dirs; custom store path; lock-held
(an `flock` held by the test process during the full flow); the
walk-root security fixtures (a symlinked `.uf` component — and a
symlinked intermediate walk-root component — both fail closed; a
symlinked destination path inside the checked-out base tree fails
closed; an unreadable artifact directory fails closed); malformed-plan validation (non-hex `repo.stash_refs`
values, vocabulary-violating `sources[]` labels, non-64-hex sha256
fields, a newline-bearing `--approved-by` forging a trailer — all
fail closed); and
`dewey init` scaffolding of the new command file
(`cli_test.go` precedent at line 751).

### Determinism governance mapping (`.opencode/uf/packs/determinism.md`)

- **DE-001 [MUST]** — enumeration, dedup, hashing, content
  screening (both scanners), path/structural plan validation, git
  choreography, commit-message templating (including the
  `Approved-by` and `Plan-sha256` trailers),
  PR-body rendering, conflict diff rendering (bounded, elided,
  deterministic), human-summary itemized artifact listing, and
  output rendering live in the `artifacts`
  package with tests; nothing mechanical is instructed in prose.
- **DE-003 [MUST]** — plan summary (the plan JSON's
  `rendered_summary`, including the itemized tag-grouped artifact
  listing and the per-conflict bounded unified diffs), plan/sync
  JSON, the commit
  message, and the PR-body fragment (`pr_body`) are tool-rendered;
  the command layer relays them verbatim and never generates its
  own rendering of mechanical output.
- **DE-005 [MUST]** — judgment products (gate approval, flowing
  through `--approve`/`--approved-by`; path-keyed collision
  resolutions) arrive as parameters at the sync seam. The PR body's
  tag-grouped listing is deterministic and therefore tool-rendered
  (DE-001/DE-003); only judgment framing around the relayed fragment
  remains a command-layer product. The human gate between plan and
  sync is the judgment step that keeps the orchestration legitimately
  in the command layer.
- **DE-007 [MUST]** — tasks.md carries seam-verification tasks
  naming the three layers: registration surface (cobra command group
  + flags), documented output shape (golden plan/sync JSON), and
  write→read round-trips (plan file → sync; sync commit → `git show`).
- **DE-008 [MUST]** — apply-phase delegations of deterministic tasks
  carry verified skill/script references through the delegation-tool
  parameter contract, not prose.

## Risks / Trade-offs

- **Stashes without a `^3` parent** (created without `-u`): probed
  with `rev-parse --verify --quiet` and skipped — no error, no
  false empties.
- **`git add -f` against wholesale-ignored `.uf/`**: intentional and
  load-bearing (that ignore state is the most common stranding);
  surfaced as a gate warning rather than silently applied.
- **Worktree left behind on failure**: path always reported; command
  doc has an explicit cleanup step; `git worktree prune` recovers
  stale registrations. Worktree add/remove provably does not touch
  `git status` or the stash stack (asserted in tests).
- **Subprocess volume on long unpushed histories**: per-commit
  `ls-tree` is one subprocess per unpushed commit; acceptable at
  realistic scales, with `git cat-file --batch` noted as the
  optimization path if profiling demands it.
- **Stale plans** (repository changed between plan and sync): every
  source is sha256-re-verified — working-tree sources before the
  copy (regular-file check included) and every written file after
  materialization. Commit sources are immutable (sha-addressed);
  stash LABELS are mutable reflog positions (they renumber when
  stashes are pushed or dropped), so the plan records resolved
  stash commit shas (`repo.stash_refs`) and sync fetches by sha,
  failing closed when a recorded sha has left the object store.
  Mismatch fails closed.
- **Enumeration scope is conservative**: `--not --remotes` excludes
  commits reachable from ANY remote-tracking ref. Fork-branch commits
  pushed to a non-publish remote and stale remote-tracking refs are
  documented non-coverage — the sweep targets truly-local commits and
  the boundary is stated in the goal.
- **Same-day branch-name collision**: deterministic numeric suffix.
- **Plan determinism vs. remote drift**: plan reads only local refs;
  the command layer's fetch + `set-head` step is what keeps the local
  view fresh, keeping the twice-test well-defined on an unchanged
  repository.
- **Concurrent server writes during the sweep**: the server may add
  new learnings mid-sweep; those are simply absent from this plan
  and picked up by the next run (idempotent by design).
- **Git index residence (known non-coverage)**: two index-resident
  states are out of scope — a file staged then deleted from disk
  (it exists only in the live index, not on disk and not in any
  commit) and the stash index part `stash@{N}^2` (a staged variant
  that differed from the stashed working-tree state). Both are rare
  for Dewey-written files and documented as a boundary; index
  enumeration (`git ls-tree` on the live index and `stash@{N}^2`) is
  the explicitly scoped follow-up if real-world strandings appear.
- **Tag-reachable unpushed content (known non-coverage)**: commits
  reachable only from tags are not swept — the scope is local
  branches plus detached HEAD. Documented boundary, not an
  oversight.
