---
description: >
  Execute a prioritized work plan through the full Unbound
  Force pipeline. Each work item is executed through two
  uf_delegate_command delegations (/uf.unleash, then
  /uf.finale) with paused-exit handling and per-item
  todowrite tracking.
---
<!-- scaffolded by uf vdev -->

# Command: /uf.run-pipeline-plan

## User Input

```text
$ARGUMENTS
```

## Description

Orchestrate a prioritized work plan through the full
Unbound Force pipeline. Each work item is executed
through two sequential `uf_delegate_command`
delegations:

1. **Delegation 1** (`/uf.unleash`): delegated with
   `command_path: ".opencode/commands/uf.unleash.md"`
   and a prompt carrying the work item's description
   and issue reference(s). The child session resolves
   the issue, routes the workflow tier, runs the
   tier's entry command, and executes the pipeline
   through the demo step.
2. **Delegation 2** (`/uf.finale`): delegated with
   `command_path: ".opencode/commands/uf.finale.md"` and
   a prompt carrying the work item's context, only after
   the demo completes. The child session drives staging
   and commit preparation, then hands off at its
   commit-approval gate; the human completes the finale
   interactively.

`/uf.unleash` paused exits are recognized, relayed
verbatim, and block the work item.
Progress is tracked per item via `todowrite`.

Supports two input modes: a plan file path or space-
separated GitHub issue references (`#NNN` or URLs).

## Usage

```
/uf.run-pipeline-plan sprint-42.md
/uf.run-pipeline-plan #123 #456 #789
/uf.run-pipeline-plan https://github.com/unbound-force/unbound-force/issues/42 #99
```

## Instructions

<protect>

> **SESSION-RESUME GUARD**: When resuming after context
> compression or a session restart, re-read this entire
> template before continuing. Compressed context summaries
> MUST NOT determine item completion. Only `todowrite`
> entries and disk state (branches, spec artifacts, PRs)
> are authoritative for resumability.

### Execution Checklist

Maintain this checklist using the `todowrite` tool: one
entry per work item. Entry completion follows the rule
in the Tracking section.

### Phase 0 — Input Resolution and Confirmation

Phase 0 runs in the orchestrator context (no subagents).

1. **Detect input type**: Determine whether `$ARGUMENTS`
   is an existing file path.
   - File path: PROCEED to the Parse Plan File step.
   - Otherwise: PROCEED to the Resolve Issue Metadata
     step. Treat the input as space-separated GitHub
     issue references (`#NNN` or full GitHub URLs).

2. **Resolve issue metadata** (issue-ref input only):
   For each issue reference, run:

   ```bash
   gh issue view <N> --json title,body,number
   ```

   **On failure**: STOP and report the `gh` error.
   From each issue, record a kebab-case change name
   (from the title) and a one-line description (from
   the body).

3. **Parse plan file** (file input only): Read the plan
   file line by line. Each numbered item MUST carry the
   fields defined in the Plan File Contract.

4. **Flag non-mechanical items**: ASK the human to
   resolve every item tagged `[INVESTIGATION]` or
   `[POLICY]`.

   | Resolution | Meaning |
   |---|---|
   | remove | Drop the item from this run |
   | convert to mechanical | Rewrite the item as a mechanical work item |
   | defer | Postpone the item to a later plan |

   **On failure**: none (the question tool blocks until
   answered). The pipeline MUST NOT auto-execute flagged
   items.

5. **Present work-item table**: ASK the human to confirm
   the full ordered work-item list. Include issue refs,
   change names, and descriptions.
   - Plan-file input: group items as the plan file
     defines.
   - Bare issue list: ASK how to group issues into work
     items (default: one PR per issue).

   **On failure**: none (the question tool blocks until
   answered). The pipeline MUST NOT PROCEED before the
   human confirms.

**Sibling repository references**: Before filesystem or
web search for cross-repo reference material, check
`.uf/sibling-repos.yaml`. When the repo declares siblings,
use `acquire_sibling_evidence` (or read the in-repo
`.uf/sibling-clones/<name>/` or `.uf/sibling-cache/<name>/`
copies) for their contract files. NEVER read sibling
repositories from outside-workspace paths — out-of-workspace
reads trigger permission asks that wedge parented child
sessions (see Direct Human Gate Surfacing).

### Plan File Contract

A plan file is a numbered markdown list. Each item
carries:

```
1. #42 Change: add-run-plan-command — Create /uf.run-pipeline-plan slash command
2. #99 Change: fix-scaffold-drift — Sync embedded asset copies
3. #55 [INVESTIGATION] Investigate CI flakiness in TestEmbeddedAssets
```

- Issue refs: `#NNN` (one or more per item)
- Change name: `Change: <kebab-case-name>`
- Description: text after the `—` separator (optional)
- Non-mechanical tag: `[INVESTIGATION]` or `[POLICY]`

### Per-Item Pipeline Protocol

For each confirmed work item, execute two sequential
delegations. Both run through `uf_delegate_command` in
parented child sessions:

1. **Delegation 1** (`/uf.unleash`): DELEGATE with
   `command_path: ".opencode/commands/uf.unleash.md"`
   and a prompt carrying the work item's description
   and issue reference(s). The child resolves the issue
   metadata, routes the workflow tier, runs the tier's
   entry command, and executes the pipeline through the
   demo step. Omit `task_classification` (command-path
   delegation), `checkpoint_step`, and
   `expected_artifact` — the delegated command owns its
   durable progress state. Set `timeout: 1800000` (the
   `uf_delegate_command` maximum); whole-pipeline runs
   have hit this cap (learning
   `uf-workflow-20261026T182050`), so a timeout routes
   through gate-timeout recovery below rather than a
   hard failure.

2. **Delegation 2** (`/uf.finale`): DELEGATE with
   `command_path: ".opencode/commands/uf.finale.md"` and a
   prompt carrying the work item's context (title,
   description, issue reference(s), and that the PR
   targets the parent `unbound-force/unbound-force` repo),
   only after the `/uf.unleash` run presents the demo
   block. `/uf.finale` runs nine steps: branch safety
   gate, check for changes and stage, commit message,
   sync with the base branch, push, create or find the
   PR, watch CI checks, return to `main`, and summary.
   Expected outcome: a designed handoff at the
   commit-approval gate — the child drives staging and
   commit preparation, then wedges at its
   commit-approval question gate (question prompts do
   not surface from a parented child; see Direct Human
   Gate Surfacing). The human completes `/uf.finale`
   interactively on the feature branch from the child's
   checkpoints. The orchestrator then verifies the
   outcome on disk: a PR URL; the PR stays open for
   human review; `Closes #N` threads via
   `originating_issue` in `.openspec.yaml`; the tree
   returns to `main`. Set `timeout: 600000` (10 minutes):
   the child's work before the commit-approval gate is
   bounded (branch gate, staging, commit message prep),
   and the wedge at that gate is the designed handoff —
   the timeout bounds the orchestrator's wait before
   gate-timeout recovery engages. Omit
   `task_classification`, `checkpoint_step`, and
   `expected_artifact` as above.

**Disk handoff verification**: VERIFY outcomes on disk
at each boundary:

| Boundary | VERIFY on disk |
|---|---|
| Before `/uf.finale` | The working tree is on the item's feature branch (not `main`), and any untracked `internal/scaffold/AGENTS.md` has been removed (Stray file removal) |
| After `/uf.finale` | A PR exists for the branch; the tree is back on `main` and clean |

**On failure**: route the item through the blocked-item
protocol (Unleash Paused-Exit Handling below) and report
the missing outcome.

**Clean tree between items**: After each item completes
(PR created), VERIFY the return to `main` with a clean
working tree. `git status --short` MUST be empty before
the next item starts.

**On failure**: STOP and report the dirty tree.

### Direct Human Gate Surfacing

Tool-rendered paused exits inside delegated commands
surface to the human: the child returns its exit text
and the orchestrator relays it verbatim (Unleash
Paused-Exit Handling below). Question-tool prompts
inside a delegated child do NOT surface — the parented
child session cannot carry them to the human, and the
delegation wedges at the gate until its timeout. The
orchestrator MUST NOT relay gates via structured values,
MUST NOT resume children via `task_id`, and MUST NOT
instruct children to avoid the question tool.

**Gate-timeout recovery**: when a delegation crosses a
mandatory question gate (for example `/uf.unleash`
spec-review or `/uf.finale` commit approval) and wedges
until timeout, the human either clears the gate
interactively on the feature branch (re-entering the
child command in a direct session) before re-delegating,
or the orchestrator re-anchors via `get_progress`,
`parse_tasks`, and `git status` and drives the remaining
steps. Direct-drive is bounded to mechanical steps: any
gate requiring consent — committing, pushing, or opening
a PR — MUST be answered by the human through a
pause-relay question before the orchestrator executes
the consented step; the orchestrator MUST NOT commit,
push, or open a PR on its own authority. Re-delegating
without clearing the gate re-hangs at the same gate.

The orchestrator's own `question` tool use is limited
to Phase 0 triage (non-mechanical item resolution and
the work-item confirmation table) and pause-relay
decisions — `/uf.unleash` paused exits and delegation
failures (the blocked-item question below) and
non-green CI on a created PR (see Tracking). All other
human interaction happens inside the delegated commands'
own gate protocols — surfaced to the orchestrator as
paused exits or timeouts, not as live questions.

### Unleash Paused-Exit Handling

`/uf.unleash` may exit paused, requiring human action
before the item can continue. Recognize any
`/uf.unleash paused at:` heading; the variant named in
the heading selects the re-run class in step 4 below.

On a paused exit — or a hard delegation failure (an
error envelope carrying neither exit text nor a demo
block):

1. **Relay**: emit the exit text verbatim.
2. **Block**: mark the todowrite entry `in_progress`
   with a `[BLOCKED: <variant>]` annotation.
3. **Ask**: present the pause-relay question — re-run
   the item after human action, skip the item, or
   abort the run. When the wedge is a consent gate
   (for example the finale commit approval), the
   question relays the pending consent decision and
   the orchestrator completes only the consented
   mechanical steps.
4. **On re-run**: delegate `/uf.unleash` again with the
   same `command_path` and no routing context — that is,
   omitting orchestrator-side tier and branch directives
   while the prompt still carries the work item's
   description and issue reference(s). Split by variant
   class (the orchestrator holds the variant from the
   `[BLOCKED:]` annotation):
   - **Mid-pipeline variants** (clarify, spec-review,
     implement-worker-failed, implement-merge-conflict,
     implement-phase-checkpoint, code-review-preflight,
     code-review-no-success,
     code-review-iterations-exhausted, planner-failure):
     the feature branch and its durable state exist; the
     child re-anchors and resumes.
   - **Tier-routing variants** (tier-routing-dirty-tree,
     tier-routing-missing-tooling): the exit fired inside
     `uf_unleash_route_decision` before any branch or
     durable state exists; the re-run is a fresh restart
     on `main` whose delegation prompt MUST re-carry the
     description and issue reference(s) — without them
     the child wedges asking what to build.
5. **Finale**: delegate `/uf.finale` only after a run
   completes and presents the demo block. Orchestrated
   runs supersede the demo block's interactive
   next-step choice; its two options address
   interactive runs.

The remaining human control points are the Phase 0
work-item table, the delegated commands' own gates
(surfaced as paused exits or gate-timeout wedges),
pause-relay questions, and non-green CI on a created
PR (see Tracking).

### Guardrails

These rules are non-negotiable:

- **Scaffold drift sync**: Any file under
  `.opencode/commands/` or `openspec/` that changes
  MUST have its `internal/scaffold/assets/...` copy
  synced byte-identical, or `TestEmbeddedAssets_MatchSource`
  fails.
- **Explicit staging**: The orchestrator stages and
  commits nothing — staging happens inside the
  delegated `/uf.finale` child via `uf_finale_stage`.
  The orchestrator-side rule is verification only; if
  it ever stages, it MUST use explicit file paths.
  At the orchestrator level, `git add -A` and
  `git add .` are forbidden.
- **Stray file removal**: Remove any untracked
  `internal/scaffold/AGENTS.md` before staging. Never
  commit it.
- **No force-push**: `git push --force` is forbidden.
  Only `git push --force-with-lease` is allowed.
- **PR targeting**: PRs MUST target the parent
  `unbound-force/unbound-force` repo with head
  `jflowers:<branch>`.
- **Closes-linkage backstop**: `/uf.finale` threads
  `Closes #<issue>` into the PR body from
  `originating_issue` in `.openspec.yaml` (recorded by
  the tier's entry command from the route decision's
  issue number when routed from an issue). After the
  PR is created, VERIFY the PR body contains
  `Closes #<issue>` for each issue in the work item via
  `gh pr view <PR number> --json body`; append missing
  lines via `gh pr edit <PR number>` and report the
  repair.
- **No auto-merge**: The PR MUST NOT be merged
  automatically. It stays open for human review.

### Tracking

Maintain `todowrite` with one entry per work item:

```
- [ ] Item 1: #42 add-run-plan-command
- [ ] Item 2: #99 fix-scaffold-drift
- [ ] Item 3: #55 [INVESTIGATION] CI flakiness
```

Mark an entry complete (`[x]`) only after both hold:
1. The item's PR has been created
2. `gh pr checks <PR number>` is green

Blocked items stay `in_progress` with the paused
variant annotated. On non-green CI, keep the entry
`in_progress`, report the failing checks, and ask the
human whether to re-run `/uf.unleash` on the feature
branch, leave the item blocked, or continue with the
next item.

Entries MUST NOT be marked complete prematurely. The
`todowrite` tracker is the source of truth for item
completion, not the orchestrator's context.

</protect>
