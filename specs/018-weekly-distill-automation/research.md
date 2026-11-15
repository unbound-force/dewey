# Research: Weekly Unattended Distill

**Feature**: 018-weekly-distill-automation  
**Date**: 2026-11-15  
**Depends on**: #203, #204, #194, #150, #147, jflowers/gh-hud#28, jflowers/opencode-supervisor#51

## Decisions

### D-001: Trigger — shipped template workflow with dedup

**Decision**: Dewey ships `.github/workflows/dewey-distill.yml.example` with `on: schedule` weekly cron (default Mon 09:00 UTC, editable), `workflow_dispatch` manual re-trigger, and `permissions: contents:read, issues:write` (least privilege: the job only reads the repository and creates/comments issues — no PR write). Dedup job lists open issues with label `dewey-distill`; if found posts a comment `<!-- dewey-distill-trigger --> /dewey-distill-auto #N`, else creates the issue with no assignee (the `GITHUB_TOKEN` installation identity cannot resolve `@me`; gh-hud dispatch keys off the label). Copied via `dewey init` docs, not auto-installed.

**Rationale**: Follows existing Dewey pattern: `dewey init` scaffolds `.opencode/agents/curator.md` when `.opencode/` exists, never overwriting. Opt-in copy respects Constitution I (composability) — repos without gh-hud get no-op issues, no failure.

**Alternatives**: Built-in scheduler inside Dewey server — rejected: server is not always running; GH Actions is canonical for weekly.

**Unresolved**: None — adopt from #153 weekly cadence.

### D-002: Scope — git ls-tree primary, store/filesystem fallback

**Decision**: Primary path: `git ls-tree -r HEAD:.uf/dewey/learnings` → identities (filename minus .md), and `git ls-tree -r HEAD:.uf/dewey/compiled` → frontmatter `sources` arrays. Scope = all learning ids − cited ids. Secondary: when no git or no HEAD ref, scan filesystem + SQLite `store` as in `vault` re-ingestion (`tools/lint.go` precedent). Non-git vault reports degraded mode.

**Rationale**: Determinism invariant SC-001 requires same answer without a running server. git object DB is sole trust source per #203; compiled `sources` is the human-vouched citation set. Precedent: `gitref/` already does batch ls-tree + blob hashing.

**Alternatives**: Query SQLite `pages` table — rejected: DB is cache, not vouch.

**Open**: Exact compiled path: `.uf/dewey/compiled/` vs `specs/NNN/plan.md` — confirmed from AGENTS.md: compiled lives under `.uf/dewey/compiled/`.

### D-003: Assignment — embeddings + compile clustering, tag fallback

**Decision**: Embeddings-first assignment (Ollama/Vertex/OpenAI-compatible via `embed.NewEmbedderFromConfig`), fallback tag grouping when unavailable. Reuse existing `tools/compile.go` clustering; add provenance log (embedding distance, rule name, judge identity). Ambiguity tie-breaker: closest centroid deterministically, logged.

**Rationale**: 394 tags with many near-duplicates require semantic grouping. Compile clustering already proven on 1,729 learnings.

**Alternatives**: LLM judge day-one — deferred per Out of Scope; flag-gated future add.

### D-004: Supersession grouping

**Decision**: Consume explicit `supersedes`/`superseded_by` links (#150) as must-same-theme constraint. Grouping step loads links from git frontmatter (same ls-tree pass) and merges components before embedding assignment.

**Rationale**: Additive append-only per Constitution; grouping respects human intent.

### D-005: Rolling PR — per-theme branch with history preservation

**Decision**: Branch `dewey-distill/<theme>` (sanitized tag), label `human-gate`. Update via `git fetch origin/<branch>` + rebase preserving author commits; human commits detected via `git log` author not `Dewey Distiller`. Conflicts → label `needs-human-resolution`, body notes conflict files. Closed PR → new PR. Empty scope → no-op.

**Rationale**: Matches success criteria SC-004..SC-006. CODEOWNERS + ruleset 26764121 blocks bot-only merge.

**Alternatives**: Force-push squash — rejected: drops review commits (violates FR-015).

### D-006: Review queue body

**Decision**: Render Markdown table per article: summary (diff of cited learnings), newly cited ids, supersessions, `dewey lint` drift findings. Generated server-side; body written via `gh` API with `human-gate` label.

**Rationale**: Body is primary human review surface.

### D-007: Distiller allowlist via global config

**Decision**: Agent `distiller` (`opencode/agents/distiller.md`, command `dewey-distill-auto`) lists allowed tools (`read`, `lint`, `compile`, `store`, `verify_candidates` etc.), denies `question` + any `tier: validated` write (enforced server-side guard in `tools/promote.go`). Widening only from global `opencode-supervisor` `command: dewey-distill-auto` profile (#51); repo-local additions ignored.

**Rationale**: Prevents privilege escalation via malicious PR editing local config.

**Alternatives**: Repo-local allowlist — rejected per US-5 IS-2.

### D-008: Backlog bootstrap

**Decision**: One-time watched-pane run seeding themes from 2026-10-25 index partition; subsequent weekly schedule steady state (~500/week). Documented in quickstart.

### D-009: Policy file

**Decision**: Optional `.uf/dewey/distill-policy.yaml` with `exclude_tags: [glob]` + `exclude_stores: [name]`. Malformed entry → warn + skip, per edge case.

## References

- #203 git-is-source-of-record: `vault.DeriveEffectiveTier` + `gitref.TierBlobResolver`
- #194 CODEOWNERS human-gate: `.uf/dewey/compiled/**`
- #150 explicit supersession
- #147 anchor drift lint
- #210 workspace-layout trust display (`.uf/dewey/compiled`) — pending follow-up documented in US6
- jflowers/gh-hud#28 dispatch, jflowers/opencode-supervisor#51 command-scoped profile
- Existing: `.opencode/agents/curator.md`, `.opencode/commands/dewey-distill.md` (292 lines), `tools/compile.go`, `store/` schema v3
