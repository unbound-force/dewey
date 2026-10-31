/**
 * Pure per-run timeout derivation shared by review-dispatch tools.
 *
 * Replaces the flat 600 s omitted-argument default with a
 * deterministic function of the work being dispatched: base window,
 * prompt size, plan-entry tier, configured parallel contention
 * (OpenSpec change derive-per-run-timeout, design decisions D1/D2/D5,
 * issue #849), and the manifest-mode retrieval allowance (OpenSpec
 * change review-dispatch-diff-manifest, RD-FR-036, design D8,
 * issue #877).
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

  const ceilingMs = Math.max(
    Math.min(signals.configuredCeilingMs, MAX_TIMEOUT_MS),
    signals.baseMs,
  )
  const effectiveMs = Math.min(Math.max(derived, signals.baseMs), ceilingMs)

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
