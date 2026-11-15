# Feature Specification: Weekly Unattended Distill with Rolling Per-Theme Review PRs

**Feature Branch**: `018-weekly-distill-automation`

**Created**: 2026-11-15

**Status**: Draft

**Issue**: [#193](https://github.com/unbound-force/dewey/issues/193)

**Depends on**: [#203](https://github.com/unbound-force/dewey/issues/203) (git is the source of record), [#204](https://github.com/unbound-force/dewey/issues/204) (stale validated tier), [#194](https://github.com/unbound-force/dewey/issues/194) (CODEOWNERS human-gate enforcement)

**Epic**: [#153](https://github.com/unbound-force/dewey/issues/153) — Automated knowledge curation

**Input**: User description: "What did we do so far? (feasibility check 018-weekly-distill-automation: unattended weekly /dewey-distill with rolling per-theme review PRs. Part of #153 (automate demotion not promotion, never turn guard fully off, never weaken dev guards, auto-answer never touches curation human-gate questions, re-ingested learnings claiming tier validated treated as draft, trust from git PR vouch, revision append-only supersession). Depends on #203 (git is source of record) and #204. Revised promotion-manifest design — under #203 merged code-owner-approved PR is the vouch, so no manifest or promote --manifest step is needed. Trigger: weekly scheduled GitHub Action (Dewey ships template workflow), scope is deterministic no question, assignment and supersession grouping, rolling per-theme review PRs behind human gates, distiller agent with narrowed tool allowlist via global config (never calls question tool, never writes validated tier)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Weekly Scheduled Trigger Opens or Re-triggers a Distill Issue (Priority: P1)

A knowledge repository has been accumulating learnings via daily `chore(dewey)` PRs from fleet machines. The maintainer wants curation to happen automatically every week without remembering to run it. A scheduled GitHub Action fires weekly; it checks for an open issue labeled `dewey-distill`. If one already exists, the workflow posts a comment to re-trigger dispatch rather than opening a duplicate. If none exists, it opens a new issue assigned to the maintainer. `gh-hud` picks up the issue and dispatches `/dewey-distill-auto #{num}` as the `autonomy` agent in a TUI pane (jflowers/gh-hud#28). The maintainer sees exactly one active distill request at any time, never a growing queue of stale issues.

**Why this priority**: Without a reliable, non-duplicating trigger, unattended distill never starts. Everything else depends on the run being initiated. This is the entry point and the narrowest slice that delivers visible progress.

**Independent Test**: Configure the template workflow in a fixture repo. Run the workflow twice without closing the issue. Verify the second run posts a comment on the existing issue and does not create a second open issue. Close the issue and run again; verify a new issue is created. Check that at no point are there two open issues with label `dewey-distill`.

**Acceptance Scenarios**:

1. **Given** no open issue labeled `dewey-distill` exists, **When** the scheduled workflow runs, **Then** a new issue is opened with label `dewey-distill`, assigned to the repository maintainer, containing the dispatch command reference.
2. **Given** an open issue labeled `dewey-distill` already exists, **When** the scheduled workflow runs again, **Then** no new issue is created and a comment is posted on the existing issue to re-trigger dispatch.
3. **Given** `gh-hud` observes a newly opened or re-triggered `dewey-distill` issue, **When** the `autonomy` agent processes it, **Then** `/dewey-distill-auto #{num}` is dispatched in a TUI pane (per jflowers/gh-hud#28).
4. **Given** the template workflow file exists in `.github/workflows/dewey-distill.yml`, **When** a user forks or initializes a new knowledge repo with `dewey init`, **Then** the workflow template is available to copy/enable (documented in README/AGENTS.md).

---

### User Story 2 - Deterministic Scope Computation with No Interactive Question (Priority: P1)

An unattended distill run must decide what to curate without asking a human. The scope is the set of learnings no current compiled article's `sources` cites — computed entirely from git (`git ls-tree` on learning and compiled article paths). An optional `.uf/dewey/distill-policy.yaml` can exclude specific tags or knowledge stores; without it, the default is all uncited learnings. The run never invokes the Dewey `question` tool. The scope matches a hand-count on a fixture repository where the set of cited identities is known, proving determinism.

**Why this priority**: Unattended mode cannot safely answer an interactive scope question. A deterministic, git-derivable scope is the safety invariant that prevents the run from guessing or skipping content. All grouping and PR generation depends on a correct scope set.

**Independent Test**: Create a fixture git repo with known learnings (identities `learning-1` through `learning-10`) and two compiled articles citing `learning-2` and `learning-5`. Run scope computation from git alone (no running server). Verify the returned in-scope set is exactly `{learning-1, learning-3, learning-4, learning-6, learning-7, learning-8, learning-9, learning-10}`. Add a policy file excluding tag `scratch-*` and re-run; verify excluded identities are removed. Verify no call to the `question` tool occurs in server logs.

**Acceptance Scenarios**:

1. **Given** a repository with 10 learnings and 2 compiled articles citing 2 of them, **When** scope is computed, **Then** the in-scope set equals all learning identities minus every cited identity (8 in this case).
2. **Given** `.uf/dewey/distill-policy.yaml` excludes tag `scratch-*`, **When** scope is computed, **Then** learnings matching the exclusion are removed from the in-scope set before grouping.
3. **Given** no policy file exists, **When** scope is computed, **Then** the default applies (all uncited learnings, no exclusions) and no error is raised.
4. **Given** an unattended `/dewey-distill-auto` run is executing, **When** any code path would invoke the Dewey `question` tool, **Then** the tool is not called (verified by log assertion in tests) and the run proceeds with the deterministic scope.
5. **Given** the computed scope is empty (all learnings already cited), **When** the run completes scope computation, **Then** it exits without opening or updating any PR and records a no-op result.

---

### User Story 3 - Assignment and Supersession Grouping Into Themes (Priority: P1)

Once scope is known, the run groups in-scope learnings into existing compiled articles (or proposes new ones) using embeddings plus the current compile clustering logic, and applies explicit supersession per #150 (append-only, human-confirmed links). The run starts without an LLM judge; if ambiguity is high, an optional LLM judge (Jev, called server-side from Dewey, every decision logged with confidence) can be added later behind a flag. Tag sprawl (394 tags, many near-duplicates) is the expected first beneficiary — similar tags are assigned to the same theme. Each group maps to one theme, which becomes one rolling PR.

**Why this priority**: Grouping determines what the human reviews. Incorrect assignment fragments themes across PRs or merges unrelated knowledge, increasing review burden. This story transforms an unordered set of learnings into reviewable themes.

**Independent Test**: Prepare a fixture repo with 20 uncited learnings across 4 semantic clusters (e.g., auth, governance, review-system, chunker) and 3 existing compiled articles covering auth, governance, and review-system. Run assignment logic. Verify each learning is assigned to the geometrically closest article, that a new theme is proposed for the cluster with no existing article (chunker), and that any explicit `supersedes` links in the learnings are preserved in the grouping metadata. Verify every assignment decision is logged with its source (embedding distance or fallback rule) and confidence.

**Acceptance Scenarios**:

1. **Given** in-scope learnings exist and compiled articles exist, **When** assignment runs, **Then** each learning is grouped into the best-matching existing article or a new-theme bucket using embeddings and current compile clustering.
2. **Given** two in-scope learnings carry an explicit `supersedes` link per #150, **When** grouping completes, **Then** both learnings are placed in the same theme and the supersession is recorded in the theme's metadata.
3. **Given** assignment produces a grouping, **When** the grouping is written to the run's working state, **Then** every decision includes its provenance (embedding distances, rule name, or judge identity) and a confidence value.
4. **Given** no LLM judge is configured, **When** assignment encounters ambiguous candidates within a narrow distance margin, **Then** it applies a deterministic tie-breaker (closest centroid) and logs the ambiguity without calling an external model.

---

### User Story 4 - Rolling Per-Theme PRs with Review Queue Body (Priority: P1)

Each theme from assignment produces one branch `dewey-distill/<theme>` and one PR. The PR contains regenerated compiled articles at `tier: draft` (never `validated` — validated is honored only once merged under #203) and machine-side learning hygiene (dedup, `superseded` markers, `stale` flags). Each subsequent weekly run updates the open PR for its theme via rebase or in-place update, preserving the human's review commits and comments, instead of opening a duplicate PR. The PR body is the review queue: per article, what changed in what we know, which learnings it newly cites, applied supersessions, and lint drift per #147. The PR is labeled `human-gate` and is never auto-merged; CODEOWNERS + required code-owner review (per #194, ruleset 26764121) blocks merge until a code owner approves.

**Why this priority**: The PR is the human gate. Without rolling PRs, the maintainer drowns in duplicates; without a rich review queue body, review is uninformed. This is the payoff of the entire pipeline.

**Independent Test**: Run distill twice on a fixture repo with 5 new learnings for theme `governance`. After the first run, verify a PR exists on branch `dewey-distill/governance` with a body containing per-article diffs, newly cited identities, and lint findings, and that its articles have `tier: draft`. Add a human review commit (simulating a fixup) to that PR. Run distill again with 2 additional learnings. Verify the same PR is updated (rebased onto the new base), the human's commit is preserved (not dropped), the body reflects the new total (7 learnings), and no second PR for `governance` was created. Verify a closed PR for the same theme is not reopened — a new one is created instead.

**Acceptance Scenarios**:

1. **Given** assignment produced themes `governance` and `review-system`, **When** the run creates PRs, **Then** two branches exist (`dewey-distill/governance`, `dewey-distill/review-system`) each with one PR containing draft-tier compiled articles and hygiene edits.
2. **Given** a previous run's PR for `governance` is still open, **When** a new weekly run completes for `governance`, **Then** the existing PR is updated (rebased or force-pushed with history preservation), not duplicated.
3. **Given** the PR body is rendered, **When** a reviewer reads it, **Then** they see per article: a summary of what changed in what we know, the list of newly cited learning identities, applied supersessions, and lint drift findings per #147.
4. **Given** the PR is labeled `human-gate`, **When** the bot is the only approver, **Then** the PR remains blocked from merge until a code owner approves (per #194 CODEOWNERS on `.uf/dewey/compiled/**`).
5. **Given** a second weekly run finds no new learnings for a theme whose PR is still open, **When** the run completes, **Then** that theme's PR is left unchanged (no empty push).
6. **Given** a theme's PR was previously closed (merged or abandoned), **When** new learnings map to the same theme, **Then** a fresh PR is opened for that theme.

---

### User Story 5 - Distiller Agent with Narrowed Tool Allowlist via Global Config (Priority: P2)

The distill run executes as a dedicated `distiller` agent in the `autonomy` pane. Its tool allowlist is narrowed to knowledge-store tools only — it cannot call the Dewey `question` tool and it can never write `tier: validated`. The narrowing is granted only from the user's global `opencode-supervisor` config via a `command: dewey-distill-auto` profile (jflowers/opencode-supervisor#51), not from repository-local config. This ensures a compromised or misconfigured repo cannot widen the agent's privileges.

**Why this priority**: Security boundary. If the agent could write `validated` or ask interactive questions unattended, it could bypass the human gate or hang indefinitely. Narrowing from global config (not repo-local) prevents privilege escalation via a malicious PR that edits local config.

**Independent Test**: Configure a global supervisor profile that grants `distiller` only `read`, `compile`, `lint`, and `store` tools, and denies `question` and `promote --manifest` (or any validated-write). Run an unattended distill. Verify the server rejects any attempt to call `question` and any attempt to set `tier: validated` in a compiled file frontmatter (both fail with a permissions error). Modify the repo-local config to add `question` to the allowlist and re-run; verify the repo-local grant is ignored and the call still fails — only the global profile governs.

**Acceptance Scenarios**:

1. **Given** the `distiller` agent runs via `/dewey-distill-auto`, **When** it attempts to call the Dewey `question` tool, **Then** the call is rejected and logged, and the run continues via its deterministic fallback.
2. **Given** the `distiller` agent attempts to write `tier: validated` into any file frontmatter or via any MCP tool, **When** the write is attempted, **Then** it is rejected and the file retains `tier: draft`.
3. **Given** the agent's allowlist is defined in global `opencode-supervisor` config under `command: dewey-distill-auto`, **When** repo-local config attempts to widen the allowlist, **Then** the repo-local additions are ignored; only the global profile controls the allowlist.
4. **Given** interactive `/dewey-distill` is invoked by a human (not via `dewey-distill-auto`), **When** it runs, **Then** it retains both `HUMAN_GATE:` questions and the full curator tool set without restriction.

---

### User Story 6 - Human Vouch via Merge and Interactive Distill Preservation (Priority: P2)

The vouch for `validated` trust is the merge of a rolling theme PR that contains `tier: validated` in the page frontmatter, approved by a code owner per #194 — this is purely the existing #203 git-is-source-of-record behavior operating normally, not a separate gate this feature creates. A human reviewer may set `tier: validated` on articles they accept (via suggestion or by asking the agent through review comments, acted on by `gh-hud`'s `feedback` task), or they may merge at `draft` if still evaluating. Separately, interactive `/dewey-distill` remains fully functional with both `HUMAN_GATE:` questions for on-demand human-initiated curation; unattended runs do not degrade or alter the interactive experience.

**Known limitation (workspace-layout trust display)**: #203's shipped trust derivation covers vault-root `compiled/` and `knowledge/` page families only. Distill articles live at `.uf/dewey/compiled/<theme>.md` (the workspace layout, per data-model.md), so today the merge-vouch for them is recorded in git — the merged, code-owner-approved PR on the article path — but is not yet surfaced by `dewey status`/`dewey doctor`, which report those pages as `draft` pending the follow-up that extends #203's pathspec coverage (unbound-force/dewey#210). This feature documents the limitation and verifies the derivation mechanism directly (see the Independent Test, AS1, and SC-007); it does not reimplement or extend #203's derivation.

**Why this priority**: Trust must remain rooted in a human-approved merge, not in unattended automation. And the existing interactive workflow must not regress when unattended mode is added — users who prefer manual curation should see no change.

**Independent Test**: Open a fixture theme PR with draft articles. Simulate a reviewer adding a `tier: validated` suggestion on one article and an unrelated review comment ("please clarify X") on another. Run the `gh-hud` feedback task. Verify only the validated-tier edit is applied and the unrelated comment produces no file change. After merge, the vouch is the merged, code-owner-approved PR recorded in git on the article path; because distill articles live in the `.uf/dewey/compiled/` workspace layout that #203's doctor/status display does not yet cover (see the known limitation above), the test verifies the derivation mechanism directly — `vault.DeriveEffectiveTier` resolved with a workspace pathspec (the `tools/curate.go` `IndexKnowledgeStore` precedent, which passes its own store-relative pathspec to `NewPageBlobResolver`) yields `validated` for the first article and `draft` for the second — rather than asserting the `dewey doctor` display. Separately, run interactive `/dewey-distill` from a human pane and verify both `HUMAN_GATE:` questions appear and the full tool set is available.

**Acceptance Scenarios**:

1. **Given** a rolling PR contains `tier: validated` articles and has code-owner approval, **When** it is merged to the default branch, **Then** the vouch is the merged, code-owner-approved PR recorded in git, and the #203 blob-equality derivation mechanism yields `validated` for the article — verified directly via `vault.DeriveEffectiveTier` with a workspace pathspec, since distill articles live under `.uf/dewey/compiled/`. `dewey status`/`dewey doctor` surface this derivation for vault-root `compiled/`/`knowledge/` layouts today; the workspace-layout display is pending unbound-force/dewey#210 (see the known limitation above).
2. **Given** a rolling PR is merged while still at `tier: draft`, **When** re-indexing occurs, **Then** the pages remain `draft` and are eligible for future re-promotion via another PR.
3. **Given** a reviewer posts `tier: validated` via suggestion or review comment, **When** the `gh-hud` feedback task processes it, **Then** the frontmatter is updated on the PR branch.
4. **Given** interactive `/dewey-distill` is invoked, **When** it reaches the scope and promotion gates, **Then** both `HUMAN_GATE:` questions are presented and answered by the human.

---

### Edge Cases

- **No new learnings since last run**: Scope is empty. The workflow records a no-op, leaves all open theme PRs untouched, and does not post spurious comments or open empty PRs.
- **Backlog run (1,729 uncited learnings)**: The first real run operates on a volume far beyond a normal weekly batch (~500/week at peak). The spec permits a one-time backlog mode (e.g., split by themes from the 2026-10-25 index) run in a watched pane to bootstrap rolling PRs; the recurring weekly schedule handles steady state afterward.
- **Repository is not a git repo or has no default-branch ref**: Scope computation falls back to filesystem scan; if git is unavailable, the run reports degraded mode and does not attempt blob-equality checks.
- **Policy file is malformed or references non-existent tags/stores**: The run logs a warning, ignores the invalid exclusion entry, and proceeds with the remaining valid scope — mirroring existing policy-handling behavior, not aborting the entire run.
- **Two scheduled runs race (e.g., manual re-trigger while scheduled trigger is executing)**: The dedup invariant still holds — at most one open `dewey-distill` issue — and concurrent branch updates use a compare-and-swap push that retries on conflict, so no data is lost.
- **A theme's compiled article was manually edited by a human between weekly runs**: The rebase preserves the human's commits; if a conflict arises, the run creates a conflict marker commit and labels the PR `needs-human-resolution` rather than silently overwriting the human's work.
- **Learnings arrive while a theme PR for that theme is under review**: The next weekly run rebases the PR to include the new learnings, without closing or resetting the existing review thread; GitHub's review-comment threading preserves its context across rebases.
- **Embeddings are unavailable during assignment**: Assignment falls back to tag-based grouping (learnings with the same tag go to the same theme) and logs degraded mode, rather than failing the run.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Dewey MUST ship a reusable GitHub Actions template workflow at a documented path (e.g., `.github/workflows/dewey-distill.yml.example` or `templates/`) that triggers on a weekly schedule and can be copied into a knowledge repository.
- **FR-002**: The template workflow MUST implement dedup logic: if an open issue labeled `dewey-distill` exists, the workflow posts a comment to re-trigger dispatch instead of opening a new issue; otherwise it opens a new issue labeled `dewey-distill` assigned to the repository maintainer.
- **FR-003**: At any point in time, at most one open issue with label `dewey-distill` MUST exist (invariant enforced by the workflow's dedup logic).
- **FR-004**: The system MUST integrate with `gh-hud` dispatch so that `gh-hud` observing the issue dispatches `/dewey-distill-auto #{num}` as the `autonomy` agent in a TUI pane (jflowers/gh-hud#28). Dewey's responsibility is to expose any contract `gh-hud` needs (e.g., issue label, assignee, or expected command text) and document it; `gh-hud` owns the dispatch side.
- **FR-005**: Scope computation MUST be deterministic and computable from git alone: the set of all learning identities minus the set of identities cited in any compiled article's `sources`, obtained via `git ls-tree` (or equivalent) on the default branch.
- **FR-006**: Scope computation MUST never invoke the Dewey `question` tool when running via `/dewey-distill-auto`, regardless of ambiguity or policy absence.
- **FR-007**: The system MUST support an optional policy file at `.uf/dewey/distill-policy.yaml` that can exclude tags or knowledge stores from scope; when the file is absent, the default scope (all uncited learnings) applies without error.
- **FR-008**: The system MUST group in-scope learnings into themes mapped to existing compiled articles or new-theme buckets, using embeddings and the current compile clustering logic.
- **FR-009**: The grouping step MUST incorporate explicit supersession links per #150, placing supersession-related learnings in the same theme and recording the link in theme metadata.
- **FR-010**: Every grouping decision MUST be logged with its provenance (embedding distances, rule name, or judge identity if an LLM judge is present) and a confidence value.
- **FR-011**: For each theme, the run MUST create or update a branch `dewey-distill/<theme>` and a single associated PR. If an open PR for that theme already exists, the run MUST update it (rebase or in-place update) rather than create a duplicate. If no open PR exists, it MUST create a new one.
- **FR-012**: Each rolling PR MUST contain regenerated compiled articles at `tier: draft` and machine-side learning hygiene edits (dedup, `superseded`, `stale` markers). The run MUST never write `tier: validated` into any file.
- **FR-013**: Each rolling PR's body MUST serve as a review queue containing, per article: a summary of what changed in what we know, the list of newly cited learning identities, applied supersessions, and lint drift findings per #147.
- **FR-014**: Each rolling PR MUST be labeled `human-gate` and MUST be blocked from auto-merge; this is enforced via CODEOWNERS on `.uf/dewey/compiled/**` and required code-owner review (ruleset 26764121 per #194), which this spec documents but does not re-implement.
- **FR-015**: When a previous run's PR for a theme is still open, a subsequent run MUST preserve human review commits and comments through rebase — human edits MUST NOT be silently overwritten. Conflicts MUST be surfaced (e.g., `needs-human-resolution` label) rather than auto-resolved by dropping human changes.
- **FR-016**: The distiller agent (`/dewey-distill-auto`) MUST operate with a narrowed tool allowlist granted only from the global `opencode-supervisor` config via a `command: dewey-distill-auto` profile (jflowers/opencode-supervisor#51). Repository-local config MUST NOT be able to widen the allowlist.
- **FR-017**: The narrowed allowlist MUST deny the Dewey `question` tool and any capability to write `tier: validated`; attempts MUST be rejected and logged.
- **FR-018**: Interactive `/dewey-distill` MUST remain unchanged, retaining both `HUMAN_GATE:` questions and the full curator agent tool set. Unattended mode MUST NOT alter interactive behavior.
- **FR-019**: The merge of a rolling PR containing `tier: validated` articles (per existing #203 git-is-source-of-record semantics) MUST be the sole mechanism by which `validated` trust is conferred. This spec MUST NOT introduce a new validation gate or bypass the one established in #194/#203.
- **FR-020**: The system MUST support a one-time backlog bootstrap: on the first real run with a large uncited inventory (e.g., ~1,700+ learnings since 2026-10-25), themes are seeded from the existing index's theme partition, preferably executed in a watched pane before transitioning to the weekly steady state.
- **FR-021**: When scope is empty, the run MUST exit without opening or updating any PR and MUST record a no-op result — no empty PRs and no spurious comments.
- **FR-022**: Dewey MUST expose or document the workflow template's required secrets, permissions, and issue-label conventions so that a knowledge repository can adopt the workflow without reverse-engineering.

### Key Entities

- **Distill Run**: A single execution of `/dewey-distill-auto`, triggered by a `dewey-distill` issue. Owns a scope set, a theme assignment, and a set of rolling PR updates. Identified by the issue number and run timestamp. Records no-op when scope is empty.
- **Scope Set**: The deterministic set of learning identities to distill in this run: all learning identities minus all identities cited in any compiled article `sources`. Optionally filtered by policy exclusions. Computed from git blob state alone.
- **Distill Policy**: Optional file at `.uf/dewey/distill-policy.yaml` declaring tag or knowledge-store exclusions from scope. Absent means no exclusions. Malformed entries are warned and skipped, not fatal.
- **Theme**: A cluster of in-scope learnings mapped to one existing (or new) compiled article topic. One theme equals one rolling PR. Themes are seeded from the existing index's partition on the backlog run.
- **Rolling PR**: A long-lived pull request on branch `dewey-distill/<theme>` that accumulates draft-tier articles and hygiene edits across weekly runs. Labeled `human-gate`; protected by CODEOWNERS. Updated via rebase, never duplicated while open.
- **Review Queue**: The PR body for a rolling PR. Structured per article to include knowledge-change summary, newly cited learning identities, supersessions, and lint drift per #147. This is the human's primary review surface.
- **Distiller Agent**: The narrowed-privilege agent that executes `/dewey-distill-auto` in the `autonomy` pane. Tool allowlist derives solely from global `opencode-supervisor` config (`command: dewey-distill-auto` profile). Cannot call `question`; cannot write `validated`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a fixture repo with a known cited/uncited inventory, scope computed from git alone matches a hand-counted scope set for the same data (exact set equality) — 100% of fixtures in the test suite pass.
- **SC-002**: Across any sequence of scheduled triggers and manual re-triggers, at most one open issue with label `dewey-distill` exists at any time (invariant holds over a 4-week simulated schedule with overlapping triggers in tests).
- **SC-003**: Unattended runs never call the `question` tool and never write `tier: validated` into any file — verified by log/tool-call assertions over all test runs (0 violations).
- **SC-004**: A second weekly run with no new learnings since the prior run leaves all open theme PRs unchanged (no new commits, no body edits) — verified by branch SHA stability.
- **SC-005**: A weekly run with new learnings for a theme that already has an open PR updates the existing PR (single PR per theme remains) rather than creating a duplicate — verified by PR count stability (PR count for that theme stays at 1).
- **SC-006**: A rebase of a theme PR that carries a simulated human fixup commit preserves that commit in the rebased history (human edits not dropped) — verified by commit message preservation in fixtures.
- **SC-007**: A repository that adopts the shipped template workflow and has `gh-hud` installed can complete one end-to-end cycle (schedule fires → issue opened/dispatched → scope computed → theme PRs opened → human merges one → the merged, code-owner-approved PR is the vouch recorded in git, and the #203 blob-equality derivation mechanism — verified directly via `vault.DeriveEffectiveTier` with a workspace pathspec matching the distill articles' `.uf/dewey/compiled/` layout — yields `validated` for the merged article) without manual intervention beyond the final merge — demonstrated on a fixture repo. The `dewey doctor`/`dewey status` display of that derivation covers vault-root layouts today; the workspace-layout display is pending unbound-force/dewey#210 (see US6's known limitation).
- **SC-008**: The template workflow is installable from Dewey's repository without copying internal implementation details: the adopting repo copies one file and sets one label convention, documented in a single setup paragraph — verified by the existence of the template file at its documented path and a passing install-from-scratch test.

## Assumptions

- The adopting repository is a knowledge repository with a git history, a default branch, and the CODEOWNERS/human-gate enforcement from #194 already in place — or is willing to add it as part of adopting this feature (documented as a prerequisite). Without #194, rolling PRs labeled `human-gate` would not be merge-protected; the feature degrades to "PRs are opened but nothing prevents bot-only approval."
- The default branch protection and code-owner approval are repository-settings edits (ruleset 26764121) performed by an admin once; they are not code and are not committed by this feature. This follows the precedent set in #194.
- `#203` (git is the source of record for `validated` trust) and `#204` (stale `validated` tier handling) are implemented before or alongside this feature; this feature relies on merge-as-vouch semantics and does not reimplement tier derivation. Note that #203's shipped `dewey status`/`dewey doctor` display covers vault-root `compiled/`/`knowledge/` layouts only — the `.uf/dewey/` workspace-layout display gap is documented in US6 and tracked as unbound-force/dewey#210.
- `#150` (explicit supersession) is available for the grouping step to consume `supersedes` links; this feature does not define a new supersession mechanism.
- `#147` (anchor drift) lint findings are available for inclusion in the review queue body; if `#147` is not yet implemented, the body includes whatever lint outputs `dewey lint` currently provides.
- Fleet machines continue to produce learnings via daily `chore(dewey)` PRs; the weekly cadence (~500/week at recent peak) is the expected steady-state volume, with the initial backlog (~1,700) handled as a one-time bootstrap.
- The distiller agent's privilege narrowing depends on `opencode-supervisor` supporting a `command: dewey-distill-auto` profile in global config (jflowers/opencode-supervisor#51). This is a hard prerequisite for unattended mode: if the supervisor does not yet support command-scoped profiles, unattended mode is not enabled — the workflow MUST NOT dispatch as `distiller` and instead records a no-op artifact plus a notice comment `supervisor command profile unavailable, unattended distill skipped` and exits. The intended contract is documented as `command: dewey-distill-auto` denying `question` and any `tier: validated` write; no "tightest achievable" degradation is permitted that would allow repo-local widening (FR-016/FR-017 fail closed). Tracked follow-up jflowers/opencode-supervisor#51 removes the block.
- Interactive `/dewey-distill` continues to be the human-initiated curation path; unattended mode is additive and does not change interactive behavior, per #153's "automate demotion not promotion" principle — the unattended path only produces `draft` outputs for human review.
- The scheduled workflow runs weekly (exact day/time is a template default, configurable by the adopting repo); the spec does not mandate a specific cron expression, only that one is provided as a sensible default (e.g., Monday UTC).
- Learnings remain at `tier: draft` forever (per #203's design and #153's "trust from git PR vouch" rule); per-learning review is infeasible at ~73/day and is not attempted. Humans vouch for compiled articles that cite learnings.

## Dependencies & Relationship to Epic #153

This feature is part of **#153** (Automated knowledge curation) and obeys its governance rules:

- **Automate demotion, not promotion**: Unattended runs never produce `validated` content. They surface stale/superseded signals and produce draft articles for human promotion.
- **Never turn the guard fully off**: Dev guards and human gates (`human-gate` label, CODEOWNERS, required code-owner review) are never disabled by this feature — the `human-gate` label instructs tooling to stay hands-off, and CODEOWNERS enforces the gate.
- **Never weaken dev guards**: No change to branch protection, CI checks, or review thresholds.
- **Auto-answer never touches curation human-gate questions**: Unattended mode has no answers to supply; scope is deterministic, not asked.
- **Re-ingested learnings claiming `tier: validated` are treated as `draft`**: Preserved from #145/#203; this feature does not reintroduce file-based tier elevation for learnings.
- **Trust from git PR vouch**: Under #203, a merged, code-owner-approved PR containing `tier: validated` is the vouch. This feature does not invent a parallel trust mechanism.
- **Revision append-only supersession**: Supersession links are additive per #150; this feature groups them but does not mutate their append-only semantics.

## Out of Scope

- Changes to interactive `/dewey-distill` behavior — it keeps both `HUMAN_GATE:` questions and the full tool set.
- Reimplementation of `validated` tier derivation or CODEOWNERS/ruleset configuration — owned by #203 and #194 respectively.
- A new promotion manifest or `promote --manifest` mechanism — superseded per the 2026-11-13 revision; merge-as-vouch per #203 replaces it.
- `gh-hud` dispatch implementation — owned by jflowers/gh-hud#28; this spec defines only the contract Dewey exposes (label, expected command text, assignee convention).
- `opencode-supervisor` command-scoped profile implementation — owned by jflowers/opencode-supervisor#51; this spec defines only the contract Dewey requires (command name `dewey-distill-auto`, required denials for `question` and `validated` writes).
- Automatic merging of rolling PRs — all rolling PRs require human merge.
- LLM judge tuning — the initial implementation runs without a judge; adding one is a documented future enhancement triggered by observed ambiguity, not a blocker.
