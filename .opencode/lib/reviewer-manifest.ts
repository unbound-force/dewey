import { z } from "zod"

export const CATEGORY_VALUES = [
  "security",
  "cli-ux",
  "test-quality",
  "documentation",
  "ci-cd",
  "dependencies",
  "standard",
] as const

export const AgentNameSchema = z.string().regex(/^divisor-[a-z0-9-]{1,63}$/)
export const CategorySchema = z.enum(CATEGORY_VALUES)

export function uniqueArray<T extends z.ZodType>(item: T, maximum: number): z.ZodArray<T> {
  return z
    .array(item)
    .max(maximum)
    .superRefine((values, context) => {
      if (new Set(values).size !== values.length) {
        context.addIssue({ code: "custom", message: "items must be unique" })
      }
    })
}

export const ReviewerSchema = z
  .object({
    agent: AgentNameSchema,
    capability: z.enum(["review", "content"]),
    scopes: uniqueArray(CategorySchema, 7),
  })
  .strict()
  .superRefine((reviewer, context) => {
    if (reviewer.capability === "review" && reviewer.scopes.length === 0) {
      context.addIssue({ code: "custom", path: ["scopes"], message: "review scopes must not be empty" })
    }
    if (reviewer.capability === "content" && reviewer.scopes.length !== 0) {
      context.addIssue({ code: "custom", path: ["scopes"], message: "content scopes must be empty" })
    }
  })

export const ReviewerManifestSchema = z
  .object({
    version: z.literal(1),
    reviewers: z.array(ReviewerSchema).min(1),
  })
  .strict()

export type ReviewerManifest = z.infer<typeof ReviewerManifestSchema>
export type Reviewer = z.infer<typeof ReviewerSchema>
export type ReviewCategory = z.infer<typeof CategorySchema>

/** Narrow YAML parsing boundary supplied by Bun in production and by tests in isolation. */
export type YamlParser = (text: string) => unknown

const KNOWN_REVIEWERS: Readonly<Record<string, Omit<Reviewer, "agent">>> = {
  "divisor-adversary": {
    capability: "review",
    scopes: ["security", "dependencies", "standard"],
  },
  "divisor-architect": {
    capability: "review",
    scopes: ["standard", "cli-ux", "ci-cd", "documentation"],
  },
  "divisor-curator": { capability: "review", scopes: ["documentation"] },
  "divisor-guard": {
    capability: "review",
    scopes: ["standard", "cli-ux", "documentation"],
  },
  "divisor-sre": { capability: "review", scopes: ["ci-cd", "dependencies", "security"] },
  "divisor-testing": { capability: "review", scopes: ["test-quality"] },
  "divisor-envoy": { capability: "content", scopes: [] },
  "divisor-herald": { capability: "content", scopes: [] },
  "divisor-scribe": { capability: "content", scopes: [] },
}

export function formatValidationError(error: z.ZodError): string {
  return error.issues
    .map((issue) => `${issue.path.join(".") || "<root>"}: ${issue.message}`)
    .sort()
    .join("; ")
}

function stripYamlComment(line: string): string {
  let singleQuoted = false
  let doubleQuoted = false
  for (let index = 0; index < line.length; index += 1) {
    const character = line[index]
    if (character === "'" && !doubleQuoted) {
      singleQuoted = !singleQuoted
    } else if (character === '"' && !singleQuoted && line[index - 1] !== "\\") {
      doubleQuoted = !doubleQuoted
    } else if (character === "#" && !singleQuoted && !doubleQuoted) {
      return line.slice(0, index)
    }
  }
  return line
}

function rejectYamlReferences(text: string, label: string): void {
  for (const line of text.split(/\r?\n/)) {
    const content = stripYamlComment(line)
    if (/^\s*<<\s*:/.test(content) || /(^|[\s:[{,])[&*][A-Za-z0-9_-]+(?=$|[\s,\]}])/.test(content)) {
      throw new Error(`${label} contains forbidden YAML anchor, alias, or merge reference`)
    }
  }
}

export function parseYamlDocument(text: string, label: string, parser: YamlParser): unknown {
  rejectYamlReferences(text, label)
  try {
    return parser(text)
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : "unknown parser error"
    throw new Error(`parse ${label}: ${message}`)
  }
}

function equalStrings(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}

/**
 * Parses and closed-validates a version 1 reviewer manifest, including the canonical scope table.
 * @param text YAML or JSON policy text.
 * @param parser Injected YAML parser.
 * @returns The validated reviewer manifest.
 */
export function parseReviewerManifest(text: string, parser: YamlParser): ReviewerManifest {
  const result = ReviewerManifestSchema.safeParse(parseYamlDocument(text, "reviewer manifest", parser))
  if (!result.success) {
    throw new Error(`validate reviewer manifest: ${formatValidationError(result.error)}`)
  }

  const reviewers = new Map<string, Reviewer>()
  for (const reviewer of result.data.reviewers) {
    const prior = reviewers.get(reviewer.agent)
    if (prior !== undefined) {
      const kind =
        prior.capability === reviewer.capability && equalStrings(prior.scopes, reviewer.scopes)
          ? "duplicate"
          : "conflicting"
      throw new Error(`${kind} reviewer entry for ${reviewer.agent}`)
    }
    reviewers.set(reviewer.agent, reviewer)
  }

  for (const [agent, expected] of Object.entries(KNOWN_REVIEWERS)) {
    const actual = reviewers.get(agent)
    if (actual === undefined) {
      throw new Error(`missing known reviewer entry for ${agent}`)
    }
    if (actual.capability !== expected.capability || !equalStrings(actual.scopes, expected.scopes)) {
      throw new Error(`known reviewer entry for ${agent} conflicts with canonical policy`)
    }
  }
  return result.data
}

/**
 * Creates the narrow Bun YAML parser used by the production plugins.
 * @returns A parser backed by Bun.YAML.parse.
 * @throws When the supported Bun YAML API is unavailable.
 */
export function createBunYamlParser(): YamlParser {
  type BunYamlRuntime = { readonly YAML?: { readonly parse?: (text: string) => unknown } }
  const runtime = globalThis as typeof globalThis & { readonly Bun?: BunYamlRuntime }
  const parse = runtime.Bun?.YAML?.parse
  if (parse === undefined) {
    throw new Error("Bun.YAML.parse is unavailable; reviewer manifest parsing requires Bun 1.3.x or newer")
  }
  return (text: string): unknown => parse(text)
}
