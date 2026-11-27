import { type PluginInput } from "@opencode-ai/plugin"
import { z } from "zod"

import {
  ACTIVE_WINDOW_MS,
  DEFAULT_IDLE_POLL_MS,
  clampToBudget,
  classifySyntheticText,
  idleDeferralExpired,
  probeSubtreeActivity,
  registerDeadline,
  registeredProbeCounters,
  resolveIdleApplicability,
  resolveInheritedBudget,
  resolveInheritedDeadline,
  selectRunningTool,
  type RunningToolPart,
  type SyntheticTextKind,
  unregisterDeadline,
  unregisterProbeState,
} from "./delegation-budget.js"
import { normalizeStormThreshold } from "./model-matrix.js"
import { type DerivedPerTaskTimeout, type PerTaskTimeoutProvenanceRecord } from "./run-timeout.js"

// ── Constants ────────────────────────────────────────────────

export const MAX_ERROR_LENGTH = 4_096
/**
 * Maximum length of final-message text preserved in a final-text guard
 * error (compaction-storm design D3, DELEG-FR-003). The excerpt exists
 * only to debug the matched wrapper structure: it is sliced from the
 * final message text alone -- never the cached prefix or earlier context
 * that preceded the final message -- and the guard failures route the
 * whole error through sanitizeInvocationError, so neither secrets nor
 * prefix content ride the preserved text. The bound keeps a
 * preamble-plus-excerpt error message inside MAX_ERROR_LENGTH.
 */
export const MAX_PRESERVED_FINAL_TEXT_LENGTH = 2_048
export const CLEANUP_ABORT_TIMEOUT_MS = 5_000
export const DEFAULT_TIMEOUT_MILLISECONDS = 600_000
/**
 * Wall-clock timeout ceiling shared by invoke_agent and every
 * prompt-only delegation path (uf_delegate_command derives its
 * MAX_PROMPT_DELEGATION_TIMEOUT_MS from this value). Single
 * source of truth so the executor schema and the delegation
 * bound cannot drift apart.
 */
export const MAX_TIMEOUT_MILLISECONDS = 1_800_000
export const MAX_INLINE_PROMPT_BYTES = 128 * 1024
export const MAX_PROMPT_FILE_BYTES = 4 * 1024 * 1024

// ── Shared Zod schemas ──────────────────────────────────────

export const PromptSchema = z
  .string()
  .refine((value) => Buffer.byteLength(value, "utf8") <= MAX_INLINE_PROMPT_BYTES, "prompt exceeds 128 KiB UTF-8")
export const PromptFileSchema = z.string().max(1024)
export const TimeoutSchema = z.number().int().positive().max(MAX_TIMEOUT_MILLISECONDS)
export const VariantSchema = z.string().max(64).regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/)

// ── Type derivations from the OpenCode plugin client ─────────

type PromptOptions = Parameters<PluginInput["client"]["session"]["prompt"]>[0]
type PromptResponse = Awaited<ReturnType<PluginInput["client"]["session"]["prompt"]>>
type PromptPart = NonNullable<PromptResponse["data"]>["parts"][number]

type PromptBodyWithVariant = NonNullable<PromptOptions["body"]> & {
  variant?: string
}

type CurrentMessageResponse = Awaited<ReturnType<PluginInput["client"]["session"]["message"]>>
type CurrentMessageInfo = NonNullable<CurrentMessageResponse["data"]>["info"]
type CurrentAssistant = Extract<CurrentMessageInfo, { role: "assistant" }>
type CurrentAssistantWithVariant<T> = T & {
  variant?: string
}

// ── Exported types ───────────────────────────────────────────

export interface ModelIdentity {
  readonly providerID: string
  readonly modelID: string
  readonly combined: string
}

/** Token counts reported by OpenCode step-finish response parts. */
export interface InvocationTokens {
  readonly input: number
  readonly output: number
}

/** Nullable invocation usage assembled only from reported step-finish parts. */
export interface InvocationUsage {
  readonly cost_usd: number | null
  readonly tokens: InvocationTokens | null
}

/** Sanitized failure details for one invoke_agent run. */
export interface InvocationError {
  readonly code: string
  readonly message: string
  readonly retryable: boolean
}

/** Request, host-resolution, and child-report provenance for one invocation. */
export interface InvocationProvenance {
  readonly agent: string
  readonly requested_model: string | null
  readonly requested_variant: string | null
  readonly resolved_parent_model: string | null
  readonly resolved_parent_variant: string | null
  readonly reported_child_model: string | null
  readonly model_mismatch: boolean
  readonly read_only: boolean
}

/**
 * Which limit ended a run (delegation-timeout-budgets D4,
 * compaction-storm D2): the wall clock, the idle (no-progress) window, an
 * exhausted inherited budget that prevented the run from starting, or the
 * compaction-storm threshold (one session's run of consecutive compactions
 * with no substantive progress, DELEG-FR-002).
 */
export type TimeoutKind = "wall_clock" | "idle" | "budget" | "storm"

/**
 * Advisory resume hint for a timed-out per-task delegation
 * (per-task-delegation-budgets design D3, PTB-FR-003): the files the
 * child touched and the tasks it marked complete since the delegation
 * started, assembled by diffing already-observable state — the
 * worktree diff or session file tracking and the tasks.md checkbox
 * state — never by semantic analysis, and never carrying message
 * content (Constitution V). The hint is advisory input to the
 * executor-owned bounded retry (DE-005): the retry prompt relays it
 * verbatim; no semantic merge is attempted.
 */
export interface PerTaskResumeHint {
  /** Repository-relative paths modified since the delegation started. */
  readonly files_modified: readonly string[]
  /** Task identifiers marked complete since the delegation started. */
  readonly tasks_completed: readonly string[]
}

/**
 * Executor-owned bounded-retry provenance
 * (per-task-delegation-budgets PTB-FR-003, DE-005): present when the
 * per-task executor re-issued a retryable timeout termination instead
 * of returning it to the caller. The record proves the single bound
 * retry happened and whether its prompt carried concrete relayed
 * resume state, so a retried failure returned to the caller is final
 * for that termination kind. Absent when no retry ran and for every
 * non-per-task caller.
 */
export interface PerTaskRetryRecord {
  /** Timeout kind that ended the first attempt. */
  readonly first_attempt_kind: TimeoutKind
  /**
   * True when the retry prompt carried concrete relayed state — the
   * first attempt's resume hint, or a readable fresh capture's
   * completed-task anchor when the hint was unreadable. False when it
   * degraded to directing the child to the tasks.md checkboxes itself.
   */
  readonly resume_relayed: boolean
}

/** Limit details attached to runs that ended by a limit (timeout, storm, or exhausted budget; design D4). */
export interface TimeoutDetails {
  readonly kind: TimeoutKind
  /** Wall clock actually applied, after any budget clamp. */
  readonly effective_ms: number
  /** Wall clock the caller requested. */
  readonly requested_ms: number
  /** True when the inherited parent budget reduced the wall clock. */
  readonly budget_clamped: boolean
  /** Idle window, or null when the idle watchdog was disabled. */
  readonly idle_timeout_ms: number | null
  /** Milliseconds from run start to the abort. */
  readonly elapsed_ms: number
  /** Milliseconds since the last observed activity change, or null when never probed. */
  readonly last_progress_ms_ago: number | null
  /** Progress observed within ACTIVE_WINDOW_MS of the abort; null when never probed. */
  readonly child_active: boolean | null
  /** A question tool call was pending in the subtree at the last probe. */
  readonly pending_question: boolean
  /**
  /**
   * Tool name of the running part with the lexicographically smallest
   * partId at abort time (DT-FR-015), validated against
   * RUNNING_TOOL_NAME_PATTERN; null when no part was running or the
   * selected name failed validation (invalid names are never carried
   * verbatim, Constitution V).
   */
  readonly running_tool: string | null
  /**
   * The storming session for `kind: "storm"` details — the counted session
   * holding the highest per-session run of consecutive compactions
   * (compaction-storm design D3, DELEG-FR-002) — or null for every
   * non-storm kind. Both storm paths record it at abort: the normalized
   * threshold is at least 2 and the fold pairs every positive maximum with
   * its holding session, so the id is present whenever a storm fires.
   */
  readonly storm_session_id: string | null
  /**
   * Advisory per-task resume hint (per-task-delegation-budgets D3,
   * PTB-FR-003): the files the child modified and the tasks it
   * completed since the delegation started. Present only on per-task
   * delegation results that ended by the wall clock, the idle
   * watchdog, or an exhausted budget; null when the underlying state
   * was unreadable (fail-closed, per D4 of #950). Absent for every
   * non-per-task caller.
   */
  readonly resume?: PerTaskResumeHint | null
}

/**
 * Durable resume state carried by `context_thrash` failures (compaction-storm
 * design D2, DELEG-FR-002). Reuses the DelegateResumeState envelope pattern
 * (DT-FR-004) at the executor seam: what the executor durably knows about
 * the aborted run — where the child ran, which agent it was, and the
 * checkpoint step the caller asked it to record (still pending, because the
 * run aborted before recording it). The command-keyed resume state
 * (`command`, `key`, `completed_steps`) is threaded separately by
 * `uf_delegate_command` for command-path delegations.
 */
export interface InvocationResumeState {
  /** Absolute project directory the child session ran in. */
  readonly project_path: string
  /** Manifest-declared agent name the child ran as. */
  readonly agent_name: string
  /** Checkpoint step the run was asked to record; null when none applied. */
  readonly pending_checkpoint_step: string | null
}

/** Observable result from exactly one policy-free agent invocation. */
export interface InvokeAgentResult {
  readonly status: "success" | "failed" | "cancelled"
  readonly text: string
  readonly usage: InvocationUsage | null
  readonly error: InvocationError | null
  readonly provenance: InvocationProvenance
  /** Child session ID created by the executor (absent on early validation failures). */
  readonly childSessionID?: string
  /**
   * Limit details, present only when a timeout, storm, or exhausted budget
   * ended or reclassified the run (the final-text guard's context_thrash
   * reclassification reuses the storm shape, DELEG-FR-003).
   */
  readonly timeout?: TimeoutDetails
  /**
   * Durable resume state, present only on context_thrash failures -- the
   * storm abort (DELEG-FR-002) and the final-text guard reclassification
   * at or above the storm threshold (DELEG-FR-003).
   */
  readonly resume?: InvocationResumeState
  /**
   * Per-task timeout derivation provenance
   * (per-task-delegation-budgets D1, PTB-FR-001): every derivation
   * input, the unclamped derived value, the ceiling, and the clamp
   * outcome. Present on every result the per-task delegation executor
   * returns — success or failure — so the effective wall clock stays
   * observable per RD-FR-009 / RD-FR-030. Absent for every
   * non-per-task caller.
   */
  readonly timeout_provenance?: PerTaskTimeoutProvenanceRecord
  /**
   * Remaining budgeted-ancestor budget in milliseconds
   * (per-task-delegation-budgets PTB-FR-004): the ancestor deadline
   * minus the clock at result-assembly time, floored at zero. Present
   * only when a budgeted ancestor framed the run — success, failure,
   * or budget_exhausted — so the caller can thread the remaining
   * budget into its phase checkpoint. Absent for unbudgeted runs.
   */
  readonly remaining_ms?: number
  /**
   * Executor-owned bounded-retry provenance (PTB-FR-003, DE-005):
   * present when the per-task executor re-issued a retryable timeout
   * termination once with the resume context tool-rendered into the
   * retry prompt — on success (the retry finished the task) and on the
   * returned failure alike. A failure carrying this record already
   * consumed the bound, so it is final for that termination kind;
   * non-retryable terminations (e.g. budget_exhausted) never carry it.
   */
  readonly retry?: PerTaskRetryRecord
  /**
   * Finish reason reported by the child's final step-finish part, null
   * when unreported. Present on success results so dispatch can apply
   * length policy to non-empty (truncated) runs (RD-FR-060).
   */
  readonly finish_reason?: string | null
  /**
   * Name of the child's message error (for example
   * `MessageAbortedError`), present only on `child_message_error`
   * failures (delegate-abort-recovery D2, DAR-FR-002). The executor's
   * own `retryable` value is unchanged; callers that can read durable
   * state decide whether the abort is resumable.
   */
  readonly message_error_name?: string
}

/** Injected dependencies for the shared executor's child session lifecycle. */
export interface ExecutorDependencies {
  readonly client: PluginInput["client"]
  readonly sessionID: string
  readonly directory: string
  readonly parentAbort: AbortSignal
  readonly timeoutMilliseconds: number
  /**
   * Idle (no-progress) window; when set, the child's session subtree is
   * probed for activity and the run aborts with idle_timeout once no
   * change has been observed for this long (design D2).
   */
  readonly idleTimeoutMilliseconds?: number
  /** Activity probe interval (default DEFAULT_IDLE_POLL_MS). */
  readonly idlePollMilliseconds?: number
  /**
   * Compaction-storm threshold (DELEG-FR-002, design D2): consecutive
   * compaction events with no intervening substantive progress that abort
   * the child with `context_thrash`. The detector is always armed — an
   * absent value runs the documented default, and an invalid injected
   * value normalizes to it, so a bad value can neither disable the
   * detector nor abort instantly.
   */
  readonly stormThreshold?: number
  /**
   * Checkpoint step the caller asked this run to record (design D2),
   * threaded into the context_thrash resume envelope; absent when the
   * delegation records no checkpoint.
   */
  readonly checkpointStep?: string
  /**
   * Per-task wall-clock deferral (per-task-delegation-budgets D2,
   * PTB-FR-002): when armed, an expired wall clock holds while the
   * child is progressing (activity within ACTIVE_WINDOW_MS,
   * including running tool parts inside their DT-FR-014 deferral
   * bound) and re-arms each activity window, aborting once activity
   * stops; the idle watchdog stays the primary stall signal and the
   * inherited budget the absolute backstop. Armed only for per-task
   * children of a budgeted ancestor carrying an idle window; every
   * other caller keeps the absolute wall clock from #930/#950.
   */
  readonly deferWallClockWhileActive?: boolean
  /** Clock seam for budget and idle arithmetic (default Date.now). */
  readonly now?: () => number
}

/** Context required for host model resolution via parent session introspection. */
export interface HostResolutionContext {
  readonly client: PluginInput["client"]
  readonly sessionID: string
  readonly messageID: string
  readonly directory: string
}

// ── Internal helper types ────────────────────────────────────

interface ResponseErrorLike {
  readonly error?: unknown
  readonly response?: { readonly status: number }
}

// ── Utility functions ────────────────────────────────────────

export function errorText(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  if (typeof error === "string") {
    return error
  }
  try {
    return JSON.stringify(error) ?? "unknown invocation failure"
  } catch {
    return "unknown invocation failure"
  }
}

/**
 * Redacts credentials, bearer values, URL queries, and absolute home paths from an error.
 * @param message Untrusted provider, runtime, or validation error text.
 * @returns A bounded message safe to retain in invocation provenance.
 */
export function sanitizeInvocationError(message: string): string {
  return redactInvocationText(message).slice(0, MAX_ERROR_LENGTH)
}

/**
 * Redacts credentials, bearer values, URL queries, and absolute home
 * paths without bounding the length (delegate-abort-recovery D4): the
 * shared redaction behind sanitizeInvocationError, for callers that apply
 * their own bound.
 * @param message Untrusted provider, runtime, or child-session text.
 * @returns The redacted text, unbounded.
 */
export function redactInvocationText(message: string): string {
  return message
    .replace(/\bBearer\s+[^\s,;"']+/gi, "Bearer [REDACTED]")
    .replace(
      /\b(api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|token|secret|password|credential)s?\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;"']+)/gi,
      "$1$2[REDACTED]",
    )
    .replace(/(https?:\/\/[^\s?#]+)\?[^\s#]*/gi, "$1?[REDACTED]")
    .replace(/(?:\/Users|\/home)\/[^/\s]+(?:\/[^\s,;:"']*)?/g, "~/[REDACTED]")
    .replace(/[A-Za-z]:\\+Users\\+[^\\\s]+(?:\\+[^\s,;:"']*)?/g, "~\\[REDACTED]")
}

export function responseFailure(label: string, response: ResponseErrorLike): string {
  const error = response.error
  const status = response.response?.status
  // Treat empty objects as absent so the HTTP status (if available) takes precedence.
  const hasError =
    error !== undefined &&
    error !== null &&
    (typeof error !== "object" || Object.keys(error as Record<string, unknown>).length > 0)
  const detail = hasError ? error : (status !== undefined ? `HTTP ${status}` : "missing response data")
  return sanitizeInvocationError(`${label}: ${errorText(detail)}`)
}

export function directModelIdentity(model: string): ModelIdentity {
  const separator = model.indexOf("/")
  return {
    providerID: model.slice(0, separator),
    modelID: model.slice(separator + 1),
    combined: model,
  }
}

export function reportedModel(providerID: string, modelID: string): string | null {
  return providerID.length > 0 && modelID.length > 0 ? `${providerID}/${modelID}` : null
}

export function failedResult(
  provenance: InvocationProvenance,
  code: string,
  message: string,
  retryable: boolean,
  status: "failed" | "cancelled" = "failed",
  text = "",
  usage: InvocationUsage | null = null,
): InvokeAgentResult {
  return {
    status,
    text,
    usage,
    error: { code, message: sanitizeInvocationError(message), retryable },
    provenance,
  }
}

export function invocationUsage(parts: readonly PromptPart[]): InvocationUsage | null {
  const finishes = parts.filter(
    (part): part is Extract<PromptPart, { type: "step-finish" }> => part.type === "step-finish",
  )
  if (finishes.length === 0) {
    return null
  }
  return finishes.reduce<InvocationUsage>(
    (usage, finish) => ({
      cost_usd: (usage.cost_usd ?? 0) + finish.cost,
      tokens: {
        input: (usage.tokens?.input ?? 0) + finish.tokens.input,
        output: (usage.tokens?.output ?? 0) + finish.tokens.output,
      },
    }),
    { cost_usd: 0, tokens: { input: 0, output: 0 } },
  )
}

export function responseText(parts: readonly PromptPart[]): string {
  return parts
    .filter(
      (part): part is Extract<PromptPart, { type: "text" }> => part.type === "text" && part.ignored !== true,
    )
    .map((part) => part.text)
    .join("\n\n")
    .trim()
}

/** Debugging name of the matched synthetic wrapper shape (design D3). */
function syntheticTextName(kind: SyntheticTextKind): string {
  return kind === "compaction-summary" ? "compaction-summary wrapper" : "synthetic continuation prompt"
}

/**
 * Session attribution for `context_thrash` messages (compaction-storm design
 * D3, DELEG-FR-002): names the session holding the highest per-session run
 * of consecutive compactions. The fold pairs every positive maximum with
 * its holding session and the normalized threshold is at least 2, so a
 * threshold-meeting accounting always names a session; the null arm is a
 * defensive fallback for the nullable sample field, not a reachable state.
 */
function stormSessionClause(sessionID: string | null): string {
  return sessionID === null ? "in an unidentified session" : `in session ${sessionID}`
}

/**
 * Finish reason from the child's final step-finish part, null when no
 * step-finish part reported one. The last part wins because a
 * multi-step run reports one finish per step and only the final part
 * states how the stream terminated (e.g. "length"). The SDK declares
 * `reason` required, but provider and SDK variance can omit it at
 * runtime; a missing, empty, or non-string reason is unreported
 * (null), never a crash.
 *
 * @param parts Response parts from the child's final prompt result.
 * @returns The reported finish reason, or null when unreported.
 */
export function responseFinishReason(parts: readonly PromptPart[]): string | null {
  const finishes = parts.filter(
    (part): part is Extract<PromptPart, { type: "step-finish" }> => part.type === "step-finish",
  )
  if (finishes.length === 0) {
    return null
  }
  const reason: unknown = finishes[finishes.length - 1].reason
  return typeof reason === "string" && reason.length > 0 ? reason : null
}

export function validateTimeout(timeoutMilliseconds: number): void {
  if (!Number.isSafeInteger(timeoutMilliseconds) || timeoutMilliseconds <= 0 || timeoutMilliseconds > MAX_TIMEOUT_MILLISECONDS) {
    throw new Error("invoke_agent timeout must be a positive integer no greater than 1800000 milliseconds")
  }
}

/**
 * Raised when host model resolution fails for a structural reason that
 * retrying cannot fix (wrong message role, missing identity, invalid
 * variant). Carries a remediation string naming how to fix the cause.
 * Transient failures (API errors reading the parent message) remain
 * plain `Error`s so callers can keep them retryable.
 */
export class StructuralHostModelError extends Error {
  readonly remediation: string

  constructor(message: string, remediation: string) {
    super(message)
    this.name = "StructuralHostModelError"
    this.remediation = remediation
  }
}

/**
 * Introspects the parent session's current assistant message to resolve
 * the active host model identity and variant.
 *
 * The variant is optional: when the provider does not populate a variant
 * field on the assistant message, resolution succeeds with
 * `variant: undefined` and the child session is spawned without a variant
 * override. A present-but-invalid variant still fails.
 *
 * @param context Client and session context for the introspection call.
 * @param signal Abort signal for cancellation.
 * @returns Resolved model identity and variant (or undefined when the
 *   host assistant message carries no variant) from the host session.
 * @throws {StructuralHostModelError} When the parent message is not an
 *   assistant message, the model identity cannot be extracted, or the
 *   variant is present but invalid — structural causes retrying cannot fix.
 * @throws When the parent message is unavailable (transient API failure).
 */
export async function resolveHostModel(
  context: HostResolutionContext,
  signal: AbortSignal,
): Promise<{ readonly identity: ModelIdentity; readonly variant: string | undefined }> {
  const response = await context.client.session.message({
    path: { id: context.sessionID, messageID: context.messageID },
    query: { directory: context.directory },
    signal,
  })
  if (response.data === undefined) {
    throw new Error(responseFailure("current assistant message is unavailable", response))
  }
  if (response.data.info.role !== "assistant") {
    throw new StructuralHostModelError(
      "current message is not an assistant message",
      "re-invoke the command so host introspection reads the current assistant turn",
    )
  }

  const assistant = response.data.info as CurrentAssistantWithVariant<CurrentAssistant>
  const combined = reportedModel(assistant.providerID, assistant.modelID)
  if (combined === null || combined.length > 256) {
    throw new StructuralHostModelError(
      "current assistant model identity is unavailable",
      "run the command from a provider that reports the active model identity",
    )
  }
  const parsed = assistant.variant === undefined ? undefined : VariantSchema.safeParse(assistant.variant)
  if (parsed !== undefined && !parsed.success) {
    throw new StructuralHostModelError(
      "current assistant active variant is invalid",
      "run the command from a provider whose active variant is a valid slug",
    )
  }
  return {
    identity: { providerID: assistant.providerID, modelID: assistant.modelID, combined },
    variant: parsed?.data,
  }
}

// ── Shared executor ──────────────────────────────────────────

/**
 * Executes one child agent session with full lifecycle management.
 *
 * Owns: abort controller setup, child session create/prompt,
 * response parsing (text, usage, finish reason, mismatch), the
 * empty-final-response guard (RD-FR-059: trimmed-empty final text
 * fails as retryable child_empty_response), timeout, cancellation,
 * and cleanup, plus the delegation-timeout-budgets limits: the wall
 * clock is clamped to any inherited parent budget (D1), the child's
 * deadline — carrying the run's idle window for nested per-task
 * inheritance (per-task-delegation-budgets D2) — is registered for
 * nested runs, the optional idle watchdog
 * aborts a child whose session subtree stops changing (D2) -- deferred
 * while any tool part is running, within the DT-FR-014 bound (the
 * earliest observable tool deadline plus grace, else the fixed
 * IDLE_DEFERRAL_CAP_MS from the last fingerprint change), with the
 * wall clock as the absolute backstop -- except in per-task
 * deferral mode (deferWallClockWhileActive, per-task-delegation-
 * budgets D2/PTB-FR-002), where an expired wall clock holds while
 * the child is progressing and re-arms each activity window --
 * and limit-ended results
 * carry `timeout` details (D4) including the validated running_tool
 * (DT-FR-015). Also owns the
 * compaction-storm detector (compaction-storm design D2, DELEG-FR-002):
 * the always-armed activity probe aborts a child once one counted session
 * in its subtree accrues the configured number of consecutive compactions
 * without substantive progress (sibling sessions never sum, and
 * executor-managed descendants carry their own detectors), returning a
 * retryable `context_thrash` failure with a resume envelope that names
 * the storming session. And owns the final-text success guard
 * (compaction-storm design D3, DELEG-FR-003): a child whose final
 * message text structurally matches a compaction-summary wrapper or
 * the synthetic continuation prompt is reclassified from success to a
 * retryable `context_thrash` (storm threshold met, naming the storming
 * session) or `synthetic_final_text` failure that preserves sanitized
 * final text.
 * Does NOT own: input validation, manifest checking,
 * model resolution, host introspection, or prompt file reading.
 *
 * @param deps Injected client, session context, and timeout boundary.
 * @param agent Manifest-declared divisor-* review agent name.
 * @param promptText Resolved prompt text to deliver to the child session.
 * @param model Resolved model identity for the child session.
 * @param variant Optional runtime variant for the child session.
 * @param read_only Invocation provenance flag (default true).
 * @param provenance Initial provenance populated by the caller.
 * @returns Observable result with status, text, usage, error, provenance,
 *   and the finish reason on success results.
 */
export async function executeAgentSession(
  deps: ExecutorDependencies,
  agent: string,
  promptText: string,
  model: ModelIdentity,
  variant: string | undefined,
  read_only: boolean,
  provenance: InvocationProvenance,
): Promise<InvokeAgentResult> {
  const now = deps.now ?? Date.now
  const startedAt = now()
  const idleMilliseconds = deps.idleTimeoutMilliseconds
  // Per-task wall-clock deferral (per-task-delegation-budgets D2,
  // PTB-FR-002): armed only by the per-task delegation executor for a
  // child of a budgeted ancestor carrying an idle window; every other
  // caller keeps the absolute wall clock from #930/#950.
  const deferWallClock = deps.deferWallClockWhileActive === true
  // Compaction-storm threshold (DELEG-FR-002, design D2): the probe's
  // per-session compaction accounting is shared with the idle watchdog
  // (DT-FR-002); N consecutive compactions without intervening substantive
  // progress in one counted session abort the child with a retryable
  // context_thrash failure naming that session. Normalized here (not
  // trusted from the caller) so a badly injected value can neither
  // disable the detector nor fire it instantly.
  const stormThreshold = normalizeStormThreshold(deps.stormThreshold)
  // Recorded the moment the interval detector aborts (design D3): the
  // per-session maximum and the session holding it, so the failure
  // envelope names the storming session.
  let stormCompactions = 0
  let stormSessionID: string | null = null

  // Hierarchical budget (delegation-timeout-budgets D1): a nested run
  // never outlives the deadline its parent child session registered.
  const inheritedDeadline = await resolveInheritedDeadline(
    deps.client,
    deps.sessionID,
    deps.directory,
    deps.parentAbort,
  )
  const budget = clampToBudget(deps.timeoutMilliseconds, inheritedDeadline, startedAt)
  const effectiveMilliseconds = budget.effectiveMs

  // Two progress anchors (DT-FR-013/014): lastFingerprintChangeAt feeds
  // the idle and deferral math (only a fingerprint change moves it),
  // while lastProgressAt additionally counts a running tool part as
  // activity within the deferral bound so the envelope's
  // last_progress_ms_ago and child_active reflect a quiet-but-healthy
  // long tool call.
  let lastProgressAt = startedAt
  let lastFingerprintChangeAt = startedAt
  let lastFingerprint: string | null = null
  let probed = false
  let pendingQuestion = false
  let runningTools: readonly RunningToolPart[] = []
  const timeoutDetails = (kind: TimeoutKind, stormSession: string | null = null): TimeoutDetails => {
    const at = now()
    return {
      kind,
      effective_ms: effectiveMilliseconds,
      requested_ms: deps.timeoutMilliseconds,
      budget_clamped: budget.clamped,
      idle_timeout_ms: idleMilliseconds ?? null,
      elapsed_ms: at - startedAt,
      last_progress_ms_ago: probed ? at - lastProgressAt : null,
      child_active: !probed ? null : kind !== "idle" && at - lastProgressAt <= ACTIVE_WINDOW_MS,
      pending_question: pendingQuestion,
      running_tool: selectRunningTool(runningTools),
      // The storming session rides the envelope only on the storm kind;
      // every other kind carries null even if a call site passed a value
      // (design D3: non-null on both storm paths, null otherwise).
      storm_session_id: kind === "storm" ? stormSession : null,
    }
  }

  if (budget.exhausted) {
    return {
      ...failedResult(
        provenance,
        "budget_exhausted",
        `parent delegation budget exhausted (${budget.effectiveMs} ms left after the reserve); ` +
          "return to the caller so it can resume from durable state",
        false,
      ),
      timeout: timeoutDetails("budget"),
    }
  }

  const linkedAbort = new AbortController()
  let abortCause: "cancelled" | "timeout" | "idle" | "storm" | null = null
  let childID: string | null = null
  let cleanupAbort = Promise.resolve()

  const abortChild = (): void => {
    if (childID === null) {
      return
    }
    const id = childID
    cleanupAbort = cleanupAbort.then(async () => {
      const cleanupController = new AbortController()
      const cleanupTimeout = setTimeout(() => cleanupController.abort(), CLEANUP_ABORT_TIMEOUT_MS)
      try {
        await deps.client.session.abort({
          path: { id },
          query: { directory: deps.directory },
          signal: cleanupController.signal,
        })
      } catch {
        // Cleanup is best-effort because the primary cancellation result remains authoritative.
      } finally {
        clearTimeout(cleanupTimeout)
      }
    })
  }
  const cancelFromParent = (): void => {
    abortCause = "cancelled"
    linkedAbort.abort(deps.parentAbort.reason)
    abortChild()
  }
  // Wall clock with per-task deferral (per-task-delegation-budgets
  // D2, PTB-FR-002): in deferral mode an expired clock holds while
  // the child is progressing — activity within ACTIVE_WINDOW_MS,
  // which the probe maintains for running tool parts inside their
  // DT-FR-014 deferral bound — and re-arms for one activity window,
  // so the abort lands within one window of activity stopping and a
  // run whose probes keep failing (stale activity) cannot defer
  // forever. Out of deferral mode the clock stays absolute exactly
  // as in #930/#950.
  const onWallClockExpired = (): void => {
    if (deferWallClock && probed && now() - lastProgressAt <= ACTIVE_WINDOW_MS) {
      wallClockTimer = setTimeout(onWallClockExpired, ACTIVE_WINDOW_MS)
      return
    }
    // A late wall-clock timer must not overwrite an abort cause
    // already recorded by the idle watchdog or a parent
    // cancellation: the catch block classifies the result from
    // `abortCause`, and the first cause wins. `linkedAbort.abort`
    // is idempotent, so relaying the timeout stays safe either way.
    if (abortCause === null) {
      abortCause = "timeout"
    }
    linkedAbort.abort(new Error("invoke_agent run timed out"))
    abortChild()
  }
  let wallClockTimer: ReturnType<typeof setTimeout> | undefined = setTimeout(onWallClockExpired, effectiveMilliseconds)

  // Activity probe (design D2): a changed subtree fingerprint is progress
  // for the idle watchdog, and the same probe folds the compaction
  // accounting the storm detector consumes (DELEG-FR-002). A failed probe
  // neither resets nor advances the idle clock, so only a successful,
  // unchanged sample can end the run. The storm detector is always armed,
  // so the probe runs even when the idle watchdog is disabled.
  // `settled` flips as soon as the child's prompt returns (and again in
  // finally), so a probe still in flight when the run ends can never
  // abort a finished child or relabel its result.
  let sampling = false
  let settled = false
  const sample = async (): Promise<void> => {
    if (settled || sampling || childID === null || abortCause !== null) {
      return
    }
    sampling = true
    try {
      const activity = await probeSubtreeActivity(deps.client, childID, deps.directory, linkedAbort.signal)
      if (activity === null || settled || abortCause !== null) {
        return
      }
      probed = true
      pendingQuestion = activity.pendingQuestion
      // Running parts ride the executor state before any early return
      // so every abort path reports the tools observed at the last
      // probe (DT-FR-015: running_tool for every kind).
      runningTools = activity.runningTools
      // Compaction-storm detector (DELEG-FR-002, design D2): one counted
      // session with N consecutive compaction markers and no intervening
      // substantive progress — reads and failed non-read calls never reset
      // the accounting — aborts the child well before wall-clock
      // exhaustion. The signal is the highest per-session run, never a
      // subtree sum, so parallel read-heavy siblings each below the
      // threshold cannot abort the delegation (issue #1125). Checked
      // before the idle logic so a storm is diagnosed as a storm, not a
      // generic stall.
      if (activity.maxSessionCompactionsSinceProgress >= stormThreshold) {
        stormCompactions = activity.maxSessionCompactionsSinceProgress
        stormSessionID = activity.stormSessionID
        abortCause = "storm"
        linkedAbort.abort(new Error("invoke_agent run hit the compaction-storm threshold"))
        abortChild()
        return
      }
      const at = now()
      if (activity.fingerprint !== lastFingerprint) {
        lastFingerprint = activity.fingerprint
        lastProgressAt = at
        lastFingerprintChangeAt = at
        return
      }
      const deferralExpired = idleDeferralExpired(activity.runningTools, idleMilliseconds, lastFingerprintChangeAt, at)
      // DT-FR-013: a running tool part counts as activity for the
      // envelope's progress fields even with an unchanged fingerprint,
      // within the DT-FR-014 deferral bound (deferral only ever holds
      // while a tool runs), so last_progress_ms_ago stays at the
      // polling interval while a quiet long tool call is deferred.
      if (!deferralExpired) {
        lastProgressAt = at
      }
      // Idle abort (DT-FR-002/DT-FR-014): no fingerprint change for the
      // idle window AND the running-tool deferral bound expired. A true
      // stall between tool calls has no running parts and defers
      // nothing; a quiet long tool call holds idle until its deferral
      // bound (earliest observable deadline + grace, else the fixed cap
      // from the last fingerprint change). Wall clock remains the
      // absolute backstop either way.
      if (idleMilliseconds !== undefined && at - lastFingerprintChangeAt >= idleMilliseconds && deferralExpired) {
        abortCause = "idle"
        linkedAbort.abort(new Error("invoke_agent run made no progress"))
        abortChild()
      }
    } finally {
      sampling = false
    }
  }
  const probePoll = setInterval(() => void sample(), deps.idlePollMilliseconds ?? DEFAULT_IDLE_POLL_MS)

  if (deps.parentAbort.aborted) {
    cancelFromParent()
  } else {
    deps.parentAbort.addEventListener("abort", cancelFromParent, { once: true })
  }

  /** Attach childSessionID to any result returned after session creation. */
  const withChild = (result: InvokeAgentResult): InvokeAgentResult =>
    childID !== null ? { ...result, childSessionID: childID } : result

  try {
    const created = await deps.client.session.create({
      body: {
        parentID: deps.sessionID,
        title: `${agent} @ ${model.combined}${variant === undefined ? "" : ` (${variant})`}`,
      },
      query: { directory: deps.directory },
      signal: linkedAbort.signal,
    })
    childID = created.data?.id ?? null
    if (childID === null) {
      return failedResult(provenance, "child_create_failed", responseFailure("create child session", created), true)
    }
    // Nested runs inside this child inherit its absolute deadline (D1)
    // and, when this run carries an idle watchdog, its idle window
    // (per-task-delegation-budgets D2): the registry entry is the one
    // place a nested per-task call resolves both from.
    registerDeadline(childID, startedAt + effectiveMilliseconds, idleMilliseconds)
    if (linkedAbort.signal.aborted) {
      abortChild()
      throw linkedAbort.signal.reason
    }

    const body: PromptBodyWithVariant = {
      agent,
      model: { providerID: model.providerID, modelID: model.modelID },
      ...(variant === undefined ? {} : { variant }),
      parts: [{ type: "text", text: promptText }],
    }
    const prompted = await deps.client.session.prompt({
      path: { id: childID },
      query: { directory: deps.directory },
      body,
      signal: linkedAbort.signal,
    })
    settled = true
    // Reclassify abort-driven empty responses as cancellation, not prompt failure.
    if (linkedAbort.signal.aborted) {
      throw linkedAbort.signal.reason
    }
    if (prompted.data === undefined) {
      return withChild(failedResult(provenance, "child_prompt_failed", responseFailure("prompt child session", prompted), true))
    }

    const childModel = reportedModel(prompted.data.info.providerID, prompted.data.info.modelID)
    const mismatch = childModel !== model.combined
    provenance = { ...provenance, reported_child_model: childModel, model_mismatch: mismatch }
    const text = responseText(prompted.data.parts)
    const usage = invocationUsage(prompted.data.parts)
    const finishReason = responseFinishReason(prompted.data.parts)

    if (prompted.data.info.error !== undefined) {
      return withChild({
        ...failedResult(
          provenance,
          "child_message_error",
          errorText(prompted.data.info.error),
          "isRetryable" in prompted.data.info.error.data
            ? prompted.data.info.error.data.isRetryable
            : prompted.data.info.error.name !== "MessageAbortedError",
          "failed",
          text,
          usage,
        ),
        message_error_name: prompted.data.info.error.name,
      })
    }
    // Fail-closed guard (RD-FR-059, design D1): a child whose final text is
    // empty after trimming did not complete its response — including the
    // length-terminated case, whose finish reason is preserved in the error
    // message for diagnostics. The guard sits after the info.error
    // classification so session aborts keep their child_message_error
    // signature (trigger disjointness with RD-FR-026) and before the
    // model-mismatch check, per design D1.
    if (text.trim().length === 0) {
      const reasonDetail = finishReason === null ? "" : ` (finish reason: ${finishReason})`
      return withChild(
        failedResult(
          provenance,
          "child_empty_response",
          `child returned no final text${reasonDetail}; model=${childModel ?? model.combined}`,
          true,
          "failed",
          text,
          usage,
        ),
      )
    }
    if (mismatch) {
      return withChild(
        failedResult(
          provenance,
          "model_mismatch",
          `child reported ${childModel ?? "an unavailable model"}; expected ${model.combined}`,
          false,
          "failed",
          text,
          usage,
        ),
      )
    }

    // Final-text success guard (compaction-storm design D3, DELEG-FR-003):
    // a child whose final message is a compaction summary or the synthetic
    // continuation prompt is not a real completion, so it MUST NOT map to
    // success. Classification is structural (classifySyntheticText:
    // anchored, case-insensitive, length-bounded wrappers), so legitimate
    // prose that merely mentions compaction stays success. On a match, one
    // final activity probe folds the subtree's end state -- markers the
    // interval probes had not yet sampled, including the final message
    // itself when it is structurally a marker -- so the failure code
    // reflects whether the storm threshold was actually met in one counted
    // session, naming that session; when that probe cannot run, the
    // accounting the interval probes already folded is the best available
    // answer.
    const syntheticKind = classifySyntheticText(text)
    if (syntheticKind !== null) {
      const finalSample = await probeSubtreeActivity(deps.client, childID, deps.directory, linkedAbort.signal)
      const accounting = finalSample ?? registeredProbeCounters(childID)
      const compactions = accounting?.maxSessionCompactionsSinceProgress ?? 0
      const stormSession = accounting?.stormSessionID ?? null
      // The preserved excerpt is the final message text alone -- never the
      // cached prefix or earlier context -- truncated to the documented
      // bound; failedResult routes it through sanitizeInvocationError so
      // secrets and home paths are redacted before it reaches the error.
      const preserved = text.slice(0, MAX_PRESERVED_FINAL_TEXT_LENGTH)
      const structure = syntheticTextName(syntheticKind)
      if (compactions >= stormThreshold) {
        return withChild({
          ...failedResult(
            provenance,
            "context_thrash",
            `invoke_agent final text structurally matches a ${structure} and the storm threshold was met ` +
              `(${compactions} consecutive compaction events with no substantive progress ` +
              `${stormSessionClause(stormSession)}, storm threshold ${stormThreshold}); reclassified from ` +
              `success so the caller can resume from durable state. Preserved final text: ${preserved}`,
            true,
            "failed",
            "",
            usage,
          ),
          timeout: timeoutDetails("storm", stormSession),
          resume: {
            project_path: deps.directory,
            agent_name: agent,
            pending_checkpoint_step: deps.checkpointStep ?? null,
          },
        })
      }
      return withChild(
        failedResult(
          provenance,
          "synthetic_final_text",
          `invoke_agent final text structurally matches a ${structure}, not a real completion; ` +
            `reclassified from success as retryable. Preserved final text: ${preserved}`,
          true,
          "failed",
          "",
          usage,
        ),
      )
    }

    return withChild({ status: "success", text, usage, error: null, provenance, finish_reason: finishReason })
  } catch (error: unknown) {
    if (abortCause === "timeout") {
      return withChild({
        ...failedResult(provenance, "timeout", "invoke_agent run timed out", true),
        timeout: timeoutDetails("wall_clock"),
      })
    }
    if (abortCause === "idle") {
      return withChild({
        ...failedResult(
          provenance,
          "idle_timeout",
          `invoke_agent run made no progress for ${idleMilliseconds} ms`,
          true,
        ),
        timeout: timeoutDetails("idle"),
      })
    }
    if (abortCause === "storm") {
      return withChild({
        ...failedResult(
          provenance,
          "context_thrash",
          `invoke_agent run recorded ${stormCompactions} consecutive compaction events ` +
            `with no substantive progress ${stormSessionClause(stormSessionID)} ` +
            `(storm threshold ${stormThreshold}); aborting so the caller can resume from durable state`,
          true,
        ),
        timeout: timeoutDetails("storm", stormSessionID),
        resume: {
          project_path: deps.directory,
          agent_name: agent,
          pending_checkpoint_step: deps.checkpointStep ?? null,
        },
      })
    }
    if (abortCause === "cancelled") {
      return withChild(failedResult(provenance, "cancelled", "invoke_agent run was cancelled", false, "cancelled"))
    }
    return withChild(failedResult(provenance, "invocation_failed", errorText(error), true))
  } finally {
    settled = true
    if (wallClockTimer !== undefined) {
      clearTimeout(wallClockTimer)
    }
    clearInterval(probePoll)
    if (childID !== null) {
      unregisterDeadline(childID)
      // Drop the probe accounting with the deadline: the registry is keyed
      // by child session id and every run probes now that the storm
      // detector is always armed, so finished children must not accumulate
      // accounting state.
      unregisterProbeState(childID)
    }
    deps.parentAbort.removeEventListener("abort", cancelFromParent)
    await cleanupAbort
  }
}

// ── Per-task delegation executor (per-task-delegation-budgets) ─

/**
 * Seams capturing the advisory resume state (design D3, PTB-FR-003).
 * Both return repository-relative identifiers and null when the state
 * is unreadable, which fails the hint closed. Production
 * implementations live in the plugin layer (worktree diff or session
 * file tracking; tasks.md checkbox state under the parse_tasks
 * grammar); the executor only diffs what they report, so no new I/O
 * or filesystem watcher enters the lib (Constitution IV/V).
 */
export interface PerTaskResumeCapture {
  /**
   * Captures the repository-relative paths currently modified in the
   * worktree, or null when unreadable.
   *
   * @returns The modified paths, or null when the state is unreadable.
   */
  readonly captureModifiedFiles: () => Promise<readonly string[] | null>
  /**
   * Captures the task identifiers currently marked complete in
   * tasks.md, or null when unreadable.
   *
   * @returns The completed task identifiers, or null when unreadable.
   */
  readonly captureCompletedTasks: () => Promise<readonly string[] | null>
}

/**
 * Context identifying one per-task /uf.unleash implementation
 * delegation (design D2/D3, PTB-FR-001..003): the tasks.md phase the
 * task belongs to, the checkpoint step the caller asked the run to
 * record, the derived (or explicit) timeout with its provenance, the
 * idle fallback for unbudgeted callers, and the optional
 * resume-capture seams.
 */
export interface PerTaskDelegationContext {
  /** 1-based tasks.md phase number of the delegated task. */
  readonly phase: number
  /** Checkpoint step the caller asked the run to record; "implement" marks a resume-eligible per-task delegation. */
  readonly checkpointStep?: string
  /**
   * Derivation outcome from derivePerTaskTimeoutMs (PTB-FR-001). Its
   * effectiveMs is the requested wall clock and its provenance record
   * rides the result verbatim; absent when the caller supplied only
   * an explicit timeoutMilliseconds, which then passes through with
   * source "explicit".
   */
  readonly derivation?: DerivedPerTaskTimeout
  /**
   * Idle window applied only when neither an explicit
   * idleTimeoutMilliseconds nor a budgeted ancestor supplies one: the
   * caller threads its existing delegation default, keeping that
   * default's single source of truth outside this lib and unbudgeted
   * per-task callers behaving exactly as before (PTB-FR-002
   * introduces no new global idle default).
   */
  readonly fallbackIdleMs?: number
  /** Resume-capture seams; absent on call sites that cannot supply them. */
  readonly resumeCapture?: PerTaskResumeCapture
}

/** One fail-closed snapshot of the resume state (design D3). */
interface ResumeSnapshot {
  readonly files: readonly string[]
  readonly tasks: readonly string[]
}

/**
 * Capture one resume snapshot through both seams concurrently. Any
 * throw or null return marks the snapshot unreadable so the hint can
 * fail closed without masking the timeout it annotates.
 *
 * @param capture The resume-capture seams.
 * @returns The snapshot, or null when either side is unreadable.
 */
async function captureResumeSnapshot(capture: PerTaskResumeCapture): Promise<ResumeSnapshot | null> {
  try {
    const [files, tasks] = await Promise.all([capture.captureModifiedFiles(), capture.captureCompletedTasks()])
    if (files === null || tasks === null) {
      return null
    }
    return { files, tasks }
  } catch {
    return null
  }
}

/**
 * Assemble the advisory resume hint by diffing a fresh after-state
 * against the before-state captured at delegation start (PTB-FR-003):
 * the files and tasks newly modified or completed since the
 * delegation started are the child's observable progress. An
 * unreadable snapshot on either side yields null — the executor
 * never synthesizes resume content (fail-closed, per D4 of #950).
 *
 * @param capture The resume-capture seams.
 * @param before The delegation-start snapshot, or null when it was unreadable.
 * @returns The hint, or null when the state is unreadable.
 */
async function assembleResumeHint(
  capture: PerTaskResumeCapture,
  before: ResumeSnapshot | null,
): Promise<PerTaskResumeHint | null> {
  if (before === null) {
    return null
  }
  const after = await captureResumeSnapshot(capture)
  if (after === null) {
    return null
  }
  return {
    files_modified: after.files.filter((path) => !before.files.includes(path)),
    tasks_completed: after.tasks.filter((task) => !before.tasks.includes(task)),
  }
}

/** Whether a timeout kind's termination carries the per-task resume hint (PTB-FR-003). */
function carriesResumeHint(kind: TimeoutKind): boolean {
  return kind === "wall_clock" || kind === "idle" || kind === "budget"
}

/**
 * Render the resume preamble the executor prepends to the one bounded
 * retry's prompt (PTB-FR-003, DE-005): the retry opens by relaying the
 * first attempt's observable progress so the child verifies and
 * continues from durable state instead of re-deriving it — the retry
 * never starts from a blank context by construction.
 *
 * A readable hint relays its files and tasks verbatim. A null
 * (unreadable) hint falls back to a fresh capture for the
 * completed-task anchor, and finally to directing the child to the
 * tasks.md checkbox state itself. The preamble is mechanical prose
 * assembled from already-observable state, never semantic analysis
 * (Constitution V).
 *
 * @param kind Timeout kind that ended the first attempt.
 * @param phase 1-based tasks.md phase number of the delegated task.
 * @param hint The first attempt's resume hint, or null when unreadable.
 * @param capture The resume-capture seams, when the caller supplied them.
 * @returns The preamble text, and whether it carried concrete relayed
 *   state (vs. the checkbox self-read fallback).
 */
async function renderResumePreamble(
  kind: TimeoutKind,
  phase: number,
  hint: PerTaskResumeHint | null,
  capture: PerTaskResumeCapture | undefined,
): Promise<{ preamble: string; relayed: boolean }> {
  // Mechanical article for the kind interpolation (only "idle" takes
  // "an"); the preamble stays fully deterministic.
  const article = kind === "idle" ? "an" : "a"
  if (hint !== null) {
    const files = hint.files_modified.length > 0 ? hint.files_modified.join(", ") : "none"
    const tasks = hint.tasks_completed.length > 0 ? hint.tasks_completed.join(", ") : "none"
    return {
      preamble:
        `This is the single retry of a per-task delegation that ended in ${article} ${kind} timeout; ` +
        "the attempt budget is spent, so continue efficiently.\n\n" +
        `The timed-out attempt already made this durable progress:\n` +
        `- Files modified: ${files}\n` +
        `- Tasks completed: ${tasks}\n\n` +
        "Verify that state against the working tree and tasks.md, continue from it, " +
        "and do not redo completed work.\n\n",
      relayed: true,
    }
  }
  // Fail-closed fallback (D4 of #950): the hint state is unreadable, so
  // try one fresh capture for the completed-task anchor before
  // directing the child to the checkboxes itself.
  if (capture !== undefined) {
    const snapshot = await captureResumeSnapshot(capture)
    if (snapshot !== null) {
      const tasks = snapshot.tasks.length > 0 ? snapshot.tasks.join(", ") : "none"
      return {
        preamble:
          `This is the single retry of a per-task delegation that ended in ${article} ${kind} timeout; ` +
          "the attempt budget is spent, so continue efficiently.\n\n" +
          `The timed-out attempt's diff against its start could not be read, but the ` +
          `tasks.md checkbox state at retry time lists these phase tasks as complete: ${tasks}.\n\n` +
          "Verify that state against tasks.md, treat every [x] checkbox in the phase as " +
          "complete, continue from the first unchecked task, and do not redo completed work.\n\n",
        relayed: true,
      }
    }
  }
  return {
    preamble:
      `This is the single retry of a per-task delegation that ended in ${article} ${kind} timeout; ` +
      "the attempt budget is spent, so continue efficiently.\n\n" +
      `The timed-out attempt's resume state could not be read. Anchor on the tasks.md ` +
      `checkbox state instead: read tasks.md, treat every [x] checkbox in phase ${phase} as ` +
      "complete, continue from the first unchecked task, and do not redo completed work.\n\n",
    relayed: false,
  }
}

/**
 * Executes one per-task /uf.unleash implementation delegation with
 * hierarchical budgeting, inherited idle semantics, derivation
 * provenance, and the advisory resume hint
 * (per-task-delegation-budgets design D2/D3, PTB-FR-001..003,
 * AE-FR-010).
 *
 * Wraps executeAgentSession. Before any child session exists it
 * resolves the inherited budget through the delegation-budget
 * registry (direct hit on the caller's session, else at most 8
 * parent hops), clamps the requested — derived or explicit — wall
 * clock to the ancestor deadline minus the 60 s reserve, and fails
 * fast with a non-retryable budget_exhausted envelope when less than
 * the 30 s minimum remains. It resolves the idle watchdog's
 * applicability (an explicit window wins, else the budgeted
 * ancestor's registered window, else the caller's existing fallback
 * for unbudgeted runs — never a new global default) and arms the
 * wall-clock deferral for per-task children of a budgeted ancestor
 * carrying idle, so a progressing child is not killed by its derived
 * clock while the inherited budget stays the absolute backstop
 * (PTB-FR-002). Every result carries the per-task timeout
 * provenance; wall-clock, idle, and budget terminations additionally
 * carry the advisory resume hint inside the timeout envelope — null
 * when the underlying state is unreadable — for delegations that
 * identify as implement-phase per-task calls (checkpoint_step
 * "implement" plus a positive phase). Other callers of
 * executeAgentSession are untouched.
 *
 * @param deps Injected client, session context, and timeout boundary.
 * @param agent Manifest-declared agent name the child runs as.
 * @param promptText Resolved prompt text to deliver to the child session.
 * @param model Resolved model identity for the child session.
 * @param variant Optional runtime variant for the child session.
 * @param read_only Invocation provenance flag.
 * @param provenance Initial provenance populated by the caller.
 * @param perTask Per-task context: phase, checkpoint step, derivation,
 *   idle fallback, and resume-capture seams.
 * @returns Observable result with status, text, usage, error,
 *   provenance, the finish reason on success, per-task timeout
 *   provenance, and the resume hint on timeout terminations.
 */
export async function executePerTaskDelegation(
  deps: ExecutorDependencies,
  agent: string,
  promptText: string,
  model: ModelIdentity,
  variant: string | undefined,
  read_only: boolean,
  provenance: InvocationProvenance,
  perTask: PerTaskDelegationContext,
): Promise<InvokeAgentResult> {
  const now = deps.now ?? Date.now
  const startedAt = now()
  // Hierarchical budget (D2): resolve the ancestor entry before any
  // child session exists, so the fail-fast below can refuse to spawn
  // and the idle applicability can inherit through the same entry.
  const inherited = await resolveInheritedBudget(deps.client, deps.sessionID, deps.directory, deps.parentAbort)
  const requestedMs = perTask.derivation?.effectiveMs ?? deps.timeoutMilliseconds
  const budget = clampToBudget(requestedMs, inherited === null ? null : inherited.deadlineMs, startedAt)
  // Idle applicability (D2, PTB-FR-002): an explicit window wins,
  // else the budgeted ancestor's registered window, else the caller's
  // existing fallback — never a new global default.
  const applicability = resolveIdleApplicability(deps.idleTimeoutMilliseconds, inherited)
  const idleMilliseconds = applicability.idleMs ?? perTask.fallbackIdleMs
  // Timeout provenance (D1, PTB-FR-001): the derivation record rides
  // the result verbatim; a call without one passes its explicit
  // request through with source "explicit".
  const timeoutProvenance: PerTaskTimeoutProvenanceRecord =
    perTask.derivation?.provenance ?? { effective_ms: requestedMs, source: "explicit" }
  // Resume applicability (D3, PTB-FR-003): only implement-phase
  // per-task calls (checkpoint_step "implement" plus a positive
  // phase) carry the hint; every other caller is unaffected.
  const resumeEligible = perTask.checkpointStep === "implement" && perTask.phase > 0
  const capture = resumeEligible ? perTask.resumeCapture : undefined
  const before = capture === undefined ? null : await captureResumeSnapshot(capture)

  if (budget.exhausted) {
    const details: TimeoutDetails = {
      kind: "budget",
      effective_ms: budget.effectiveMs,
      requested_ms: requestedMs,
      budget_clamped: budget.clamped,
      idle_timeout_ms: idleMilliseconds ?? null,
      elapsed_ms: now() - startedAt,
      last_progress_ms_ago: null,
      child_active: null,
      pending_question: false,
      running_tool: null,
      storm_session_id: null,
    }
    return {
      ...failedResult(
        provenance,
        "budget_exhausted",
        `parent delegation budget exhausted (${budget.effectiveMs} ms left after the reserve); ` +
          "return to the caller so it can resume from durable state",
        false,
      ),
      timeout: resumeEligible
        ? { ...details, resume: capture === undefined ? null : await assembleResumeHint(capture, before) }
        : details,
      timeout_provenance: timeoutProvenance,
      // PTB-FR-004: budget_exhausted reports what little budget remains
      // (floored at zero) so the caller can record it at its checkpoint.
      // The exhausted branch only fires under a budgeted ancestor, but
      // the guard keeps the narrowing local rather than trusting the
      // clamp contract.
      remaining_ms: inherited === null ? undefined : Math.max(0, inherited.deadlineMs - now()),
    }
  }

  // The shared executor owns the child lifecycle: it re-resolves the
  // same registry entry and re-clamps (its clamp is never looser),
  // registers the child's deadline — carrying the resolved idle
  // window — and unregisters it when the run ends. Both the first
  // attempt and the bounded retry run through this wiring, so they
  // share the inherited budget and idle semantics.
  const runAttempt = (prompt: string): Promise<InvokeAgentResult> =>
    executeAgentSession(
      {
        ...deps,
        timeoutMilliseconds: requestedMs,
        idleTimeoutMilliseconds: idleMilliseconds,
        checkpointStep: deps.checkpointStep ?? perTask.checkpointStep,
        deferWallClockWhileActive: inherited !== null && idleMilliseconds !== undefined,
      },
      agent,
      prompt,
      model,
      variant,
      read_only,
      provenance,
    )
  // PTB-FR-004: every result under a budgeted ancestor carries the
  // remaining budget at result time; unbudgeted runs omit the field.
  // Recomputed at each attempt's result-assembly time.
  const decorate = (result: InvokeAgentResult): InvokeAgentResult => ({
    ...result,
    timeout_provenance: timeoutProvenance,
    remaining_ms: inherited === null ? undefined : Math.max(0, inherited.deadlineMs - now()),
  })
  // Attach the advisory resume hint to the final result's timeout
  // envelope (PTB-FR-003): the after-state at result-assembly time
  // diffed against the delegation-start snapshot.
  const finalize = async (result: InvokeAgentResult): Promise<InvokeAgentResult> => {
    if (!resumeEligible || result.timeout === undefined || !carriesResumeHint(result.timeout.kind)) {
      return result
    }
    return {
      ...result,
      timeout: {
        ...result.timeout,
        resume: capture === undefined ? null : await assembleResumeHint(capture, before),
      },
    }
  }

  const first = decorate(await runAttempt(promptText))
  // Executor-owned bounded retry (DE-005, PTB-FR-003): a retryable
  // timeout termination of a resume-eligible per-task delegation is
  // re-issued exactly once, with the resume context tool-rendered into
  // the retry prompt — never a blank-context re-issue, never a second
  // retry. Non-retryable terminations (budget_exhausted), storm
  // terminations and other kinds without a per-task resume hint, and
  // non-timeout failures return to the caller immediately; a failure
  // that carries the retry record already consumed the bound.
  if (
    !resumeEligible ||
    first.timeout === undefined ||
    first.error?.retryable !== true ||
    !carriesResumeHint(first.timeout.kind)
  ) {
    return await finalize(first)
  }
  const hint = capture === undefined ? null : await assembleResumeHint(capture, before)
  const { preamble, relayed } = await renderResumePreamble(first.timeout.kind, perTask.phase, hint, capture)
  const retried = decorate(await runAttempt(preamble + promptText))
  return await finalize({
    ...retried,
    retry: { first_attempt_kind: first.timeout.kind, resume_relayed: relayed },
  })
}
