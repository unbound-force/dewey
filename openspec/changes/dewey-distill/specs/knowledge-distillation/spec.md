# Delta Spec: knowledge-distillation

## ADDED Requirements

### Requirement: OpenAI-Compatible Synthesis Provider

Dewey MUST provide a synthesis provider named `openai-compatible`
that sends prompts to an OpenAI-compatible chat-completions endpoint
and returns the assistant message content. It MUST be configured
through `synthesis.provider`, `synthesis.endpoint`, and
`synthesis.model`, with optional `api_key`, `timeout`, and
`max_tokens` (defaults: 300 seconds, 16000 tokens). The API key MUST
be sent only in the `Authorization` header.

#### Scenario: Successful synthesis

- **GIVEN** a configured `openai-compatible` provider and a reachable
  endpoint
- **WHEN** `Synthesize` is called with a prompt
- **THEN** Dewey MUST send `POST <endpoint>/chat/completions` with the
  configured model and max tokens
- **AND** MUST return the text from `choices[0].message.content`

#### Scenario: Endpoint already names the chat path

- **GIVEN** an endpoint ending in `/chat/completions`
- **WHEN** `Synthesize` is called
- **THEN** Dewey MUST NOT append a second `/chat/completions`

#### Scenario: Truncated output

- **GIVEN** a provider response with `finish_reason` equal to `length`
- **WHEN** `Synthesize` parses the response
- **THEN** it MUST return an error identifying output truncation and
  the `max_tokens` setting
- **AND** MUST NOT return partially truncated text as a successful
  result

#### Scenario: Rate limiting

- **GIVEN** an endpoint that returns HTTP 429 with a `Retry-After`
  header
- **WHEN** `Synthesize` is called
- **THEN** Dewey MUST retry up to five times with bounded exponential
  backoff honoring `Retry-After`
- **AND** MUST stop immediately when the context is cancelled
- **AND** MUST return an error after the final retry

#### Scenario: Authentication failure

- **GIVEN** an endpoint that rejects the request with HTTP 401
- **WHEN** `Synthesize` is called
- **THEN** Dewey MUST return an error containing the HTTP status
- **AND** the error MUST NOT contain the API key

### Requirement: Credential Resolution and Redaction

The API key MUST resolve from the `DEWEY_SYNTHESIS_API_KEY`
environment variable first and from `synthesis.api_key` second. The
key MUST NOT be written to logs, tool output, persisted files, or
error messages. If a provider response body echoes the key, Dewey MUST
replace it with `[REDACTED]` before surfacing the body.

#### Scenario: Environment variable wins

- **GIVEN** both `DEWEY_SYNTHESIS_API_KEY` and `synthesis.api_key` are
  set to different values
- **WHEN** the provider is constructed
- **THEN** it MUST use the environment variable value

#### Scenario: Missing key

- **GIVEN** no environment variable and no `api_key` in config
- **WHEN** `NewSynthesizerFromConfig` is called with
  `provider: openai-compatible`
- **THEN** it MUST return an error naming both accepted key sources

#### Scenario: Key absent from diagnostics

- **GIVEN** a configured provider with a key
- **WHEN** `dewey doctor` runs and a synthesis error is produced
- **THEN** neither output MUST contain the key

### Requirement: Server-Side Synthesis Wiring

When a synthesis model is configured and the provider can be
constructed, `dewey serve` MUST pass that synthesizer to the `compile`
and `curate` MCP tools so they perform synthesis server-side. The same
instance SHOULD be reused by background curation. The wiring MUST NOT
add a network call to server startup.

#### Scenario: Compile synthesizes server-side

- **GIVEN** a running server with a configured and constructible
  synthesizer and stored draft learnings
- **WHEN** the `compile` MCP tool is called
- **THEN** its result MUST have `status: "compiled"`
- **AND** it MUST write `.uf/dewey/compiled/{tag}.md` for each topic
- **AND** no CLI process MUST be required

#### Scenario: Curate synthesizes server-side

- **GIVEN** a running server with a configured and constructible
  synthesizer and a configured knowledge store
- **WHEN** the `curate` MCP tool is called
- **THEN** its result MUST have `status: "complete"`
- **AND** curated files MUST be written and indexed as before

#### Scenario: No provider configured

- **GIVEN** no synthesis model configured
- **WHEN** the server starts
- **THEN** the synthesizer passed to the tools MUST be nil
- **AND** `compile`/`curate` MUST return their prompt-mode payloads

#### Scenario: Partial compilation reports failures

- **GIVEN** a synthesizer that succeeds for one topic and fails for another
- **WHEN** `compile` is called
- **THEN** the result MUST have `status: "partial"`, an `IsError` result,
  and a `failed` array naming each failed topic
- **AND** successful topics MUST be written and indexed
- **AND** a failed topic's existing article and tier MUST be retained

#### Scenario: Total compilation failure changes nothing

- **GIVEN** a synthesizer that fails for every topic
- **WHEN** `compile` is called
- **THEN** the result MUST have `status: "failed"` and `IsError`
- **AND** no existing compiled page or file MUST be deleted or replaced

#### Scenario: Provenance and tier preservation

- **GIVEN** a compiled article with tier `validated`
- **WHEN** the article is recompiled
- **THEN** its tier MUST remain `validated`
- **AND** the article frontmatter, page properties, and result MUST
  record the synthesizing model as `compiled_by`

#### Scenario: Partial curation reports failures

- **GIVEN** multiple configured stores and one store's curation fails
- **WHEN** `curate` is called
- **THEN** the result MUST have `status: "partial"` and `IsError`
- **AND** MUST include each store's result and error

### Requirement: Prompt-Mode Fallback Compatibility

When synthesis is unconfigured, unavailable, or the provider cannot be
constructed, `compile` MUST return `status: "prompts_ready"` with the
existing cluster/prompt fields and `curate` MUST return
`status: "prompt_ready"` with the existing store/prompt fields. For
non-empty learning collections these payloads MUST remain
byte-compatible with the pre-change behavior so calling agents can
synthesize and call `store_compiled`. When no learnings exist and a
synthesizer is available, `compile` MUST clear draft compiled pages,
retain validated ones, and report surviving tags in
`retained_validated` — validated knowledge is never silently
destroyed by an empty collection (PR 145 amendment).

#### Scenario: Compile prompt mode

- **GIVEN** a server with no synthesizer
- **WHEN** `compile` is called on stored learnings
- **THEN** the result MUST contain `status: "prompts_ready"` and one
  cluster per topic with its `synthesis_prompt`
- **AND** no compiled file MUST be written

#### Scenario: Agent completes prompt mode

- **GIVEN** a `prompts_ready` compile result
- **WHEN** the agent synthesizes a cluster prompt and calls
  `store_compiled` with the tag, content, and sources
- **THEN** `store_compiled` MUST persist the article at
  `compiled/{tag}` with tier `draft` as before

#### Scenario: Curate prompt mode

- **GIVEN** a server with no synthesizer and configured stores
- **WHEN** `curate` is called
- **THEN** the result MUST contain `status: "prompt_ready"` and each
  store's `extraction_prompt`
- **AND** no curated file MUST be written

#### Scenario: Prompt mode preserves artifacts

- **GIVEN** existing compiled articles and a server with no synthesizer
- **WHEN** `compile` is called
- **THEN** it MUST return `prompts_ready`
- **AND** MUST NOT delete existing compiled pages or files

### Requirement: Compile and Index Mutual Exclusion

`compile` MUST share the pipeline-wide index mutex with indexing and
curation. If the lock is held, `compile` MUST return an "already in
progress" error without reading or writing store content. Concurrent
compiles MUST also be refused.

#### Scenario: Compile during indexing

- **GIVEN** a background index holds the shared mutex
- **WHEN** `compile` is called
- **THEN** it MUST return an error indicating compilation cannot run
  while indexing is in progress
- **AND** it MUST NOT modify compiled pages or files

#### Scenario: Index requests during compile

- **GIVEN** a compile holds the shared mutex
- **WHEN** an index or curate request arrives
- **THEN** it MUST be refused by the same lock rather than racing the
  compile

### Requirement: Namespace-Aware Page Resolution

Page promotion MUST resolve a page name with deterministic precedence:
exact match, case-insensitive match, namespace candidates
(`compiled/{name}`, `learning/{name}`, `knowledge/*/{name}`) for bare
names, then `source_id`/`source_doc_id` matching. If a tier yields
multiple candidates, the tool MUST return an ambiguity error naming
them. A real store query error MUST be surfaced, not reported as
"Page not found".

#### Scenario: Bare compiled tag

- **GIVEN** a page named `compiled/gateway` with tier `draft`
- **WHEN** `promote` is called with `page: "gateway"`
- **THEN** the page MUST be resolved and promoted to `validated`
- **AND** the result MUST report the resolved page name
  `compiled/gateway`

#### Scenario: Case mismatch

- **GIVEN** a page named `compiled/Gateway` with tier `draft`
- **WHEN** `promote` is called with `page: "gateway"`
- **THEN** the page MUST be resolved case-insensitively

#### Scenario: Ambiguous bare name

- **GIVEN** both `learning/gateway` and `compiled/gateway` exist
- **WHEN** `promote` is called with `page: "gateway"`
- **THEN** the tool MUST return an ambiguity error listing both
  candidates
- **AND** MUST NOT promote either

#### Scenario: Store failure

- **GIVEN** the store query fails
- **WHEN** `promote` resolves the page
- **THEN** the error MUST describe the store failure
- **AND** MUST NOT be replaced by "Page not found"

### Requirement: Fail-Closed Batch Promotion

`promote` MUST accept an optional `pages` array in addition to the
existing `page` field. When `pages` is provided, the tool MUST resolve
and validate every page before changing any tier. If any page is
missing, ambiguous, or not `draft`, the tool MUST promote none and
return per-page errors. Supplying both `page` and `pages` MUST be a
validation error. Duplicate names MUST be deduplicated.

#### Scenario: Batch success

- **GIVEN** three draft pages resolved by name
- **WHEN** `promote` is called with all three in `pages`
- **THEN** all three MUST become `validated`
- **AND** the result MUST list each promoted page and the total

#### Scenario: Batch write failure

- **GIVEN** a batch that passes validation
- **WHEN** the tier update fails partway through
- **THEN** the transaction MUST roll back
- **AND** no page's tier MUST change
- **AND** the result MUST report the failure

#### Scenario: Batch with an invalid page

- **GIVEN** two draft pages and one page that is already `validated`
- **WHEN** `promote` is called with all three in `pages`
- **THEN** no page's tier MUST change
- **AND** the result MUST identify the failing page and its reason

#### Scenario: Single-page compatibility

- **GIVEN** a draft page
- **WHEN** `promote` is called with only `page`
- **THEN** the result shape MUST remain
  `{page, previous_tier, new_tier, message}`

#### Scenario: Both fields supplied

- **WHEN** `promote` is called with both `page` and a non-empty `pages`
- **THEN** it MUST return a validation error without mutating anything

### Requirement: Gated Distill Workflow

Dewey MUST provide a `/dewey-distill` slash command, scaffolded by
`dewey init` and tracked in `.opencode/commands/`, that runs lint,
inventory, a mandatory scope gate, compile/curate through MCP, a
mandatory review gate, batch promote, and a report. It MUST use MCP
tools rather than CLI subcommands while the server holds the store
lock.

#### Scenario: Full pipeline with synthesis

- **GIVEN** a running server with a working synthesizer
- **WHEN** the human invokes `/dewey-distill`
- **THEN** the command MUST present lint findings and an inventory
  before requesting scope approval
- **AND** after scope approval MUST call `compile` and/or `curate`
- **AND** MUST present generated artifacts and require review approval
  before calling `promote`
- **AND** MUST report results and suggest `/dewey-publish`

#### Scenario: Prompt-mode fallback

- **GIVEN** `compile` returns `prompts_ready`
- **WHEN** the command reaches the synthesis step
- **THEN** it MUST synthesize each cluster prompt and call
  `store_compiled` for each
- **AND** it MUST report that curation requires server-side synthesis
  if `curate` returns `prompt_ready`

#### Scenario: No CLI lock conflict

- **GIVEN** a running MCP server
- **WHEN** the command executes any stage
- **THEN** it MUST NOT run `dewey compile`, `dewey curate`,
  `dewey lint`, or `dewey promote`

### Requirement: Doctor Provider Diagnostics

`dewey doctor` MUST recognize the `openai-compatible` provider and
report endpoint, model, and credential presence without making a live
API call or printing the key. The unknown-provider message MUST list
the supported providers including `openai-compatible`.

#### Scenario: Complete configuration

- **GIVEN** endpoint, model, and a key
- **WHEN** `dewey doctor` runs
- **THEN** the synthesis section MUST report PASS with the endpoint and
  model
- **AND** MUST NOT print the key

#### Scenario: Missing credential

- **GIVEN** endpoint and model but no key
- **WHEN** `dewey doctor` runs
- **THEN** the synthesis section MUST report FAIL
- **AND** MUST name `DEWEY_SYNTHESIS_API_KEY` and
  `synthesis.api_key` as the accepted sources

### Requirement: Backward Compatibility

The change MUST NOT alter the contract or output of the 37 inherited
graphthulhu MCP tools, MUST NOT add module dependencies or CGO, and MUST
NOT change the database schema except for the additive, index-only
v2→v3 migration that creates two NOCASE indexes backing promote name
resolution (no tables or columns altered). Existing CLI `compile`/`curate` success
paths and payloads MUST remain compatible; the shared handlers gain the
same non-destructive prompt mode and partial/failed reporting described
above.

#### Scenario: Inherited tools unchanged

- **WHEN** the server registers tools
- **THEN** every pre-existing tool MUST keep its name, input
  contract, and output shape
- **AND** only `promote` MAY gain the additive optional `pages` field

#### Scenario: Existing configs parse

- **GIVEN** a `config.yaml` without `api_key`, `timeout`, or
  `max_tokens`
- **WHEN** `ReadSynthesisConfig` reads it
- **THEN** it MUST return the same provider, model, and endpoint as
  before
- **AND** the provider defaults MUST fill the absent optional values

#### Scenario: 017 synthesis requirements superseded

- **GIVEN** the OpenAI-compatible synthesis provider introduced by this change
- **THEN** it MUST honor `synthesis.api_key` with `DEWEY_SYNTHESIS_API_KEY` taking precedence, superseding 017 FR-006's environment-only rule (which remains embedding-only)
- **AND** the synthesis credential chain MUST NOT read `DEWEY_API_KEY` or `OPENROUTER_API_KEY`; 017 FR-007's full chain remains embedding-only
- **AND** a missing synthesis key MUST be a configuration error naming both accepted sources, superseding 017 FR-008's keyless gateway allowance for synthesis (embedding keeps keyless support)
- **AND** synthesis availability MUST be configuration completeness, not a `/models` probe, superseding 017 FR-014 for synthesis (the embedding transport keeps its cached probe)
- **AND** synthesis endpoint resolution MUST be config-first, falling back to `DEWEY_SYNTHESIS_ENDPOINT` only when `synthesis.endpoint` is empty, superseding 017 FR-022 for synthesis
- **AND** no synthesis-side startup credential warnings MUST be emitted; 017 FR-024's warnings remain embedding-only

#### Scenario: openaicompat transport is embedding-only

- **WHEN** the `openaicompat` package is inspected after this change
- **THEN** it MUST expose embedding operations only — the `/embeddings` and `/models` paths and the embedding capability's credential chain
- **AND** the synthesis provider MUST own its transport inside the `llm` package and MUST NOT import `openaicompat`
- **AND** the superseded 017 synthesis-probe CLI tests (`SynthesisGateway*`) MUST NOT be restored; this change's configuration-completeness doctor tests replace them

### Requirement: Learning Re-ingestion Tier Neutrality

Learning markdown files are untrusted backups. Re-ingestion MUST NOT
process tier claims: files declaring `tier: validated` or `tier:
authored` in frontmatter MUST be ingested as `draft`, with a warning
naming the ignored claim. Tier transitions route exclusively through
the `promote` gate. Re-ingestion only fills pages absent from the
store, so an existing promoted page is never demoted.

#### Scenario: Claimed tier is ignored

- **GIVEN** a learning markdown file whose frontmatter declares
  `tier: validated` and whose `learning/{identity}` page is absent
  from the store
- **WHEN** re-ingestion runs (for example, after `graph.db` is
  recreated)
- **THEN** the page MUST be created with tier `draft`
- **AND** a warning MUST be logged that the claimed tier was ignored

#### Scenario: Human edits take precedence for content and metadata

- **GIVEN** a human edits a learning markdown file's content or
  metadata
- **WHEN** the file is re-ingested while its store page is absent
- **THEN** the edited content and metadata MUST take precedence
- **AND** the tier MUST still land at `draft` — tier claims never take
  effect via re-ingestion

#### Scenario: Existing store pages are not demoted

- **GIVEN** a promoted learning page (`learning/{identity}`, tier
  `validated`) exists in the store
- **WHEN** re-ingestion runs
- **THEN** the file MUST be skipped and the stored tier MUST remain
  unchanged
