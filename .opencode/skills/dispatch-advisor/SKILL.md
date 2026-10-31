---
name: dispatch-advisor
description: >-
  Plans deterministic Divisor review fan-out through
  plan_review_dispatch. Use before review-council, triage-issue,
  address-feedback, or Speckit test-review dispatch.
---
<!-- scaffolded by uf vdev -->
# Skill: Dispatch Advisor

Create one validated dispatch plan before a Divisor workflow starts child
review sessions. This skill adapts the verified fullsend advisor while
removing fullsend-only and content-persona review behavior.

The `review-dispatch` plugin owns policy. This skill and every consuming
command MUST call `plan_review_dispatch`; they MUST NOT parse the review
matrix or reviewer manifest, select reviewers, classify tiers, resolve
models or variants, augment runs, deduplicate runs, or enforce limits on
their own.

## Inputs

Pass these raw inputs to `plan_review_dispatch`:

| Input | Contract |
|-------|----------|
| `mode` | `code`, `specs`, `triage`, `feedback`, or `test` |
| `discovered_agents` | Every discovered `divisor-*` agent name; do not pre-prune |
| `full` | Whether the validated command arguments contain `--full` |
| `augment` | Explicit caller opt-in to configured risk augmentation |
| `changed_files` | For non-triage modes, path plus inserted/deleted counts for each changed file |
| `issue` | For triage, raw title, nullable body, and comments with numeric id, creation time, and body |

Do not synthesize a diff for issue triage. Do not summarize or normalize
issue content before passing it to the tool. The planner owns Unicode and
line-ending normalization, comment ordering, framing, hashing, keyword
classification, and issue tier boundaries.

For diff-backed modes, pass the reviewed base-to-head file list and line
counts. The planner owns Protocol 3 path classification, component span,
security and user-facing signals, reviewer relevance, and total tiering.
`specs` is retained as the command mode while the planner normalizes its
matrix lookup to `spec`. `test` uses code profiling and restricts selection
to `divisor-testing`.

## Plan Contract

Treat the returned JSON bytes as authoritative. The planner emits version 1
with stable agent-name and run-sequence ordering. Every entry records:

- agent and include/skip decision;
- reason code and human-readable reason;
- `explicit`, `advisor`, or `host` source;
- positive per-agent sequence and read-only intent;
- nullable tier, model, and variant; and
- validation errors.

Explicit arrays replace advisor-generated runs. Agents absent from both
explicit and advisor configuration receive one host-model run. Profile and
run variants are already resolved in the plan. Duplicate model/variant
pairs are omitted with provenance. Risk augmentation occurs only when the
caller opts in and matrix policy permits it.

Content-capability personas never receive assessment runs. Adversary and
Guard remain baseline reviewers. Other review personas require their
manifest scope to intersect the deterministic change categories. Curator
runs only for documentation or user-facing changes and may receive at most
one assessment run. Every child prompt MUST forbid issue creation; this is
especially important for Curator. Only the parent may perform one
deduplicated curation action after consolidation and the existing human
gate.

## Full Panel

When `full` is true, the planner includes each discovered review-capable
persona exactly once at the standard profile. It excludes content personas
and disables pruning, explicit fan-out, overrides, and risk augmentation.
It never truncates the panel. If configured persona or total-run limits
cannot hold the complete panel, the result is `INCONCLUSIVE` and records
the required and configured counts.

## Fail Closed

Proceed to execution only when `status` is `ready`, `workflow_result` is
null, and `errors` is empty. Any malformed matrix, malformed reviewer
manifest, unknown discovered agent, invalid explicit run, Curator run-limit
violation, or configured/absolute limit failure returns `INCONCLUSIVE`
before child dispatch.

Never repair, reinterpret, truncate, or regenerate a failed plan in a
Markdown command. Report the planner errors and request configuration repair
or human review.

## Invocation Tool

All dispatch-planned runs MUST be executed through `dispatch_review_runs`,
not `invoke_agent` or individual `dispatch_agent_run` calls.
`dispatch_review_runs` acquires sibling evidence, builds review prompts,
and dispatches all included plan entries in parallel batches — replacing
7-8 sequential tool calls with one deterministic aggregation. Pass the
plan entries array from `plan_review_dispatch` directly. Reserve
`invoke_agent` for ad-hoc, non-dispatch agent calls only.

Execution belongs to the separate invocation contract; this skill does not
invoke agents or persist the legacy fullsend cost log.
