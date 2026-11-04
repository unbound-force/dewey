# Tasks: dewey-distill

## 1. OpenAI-Compatible Provider

- [x] 1.1 Create `llm/openai_compatible.go`: `OpenAICompatibleSynthesizer`
  implementing `Synthesizer` (`Synthesize`, `Available`, `ModelID`),
  with constants for the 300s timeout, 16000 max tokens, 5 retries,
  1s base delay, and 60s cap; request/response structs for
  `POST {endpoint}/chat/completions`; endpoint joining that tolerates a
  base URL or a URL already ending in `/chat/completions`; GoDoc on
  every exported symbol
- [x] 1.2 Implement bounded retry: only HTTP 429 is retried, using
  `Retry-After` integer seconds when present and exponential backoff
  with ±25% jitter otherwise; honor context cancellation between
  attempts; return a final error naming the retry count; read bodies
  with a 50 MiB limit and truncate error excerpts to 512 runes
- [x] 1.3 Implement safety guards: treat `choices[0].message.content`
  as required (empty → error naming the response contract); treat
  `finish_reason == "length"` as an error naming
  `synthesis.max_tokens`; redact the API key from any response-body
  excerpt returned in an error; never log the key or the
  `Authorization` header
- [x] 1.4 Implement `Available()` as configuration completeness
  (endpoint, model, and key all non-empty) with no network call, and
  `ModelID()` returning the configured model
- [x] 1.5 Add `llm/openai_compatible_test.go`: success path asserting
  method, path, bearer header, model, max tokens, and parsed content;
  endpoint already ending in `/chat/completions`; 401 error containing
  the status and not the key; malformed JSON; empty content;
  `finish_reason: length`; 429 then success with `Retry-After: 0`/small
  value; exhausted 429 retries; context cancellation; key echoed in an
  error body is redacted; `Available()` true/false matrix

## 2. Configuration, Factory, and Doctor

- [x] 2.1 Extend `llm.ProviderConfig` with `APIKey`, `Timeout`
  (`time.Duration`), and `MaxTokens` (int), with GoDoc describing
  precedence and defaults
- [x] 2.2 Extend `llm/config.go`'s `configFile.Synthesis` with
  `api_key`, `timeout` (duration string, e.g. `300s`), and `max_tokens`
  YAML fields; populate them in both the per-vault and global synthesis
  branches of `ReadSynthesisConfig`; preserve existing precedence and
  the legacy `compile_model` path unchanged
- [x] 2.3 Implement API key resolution: `DEWEY_SYNTHESIS_API_KEY` first,
  then `ProviderConfig.APIKey`; apply the 300s/16000 defaults when the
  optional values are zero; do not log the key
- [x] 2.4 Add the `openai-compatible` branch to
  `NewSynthesizerFromConfig`: require endpoint, model, and a resolved
  key, returning actionable errors naming the missing field; update the
  unknown-provider message to list `ollama`, `vertex`, and
  `openai-compatible`
- [x] 2.5 Add `case "openai-compatible"` to the doctor synthesis layer
  in `cli.go`: print endpoint and model, PASS when endpoint/model/key
  are complete, FAIL with fix text naming `DEWEY_SYNTHESIS_API_KEY`
  and `synthesis.api_key` otherwise, no live call, never print the key;
  update the default-branch supported list
- [x] 2.6 Add `llm/config_test.go` / `llm/provider_test.go` coverage:
  new YAML fields parse from per-vault and global config; env key wins
  over config; defaults fill zero values; legacy configs unchanged;
  factory success and each missing-field error; provider list in the
  unknown-provider error

## 3. Server Wiring and Compile Lock

- [x] 3.1 Add `synth llm.Synthesizer` to `serverConfig` and a
  `WithSynthesizer` option; pass `cfg.synth` to `tools.NewCompile` and
  `tools.NewCurate` (replacing the nil literals); keep registration
  count and all other tool registrations unchanged
- [x] 3.2 In `executeServe`, resolve the vault path from the assembled
  server options, read `llm.ReadSynthesisConfig`, construct the
  synthesizer, and append `WithSynthesizer` when a model is configured
  and construction succeeds; on any failure leave the option unset and
  log at debug/info without credentials
- [x] 3.3 Change `tools.NewCompile` to accept `mu *sync.Mutex` (nil
  creates a private mutex) in the same position as `NewCurate`; use
  `TryLock`/`Unlock` in `Compile`; keep the "already in progress" error
  behavior and align its message with the index pipeline
- [x] 3.4 Update all `NewCompile` call sites: `server.go` (shared
  `cfg.indexMutex`), `cli.go` (fresh mutex), and tests/integration
  tests (nil unless a test exercises contention)
- [x] 3.5 Refactor `backgroundCuration` to accept the shared
  `llm.Synthesizer` instead of constructing its own; keep the
  after-index `Available()` gate and the "no synthesis model
  configured" log path; pass the instance from `executeServe`
- [x] 3.6 Add wiring tests: `newServer` with a fake synthesizer reaches
  compile/curate (tool result status proves synthesis path);
  `NewCompile` refuses while the shared mutex is held and writes
  nothing; existing prompt-mode tests still pass

## 4. Page Resolution and Batch Promotion

- [x] 4.1 Add the resolution ladder: `resolvePromotePage` in
  `tools/promote.go` (exact → NOCASE → bare-name namespace
  candidates including `knowledge/*` → `source_id`/`source_doc_id`),
  with deterministic ambiguity errors listing candidates, empty-name
  validation, and wrapped query errors; backed by three narrow store
  list methods (`ListPagesByNameFoldCase`,
  `ListKnowledgePagesFoldCase`, `GetPageBySourceDoc`) and a schema v3
  additive, index-only migration creating the two NOCASE indexes
- [x] 4.2 Add resolver tests in `tools/promote_resolve_test.go`:
  exact; case mismatch; bare tag resolving `compiled/` and `learning/`;
  `knowledge/{store}/{name}` (including a mixed-case fixture);
  namespaced source-doc-id match; ambiguity (with candidate cap);
  missing page returns `(nil, nil)`; empty name error;
  store/query error propagation; namespace-beats-source-doc
  precedence — plus `store` tests asserting `EXPLAIN QUERY PLAN` pins
  each query to an index `SEARCH` and a v2→v3 migration test
- [x] 4.3 Extend `types.PromoteInput` with
  `Pages []string` (`json:"pages,omitempty"`) while keeping `Page`;
  document mutual exclusivity in the jsonschema text
- [x] 4.4 Rewrite `tools/promote.go`: resolve via the tools-side
  `resolvePromotePage` ladder,
  surface real store errors, keep the single-page result shape when
  only `page` is set, add batch mode with validate-all-then-mutate,
  dedupe in request order, per-page `{page, error}` failures, no
  mutations on failure, `{status, promoted, total_promoted, message}`
  on success, and a validation error when both `page` and `pages` are
  set
- [x] 4.5 Add `tools/promote_test.go` coverage: bare compiled tag
  (#91 repro), case mismatch, `knowledge/*` resolution, ambiguity
  refusal, store error not masked as "Page not found", single-page
  shape unchanged, batch success, batch fail-closed with an
  already-validated page, duplicate dedupe, both-fields validation,
  empty `pages` element
- [x] 4.6 Update `newPromoteCmd` in `cli.go` to accept
  `PAGE_NAME...` (`cobra.MinimumNArgs(1)`), route one argument through
  `Page` and multiple through `Pages`, print both result shapes, and
  update the help/examples
- [x] 4.7 Add CLI tests: multi-argument promotion, bare-tag promotion,
  and batch failure exit behavior

## 5. Slash Command

- [x] 5.1 Create `.opencode/commands/dewey-distill.md` per design D8:
  frontmatter description; usage; `<protect>`-wrapped instructions for
  lint → inventory (`list_pages` for `learning` and `compiled`) →
  mandatory scope gate → `compile`/`curate` → prompt-mode fallback
  (`store_compiled` for compile clusters; report that curation needs
  server-side synthesis) → mandatory review gate → single batch
  `promote` → report and suggest `/dewey-publish`; explicitly forbid
  CLI subcommands that open the store lock
- [x] 5.2 Add the `dewey-distill.md` entry to `deweySlashCommands` in
  `slash_commands.go` with content byte-identical to the tracked file
  (use the existing backtick-concatenation pattern)
- [x] 5.3 Extend the slash-command byte-comparison test in
  `cli_test.go` (currently publish-specific at ~line 4450) to cover
  `dewey-distill.md` in both the scaffolded output and the tracked
  `.opencode/commands/` file; verify `dewey init` still does not
  overwrite existing files

## 6. DE-007 Seam Verification

- [x] 6.1 Registration surface: assert `promote` advertises the
  optional `pages` array alongside `page` while every other MCP tool
  registration is unchanged (tool count constant except additive
  schema fields)
- [x] 6.2 Documented output shapes: pin the provider error vocabulary
  (HTTP status, truncation, missing content, missing key),
  `promote` single vs batch result shapes, and the unchanged
  `prompts_ready`/`prompt_ready` payload keys in tests or golden
  assertions
- [x] 6.3 Write→read round-trips: `store_compiled(tag)` → `promote`
  with the bare tag → re-read tier `validated`; `compile` with a fake
  synthesizer → `promote` `compiled/{tag}` → tier check; `curate` with
  a fake synthesizer → knowledge files indexed under
  `knowledge/{store}/*` with tier `curated`

## 7. Documentation, Governance, and Verification

- [x] 7.1 [P] Update `README.md`: add the `openai-compatible` provider
  to the synthesis configuration section (YAML example with endpoint,
  model, timeout, max_tokens, and the `DEWEY_SYNTHESIS_API_KEY`/
  `synthesis.api_key` precedence), document the `/dewey-distill` flow
  and its two gates, and update the `dewey promote` usage for multiple
  pages and bare tags
- [x] 7.2 [P] Update `AGENTS.md`: provider configuration fields, the
  `promote` batch/namespace behavior, the `/dewey-distill` command,
  server-side synthesis wiring note, and an Active Technologies /
  Recent Changes entry
- [x] 7.3 [P] Create the website documentation sync issue with a
  pre-filing duplicate search
  (`gh issue list --repo unbound-force/website --state open --search "dewey distill"`):
  `gh issue create --repo unbound-force/website --label docs` titled
  `docs: sync dewey distill documentation`, describing the new
  provider, `/dewey-distill`, promote batch behavior, and affected
  `content/docs/` pages
- [x] 7.4 Verify secret hygiene: grep the diff and test output for the
  key pattern used in tests; confirm no config example contains a real
  key; confirm gitleaks-relevant strings are absent
- [x] 7.5 CI parity (derive the exact commands from
  `.github/workflows/ci.yml` and `.github/workflows/mega-linter.yml` at
  execution time, not from memory): build, vet, `-race -count=1`
  tests with coverage, Gaze thresholds, and the linter — all green
  before any task in this group is marked complete
- [x] 7.6 Constitution alignment verification: confirm all four
  principles still PASS, the 37 inherited tools are untouched, no
  `go.mod` change, no CGO, only an additive index-only schema
  migration (v3 NOCASE indexes), and the provider is
  tested without network or real credentials
- [ ] 7.7 Run `/uf.review-council` and resolve all REQUEST CHANGES
  findings until every reviewer returns APPROVE, immediately before PR
  submission
- [x] 7.8 Document learning re-ingestion tier semantics (PR 145):
  `reIngestLearnings` in `main.go` keeps forcing `draft` for re-ingested
  learning files — frontmatter tier claims are ignored with a warning,
  pinned by `TestReIngestLearnings_IgnoresClaimedTier` in `main_test.go`;
  spec 015 and the delta requirement route tier transitions exclusively
  through the promote gate
