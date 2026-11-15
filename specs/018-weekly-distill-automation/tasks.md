# Tasks: Weekly Unattended Distill with Rolling Per-Theme Review PRs

**Input**: Design documents from `/specs/018-weekly-distill-automation/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are OPTIONAL - only included where spec mandates fixture/log assertions (SC-001..SC-008, FR-006 invariant).

**Organization**: Tasks grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Single project**: flat Go module `github.com/unbound-force/dewey/v3` at repository root
- Paths per plan.md structure: `.github/workflows/`, `.opencode/agents/`, `.opencode/commands/`, `tools/`, `gitref/`, `vault/`, `store/`, `.uf/dewey/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure for weekly unattended distill

- [x] T001 Create workflow template file `.github/workflows/dewey-distill.yml.example` with `on: schedule` cron `0 9 * * 1` + `workflow_dispatch`, permissions `contents: read, issues: write, pull-requests: write` per contracts/workflow-template.md
- [x] T002 [P] Create optional policy example `.uf/dewey/distill-policy.yaml.example` with `exclude_tags` and `exclude_stores` defaults per research.md D-009 and data-model.md DistillPolicy
- [x] T003 [P] Create distiller agent definition `.opencode/agents/distiller.md` with `command: dewey-distill-auto`, allowlist documentation (`read`, `list_pages`, `get_page`, `lint`, `compile`, `curate`, `verify_candidates`, `store_compiled` draft-only, `supersede_learning`), denied `question` + `tier: validated` per contracts/distiller-allowlist.md
- [x] T004 [P] Create unattended command `.opencode/commands/dewey-distill-auto.md` as unattended entry point (scope→group→rolling PR pipeline, no HUMAN_GATE questions) distinct from existing `.opencode/commands/dewey-distill.md`
- [x] T005 Update `cli.go` `dewey init` scaffolding to ship workflow template and document copy path `cp .github/workflows/dewey-distill.yml.example .github/workflows/dewey-distill.yml` per FR-001/FR-022 and plan.md Project Structure

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T006 Define core data-model types in `tools/scope.go` per data-model.md: `ScopeSet` (all_learnings, cited, in_scope, empty, provenance `git:<ref>:<sha>` or `filesystem:degraded`), `DistillPolicy` (exclude_tags/exclude_stores), `Theme` (name, existing_article, members, supersessions, decisions), `AssignmentDecision` (identity, target_theme, source enum, confidence, details), `RollingPR` (branch `dewey-distill/<theme>`, pr_number, state, labels, head_sha, human_commits_preserved, body, tier_written `draft`), `ReviewQueue` per-article rendering struct
- [x] T007 Implement `DistillPolicy` YAML loader in `tools/scope.go` with malformed-entry handling: log warning, skip invalid exclusion, proceed with valid scope (edge case: malformed policy) - supports `.uf/dewey/distill-policy.yaml` optional file per FR-007
- [x] T008 Add branch-name sanitizer utility in `tools/rolling_pr.go` (or `tools/scope.go` shared): sanitize theme→branch `[a-z0-9-_/]+`, max 100 chars, used for `dewey-distill/<theme>` per data-model.md Validation
- [x] T009 Configure server-side guard enforcement hook in `tools/promote.go` and `store/page.go` (or verify existing): reject any write with `tier: validated` from distiller context, file stays `draft` per FR-012, FR-017, data-model.md RollingPR.tier_written
- [x] T010 Add shared git helper fallback path using existing `gitref/` batch `ls-tree` + `vault/` filesystem scan precedent from `tools/lint.go` for non-git vault degraded mode per research.md D-002

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Weekly Scheduled Trigger Opens or Re-triggers a Distill Issue (Priority: P1) 🎯 MVP

**Goal**: Weekly GitHub Action fires, dedups to at most one open `dewey-distill` issue, posts comment `<!-- dewey-distill-trigger --> /dewey-distill-auto #N` if exists else creates issue assigned to maintainer, exposing contract for `gh-hud#28` dispatch

**Independent Test**: Configure template workflow in fixture repo. Run twice without closing issue → second run posts comment, no second issue. Close and re-run → new issue created. At no point two open issues with label `dewey-distill`.

- [ ] T011 [US1] Implement dedup job steps in `.github/workflows/dewey-distill.yml.example`: `gh issue list --label dewey-distill --state open --json number` → if any, `gh issue comment N --body "<!-- dewey-distill-trigger --> /dewey-distill-auto #N"` else `gh issue create --label dewey-distill --assignee @me --title "Weekly dewey-distill" --body "Dispatch: /dewey-distill-auto"` per contracts/workflow-template.md Behavior steps
- [ ] T012 [US1] Document `gh-hud` dispatch contract in `.github/workflows/dewey-distill.yml.example` header comments and `README.md`/`AGENTS.md`: required label `dewey-distill`, assignee convention, body marker, dispatch command `/dewey-distill-auto #{num}` as `autonomy` agent per FR-004, research.md D-001, contracts/workflow-template.md gh-hud Dispatch
- [ ] T013 [US1] Add workflow template adoption documentation in `README.md` and `AGENTS.md`: prerequisites CODEOWNERS + ruleset 26764121 (#194), labels `dewey-distill` + `human-gate` creation, copy command `cp .github/workflows/dewey-distill.yml.example .github/workflows/dewey-distill.yml` per FR-022, quickstart.md Enable steps 1-3
- [ ] T014 [US1] Create fixture test for dedup invariant in `tools/rolling_pr_test.go` or `.github/workflows/dewey-distill.yml.example` validation test: 4-week simulated schedule with overlapping triggers, assert at most one open `dewey-distill` issue per SC-002, second trigger → comment not duplicate per contracts/workflow-template.md Tests
- [ ] T015 [US1] Verify `dewey init` does not auto-install workflow but documents opt-in copy per research.md D-001 rationale (composability, no-op without gh-hud) and FR-001

---

## Phase 4: User Story 2 - Deterministic Scope Computation with No Interactive Question (Priority: P1)

**Goal**: Scope = all learning identities minus all compiled `sources` via `git ls-tree` on default branch, optionally filtered by policy, never invoking `question` tool, empty scope → no-op with no PR/comment

**Independent Test**: Fixture git repo with 10 learnings, 2 compiled citing 2 → 8 in-scope exact equality. Add policy excluding `scratch-*` → removed. Verify no `question` tool call in logs.

- [ ] T016 [US2] Implement `ComputeScope(default_ref string, policy DistillPolicy) (ScopeSet, error)` in `tools/scope.go` per contracts/scope.md: `learnings = ls_tree(ref, .uf/dewey/learnings/) → {identities}`, `compiled_blobs = ls_tree(ref, .uf/dewey/compiled/) → {paths}`, `cited = union(parse_frontmatter(blob).sources)`, `raw = learnings - cited`, `filtered = raw - policy.exclude_tags`, return `{all, cited, in_scope: filtered, provenance: git:<ref>:<sha>}` deterministic from git alone per FR-005, research.md D-002, SC-001
- [ ] T017 [US2] Implement fallback path in `tools/scope.go` when not git/no ref: scan `.uf/dewey/learnings/*.md` + SQLite `pages` with `namespace=learning`, provenance `filesystem:degraded` + warning log, no panic per contracts/scope.md Fallback, edge case non-git vault
- [ ] T018 [US2] Add hard denial of `question` tool in distiller mode in `server.go` or `tools/scope.go` caller: assert log grep `question` == 0, any code path attempting `question` is rejected + fallback to deterministic scope per FR-006, contracts/scope.md Invariants FR-006, SC-003
- [ ] T019 [US2] Handle empty scope no-op in `tools/scope.go` or pipeline entry `tools/rolling_pr.go` orchestrator: when `in_scope == ∅` exit without opening/updating PR, record no-op result per FR-021, contracts/scope.md Invariants empty scope, edge case no new learnings
- [ ] T020 [US2] Create fixture test in `tools/scope_test.go`: 10 learnings, 2 cited → 8 in-scope exact equality; add policy `scratch-*` → removed; missing policy → default no error; empty → no-op; non-git → degraded no panic per contracts/scope.md Tests
- [ ] T021 [US2] Add CLI dry-run/preview flag handling for scope verification: `go test -run Scope -count=1 -race ./...` and optional `/dewey-distill-auto --dry-run` preview per quickstart.md Verify

---

## Phase 5: User Story 3 - Assignment and Supersession Grouping Into Themes (Priority: P1)

**Goal**: Group in-scope learnings into existing compiled articles or new-theme buckets via embeddings + compile clustering, preserving explicit `supersedes` links, logging every decision with provenance and confidence, deterministic tie-breaker without LLM judge

**Independent Test**: 20 learnings / 4 clusters / 3 existing articles → 3 assignments correct + 1 new theme, supersession pair same theme, every decision logged with source/confidence

- [ ] T022 [US3] Implement must-colocate component building from supersession edges in `tools/group.go` per contracts/grouping.md: load `supersedes` frontmatter from git ls-tree pass, merge components before assignment per FR-009, research.md D-004, data-model.md Theme.supersessions
- [ ] T023 [US3] Implement theme assignment in `tools/group.go` using embeddings (`embed.NewEmbedderFromConfig` with Ollama/Vertex/OpenAI-compatible) + existing `tools/compile.go` clustering: for each component compute closest compiled article via embedding cosine or tag centroid, if distance < threshold or tag match assign to theme else new-theme bucket per contracts/grouping.md Behavior steps 2-3, FR-008, research.md D-003
- [ ] T024 [US3] Implement deterministic tie-breaker in `tools/group.go`: when ambiguity within narrow margin, apply closest centroid, log ambiguity without calling external model when no LLM judge configured per FR-010, contracts/grouping.md step 4, quickstart edge case
- [ ] T025 [US3] Add provenance logging per decision in `tools/group.go`: every `AssignmentDecision` includes `{source: embedding_distance|tag_fallback|supersession_coercion|tie_breaker|llm_judge, confidence 0..1, details}` with embedding distances/rule name/judge identity per FR-010, contracts/grouping.md Output, Invariants
- [ ] T026 [US3] Add future LLM judge (Jev) scaffolding behind flag in `tools/group.go` (disabled by default, server-side, every decision logged when configured) per research.md D-003, contracts/grouping.md step 5, but not active in MVP per Out of Scope
- [ ] T027 [US3] Implement embeddings fallback to tag-based grouping in `tools/group.go` when embeddings unavailable: same tag → same theme, log degraded mode per edge case, contracts/grouping.md Tests, research.md D-003
- [ ] T028 [US3] Create fixture test in `tools/group_test.go`: 20 learnings/4 clusters/3 articles → 1 new theme 3 assignments correct; supersession pair → same theme; embeddings unavailable → tag fallback; tight margin → tie-breaker per contracts/grouping.md Tests, research.md D-003

---

## Phase 6: User Story 4 - Rolling Per-Theme PRs with Review Queue Body (Priority: P1)

**Goal**: Each theme → branch `dewey-distill/<theme>` + PR with draft-tier articles + hygiene, subsequent weekly runs update via rebase preserving human commits, PR body per-article review queue (changed, newly cited, supersessions, lint drift), labeled `human-gate` blocked by CODEOWNERS, empty update → no push, closed PR → fresh create, conflicts → `needs-human-resolution`

**Independent Test**: Run distill twice for `governance` with 5 then +2 learnings + simulated human fixup commit → same PR updated, human commit preserved, body reflects 7 learnings, no duplicate, SHA stable when no new learnings

- [ ] T029 [US4] Implement rolling PR create/update orchestrator in `tools/rolling_pr.go` per contracts/rolling-pr.md: per theme branch `dewey-distill/<sanitized>`, if open PR exists via `gh pr list --label dewey-distill --state open` → `git fetch origin/<branch>`, rebase preserving human commits, regenerate draft articles + hygiene, `gh pr edit` body, `git push --force-with-lease` else create branch, write draft articles (`tier: draft` + `validity`/`superseded_by` hygiene), `gh pr create --label human-gate --body ReviewQueue` per FR-011, FR-012, research.md D-005
- [ ] T030 [US4] Implement review queue body renderer in `tools/rolling_pr.go` per contracts/rolling-pr.md Review Queue Body: per article markdown `## <article> (<path>)` + `What changed: <delta>` + `Newly cited: learning/<id> (N)` + `Supersessions: <successor> supersedes <target>` + `Lint: anchor_drift/embedding_gap` from `tools/lint.go` #147 + `Confidence: avg + per-decision distances` per FR-013, research.md D-006, data-model.md ReviewQueue
- [ ] T031 [US4] Enforce rolling PR invariants in `tools/rolling_pr.go`: never write `tier: validated` (server guard rejects, file stays `draft`) per FR-012, contracts/rolling-pr.md step 7; no new learnings → leave open PR untouched no empty push per FR-011 edge, contracts/rolling-pr.md step 6; closed PR for same theme → fresh create never reopen per contracts/rolling-pr.md step 5, data-model.md RollingPR transitions
- [ ] T032 [US4] Implement rebase conflict handling in `tools/rolling_pr.go`: on rebase conflict leave conflict markers, label `needs-human-resolution`, body lists conflict files per contracts/rolling-pr.md step 4, FR-015, research.md D-005, edge case human-edited article
- [ ] T033 [US4] Implement human commit preservation detection in `tools/rolling_pr.go`: via `git log` author not `Dewey Distiller`, ensure rebased history retains human edits, never silently overwrite per FR-015, research.md D-005, SC-006, data-model.md RollingPR.human_commits_preserved
- [ ] T034 [US4] Ensure PR labeling and merge protection documentation in `tools/rolling_pr.go` and `README.md`: label `human-gate` hands-off + CODEOWNERS `.uf/dewey/compiled/**` + ruleset 26764121 required code-owner review per FR-014, research.md D-005, FR-019
- [ ] T035 [US4] Create fixture tests in `tools/rolling_pr_test.go`: two themes → two PRs draft only; second run updates same PR preserves fixup commit; empty update → SHA stable; closed → new PR; conflict → label + body notes; attempted validated write → rejected per contracts/rolling-pr.md Tests, SC-004, SC-005, SC-006, SC-007

---

## Phase 7: User Story 5 - Distiller Agent with Narrowed Tool Allowlist via Global Config (Priority: P2)

**Goal**: Distiller agent (`/dewey-distill-auto` in `autonomy` pane) narrowed to knowledge-store tools only via global `opencode-supervisor` `command: dewey-distill-auto` profile (jflowers/opencode-supervisor#51), repo-local widening ignored, denies `question` and any `tier: validated` write

**Independent Test**: Global profile denies `question`/`validated` → rejected + logged; repo-local adds `question` → still denied; interactive `/dewey-distill` retains both HUMAN_GATE questions + full tool set

- [ ] T036 [US5] Implement allowlist enforcement in `server.go` and `.opencode/agents/distiller.md`: allowed `read`, `list_pages`, `get_page`, `lint`, `compile`, `curate`, `verify_candidates`, `store_compiled` (draft only), `supersede_learning` filesystem git reads; denied `question` (any) rejected + logged fallback to deterministic scope, `promote` to validated denied, any `tier: validated` write server guard in `tools/promote.go`/`store/` rejects per FR-016, FR-017, contracts/distiller-allowlist.md Allowed/Denied, research.md D-007
- [ ] T037 [US5] Implement global-only config precedence rule in `server.go` or `gate.go`: only global `opencode-supervisor` `command: dewey-distill-auto` profile may narrow/widen, repo-local `.opencode/supervisor.yaml` additions for distiller ignored, verified by config-precedence test where repo-local `question` add still denied per FR-016, contracts/distiller-allowlist.md Widening Rule, research.md D-007, plan.md Constraints
- [ ] T038 [US5] Verify interactive `/dewey-distill` unchanged in `.opencode/agents/curator.md` and `.opencode/commands/dewey-distill.md`: retains both `HUMAN_GATE:` prompts + full curator tool set without restriction per FR-018, contracts/distiller-allowlist.md Interactive, FR-019 boundary
- [ ] T039 [US5] Create allowlist tests in `tools/scope_test.go` or `server_test.go`: distiller `question` → reject; distiller validated write → reject remains draft; repo-local widen → ignored; interactive → both gates present per contracts/distiller-allowlist.md Tests, SC-003

---

## Phase 8: User Story 6 - Human Vouch via Merge and Interactive Distill Preservation (Priority: P2)

**Goal**: Vouch = merge of rolling theme PR containing `tier: validated` approved by code owner via #203 blob equality (existing behavior, not reimplemented), `dewey status`/`doctor` reflect validated tier, PR merged at draft stays draft, reviewer suggestion via `gh-hud` feedback applies validated edit, interactive `/dewey-distill` both HUMAN_GATE questions preserved

**Independent Test**: Open fixture theme PR draft, simulate reviewer `tier: validated` suggestion via `gh-hud` feedback → only validated edit applied, after merge `dewey doctor` shows validated per #203; interactive `/dewey-distill` shows both HUMAN_GATE questions

- [ ] T040 [US6] Document and rely on existing #203 git-is-source-of-record trust derivation (no code reimplementation) in `vault/DeriveEffectiveTier` + `gitref/TierBlobResolver` + `dewey status`/`dewey doctor` display: merge of CODEOWNERS-approved PR with `tier: validated` → re-index derives `validated` via blob equality, `dewey doctor` shows validated, merged at draft stays draft eligible for future re-promotion per FR-019, spec.md US6 AS1-2, research.md References, plan.md Constraints
- [ ] T041 [US6] Validate `gh-hud` feedback task integration for reviewer vouch in rolling PR: reviewer posts `tier: validated` via suggestion/review comment → `gh-hud` feedback task updates frontmatter on PR branch, unrelated comment produces no file change per spec.md US6 IS, data-model.md RollingPR transitions
- [ ] T042 [US6] Add e2e verification in fixture repo per SC-007: schedule fires → issue opened/dispatched → scope computed → theme PRs opened → human merges one → `dewey doctor` shows `validated` per #203 without manual intervention beyond final merge; separate run verifies interactive `/dewey-distill` both HUMAN_GATE questions appear with full tool set per FR-018, SC-007, contracts/distiller-allowlist.md Interactive

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: One-time backlog bootstrap, edge cases, and documentation finalization

- [ ] T043 Implement one-time backlog bootstrap support in `tools/rolling_pr.go` or CLI flag handling: themes seeded from 2026-10-25 index partition for ~1,729 uncited learnings, watched pane execution `--backlog-from 2026-10-25 --dry-run` preview then real run per FR-020, research.md D-008, spec.md Edge Cases backlog run, quickstart.md Backlog bootstrap, data-model.md Theme seeded note
- [ ] T044 [P] Handle remaining edge cases per spec.md Edge Cases: policy malformed → warn + skip not fatal; two scheduled runs race → dedup invariant + compare-and-swap push retry; learnings arrive while PR under review → next weekly rebase preserves review threads; embeddings unavailable → tag fallback already covered but verify degraded logging per research.md D-003
- [ ] T045 [P] Validate performance goals in `tools/scope_test.go`/`tools/group_test.go`: scope <2s for ~2k learnings, grouping <5s for 500/week steady state, <30s for 1,729 backlog (watched pane acceptable), PR body <1s per theme per plan.md Performance Goals
- [ ] T046 [P] Update website docs sync issue in `unbound-force/website` repo per Development Workflow: workflow template install, `dewey-distill-auto` command, distiller vs curator agents, distill-policy.yaml, rolling PR body format, backlog bootstrap per constitution Website Documentation Sync
- [ ] T047 Run full CI parity gate per constitution Development Workflow: `go build -v ./...`, `go vet ./...`, `go test -race -count=1 -coverprofile=coverage.out ./...`, `gaze crap --max-crapload=48 --max-gaze-crapload=45` and verify all four reviewers (Adversary, Architect, Guard, Tester) APPROVE before PR per constitution Development Workflow Review Council Gate
- [ ] T048 Verify no orphaned code/unused dependencies per Behavioral Constraints Zero-Waste: template is copy-not-install, no Feature Zombie bloat, 37 inherited graphthulhu MCP tools still work identically per AGENTS.md Neighborhood Rule

---

## Dependencies & Execution Order

**Phase Dependencies**

- Setup (Phase 1): No dependencies - can start immediately
- Foundational (Phase 2): Depends on Setup completion - BLOCKS all user stories
- User Stories (Phase 3-8): All depend on Foundational checkpoint
  - US1 (P1) → US2 (P1) → US3 (P1) → US4 (P1) are logically sequential for e2e but can be developed in parallel with interface mocks
  - US5 (P2) and US6 (P2) can run in parallel with US1-US4 after Foundational (different files: server.go/gate.go vs workflow/policy)
  - US1 workflow template is independent of Go pipeline; can parallelize with scope/group development
- Polish (Phase 9): Depends on all desired user stories being complete

**User Story Dependencies**

- US1 (Weekly Trigger): No Go dependency beyond template + docs; enables gh-hud dispatch contract
- US2 (Scope): Depends on Foundational types + gitref ls-tree; required for US3/US4
- US3 (Grouping): Depends on US2 ScopeSet + embeddings/tag fallback + supersession links
- US4 (Rolling PRs): Depends on US3 Themes + US2 Scope; produces review queue per article
- US5 (Distiller Allowlist): Independent of US1-US4 pipeline but must gate Phase 9 verification; enforces SC-003 never-question/never-validated
- US6 (Human Vouch): Depends on US4 Rolling PRs + existing #203/#194 infrastructure; validates e2e SC-007

**Within Each User Story**

- Tests (if included) should be written first, then implementation
- Scope → Group → PR is the pipeline order; allowlist and vouch can parallelize
- Commit atomically after each task or logical group

**Parallel Opportunities**

- T002, T003, T004 can run in parallel (different files: policy example, distiller.md, dewey-distill-auto.md)
- T011 (workflow) can run in parallel with T016-T021 (scope Go code)
- T036-T039 (allowlist) can run in parallel with T022-T028 (grouping) and T029-T035 (rolling PR)
- T044, T045, T046 can run in parallel (edge cases, performance, docs)

---

## Parallel Example: User Story 1

```bash
# Launch all parallel setup for US1 together:
Task: "Add dedup job steps to workflow template at .github/workflows/dewey-distill.yml.example"
Task: "Document gh-hud dispatch contract in README.md and AGENTS.md"
Task: "Create fixture dedup invariant test in tools/rolling_pr_test.go"
```

---

## Implementation Strategy

**MVP First (User Stories 1-2)**

1. Complete Phase 1: Setup (workflow template, policy example, agent/command docs, init scaffolding)
2. Complete Phase 2: Foundational (data-model types, policy loader, branch sanitizer, server guard, git fallback)
3. Complete Phase 3: US1 Weekly Trigger → Merged PR validates template dedup (FR-001..FR-004, SC-002, SC-008)
4. Complete Phase 4: US2 Deterministic Scope → Merged PR validates 10/2→8 fixture + no-question invariant (FR-005..FR-007, SC-001, SC-003, FR-021)
5. **Checkpoint**: MVP delivers visible weekly trigger + provably correct scope without human questions

**Incremental Delivery (User Stories 3-4)**

6. Add Phase 5: US3 Grouping → Each story adds value without breaking MVP, test independently (FR-008..FR-010)
7. Add Phase 6: US4 Rolling PRs → Full pipeline payoff: rolling per-theme PRs with review queue (FR-011..FR-015, SC-004..SC-006, SC-007)
8. **Checkpoint**: Feature is usable end-to-end: schedule fires → issue → scope → themes → rolling PRs → human merges

**Maturity & Trust (User Stories 5-6)**

9. Add Phase 7: US5 Distiller Allowlist → Security boundary proven (FR-016..FR-017, SC-003)
10. Add Phase 8: US6 Human Vouch → Trust via #203 merge validated (FR-018..FR-019, SC-007 interactive preservation)
11. Add Phase 9: Polish → Backlog bootstrap (~1,729), edge cases, performance, website sync, CI parity

**Feature Branch Strategy**

- Single feature branch `018-weekly-distill-automation` (Speckit NNN-*) contains all user stories
- Consider sub-branches per user story if parallel development needed, merged back to feature branch via PR
- Each user story can be demoed independently via its fixture tests before merging

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability (US1..US6)
- Each user story should be independently completable and testable per Independent Test in spec.md
- Commit after each task or logical group; mark tasks `[x]` immediately per Development Workflow Task Completion Bookkeeping
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, missing file paths, front-end-only descriptions with no data persistence details
- Dependencies #203/#204/#194/#150/#147 are prerequisites owned elsewhere — document as assumed, do not reimplement tier derivation or CODEOWNERS/ruleset

