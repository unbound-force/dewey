/**
 * Shared gh failure classification for every plugin that shells out to
 * the gh CLI (OpenSpec change fix-feedback-reply-followups, design D2).
 *
 * Before this module, the two gh consumers classified the same failure
 * two ways: feedback-replies' `classifyGhError` mapped gh exit code 4
 * to `gh_auth_failed` (not retryable), matching gh's documented
 * convention, while review-dispatch's `ghFailureError` mapped the same
 * exit code to `rate_limited` (retryable). One failure class produced
 * opposite retry decisions depending on which plugin reported it
 * (issue #1023, MEDIUM 3). Both consumers now delegate here, so the
 * decision table exists exactly once and adapter parity is pinned by
 * tests.
 *
 * Semantics, first match wins:
 * 1. spawn `ENOENT` → `gh_not_found` (not retryable)
 * 2. spawn `ETIMEDOUT`, `SIGTERM`, or "timed out"/"timeout" text →
 *    `timeout` (retryable)
 * 3. "rate limit" text → `rate_limited` (retryable)
 * 4. "gh auth login" / "not logged" text → `gh_auth_failed`
 * 5. gh exit code 4 → `gh_auth_failed`
 * 6. anything else → `api_error`
 *
 * Stderr text keys outrank exit code 4: gh documents exit 4 as "if a
 * command requires authentication" (`gh help exit-codes`), but `gh
 * api` also exits 4 on other HTTP 4xx responses whose stderr names
 * the real condition (for example rate limits), so the text is the
 * stronger signal and the exit code is the fallback.
 *
 * This module is deliberately free of process spawning and
 * filesystem access: classification is a pure function over a signals
 * record, testable in isolation (constitution Principle IV).
 */

/** Machine-parseable failure code for one gh CLI invocation. */
export type GhErrorCode =
  | "gh_not_found"
  | "timeout"
  | "rate_limited"
  | "gh_auth_failed"
  | "api_error"

/** Raw signals of a failed gh invocation, as produced by its executor. */
export interface GhFailureSignals {
  /** The gh process's stderr text, when captured. */
  readonly stderr?: string
  /** The thrown error's message, used as a secondary text-key source. */
  readonly message?: string
  /** The gh process's exit code (gh documents 4 as auth-required). */
  readonly exitCode?: number
  /** The executor's spawn error code (`ENOENT`, `ETIMEDOUT`). */
  readonly spawnCode?: string
  /** The termination signal, when the process was killed (`SIGTERM`). */
  readonly signal?: string
}

/** Classification of one gh failure: the code plus its retryability. */
export interface GhErrorClassification {
  readonly code: GhErrorCode
  readonly retryable: boolean
}

/**
 * Classify a gh CLI failure into the shared code and retryability.
 *
 * Both adapters — feedback-replies' `classifyGhError` and
 * review-dispatch's `ghFailureError` — MUST route through this
 * function so the two plugin families report identical
 * classifications for identical failures (FRR-FR-001).
 *
 * @param signals The raw failure signals; every field is optional and
 *   absent fields simply do not match their keys.
 * @returns The classification. Never throws.
 */
export function classifyGhFailure(signals: GhFailureSignals): GhErrorClassification {
  const text = `${signals.message ?? ""}\n${signals.stderr ?? ""}`.toLowerCase()

  if (signals.spawnCode === "ENOENT") {
    return { code: "gh_not_found", retryable: false }
  }
  if (
    signals.spawnCode === "ETIMEDOUT" ||
    signals.signal === "SIGTERM" ||
    text.includes("timed out") ||
    text.includes("timeout")
  ) {
    return { code: "timeout", retryable: true }
  }
  if (text.includes("rate limit")) {
    return { code: "rate_limited", retryable: true }
  }
  if (text.includes("gh auth login") || text.includes("not logged")) {
    return { code: "gh_auth_failed", retryable: false }
  }
  // Fallback authentication signal: gh's documented exit code for
  // commands that require authentication. Text keys above outrank it.
  if (signals.exitCode === 4) {
    return { code: "gh_auth_failed", retryable: false }
  }
  return { code: "api_error", retryable: false }
}
