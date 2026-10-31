import { type PluginInput } from "@opencode-ai/plugin"
import { z } from "zod"

// ── Constants ────────────────────────────────────────────────

export const MAX_ERROR_LENGTH = 4_096
export const CLEANUP_ABORT_TIMEOUT_MS = 5_000
export const DEFAULT_TIMEOUT_MILLISECONDS = 600_000
export const MAX_INLINE_PROMPT_BYTES = 128 * 1024
export const MAX_PROMPT_FILE_BYTES = 4 * 1024 * 1024

// ── Shared Zod schemas ──────────────────────────────────────

export const PromptSchema = z
  .string()
  .refine((value) => Buffer.byteLength(value, "utf8") <= MAX_INLINE_PROMPT_BYTES, "prompt exceeds 128 KiB UTF-8")
export const PromptFileSchema = z.string().max(1024)
export const TimeoutSchema = z.number().int().positive().max(1_800_000)
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

/** Observable result from exactly one policy-free agent invocation. */
export interface InvokeAgentResult {
  readonly status: "success" | "failed" | "cancelled"
  readonly text: string
  readonly usage: InvocationUsage | null
  readonly error: InvocationError | null
  readonly provenance: InvocationProvenance
  /** Child session ID created by the executor (absent on early validation failures). */
  readonly childSessionID?: string
}

/** Injected dependencies for the shared executor's child session lifecycle. */
export interface ExecutorDependencies {
  readonly client: PluginInput["client"]
  readonly sessionID: string
  readonly directory: string
  readonly parentAbort: AbortSignal
  readonly timeoutMilliseconds: number
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
  if (!Number.isSafeInteger(timeoutMilliseconds) || timeoutMilliseconds <= 0 || timeoutMilliseconds > 1_800_000) {
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
 * and cleanup. Does NOT own: input validation, manifest checking,
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
  const linkedAbort = new AbortController()
  let abortCause: "cancelled" | "timeout" | null = null
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
    abortCause = "timeout"
    linkedAbort.abort(new Error("invoke_agent run timed out"))
    abortChild()
  }, deps.timeoutMilliseconds)

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
      return withChild(failedResult(provenance, "timeout", "invoke_agent run timed out", true))
    }
    if (abortCause === "cancelled") {
      return withChild(failedResult(provenance, "cancelled", "invoke_agent run was cancelled", false, "cancelled"))
    }
    return withChild(failedResult(provenance, "invocation_failed", errorText(error), true))
  } finally {
    clearTimeout(timeout)
    deps.parentAbort.removeEventListener("abort", cancelFromParent)
    await cleanupAbort
  }
}
