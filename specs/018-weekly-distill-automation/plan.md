# Implementation Plan: Weekly Unattended Distill with Rolling Per-Theme Review PRs

**Branch**: `018-weekly-distill-automation` | **Date**: 2026-11-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/018-weekly-distill-automation/spec.md`

## Summary

Automate curation **demotion, never promotion**: a weekly GitHub Action fires, dedups to one open `dewey-distill` issue, `gh-hud` dispatches `/dewey-distill-auto` as the `autonomy` distiller agent; scope is deterministic from git (`learning` identities minus `compiled` `sources` via `git ls-tree`), grouped into themes via embeddings + current compile clustering (with #150 supersession links preserved), and materialized as rolling `dewey-distill/<theme>` PRs at `tier: draft` with machine hygiene and a rich review-queue body (lint drift #147, newly cited identities, supersessions). Trust remains git-is-source-of-record (#203) — merge of a CODEOWNERS-approved PR carrying `tier: validated` is the only vouch; no manifest or `promote --manifest` exists. The distiller's allowlist is narrowed to knowledge-store tools only via global `opencode-supervisor` `command: dewey-distill-auto` profile, denying `question` and any validated write.

## Technical Context

**Language/Version**: Go 1.25.0 (module `github.com/unbound-force/dewey/v3`) + GitHub Actions YAML (workflow template) + Markdown (agent commands, policy file)

**Primary Dependencies**: `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` v1.4.1 (MCP), `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/charmbracelet/log` (logging), `github.com/fsnotify/fsnotify`, `github.com/k3a/html2text`, `golang.org/x/oauth2`, existing `llm/`, `embed/`, `openaicompat/`, `store/`, `vault/`, `gitref/`, `sanitize/`, `curate/` packages; external contracts: `jflowers/gh-hud#28` (issue dispatch), `jflowers/opencode-supervisor#51` (global `command:` profile), `gh` CLI (workflow dedup)

**Storage**: SQLite ` .uf/dewey/graph.db` + git object DB (scope via `git ls-tree` on `learning/` and `compiled/` paths) + markdown files `.uf/dewey/learnings/*.md`, `.uf/dewey/compiled/*.md`, `.uf/dewey/knowledge/**`, optional `.uf/dewey/distill-policy.yaml`

**Testing**: `go build -v ./...`, `go vet ./...`, `go test -race -count=1 -coverprofile=coverage.out ./...`, `gaze crap --max-crapload=48 --max-gaze-crapload=45`; fixture repos for git-only scope, tie-breaker, rebase preservation; log/tool-call assertions for never-call invariants; Gaze AI report on main only

**Target Platform**: Linux/macOS developer machines, GitHub Actions `ubuntu-latest` runners, MCP stdio transport (server holds `dewey.lock`); vault may be non-git or git

**Project Type**: CLI + MCP server + GitHub automation template + Opencode agent (`distiller`/`curator` variant)

**Performance Goals**: Scope computation <2s for ~2k learnings (git `ls-tree` + frontmatter parse); grouping O(n*m) with embeddings, <5s for 500/week steady state, <30s for one-time 1,729 backlog bootstrap (watched pane acceptable); PR body generation <1s per theme; weekly schedule — not latency-critical

**Constraints**: Pure Go, no CGO; local-only processing by default (cloud LLM opt-in via config.yaml + env-only keys); never write `tier: validated` in unattended mode; never invoke `question` tool; never auto-merge; `human-gate` label hands-off + CODEOWNERS `.uf/dewey/compiled/**` + ruleset 26764121 required code-owner review; existing 37 graphthulhu + current MCP tools must remain compatible; backend behind `backend.Backend`; `gitref/` blob-equality is trust authority per #203; re-ingested learnings claiming `tier: validated` forced to `draft` per #145/#203

**Scale/Scope**: ~1,729 uncited learnings backlog (2026-10-25 index), ~500/week steady-state peak, 394 tags (near-duplicates), themes map 1:1 to rolling PRs `dewey-distill/<theme>`; at most one open `dewey-distill` issue repo-wide; PR bodies per-article (knowledge delta, cited identities, supersessions, lint drift)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Verdict | Evidence |
|-----------|------|---------|----------|
| I. Composability First | No hard dependency on `gh-hud`/supervisor/opencode; graceful fallback if unavailable | PASS | Workflow template is opt-in copy (`dewey init` ships example); scope fallback to filesystem scan when not git; embeddings fallback to tag grouping when unavailable; MCP unavailable → direct file reads |
| II. Autonomous Collaboration | All agent communication via MCP tool calls, no shared memory | PASS | Distiller uses `lint`, `list_pages`, `compile`, `curate`, `verify_candidates`, `store_compiled`, `supersede_learning`, `promote` via MCP; background index respects `indexReady` + shared mutex; no new direct function coupling |
| III. Observable Quality | Every result carries provenance; index state auditable via `health`/`status`/`doctor` | PASS | Grouping decisions log provenance (embedding distance / rule / judge + confidence); PR bodies expose newly cited identities, supersessions, drift; durable run artifacts (`.uf/dewey/distill-runs/`) audit every run incl. no-ops; `dewey status`/`doctor` report trust tiers via `DeriveEffectiveTier` + blob equality for vault-root `compiled/`/`knowledge/` layouts today — distill articles' `.uf/dewey/compiled` workspace layout is displayed pending dewey#210 (US6 known limitation); compile `coverage` surfaced |
| IV. Testability | Every package testable isolation; no external service required; contract tests required | PASS | Scope has git-only pure function + filesystem fixture tests; allowlist has unit tests denying `question`/`validated`; rebase preservation via fixture repos with temp git dirs; `NoopSynthesizer`/`anchorChecker` seams retained; coverage ratchets enforced in CI |
| **Development Workflow** | Spec-first, branch naming, review council, CI parity, task bookkeeping, docs sync | PASS | Feature branch `018-weekly-distill-automation` (Speckit `NNN-*`); spec exists before plan; PR requires council APPROVE *all four*; CI parity `go build/vet/test -race` + gaze thresholds replicated locally; tasks marked `[x]` immediately; website sync issues for new workflow template |

No violations to justify — Complexity Tracking empty.

## Project Structure

### Documentation (this feature)

```text
specs/018-weekly-distill-automation/
├── plan.md              # This file (/speckit.plan output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (MCP + workflow + PR body)
│   ├── scope.md
│   ├── grouping.md
│   ├── rolling-pr.md
│   ├── workflow-template.md
│   └── distiller-allowlist.md
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Dewey flat package layout — this feature touches:
.github/
└── workflows/
    └── dewey-distill.yml.example   # shipped template (FR-001/FR-022)

workflow_templates.go               # embedded deweyWorkflowTemplates map — the template's scaffold source
                                    # (dewey init writes the example into adopting repos; never overwrites)

.uf/dewey/
├── distill-policy.yaml.example     # optional exclusions (FR-007)

opencode/
├── agents/
│   ├── curator.md                  # existing: /dewey-distill interactive
│   └── distiller.md               # new: /dewey-distill-auto unattended (allowlist doc)
└── commands/
    ├── dewey-distill.md           # existing interactive — unchanged
    └── dewey-distill-auto.md      # new: unattended entry point (also embedded in slash_commands.go)

server.go                           # register new MCP tool(s) if needed (scope/dry-run)
tools/
├── scope.go / scope_test.go        # deterministic scope from git ls-tree + backlog_from filter
├── group.go / group_test.go        # theme assignment + supersession coercion
├── rolling_pr.go / rolling_pr_test.go # PR create/update/rebase + body render + dry-run preview
└── verify.go (existing)            # reused for per-theme review queue

vault/                              # Vault.GetPage / tier derivation interaction
gitref/                             # blob equality authority, policy file hashing
store/                              # pages/blocks schema v3 — no schema change; tier validated only via merge
cli.go / gate.go                    # dewey init ships workflow template
```

**Structure Decision**: Single-project (DEFAULT) — Dewey is a flat Go module (`github.com/unbound-force/dewey/v3`) with MCP server + Cobra CLI. No `src/` split; feature adds a workflow template, an opencode agent/command pair, and Go packages under existing `tools/`, `vault/`, `gitref/` seams. Adopting repos copy one workflow file and one policy example — no new service or sub-project.

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
