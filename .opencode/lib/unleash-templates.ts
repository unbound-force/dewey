/**
 * Pure renderers for the /uf.unleash exit and demo output templates.
 *
 * Extracts the prescribed terminal output of /uf.unleash — the eleven
 * "## /uf.unleash paused at:" blocks embedded in tier routing and
 * Steps 1, 4, 5, and 6 and the Step 8 (Demo) format block — from
 * command prose into code
 * (OpenSpec change unleash-deterministic-tools, design decision D6), so
 * exits and the demo render identically by construction instead of by
 * re-reading instructions under compressed context.
 *
 * Template source of truth: .opencode/commands/uf.unleash.md. The command
 * file wraps its prose to a <72-character line discipline; these renderers
 * emit each heading, label line, guidance sentence, and list item as a
 * single unwrapped line — semantically identical markdown.
 *
 * This module is deliberately free of filesystem, git, and zod access:
 * rendering is a pure string function over validated data, testable in
 * isolation (constitution Principle IV). The zod input contracts and the
 * tool wrappers live in plugins/uf-workflow/index.ts.
 */

/**
 * Workflow tier value set. Single source for the tier union: the
 * UnleashTier type here and the WorkflowTier type plus every tier
 * z.enum in plugins/uf-workflow/index.ts (ExitTierSchema,
 * DetectResumeInputSchema) all derive from this list, so a value
 * added or renamed in one declaration cannot drift from the rest.
 */
export const WORKFLOW_TIERS = ["speckit", "openspec", "quick"] as const

/** Workflow tier selecting tier-specific template options. */
export type UnleashTier = (typeof WORKFLOW_TIERS)[number]

/**
 * The eleven exit scenarios /uf.unleash pauses at, matching the paused-at
 * template blocks in the command. The variant type derives from this list
 * so the two cannot drift apart. Tier routing precedes every numbered
 * step, so its two variants lead the list in command order; the
 * planner-failure variant follows because review planning precedes the
 * review steps it guards (unleash-pipeline-hardening, audit findings
 * F4/F5/F8: DIRTY_TREE, ROUTE_TIER_TOOLING, and planner INCONCLUSIVE
 * exits were hand-composed because no variant existed).
 */
export const UNLEASH_EXIT_VARIANTS = [
  "tier-routing-dirty-tree",
  "tier-routing-missing-tooling",
  "planner-failure",
  "clarify",
  "spec-review",
  "implement-worker-failed",
  "implement-merge-conflict",
  "implement-phase-checkpoint",
  "code-review-preflight",
  "code-review-no-success",
  "code-review-iterations-exhausted",
] as const

/** Exit variant identifier — which paused-at scenario to render. */
export type UnleashExitVariant = (typeof UNLEASH_EXIT_VARIANTS)[number]

/**
 * Data for the "tier-routing-dirty-tree" exit variant. The route decision
 * tool rejected the recording because the working tree is dirty — after
 * the issue resolution and rubric analysis were already spent, so the exit
 * surfaces the tool's failure message instead of a bare stop.
 */
export interface TierRoutingDirtyTreeExitData {
  /** The uf_unleash_route_decision DIRTY_TREE failure message. */
  readonly message: string
}

/**
 * Data for the "tier-routing-missing-tooling" exit variant. The route
 * decision tool rejected the recording because the chosen tier's entry
 * commands are not scaffolded (ROUTE_TIER_TOOLING); its failure message
 * names the missing paths and the tiers whose entry commands exist.
 */
export interface TierRoutingMissingToolingExitData {
  /** The uf_unleash_route_decision ROUTE_TIER_TOOLING failure message. */
  readonly message: string
}

/**
 * Data for the "planner-failure" exit variant. plan_review_dispatch
 * failed or returned an INCONCLUSIVE plan (fail-closed); the exit
 * surfaces the plan errors, which carry their own remediation since
 * unleash-pipeline-hardening (audit finding F5).
 */
export interface PlannerFailureExitData {
  /** Plan errors from plan_review_dispatch (INCONCLUSIVE or failed). */
  readonly reasons: readonly string[]
}

/** One unanswerable clarification question with its surrounding spec context. */
export interface ClarifyQuestion {
  /**
   * Question text extracted from the marker. Markers take three forms:
   * colon ([NEEDS CLARIFICATION: <question>]), bare-with-text
   * ([NEEDS CLARIFICATION <text>]), and completely bare
   * ([NEEDS CLARIFICATION]); bare/empty forms extract as unscoped
   * questions with the raw marker text as fallback.
   */
  readonly question: string
  /** Surrounding spec.md lines, as scan_clarifications extracted them. */
  readonly context: string
}

/** Data for the "clarify" exit variant. */
export interface ClarifyExitData {
  /** Unanswerable questions presented all at once; the count renders as N. */
  readonly questions: readonly ClarifyQuestion[]
}

/** Data for the "spec-review" exit variant. */
export interface SpecReviewExitData {
  /** Workflow tier selecting the recovery guidance. */
  readonly tier: UnleashTier
  /** Unresolved findings (pause choice or iteration cap), each with context. */
  readonly findings: readonly string[]
  /**
   * Optional change name interpolated into the openspec recovery guidance
   * (openspec/changes/<name>/); the literal placeholder renders when
   * omitted.
   */
  readonly change_name?: string
}

/** Data for the "implement-worker-failed" exit variant. */
export interface ImplementWorkerFailedExitData {
  /** 1-based phase number from tasks.md. */
  readonly phase: number
  /** Failed task ID and description. */
  readonly failed_task: string
  /** Error details from the worker. */
  readonly error: string
}

/** Data for the "implement-merge-conflict" exit variant. */
export interface ImplementMergeConflictExitData {
  /** 1-based phase number from tasks.md. */
  readonly phase: number
  /** Files with conflict markers after the cherry-pick merge. */
  readonly conflicting_files: readonly string[]
}

/** Data for the "implement-phase-checkpoint" exit variant. */
export interface ImplementPhaseCheckpointExitData {
  /** 1-based phase number from tasks.md. */
  readonly phase: number
  /** The command that failed the phase checkpoint. */
  readonly failed_command: string
  /** Error output of the failed command. */
  readonly output: string
}

/** Data for the "code-review-preflight" exit variant. */
export interface CodeReviewPreflightExitData {
  /** Branch-caused failures, each a CRITICAL finding with full output. */
  readonly findings: readonly string[]
}

/** Data for the "code-review-no-success" exit variant. */
export interface CodeReviewNoSuccessExitData {
  /** Which reviewers ran or failed, and why. */
  readonly reasons: readonly string[]
}

/** Data for the "code-review-iterations-exhausted" exit variant. */
export interface CodeReviewIterationsExhaustedExitData {
  /** Findings persisting across iterations, noting fixed vs. remaining. */
  readonly persistent_findings: readonly string[]
  /**
   * Reviewer-check cycles where fixing one reviewer's finding causes
   * another reviewer's check to fail. The "Circular dependencies (if
   * any)" section renders only when present.
   */
  readonly circular_dependencies?: readonly string[]
}

/** Per-variant exit data, keyed by exit variant. */
export type UnleashExitDataMap = {
  readonly "tier-routing-dirty-tree": TierRoutingDirtyTreeExitData
  readonly "tier-routing-missing-tooling": TierRoutingMissingToolingExitData
  readonly "planner-failure": PlannerFailureExitData
  readonly clarify: ClarifyExitData
  readonly "spec-review": SpecReviewExitData
  readonly "implement-worker-failed": ImplementWorkerFailedExitData
  readonly "implement-merge-conflict": ImplementMergeConflictExitData
  readonly "implement-phase-checkpoint": ImplementPhaseCheckpointExitData
  readonly "code-review-preflight": CodeReviewPreflightExitData
  readonly "code-review-no-success": CodeReviewNoSuccessExitData
  readonly "code-review-iterations-exhausted": CodeReviewIterationsExhaustedExitData
}

/** Union of every per-variant exit data shape. */
export type UnleashExitData = UnleashExitDataMap[UnleashExitVariant]

/** Input to the Step 8 (Demo) block renderer. */
export interface UnleashDemoData {
  /** Workflow tier selecting the second Next Steps option. */
  readonly tier: UnleashTier
  /** What Was Built: summary from the spec user stories or change proposal. */
  readonly whatWasBuilt: string
  /** How to Verify: verification commands from quickstart.md or acceptance scenarios. */
  readonly howToVerify: string
  /** Test Results: pass/fail summary with counts. */
  readonly testResults: string
  /**
   * Optional change name interpolated into the openspec second option
   * (openspec/changes/<name>/); the literal placeholder renders when
   * omitted. The three content fields use the write_unleash_summary
   * demo-payload names (whatWasBuilt/howToVerify/testResults) so the same
   * values compose the rendered demo and the persisted summary artifact.
   */
  readonly changeName?: string
}

/**
 * Interpolate the change name into an openspec/changes/<name>/ guidance
 * fragment. The command template's <name> is a placeholder for the change
 * name; when the caller cannot supply one, the literal placeholder renders
 * (the template text verbatim).
 */
function openspecChangePath(changeName?: string): string {
  return changeName === undefined ? "openspec/changes/<name>/" : `openspec/changes/${changeName}/`
}

/**
 * Tier-specific "What to do next" recovery guidance for the spec-review
 * exit (command Step 4): Speckit routes through /speckit.clarify, OpenSpec
 * through the change's artifacts directory. Quick has no spec artifacts to
 * recover into — the branch is unreachable in a well-formed quick run
 * (spec review is skipped), so its guidance addresses the code directly.
 */
function specReviewRecoveryGuidance(tier: UnleashTier, changeName?: string): string {
  if (tier === "speckit") {
    return "Run `/speckit.clarify` to address the findings, then re-run `/uf.unleash`."
  }
  if (tier === "quick") {
    return "Quick changes carry no spec artifacts; address the findings in the code, then re-run `/uf.unleash`."
  }
  return `Update the artifacts under \`${openspecChangePath(changeName)}\` to address the findings, then re-run \`/uf.unleash\`.`
}

/**
 * Tier-selected second Next Steps option for the demo block (command
 * Step 8): Speckit iterates through /speckit.clarify, OpenSpec through
 * the change's artifacts directory, Quick offers a follow-up issue —
 * there are no artifacts to refine and trivial changes should not
 * accrete scope.
 */
function demoRefinementOption(tier: UnleashTier, changeName?: string): string {
  if (tier === "speckit") {
    return "Run `/speckit.clarify` to refine and iterate"
  }
  if (tier === "quick") {
    return "Open a follow-up issue for anything this change surfaced"
  }
  return `Update the artifacts under \`${openspecChangePath(changeName)}\` to refine and iterate`
}

/** Render a string list as markdown bullets, one "- " item per line. */
function bulletList(items: readonly string[]): string {
  return items.map((item) => `- ${item}`).join("\n")
}

/**
 * Assemble the shared paused-at exit skeleton: heading, reason, the
 * step-specific body, "What to do next" guidance, and the "Then resume"
 * line, separated by blank lines exactly as the command templates show.
 */
function assemblePausedExit(parts: {
  readonly step: string
  readonly reason: string
  readonly body: string
  readonly whatToDoNext: string
  readonly resume: string
}): string {
  return [
    `## /uf.unleash paused at: ${parts.step}`,
    "",
    `**Reason**: ${parts.reason}`,
    "",
    parts.body,
    "",
    "### What to do next",
    parts.whatToDoNext,
    "",
    "### Then resume",
    parts.resume,
  ].join("\n")
}

/**
 * Render the tier-routing dirty-tree exit. The route analysis was spent
 * before the recording guard rejected the dirty tree, so the exit body
 * carries the tool's failure message verbatim.
 */
function renderTierRoutingDirtyTreeExit(data: TierRoutingDirtyTreeExitData): string {
  return assemblePausedExit({
    step: "tier routing",
    reason: "Dirty working tree rejected the route decision",
    body: `**Route decision failure**: ${data.message}`,
    whatToDoNext:
      "Inspect `git status`, clean or stash the uncommitted work, then re-run `/uf.unleash` — tier routing re-derives the decision on the next run.",
    resume: "Run `/uf.unleash` to continue from tier routing.",
  })
}

/**
 * Render the tier-routing missing-tooling exit (ROUTE_TIER_TOOLING). The
 * tool's failure message already names the missing entry commands and the
 * tiers whose tooling is scaffolded, so the exit body carries it verbatim.
 */
function renderTierRoutingMissingToolingExit(data: TierRoutingMissingToolingExitData): string {
  return assemblePausedExit({
    step: "tier routing",
    reason: "Chosen tier's entry commands are not scaffolded",
    body: `**Route decision failure**: ${data.message}`,
    whatToDoNext:
      "Re-route at a tier whose entry commands exist (the failure message lists them), or scaffold the missing command, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from tier routing.",
  })
}

/**
 * Render the planner-failure exit. plan_review_dispatch failed or
 * returned an INCONCLUSIVE plan (fail-closed); the exit surfaces the
 * plan errors, which carry their own remediation since
 * unleash-pipeline-hardening (audit finding F5).
 */
function renderPlannerFailureExit(data: PlannerFailureExitData): string {
  return assemblePausedExit({
    step: "review planning",
    reason: "Review dispatch planner failed to produce a ready plan",
    body: `### Plan errors\n\n${bulletList(data.reasons)}`,
    whatToDoNext:
      "Repair the configuration named in the errors (the plan errors carry remediation), then re-run the review step.",
    resume: "Run `/uf.unleash` to continue from the review step.",
  })
}

/** Render the clarify exit: N question(s) with per-question context. */
function renderClarifyExit(data: ClarifyExitData): string {
  const body = data.questions
    .map((entry, index) => `Q${index + 1}: ${entry.question}\nContext: ${entry.context}`)
    .join("\n\n")
  return assemblePausedExit({
    step: "clarify",
    reason: `${data.questions.length} question(s) require human input`,
    body,
    whatToDoNext: "Answer these questions in the spec, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from the plan step.",
  })
}

/** Render the spec-review exit: unresolved findings plus tier guidance. */
function renderSpecReviewExit(data: SpecReviewExitData): string {
  return assemblePausedExit({
    step: "spec review",
    reason: "Unresolved spec review findings (pause or iteration cap)",
    body: `### Findings\n\n${bulletList(data.findings)}`,
    whatToDoNext: specReviewRecoveryGuidance(data.tier, data.change_name),
    resume: "Run `/uf.unleash` to continue from spec review.",
  })
}

/** Render the implement worker-failure exit for the failed phase. */
function renderImplementWorkerFailedExit(data: ImplementWorkerFailedExitData): string {
  return assemblePausedExit({
    step: `implement (Phase ${data.phase})`,
    reason: "Parallel worker failed",
    body: `**Failed task**: ${data.failed_task}\n**Error**: ${data.error}`,
    whatToDoNext: "Fix the issue described above, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from the failed phase.",
  })
}

/** Render the implement merge-conflict exit with the conflicting files. */
function renderImplementMergeConflictExit(data: ImplementMergeConflictExitData): string {
  return assemblePausedExit({
    step: `implement (Phase ${data.phase})`,
    reason: "Worktree merge conflict",
    body: `**Conflicting files**:\n${bulletList(data.conflicting_files)}`,
    whatToDoNext: "Resolve the merge conflicts manually, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from the current phase.",
  })
}

/** Render the phase-checkpoint exit with the failed command and output. */
function renderImplementPhaseCheckpointExit(data: ImplementPhaseCheckpointExitData): string {
  return assemblePausedExit({
    step: `implement (Phase ${data.phase} checkpoint)`,
    reason: `Build or test failure after Phase ${data.phase}`,
    body: `**Failed command**: ${data.failed_command}\n**Output**: ${data.output}`,
    whatToDoNext: "Fix the build/test failure, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from the next phase.",
  })
}

/** Render the code-review pre-flight exit: branch-caused failures. */
function renderCodeReviewPreflightExit(data: CodeReviewPreflightExitData): string {
  return assemblePausedExit({
    step: "code review",
    reason: "Pre-flight gate: branch-caused failures",
    body: `### Findings\n\n${bulletList(data.findings)}`,
    whatToDoNext: "Fix the branch-caused failures, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from code review.",
  })
}

/** Render the no-success exit: which reviewers ran or failed, and why. */
function renderCodeReviewNoSuccessExit(data: CodeReviewNoSuccessExitData): string {
  return assemblePausedExit({
    step: "code review",
    reason: "Code review produced no successful assessment",
    body: `### No-success reasons\n\n${bulletList(data.reasons)}`,
    whatToDoNext: "Address the reviewer-run failures, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from code review.",
  })
}

/**
 * Render the iterations-exhausted exit. The "Circular dependencies (if
 * any)" section renders only when cycles are reported — its heading's
 * "if any" marks the section conditional in the command template.
 */
function renderCodeReviewIterationsExhaustedExit(
  data: CodeReviewIterationsExhaustedExitData,
): string {
  let body = `### Persistent findings\n\n${bulletList(data.persistent_findings)}`
  if (data.circular_dependencies !== undefined && data.circular_dependencies.length > 0) {
    body += `\n\n### Circular dependencies (if any)\n\n${bulletList(data.circular_dependencies)}`
  }
  return assemblePausedExit({
    step: "code review",
    reason: "3 review iterations exhausted",
    body,
    whatToDoNext: "Fix the outstanding findings manually, then re-run `/uf.unleash`.",
    resume: "Run `/uf.unleash` to continue from code review.",
  })
}

/**
 * Per-variant renderers. The mapped type keeps each renderer paired with
 * its variant's data shape, so formatUnleashExit dispatches without casts.
 */
const EXIT_RENDERERS: {
  readonly [V in UnleashExitVariant]: (data: UnleashExitDataMap[V]) => string
} = {
  "tier-routing-dirty-tree": renderTierRoutingDirtyTreeExit,
  "tier-routing-missing-tooling": renderTierRoutingMissingToolingExit,
  "planner-failure": renderPlannerFailureExit,
  clarify: renderClarifyExit,
  "spec-review": renderSpecReviewExit,
  "implement-worker-failed": renderImplementWorkerFailedExit,
  "implement-merge-conflict": renderImplementMergeConflictExit,
  "implement-phase-checkpoint": renderImplementPhaseCheckpointExit,
  "code-review-preflight": renderCodeReviewPreflightExit,
  "code-review-no-success": renderCodeReviewNoSuccessExit,
  "code-review-iterations-exhausted": renderCodeReviewIterationsExhaustedExit,
}

/**
 * Render a /uf.unleash paused-at exit message for one of the eleven exit
 * variants, reproducing the command template's structure exactly: the
 * "## /uf.unleash paused at: <step>" heading, the **Reason** line, the
 * step-specific body, "### What to do next" guidance (tier-specific for
 * spec-review), and the "### Then resume" line.
 *
 * The command emits the returned markdown verbatim; nothing is
 * paraphrased, added, or removed (design decision D6 — the OUTPUT
 * FIDELITY GUARD becomes a structural guarantee).
 *
 * @param variant Exit scenario to render.
 * @param data Per-variant payload; the generic signature pairs each
 *   variant with its data shape at compile time.
 * @returns The rendered exit markdown (no trailing newline).
 */
export function formatUnleashExit<V extends UnleashExitVariant>(
  variant: V,
  data: UnleashExitDataMap[V],
): string {
  return EXIT_RENDERERS[variant](data)
}

/**
 * Render the /uf.unleash Step 8 (Demo) block: What Was Built / How to
 * Verify / Test Results / Next Steps. The Next Steps list carries exactly
 * the two prescribed options — the fixed /uf.finale option and the
 * tier-selected refinement option (Speckit: /speckit.clarify; OpenSpec:
 * update the change's artifacts directory).
 *
 * @param data Demo payload; the content field names match the
 *   write_unleash_summary demo payload (whatWasBuilt/howToVerify/
 *   testResults) so the same values compose the rendered demo and the
 *   persisted summary artifact.
 * @returns The rendered demo markdown (no trailing newline).
 */
export function formatUnleashDemo(data: UnleashDemoData): string {
  return [
    "## What Was Built",
    "",
    data.whatWasBuilt,
    "",
    "## How to Verify",
    "",
    data.howToVerify,
    "",
    "## Test Results",
    "",
    data.testResults,
    "",
    "## Next Steps",
    "",
    "- Run `/uf.finale` to create PR and watch CI",
    `- ${demoRefinementOption(data.tier, data.changeName)}`,
  ].join("\n")
}
