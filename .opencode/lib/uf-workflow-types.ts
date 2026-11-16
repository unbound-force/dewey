/**
 * Result envelope types for uf-workflow plugin tools.
 *
 * Every tool response is wrapped in a discriminated union
 * so callers can branch on `status` without guessing.
 *
 * Error messages SHOULD NOT include file paths or stack
 * traces — keep them user-facing and sanitized.
 */

import { execFileSync } from "node:child_process"

/** Successful tool result with optional typed payload. */
export interface ToolSuccess<T = void> {
  readonly status: "ok"
  readonly data?: T
}

/** Failed tool result with structured error metadata. */
export interface ToolFailure {
  readonly status: "error"
  readonly message: string
  readonly retryable: boolean
  readonly code?: string
  /**
   * Child session id, present only on delegation failures
   * (DELEGATE_FAILED / DELEGATE_CANCELLED) where the executor
   * created a child session before failing. Omitted when no
   * child session ever existed.
   */
  readonly child_session_id?: string
}

/** Discriminated union returned by every tool handler. */
export type ToolResult<T = void> = ToolSuccess<T> | ToolFailure

/** Build a success envelope, optionally carrying data. */
export function success<T = void>(data?: T): ToolSuccess<T> {
  if (data === undefined) {
    return { status: "ok" } as ToolSuccess<T>
  }
  return { status: "ok", data }
}

/** Build a failure envelope. `retryable` defaults to false. */
export function failure(
  message: string,
  options?: { retryable?: boolean; code?: string },
): ToolFailure {
  const result: ToolFailure = {
    status: "error",
    message,
    retryable: options?.retryable ?? false,
    ...(options?.code !== undefined && { code: options.code }),
  }
  return result
}

// ---------------------------------------------------------------------------
// Shared utilities (DRY: used by both uf-workflow and uf-finale plugins)
// ---------------------------------------------------------------------------

/** Git execution function signature for testability (args array, no shell). */
export type GitExec = (args: string[], cwd: string) => string

/** Default git executor using execFileSync (no shell interpolation). */
export const defaultGitExec: GitExec = (args: string[], cwd: string): string =>
  execFileSync("git", args, { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] }).trim()

/** Maximum slug length after sanitization. */
const MAX_SLUG_LENGTH = 100

/**
 * Sanitize a string to a filesystem-safe slug (FR-004).
 * Replaces `/` with `-`, strips characters outside [a-zA-Z0-9._-],
 * collapses dot runs, and truncates to 100 characters.
 * Throws if the result is empty after sanitization.
 */
export function sanitizeSlug(input: string): string {
  const result = input
    .replace(/\//g, "-")
    .replace(/[^a-zA-Z0-9._-]/g, "")
    .replace(/\.{2,}/g, ".")
    .slice(0, MAX_SLUG_LENGTH)
  if (result === "") {
    const safe = input.slice(0, 100).replace(/[\x00-\x1f]/g, "?")
    throw new Error(
      `Invalid input: "${safe}" produces empty slug after sanitization`,
    )
  }
  return result
}

/**
 * Normalize a raw model identifier to a canonical short name.
 * Algorithm (6 steps):
 * 1. Strip everything before and including the last `/`
 * 2. Strip everything after and including the first `@`
 * 3. Strip trailing date suffix `-YYYYMMDD`
 * 4. Repeatedly strip chained trailing `-N` (only when preceded by digit)
 * 5. Validate result contains only `[a-zA-Z0-9._-]`
 * 6. Return `unknown-model` if empty or invalid
 */
export function normalizeModelName(raw: string): string {
  if (!raw || raw.trim() === "") return "unknown-model"

  // Step 1: Strip before last `/`
  let name = raw
  const lastSlash = name.lastIndexOf("/")
  if (lastSlash !== -1) {
    name = name.slice(lastSlash + 1)
  }

  // Step 2: Strip after first `@`
  const atIndex = name.indexOf("@")
  if (atIndex !== -1) {
    name = name.slice(0, atIndex)
  }

  // Step 3: Strip trailing `-YYYYMMDD`
  name = name.replace(/-\d{8}$/, "")

  // Step 4: Repeatedly strip trailing `-N` when preceded by digit
  while (/\d-\d$/.test(name)) {
    name = name.slice(0, -2)
  }

  // Step 5: Validate
  const isValid = /^[a-zA-Z0-9._-]+$/.test(name)

  // Step 6: Fallback
  if (!name || !isValid) return "unknown-model"

  return name
}

// ---------------------------------------------------------------------------
// Spec-review gate record (harden-spec-review-gate, HARDEN-001..006)
//
// Single normative shape for the gate record persisted at
// `data["spec-review"]` of the uf.unleash progress state. Producers and
// consumers (consolidate_dispatch, dispatch_review_runs, checkpoint,
// get_progress, relay_spec_review_gate, uf_unleash_detect_resume) all
// import from here so the shape cannot drift between plugins.
// ---------------------------------------------------------------------------

/**
 * Command whose progress state carries the spec-review gate record.
 * The record lives at `data["spec-review"]` of this command's state file.
 */
export const SPEC_REVIEW_STATE_COMMAND = "uf.unleash"

/** Progress-state data key holding the spec-review gate record. */
export const SPEC_REVIEW_RECORD_KEY = "spec-review"

/**
 * Maximum spec-review fix-loop iterations enforced before dispatch
 * (HARDEN-003): the initial review plus this many re-reviews may dispatch
 * (4 total dispatches at the default), so a dispatch that would start
 * iteration 5 is refused — a persistent REQUEST CHANGES exits through the
 * spec-review paused path instead of re-rolling (uf.unleash fix-loop
 * budget).
 */
export const SPEC_REVIEW_ITERATION_CAP = 3

/** Per-finding adjudication choice recorded against a non-approving iteration. */
export type SpecReviewDecisionChoice = "apply" | "override" | "pause"

/** One human decision covering one finding of the latest iteration (HARDEN-002). */
export interface SpecReviewDecision {
  readonly finding_id: string
  readonly decision: SpecReviewDecisionChoice
  /** Required with `apply`: reference to the applied fix (task/file). */
  readonly fix_reference?: string
  /** Required with `override`: non-empty audit reason. */
  readonly reason?: string
}

/** Audit projection of an override decision (HARDEN-002 override auditing). */
export interface SpecReviewOverride {
  readonly finding_id: string
  readonly reason: string
}

/**
 * The recorded adjudication of one iteration's findings (HARDEN-002). The
 * set carries the iteration it adjudicates, so appending a new iteration
 * leaves it inert for gate purposes — the new iteration's findings must be
 * decided afresh — while the resume probe can still see that the earlier
 * iteration was adjudicated (HARDEN-006).
 */
export interface SpecReviewDecisionSet {
  /** Iteration number (string key of `iterations`) these decisions adjudicate. */
  readonly iteration: string
  readonly decisions: readonly SpecReviewDecision[]
}

/** One recorded spec-review consolidation (HARDEN-001). */
export interface SpecReviewIterationEntry {
  /**
   * Council operation verdict: APPROVE, APPROVE WITH ADVISORIES,
   * REQUEST CHANGES, INCONCLUSIVE, or UNAVAILABLE.
   */
  readonly verdict: string
  /** Correlation ID of the persisted dispatch artifact backing this iteration. */
  readonly correlation_id: string
  /** Number of consolidated findings the iteration recorded. */
  readonly finding_count: number
  /**
   * 1-based finding identities — indices into the artifact's immutable
   * findings array, stable because the artifact never changes after
   * finalization. Per-finding decisions reference these ids.
   */
  readonly finding_ids: readonly string[]
  /** ISO timestamp of the consolidation that recorded the iteration. */
  readonly recorded_at: string
}

/** The spec-review gate record persisted at `state.data["spec-review"]`. */
export interface SpecReviewGateRecord {
  /** Recorded iterations keyed by iteration number string; append-only. */
  readonly iterations: Readonly<Record<string, SpecReviewIterationEntry>>
  /** Adjudication of the iteration named by `decisions.iteration`; absent while undecided. */
  readonly decisions?: SpecReviewDecisionSet
  /** Derived from the decision set's override entries; `[]` when it holds none. */
  readonly overrides?: readonly SpecReviewOverride[]
}

/** Type guard for non-null plain objects (untyped JSON access). */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/**
 * Verdicts that pass the gate without per-finding decisions (HARDEN-002):
 * a plain APPROVE opens advancement directly. APPROVE WITH ADVISORIES is
 * deliberately excluded — SYNTH-004 reverses the old APPROVE-family
 * treatment because the review-dispatch finalizer maps every AWA
 * consolidation to the canonical decision ESCALATED, so those verdicts
 * are relayed (see isEscalatedAdvisorySpecVerdict) instead of advancing.
 */
export function isApprovingSpecVerdict(verdict: string): boolean {
  return verdict === "APPROVE"
}

/**
 * Verdicts the review-dispatch finalizer maps to the canonical decision
 * ESCALATED (SYNTH-004): every APPROVE WITH ADVISORIES consolidation, so
 * "marked ESCALATED" is not a distinguishing subset of advisory results.
 * Relay purposes treat these like REQUEST CHANGES — the widened
 * relay_spec_review_gate predicate pauses on them — while a plain
 * APPROVE (canonical decision APPROVED) still advances without relay.
 */
export function isEscalatedAdvisorySpecVerdict(verdict: string): boolean {
  return verdict === "APPROVE WITH ADVISORIES"
}

/** Parse one iteration entry from untrusted JSON; null when malformed. */
function parseIterationEntry(value: unknown): SpecReviewIterationEntry | null {
  if (!isRecord(value)) return null
  const verdict = value.verdict
  const correlationId = value.correlation_id
  const findingCount = value.finding_count
  const findingIds = value.finding_ids
  const recordedAt = value.recorded_at
  if (typeof verdict !== "string" || verdict === "") return null
  if (typeof correlationId !== "string" || correlationId === "") return null
  if (
    typeof findingCount !== "number" ||
    !Number.isInteger(findingCount) ||
    findingCount < 0
  ) {
    return null
  }
  if (!Array.isArray(findingIds)) return null
  for (const id of findingIds) {
    if (typeof id !== "string" || id === "") return null
  }
  if (typeof recordedAt !== "string" || recordedAt === "") return null
  return {
    verdict,
    correlation_id: correlationId,
    finding_count: findingCount,
    finding_ids: findingIds,
    recorded_at: recordedAt,
  }
}

/**
 * Parse one decision from untrusted JSON; null when malformed. Validation
 * is strict about the conditional fields (apply requires fix_reference,
 * override requires reason): a malformed decision is dropped by the record
 * parser, which leaves its finding undecided — the fail-closed direction.
 */
function parseDecision(value: unknown): SpecReviewDecision | null {
  if (!isRecord(value)) return null
  const findingId = value.finding_id
  const decision = value.decision
  const fixReference = value.fix_reference
  const reason = value.reason
  if (typeof findingId !== "string" || findingId === "") return null
  if (decision !== "apply" && decision !== "override" && decision !== "pause") {
    return null
  }
  if (decision === "apply" && (typeof fixReference !== "string" || fixReference === "")) {
    return null
  }
  if (decision === "override" && (typeof reason !== "string" || reason === "")) {
    return null
  }
  const parsed: SpecReviewDecision = { finding_id: findingId, decision }
  if (typeof fixReference === "string") parsed.fix_reference = fixReference
  if (typeof reason === "string") parsed.reason = reason
  return parsed
}

/**
 * Parse an untrusted value as a gate record; null when the load-bearing
 * shape (iterations, overrides members) is malformed. Dropped decisions
 * stay dropped so their findings remain undecided (fail closed); callers
 * that must not fail open treat a null result as a hard error.
 */
export function parseSpecReviewGateRecord(value: unknown): SpecReviewGateRecord | null {
  if (!isRecord(value)) return null
  const iterations = value.iterations
  if (!isRecord(iterations)) return null
  const parsedIterations: Record<string, SpecReviewIterationEntry> = {}
  for (const [key, entry] of Object.entries(iterations)) {
    if (!/^\d+$/.test(key)) return null
    const parsed = parseIterationEntry(entry)
    if (parsed === null) return null
    parsedIterations[key] = parsed
  }
  const result: SpecReviewGateRecord = { iterations: parsedIterations }
  if (value.decisions !== undefined) {
    if (!isRecord(value.decisions)) return null
    const setIteration = value.decisions.iteration
    if (typeof setIteration !== "string" || !/^\d+$/.test(setIteration)) return null
    if (!Array.isArray(value.decisions.decisions)) return null
    const parsedDecisions: SpecReviewDecision[] = []
    for (const decision of value.decisions.decisions) {
      const parsed = parseDecision(decision)
      if (parsed !== null) parsedDecisions.push(parsed)
    }
    result.decisions = { iteration: setIteration, decisions: parsedDecisions }
  }
  if (value.overrides !== undefined) {
    if (!Array.isArray(value.overrides)) return null
    const parsedOverrides: SpecReviewOverride[] = []
    for (const override of value.overrides) {
      if (
        isRecord(override) &&
        typeof override.finding_id === "string" &&
        typeof override.reason === "string" &&
        override.finding_id !== "" &&
        override.reason !== ""
      ) {
        parsedOverrides.push({
          finding_id: override.finding_id,
          reason: override.reason,
        })
      }
    }
    result.overrides = parsedOverrides
  }
  return result
}

/**
 * Highest recorded iteration and its entry; null when no iteration is
 * recorded. The gate always adjudicates the highest iteration number.
 */
export function latestSpecReviewIteration(
  record: SpecReviewGateRecord,
): { iteration: number; entry: SpecReviewIterationEntry } | null {
  let latest: { iteration: number; entry: SpecReviewIterationEntry } | null = null
  for (const [key, entry] of Object.entries(record.iterations)) {
    const iteration = Number(key)
    if (latest === null || iteration > latest.iteration) {
      latest = { iteration, entry }
    }
  }
  return latest
}

/** Iteration number the next consolidation occupies (max recorded + 1, or 1). */
export function nextSpecReviewIteration(record: SpecReviewGateRecord): number {
  const latest = latestSpecReviewIteration(record)
  return latest === null ? 1 : latest.iteration + 1
}

/** 1-based finding identities for a findings count. */
export function specReviewFindingIds(count: number): string[] {
  return Array.from({ length: count }, (_, index) => String(index + 1))
}

/**
 * Whether a decision carries the fields its choice requires (HARDEN-002):
 * apply requires a non-empty fix reference, override a non-empty reason.
 */
export function isValidSpecReviewDecision(decision: SpecReviewDecision): boolean {
  if (decision.decision === "apply") {
    return typeof decision.fix_reference === "string" && decision.fix_reference !== ""
  }
  if (decision.decision === "override") {
    return typeof decision.reason === "string" && decision.reason !== ""
  }
  return true
}

/**
 * Finding ids of the entry lacking a valid decision — the
 * `missing_per_finding_decisions` refusal payload (HARDEN-002).
 */
export function missingSpecReviewDecisions(
  entry: SpecReviewIterationEntry,
  decisions: readonly SpecReviewDecision[] | undefined,
): readonly string[] {
  const decided = new Set(
    (decisions ?? []).filter(isValidSpecReviewDecision).map((decision) => decision.finding_id),
  )
  return entry.finding_ids.filter((id) => !decided.has(id))
}

/** Whether any recorded decision is `apply`. */
export function hasApplyDecision(
  decisions: readonly SpecReviewDecision[] | undefined,
): boolean {
  return (decisions ?? []).some((decision) => decision.decision === "apply")
}

/**
 * Derive the audit overrides list from override decisions (HARDEN-002):
 * every valid override decision projects to `{finding_id, reason}`.
 */
export function deriveSpecReviewOverrides(
  decisions: readonly SpecReviewDecision[] | undefined,
): SpecReviewOverride[] {
  return (decisions ?? [])
    .filter(
      (decision): decision is SpecReviewDecision & { reason: string } =>
        decision.decision === "override" && isValidSpecReviewDecision(decision),
    )
    .map((decision) => ({ finding_id: decision.finding_id, reason: decision.reason }))
}

/** Decision ids referencing findings the entry does not carry. */
export function unknownSpecReviewDecisionIds(
  entry: SpecReviewIterationEntry,
  decisions: readonly SpecReviewDecision[] | undefined,
): readonly string[] {
  const known = new Set(entry.finding_ids)
  return [...new Set((decisions ?? []).map((decision) => decision.finding_id))].filter(
    (id) => !known.has(id),
  )
}

/** Finding ids decided more than once. */
export function duplicateSpecReviewDecisionIds(
  decisions: readonly SpecReviewDecision[] | undefined,
): readonly string[] {
  const seen = new Set<string>()
  const duplicated = new Set<string>()
  for (const decision of decisions ?? []) {
    if (seen.has(decision.finding_id)) duplicated.add(decision.finding_id)
    seen.add(decision.finding_id)
  }
  return [...duplicated]
}

/**
 * Append one iteration to the record. Append-only: refuses to overwrite an
 * existing iteration number. The decision set and derived overrides are
 * preserved verbatim: the set carries the iteration it adjudicates, so a
 * new iteration leaves it inert for gate purposes (its findings must be
 * decided afresh) while the resume probe can still see that the earlier
 * iteration was adjudicated (HARDEN-006).
 */
export function appendSpecReviewIteration(
  record: SpecReviewGateRecord,
  iteration: number,
  entry: SpecReviewIterationEntry,
): SpecReviewGateRecord {
  const key = String(iteration)
  const existing = record.iterations[key]
  if (existing !== undefined) {
    throw new Error(
      `spec-review iteration ${iteration} already recorded (correlation ${existing.correlation_id})`,
    )
  }
  return { ...record, iterations: { ...record.iterations, [key]: entry } }
}

/**
 * Decisions adjudicating the LATEST recorded iteration, when a recorded set
 * targets it; otherwise empty — a set scoped to an earlier iteration does
 * not decide the latest one's findings (each new REQUEST CHANGES iteration
 * re-opens adjudication).
 */
export function decisionsForLatestIteration(
  record: SpecReviewGateRecord,
): readonly SpecReviewDecision[] {
  const set = record.decisions
  if (set === undefined) return []
  const latest = latestSpecReviewIteration(record)
  if (latest === null) return []
  if (set.iteration !== String(latest.iteration)) return []
  return set.decisions
}

/**
 * Steps `checkpoint` may record while the spec-review gate holds: they
 * precede the gate in pipeline order or belong to its re-entry machinery,
 * so recording them never advances the pipeline past spec-review. Every
 * other step — `spec-review` itself, `implement`, `implement-phase-N`,
 * `quick-pre-flight`, `code-review`, `retrospective`, `demo`, … — is
 * refused until the gate opens (HARDEN-002).
 */
export const SPEC_REVIEW_GATE_STEP_ALLOWLIST: readonly string[] = [
  "branch-safety-gate",
  "tier-routing",
  "resumability-detection",
  "clarify",
  "plan",
  "tasks",
  "quick-fix-budget",
]

/** Outcome of evaluating the spec-review gate for a checkpoint recording. */
export type SpecReviewGateStatus =
  | { kind: "open" }
  | {
      kind: "missing_decisions"
      iteration: number
      verdict: string
      undecided: readonly string[]
    }
  | { kind: "pending_approving_iteration"; iteration: number; verdict: string }
  | {
      kind: "paused"
      iteration: number
      verdict: string
      paused: readonly string[]
    }
  | {
      kind: "advanceable"
      iteration: number
      verdict: string
      overrides: readonly SpecReviewOverride[]
    }

/**
 * Evaluate the spec-review gate (HARDEN-002). The gate holds only while the
 * latest iteration consolidated to REQUEST CHANGES carrying findings; an
 * approving latest, a non-REQUEST-CHANGES verdict, or no record at all
 * leaves the pipeline open. `advanceable` is the all-override outcome: the
 * human adjudicated every finding without any apply decision, so
 * advancement is permitted with the overrides recorded.
 *
 * An escalated-advisory latest (APPROVE WITH ADVISORIES) also evaluates
 * open (SYNTH-004): its hold is the relay predicate plus the
 * advancement-step gate, not this per-finding machinery, so checkpoint's
 * allowlist hold, the resume probe, and the dispatch guard keep their
 * exact pre-SYNTH-004 behavior for advisory results.
 */
export function evaluateSpecReviewGate(
  record: SpecReviewGateRecord | null,
): SpecReviewGateStatus {
  if (record === null) return { kind: "open" }
  const latest = latestSpecReviewIteration(record)
  if (latest === null) return { kind: "open" }
  const { verdict } = latest.entry
  if (isApprovingSpecVerdict(verdict)) return { kind: "open" }
  if (isEscalatedAdvisorySpecVerdict(verdict)) {
    // SYNTH-004: the escalated-advisory hold is relay-purposes-only —
    // relay_spec_review_gate pauses on the verdict and the advancement
    // steps are held separately — never this per-finding gate, whose
    // allowlist hold shape the spec keeps distinct from the advisory
    // hold ("a hold over advancement steps, not an allowlist of
    // permitted ones").
    return { kind: "open" }
  }
  if (verdict !== "REQUEST CHANGES" || latest.entry.finding_count === 0) {
    // Non-REQUEST-CHANGES outcomes (INCONCLUSIVE, UNAVAILABLE) carry no
    // adjudicable findings — the per-finding gate cannot operate on them,
    // so they never block advancement.
    return { kind: "open" }
  }
  const decisions = decisionsForLatestIteration(record)
  const undecided = missingSpecReviewDecisions(latest.entry, decisions)
  if (undecided.length > 0) {
    return {
      kind: "missing_decisions",
      iteration: latest.iteration,
      verdict,
      undecided,
    }
  }
  const paused = decisions
    .filter((decision) => decision.decision === "pause")
    .map((decision) => decision.finding_id)
  if (paused.length > 0) {
    return { kind: "paused", iteration: latest.iteration, verdict, paused }
  }
  if (hasApplyDecision(decisions)) {
    return {
      kind: "pending_approving_iteration",
      iteration: latest.iteration,
      verdict,
    }
  }
  return {
    kind: "advanceable",
    iteration: latest.iteration,
    verdict,
    overrides: deriveSpecReviewOverrides(decisions),
  }
}

/**
 * True when the record's latest iteration is approving and an earlier
 * iteration's decision set contains at least one apply decision — the
 * applied-fixes outcome (HARDEN-006, Decision 6). The spec-artifact dirt
 * those applied fixes caused must not re-fail the resume probe: the
 * adjudication that ordered the fixes plus the recorded subsequent
 * APPROVE already carry the verdict, so only the ancestry rule still
 * applies. Approving here is the plain APPROVE of isApprovingSpecVerdict:
 * an APPROVE WITH ADVISORIES latest is escalated (SYNTH-004) and does not
 * complete the applied-fixes outcome.
 */
export function approvalFollowsApplyDecisions(
  record: SpecReviewGateRecord | null,
): boolean {
  if (record === null) return false
  const latest = latestSpecReviewIteration(record)
  if (latest === null || !isApprovingSpecVerdict(latest.entry.verdict)) return false
  const set = record.decisions
  if (set === undefined) return false
  // The decision set must target an iteration strictly before the approving
  // latest — the applies ordered fixes, and the subsequent dispatch
  // recorded the approving verdict. The integer guard keeps a malformed
  // iteration key from satisfying the comparison.
  const decided = Number(set.iteration)
  if (!Number.isInteger(decided) || decided >= latest.iteration) return false
  return set.decisions.some((decision) => decision.decision === "apply")
}

// ---------------------------------------------------------------------------
// Human-decision record (synthetic-compaction-continue-prompt-rec,
// SYNTH-001/006)
//
// Single normative shape for human-decision authorization: a gate-override
// `checkpoint` call cites a `human_decision_ref` naming a `decision_id`
// recorded in the append-only human-decision record at
// `.uf/progress/<key>/human-decisions.json` (SYNTH-006 store contract).
// Producers and consumers (checkpoint, uf_delegate_command's
// `human_decisions` envelope, record_human_decision) all import from here
// so the shape cannot drift between plugins; the record's file I/O is
// progress-state territory, mirroring the spec-review record layering.
// ---------------------------------------------------------------------------

/**
 * Maximum length of a `human_decision_ref` checkpoint argument (SYNTH-006).
 * The ref names a UUID v4 `decision_id`, so the bound guards against
 * runaway input; length and pattern are both checked before any
 * security-sensitive operation.
 */
export const HUMAN_DECISION_REF_MAX_LENGTH: number = 256

/**
 * Maximum size of a human-decision entry's question or answer text
 * (SYNTH-006): at most 16 KiB each, measured in UTF-8 bytes by the
 * recording surfaces.
 */
export const HUMAN_DECISION_TEXT_MAX_BYTES: number = 16 * 1024

/**
 * UUID v4 pattern a `decision_id` — and the `human_decision_ref` naming it
 * — MUST match (SYNTH-006): canonical lowercase hex, version nibble `4`,
 * RFC 4122 variant (`8`, `9`, `a`, or `b`), the shape machine-generated
 * `crypto.randomUUID()` output produces.
 */
export const HUMAN_DECISION_ID_PATTERN: Readonly<RegExp> =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

/**
 * Reference to a recorded human decision, supplied as the
 * `human_decision_ref` argument of a gate-override `checkpoint` call
 * (SYNTH-001). Reference only — `checkpoint` MUST NOT accept an inline
 * human-decision payload in place of a reference. At run time the value
 * MUST be at most `HUMAN_DECISION_REF_MAX_LENGTH` characters and match
 * `HUMAN_DECISION_ID_PATTERN` (the UUID v4 shape of the `decision_id` it
 * names); violations are rejected with a machine-parseable error before
 * any security-sensitive operation.
 */
export type HumanDecisionRef = string

/**
 * The write surface that recorded an entry — exactly two exist
 * (SYNTH-001): the in-session `question`-tool path
 * (`record_human_decision`) and the orchestrator relay
 * (`uf_delegate_command`'s typed `human_decisions` envelope, persisted at
 * spawn time before the delegated child session runs).
 */
export type HumanDecisionSource = "question-tool" | "orchestrator-relay"

/**
 * One entry of the append-only human-decision record (SYNTH-006). Callers
 * supply only source, question, and answer; `decision_id` (UUID v4) and
 * `recorded_at` are machine-generated by the recording surface. Question
 * and answer content MUST NOT be logged at INFO or above — errors and
 * logs name bounded identifiers only (`decision_id`, step, data key).
 */
export interface HumanDecisionEntry {
  /**
   * Machine-generated UUID v4 identity; the target a `human_decision_ref`
   * names. Callers never supply it.
   */
  readonly decision_id: string
  /** The write surface that recorded the entry. */
  readonly source: HumanDecisionSource
  /** Question text as presented to the human; at most 16 KiB. */
  readonly question: string
  /** Answer text as received from the human; at most 16 KiB. */
  readonly answer: string
  /** Machine-generated ISO timestamp of the recording. */
  readonly recorded_at: string
}

/**
 * Check a question or answer text against the SYNTH-006 byte bound
 * (at most `HUMAN_DECISION_TEXT_MAX_BYTES`, measured in UTF-8 bytes).
 * Both recording surfaces and the record parser enforce this predicate so
 * zod's character-counting `.max()` is never the only line of defense —
 * the bound is a byte bound, and violations are rejected before any
 * security-sensitive operation.
 *
 * @param text Question or answer text to bound-check.
 * @returns True when the text is within 16 KiB UTF-8 bytes.
 */
export function isHumanDecisionTextWithinBounds(text: string): boolean {
  return Buffer.byteLength(text, "utf8") <= HUMAN_DECISION_TEXT_MAX_BYTES
}

/**
 * The synthetic compaction prompt opencode injects after every compaction
 * (SYNTH-006 deny-list needle). This text is machinery output, never human
 * input, so a human-decision answer reproducing it is a session quoting
 * its own context as consent — exactly the incident shape the checkpoint
 * gate closes. Quoted without a trailing period: the match is substring
 * containment, so the injected message's period never breaks it.
 */
export const SYNTHETIC_COMPACTION_PROMPT: string =
  "Continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed"

/**
 * Normalize text for the SYNTH-006 deny-list comparison: trim the ends,
 * collapse interior whitespace runs (spaces, tabs, newlines) to single
 * spaces, and case-fold. Normalization is exactly the three steps the
 * spec names — no punctuation or quote stripping — because the match is
 * substring containment, which already covers the prompt wrapped in any
 * quoting or embedded in a larger justification.
 *
 * @param text Question or answer text to normalize.
 * @returns The trimmed, whitespace-collapsed, case-folded text.
 */
export function normalizeHumanDecisionText(text: string): string {
  return text.trim().replace(/\s+/g, " ").toLowerCase()
}

/** The deny-list needle under its own normalization (computed once). */
const NORMALIZED_SYNTHETIC_COMPACTION_PROMPT: string = normalizeHumanDecisionText(
  SYNTHETIC_COMPACTION_PROMPT,
)

/**
 * Whether an answer text, after SYNTH-006 normalization, contains the
 * normalized synthetic compaction prompt as a substring — the deny-list
 * match `record_human_decision` refuses. Containment covers the prompt
 * verbatim, in any case, with any whitespace variation, wrapped in any
 * quoting, or embedded in a larger justification; a partial quote that
 * does not contain the full prompt is outside this predicate (SYNTH-001's
 * reference requirement governs those at checkpoint time, not the
 * deny-list).
 *
 * @param answer Answer text as received from the human.
 * @returns True when the normalized answer contains the normalized prompt.
 */
export function isSyntheticCompactionPromptAnswer(answer: string): boolean {
  return normalizeHumanDecisionText(answer).includes(NORMALIZED_SYNTHETIC_COMPACTION_PROMPT)
}

/**
 * Caller-supplied half of a human-decision append (SYNTH-006): the two
 * recording surfaces pass exactly source, question, and answer —
 * `decision_id` and `recorded_at` are machine-stamped by the store, never
 * accepted from a caller. The orchestrator-relay envelope constrains
 * `source` to `"orchestrator-relay"` structurally (the surface is the
 * source), so a caller can never claim question-tool provenance through
 * the relay.
 */
export interface HumanDecisionAppendInput {
  /** The write surface recording the entry. */
  readonly source: HumanDecisionSource
  /** Question text as presented to the human; at most 16 KiB, non-empty. */
  readonly question: string
  /** Answer text as received from the human; at most 16 KiB, non-empty. */
  readonly answer: string
}

/**
 * Parse an untrusted value as the persisted human-decision record (SYNTH-006):
 * a JSON array of `HumanDecisionEntry`. Returns null when the value is not
 * an array or any entry violates the record contract (UUID v4
 * `decision_id`, a known `source`, non-empty question and answer within the
 * 16 KiB byte bound, string `recorded_at`) — a corrupted record must fail
 * closed at the caller, never silently shrink to the entries that happen to
 * parse. Mirrors `parseSpecReviewGateRecord`'s pure-strictness layering.
 *
 * @param value Untrusted JSON value read from the record file.
 * @returns The validated entries, or null when the record is malformed.
 */
export function parseHumanDecisionRecord(value: unknown): readonly HumanDecisionEntry[] | null {
  if (!Array.isArray(value)) return null
  const entries: HumanDecisionEntry[] = []
  for (const raw of value) {
    if (typeof raw !== "object" || raw === null) return null
    const entry = raw as Record<string, unknown>
    if (typeof entry.decision_id !== "string" || !HUMAN_DECISION_ID_PATTERN.test(entry.decision_id)) {
      return null
    }
    if (entry.source !== "question-tool" && entry.source !== "orchestrator-relay") return null
    if (typeof entry.question !== "string" || entry.question === "" || !isHumanDecisionTextWithinBounds(entry.question)) {
      return null
    }
    if (typeof entry.answer !== "string" || entry.answer === "" || !isHumanDecisionTextWithinBounds(entry.answer)) {
      return null
    }
    if (typeof entry.recorded_at !== "string" || entry.recorded_at === "") return null
    entries.push({
      decision_id: entry.decision_id,
      source: entry.source,
      question: entry.question,
      answer: entry.answer,
      recorded_at: entry.recorded_at,
    })
  }
  return entries
}
