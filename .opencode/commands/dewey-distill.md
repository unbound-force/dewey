---
description: Lint, compile, curate, and promote knowledge through the running MCP server.
---

# Command: /dewey-distill

## Description

Turn draft learnings into compiled articles and curated knowledge through
the running Dewey MCP server, with two mandatory human gates. The pipeline
is lint → inventory → scope gate → compile/curate → review gate → batch
promote → report. Nothing is promoted without explicit approval.

The command uses MCP tools only. While the server runs it holds an
exclusive lock on `.uf/dewey/dewey.lock`, so `dewey lint`, `dewey compile`,
`dewey curate`, and `dewey promote` would fail with a lock error.

## Usage

```
/dewey-distill
/dewey-distill --tags authentication,gateway
```

<protect>
## Instructions

### 1. Lint

Call the `lint` MCP tool with `fix: false`. Present the findings grouped by
kind: stale decisions, uncompiled learnings, embedding gaps, contradictions,
and knowledge-store quality. Note that lint is advisory — do not change
tiers or delete content from this step.

### 2. Inventory

Call `list_pages` with `namespace: "learning"` and a high `limit` (for
example 200). Call it again with `namespace: "compiled"`. Present:
- the number of learning pages and their tags/identities, and
- the number of compiled articles and their tags.

If the invocation supplied tags (for example `/dewey-distill --tags authentication,gateway`), narrow the presentation to those tags. Read tags
from page names (`learning/{tag}-...`, `compiled/{tag}`) and from the
`tag` property when present. Do not infer tags from prose.

### 3. Scope Gate (MANDATORY)

Ask the human to choose:
- which tags to compile (all, or the listed subset — a subset requires
  incremental mode, because a full rebuild always covers every tag), and
- whether compilation is full or incremental, and
- which configured knowledge store to curate, curate all stores, or skip
  curation.

Do not call `compile` or `curate` before the human answers. If the human
chooses skip curation, do not call `curate` at all. Call `curate` with no
`store` only when the human explicitly approved all stores.

### 4. Compile and Curate

Call the `compile` MCP tool. For a full rebuild pass no arguments; for
incremental tags pass the learning identities for the approved tags in
`incremental`.

Inspect the result status:
- `status: "compiled"`: report each article path and topic from
  `articles`, and the total counts.
- `status: "partial"`: some topics failed. Report every entry in
  `failed`, keep the successfully written articles in scope for the review
  gate, and do not retry automatically.
- `status: "failed"`: every topic failed; existing compiled articles were
  left unchanged. Relay the `failed` errors and stop compilation. Do not
  retry without the human's direction.
- `status: "prompts_ready"`: server-side synthesis is unavailable. For
  each cluster, perform the synthesis yourself using its
  `synthesis_prompt` and `category_instructions`, then call
  `store_compiled` with the cluster's `dominant_tag`, the synthesized
  content, the cluster's learning identities as `sources`, and the model
  identifier you used as `model`. Report each stored page. Note:
  `store_compiled` refuses to overwrite an article whose existing tier
  is `validated` and suggests storing under a new tag — if you hit that
  refusal, report it to the human rather than retrying the same tag. Do
  not skip a cluster silently.

If curation was approved, call the `curate` MCP tool for the approved
store (or with no `store` only when all stores were approved). Inspect the
result status:
- `status: "complete"`: report each store's files created and any per-store
  error. The tool does not return per-item quality flags.
- `status: "partial"`: at least one store failed. Relay each store's
  result and error; successful stores remain reviewable.
- `status: "prompt_ready"`: there is no MCP tool that writes curated
  content back to a store. Report that curation requires a configured
  server-side synthesizer and leave store content unchanged. Do not write
  curated files by hand.

### 5. Review Gate (MANDATORY)

Present the generated and changed content before promoting anything:
- compiled articles: page names (`compiled/{tag}`), topics, and source
  counts from the compile result;
- learnings that were included in the compiled clusters;
- curated stores and item counts from the curate result.

Ask the human which pages to promote. Only draft pages are eligible.
Do not promote anything before the human answers, and do not promote a
page the human did not select.

### 6. Promote

Call the `promote` MCP tool once with a `pages` array of the approved page
names. Batch promotion is all-or-nothing: if any page is missing,
ambiguous, or not draft, no tiers change and the result lists each
failure. Relay the per-page failures verbatim. Do not retry by dropping
pages without asking the human first; do not fall back to one call per
page to work around a failure.

### 7. Report

Report concisely:
- the initial lint findings and which ones the run addressed,
- articles written or stored and their paths,
- curation results or the reason curation was skipped,
- pages promoted to validated,
- pages that remain draft and why.

Then suggest `/dewey-publish` if new compiled or curated artifacts should
be committed to the repository.

## Constraints

- Never run `dewey lint`, `dewey compile`, `dewey curate`, or
  `dewey promote` while the MCP server is running; use the MCP tools.
- Never call `promote` without the review-gate approval.
- Never invent curated content when `curate` returns `prompt_ready`.
- Never promote authored or already-validated pages.
- Keep the API key out of output. If a synthesis error mentions
  credentials, report only that authentication failed.
</protect>
