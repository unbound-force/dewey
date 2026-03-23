<!--
  Sync Impact Report
  ==================
  Version change: 1.1.0 → 1.2.0 (MINOR: Testability expanded, CI
    updated, Governance strengthened)
  Amendment date: 2026-03-23

  Modified principles:
    - IV. Testability: Added coverage ratchet enforcement rule and
      CRITICAL-severity missing-coverage-strategy rule (adopted from
      org constitution v1.1.0)

  Modified sections:
    - Development Workflow: CI now includes "build, lint, vet, tests"
      (was "build, vet, tests") to reflect MegaLinter addition
    - Governance: Added explicit Compliance Review rule requiring
      Constitution Check at each planning phase (spec, plan, tasks)

  Unchanged principles:
    - I. Composability First
    - II. Autonomous Collaboration
    - III. Observable Quality

  Unchanged sections:
    - Upstream Stewardship
    - Development Standards

  Templates requiring updates:
    - .specify/templates/plan-template.md ✅ compatible
    - .specify/templates/spec-template.md ✅ compatible
    - .specify/templates/tasks-template.md ✅ compatible

  Constitution check: ALIGNED (all 4 principles match org constitution)

  Previous version history:
    - 1.1.0 (2026-03-22): Added Development Workflow section (spec-first,
      review council gate, branching, CI, releases, commit messages)
    - 1.0.0 (2026-03-22): Initial ratification with 4 principles,
      upstream stewardship, development standards, governance
-->

# Dewey Constitution

## Core Principles

### I. Composability First

Dewey MUST be independently installable and usable without any other
Unbound Force tool. A developer MUST be able to run `dewey serve` on
any Markdown vault and get full MCP functionality without installing
OpenCode, Swarm, Gaze, or any other ecosystem component.

Every agent persona MUST treat Dewey as an optional enhancement, not
a hard dependency. When Dewey MCP tools are unavailable, agents MUST
fall back to direct file reads and CLI queries with reduced but
functional capability.

**Rationale**: The Unbound Force ecosystem grows by composition, not
coupling. If Dewey becomes a hard dependency, a failure in Dewey
blocks the entire swarm. Graceful degradation preserves autonomy.

### II. Autonomous Collaboration

Dewey communicates with agent personas exclusively through MCP tool
calls and their structured responses. Dewey MUST NOT require runtime
coupling, shared memory, or direct function calls from other tools.

All indexed content MUST be accessible through well-defined MCP tools
with documented input schemas and structured JSON responses. Agents
discover and use Dewey's capabilities through the MCP tool registry,
not through internal APIs or shared libraries.

**Rationale**: Artifact-based (tool-call-based) communication allows
any MCP client to use Dewey without knowledge of its internals. This
enables independent evolution of Dewey and its consumers.

### III. Observable Quality

Every query result MUST include provenance metadata: source type,
document origin, fetch timestamp, and relevance score where applicable.
Agents MUST be able to assess result quality and attribution without
inspecting Dewey's internals.

The `health` and `dewey status` tools MUST report index state: page
count, source freshness, embedding coverage, and backend type. The
system MUST be auditable at rest -- a human or agent can inspect
`.dewey/` artifacts to understand what is indexed and when it was
last updated.

**Rationale**: AI agents make decisions based on retrieved context.
If they cannot assess the freshness, source, or relevance of that
context, they risk acting on stale or misattributed information.

### IV. Testability

Every package MUST be testable in isolation without requiring external
services. The vault package uses local `testdata/` fixtures, not a
running Obsidian or Logseq instance. The embedding integration MUST
be testable without a running Ollama server (via interface mocking
or pre-computed fixture embeddings).

New features MUST include tests that exercise the feature's contract.
Test assertions MUST verify behavior, not implementation details.
Tests MUST pass with `go test ./...` on a clean checkout with no
external dependencies beyond the Go toolchain.

Coverage ratchets MUST be enforced by automated checks in CI; any
coverage regression MUST be treated as a build failure and block
the pull request. Missing coverage strategy in a spec or plan is
a CRITICAL-severity finding and MUST be resolved before
implementation begins.

**Rationale**: A project that cannot be tested without external
services cannot be reliably developed, reviewed, or CI-validated.
The fork inherits graphthulhu's test discipline and MUST maintain it.
AI agents generate code rapidly; if that code is not structurally
testable, the system will collapse under its own unverified
complexity.

## Upstream Stewardship

Dewey is a hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu)
by Max Skridlevsky. The original MIT license and copyright MUST be
retained in the LICENSE file alongside the Unbound Force copyright.

The `upstream` git remote MUST be preserved for cherry-picking bug
fixes and improvements from the original project. When cherry-picking
upstream commits, the original authorship MUST be preserved in the
git history.

New Dewey features (persistence, vector search, content sources)
MUST NOT break graphthulhu-compatible functionality. The existing 37
MCP tools MUST continue to work with the same input/output contracts.
An existing MCP client configured for graphthulhu MUST be able to
switch to Dewey by changing only the server name in its configuration.

## Development Standards

**Language**: Go (version specified in `go.mod`). All code MUST pass
`go build ./...`, `go test ./...`, and `go vet ./...` without errors.

**Protocol**: MCP (Model Context Protocol) via stdio transport is
the primary interface. HTTP transport is supported as an alternative.
New capabilities MUST be exposed as MCP tools, not as CLI subcommands
or library APIs.

**Storage**: SQLite for persistent indexes (knowledge graph and
vector embeddings). No external database servers. All state MUST be
contained within the `.dewey/` directory in the repository root.

**Embedding**: Ollama API for local model inference. No data MUST
leave the developer's machine for core functionality. The embedding
model MUST be configurable (default: `granite-embedding:30m`).

**Backend interface**: All MCP tools MUST program against the
`backend.Backend` interface, not concrete client implementations.
Adding a new backend MUST NOT require changes to existing tools.

**Dependencies**: Minimize external dependencies. New dependencies
MUST be justified by a clear need that cannot be met by the Go
standard library or existing dependencies. CGO dependencies MUST be
avoided unless no pure-Go alternative exists for a critical feature.

## Development Workflow

- **Spec-First Development**: All changes that modify production code,
  test code, agent prompts, embedded assets, or CI configuration MUST
  be preceded by a spec workflow (either the Speckit pipeline under
  `specs/` or the OpenSpec pipeline under `openspec/changes/`). The
  spec artifacts (proposal, design, tasks at minimum) MUST exist
  before implementation begins. This ensures every change has a
  planning record, a reviewable intent, and a traceable rationale.
  Exempt from this requirement:
    - Constitution amendments (governed by the Governance section below)
    - Trivial fixes: typo corrections, comment-only changes, and
      single-line formatting fixes that do not alter behavior
    - Emergency hotfixes: critical production bugs where the fix is
      a single well-understood correction (must be retroactively
      documented)
  When in doubt, use a spec. The cost of an unnecessary spec is
  minutes; the cost of an unplanned change is rework, drift, and
  broken CI.
- **Branching**: All work MUST occur on feature branches. Direct
  commits to the main branch are prohibited except for trivial
  documentation fixes.
- **Code Review**: Every pull request MUST receive at least one
  approving review before merge.
- **Review Council Gate**: Before submitting a pull request, agents
  MUST run the `/review-council` command and receive an APPROVE
  verdict from all reviewers (Adversary, Architect, Guard). Any
  REQUEST CHANGES findings MUST be resolved before PR submission.
  There MUST be minimal to no code changes between the council's
  APPROVE and the PR submission -- the council reviews the code
  that will be submitted, not a draft that changes afterward.
- **Continuous Integration**: The CI pipeline MUST pass (build, lint,
  vet, tests) before a pull request is eligible for merge.
- **Releases**: Follow semantic versioning (MAJOR.MINOR.PATCH).
  Breaking changes to MCP tool contracts or backend interfaces
  require a MAJOR bump.
- **Commit Messages**: Use conventional commit format
  (`type: description`) to enable automated changelog generation.

## Governance

This constitution is the authoritative source for project principles
and development standards. All pull requests and code reviews MUST
verify compliance with these principles.

Amendments to this constitution require:
1. A written proposal documenting the change and its rationale
2. An assessment of impact on existing code and downstream consumers
3. A migration plan if the change affects existing behavior
4. Version increment following semantic versioning:
   - MAJOR: Principle removal or incompatible redefinition
   - MINOR: New principle or materially expanded guidance
   - PATCH: Clarifications, wording fixes, non-semantic changes

Complexity beyond what these principles permit MUST be justified in
the Complexity Tracking section of the implementation plan. The
justification MUST explain why a simpler alternative was rejected.

**Compliance Review**: At each planning phase (spec, plan, tasks),
the Constitution Check gate MUST verify that the proposed work
aligns with all active principles. Constitution violations are
CRITICAL severity and non-negotiable.

**Version**: 1.2.0 | **Ratified**: 2026-03-22 | **Last Amended**: 2026-03-23
**Parent Constitution**: unbound-force org constitution v1.1.0
