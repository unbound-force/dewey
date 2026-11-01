import { createHash } from "node:crypto"

import { z } from "zod"

import { parseYamlDocument, type YamlParser } from "./reviewer-manifest.js"

// ── Constants ────────────────────────────────────────────────

/** Repository-relative location of the tracked model matrix (design D1, D8). */
export const MODEL_MATRIX_PATH = ".uf/model-matrix.yaml"

/** Repository-relative location of the gitignored per-developer override (design D4, MM-FR-002). */
export const MODEL_MATRIX_OVERRIDE_PATH = ".uf/model-matrix.override.yaml"

/** Repository-relative directory whose `<pack>.md` files back `packs` keys (MM-FR-001). */
export const PACKS_DIRECTORY = ".opencode/uf/packs"

/** Closed task-kind key set the `defaults` mapping accepts (MM-FR-001). */
export const DEFAULT_TASK_KINDS = ["plan", "tasks", "implement"] as const

/** Inferred union of the closed `defaults` task-kind key set. */
export type DefaultTaskKind = (typeof DEFAULT_TASK_KINDS)[number]

/** Maximum number of capability profiles (MM-FR-001: 1-32 entries). */
const MAX_PROFILES = 32

/** Maximum number of agent pins, mirroring the review-matrix agent-map bound. */
const MAX_AGENT_PINS = 32

// ── Selection audit types (design D7, MM-FR-007) ──────────────

/** The eight-value selection provenance enum recorded on every delegation. */
export const SELECTION_SOURCE_VALUES = [
  "explicit-model",
  "explicit-profile",
  "explicit-pack",
  "agent-pin",
  "default-kind",
  "classifier-agent",
  "classifier-llm",
  "host",
] as const

/** Inferred union of the selection provenance enum. */
export type SelectionSource = (typeof SELECTION_SOURCE_VALUES)[number]

/**
 * Resolved model selection for one delegation, consumed by the selection
 * audit trail (MM-FR-007). `model` carries the combined `provider/model`
 * identity string (split into a `ModelIdentity` by the delegation tool);
 * a null `model` with `selection_source: "host"` records host fallback.
 */
export interface SelectionResult {
  /** Which precedence-chain step produced this selection (MM-FR-003). */
  readonly selection_source: SelectionSource
  /**
   * Selected, classified, or explicitly supplied convention pack, when one
   * participated. An explicitly supplied pack that passes validation rides
   * along even when a higher-precedence parameter supplies the model
   * (MM-FR-003: never silently dropped).
   */
  readonly pack: string | null
  /** Resolved capability name, when selection resolved a profile. */
  readonly capability: string | null
  /** Resolved `provider/model` identity, or null for host fallback. */
  readonly model: string | null
  /** Resolved model variant, when the profile or call carried one. */
  readonly variant: string | null
  /** Classifier confidence, when a classifier engine participated. */
  readonly confidence: number | null
  /** Why a fallback occurred (classifier failure, profile without a model). */
  readonly fallback_reason: string | null
}

// ── Closed v1 Zod schema (design D1/D3, MM-FR-001) ────────────
//
// Every object level is `.strict()` so unknown keys fail validation: the
// Zod surface must stay identical to `schemas/model-matrix/v1.schema.json`,
// and the cross-validation harness asserts identical verdicts on fixtures.

const CapabilityNameSchema = z.string().regex(/^[a-z][a-z0-9-]{0,31}$/)
const ProfileModelSchema = z
  .string()
  .min(3)
  .max(256)
  .regex(/^[A-Za-z0-9][A-Za-z0-9._-]*\/[A-Za-z0-9][A-Za-z0-9._@-]*(?:\/[A-Za-z0-9][A-Za-z0-9._@-]*)*$/)
const ProfileVariantSchema = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/)
const PackNameSchema = z.string().regex(/^[a-z][a-z0-9-]{0,63}$/)
// Negative lookahead keeps reviewer agents out of `agents` pins: divisor-*
// selection is governed by the review matrix, not the model matrix.
const AgentPinNameSchema = z.string().regex(/^(?!divisor-)[a-z][a-z0-9-]{0,63}$/)

const ModelMatrixProfileSchema = z
  .object({
    model: ProfileModelSchema.optional(),
    variant: ProfileVariantSchema.optional(),
  })
  .strict()

const ProfilesSchema = z
  .record(CapabilityNameSchema, ModelMatrixProfileSchema)
  .refine((profiles) => Object.keys(profiles).length >= 1, "at least one profile is required")
  .refine((profiles) => Object.keys(profiles).length <= MAX_PROFILES, `at most ${MAX_PROFILES} profiles are allowed`)

const PacksSchema = z.record(PackNameSchema, CapabilityNameSchema)

// Closed task-kind key set: entries are individually optional so a matrix can
// omit static routing, but `plan`, `tasks`, and `implement` are the only keys
// ever accepted (MM-FR-001; unknown kinds fail with the set named).
const DefaultsSchema = z
  .object({
    plan: CapabilityNameSchema.optional(),
    tasks: CapabilityNameSchema.optional(),
    implement: CapabilityNameSchema.optional(),
  })
  .strict()

const AgentsSchema = z
  .record(AgentPinNameSchema, CapabilityNameSchema)
  .refine((pins) => Object.keys(pins).length <= MAX_AGENT_PINS, `at most ${MAX_AGENT_PINS} agent pins are allowed`)

// Classifier structure only (MM-FR-001): the tracked matrix carries no model
// strings — per-developer values under `classifier.llm` arrive exclusively
// via the gitignored override (design D4), and the merged result re-validates
// against this same schema, so the keys are closed while values stay strings.
// The `{env:VAR}`-only apiKey rule is enforced by the LLM engine (MM-FR-006).
const ClassifierLlmSchema = z
  .object({
    endpoint: z.string().min(1).max(2048).optional(),
    model: z.string().min(1).max(256).optional(),
    apiKey: z.string().min(1).max(512).optional(),
    request_timeout_ms: z.number().int().positive().max(180_000).optional(),
  })
  .strict()

const ClassifierSchema = z
  .object({
    enabled: z.boolean().optional(),
    agent: z
      .string()
      .regex(/^[a-z][a-z0-9-]{0,63}$/)
      .optional(),
    agent_timeout_ms: z.number().int().positive().max(1_800_000).optional(),
    llm: ClassifierLlmSchema.optional(),
    confidence_threshold: z.number().min(0).max(1).optional(),
  })
  .strict()

/** Closed Zod schema for the v1 model matrix surface (mirrors v1.schema.json). */
export const ModelMatrixSchema = z
  .object({
    version: z.literal(1),
    profiles: ProfilesSchema,
    packs: PacksSchema.optional(),
    defaults: DefaultsSchema.optional(),
    agents: AgentsSchema.optional(),
    classifier: ClassifierSchema.optional(),
  })
  .strict()
  .superRefine((matrix, context) => {
    // Cross-reference validation: every mapped capability must be an own
    // property of `profiles`. Object.hasOwn (not `in`) so prototype-chain
    // names such as "constructor" can never satisfy a reference.
    for (const [pack, capability] of Object.entries(matrix.packs ?? {})) {
      if (!Object.hasOwn(matrix.profiles, capability)) {
        context.addIssue({ code: "custom", path: ["packs", pack], message: `unknown profile ${capability}` })
      }
    }
    for (const [kind, capability] of Object.entries(matrix.defaults ?? {})) {
      if (!Object.hasOwn(matrix.profiles, capability)) {
        context.addIssue({ code: "custom", path: ["defaults", kind], message: `unknown profile ${capability}` })
      }
    }
    for (const [agent, capability] of Object.entries(matrix.agents ?? {})) {
      if (!Object.hasOwn(matrix.profiles, capability)) {
        context.addIssue({ code: "custom", path: ["agents", agent], message: `unknown profile ${capability}` })
      }
    }
  })

/** Parsed and closed-validated model matrix document. */
export type ModelMatrix = z.infer<typeof ModelMatrixSchema>

/** One capability-named model slot (`model` and `variant` both optional). */
export type ModelMatrixProfile = z.infer<typeof ModelMatrixProfileSchema>

// ── Canonical no-op matrix (MM-FR-004) ────────────────────────

/**
 * Canonical empty matrix for absent-content documents. A present-but-empty
 * or comments-only `.uf/model-matrix.yaml` is a VALID no-op (MM-FR-004), so
 * parsing resolves it to this value and every precedence step falls through
 * to the host model. Frozen because it is shared by reference across loads
 * (convention pack AP-004 immutability).
 */
export const EMPTY_MODEL_MATRIX: ModelMatrix = Object.freeze({
  version: 1,
  profiles: Object.freeze({}),
  packs: Object.freeze({}),
  defaults: Object.freeze({}),
  agents: Object.freeze({}),
  classifier: Object.freeze({}),
} satisfies ModelMatrix)

// ── Parsing ───────────────────────────────────────────────────

/** Extracts a message from an unknown rejection. */
function errorText(error: unknown): string {
  return error instanceof Error ? error.message : "unknown error"
}

/**
 * Formats model-matrix validation issues for the fail-closed error path.
 * Augments `defaults` unrecognized-key failures with the accepted task-kind
 * set: MM-FR-001 requires the failure to name the accepted key set, and
 * zod's default message names only the offending key.
 * @param error The Zod validation error.
 * @returns Sorted, "; "-joined issue descriptions.
 */
function formatModelMatrixIssues(error: z.ZodError): string {
  return error.issues
    .map((issue) => {
      const location = issue.path.join(".") || "<root>"
      if (issue.code === "unrecognized_keys" && issue.path[0] === "defaults") {
        return `${location}: ${issue.message} (accepted task kinds: ${DEFAULT_TASK_KINDS.join(", ")})`
      }
      return `${location}: ${issue.message}`
    })
    .sort()
    .join("; ")
}

/**
 * Parses and closed-validates a model matrix document. Empty and
 * comments-only documents (YAML null) resolve to the canonical no-op matrix
 * per MM-FR-004; any other document that fails the closed v1 surface throws.
 * @param text YAML or JSON matrix text.
 * @param parser Injected YAML parser.
 * @returns The validated matrix, or the frozen no-op matrix for empty input.
 * @throws When the document parses to a non-null value that fails validation.
 */
export function parseModelMatrix(text: string, parser: YamlParser): ModelMatrix {
  const document = parseYamlDocument(text, "model matrix", parser)
  if (document === null || document === undefined) {
    return EMPTY_MODEL_MATRIX
  }
  const result = ModelMatrixSchema.safeParse(document)
  if (!result.success) {
    throw new Error(`validate model matrix: ${formatModelMatrixIssues(result.error)}`)
  }
  return result.data
}

// ── Override (design D4, MM-FR-002) ──────────────────────────

/**
 * The fail-closed override whitelist (MM-FR-002). Every override validation
 * error names this set so a misconfigured override states its own remedy.
 */
const OVERRIDE_WHITELIST = "profiles.*.model, profiles.*.variant, classifier.llm.*"

/** Narrows a parsed YAML value to a non-array object (the override walk shape). */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/**
 * Validates a parsed override document against the fail-closed whitelist
 * `profiles.*.model`, `profiles.*.variant`, `classifier.llm.*` (design D4,
 * MM-FR-002). Only per-developer identity paths are permitted: structural
 * keys (`packs`, `defaults`, `agents`, `classifier.enabled`, `classifier.agent`,
 * ...) are tracked-matrix surface, so accepting them would let a gitignored
 * file silently change governance. Any nested key under `classifier.llm` is
 * admitted here — that subtree is exclusively per-developer — while the
 * closed schema still bounds the accepted keys at merged-result
 * re-validation, a deliberate two-layer defense.
 * @param override Parsed override document (null/undefined for empty files).
 * @throws When any key or nested path is not whitelisted.
 */
export function validateModelMatrixOverride(override: unknown): void {
  // Empty and comments-only override documents parse to null/undefined; an
  // object without keys carries no paths. All three are valid no-ops.
  if (override === null || override === undefined) {
    return
  }
  if (!isRecord(override)) {
    throw new Error(`model matrix override must be an object — only ${OVERRIDE_WHITELIST} paths are allowed`)
  }
  if (Object.keys(override).length === 0) {
    return
  }
  for (const key of Object.keys(override)) {
    if (key !== "profiles" && key !== "classifier") {
      throw new Error(
        `model matrix override key "${key}" is not permitted — only ${OVERRIDE_WHITELIST} paths are allowed`,
      )
    }
  }
  validateOverrideProfiles(override.profiles)
  validateOverrideClassifier(override.classifier)
}

/**
 * Walks the override's `profiles` subtree: only `model` and `variant` leaves
 * are whitelisted per capability (MM-FR-002).
 * @param profiles The parsed `profiles` value (null/undefined when absent).
 * @throws When a profile entry or leaf key is not whitelisted.
 */
function validateOverrideProfiles(profiles: unknown): void {
  if (profiles === null || profiles === undefined) {
    return // absent profiles is a no-op
  }
  if (!isRecord(profiles)) {
    throw new Error(`model matrix override profiles must be an object — only ${OVERRIDE_WHITELIST} paths are allowed`)
  }
  for (const capability of Object.keys(profiles)) {
    const profile = profiles[capability]
    if (profile === null || profile === undefined) {
      continue // an empty capability entry is a no-op
    }
    if (!isRecord(profile)) {
      throw new Error(`model matrix override profiles.${capability} must be an object`)
    }
    for (const key of Object.keys(profile)) {
      if (key !== "model" && key !== "variant") {
        throw new Error(
          `model matrix override key "profiles.${capability}.${key}" is not permitted — only ${OVERRIDE_WHITELIST} paths are allowed`,
        )
      }
    }
  }
}

/**
 * Walks the override's `classifier` subtree: everything under `classifier`
 * is structural and tracked except `llm`, whose nested keys are exclusively
 * per-developer (design D4), so any key under `classifier.llm` is admitted
 * here and the closed schema bounds the accepted set at merged-result
 * re-validation.
 * @param classifier The parsed `classifier` value (null/undefined when absent).
 * @throws When a classifier key outside `llm` is present.
 */
function validateOverrideClassifier(classifier: unknown): void {
  if (classifier === null || classifier === undefined) {
    return // absent classifier is a no-op
  }
  if (!isRecord(classifier)) {
    throw new Error(`model matrix override classifier must be an object — only ${OVERRIDE_WHITELIST} paths are allowed`)
  }
  for (const key of Object.keys(classifier)) {
    if (key !== "llm") {
      throw new Error(
        `model matrix override key "classifier.${key}" is not permitted — only ${OVERRIDE_WHITELIST} paths are allowed`,
      )
    }
  }
  const llm = classifier.llm
  if (llm === null || llm === undefined) {
    return // absent llm is a no-op
  }
  if (!isRecord(llm)) {
    throw new Error("model matrix override classifier.llm must be an object")
  }
}

/**
 * Performs the nested-path merge of a whitelisted override into the base
 * matrix (design D4, MM-FR-002): each override leaf replaces its base
 * counterpart, absent paths fall through to the base, and an explicit
 * `null` model or variant becomes an absent key — host fallback for models.
 * The `profiles` map is never wholesale-replaced, and only capabilities
 * already defined in the base may be overridden: adding capabilities is a
 * tracked-matrix change, not a per-developer one. The merged result is
 * re-validated against the identical schema via a JSON round-trip, which
 * also returns a sanitized clone with plain prototypes. Callers MUST run
 * {@link validateModelMatrixOverride} first so non-whitelisted keys fail
 * closed instead of being silently skipped by the walk.
 * @param base The validated base matrix.
 * @param override The parsed, whitelist-validated override document.
 * @returns The merged, re-validated matrix, or `base` unchanged when the
 *   override carries no whitelisted entries.
 * @throws When the override names a capability absent from the base, or the
 *   merged result fails the closed schema.
 */
export function mergeOverride(base: ModelMatrix, override: Record<string, unknown>): ModelMatrix {
  const profileOverrides = isRecord(override.profiles) ? override.profiles : null
  const classifierOverride = isRecord(override.classifier) ? override.classifier : null
  const llmOverrides = classifierOverride !== null && isRecord(classifierOverride.llm) ? classifierOverride.llm : null

  const hasProfileEntries = profileOverrides !== null && Object.keys(profileOverrides).length > 0
  const hasLlmEntries = llmOverrides !== null && Object.keys(llmOverrides).length > 0
  if (!hasProfileEntries && !hasLlmEntries) {
    return base // no whitelisted entries — a no-op (MM-FR-002)
  }

  const merged: Record<string, unknown> = { ...base }

  if (profileOverrides !== null) {
    const mergedProfiles: Record<string, unknown> = Object.create(null)
    for (const capability of Object.keys(base.profiles)) {
      mergedProfiles[capability] = { ...base.profiles[capability] }
    }
    for (const capability of Object.keys(profileOverrides)) {
      const profileOverride = profileOverrides[capability]
      if (!isRecord(profileOverride)) {
        continue // null and non-object entries carry no leaves (validated upstream)
      }
      // Own-property check so prototype-chain names such as "constructor" can
      // never satisfy the capability reference (the loadPolicies pattern).
      if (!Object.hasOwn(base.profiles, capability)) {
        throw new Error(
          `model matrix override profiles.${capability} is not a defined capability in the base matrix — only existing capabilities may be overridden`,
        )
      }
      const mergedProfile: Record<string, unknown> = { ...base.profiles[capability] }
      if ("model" in profileOverride) {
        // Null propagates host fallback: optional Zod fields accept undefined,
        // never null (Bun.YAML null-for-empty semantics).
        mergedProfile.model = profileOverride.model ?? undefined
      }
      if ("variant" in profileOverride) {
        mergedProfile.variant = profileOverride.variant ?? undefined
      }
      mergedProfiles[capability] = mergedProfile
    }
    merged.profiles = mergedProfiles
  }

  if (llmOverrides !== null) {
    const baseClassifier = isRecord(base.classifier) ? base.classifier : {}
    // Explicit annotation: isRecord's guard intersects with the zod-inferred
    // llm leaf type, whose specific keys would otherwise reject the string
    // indexing below under typecheck (the merge walks it as an open record).
    const baseLlm: Record<string, unknown> = isRecord(baseClassifier.llm) ? baseClassifier.llm : {}
    const mergedLlm: Record<string, unknown> = Object.create(null)
    for (const key of Object.keys(baseLlm)) {
      mergedLlm[key] = baseLlm[key]
    }
    for (const key of Object.keys(llmOverrides)) {
      mergedLlm[key] = llmOverrides[key] ?? undefined
    }
    merged.classifier = { ...baseClassifier, llm: mergedLlm }
  }

  // Re-validate the merged result against the identical schema (MM-FR-002).
  // The JSON round-trip sanitizes the clone — null-prototype records become
  // plain objects and undefined-valued keys drop — before the closed surface
  // re-validates, so override typos fail closed at load.
  return parseModelMatrix(JSON.stringify(merged), (text: string) => JSON.parse(text) as unknown)
}

// ── Loader ────────────────────────────────────────────────────

/** Type guard for ENOENT filesystem errors (the loadPolicies convention). */
function isEnoent(error: unknown): boolean {
  return error instanceof Error && "code" in error && (error as NodeJS.ErrnoException).code === "ENOENT"
}

/**
 * Collects `packs` keys whose pack file is missing under `PACKS_DIRECTORY`.
 * @param matrix The validated matrix.
 * @param fileExists Injected existence checker for project-relative paths.
 * @returns Sorted names of packs without a `<pack>.md` file.
 */
function missingPackFiles(matrix: ModelMatrix, fileExists: (relativePath: string) => boolean): readonly string[] {
  const missing: string[] = []
  for (const pack of Object.keys(matrix.packs ?? {})) {
    if (!fileExists(`${PACKS_DIRECTORY}/${pack}.md`)) {
      missing.push(pack)
    }
  }
  return missing.sort()
}

/** `classifier.llm` leaves the tracked-role population rule governs (MM-FR-001). */
const TRACKED_LLM_KEYS = ["endpoint", "model", "apiKey", "request_timeout_ms"] as const

/**
 * Collects populated `classifier.llm` leaves of a parsed matrix document for
 * the tracked-role population rule (MM-FR-001): the tracked matrix carries
 * `classifier.llm` structure only, so any populated value in the tracked
 * file fails load with the override file named as the remediation. The
 * check is role-aware by necessity — the identical schema accepts populated
 * values in merged views (override values legitimately populate those
 * paths, MM-FR-002), so it runs only against the pre-merge tracked
 * document, never against a merged result.
 * @param matrix The parsed tracked-file matrix, before any override merge.
 * @returns Sorted names of populated `classifier.llm` leaves.
 */
function populatedTrackedLlmLeaves(matrix: ModelMatrix): readonly string[] {
  const llm = matrix.classifier?.llm
  if (llm === undefined) {
    return []
  }
  return TRACKED_LLM_KEYS.filter((key) => llm[key] !== undefined).sort()
}

/** Injectable dependencies for loading the model matrix (the loadPolicies pattern). */
export interface ModelMatrixDependencies {
  /** Reads a project-relative file; absence MUST reject with an ENOENT-coded error. */
  readonly readText: (relativePath: string) => Promise<string>
  /** Narrow YAML parsing boundary supplied by Bun in production, tests in isolation. */
  readonly parseYaml: YamlParser
  /** Reports whether a project-relative path exists (pack existence, MM-FR-001). */
  readonly fileExists: (relativePath: string) => boolean
}

/** Discriminated load outcome for the tracked matrix (design D8 zero-config probe). */
export type ModelMatrixLoadResult =
  | { readonly status: "absent" }
  | { readonly status: "ok"; readonly matrix: ModelMatrix }
  | { readonly status: "invalid"; readonly error: string }

/**
 * Loads the tracked model matrix and applies the gitignored per-developer
 * override when one is present (design D4). An absent matrix resolves to the
 * zero-config `absent` state (host model, no override read, MM-FR-004); a
 * present matrix that fails the closed schema, carries populated
 * `classifier.llm` values in the tracked file (the tracked-role population
 * rule, MM-FR-001), names packs without pack
 * files, or pairs with an override that violates the whitelist or fails
 * merged-result re-validation resolves to `invalid` with an error naming the
 * path, accepted key set, whitelist, or offending pack keys (fail-closed).
 * Never throws: every failure is carried in the result so the delegation
 * tool has one uniform failure path.
 * @param dependencies Injected read, parse, and existence seams.
 * @returns The discriminated tristate load result.
 */
export async function loadModelMatrix(dependencies: ModelMatrixDependencies): Promise<ModelMatrixLoadResult> {
  let text: string
  try {
    text = await dependencies.readText(MODEL_MATRIX_PATH)
  } catch (error: unknown) {
    if (isEnoent(error)) {
      return { status: "absent" }
    }
    return { status: "invalid", error: `cannot read model matrix ${MODEL_MATRIX_PATH}: ${errorText(error)}` }
  }

  let matrix: ModelMatrix
  try {
    matrix = parseModelMatrix(text, dependencies.parseYaml)
  } catch (error: unknown) {
    return { status: "invalid", error: `model matrix ${MODEL_MATRIX_PATH} is invalid: ${errorText(error)}` }
  }

  // Tracked-role population rule (MM-FR-001, task 2.4): the tracked file
  // carries `classifier.llm` structure only. The check runs against the
  // tracked document BEFORE the override merge — merged views legitimately
  // populate those paths (MM-FR-002) — and separately from Zod schema
  // validation, which accepts populated values by design so the merged
  // result can re-validate against the identical schema. The no-op matrix
  // (comments-only file) resolves to the canonical empty classifier and
  // stays valid (MM-FR-004).
  const populatedLlm = populatedTrackedLlmLeaves(matrix)
  if (populatedLlm.length > 0) {
    return {
      status: "invalid",
      error:
        `model matrix ${MODEL_MATRIX_PATH} is invalid: tracked classifier.llm carries populated values ` +
        `(${populatedLlm.join(", ")}) — per-developer engine configuration belongs in ` +
        `${MODEL_MATRIX_OVERRIDE_PATH}; keep the tracked file structure only`,
    }
  }

  const missing = missingPackFiles(matrix, dependencies.fileExists)
  if (missing.length > 0) {
    const details = missing.map((pack) => `${PACKS_DIRECTORY}/${pack}.md`).join(", ")
    return {
      status: "invalid",
      error: `model matrix ${MODEL_MATRIX_PATH} is invalid: packs key(s) ${missing.join(", ")} have no pack file (${details})`,
    }
  }

  // Per-developer override (design D4): an absent override keeps the base; a
  // present one must pass the whitelist, merge by nested path, and survive
  // re-validation against the identical schema. The override can never carry
  // `packs` keys, so the pack-existence check above stays valid for the merged
  // result. Structural override failures fail closed exactly like base-matrix
  // failures, and a whitelist violation throws before any merge runs, so no
  // merged configuration is produced (MM-FR-002).
  let overrideText: string | null = null
  try {
    overrideText = await dependencies.readText(MODEL_MATRIX_OVERRIDE_PATH)
  } catch (error: unknown) {
    if (!isEnoent(error)) {
      return {
        status: "invalid",
        error: `cannot read model matrix override ${MODEL_MATRIX_OVERRIDE_PATH}: ${errorText(error)}`,
      }
    }
    // Override file does not exist — the base matrix is used as-is.
  }

  if (overrideText !== null) {
    try {
      const document = parseYamlDocument(overrideText, "model matrix override", dependencies.parseYaml)
      if (document !== null && document !== undefined) {
        validateModelMatrixOverride(document)
        matrix = mergeOverride(matrix, document as Record<string, unknown>)
      }
    } catch (error: unknown) {
      return {
        status: "invalid",
        error: `model matrix override ${MODEL_MATRIX_OVERRIDE_PATH} is invalid: ${errorText(error)}`,
      }
    }
  }

  return { status: "ok", matrix }
}

// ── Selection precedence (design D2, MM-FR-003/MM-FR-004) ──────

/** Default classifier confidence threshold when the matrix omits one (design D5). */
export const DEFAULT_CONFIDENCE_THRESHOLD = 0.6

/** Which classifier engine produced a winning cascade result (MM-FR-005 order). */
export type ClassifierEngine = "agent" | "llm"

/**
 * Pre-computed classifier cascade outcome supplied to {@link resolveSelection}.
 * Selection is a pure function (Constitution IV, design D5), so the cascade's
 * engines — the hidden-subagent spawn and the LLM fetch — run before
 * resolution and participate only as this value: the cascade (task 3.2)
 * supplies the winning engine, pack, and confidence, or a not-run/failed
 * state, and {@link resolveSelection} applies matrix-validity and threshold
 * gating to whatever it receives.
 */
export type ClassifierOutcome =
  /** The cascade did not run (static task kind, classifier disabled, or zero-config). */
  | { readonly status: "not-run"; readonly reason?: string }
  /** Every engine failed; `reason` names the engines attempted and why each failed. */
  | { readonly status: "failed"; readonly reason: string }
  /** A winning result: which engine produced it, its pack, and its confidence. */
  | {
      readonly status: "selected"
      readonly engine: ClassifierEngine
      readonly pack: string
      readonly confidence: number
    }

/**
 * Selection parameters as supplied by one delegation call (design D2).
 * Parameter shapes are validated at the tool boundary (the delegation
 * schema's model, variant, profile, and pack parameters); this module treats
 * any non-undefined value as present.
 */
export interface SelectionInputs {
  /** Explicit `model` parameter — a `provider/model` identity that wins outright (step 1). */
  readonly model?: string
  /** Explicit `variant` parameter, paired with `model` (step 1). */
  readonly variant?: string
  /** Explicit `profile` parameter naming a matrix capability (step 2). */
  readonly profile?: string
  /** Explicit `pack` parameter naming a `packs` key (step 3). */
  readonly pack?: string
  /** Delegation `checkpoint_step` task kind: `plan`, `tasks`, `implement`, or another/absent value. */
  readonly checkpoint_step?: string
  /** Delegated agent name, consulted for the `agents:` pin (step 4). */
  readonly agent?: string
}

/** Stable failure codes for non-retryable selection validation failures (design D2). */
export const SELECTION_FAILURE_CODES = ["MATRIX_NOT_CONFIGURED", "UNKNOWN_PROFILE", "INVALID_PACK"] as const

/** Inferred union of the selection failure codes. */
export type SelectionFailureCode = (typeof SELECTION_FAILURE_CODES)[number]

/**
 * Discriminated resolution outcome for one delegation's model selection: the
 * resolved {@link SelectionResult} on success, or a non-retryable failure
 * carrying a stable code and a message that names the offending value and its
 * remediation — the `SKILL_NOT_FOUND` envelope pattern. Validation failures
 * never resolve on retry, so the delegation tool fails the call pre-spawn
 * with zero child sessions (design D2).
 */
export type SelectionResolution =
  | { readonly ok: true; readonly selection: SelectionResult }
  | {
      readonly ok: false
      readonly code: SelectionFailureCode
      readonly error: string
      readonly retryable: false
    }

/** Attempt outcome for a selected classifier result (internal to step 6). */
type ClassifierAttempt =
  | { readonly resolved: true; readonly result: SelectionResolution }
  | { readonly resolved: false; readonly fallbackReason: string }

/**
 * Builds a host-fallback selection (MM-FR-003 step 7): no matrix model
 * resolved, so the delegation tool introspects the parent session. The pack
 * and capability stay recorded when a precedence step resolved them, keeping
 * the fallback auditable (MM-FR-007).
 * @param pack Selected or classified pack, when one participated.
 * @param capability Resolved capability, when a step resolved one.
 * @param fallbackReason Why host fallback occurred, when applicable.
 * @param confidence Classifier confidence, when an accepted classifier
 *   result resolved a model-less profile.
 * @returns The host-fallback selection resolution.
 */
function hostSelection(
  pack: string | null,
  capability: string | null,
  fallbackReason: string | null,
  confidence: number | null = null,
): SelectionResolution {
  return {
    ok: true,
    selection: {
      selection_source: "host",
      pack,
      capability,
      model: null,
      variant: null,
      confidence,
      fallback_reason: fallbackReason,
    },
  }
}

/**
 * Resolves a capability through its profile (MM-FR-003: steps 2-6 resolve a
 * capability first, then its profile). A profile carrying a model yields the
 * given `source`; a model-less profile falls to host with the capability and
 * pack still recorded for the audit trail. Closed-schema validation
 * guarantees every mapped capability is an own property of `profiles`, so
 * the lookup cannot miss.
 * @param matrix The configured (non-empty) matrix.
 * @param capability The capability a precedence step resolved.
 * @param source The selection source recorded when the profile carries a model.
 * @param pack The pack that produced the capability, when one participated.
 * @param fallbackReason Prior fallback context (classifier failure) to
 *   record alongside the profile outcome.
 * @returns The resolved or host-fallback selection.
 */
function capabilitySelection(
  matrix: ModelMatrix,
  capability: string,
  source: SelectionSource,
  pack: string | null,
  fallbackReason: string | null,
): SelectionResolution {
  const profile = matrix.profiles[capability]
  if (profile.model !== undefined) {
    return {
      ok: true,
      selection: {
        selection_source: source,
        pack,
        capability,
        model: profile.model,
        variant: profile.variant ?? null,
        confidence: null,
        fallback_reason: fallbackReason,
      },
    }
  }
  const noModelReason = `profile "${capability}" carries no model`
  return hostSelection(
    pack,
    capability,
    fallbackReason !== null ? `${fallbackReason}; ${noModelReason}` : noModelReason,
  )
}

/**
 * Applies classifier gating to a selected cascade outcome (MM-FR-005): the
 * pack must be a `packs` key of the configured matrix, and the confidence
 * must meet the configured threshold (default 0.6). An accepted result
 * resolves through the pack's profile — the same double duty as an explicit
 * pack parameter; a discarded result reports the fallback reason for the
 * static-default path so the fallback stays auditable (MM-FR-007).
 * @param matrix The configured matrix supplying `packs` and the threshold.
 * @param outcome The winning cascade outcome.
 * @returns The resolved selection, or the fallback reason when discarded.
 */
function attemptClassifierSelection(
  matrix: ModelMatrix,
  outcome: Extract<ClassifierOutcome, { status: "selected" }>,
): ClassifierAttempt {
  const packs = matrix.packs ?? {}
  if (!Object.hasOwn(packs, outcome.pack)) {
    return {
      resolved: false,
      fallbackReason: `classifier pack "${outcome.pack}" is not a packs key in the configured matrix`,
    }
  }
  const threshold = matrix.classifier?.confidence_threshold ?? DEFAULT_CONFIDENCE_THRESHOLD
  if (outcome.confidence < threshold) {
    return {
      resolved: false,
      fallbackReason: `classifier confidence ${outcome.confidence} is below the threshold ${threshold}`,
    }
  }
  const capability = packs[outcome.pack]
  const profile = matrix.profiles[capability]
  if (profile.model !== undefined) {
    return {
      resolved: true,
      result: {
        ok: true,
        selection: {
          selection_source: outcome.engine === "agent" ? "classifier-agent" : "classifier-llm",
          pack: outcome.pack,
          capability,
          model: profile.model,
          variant: profile.variant ?? null,
          confidence: outcome.confidence,
          fallback_reason: null,
        },
      },
    }
  }
  return {
    resolved: true,
    result: hostSelection(outcome.pack, capability, `profile "${capability}" carries no model`, outcome.confidence),
  }
}

/**
 * Renders the auditable fallback reason for a classifier outcome that did
 * not yield a selection (MM-FR-007: fallbacks are recorded, never silent).
 * @param outcome The not-run or failed cascade outcome.
 * @returns The fallback reason recorded with the static fallback.
 */
function classifierFallbackReason(outcome: ClassifierOutcome): string {
  if (outcome.status === "failed") {
    return `classifier cascade failed: ${outcome.reason}`
  }
  if (outcome.status === "not-run" && outcome.reason !== undefined) {
    return `classifier not run: ${outcome.reason}`
  }
  return "classifier not run"
}

/**
 * Builds a non-retryable selection failure (design D2). The message names
 * the offending value and its remediation — the `SKILL_NOT_FOUND` envelope
 * pattern — so the delegation tool can fail the call pre-spawn with zero
 * child sessions.
 * @param code Stable failure code.
 * @param error Message naming the failure and its remediation.
 * @returns The discriminated failure resolution.
 */
function selectionFailure(code: SelectionFailureCode, error: string): SelectionResolution {
  return { ok: false, code, error, retryable: false }
}

/**
 * Renders a sorted, comma-joined name list for failure remediation text.
 * @param names Capability or pack names to list.
 * @returns The joined list, or "(none)" when empty.
 */
function nameList(names: readonly string[]): string {
  return names.length > 0 ? [...names].sort().join(", ") : "(none)"
}

/**
 * Resolves one delegation's model selection through the fixed precedence
 * chain (design D2, MM-FR-003), stopping at the first step that yields:
 *
 * 1. an explicit `model` (with optional `variant`) — it needs no matrix;
 * 2. an explicit `profile` naming a declared capability;
 * 3. an explicit `pack` that is a `packs` key of the configured matrix;
 * 4. an `agents:` pin for the delegated agent;
 * 5. static `defaults:` by task kind — `plan`/`tasks` resolve statically
 *    and never reach the classifier;
 * 6. the classifier outcome for `implement` and unknown kinds, falling back
 *    to the static `defaults` entry for the kind, else host;
 * 7. host — a resolved profile without a model, or no matrix configured.
 *
 * Steps 2-6 resolve a capability first, then its profile; a resolved profile
 * without a model falls to host (step 7) rather than continuing down the
 * chain. Validation of explicit selection parameters is unconditional, not
 * reach-based: `profile` and `pack` are validated in full, in chain order,
 * before the chain runs, so a winning higher-precedence parameter never
 * shadows an invalid or unknown companion. An explicitly supplied `pack`
 * that passes validation is never silently dropped — it rides along for
 * directive rendering even when `model` or `profile` supplies the child
 * model — and an explicit `model`, `profile`, or `pack` bypasses the
 * classifier cascade entirely (the cascade participates only in chains
 * that reach step 6). The function is pure — no
 * filesystem, clock, network, or classifier invocation; the classifier
 * participates only as the pre-computed {@link ClassifierOutcome}. An
 * unknown explicit profile, an explicit pack that is not a `packs` key, or
 * an explicit `profile`/`pack` with no matrix configured fails closed with
 * a non-retryable error naming the remediation (MM-FR-003/MM-FR-004).
 * @param matrix The loaded matrix, or null when none is configured. The
 *   canonical no-op matrix (comments-only file) counts as not configured.
 * @param inputs The delegation call's selection parameters.
 * @param classifier The pre-computed classifier cascade outcome.
 * @returns The discriminated selection resolution.
 */
export function resolveSelection(
  matrix: ModelMatrix | null,
  inputs: SelectionInputs,
  classifier: ClassifierOutcome,
): SelectionResolution {
  // Normalize the not-configured state (MM-FR-004): an absent matrix (null)
  // and the canonical no-op matrix (comments-only file) behave identically,
  // and the closed schema requires at least one profile for any other
  // matrix — so an empty `profiles` map exactly identifies "no matrix is
  // configured" regardless of which shape the caller passed.
  const configured = matrix !== null && Object.keys(matrix.profiles).length > 0 ? matrix : null

  // Unconditional validation (MM-FR-003): explicit profile and pack
  // parameters are validated in full, in chain order, before the chain
  // runs — a winning higher-precedence parameter never shadows an invalid
  // or unknown companion (model + unknown profile, model + invalid pack,
  // and profile + invalid pack all fail closed pre-spawn, zero child
  // sessions).
  if (inputs.profile !== undefined) {
    if (configured === null) {
      return selectionFailure(
        "MATRIX_NOT_CONFIGURED",
        `profile "${inputs.profile}" cannot be used: no model matrix is configured at ${MODEL_MATRIX_PATH}. ` +
          `Remediation: declare the profile under profiles in ${MODEL_MATRIX_PATH}, or omit the profile parameter; ` +
          "retrying will not fix this.",
      )
    }
    if (!Object.hasOwn(configured.profiles, inputs.profile)) {
      return selectionFailure(
        "UNKNOWN_PROFILE",
        `profile "${inputs.profile}" is not a capability declared in the model matrix. ` +
          `Configured capabilities: ${nameList(Object.keys(configured.profiles))}. ` +
          "Remediation: pass a declared capability name, or omit the profile parameter; retrying will not fix this.",
      )
    }
  }
  if (inputs.pack !== undefined) {
    if (configured === null) {
      return selectionFailure(
        "MATRIX_NOT_CONFIGURED",
        `pack "${inputs.pack}" cannot be used: no model matrix is configured at ${MODEL_MATRIX_PATH}. ` +
          `Remediation: map the pack under packs in ${MODEL_MATRIX_PATH}, or omit the pack parameter; ` +
          "retrying will not fix this.",
      )
    }
    const packs = configured.packs ?? {}
    if (!Object.hasOwn(packs, inputs.pack)) {
      return selectionFailure(
        "INVALID_PACK",
        `pack "${inputs.pack}" is not a packs key in the model matrix. ` +
          `Configured packs: ${nameList(Object.keys(packs))}. ` +
          `Remediation: pass a pack mapped under packs in ${MODEL_MATRIX_PATH}, or omit the pack parameter; ` +
          "retrying will not fix this.",
      )
    }
  }

  // Step 1: an explicit model wins outright and needs no matrix (MM-FR-004).
  // An explicitly supplied, validated pack rides along for directive
  // rendering — never silently dropped (MM-FR-003, UF-FR-002).
  if (inputs.model !== undefined) {
    return {
      ok: true,
      selection: {
        selection_source: "explicit-model",
        pack: inputs.pack ?? null,
        capability: null,
        model: inputs.model,
        variant: inputs.variant ?? null,
        confidence: null,
        fallback_reason: null,
      },
    }
  }

  // Step 2: an explicit profile (validated above) resolves through its
  // profile; an explicitly supplied pack rides along.
  if (inputs.profile !== undefined && configured !== null) {
    return capabilitySelection(configured, inputs.profile, "explicit-profile", inputs.pack ?? null, null)
  }

  // Step 3: an explicit pack (validated above) resolves through its mapped
  // profile. Pack-file existence was already enforced per key at load time
  // (the loader's missingPackFiles check, MM-FR-001), so validation reduced
  // to the key-membership test above.
  if (inputs.pack !== undefined && configured !== null) {
    const packs = configured.packs ?? {}
    return capabilitySelection(configured, packs[inputs.pack], "explicit-pack", inputs.pack, null)
  }

  // Step 4: an `agents:` pin for the delegated agent.
  if (configured !== null && inputs.agent !== undefined) {
    const pins = configured.agents ?? {}
    if (Object.hasOwn(pins, inputs.agent)) {
      return capabilitySelection(configured, pins[inputs.agent], "agent-pin", null, null)
    }
  }

  const kind = inputs.checkpoint_step

  // Step 5: `plan`/`tasks` resolve statically from `defaults` and never
  // reach the classifier (MM-FR-003 scenario: classifier not invoked).
  if (kind === "plan" || kind === "tasks") {
    if (configured !== null) {
      const capability = configured.defaults?.[kind]
      if (capability !== undefined) {
        return capabilitySelection(configured, capability, "default-kind", null, null)
      }
    }
    return hostSelection(null, null, null)
  }

  // Step 6: the classifier cascade for `implement` and unknown kinds
  // (MM-FR-005). With no matrix there is no static default and no classifier
  // participation — plain host, zero-config semantics (MM-FR-004).
  if (configured === null) {
    return hostSelection(null, null, null)
  }
  const attempt: ClassifierAttempt =
    classifier.status === "selected"
      ? attemptClassifierSelection(configured, classifier)
      : { resolved: false, fallbackReason: classifierFallbackReason(classifier) }
  if (attempt.resolved) {
    return attempt.result
  }
  // Static fallback: the `defaults` entry for the kind when present (only
  // `implement` can have one — unknown kinds are outside the closed key set).
  if (kind === "implement") {
    const staticCapability = configured.defaults?.implement
    if (staticCapability !== undefined) {
      return capabilitySelection(configured, staticCapability, "default-kind", null, attempt.fallbackReason)
    }
  }
  return hostSelection(null, null, `${attempt.fallbackReason}; no static default for task kind "${kind ?? "unknown"}"`)
}

// ── Classifier cascade (design D5, MM-FR-005/MM-FR-006) ────────

/** Default hidden-subagent name for the agent engine when `classifier.agent` is absent (MM-FR-005). */
export const DEFAULT_CLASSIFIER_AGENT = "model-matrix-classifier"

/** Default agent-engine bound in milliseconds when `classifier.agent_timeout_ms` is absent (MM-FR-005). */
export const DEFAULT_AGENT_TIMEOUT_MS = 45_000

/** Default LLM-engine request bound in milliseconds when `classifier.llm.request_timeout_ms` is absent (MM-FR-006). */
export const DEFAULT_LLM_REQUEST_TIMEOUT_MS = 10_000

/** Maximum classifier response body in bytes: 50 MiB (MM-FR-006 unbounded-body guard). */
export const MAX_LLM_RESPONSE_BYTES = 52_428_800

/**
 * Bounded result-cache capacity. A named constant, deliberately not
 * matrix-configurable in v1 (design D5, mirroring the derive-per-run-timeout
 * stance on keeping the new config surface minimal).
 */
export const CLASSIFIER_CACHE_CAPACITY = 128

/** Loopback hosts an `http` classifier endpoint may target (design D4, MM-FR-006). */
const LOOPBACK_HOSTS: ReadonlySet<string> = new Set(["localhost", "127.0.0.1", "[::1]"])

/** The `{env:VAR}`-only apiKey reference form (MM-FR-006: never a literal key). */
const ENV_API_KEY_PATTERN = /^\{env:([A-Za-z_][A-Za-z0-9_]*)\}$/

/** A parsed classifier engine response — the agent contract's JSON object. */
interface ClassifierResponse {
  readonly pack: string
  readonly confidence: number
  readonly reasoning?: string
}

/** One engine's cascade attempt: a gated selection or the recorded failure reason. */
type EngineAttempt =
  | {
      readonly selected: true
      readonly engine: ClassifierEngine
      readonly pack: string
      readonly confidence: number
    }
  | { readonly selected: false; readonly reason: string }

/** One LLM-engine HTTP attempt outcome: the bounded body text or the failure reason. */
type LlmHttpResponse =
  | { readonly ok: true; readonly content: string }
  | { readonly ok: false; readonly status: number; readonly reason: string }

/** The resolved apiKey state: an interpolated value or the recorded skip reason. */
type InterpolatedApiKey =
  | { readonly status: "ok"; readonly value: string }
  | { readonly status: "skipped"; readonly reason: string }

/** Whether the cascade runs for one delegation, carrying the configured matrix when it does. */
type ClassifierRunState =
  | { readonly run: false; readonly reason: string }
  | { readonly run: true; readonly matrix: ModelMatrix }

/** A cached winning classification — the `selected` outcome payload (design D5). */
export interface ClassifierCacheEntry {
  readonly engine: ClassifierEngine
  readonly pack: string
  readonly confidence: number
}

/**
 * Injectable in-process classifier result cache (design D5). One instance
 * belongs to exactly one cascade and is never module-level state, so
 * concurrent sessions and tests share no mutable cache (Constitution IV;
 * TC-002/TC-004 isolation). Only winning selections are cached: a failed
 * cascade is not a result, and pinning a transient engine failure would
 * prevent the next delegation from retrying classification.
 */
export interface ClassifierCache {
  get(key: string): ClassifierCacheEntry | undefined
  set(key: string, entry: ClassifierCacheEntry): void
}

/**
 * Creates a bounded FIFO result cache. Insertion order (Map semantics)
 * decides eviction: when the cache is at capacity, the oldest entry is
 * dropped to admit the new one. Re-setting an existing key updates its
 * value without refreshing its eviction position.
 * @param capacity Maximum number of entries (defaults to {@link CLASSIFIER_CACHE_CAPACITY}).
 * @returns The bounded cache.
 */
export function createClassifierCache(capacity: number = CLASSIFIER_CACHE_CAPACITY): ClassifierCache {
  const entries = new Map<string, ClassifierCacheEntry>()
  return {
    get: (key: string): ClassifierCacheEntry | undefined => entries.get(key),
    set: (key: string, entry: ClassifierCacheEntry): void => {
      if (!entries.has(key) && entries.size >= capacity) {
        const oldest = entries.keys().next().value
        if (oldest !== undefined) {
          entries.delete(oldest)
        }
      }
      entries.set(key, entry)
    },
  }
}

/**
 * Computes the classifier cache key: the SHA-256 of the feature dir,
 * prompt, and delegated agent name (MM-FR-005). The JSON-array
 * serialization keeps the three components unambiguously delimited, so
 * no (feature_dir, prompt, agent) triple can collide with another.
 * @param featureDir Feature directory of the delegated work, or null.
 * @param prompt The delegated task's prompt text.
 * @param agent The delegated agent name, when the call carried one.
 * @returns The hex-encoded SHA-256 cache key.
 */
export function classifierCacheKey(featureDir: string | null, prompt: string, agent: string | undefined): string {
  return createHash("sha256")
    .update(JSON.stringify([featureDir ?? "", prompt, agent ?? ""]))
    .digest("hex")
}

/** Task context for one classification request. */
export interface ClassifierRequest {
  /** Selection parameters of the delegation call (bypass predicate inputs; `agent` joins the cache key). */
  readonly inputs: SelectionInputs
  /** Feature directory of the delegated work, when one is active (cache key; engine prompt context). */
  readonly feature_dir: string | null
  /** The delegated task's prompt text (cache key; engine prompt input). */
  readonly prompt: string
}

/**
 * Injected seams for the classifier cascade (design D5; Constitution IV —
 * every classifier behavior is testable without a network or a live
 * subagent).
 */
export interface ClassifierCascadeDependencies {
  /**
   * Spawns the hidden classifier subagent and resolves its response text.
   * Production wiring reuses the agent-executor session path with the
   * configured classifier agent name and the signal as its parent abort;
   * the child session inherits the session model hierarchy (no endpoint
   * configuration, MM-FR-005).
   */
  readonly spawnAgent: (agent: string, prompt: string, signal: AbortSignal) => Promise<string>
  /**
   * Issues the LLM engine's chat-completions POST to the configured
   * endpoint (production wiring uses the global fetch). The endpoint is
   * the full chat-completions URL, configured only in the gitignored
   * override under `classifier.llm.endpoint` (MM-FR-006).
   */
  readonly fetch: typeof fetch
  /** Reads one environment variable for `{env:VAR}` apiKey interpolation (production wiring reads process.env). */
  readonly readEnv: (name: string) => string | undefined
  /** Result-cache override; defaults to a per-instance bounded cache. */
  readonly cache?: ClassifierCache
}

/**
 * The two-engine classifier cascade for `implement` and unknown task kinds
 * (design D5, MM-FR-005/MM-FR-006): agent engine, then LLM engine, then
 * static fallback. The cascade produces the {@link ClassifierOutcome} that
 * {@link resolveSelection} consumes — selection stays a pure function, so
 * the engines run before resolution and participate only as that value.
 *
 * Gating: the first valid JSON response naming a matrix-valid pack (a
 * `packs` key of the configured matrix) with confidence at or above the
 * configured threshold wins. Below-threshold, malformed (including
 * fences-wrapped JSON), matrix-invalid, timed-out, or errored results fall
 * to the next engine; when both engines fail, the cascade reports
 * `failed` with each engine's reason and selection falls back statically.
 * Classifier failure NEVER throws to the caller and never blocks
 * delegation (MM-FR-005).
 *
 * Constructed once per delegation-tool plugin instance; the result cache
 * therefore lives and dies with the instance and is never shared across
 * cascades. All configuration (engine names, timeouts, threshold, LLM
 * endpoint) is read from the loaded matrix per call, so merged-view
 * override values govern exactly as they do for selection.
 */
export class ClassifierCascade {
  private readonly cache: ClassifierCache

  /**
   * @param dependencies Injected spawn, fetch, environment, and cache seams.
   */
  constructor(private readonly dependencies: ClassifierCascadeDependencies) {
    this.cache = dependencies.cache ?? createClassifierCache()
  }

  /**
   * Runs the cascade for one delegation.
   *
   * The bypass predicate runs first (MM-FR-003): an explicit `model`,
   * `profile`, or `pack` parameter, an `agents:` pin that yields before
   * step 6, a static `plan`/`tasks` kind, `classifier.enabled: false`, or
   * no configured matrix resolves to `not-run` without touching either
   * engine or the cache. Otherwise the cache is consulted (a hit skips
   * both engines), then the agent engine, then the LLM engine.
   * @param matrix The loaded matrix, or null when none is configured.
   * @param request The delegation's selection inputs, feature dir, and prompt.
   * @returns The cascade outcome for {@link resolveSelection}. Never throws.
   */
  async classify(matrix: ModelMatrix | null, request: ClassifierRequest): Promise<ClassifierOutcome> {
    const state = classifierRunState(matrix, request.inputs)
    if (!state.run) {
      return { status: "not-run", reason: state.reason }
    }
    const key = classifierCacheKey(request.feature_dir, request.prompt, request.inputs.agent)
    const cached = this.cache.get(key)
    if (cached !== undefined) {
      return { status: "selected", engine: cached.engine, pack: cached.pack, confidence: cached.confidence }
    }
    const agentAttempt = await this.runAgentEngine(state.matrix, request)
    if (agentAttempt.selected) {
      this.cache.set(key, agentAttempt)
      return {
        status: "selected",
        engine: agentAttempt.engine,
        pack: agentAttempt.pack,
        confidence: agentAttempt.confidence,
      }
    }
    const llmAttempt = await this.runLlmEngine(state.matrix, request)
    if (llmAttempt.selected) {
      this.cache.set(key, llmAttempt)
      return {
        status: "selected",
        engine: llmAttempt.engine,
        pack: llmAttempt.pack,
        confidence: llmAttempt.confidence,
      }
    }
    return { status: "failed", reason: `agent engine: ${agentAttempt.reason}; llm engine: ${llmAttempt.reason}` }
  }

  /**
   * Runs the agent engine: spawns the hidden classifier subagent through
   * the injected session-spawn seam, bounded by the configured
   * `agent_timeout_ms` (default 45,000). The bound is enforced by racing
   * the spawn against a timer that aborts the signal — the production
   * seam forwards that signal to the executor's parent-abort chain, so a
   * timed-out child session is cancelled, and a seam that ignores the
   * signal still cannot exceed the bound.
   * @param matrix The configured matrix (agent name, timeout, packs, threshold).
   * @param request The delegation context.
   * @returns The gated engine attempt.
   */
  private async runAgentEngine(matrix: ModelMatrix, request: ClassifierRequest): Promise<EngineAttempt> {
    const agentName = matrix.classifier?.agent ?? DEFAULT_CLASSIFIER_AGENT
    const timeoutMs = matrix.classifier?.agent_timeout_ms ?? DEFAULT_AGENT_TIMEOUT_MS
    const controller = new AbortController()
    let timer: NodeJS.Timeout | undefined
    const timeoutPromise = new Promise<never>((_, reject) => {
      timer = setTimeout(() => {
        controller.abort(new Error(`classifier agent engine exceeded agent_timeout_ms ${timeoutMs}`))
        reject(controller.signal.reason)
      }, timeoutMs)
    })
    // The losing promise's late rejection must never surface unhandled:
    // the race result is authoritative, and the spawn is cancellable only
    // through the signal (Constitution III — no silent background failures).
    const spawn = this.dependencies.spawnAgent(
      agentName,
      renderAgentEnginePrompt(request, sortedPackNames(matrix)),
      controller.signal,
    )
    void spawn.catch(() => {})
    try {
      const text = await Promise.race([spawn, timeoutPromise])
      return gateClassifierResponse(matrix, "agent", parseClassifierResponse(text))
    } catch (error: unknown) {
      return { selected: false, reason: errorText(error) }
    } finally {
      clearTimeout(timer)
    }
  }

  /**
   * Runs the LLM engine: a chat-completions POST to the endpoint
   * configured under the merged matrix's `classifier.llm.*` (populated
   * values arrive exclusively via the gitignored override, MM-FR-006).
   * Unconfigured (no endpoint or model) skips the engine without any
   * network call. Each request carries `temperature: 0` and
   * `response_format: {type: "json_object"}`, with a single 400-retry
   * without the `response_format` field; each attempt is bounded by
   * `request_timeout_ms` (default 10,000) via AbortSignal, and the
   * response body is capped at 50 MiB without buffering an oversized
   * body.
   * @param matrix The configured matrix (LLM config, packs, threshold).
   * @param request The delegation context.
   * @returns The gated engine attempt.
   */
  private async runLlmEngine(matrix: ModelMatrix, request: ClassifierRequest): Promise<EngineAttempt> {
    const llm = matrix.classifier?.llm
    if (llm === undefined || llm.endpoint === undefined || llm.model === undefined) {
      return {
        selected: false,
        reason: "skipped — classifier.llm carries no endpoint configuration (endpoint and model arrive via the gitignored override)",
      }
    }
    let url: URL
    try {
      url = validateLlmEndpoint(llm.endpoint)
    } catch (error: unknown) {
      return { selected: false, reason: `endpoint validation failed: ${errorText(error)}` }
    }
    const apiKey = llm.apiKey === undefined ? null : interpolateLlmApiKey(llm.apiKey, this.dependencies.readEnv)
    if (apiKey !== null && apiKey.status !== "ok") {
      return { selected: false, reason: `skipped — ${apiKey.reason}` }
    }
    const timeoutMs = llm.request_timeout_ms ?? DEFAULT_LLM_REQUEST_TIMEOUT_MS
    const baseBody: Record<string, unknown> = {
      model: llm.model,
      messages: [{ role: "user", content: renderLlmEnginePrompt(request, sortedPackNames(matrix)) }],
      temperature: 0,
    }
    let attempt = await this.attemptLlmRequest(
      url,
      { ...baseBody, response_format: { type: "json_object" } },
      apiKey,
      timeoutMs,
    )
    if (!attempt.ok && attempt.status === 400) {
      // Single retry without response_format: some endpoints reject the
      // field (MM-FR-006). The retry is a fresh request with its own bound.
      attempt = await this.attemptLlmRequest(url, baseBody, apiKey, timeoutMs)
    }
    if (!attempt.ok) {
      return { selected: false, reason: attempt.reason }
    }
    const content = chatCompletionsContent(attempt.content)
    if (content === null) {
      return { selected: false, reason: "response is not a chat-completions choices[0].message.content string" }
    }
    return gateClassifierResponse(matrix, "llm", parseClassifierResponse(content))
  }

  /**
   * Issues one LLM-engine HTTP attempt bounded by the request timeout.
   * The bound is enforced by racing the request pipeline against a timer
   * that aborts the controller — the injected fetch receives the signal
   * (the production seam aborts with it), but the bound holds even for a
   * seam that ignores it, mirroring the agent engine's race discipline.
   * The bound covers the request and the body read.
   * @param url The validated endpoint URL.
   * @param body The JSON request body.
   * @param apiKey The interpolated apiKey state, or null for no header.
   * @param timeoutMs The per-request timeout in milliseconds.
   * @returns The bounded body text or the failure reason with the HTTP status.
   */
  private async attemptLlmRequest(
    url: URL,
    body: Record<string, unknown>,
    apiKey: InterpolatedApiKey | null,
    timeoutMs: number,
  ): Promise<LlmHttpResponse> {
    const controller = new AbortController()
    let timer: NodeJS.Timeout | undefined
    const timeoutPromise = new Promise<never>((_, reject) => {
      timer = setTimeout(() => {
        controller.abort(new Error(`classifier LLM request exceeded request_timeout_ms ${timeoutMs}`))
        reject(controller.signal.reason)
      }, timeoutMs)
    })
    // The losing promise's late rejection must never surface unhandled:
    // the race result is authoritative (same discipline as the agent engine).
    const request = this.issueLlmRequest(url, body, apiKey, controller.signal)
    void request.catch(() => {})
    try {
      return await Promise.race([request, timeoutPromise])
    } catch (error: unknown) {
      if (controller.signal.aborted) {
        return { ok: false, status: 0, reason: `request timed out after ${timeoutMs}ms (request_timeout_ms)` }
      }
      return { ok: false, status: 0, reason: `request failed: ${errorText(error)}` }
    } finally {
      clearTimeout(timer)
    }
  }

  /**
   * Issues the LLM-engine fetch and reads its body under the 50 MiB cap.
   * A non-2xx response cancels its body without buffering the error
   * payload; an ok response is read under the cap without buffering an
   * oversized body.
   * @param url The validated endpoint URL.
   * @param body The JSON request body.
   * @param apiKey The interpolated apiKey state, or null for no header.
   * @param signal The per-attempt abort signal.
   * @returns The bounded body text or the failure reason with the HTTP status.
   */
  private async issueLlmRequest(
    url: URL,
    body: Record<string, unknown>,
    apiKey: InterpolatedApiKey | null,
    signal: AbortSignal,
  ): Promise<LlmHttpResponse> {
    const response = await this.dependencies.fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(apiKey !== null && apiKey.status === "ok" ? { Authorization: `Bearer ${apiKey.value}` } : {}),
      },
      body: JSON.stringify(body),
      signal,
    })
    if (!response.ok) {
      // The error body is never needed — cancel it best-effort so the
      // connection is released without buffering an error payload.
      void response.body?.cancel().catch(() => {})
      return { ok: false, status: response.status, reason: `endpoint returned HTTP ${response.status}` }
    }
    const content = await readBoundedResponseBody(response)
    if (content === null) {
      return {
        ok: false,
        status: response.status,
        reason: `response body exceeds the ${MAX_LLM_RESPONSE_BYTES}-byte cap (50 MiB) and is treated as malformed`,
      }
    }
    return { ok: true, content }
  }
}

/**
 * Decides whether the cascade runs for one delegation (MM-FR-003: only
 * chains that reach step 6). An explicit `model`, `profile`, or `pack`
 * parameter bypasses the classifier entirely; an `agents:` pin yields at
 * step 4 before the classifier; `plan`/`tasks` resolve statically; an
 * absent `classifier.enabled` defaults to enabled (only `false`
 * disables); and a missing or no-op matrix has no classifier
 * participation (MM-FR-004 zero-config).
 * @param matrix The loaded matrix, or null.
 * @param inputs The delegation call's selection parameters.
 * @returns The run state with the configured matrix, or the not-run reason.
 */
function classifierRunState(matrix: ModelMatrix | null, inputs: SelectionInputs): ClassifierRunState {
  if (inputs.model !== undefined || inputs.profile !== undefined || inputs.pack !== undefined) {
    return { run: false, reason: "an explicit model, profile, or pack parameter bypasses the classifier" }
  }
  const configured = matrix !== null && Object.keys(matrix.profiles).length > 0 ? matrix : null
  if (configured === null) {
    return { run: false, reason: "no model matrix is configured" }
  }
  if (configured.classifier?.enabled === false) {
    return { run: false, reason: "classifier is disabled (classifier.enabled: false)" }
  }
  const kind = inputs.checkpoint_step
  if (kind === "plan" || kind === "tasks") {
    return { run: false, reason: `task kind "${kind}" resolves statically and never reaches the classifier` }
  }
  if (inputs.agent !== undefined && Object.hasOwn(configured.agents ?? {}, inputs.agent)) {
    return { run: false, reason: `agents pin for "${inputs.agent}" yields before the classifier` }
  }
  return { run: true, matrix: configured }
}

/** Sorted `packs` keys of a configured matrix — the classifier's candidate set. */
function sortedPackNames(matrix: ModelMatrix): string[] {
  return Object.keys(matrix.packs ?? {}).sort()
}

/**
 * Renders the agent engine's spawn prompt. The subagent definition
 * (`.opencode/agents/model-matrix-classifier.md`) carries the
 * investigate-then-decide protocol; this prompt carries the task
 * specifics — the candidate pack set and the delegated task text.
 * @param request The delegation context.
 * @param packs The sorted candidate pack names.
 * @returns The spawn prompt text.
 */
function renderAgentEnginePrompt(request: ClassifierRequest, packs: readonly string[]): string {
  const sections = ["Classify the delegated task below into exactly one UF convention pack."]
  if (request.feature_dir !== null) {
    sections.push(`Feature directory: ${request.feature_dir}`)
  }
  sections.push(`Candidate packs: ${packs.join(", ")}`)
  sections.push(
    'Investigate the feature directory\'s spec, plan, and tasks plus the relevant code, then respond with ONLY the JSON object ' +
      '{"pack": "<name>", "confidence": <0.0-1.0>, "reasoning": "<optional>"} — no markdown fences, no prose, never ask questions.',
  )
  sections.push(`Delegated task:\n\n${request.prompt}`)
  return sections.join("\n\n")
}

/**
 * Renders the LLM engine's chat-completions message. Self-contained (the
 * endpoint has no investigation tools), and it names JSON explicitly —
 * endpoints enforcing `response_format: json_object` require the request
 * to mention JSON.
 * @param request The delegation context.
 * @param packs The sorted candidate pack names.
 * @returns The message content text.
 */
function renderLlmEnginePrompt(request: ClassifierRequest, packs: readonly string[]): string {
  const sections = ["Classify the delegated task below into exactly one UF convention pack."]
  if (request.feature_dir !== null) {
    sections.push(`Feature directory: ${request.feature_dir}`)
  }
  sections.push(`Candidate packs: ${packs.join(", ")}`)
  sections.push(
    'Respond with ONLY a JSON object {"pack": "<name>", "confidence": <0.0-1.0>} — no markdown fences, no prose.',
  )
  sections.push(`Delegated task:\n\n${request.prompt}`)
  return sections.join("\n\n")
}

/**
 * Parses one engine's response under the strict JSON-only contract
 * (MM-FR-005): the text must be a bare JSON object carrying a non-empty
 * string `pack` and a finite `confidence` between 0 and 1, with an
 * optional string `reasoning`. Markdown-fenced or prose-wrapped
 * responses fail `JSON.parse` and are malformed by contract; extra keys
 * are tolerated because the gates consume only `pack` and `confidence`.
 * @param text The engine's response text.
 * @returns The parsed response, or null when malformed.
 */
function parseClassifierResponse(text: string): ClassifierResponse | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return null
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return null
  }
  const { pack, confidence, reasoning } = parsed as Record<string, unknown>
  if (typeof pack !== "string" || pack.length === 0) {
    return null
  }
  if (typeof confidence !== "number" || !Number.isFinite(confidence) || confidence < 0 || confidence > 1) {
    return null
  }
  if (reasoning !== undefined && typeof reasoning !== "string") {
    return null
  }
  return reasoning === undefined ? { pack, confidence } : { pack, confidence, reasoning }
}

/**
 * Applies the cascade gates to one parsed engine response (MM-FR-005):
 * the named pack must be a `packs` key of the configured matrix (pack
 * files were existence-checked at load time, so key membership is the
 * matrix-validity test), and the confidence must meet the configured
 * threshold (default 0.6). `resolveSelection` re-applies the identical
 * gates to whatever outcome it receives — defense in depth for callers
 * that present a pre-computed outcome the cascade never gated.
 * @param matrix The configured matrix supplying `packs` and the threshold.
 * @param engine The engine whose response is being gated.
 * @param response The parsed response, or null when malformed.
 * @returns The gated selection or the recorded failure reason.
 */
function gateClassifierResponse(
  matrix: ModelMatrix,
  engine: ClassifierEngine,
  response: ClassifierResponse | null,
): EngineAttempt {
  if (response === null) {
    return {
      selected: false,
      reason: 'malformed response — expected the bare JSON object {"pack": "...", "confidence": ...} without markdown fences or prose',
    }
  }
  const packs = matrix.packs ?? {}
  if (!Object.hasOwn(packs, response.pack)) {
    return { selected: false, reason: `pack "${response.pack}" is not a packs key in the configured matrix` }
  }
  const threshold = matrix.classifier?.confidence_threshold ?? DEFAULT_CONFIDENCE_THRESHOLD
  if (response.confidence < threshold) {
    return { selected: false, reason: `confidence ${response.confidence} is below the threshold ${threshold}` }
  }
  return { selected: true, engine, pack: response.pack, confidence: response.confidence }
}

/**
 * Validates the configured LLM endpoint (design D4, MM-FR-006): the URL
 * must parse and use the `https` scheme, or `http` with a loopback host
 * (`localhost`, `127.0.0.1`, `[::1]` — local endpoints such as Ollama).
 * Any other `http` endpoint fails before a request is issued.
 * @param endpoint The configured endpoint URL string.
 * @returns The parsed endpoint URL.
 * @throws When the endpoint is not a valid URL or violates the scheme rule.
 */
function validateLlmEndpoint(endpoint: string): URL {
  let url: URL
  try {
    url = new URL(endpoint)
  } catch {
    throw new Error(`classifier.llm.endpoint "${endpoint}" is not a valid URL`)
  }
  if (url.protocol !== "https:" && !(url.protocol === "http:" && LOOPBACK_HOSTS.has(url.hostname))) {
    throw new Error(
      `classifier.llm.endpoint "${endpoint}" must use the https scheme or an http loopback host (localhost, 127.0.0.1, [::1])`,
    )
  }
  return url
}

/**
 * Interpolates the configured apiKey from its `{env:VAR}` reference
 * (MM-FR-006). A literal key is rejected without echoing it — the value
 * is a secret and must never land in a reason string or the audit trail
 * (Constitution V); a reference naming an unset variable skips the
 * engine for this invocation, never an empty credential.
 * @param apiKey The configured apiKey string.
 * @param readEnv Injected environment reader.
 * @returns The interpolated value or the recorded skip reason.
 */
function interpolateLlmApiKey(apiKey: string, readEnv: (name: string) => string | undefined): InterpolatedApiKey {
  const match = ENV_API_KEY_PATTERN.exec(apiKey)
  if (match === null) {
    return {
      status: "skipped",
      reason: "classifier.llm.apiKey is a literal key — an {env:VAR} reference is required; engine skipped",
    }
  }
  const value = readEnv(match[1])
  if (value === undefined || value === "") {
    return {
      status: "skipped",
      reason: `environment variable ${match[1]} referenced by classifier.llm.apiKey is not set — engine skipped (never an empty credential)`,
    }
  }
  return { status: "ok", value }
}

/**
 * Reads a response body under the 50 MiB cap without buffering an
 * oversized body (MM-FR-006): a declared `content-length` over the cap
 * is rejected before a single byte is read; a streaming body is read
 * incrementally and cancelled as soon as the running byte count exceeds
 * the cap, discarding the bytes already held. A null body (mock and
 * empty responses) falls back to `text()` with the same length check.
 * @param response The ok HTTP response.
 * @returns The body text, or null when the body exceeds the cap.
 */
async function readBoundedResponseBody(response: Response): Promise<string | null> {
  const declared = response.headers.get("content-length")
  if (declared !== null) {
    const length = Number(declared)
    if (Number.isFinite(length) && length > MAX_LLM_RESPONSE_BYTES) {
      // Oversized by declaration — rejected before buffering anything.
      void response.body?.cancel().catch(() => {})
      return null
    }
  }
  if (response.body === null) {
    const text = await response.text()
    return Buffer.byteLength(text, "utf8") > MAX_LLM_RESPONSE_BYTES ? null : text
  }
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let received = 0
  for (;;) {
    const { done, value } = await reader.read()
    if (done) {
      break
    }
    if (value === undefined) {
      continue
    }
    received += value.byteLength
    if (received > MAX_LLM_RESPONSE_BYTES) {
      // Oversized mid-stream — cancel the reader without reading further;
      // the already-buffered chunks are discarded with the result.
      void reader.cancel().catch(() => {})
      return null
    }
    chunks.push(value)
  }
  return Buffer.concat(chunks).toString("utf8")
}

/**
 * Extracts the assistant message content from a chat-completions
 * response body (`choices[0].message.content`).
 * @param body The response body text.
 * @returns The content string, or null when the shape is absent.
 */
function chatCompletionsContent(body: string): string | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(body)
  } catch {
    return null
  }
  if (typeof parsed !== "object" || parsed === null) {
    return null
  }
  const choices = (parsed as Record<string, unknown>).choices
  if (!Array.isArray(choices) || choices.length === 0) {
    return null
  }
  const first = choices[0]
  if (typeof first !== "object" || first === null) {
    return null
  }
  const message = (first as Record<string, unknown>).message
  if (typeof message !== "object" || message === null) {
    return null
  }
  const content = (message as Record<string, unknown>).content
  return typeof content === "string" ? content : null
}
