---
description: Weekly unattended distill — deterministic scope, theme grouping, and rolling per-theme PRs via MCP server.
agent: distiller
---

# Command: /dewey-distill-auto

## Description

Unattended weekly pipeline for draft curation. Computes deterministic scope from
git (`learnings - cited - policy exclusions`), groups uncited learnings into
themes (must-colocate supersessions, embedding/tag assignment with provenance),
and manages rolling per-theme PRs (`dewey-distill/<sanitized>` branch, draft
`tier: draft` only, review-queue body). No `HUMAN_GATE` questions; no validated
writes. Triggered via scheduled workflow dispatch (`/dewey-distill-auto #N`).

Uses MCP tools exclusively, with two documented exceptions: the mandatory startup
gates run `dewey gate auto-answer` and `dewey gate distill-unattended`, which
never touch the store. While the server runs it holds an exclusive lock on
`.uf/dewey/dewey.lock`.

## Usage

```
/dewey-distill-auto
/dewey-distill-auto --dry-run
/dewey-distill-auto --backlog-from 2026-10-25 --dry-run
/dewey-distill-auto --backlog-from 2026-10-25
```

Flags (parsed from the invocation text before any pipeline stage):

- `--dry-run` — output-only preview: run scope and grouping normally,
  then pass `dry_run: true` to the `rolling_pr` tool and stop after its
  preview result. No branch is created, no article is written, no PR is
  opened or edited, and no run artifact is recorded — the preview result
  is the run's entire output. The preview still enforces the
  clean-tracked-tree precondition (the real run would fail on a dirty
  tree), so a passing preview means an executable run.
- `--backlog-from YYYY-MM-DD` — one-time backlog bootstrap (FR-020):
  pass `backlog_from: "YYYY-MM-DD"` to the `scope` tool so only learning
  identities created on or before that date (the accumulated backlog
  partition; legacy identities without a timestamp always pass) enter
  scope. The weekly steady state handles everything created after the
  date.

Triggered by `gh-hud` from workflow issue comment `<!-- dewey-distill-trigger --> /dewey-distill-auto #N`.

<protect>
## Instructions

### 0. Startup Gates (MANDATORY)

Before any pipeline stage, run `dewey gate auto-answer`. Treat non-pass as
refusal: stop before scope and relay output. Do not retry.

Then run `dewey gate distill-unattended`. On refusal (exit 1 — the expected
state until jflowers/opencode-supervisor#51 ships), the no-op run artifact is
already recorded under `.uf/dewey/distill-runs/`: post the verdict's Notice
line (`supervisor command profile unavailable, unattended distill skipped`) as
a comment on the dispatch issue via `gh issue comment <N>` and stop before
scope. Do not retry.

### 1. Scope Computation (Deterministic, No Question)

Call the `scope` MCP tool (no arguments unless `--backlog-from` was
supplied — then pass `backlog_from` with the date; the vault, the
default-branch ref
via the `gitref` precedence chain: vault config → global config →
`DEWEY_DEFAULT_BRANCH_REF` → git config `dewey.defaultBranchRef` →
`origin/main`, and the exclusion policy all resolve server-side). Tool behavior
per `contracts/scope.md`:

```
learnings = ls_tree(ref, .uf/dewey/learnings/) -> {identities}
compiled_blobs = ls_tree(ref, .uf/dewey/compiled/) -> {paths}
cited = union(parse_frontmatter(blob).sources for blob in compiled_blobs)
raw = learnings - cited
filtered = raw - policy.exclude_tags.matches(raw)
return {all, cited, in_scope: filtered, provenance: git:<ref>:<sha>}
```

Policy `.uf/dewey/distill-policy.yaml` optional, bounded (≤16 KiB, depth ≤3,
≤100 entries, 200 chars each); `["*"]` overly-broad exclusion ignored with
warning; malformed entries warned/skipped, run proceeds. Fallback when not git
or no ref: scan `.uf/dewey/learnings/*.md` + SQLite `pages` namespace `learning`,
provenance `filesystem:degraded` + warning, no panic. Never call `question`;
assert via log grep `question` == 0. If `in_scope == ∅` write durable no-op
artifact `.uf/dewey/distill-runs/<YYYYMMDD-HHMMSS>.json` `{schema: 1, run_at, scope_size:0, themes:[], prs_touched:[], no_op_reason:"empty scope"}` and exit — no PR, no
comment.

### 2. Grouping Into Themes

Call `group_learners` MCP tool with `in_scope` identities per `contracts/grouping.md`:

1. Build must-colocate components from `supersedes` frontmatter edges (FR-009).
2. For each component compute closest compiled article via embedding cosine (Ollama/Vertex/OpenAI-compatible via `embed.NewEmbedderFromConfig`) or tag centroid.
3. If distance < threshold or tag match assign to that theme; else new-theme bucket.
4. Tie within margin → deterministic centroid tie-breaker, log ambiguity (FR-010).
5. LLM judge (Jev) behind flag only if configured; server-side, every decision logged.

Every `AssignmentDecision` persisted with `{source: embedding_distance|tag_fallback|supersession_coercion|tie_breaker|llm_judge, confidence 0..1, details}`. Embeddings
unavailable → tag fallback, log degraded mode. Results: `themes: Theme[]` one per
rolling PR.

Sanitization gate: learnings scanned by `sanitize` per-source; critical/high
findings (prompt injection, data URIs, exfiltration) dropped from synthesis
prompt; PR body notes `sanitization: dropped <id> (critical)`.

### 3. Rolling Per-Theme PRs with Review Queue Body

Call `rolling_pr` MCP tool per `contracts/rolling-pr.md` (pass
`dry_run: true` when invoked with `--dry-run`, then stop after the
preview result — report the would-be per-theme actions and exit; no run
artifact is recorded on a dry run):

Per theme `branch = dewey-distill/<sanitized>` where sanitize:
lowercase → replace `[^a-z0-9]` with `-` → collapse `--` → trim `-` → `untagged` if empty → truncate base to 50 chars → append `-`+8-hex `sha256(theme)[:8]` → collision append `-2`/`-3` lexicographically. Max total `dewey-distill/<sanitized>` 80 chars; traversal `..`/`/` rejected; charset `[a-z0-9-_/]+`.

The tool implements the contract's behavior server-side — the agent never runs these git or gh commands itself (they are not on its bash allowlist):

- If open PR exists via `gh pr list --label dewey-distill --state open` → `git fetch origin/<branch>`, rebase preserving human commits (fixup! or non-distiller author), compare working-tree vs `origin/<branch>` last-committed content; if human edited file also regenerated refuse overwrite, label `needs-human-resolution`, list files in body, skip that file; else regenerate draft articles + hygiene, `gh pr edit` body, `git push --force-with-lease`.
- Else → create branch, write draft articles (`tier: draft` + `validity`/`superseded_by` hygiene), `gh pr create --label human-gate --body ReviewQueue`.
- Conflicts on rebase → leave markers, label `needs-human-resolution`, body lists files.
- Closed PR for same theme → fresh create, never reopen.
- No new learnings for theme → leave open PR untouched, no empty push.
- Concurrency: `gh pr list → branch exists` check before create; push uses `--force-with-lease=<branch>:<expected-sha>` CAS; on stale info retry as update once.
- Never write `tier: validated`; server guard rejects.

Review queue body per article (capped 65k, escaped Markdown for `[ ] ( ) ! # * _ ` < >`):

```md
## <article title> (`<path>`)
**What changed**: <1-line theme delta>
**Newly cited**: `learning/<id>` (N)
**Supersessions**: `<successor> supersedes <target>`
**Lint**: `anchor_drift` / `embedding_gap` findings
**Confidence**: avg + per-decision distances
```

Also write durable artifact `.uf/dewey/distill-runs/<ts>.json` `{schema: 1, run_at, scope_size, themes, prs_touched:[{branch,pr_number,state,head_sha,human_commits_preserved}], no_op_reason?}` and log provenance. The `schema` field is the record's JSON schema version (1 today) so future field changes remain distinguishable across the append-only history.

### 4. Report

Log themes, PRs touched (branch, PR number, head SHA, human commits preserved),
sanitizations dropped, lint findings, and no-op reasons. No promotion; human vouch
via merge of `human-gate` PR enables #203 trust derivation (`dewey doctor` shows
`validated` after re-index via blob equality).

## Constraints

- Never call `question` tool; fallback to deterministic scope.
- Never write `tier: validated`; only `tier: draft`.
- Never run raw git or gh write commands — they are not on the distiller's
  bash allowlist; the `rolling_pr` tool performs every git and gh mutation
  server-side, confined to `dewey-distill/*` branches.
- Never call `promote` or validate without human merge.
- Keep API keys out of logs.
</protect>
<!-- dewey:scaffold dev sha256:db7f12b8e835f6a8df19a15ef11903b37a8966551f19d3ad3874abeb79b4f45a -->
