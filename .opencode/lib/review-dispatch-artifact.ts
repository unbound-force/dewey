/**
 * Shared review-dispatch artifact contract.
 *
 * Extracts the on-disk artifact knowledge previously duplicated between
 * the review-dispatch plugin (producer: store directory, envelope
 * discriminator) and the uf-workflow plugin (consumer: envelope parsing
 * for resume detection) — council finding MEDIUM, architect run 76c2845a.
 * The duplication's failure mode is silent fail-open: when the consumer's
 * envelope knowledge drifts from the producer's, resume detection reads
 * no valid artifacts and re-runs full review councils.
 *
 * This module is deliberately free of filesystem access: parsing is a
 * pure string function, testable in isolation (constitution
 * Principle IV).
 */

/** Dispatch artifact store relative to the repo root (flat, repo-global). */
export const DISPATCH_ARTIFACT_DIRECTORY = ".uf/artifacts/dispatch"

/** Envelope discriminator for review-dispatch artifacts. */
export const REVIEW_DISPATCH_ARTIFACT_TYPE = "review-dispatch"

/** Decision fields consumed from one dispatch artifact envelope. */
export interface ReviewDispatchArtifactSummary {
  /** Artifact file name in the flat store (recency tie-break). */
  readonly source: string
  /** Review mode: "specs" (spec review) or "code" (code review). */
  readonly mode: string
  /** Branch the dispatch ran against (envelope context.branch). */
  readonly branch: string
  /** Head SHA the dispatch reviewed (payload.input_context.head_sha). */
  readonly headSha: string
  /** Recorded council verdict (payload.verdict). */
  readonly verdict: string
  /** Envelope write time in epoch ms (a missing timestamp sorts oldest). */
  readonly writtenAt: number
}

/** Type guard for non-null plain objects (untyped JSON payload access). */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/**
 * Parse one dispatch artifact envelope into the decision fields resume
 * detection consumes (payload mode/verdict, context branch, input_context
 * head SHA, envelope write timestamp). Returns null for anything that does
 * not match the review-dispatch envelope shape — malformed JSON, a foreign
 * artifact type, or missing decision fields — so callers skip it
 * (fail-open to "not done").
 */
export function parseReviewDispatchArtifact(
  raw: string,
  source: string,
): ReviewDispatchArtifactSummary | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isRecord(parsed)) return null
  if (parsed.artifact_type !== REVIEW_DISPATCH_ARTIFACT_TYPE) return null
  const context = parsed.context
  const payload = parsed.payload
  if (!isRecord(context) || !isRecord(payload)) return null
  const inputContext = payload.input_context
  if (!isRecord(inputContext)) return null
  const mode = payload.mode
  const verdict = payload.verdict
  const branch = context.branch
  const headSha = inputContext.head_sha
  if (
    typeof mode !== "string" ||
    typeof verdict !== "string" ||
    typeof branch !== "string" ||
    typeof headSha !== "string"
  ) {
    return null
  }
  const writtenAt =
    typeof parsed.timestamp === "string" ? Date.parse(parsed.timestamp) : Number.NaN
  return {
    source,
    mode,
    branch,
    headSha,
    verdict,
    // A missing or unparseable timestamp sorts the artifact oldest rather
    // than skipping it: the decision fields above are what make an
    // artifact valid.
    writtenAt: Number.isNaN(writtenAt) ? 0 : writtenAt,
  }
}
