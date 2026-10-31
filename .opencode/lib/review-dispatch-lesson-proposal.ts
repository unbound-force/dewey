import { Buffer } from "node:buffer"
import { createHash } from "node:crypto"

import { tool } from "@opencode-ai/plugin"
import { z } from "zod"

import {
  UNTRUSTED_SIBLING_EVIDENCE_BEGIN,
  UNTRUSTED_SIBLING_EVIDENCE_END,
  type AcquireSiblingEvidenceResult,
  type SiblingEvidenceFile,
} from "./review-dispatch-sibling-evidence.js"

const MAX_CHILD_OUTPUT_BYTES = 1024 * 1024
const MAX_LESSON_SECTION_BYTES = 8 * 1024
const MAX_PATH_BYTES = 512
const MAX_EXCERPT_BYTES = 1024
const MAX_INFORMATION_BYTES = 2 * 1024
const MAX_KNOWN_DEDUPE_HASHES = 1024
const LESSON_CATEGORY = "reference" as const
const PROVENANCE_PREFIX = "UF_LESSON_PROVENANCE_V1 "

/** Exact opening delimiter for a version 1 lesson proposal. */
export const LESSON_PROPOSAL_BEGIN = "<!-- uf-lesson-proposal:v1 -->"

/** Exact closing delimiter for a version 1 lesson proposal. */
export const LESSON_PROPOSAL_END = "<!-- /uf-lesson-proposal -->"

const boundedUTF8String = (maximumBytes: number): z.ZodString =>
  z.string().min(1).superRefine((value, context) => {
    if (Buffer.byteLength(value, "utf8") > maximumBytes) {
      context.addIssue({ code: "custom", message: `must be at most ${maximumBytes} UTF-8 bytes` })
    }
  })

const LessonSourceSchema = z
  .object({
    path: boundedUTF8String(MAX_PATH_BYTES),
    sha256: z.string().regex(/^[0-9a-f]{64}$/),
    excerpt: boundedUTF8String(MAX_EXCERPT_BYTES),
  })
  .strict()

const LessonProposalSchema = z
  .object({
    schema_version: z.literal("1.0.0"),
    sibling: z.string().regex(/^[a-z][a-z0-9-]{0,62}$/),
    commit: z.string().regex(/^[0-9a-f]{40}$/),
    sources: z.array(LessonSourceSchema).min(1).max(8),
    information: boundedUTF8String(MAX_INFORMATION_BYTES),
  })
  .strict()

const SiblingEvidenceFileSchema = z
  .object({
    sibling: z.string(),
    commit: z.string(),
    path: z.string(),
    sha256: z.string(),
    source_mode: z.enum(["local", "clone", "cache", "offline-cache"]),
  })
  .strict()

const SiblingEvidenceRejectionSchema = z
  .object({ scope: z.enum(["candidate", "network", "file"]), subject: z.string(), reason_code: z.string(), reason: z.string() })
  .strict()

const AcquireSiblingEvidenceResultSchema = z
  .object({
    version: z.literal(1),
    status: z.enum(["ok", "invalid-config"]),
    prompt: z.string(),
    siblings: z.array(
      z
        .object({
          sibling: z.string(),
          status: z.enum(["available", "unavailable"]),
          commit: z.string().nullable(),
          source_mode: z.enum(["local", "clone", "cache", "offline-cache"]).nullable(),
          source_candidate: z.string().nullable(),
          evidence: z.array(SiblingEvidenceFileSchema),
          rejections: z.array(SiblingEvidenceRejectionSchema),
          unavailable_reason: z.string().nullable(),
        })
        .strict(),
    ),
    errors: z.array(z.string()),
  })
  .strict()

const KnownDedupeHashesSchema = z
  .array(z.string().regex(/^[0-9a-f]{64}$/))
  .max(MAX_KNOWN_DEDUPE_HASHES)
  .superRefine((hashes, context) => {
    if (new Set(hashes).size !== hashes.length) context.addIssue({ code: "custom", message: "hashes must be unique" })
  })

const PrepareLessonLearningInputSchema = z
  .object({
    child_output: z.string(),
    sibling_evidence: AcquireSiblingEvidenceResultSchema,
    known_dedupe_sha256: KnownDedupeHashesSchema,
  })
  .strict()

/** Closed version 1 lesson proposal emitted by a child review. */
export type LessonProposal = z.output<typeof LessonProposalSchema>

/** One normalized, source-grounded provenance entry retained for audit. */
export interface LessonProvenanceSource {
  readonly path: string
  readonly sha256: string
  readonly excerpt: string
}

/** Complete immutable provenance for a prepared learning. */
export interface LessonProvenance {
  readonly sibling: string
  readonly commit: string
  readonly sources: readonly LessonProvenanceSource[]
}

/** Exact payload accepted by Dewey's existing store_learning interface. */
export interface DeweyStoreLearningPayload {
  readonly information: string
  readonly tag: string
  readonly category: "reference"
}

/** Stable, non-sensitive reason recorded when a proposal is skipped. */
export type LessonSkipReason =
  | "no-delimited-proposal"
  | "child-output-too-large"
  | "malformed-section"
  | "multiple-sections"
  | "section-too-large"
  | "schema-invalid"
  | "information-empty-after-normalization"
  | "secret-prefix-sk"
  | "secret-prefix-akia"
  | "secret-prefix-github"
  | "secret-prefix-slack"
  | "secret-bearer-token"
  | "secret-api-key-assignment"
  | "secret-environment-assignment"
  | "tool-learn-directive"
  | "tool-invoke-agent"
  | "tool-dewey-store-learning"
  | "tool-dewey-store"
  | "tool-hivemind-store"
  | "tool-forge-record-outcome"
  | "tool-execute"
  | "tool-fenced-command"
  | "grounding-unavailable"
  | "grounding-provenance-mismatch"
  | "grounding-excerpt-mismatch"
  | "duplicate"

/** Closed result returned to parent commands before optional Dewey storage. */
export type PreparedLessonLearning =
  | {
      readonly status: "ready"
      readonly reason_code: null
      readonly learning: DeweyStoreLearningPayload
      readonly dedupe_sha256: string
      readonly provenance: LessonProvenance
    }
  | {
      readonly status: "skipped"
      readonly reason_code: LessonSkipReason
      readonly learning: null
      readonly dedupe_sha256: null
      readonly provenance: null
    }

interface AcceptedEvidenceContent {
  readonly metadata: SiblingEvidenceFile
  readonly content: string
}

interface InformationDetector {
  readonly reason: LessonSkipReason
  readonly expression: RegExp
}

interface StoredProvenanceSource {
  readonly path: string
  readonly sha256: string
}

interface StoredProvenance {
  readonly dedupe_sha256: string
  readonly sibling: string
  readonly commit: string
  readonly sources: readonly StoredProvenanceSource[]
}

const INFORMATION_DETECTORS: readonly InformationDetector[] = [
  { reason: "secret-prefix-sk", expression: /sk-/iu },
  { reason: "secret-prefix-akia", expression: /akia/iu },
  { reason: "secret-prefix-github", expression: /ghp_/iu },
  { reason: "secret-prefix-slack", expression: /xox[baprs]-/iu },
  { reason: "secret-bearer-token", expression: /bearer [^\s]+/iu },
  {
    reason: "secret-environment-assignment",
    expression:
      /\b(?:anthropic_api_key|anthropic_auth_token|aws_secret_access_key|github_token|openai_api_key)\s*=/iu,
  },
  { reason: "secret-api-key-assignment", expression: /api[_-]?key\s*[:=]/iu },
  { reason: "tool-learn-directive", expression: /> learn:/iu },
  { reason: "tool-invoke-agent", expression: /invoke_agent/iu },
  { reason: "tool-dewey-store-learning", expression: /dewey_store_learning/iu },
  { reason: "tool-dewey-store", expression: /dewey_store/iu },
  { reason: "tool-hivemind-store", expression: new RegExp(`hivemind${"_store"}`, "iu") },
  { reason: "tool-forge-record-outcome", expression: /forge_record_outcome/iu },
  { reason: "tool-execute", expression: /tool\.execute/iu },
]

function occurrences(value: string, token: string): readonly number[] {
  const positions: number[] = []
  let cursor = 0
  while (cursor <= value.length - token.length) {
    const position = value.indexOf(token, cursor)
    if (position < 0) break
    positions.push(position)
    cursor = position + token.length
  }
  return positions
}

function hasUnpairedSurrogate(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const unit = value.charCodeAt(index)
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const next = value.charCodeAt(index + 1)
      if (next < 0xdc00 || next > 0xdfff) return true
      index += 1
    } else if (unit >= 0xdc00 && unit <= 0xdfff) return true
  }
  return false
}

function parseProposalSection(childOutput: string): LessonProposal | LessonSkipReason {
  if (Buffer.byteLength(childOutput, "utf8") > MAX_CHILD_OUTPUT_BYTES) return "child-output-too-large"
  if (hasUnpairedSurrogate(childOutput)) return "malformed-section"

  const openings = occurrences(childOutput, LESSON_PROPOSAL_BEGIN)
  const closings = occurrences(childOutput, LESSON_PROPOSAL_END)
  if (openings.length === 0 && closings.length === 0) return "no-delimited-proposal"
  if (openings.length > 1 || closings.length > 1) return "multiple-sections"
  if (openings.length !== 1 || closings.length !== 1 || closings[0]! < openings[0]!) return "malformed-section"

  const sectionEnd = closings[0]! + LESSON_PROPOSAL_END.length
  const section = childOutput.slice(openings[0], sectionEnd)
  if (Buffer.byteLength(section, "utf8") > MAX_LESSON_SECTION_BYTES) return "section-too-large"

  const body = childOutput.slice(openings[0]! + LESSON_PROPOSAL_BEGIN.length, closings[0])
  let parsed: unknown
  try {
    parsed = JSON.parse(body) as unknown
  } catch {
    return "malformed-section"
  }
  const proposal = LessonProposalSchema.safeParse(parsed)
  return proposal.success ? proposal.data : "schema-invalid"
}

function evidenceKey(metadata: Pick<SiblingEvidenceFile, "sibling" | "commit" | "path" | "sha256">): string {
  return `${metadata.sibling}\u0000${metadata.commit}\u0000${metadata.path}\u0000${metadata.sha256}`
}

function parseAcceptedEvidence(acquisition: AcquireSiblingEvidenceResult): ReadonlyMap<string, AcceptedEvidenceContent> {
  if (acquisition.status !== "ok") throw new Error("sibling evidence acquisition was not accepted")

  const accepted = new Map<string, SiblingEvidenceFile>()
  for (const sibling of acquisition.siblings) {
    if (sibling.status !== "available" || sibling.commit === null) continue
    for (const metadata of sibling.evidence) {
      if (metadata.sibling !== sibling.sibling || metadata.commit !== sibling.commit) {
        throw new Error("sibling evidence metadata disagrees with its repository result")
      }
      const key = evidenceKey(metadata)
      if (accepted.has(key)) throw new Error("sibling evidence metadata is duplicated")
      accepted.set(key, metadata)
    }
  }

  const contents = new Map<string, AcceptedEvidenceContent>()
  let cursor = 0
  while (cursor < acquisition.prompt.length) {
    const opening = acquisition.prompt.indexOf(UNTRUSTED_SIBLING_EVIDENCE_BEGIN, cursor)
    if (opening < 0) {
      if (acquisition.prompt.slice(cursor).trim() !== "") throw new Error("unexpected sibling prompt content")
      break
    }
    if (acquisition.prompt.slice(cursor, opening).trim() !== "") throw new Error("unexpected sibling prompt prefix")
    const bodyStart = opening + UNTRUSTED_SIBLING_EVIDENCE_BEGIN.length
    if (acquisition.prompt[bodyStart] !== "\n") throw new Error("malformed sibling evidence section")
    const metadataEnd = acquisition.prompt.indexOf("\n", bodyStart + 1)
    const closingPrefix = `\n${UNTRUSTED_SIBLING_EVIDENCE_END}`
    const closing = acquisition.prompt.indexOf(closingPrefix, metadataEnd + 1)
    if (metadataEnd < 0 || closing < 0) throw new Error("malformed sibling evidence section")

    let metadataValue: unknown
    try {
      metadataValue = JSON.parse(acquisition.prompt.slice(bodyStart + 1, metadataEnd)) as unknown
    } catch {
      throw new Error("malformed sibling evidence metadata")
    }
    const metadata = SiblingEvidenceFileSchema.parse(metadataValue)
    const key = evidenceKey(metadata)
    const acceptedMetadata = accepted.get(key)
    if (acceptedMetadata === undefined || acceptedMetadata.source_mode !== metadata.source_mode || contents.has(key)) {
      throw new Error("sibling prompt metadata was not accepted")
    }
    contents.set(key, { metadata, content: acquisition.prompt.slice(metadataEnd + 1, closing) })
    cursor = closing + closingPrefix.length
  }
  if (contents.size !== accepted.size) throw new Error("accepted sibling evidence content is incomplete")
  return contents
}

function normalizeLineEndings(value: string): string {
  return value.replaceAll("\r\n", "\n")
}

function detectUnsafeInformation(information: string): LessonSkipReason | null {
  for (const detector of INFORMATION_DETECTORS) {
    if (detector.expression.test(information)) return detector.reason
  }
  const fencedBlocks = information.matchAll(/```[^\r\n]*\r?\n([\s\S]*?)```/giu)
  for (const block of fencedBlocks) {
    if (/\b(?:uf|opencode|dewey|replicator|gh|git)\b/iu.test(block[1] ?? "")) return "tool-fenced-command"
  }
  return null
}

/**
 * Normalizes lesson information for storage and deduplication only.
 * @param information Untrusted proposal information.
 * @returns NFC text with Unicode edge whitespace trimmed and ASCII whitespace runs collapsed.
 */
export function normalizeLessonInformation(information: string): string {
  return information.normalize("NFC").trim().replace(/[ \t\r\n]+/gu, " ")
}

/**
 * Computes the parent-owned lesson identity from normalized information and grounded sources.
 * @param normalizedInformation Information returned by normalizeLessonInformation.
 * @param sources Validated source identities.
 * @returns A lowercase SHA256 dedupe identity.
 */
export function lessonDedupeSHA256(
  normalizedInformation: string,
  sources: readonly Pick<LessonProvenanceSource, "path" | "sha256">[],
): string {
  const pairs = sources.map((source) => `${source.path}:${source.sha256}`).sort()
  return createHash("sha256").update(`${normalizedInformation}|${pairs.join("|")}`, "utf8").digest("hex")
}

function deweyInformation(information: string, dedupeSHA256: string, provenance: LessonProvenance): string {
  const audit: StoredProvenance = {
    dedupe_sha256: dedupeSHA256,
    sibling: provenance.sibling,
    commit: provenance.commit,
    sources: provenance.sources
      .map((source) => ({ path: source.path, sha256: source.sha256 }))
      .sort((left, right) => {
        const leftIdentity = `${left.path}:${left.sha256}`
        const rightIdentity = `${right.path}:${right.sha256}`
        return leftIdentity < rightIdentity ? -1 : leftIdentity > rightIdentity ? 1 : 0
      }),
  }
  return `${information}\n\n${PROVENANCE_PREFIX}${JSON.stringify(audit)}`
}

function skipped(reasonCode: LessonSkipReason): PreparedLessonLearning {
  return { status: "skipped", reason_code: reasonCode, learning: null, dedupe_sha256: null, provenance: null }
}

/**
 * Validates one child output and prepares one existing-Dewey learning payload.
 * @param childOutput Complete child review output, bounded to one MiB.
 * @param siblingEvidence Structured accepted evidence returned by task 3.1 acquisition.
 * @param knownDedupeSHA256 Bounded hashes found by the parent in existing learnings.
 * @returns A closed ready-to-store payload or a non-sensitive recorded skip reason.
 */
export function prepareLessonLearning(
  childOutput: string,
  siblingEvidence: AcquireSiblingEvidenceResult,
  knownDedupeSHA256: readonly string[],
): PreparedLessonLearning {
  const knownHashes = KnownDedupeHashesSchema.safeParse(knownDedupeSHA256)
  if (!knownHashes.success) return skipped("schema-invalid")

  const parsed = parseProposalSection(childOutput)
  if (typeof parsed === "string") return skipped(parsed)

  const unsafe = detectUnsafeInformation(parsed.information)
  if (unsafe !== null) return skipped(unsafe)

  let accepted: ReadonlyMap<string, AcceptedEvidenceContent>
  try {
    accepted = parseAcceptedEvidence(siblingEvidence)
  } catch {
    return skipped("grounding-unavailable")
  }

  const sources: LessonProvenanceSource[] = []
  for (const source of parsed.sources) {
    const key = evidenceKey({ sibling: parsed.sibling, commit: parsed.commit, path: source.path, sha256: source.sha256 })
    const evidence = accepted.get(key)
    if (evidence === undefined) return skipped("grounding-provenance-mismatch")
    const excerpt = normalizeLineEndings(source.excerpt)
    const content = normalizeLineEndings(evidence.content)
    if (!Buffer.from(content, "utf8").includes(Buffer.from(excerpt, "utf8"))) {
      return skipped("grounding-excerpt-mismatch")
    }
    sources.push({ path: source.path, sha256: source.sha256, excerpt })
  }

  const information = normalizeLessonInformation(parsed.information)
  if (information === "") return skipped("information-empty-after-normalization")
  const dedupeSHA256 = lessonDedupeSHA256(information, sources)
  if (knownHashes.data.includes(dedupeSHA256)) return skipped("duplicate")

  const provenance: LessonProvenance = { sibling: parsed.sibling, commit: parsed.commit, sources }
  return {
    status: "ready",
    reason_code: null,
    learning: {
      information: deweyInformation(information, dedupeSHA256, provenance),
      tag: `sibling-${parsed.sibling}`,
      category: LESSON_CATEGORY,
    },
    dedupe_sha256: dedupeSHA256,
    provenance,
  }
}

/**
 * Creates the pure parent-facing prepare_lesson_learning tool.
 * @returns A provider-free OpenCode tool that never reads or writes Dewey directly.
 */
export function createPrepareLessonLearningTool(): ReturnType<typeof tool> {
  return tool({
    description:
      "Validate one exact delimited lesson proposal against accepted sibling evidence and prepare a deduplicated Dewey learning payload.",
    args: {
      child_output: z.string(),
      sibling_evidence: AcquireSiblingEvidenceResultSchema,
      known_dedupe_sha256: KnownDedupeHashesSchema,
    },
    async execute(args): Promise<string> {
      const input = PrepareLessonLearningInputSchema.parse(args)
      return `${JSON.stringify(
        prepareLessonLearning(input.child_output, input.sibling_evidence, input.known_dedupe_sha256),
        null,
        2,
      )}\n`
    },
  })
}
