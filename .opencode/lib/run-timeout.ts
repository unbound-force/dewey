/**
 * Pure per-run and per-task timeout derivation shared by
 * review-dispatch and unleash delegation tools.
 *
 * Replaces the flat 600 s omitted-argument default with a
 * deterministic function of the work being dispatched: base window,
 * prompt size, plan-entry tier, configured parallel contention
 * (OpenSpec change derive-per-run-timeout, design decisions D1/D2/D5,
 * issue #849), and the manifest-mode retrieval allowance (OpenSpec
 * change review-dispatch-diff-manifest, RD-FR-036, design D8,
 * issue #877).
 *
 * The per-task variant (OpenSpec change per-task-delegation-budgets,
 * design D1, PTB-FR-001, issue #1080) derives the wall clock for a
 * `/uf.unleash` implementation-phase `uf_delegate_command` delegation
 * from the same base, tier, prompt, and contention signals plus the
 * phase's work shape: sibling task count and files touched. It
 * replaces the flat 30-minute per-task wall clock that lost about
 * 1 h 50 m of a 4-hour outer budget to re-deriving timed-out work.
 *
 * This module is deliberately free of filesystem, clock, and zod
 * access: derivation is a pure function of scalar signals, testable
 * in isolation and usable standalone by any tool (constitution
 * Principles II and IV).
 */

/** Review tier as resolved by the dispatch planner. */
export type TimeoutTier = "lightweight" | "standard" | "heavy"

/**
 * Named derivation coefficients (design D1). Deliberately NOT
 * matrix-configurable in this change; only the ceiling and per-tier
 * base overrides are (design D6). Every derivation input is recorded
 * per run (D5), so recalibration is evidence-driven.
 */
export const PROMPT_FREE_BYTES = 65_536
export const MS_PER_KIB_ABOVE_FREE = 1_500
export const TIER_TERM_MS = {
  lightweight: 0,
  standard: 120_000,
  heavy: 300_000,
} as const
export const CONTENTION_FACTOR_PER_PARALLEL_RUN = 0.15
export const MAX_CONTENTION_MULTIPLIER = 1.5

/**
 * Manifest-mode retrieval allowance (RD-FR-036, design D8): a flat
 * additive constant applied when — and only when — the run's prompt
 * was assembled in manifest delivery mode. Manifest prompts are
 * smaller (manifest, not diff) while the run does more work (tool
 * round-trips plus review); without compensation, manifest-mode runs
 * would be systematically under-timed. Applied AFTER the contention
 * multiplier so an otherwise-identical manifest-mode run exceeds its
 * inline twin by exactly this constant (absent clamping).
 */
export const MANIFEST_RETRIEVAL_ALLOWANCE_MS = 300_000

/**
 * Hard ceiling matching the executor `TimeoutSchema` bound
 * (agent-executor.ts). The derivation can never exceed it.
 */
export const MAX_TIMEOUT_MS = 1_800_000

/** Ceiling applied when the matrix configures none (design D6). */
export const DEFAULT_CEILING_MS = MAX_TIMEOUT_MS

/** Scalar signals the derivation is a pure function of. */
export interface TimeoutDerivationSignals {
  /**
   * Resolved base window in milliseconds (RD-FR-029: the per-tier
   * override for the run's resolved tier when configured, else the
   * scalar matrix limit).
   */
  readonly baseMs: number
  /** The run's prompt size in bytes (persisted prompt file length). */
  readonly promptBytes: number
  /**
   * The plan entry's resolved tier, or null for a tier-less
   * (direct-model) run. The sole tier input to the derivation.
   */
  readonly resolvedTier: TimeoutTier | null
  /**
   * The session change-profile tier, recorded as a provenance signal
   * only — it MUST NOT alter the derivation (design D1).
   */
  readonly sessionProfileTier: TimeoutTier | null
  /**
   * Configured concurrency cap. At or below 1 no contention
   * multiplier applies (redispatch re-runs are sequential, RD-FR-028).
   */
  readonly maxParallelRuns: number
  /** Configured derivation ceiling in milliseconds (design D6). */
  readonly configuredCeilingMs: number
  /**
   * True when the run's prompt was assembled in manifest delivery
   * mode (RD-FR-036): the prompt carries a diff manifest and the
   * reviewer retrieves diff content on demand via read_diff_chunk.
   * Absent/false applies no retrieval allowance.
   */
  readonly manifestMode?: boolean
  /** Explicit per-run timeout argument; when present it wins outright. */
  readonly explicitTimeoutMs?: number
}

/**
 * Run-file-only audit record (design D5). Field names are
 * serialized snake_case to match the established run-record shape
 * (`prompt`, `post_submission_abort`, `discarded_submissions`).
 * Older run files without a `timeout` field remain valid; the strict
 * consolidate mapping deliberately omits it.
 */
export interface TimeoutProvenanceRecord {
  readonly effective_ms: number
  readonly source: "explicit" | "derived"
  readonly base_ms?: number
  readonly prompt_bytes?: number
  readonly prompt_term_ms?: number
  readonly profile_term_ms?: number
  readonly resolved_tier?: TimeoutTier | null
  readonly session_profile_tier?: TimeoutTier | null
  readonly contention_multiplier?: number
  readonly max_parallel_runs?: number
  /** Retrieval allowance applied (RD-FR-036); present only for manifest-mode runs. */
  readonly retrieval_allowance_ms?: number
  readonly ceiling_ms?: number
  readonly clamped?: boolean
  /**
   * Executor outcome fields (run-file audit only): the effective
   * timeout the agent executor actually applied after its
   * delegation-budget clamp, copied from the executor result's
   * `timeout` details. Present only when the run reached the
   * executor and produced timeout details; readers preferring
   * the applied value over the derived one should read these
   * first.
   */
  readonly executor_effective_ms?: number
  readonly executor_budget_clamped?: boolean
}

/** The derivation result: effective window plus its provenance. */
export interface DerivedTimeout {
  readonly effectiveMs: number
  readonly provenance: TimeoutProvenanceRecord
}

/** Merged matrix limits the base window resolves from (RD-FR-029). */
export interface BaseWindowConfig {
  /** Scalar per-run limit in seconds (matrix or code default). */
  readonly per_run_timeout_seconds: number
  /** Optional per-tier base-window overrides in seconds (design D6). */
  readonly per_run_timeout_tiers?:
    | { readonly lightweight?: number; readonly standard?: number; readonly heavy?: number }
    | null
}

/**
 * Prompt-size term: whole KiB above the free window, at
 * `MS_PER_KIB_ABOVE_FREE` per KiB. Prompts at or below 64 KiB add
 * nothing; the ~175 KB incident prompts add about 167 s.
 */
export function promptTermMs(promptBytes: number): number {
  const bytesAboveFree = Math.max(0, promptBytes - PROMPT_FREE_BYTES)
  return Math.floor(bytesAboveFree / 1024) * MS_PER_KIB_ABOVE_FREE
}

/**
 * Tier additive term (design D1): lightweight 0, standard 120_000,
 * heavy 300_000 — and 0 for a tier-less (direct-model) entry. The
 * plan entry's resolved tier is the sole tier input.
 */
export function profileTermMs(resolvedTier: TimeoutTier | null): number {
  return resolvedTier === null ? 0 : TIER_TERM_MS[resolvedTier]
}

/**
 * Contention multiplier: 1 + 0.15 x (maxParallelRuns - 1), capped at
 * 1.5, applied only when the configured cap exceeds 1 (the configured
 * concurrency cap, not observed concurrency).
 */
export function contentionMultiplier(maxParallelRuns: number): number {
  if (maxParallelRuns <= 1) return 1
  // Integer-milli arithmetic keeps serialized values clean (1.45, not
  // 1.4499999999999997) while staying pure and deterministic.
  const milli = Math.min(
    1000 + Math.round(CONTENTION_FACTOR_PER_PARALLEL_RUN * 1000) * (maxParallelRuns - 1),
    Math.round(MAX_CONTENTION_MULTIPLIER * 1000),
  )
  return milli / 1000
}

/**
 * Resolve the base window (RD-FR-029): the per-tier override for the
 * resolved tier when configured, else the scalar limit. Tier-less
 * (direct-model) runs use the scalar limit.
 */
export function resolveBaseWindowMs(tier: TimeoutTier | null, config: BaseWindowConfig): number {
  const overrideSeconds = tier === null ? undefined : config.per_run_timeout_tiers?.[tier]
  return (overrideSeconds ?? config.per_run_timeout_seconds) * 1000
}

/**
 * Shared floor/ceiling clamp for both derivation variants (per-run
 * design D1/D2; per-task PTB-FR-001): the effective window never falls
 * below the base and never exceeds min(configuredCeilingMs,
 * MAX_TIMEOUT_MS). Floor-at-base precedence: a configured ceiling
 * below the base window is raised to the base, so a derivation never
 * reduces a run or task below its base window.
 *
 * @param derivedMs Unclamped derived value.
 * @param baseMs Resolved base window; the derivation floor.
 * @param configuredCeilingMs Configured ceiling in milliseconds.
 * @returns The effective window plus the ceiling it was clamped against.
 */
function clampDerivedMs(
  derivedMs: number,
  baseMs: number,
  configuredCeilingMs: number,
): { readonly ceilingMs: number; readonly effectiveMs: number } {
  const ceilingMs = Math.max(Math.min(configuredCeilingMs, MAX_TIMEOUT_MS), baseMs)
  return { ceilingMs, effectiveMs: Math.min(Math.max(derivedMs, baseMs), ceilingMs) }
}

/**
 * Derive a reviewer run's timeout from the work being dispatched.
 *
 * Composition (RD-FR-028 as modified by review-dispatch-diff-manifest,
 * design D1/D8): round((baseMs + promptTermMs + profileTermMs) x
 * contentionMultiplier), plus the flat manifest-mode retrieval
 * allowance (RD-FR-036) when the run's prompt was assembled in
 * manifest mode — added after the multiplier so the manifest-vs-inline
 * difference is exactly the allowance. Clamped to the floor baseMs and
 * the ceiling min(configuredCeilingMs, MAX_TIMEOUT_MS). Floor
 * precedence: a configured ceiling below the base window is raised to
 * the base, so the derivation never reduces a run below its base
 * window. An explicit timeout argument passes through unchanged with
 * source "explicit" (design D2).
 */
export function derivePerRunTimeoutMs(signals: TimeoutDerivationSignals): DerivedTimeout {
  if (signals.explicitTimeoutMs !== undefined) {
    return {
      effectiveMs: signals.explicitTimeoutMs,
      provenance: { effective_ms: signals.explicitTimeoutMs, source: "explicit" },
    }
  }

  const prompt = promptTermMs(signals.promptBytes)
  const profile = profileTermMs(signals.resolvedTier)
  const multiplier = contentionMultiplier(signals.maxParallelRuns)
  const retrievalAllowance = signals.manifestMode === true ? MANIFEST_RETRIEVAL_ALLOWANCE_MS : 0
  const derived = Math.round((signals.baseMs + prompt + profile) * multiplier) + retrievalAllowance
  const { ceilingMs, effectiveMs } = clampDerivedMs(
    derived,
    signals.baseMs,
    signals.configuredCeilingMs,
  )

  return {
    effectiveMs,
    provenance: {
      effective_ms: effectiveMs,
      source: "derived",
      base_ms: signals.baseMs,
      prompt_bytes: signals.promptBytes,
      prompt_term_ms: prompt,
      profile_term_ms: profile,
      resolved_tier: signals.resolvedTier,
      session_profile_tier: signals.sessionProfileTier,
      contention_multiplier: multiplier,
      max_parallel_runs: signals.maxParallelRuns,
      ...(retrievalAllowance > 0 ? { retrieval_allowance_ms: retrievalAllowance } : {}),
      ceiling_ms: ceilingMs,
      clamped: effectiveMs !== derived,
    },
  }
}

/**
 * Named per-task derivation coefficients (per-task-delegation-budgets
 * design D1). Calibrated against the issue #1080 incident panel:
 * successful per-task delegations ran 6-20 min inside an 11-task
 * phase, and each of the four that outgrew the flat 30-minute wall
 * clock re-derived its work in 12-20 min on retry — per-task work is
 * minutes-scale and phase scope is the missing signal. Deliberately
 * NOT matrix-configurable, matching the per-run coefficients: every
 * derivation input is recorded in the provenance record, so
 * recalibration stays evidence-driven.
 */

/** Sibling tasks at or below this count add no phase-scope overhead. */
export const PHASE_FREE_TASKS = 1

/** Phase-scope charge per task above `PHASE_FREE_TASKS`. */
export const MS_PER_PHASE_TASK_ABOVE_FREE = 60_000

/** Core file set every task touches free of phase-scope overhead. */
export const PHASE_FREE_FILES = 4

/** Phase-scope charge per file above `PHASE_FREE_FILES`. */
export const MS_PER_PHASE_FILE_ABOVE_FREE = 30_000

/**
 * Scalar work signals the per-task derivation is a pure function of
 * (PTB-FR-001). The base window and ceiling are resolved by the caller
 * exactly as for the per-run variant; the phase shape comes from
 * `parse_tasks` metadata or the phase's `files` list.
 */
export interface PerTaskTimeoutSignals {
  /**
   * Resolved base window in milliseconds. The derivation floor: the
   * effective timeout never falls below it.
   */
  readonly baseMs: number
  /**
   * Task count of the enclosing `tasks.md` phase (`parse_tasks` phase
   * header). A monotonically non-decreasing work signal (PTB-FR-001
   * item 2).
   */
  readonly phaseTaskCount: number
  /**
   * File count the phase declares (phase `files` list or `tasks.md`
   * metadata). A monotonically non-decreasing work signal (PTB-FR-001
   * item 3).
   */
  readonly phaseFileCount: number
  /**
   * The delegation prompt size in bytes (persisted prompt length);
   * 0 when no prompt payload is present.
   */
  readonly promptBytes: number
  /**
   * Resolved throughput tier sharing the per-run `TIER_TERM_MS` terms
   * (PTB-FR-001 item 1). Tier-less (null) contributes zero.
   */
  readonly tier: TimeoutTier | null
  /**
   * Configured concurrency cap of the implement-phase fan-out. At or
   * below 1 no contention multiplier applies.
   */
  readonly maxParallelRuns: number
  /** Configured derivation ceiling in milliseconds. */
  readonly configuredCeilingMs: number
  /** Explicit per-task timeout argument; when present it wins outright. */
  readonly explicitTimeoutMs?: number
}

/**
 * Per-task timeout provenance (PTB-FR-001, mirroring the per-run
 * run-file audit record): every derivation input, the unclamped derived
 * value, the ceiling, and the clamp outcome. Field names serialize
 * snake_case to match the established run-record shape.
 */
export interface PerTaskTimeoutProvenanceRecord {
  readonly effective_ms: number
  readonly source: "explicit" | "derived"
  readonly base_ms?: number
  readonly phase_task_count?: number
  readonly phase_task_term_ms?: number
  readonly phase_file_count?: number
  readonly phase_file_term_ms?: number
  readonly prompt_bytes?: number
  readonly prompt_term_ms?: number
  readonly tier?: TimeoutTier | null
  readonly tier_term_ms?: number
  readonly contention_multiplier?: number
  readonly max_parallel_runs?: number
  /** Unclamped derived value, recorded before floor/ceiling clamping. */
  readonly derived_ms?: number
  readonly ceiling_ms?: number
  readonly clamped?: boolean
}

/** The per-task derivation result: effective window plus provenance. */
export interface DerivedPerTaskTimeout {
  readonly effectiveMs: number
  readonly provenance: PerTaskTimeoutProvenanceRecord
}

/**
 * Phase-size term (PTB-FR-001 item 2): whole tasks above the free
 * window, at `MS_PER_PHASE_TASK_ABOVE_FREE` per task. Phases at or
 * below the free window add nothing; the term is monotonically
 * non-decreasing in the phase task count by construction. The
 * delegated task itself is the free window — its cost is the base
 * window's — so only sibling tasks charge phase-scope overhead.
 *
 * @param phaseTaskCount Task count of the enclosing `tasks.md` phase.
 * @returns Milliseconds charged for the phase's sibling tasks.
 */
export function phaseTaskTermMs(phaseTaskCount: number): number {
  const tasksAboveFree = Math.max(0, phaseTaskCount - PHASE_FREE_TASKS)
  return tasksAboveFree * MS_PER_PHASE_TASK_ABOVE_FREE
}

/**
 * Files-touched term (PTB-FR-001 item 3), the prompt-size analogue
 * from RD-FR-028: whole files above the free window, at
 * `MS_PER_PHASE_FILE_ABOVE_FREE` per file. Monotonically non-decreasing
 * in the phase file count by construction; a small core file set is
 * the baseline cost of any task, while wider phase file sets add
 * read/edit/verify surface.
 *
 * @param phaseFileCount File count the phase declares.
 * @returns Milliseconds charged for the phase's file surface.
 */
export function phaseFileTermMs(phaseFileCount: number): number {
  const filesAboveFree = Math.max(0, phaseFileCount - PHASE_FREE_FILES)
  return filesAboveFree * MS_PER_PHASE_FILE_ABOVE_FREE
}

/**
 * Derive a per-task delegation's timeout from the phase's work signals
 * (per-task-delegation-budgets design D1, PTB-FR-001).
 *
 * Composition: round((baseMs + tierTerm + phaseTaskTerm + phaseFileTerm
 * + promptTerm) x contentionMultiplier), reusing the per-run tier
 * (`TIER_TERM_MS`; zero when tier-less), prompt (`promptTermMs`), and
 * contention (1 + 0.15 x (maxParallelRuns - 1), capped at 1.5, applied
 * only above a configured cap of 1) terms. The result is clamped to
 * the floor `baseMs` and the ceiling min(configuredCeilingMs,
 * MAX_TIMEOUT_MS) with floor-at-base precedence. An explicit timeout
 * argument passes through unchanged with source "explicit".
 *
 * @param signals Per-task work signals; pure — no I/O, clock, or LLM
 *   judgment. Identical inputs produce an identical timeout.
 * @returns The effective window plus its provenance record.
 */
export function derivePerTaskTimeoutMs(signals: PerTaskTimeoutSignals): DerivedPerTaskTimeout {
  if (signals.explicitTimeoutMs !== undefined) {
    return {
      effectiveMs: signals.explicitTimeoutMs,
      provenance: { effective_ms: signals.explicitTimeoutMs, source: "explicit" },
    }
  }

  const tierTerm = profileTermMs(signals.tier)
  const taskTerm = phaseTaskTermMs(signals.phaseTaskCount)
  const fileTerm = phaseFileTermMs(signals.phaseFileCount)
  const promptTerm = promptTermMs(signals.promptBytes)
  const multiplier = contentionMultiplier(signals.maxParallelRuns)
  const derived = Math.round(
    (signals.baseMs + tierTerm + taskTerm + fileTerm + promptTerm) * multiplier,
  )
  const { ceilingMs, effectiveMs } = clampDerivedMs(
    derived,
    signals.baseMs,
    signals.configuredCeilingMs,
  )

  return {
    effectiveMs,
    provenance: {
      effective_ms: effectiveMs,
      source: "derived",
      base_ms: signals.baseMs,
      phase_task_count: signals.phaseTaskCount,
      phase_task_term_ms: taskTerm,
      phase_file_count: signals.phaseFileCount,
      phase_file_term_ms: fileTerm,
      prompt_bytes: signals.promptBytes,
      prompt_term_ms: promptTerm,
      tier: signals.tier,
      tier_term_ms: tierTerm,
      contention_multiplier: multiplier,
      max_parallel_runs: signals.maxParallelRuns,
      derived_ms: derived,
      ceiling_ms: ceilingMs,
      clamped: effectiveMs !== derived,
    },
  }
}
