import { type PluginInput } from "@opencode-ai/plugin"
import { z } from "zod"

import {
  ACTIVE_WINDOW_MS,
  DEFAULT_IDLE_POLL_MS,
  clampToBudget,
  probeSubtreeActivity,
  registerDeadline,
  resolveInheritedDeadline,
  unregisterDeadline,
} from "./delegation-budget.js"

// ── Constants ────────────────────────────────────────────────

export const MAX_ERROR_LENGTH = 4_096
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
 * Which limit ended a run (delegation-timeout-budgets D4): the wall
 * clock, the idle (no-progress) window, or an exhausted inherited
 * budget that prevented the run from starting.
 */
export type TimeoutKind = "wall_clock" | "idle" | "budget"

/** Limit details attached to runs that ended by a timeout (design D4). */
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
  /** Limit details, present only when a timeout or exhausted budget ended the run. */
  readonly timeout?: TimeoutDetails
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
  const sanitized = message
    .replace(/\bBearer\s+[^\s,;"']+/gi, "Bearer [REDACTED]")
    .replace(
      /\b(api[-_ ]?key|access[-_ ]?token|refresh[-_ ]?token|token|secret|password|credential)s?\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;"']+)/gi,
      "$1$2[REDACTED]",
    )
    .replace(/(https?:\/\/[^\s?#]+)\?[^\s#]*/gi, "$1?[REDACTED]")
    .replace(/(?:\/Users|\/home)\/[^/\s]+(?:\/[^\s,;:"']*)?/g, "~/[REDACTED]")
    .replace(/[A-Za-z]:\\+Users\\+[^\\\s]+(?:\\+[^\s,;:"']*)?/g, "~\\[REDACTED]")
  return sanitized.slice(0, MAX_ERROR_LENGTH)
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
 * response parsing (text, usage, mismatch), timeout, cancellation,
 * and cleanup, plus the delegation-timeout-budgets limits: the wall
 * clock is clamped to any inherited parent budget (D1), the child's
 * deadline is registered for nested runs, the optional idle watchdog
 * aborts a child whose session subtree stops changing (D2), and
 * limit-ended results carry `timeout` details (D4). Does NOT own: input validation, manifest checking,
 * model resolution, host introspection, or prompt file reading.
 *
 * @param deps Injected client, session context, and timeout boundary.
 * @param agent Manifest-declared divisor-* review agent name.
 * @param promptText Resolved prompt text to deliver to the child session.
 * @param model Resolved model identity for the child session.
 * @param variant Optional runtime variant for the child session.
 * @param read_only Invocation provenance flag (default true).
 * @param provenance Initial provenance populated by the caller.
 * @returns Observable result with status, text, usage, error, and provenance.
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

  let lastProgressAt = startedAt
  let lastFingerprint: string | null = null
  let probed = false
  let pendingQuestion = false
  const timeoutDetails = (kind: TimeoutKind): TimeoutDetails => {
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
  let abortCause: "cancelled" | "timeout" | "idle" | null = null
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
  const timeout = setTimeout(() => {
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
  }, effectiveMilliseconds)

  // Idle watchdog (design D2): a changed subtree fingerprint is progress;
  // a failed probe neither resets nor advances the idle clock, so only a
  // successful, unchanged sample can end the run.
  // `settled` flips as soon as the child's prompt returns (and again in
  // finally), so a probe still in flight when the run ends can never
  // abort a finished child or relabel its result as idle_timeout.
  let sampling = false
  let settled = false
  const sample = async (): Promise<void> => {
    if (settled || sampling || childID === null || abortCause !== null || idleMilliseconds === undefined) {
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
      const at = now()
      if (activity.fingerprint !== lastFingerprint) {
        lastFingerprint = activity.fingerprint
        lastProgressAt = at
        return
      }
      if (at - lastProgressAt >= idleMilliseconds) {
        abortCause = "idle"
        linkedAbort.abort(new Error("invoke_agent run made no progress"))
        abortChild()
      }
    } finally {
      sampling = false
    }
  }
  const idlePoll =
    idleMilliseconds === undefined
      ? undefined
      : setInterval(() => void sample(), deps.idlePollMilliseconds ?? DEFAULT_IDLE_POLL_MS)

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
    // Nested runs inside this child inherit its absolute deadline (D1).
    registerDeadline(childID, startedAt + effectiveMilliseconds)
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

    if (prompted.data.info.error !== undefined) {
      return withChild(
        failedResult(
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

    return withChild({ status: "success", text, usage, error: null, provenance })
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
    if (abortCause === "cancelled") {
      return withChild(failedResult(provenance, "cancelled", "invoke_agent run was cancelled", false, "cancelled"))
    }
    return withChild(failedResult(provenance, "invocation_failed", errorText(error), true))
  } finally {
    settled = true
    clearTimeout(timeout)
    if (idlePoll !== undefined) {
      clearInterval(idlePoll)
    }
    if (childID !== null) {
      unregisterDeadline(childID)
    }
    deps.parentAbort.removeEventListener("abort", cancelFromParent)
    await cleanupAbort
  }
}
