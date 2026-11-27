import { type PluginInput } from "@opencode-ai/plugin"

import { redactInvocationText } from "./agent-executor.js"

/**
 * Report-time recovery for delegated runs (delegate-abort-recovery,
 * issue 1199). When a child session's final report is lost to a message
 * abort or replaced by an injected turn, the delegation wrapper reads the
 * child's persisted transcript once and recovers its last assistant text
 * plus the diagnostics that name the turn that ended the run.
 *
 * Everything here fails open (design D5): an unreadable transcript or an
 * unexpected message shape yields null fields, never an error.
 */

/**
 * Maximum characters of recovered final text kept in a failure envelope
 * (design D4). Reports end with their demo block, so the bound keeps the
 * tail.
 */
export const MAX_DELEGATE_FINAL_TEXT_LENGTH = 16_384

/** Most recent child messages one transcript read examines (design D1). */
export const MAX_RECOVERY_MESSAGES = 50

/** Where a recovered final text came from (DAR-FR-001). */
export type FinalTextSource = "response" | "transcript"

/** The user turn that ended a run and the model that answered it (DAR-FR-003). */
export interface FinalTurn {
  readonly agent: string | null
  readonly model: string | null
  readonly assistant_model: string | null
  /** True when every text part of that user message is marked synthetic. */
  readonly injected: boolean
}

/** Transcript-derived facts about a child session (pure summary). */
export interface TranscriptSummary {
  readonly messages_examined: number
  readonly last_assistant_text: string | null
  readonly last_tool: { readonly tool: string; readonly status: string | null } | null
  readonly last_input_tokens: number | null
  readonly final_turn: FinalTurn | null
}

/** Diagnostics a report-time failure envelope carries (DAR-FR-003). */
export interface ReportDiagnostics {
  readonly error_name: string | null
  readonly elapsed_ms: number
  readonly messages_examined: number | null
  readonly last_tool: TranscriptSummary["last_tool"]
  readonly last_input_tokens: number | null
  readonly final_turn: FinalTurn | null
}

interface RecordLike {
  readonly [key: string]: unknown
}

function record(value: unknown): RecordLike | null {
  return typeof value === "object" && value !== null ? (value as RecordLike) : null
}

function nonEmptyString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null
}

function combinedModel(providerID: unknown, modelID: unknown): string | null {
  const provider = nonEmptyString(providerID)
  const model = nonEmptyString(modelID)
  return provider !== null && model !== null ? `${provider}/${model}` : null
}

function partsOf(message: RecordLike): RecordLike[] {
  const parts = message.parts
  return Array.isArray(parts) ? parts.map(record).filter((part): part is RecordLike => part !== null) : []
}

function textParts(parts: readonly RecordLike[]): RecordLike[] {
  return parts.filter((part) => part.type === "text" && typeof part.text === "string" && part.ignored !== true)
}

function messageText(parts: readonly RecordLike[]): string {
  return textParts(parts)
    .map((part) => part.text as string)
    .join("\n\n")
    .trim()
}

/**
 * Redact and bound a final text for a failure envelope (design D4):
 * secrets and home paths are redacted first, then only the last
 * MAX_DELEGATE_FINAL_TEXT_LENGTH characters are kept behind a marker.
 *
 * @param text The child's final text.
 * @returns The bounded text, or null when it is empty after trimming.
 */
export function boundFinalText(text: string): string | null {
  // Redact the whole text before cutting, so a secret that straddles the
  // cut is never left half-visible.
  const redacted = redactInvocationText(text.trim())
  if (redacted.length === 0) {
    return null
  }
  const dropped = redacted.length - MAX_DELEGATE_FINAL_TEXT_LENGTH
  return dropped > 0 ? `…[truncated ${dropped} chars]\n${redacted.slice(dropped)}` : redacted
}

/**
 * Summarize a child session's most recent messages (pure; DAR-FR-001,
 * DAR-FR-003). Messages arrive oldest first, as the messages endpoint
 * returns them.
 *
 * @param messages The child's recent messages, oldest first.
 * @returns The recovered assistant text and the final-turn diagnostics.
 */
export function summarizeTranscript(messages: readonly unknown[]): TranscriptSummary {
  const entries = messages.map(record).filter((entry): entry is RecordLike => entry !== null)
  let lastAssistantText: string | null = null
  let lastTool: TranscriptSummary["last_tool"] = null
  let lastInputTokens: number | null = null
  let lastAssistantModel: string | null = null
  let lastUser: { readonly info: RecordLike; readonly parts: RecordLike[] } | null = null

  for (const entry of entries) {
    const info = record(entry.info)
    if (info === null) {
      continue
    }
    const parts = partsOf(entry)
    if (info.role === "user") {
      lastUser = { info, parts }
      continue
    }
    if (info.role !== "assistant") {
      continue
    }
    lastAssistantModel = combinedModel(info.providerID, info.modelID) ?? lastAssistantModel
    const text = messageText(parts)
    if (text.length > 0) {
      lastAssistantText = text
    }
    const tokens = record(info.tokens)
    if (tokens !== null && typeof tokens.input === "number") {
      lastInputTokens = tokens.input
    }
    for (const part of parts) {
      const tool = nonEmptyString(part.tool)
      if (part.type === "tool" && tool !== null) {
        lastTool = { tool, status: nonEmptyString(record(part.state)?.status) }
      }
    }
  }

  let finalTurn: FinalTurn | null = null
  if (lastUser !== null) {
    const userModel = record(lastUser.info.model)
    const texts = textParts(lastUser.parts)
    finalTurn = {
      agent: nonEmptyString(lastUser.info.agent),
      model: userModel === null ? null : combinedModel(userModel.providerID, userModel.modelID),
      assistant_model: lastAssistantModel,
      injected: texts.length > 0 && texts.every((part) => part.synthetic === true),
    }
  }

  return {
    messages_examined: entries.length,
    last_assistant_text: lastAssistantText,
    last_tool: lastTool,
    last_input_tokens: lastInputTokens,
    final_turn: finalTurn,
  }
}

/**
 * Read and summarize a child session's recent transcript, failing open
 * (design D1, D5): any transport error, in-band error, or missing data
 * returns null.
 *
 * @param client The plugin's OpenCode client.
 * @param childSessionID The child session to read.
 * @param directory The project directory the session belongs to.
 * @returns The transcript summary, or null when it cannot be read.
 */
export async function readChildTranscript(
  client: PluginInput["client"],
  childSessionID: string,
  directory: string,
): Promise<TranscriptSummary | null> {
  try {
    const response = await client.session.messages({
      path: { id: childSessionID },
      query: { directory, limit: MAX_RECOVERY_MESSAGES },
    })
    if (!Array.isArray(response?.data)) {
      return null
    }
    return summarizeTranscript(response.data)
  } catch {
    return null
  }
}
