import { Buffer } from "node:buffer"
import { readFile } from "node:fs/promises"
import { resolve } from "node:path"

import { tool, type PluginInput, type PluginModule, type ToolContext } from "@opencode-ai/plugin"
import { z } from "zod"

import {
  DEFAULT_TIMEOUT_MILLISECONDS,
  MAX_PROMPT_FILE_BYTES,
  PromptSchema,
  PromptFileSchema,
  TimeoutSchema,
  VariantSchema,
  directModelIdentity,
  errorText,
  executeAgentSession,
  failedResult,
  resolveHostModel,
  sanitizeInvocationError,
  validateTimeout,
  type InvocationProvenance,
  type InvokeAgentResult,
  type ModelIdentity,
} from "../../lib/agent-executor.js"
import {
  createBunYamlParser,
  parseReviewerManifest,
  type YamlParser,
} from "../../lib/reviewer-manifest.js"

// Re-exports for backward compatibility — existing consumers import these from invoke-agent.
export {
  type ModelIdentity,
  type InvocationTokens,
  type InvocationUsage,
  type InvocationError,
  type InvocationProvenance,
  type InvokeAgentResult,
  sanitizeInvocationError,
} from "../../lib/agent-executor.js"

const MANIFEST_PATH = ".uf/reviewer-capabilities.yaml"

const AgentSchema = z.string().max(128).regex(/^divisor-[a-z0-9-]{1,63}$/)
const ModelSchema = z
  .string()
  .max(256)
  .refine((value) => {
    const separator = value.indexOf("/")
    return separator > 0 && separator < value.length - 1
  }, "model must contain non-empty provider and model-id values separated by the first slash")
const InvokeAgentInputSchema = z
  .object({
    agent: AgentSchema,
    prompt: PromptSchema.optional(),
    promptFile: PromptFileSchema.optional(),
    model: ModelSchema.optional(),
    variant: VariantSchema.optional(),
    read_only: z.boolean().optional(),
    timeout: TimeoutSchema.optional(),
  })
  .strict()
  .superRefine((input, context) => {
    if (input.model === undefined && input.variant !== undefined) {
      context.addIssue({
        code: "custom",
        path: ["variant"],
        message: "variant requires an explicit model; host runs replay the active parent variant",
      })
    }
    if (input.prompt !== undefined && input.promptFile !== undefined) {
      context.addIssue({
        code: "custom",
        path: ["promptFile"],
        message: "prompt and promptFile are mutually exclusive",
      })
    }
    if (input.prompt === undefined && input.promptFile === undefined) {
      context.addIssue({
        code: "custom",
        path: ["prompt"],
        message: "one of prompt or promptFile is required",
      })
    }
  })

type ValidatedInput = z.output<typeof InvokeAgentInputSchema>

/** Injectable I/O, client, parser, and timeout boundary used by invoke_agent. */
export interface InvokeAgentDependencies {
  readonly client: PluginInput["client"]
  readonly readText: (relativePath: string) => Promise<string>
  readonly parseYaml: YamlParser
  readonly timeoutMilliseconds: number
}

function initialProvenance(input: ValidatedInput): InvocationProvenance {
  return {
    agent: input.agent,
    requested_model: input.model ?? null,
    requested_variant: input.model === undefined ? null : (input.variant ?? null),
    resolved_parent_model: null,
    resolved_parent_variant: null,
    reported_child_model: null,
    model_mismatch: false,
    read_only: input.read_only ?? true,
  }
}

async function validateReviewAgent(input: ValidatedInput, dependencies: InvokeAgentDependencies): Promise<void> {
  const text = await dependencies.readText(MANIFEST_PATH)
  const manifest = parseReviewerManifest(text, dependencies.parseYaml)
  const reviewer = manifest.reviewers.find((candidate) => candidate.agent === input.agent)
  if (reviewer === undefined) {
    throw new Error(`agent ${input.agent} is absent from the reviewer manifest`)
  }
  if (reviewer.capability !== "review") {
    throw new Error(`agent ${input.agent} has content capability and cannot perform a review invocation`)
  }
}

/**
 * Executes exactly one manifested Divisor review-agent run.
 * @param rawInput Untrusted invocation arguments from the OpenCode tool boundary.
 * @param context Calling tool context used for parenting, host resolution, and cancellation.
 * @param dependencies Injected client, manifest reader/parser, and bounded timeout.
 * @returns Child text, nullable reported usage, and separated request/runtime provenance.
 */
export async function invokeAgent(
  rawInput: unknown,
  context: ToolContext,
  dependencies: InvokeAgentDependencies,
): Promise<InvokeAgentResult> {
  const parsed = InvokeAgentInputSchema.safeParse(rawInput)
  if (!parsed.success) {
    const placeholder: InvocationProvenance = {
      agent: "invalid",
      requested_model: null,
      requested_variant: null,
      resolved_parent_model: null,
      resolved_parent_variant: null,
      reported_child_model: null,
      model_mismatch: false,
      read_only: true,
    }
    return failedResult(placeholder, "invalid_input", parsed.error.message, false)
  }

  const input = parsed.data
  const timeoutMilliseconds = input.timeout ?? dependencies.timeoutMilliseconds
  let provenance = initialProvenance(input)

  // Resolve prompt text: inline prompt or file-backed prompt.
  let promptText: string
  if (input.promptFile !== undefined) {
    try {
      const fileContent = await dependencies.readText(input.promptFile)
      if (Buffer.byteLength(fileContent, "utf8") > MAX_PROMPT_FILE_BYTES) {
        return failedResult(provenance, "invalid_input", "promptFile content exceeds 4 MiB UTF-8", false)
      }
      promptText = fileContent
    } catch (error: unknown) {
      return failedResult(provenance, "prompt_file_read_failed", sanitizeInvocationError(errorText(error)), false)
    }
  } else {
    // superRefine guarantees exactly one of prompt/promptFile is present.
    promptText = input.prompt!
  }

  try {
    validateTimeout(timeoutMilliseconds)
    await validateReviewAgent(input, dependencies)
  } catch (error: unknown) {
    return failedResult(provenance, "invalid_invocation", errorText(error), false)
  }

  // Resolve model: host introspection or direct model identity.
  let selectedModel: ModelIdentity
  let selectedVariant: string | undefined
  if (input.model === undefined) {
    let host
    try {
      host = await resolveHostModel(
        { client: dependencies.client, sessionID: context.sessionID, messageID: context.messageID, directory: context.directory },
        context.abort,
      )
    } catch (error: unknown) {
      if (context.abort.aborted) {
        return failedResult(provenance, "cancelled", "invoke_agent run was cancelled", false, "cancelled")
      }
      return failedResult(provenance, "host_model_unavailable", errorText(error), true)
    }
    selectedModel = host.identity
    selectedVariant = host.variant
    provenance = {
      ...provenance,
      resolved_parent_model: host.identity.combined,
      resolved_parent_variant: host.variant ?? null,
    }
  } else {
    selectedModel = directModelIdentity(input.model)
    selectedVariant = input.variant
  }

  // Delegate session lifecycle to the shared executor.
  return executeAgentSession(
    {
      client: dependencies.client,
      sessionID: context.sessionID,
      directory: context.directory,
      parentAbort: context.abort,
      timeoutMilliseconds,
    },
    input.agent,
    promptText,
    selectedModel,
    selectedVariant,
    input.read_only ?? true,
    provenance,
  )
}

/**
 * Creates the invoke_agent OpenCode tool with isolated runtime dependencies.
 * @param dependencies Client, manifest, parser, and timeout dependencies.
 * @returns The policy-free OpenCode tool definition.
 */
export function createInvokeAgentTool(dependencies: InvokeAgentDependencies): ReturnType<typeof tool> {
  return tool({
    description:
      "Execute one already-planned Divisor review-agent run in a parented child session with explicit model provenance.",
    args: {
      agent: AgentSchema.describe("Manifest-declared divisor-* review agent."),
      prompt: PromptSchema.optional().describe("Bounded full prompt for this single planned run."),
      promptFile: PromptFileSchema.optional().describe(
        "Absolute path to a file containing the prompt text. Mutually exclusive with prompt. Max 1024 chars.",
      ),
      model: ModelSchema.optional().describe(
        "Optional direct provider/model-id selected by the dispatch plan. Include the full slug with any @suffix (e.g. provider/model-id@default).",
      ),
      variant: VariantSchema.optional().describe("Optional direct-model runtime variant."),
      read_only: z.boolean().optional().describe("Invocation provenance only; does not alter permissions."),
      timeout: TimeoutSchema.optional().describe("Optional run timeout in milliseconds (bounded to 1_800_000)."),
    },
    async execute(args, context): Promise<{ readonly output: string; readonly metadata: Record<string, unknown> }> {
      const result = await invokeAgent(args, context, dependencies)
      return {
        output:
          result.error === null
            ? result.text || "(no text produced)"
            : `invoke_agent error: ${result.error.message}`,
        metadata: { ...result },
      }
    },
  })
}

/** Auto-discovered OpenCode plugin exposing the policy-free invoke_agent tool. */
export const InvokeAgentPlugin = {
  id: "invoke-agent",
  server: async (input) => {
    const projectRoot = input.worktree || input.directory
    const dependencies: InvokeAgentDependencies = {
      client: input.client,
      readText: async (relativePath: string): Promise<string> =>
        readFile(resolve(projectRoot, relativePath), "utf8"),
      parseYaml: createBunYamlParser(),
      timeoutMilliseconds: DEFAULT_TIMEOUT_MILLISECONDS,
    }
    return { tool: { invoke_agent: createInvokeAgentTool(dependencies) } }
  },
} satisfies PluginModule

export default InvokeAgentPlugin
