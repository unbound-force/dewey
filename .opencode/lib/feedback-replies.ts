/**
 * Reply posting, thread resolution, issue filing, and the
 * feedback-triage artifact write for `/uf.address-feedback` (OpenSpec
 * changes feedback-reply-tools and extract-feedback-triage-artifact-tool,
 * issues #966, #998, and #1022).
 *
 * The command used to hand-choreograph these steps in prose: a temp
 * file per reply, REST endpoint selection, cache-flag bookkeeping, a
 * separate GraphQL phase for thread resolution, and shell for
 * `gh issue create` with one retry. Every one of those steps is
 * mechanical (DE-001), and the chain between the two human gates has
 * no judgment in it (DE-005), so it runs here as code:
 *
 * - `postFeedbackReplies` files each FILE item's issue, posts every
 *   reply, maps review comments to GraphQL thread node IDs, and
 *   resolves the approved threads, in one call.
 * - `fileFeedbackIssue` files one finding as an issue, with one retry
 *   and a duplicate check against the feedback cache.
 * - `writeFeedbackTriageArtifact` joins the reply outcomes to the
 *   triage items by comment identity, derives the summary counts,
 *   scans the next round number, renders and validates the
 *   feedback-triage 1.1.0 envelope, writes it atomically, and clears
 *   the command's progress state (issue #1022).
 *
 * Judgment stays at the boundary (DE-006): the composed reply text,
 * the triage decision, and the human's resolve decision arrive as
 * parameters. Ordering, endpoint selection, retry, failure-stop, and
 * thread-ID mapping are constants and pure functions in this module.
 *
 * Every gh call goes through an injectable executor with an argument
 * array (no shell), and every untrusted body reaches gh through a
 * `0o600` temp file (`--input` / `--body-file`), never an argument.
 * Crash recovery: each mutation is written to
 * `.uf/feedback/pr-<N>/state.json` (under the tool-owned `replies`
 * key) before the next one starts, so a re-run skips what is done and
 * resumes what is not — including thread resolution.
 */

import { execFileSync } from "node:child_process"
import { randomUUID } from "node:crypto"
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join, relative, resolve, sep } from "node:path"
import { z } from "zod"

import { classifyGhFailure } from "./gh-error.js"

// ---------------------------------------------------------------------------
// Contract constants
// ---------------------------------------------------------------------------

/** gh executor signature (argument array, no shell), injectable for tests. */
export type FeedbackGhExec = (args: string[], cwd: string) => string

/** Per-invocation bound for each gh subprocess, matching the fetch tools. */
export const FEEDBACK_GH_TIMEOUT_MS = 30_000

/** Default gh executor: execFileSync with an argument array and a timeout. */
export const defaultFeedbackGhExec: FeedbackGhExec = (args: string[], cwd: string): string =>
  execFileSync("gh", args, {
    cwd,
    encoding: "utf-8",
    stdio: ["pipe", "pipe", "pipe"],
    timeout: FEEDBACK_GH_TIMEOUT_MS,
  })

/** Triage decisions a reply can carry (feedback-triage 1.1.0 decision enum). */
export const REPLY_DECISIONS = ["accept", "modify", "reject", "ask", "file", "reply"] as const

/** A triage decision carried by one reply item. */
export type ReplyDecision = (typeof REPLY_DECISIONS)[number]

/**
 * Decisions whose threads are addressed and therefore offered for
 * resolution (AFS-FR-002). REJECT and ASK threads stay open: the
 * reviewer still has to answer.
 */
export const RESOLVABLE_DECISIONS: ReadonlySet<ReplyDecision> = new Set([
  "accept",
  "modify",
  "file",
  "reply",
])

/** Which comment a reply answers: an inline review thread or the PR conversation. */
export const COMMENT_KINDS = ["review", "issue"] as const

/** Comment kind of one reply item. */
export type CommentKind = (typeof COMMENT_KINDS)[number]

/** Attempts for `gh issue create` and `resolveReviewThread`: one try plus one retry. */
export const MAX_ATTEMPTS = 2

/** Page bound for the reviewThreads lookup (100 threads per page). */
export const MAX_THREAD_PAGES = 50

/** GitHub's comment body limit is 65536 characters; leave room for the FILE prefix. */
export const MAX_BODY_CHARS = 65_000

/** GitHub's issue title limit. */
export const MAX_TITLE_CHARS = 256

/** Tool-owned key in `.uf/feedback/pr-<N>/state.json`. */
export const STATE_REPLIES_KEY = "replies"

/** Progress step the reply tool records once every reply and resolution is done. */
export const REPLY_CHECKPOINT_STEP = "reply-comments"

/** Reviewer roles in the feedback-triage 1.1.0 item enum. */
export const FEEDBACK_TRIAGE_REVIEWER_ROLES = [
  "maintainer",
  "collaborator",
  "contributor",
  "external",
  "bot",
] as const

/** A reviewer role in the feedback-triage 1.1.0 item enum. */
export type FeedbackTriageReviewerRole = (typeof FEEDBACK_TRIAGE_REVIEWER_ROLES)[number]

/** Classifications in the feedback-triage 1.1.0 item enum. */
export const FEEDBACK_TRIAGE_CLASSIFICATIONS = ["data-driven", "subjective"] as const

/** A feedback classification in the feedback-triage 1.1.0 item enum. */
export type FeedbackTriageClassification = (typeof FEEDBACK_TRIAGE_CLASSIFICATIONS)[number]

/** Triage recommendations in the feedback-triage 1.1.0 item enum. */
export const FEEDBACK_TRIAGE_RECOMMENDATIONS = ["accept", "author-decides"] as const

/** A triage recommendation in the feedback-triage 1.1.0 item enum. */
export type FeedbackTriageRecommendation = (typeof FEEDBACK_TRIAGE_RECOMMENDATIONS)[number]

/** Feedback-triage artifact directory, relative to the repository root. */
export const FEEDBACK_TRIAGE_ARTIFACT_DIR = ".uf/artifacts/feedback-triage"

/** Producer identity stamped on every feedback-triage artifact envelope. */
export const FEEDBACK_TRIAGE_ARTIFACT_HERO = "cobalt-crush"

/** Envelope format version stamped on every feedback-triage artifact. */
export const FEEDBACK_TRIAGE_ENVELOPE_VERSION = "1.0.0"

/** Artifact type discriminator stamped on every feedback-triage artifact. */
export const FEEDBACK_TRIAGE_ARTIFACT_TYPE = "feedback-triage"

/** Payload schema version the artifact write renders and validates. */
export const FEEDBACK_TRIAGE_SCHEMA_VERSION = "1.1.0"

const REPO_PATTERN = /^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/

/** GraphQL query mapping every review comment to its thread node ID. */
export const REVIEW_THREADS_QUERY =
  "query($owner: String!, $name: String!, $number: Int!, $cursor: String) " +
  "{ repository(owner: $owner, name: $name) { pullRequest(number: $number) " +
  "{ reviewThreads(first: 100, after: $cursor) { pageInfo { hasNextPage endCursor } " +
  "nodes { id isResolved comments(first: 100) { nodes { databaseId } } } } } } }"

/** GraphQL mutation resolving one review thread. */
export const RESOLVE_THREAD_MUTATION =
  "mutation($threadId: ID!) { resolveReviewThread(input: { threadId: $threadId }) " +
  "{ thread { id isResolved } } }"

// ---------------------------------------------------------------------------
// Input schemas (the tool args are these shapes — input-schema parity)
// ---------------------------------------------------------------------------

const PrNumberSchema = z.number().int().min(1).max(999_999)
const RepoSchema = z.string().regex(REPO_PATTERN, "expected owner/repo")
const CommentIdSchema = z.number().int().min(1).max(Number.MAX_SAFE_INTEGER)
const BodySchema = z.string().max(MAX_BODY_CHARS).refine((value) => value.trim() !== "", {
  message: "must not be blank",
})
const TitleSchema = z.string().max(MAX_TITLE_CHARS).refine((value) => value.trim() !== "", {
  message: "must not be blank",
})

/** Issue payload for a FILE item: the finding's title and body. */
export const IssuePayloadSchema = z
  .object({
    title: TitleSchema.describe("Issue title: the finding summary"),
    body: BodySchema.describe(
      "Issue body: the finding, its severity, file:line, and a link to the review comment",
    ),
  })
  .strict()

/** One reply to post. */
export const ReplyItemSchema = z
  .object({
    comment_kind: z
      .enum(COMMENT_KINDS)
      .describe(
        "review: an inline review thread (reply goes to the thread); issue: a PR conversation comment (reply goes to the PR conversation)",
      ),
    comment_id: CommentIdSchema.describe(
      "REST id of the comment being answered: the thread's top-level review comment, or the issue comment",
    ),
    decision: z.enum(REPLY_DECISIONS).describe("The author's triage decision for this item"),
    body: BodySchema.describe(
      "The composed reply text. For file items, the tool prefixes \"Filed as #<n>: \" once the issue exists",
    ),
    issue: IssuePayloadSchema.optional().describe(
      "Required for file items, forbidden otherwise: the issue to file before the reply",
    ),
  })
  .strict()
  .superRefine((item, ctx) => {
    if (item.decision === "file" && item.issue === undefined) {
      ctx.addIssue({ code: "custom", path: ["issue"], message: "file items require an issue payload" })
    }
    if (item.decision !== "file" && item.issue !== undefined) {
      ctx.addIssue({ code: "custom", path: ["issue"], message: "only file items carry an issue payload" })
    }
  })

/** One reply item after validation. */
export type ReplyItem = z.infer<typeof ReplyItemSchema>

/** post_feedback_replies input. */
export const PostFeedbackRepliesInputSchema = z
  .object({
    pr_number: PrNumberSchema.describe("Pull request number"),
    repo: RepoSchema.optional().describe(
      "owner/repo of the PR; defaults to `gh repo view` in the working tree",
    ),
    resolve_threads: z
      .boolean()
      .describe(
        "The author's resolve decision: true resolves the threads of accept, modify, file, and reply review items once their replies are posted",
      ),
    items: z
      .array(ReplyItemSchema)
      .min(1)
      .max(200)
      .describe("Confirmed replies in posting order, one per feedback item"),
  })
  .strict()
  .superRefine((input, ctx) => {
    const seen = new Set<string>()
    input.items.forEach((item, index) => {
      const key = itemKey(item.comment_kind, item.comment_id)
      if (seen.has(key)) {
        ctx.addIssue({ code: "custom", path: ["items", index], message: `duplicate item ${key}` })
      }
      seen.add(key)
    })
  })

/** post_feedback_replies input after validation. */
export type PostFeedbackRepliesInput = z.infer<typeof PostFeedbackRepliesInputSchema>

/** file_feedback_issue input. */
export const FileFeedbackIssueInputSchema = z
  .object({
    pr_number: PrNumberSchema.describe("Pull request number the finding came from"),
    repo: RepoSchema.optional().describe(
      "owner/repo to file in; defaults to `gh repo view` in the working tree",
    ),
    comment_kind: z.enum(COMMENT_KINDS).describe("Kind of the comment that raised the finding"),
    comment_id: CommentIdSchema.describe("REST id of the comment that raised the finding (the cache key)"),
    title: TitleSchema.describe("Issue title"),
    body: BodySchema.describe("Issue body"),
  })
  .strict()

/** file_feedback_issue input after validation. */
export type FileFeedbackIssueInput = z.infer<typeof FileFeedbackIssueInputSchema>

/** post_issue_comment input (the /uf.triage-issue 4.3 DE-005 extraction). */
export const PostIssueCommentInputSchema = z
  .object({
    repo: RepoSchema.optional().describe(
      "owner/repo of the issue; defaults to `gh repo view` in the working tree",
    ),
    issue_number: PrNumberSchema.describe("Issue number to post the comment on"),
    body: BodySchema.describe("Confirmed comment body, exactly as the author approved it"),
  })
  .strict()

/** post_issue_comment input after validation. */
export type PostIssueCommentInput = z.infer<typeof PostIssueCommentInputSchema>

/** create_child_issue input (the /uf.triage-issue 4.4 DE-005 extraction). */
export const CreateChildIssueInputSchema = z
  .object({
    repo: RepoSchema.optional().describe(
      "owner/repo to create the issue in; defaults to `gh repo view` in the working tree",
    ),
    title: TitleSchema.describe("Child issue title"),
    body: BodySchema.describe("Confirmed child issue body, exactly as the author approved it"),
  })
  .strict()

/** create_child_issue input after validation. */
export type CreateChildIssueInput = z.infer<typeof CreateChildIssueInputSchema>

/** A git commit SHA, short (7 hex) or full (40 hex), as git prints it. */
const CommitShaSchema = z.string().regex(/^[0-9a-f]{7,40}$/i, "expected a git commit SHA")

/**
 * One triage item for the artifact write (design D2): the
 * feedback-triage 1.1.0 item fields plus the `comment_kind`/
 * `comment_id` identity Section 4.6 already passes to
 * `post_feedback_replies`. The identity is join input only — the 1.1.0
 * payload is `additionalProperties: false`, so the render strips it —
 * and `filed_issue_number`/`filed_issue_url` are join outputs, never
 * input: they come from the `replies_result` projection.
 */
export const FeedbackTriageItemSchema = z
  .object({
    comment_kind: z
      .enum(COMMENT_KINDS)
      .describe("Kind of the comment this item triages: an inline review thread or a PR conversation comment"),
    comment_id: CommentIdSchema.describe(
      "REST id of the triaged comment — the join identity Section 4.6 passes to post_feedback_replies",
    ),
    thread_id: z.string().min(1).max(256).describe("Unique identifier for the review thread"),
    reviewer: z.string().min(1).max(256).describe("GitHub username of the reviewer"),
    reviewer_role: z.enum(FEEDBACK_TRIAGE_REVIEWER_ROLES).describe("Role of the reviewer in the repository"),
    file: z
      .string()
      .min(1)
      .max(1024)
      .nullable()
      .default(null)
      .describe("File path the feedback applies to; null for general PR comments"),
    line: z
      .number()
      .int()
      .min(1)
      .max(Number.MAX_SAFE_INTEGER)
      .nullable()
      .default(null)
      .describe("Line number the feedback applies to; null for general PR comments"),
    classification: z
      .enum(FEEDBACK_TRIAGE_CLASSIFICATIONS)
      .describe("Whether the feedback is data-driven or subjective"),
    tier: z
      .number()
      .int()
      .min(1)
      .max(2)
      .describe("Priority tier of the feedback item; the triage model has exactly two tiers"),
    evidence: z.array(z.string().min(1).max(2048)).max(100).describe("Supporting evidence for the classification"),
    recommendation: z.enum(FEEDBACK_TRIAGE_RECOMMENDATIONS).describe("Triage recommendation for the feedback item"),
    decision: z.enum(REPLY_DECISIONS).describe("The author's final decision on the feedback item"),
    decision_reasoning: z
      .string()
      .min(1)
      .max(8192)
      .nullable()
      .default(null)
      .describe("Reasoning behind the decision; null when none was recorded"),
    commit_sha: CommitShaSchema.nullable().default(null).describe(
      "Commit SHA the feedback references; null when not applicable",
    ),
    divisor_agents_used: z.array(z.string().min(1).max(128)).max(100).describe(
      "Divisor agents consulted for this item",
    ),
    tier2_unavailable: z.boolean().default(false).describe("Whether tier 2 analysis was unavailable"),
    conflict_flag: z.boolean().default(false).describe("Whether this item conflicts with another item"),
  })
  .strict()

/** One triage item after validation. */
export type FeedbackTriageItem = z.infer<typeof FeedbackTriageItemSchema>

/**
 * Projection of one `post_feedback_replies` item result (design D2):
 * the consumed fields, pinned so an evolved result that renames one
 * fails closed as `invalid_input` instead of silently mis-joining.
 * Unknown fields are ignored, so result additions still validate. The
 * join itself matches on the derived `itemKey(comment_kind,
 * comment_id)`; `key` is pinned for shape parity with the result
 * contract, not used for routing.
 */
export const RepliesResultItemProjectionSchema = z
  .object({
    key: z.string().min(1).max(256),
    comment_kind: z.enum(COMMENT_KINDS),
    comment_id: CommentIdSchema,
    decision: z.enum(REPLY_DECISIONS),
    issue_number: z.number().int().min(1).max(Number.MAX_SAFE_INTEGER).nullable(),
    issue_url: z.string().min(1).max(2048).nullable(),
  })
  .passthrough()

/** One projected reply outcome after validation. */
export type RepliesResultItemProjection = z.infer<typeof RepliesResultItemProjectionSchema>

/**
 * Projection of the `post_feedback_replies` success data (design D2):
 * only the `items` array is consumed; every other result field
 * (pr_number, repo, counts, ...) is ignored so an evolved result
 * still validates.
 */
export const RepliesResultProjectionSchema = z
  .object({
    items: z.array(RepliesResultItemProjectionSchema).max(1000),
  })
  .passthrough()

/** The projected replies result after validation. */
export type RepliesResultProjection = z.infer<typeof RepliesResultProjectionSchema>

/**
 * write_feedback_triage_artifact input (design D2): the judgment
 * products of the triage — PR identity, artifact context, and the
 * per-item records carrying their Section 4.6 comment identity —
 * plus, when replies were posted, the `post_feedback_replies` success
 * data as a projection. Optional because the Section 4.5 "No — skip
 * posting" path skips 4.6 entirely and the artifact still records the
 * triage decisions. The `superRefine` enforces the mechanical joins:
 * unique comment identities (a duplicate would make the join
 * ambiguous) and the decision cross-check against the reply outcomes
 * — a mismatch means the caller joined the wrong run's result, and it
 * fails before anything is written.
 */
export const WriteFeedbackTriageArtifactInputSchema = z
  .object({
    pr_number: PrNumberSchema.describe("Pull request number"),
    pr_url: z
      .string()
      .regex(/^https:\/\/\S+$/, "expected an https URL")
      .max(2048)
      .describe("Pull request URL"),
    branch: z.string().min(1).max(1024).describe("Branch name the PR targets or originates from"),
    commit: CommitShaSchema.describe(
      "Current HEAD SHA at call time — after fixes are committed, or the pre-fix HEAD when no fixes were needed; never the frozen ingest-time head_sha after fixes",
    ),
    backlog_item_id: z.string().min(1).max(256).describe("Linked issue number or PR-N"),
    items: z
      .array(FeedbackTriageItemSchema)
      .min(1)
      .max(200)
      .describe("Triage items in the order the command presents them (Section 4.6 posting order)"),
    replies_result: RepliesResultProjectionSchema.optional().describe(
      "The post_feedback_replies success data; omitted when posting was skipped",
    ),
  })
  .strict()
  .superRefine((input, ctx) => {
    const seen = new Set<string>()
    input.items.forEach((item, index) => {
      const key = itemKey(item.comment_kind, item.comment_id)
      if (seen.has(key)) {
        ctx.addIssue({ code: "custom", path: ["items", index], message: `duplicate item ${key}` })
      }
      seen.add(key)
    })
    if (input.replies_result === undefined) return
    const outcomes = new Map(
      input.replies_result.items.map((outcome) => [itemKey(outcome.comment_kind, outcome.comment_id), outcome]),
    )
    input.items.forEach((item, index) => {
      const outcome = outcomes.get(itemKey(item.comment_kind, item.comment_id))
      if (outcome !== undefined && outcome.decision !== item.decision) {
        ctx.addIssue({
          code: "custom",
          path: ["items", index],
          message:
            `decision mismatch for ${itemKey(item.comment_kind, item.comment_id)}: ` +
            `the replies result recorded "${outcome.decision}", the item carries "${item.decision}"`,
        })
      }
    })
  })

/** write_feedback_triage_artifact input after validation. */
export type WriteFeedbackTriageArtifactInput = z.infer<typeof WriteFeedbackTriageArtifactInputSchema>

// ---------------------------------------------------------------------------
// Output shapes (documented in the command; pinned by golden tests)
// ---------------------------------------------------------------------------

/** Structured error, matching the fetch-tool contract (code / message / retryable). */
export interface FeedbackError {
  readonly code: string
  readonly message: string
  readonly retryable: boolean
}

/**
 * Whole-call failure envelope. It carries both the uf-workflow
 * discriminant (`status: "error"`) and the fetch-tool discriminant
 * (`error: true`), so consumers of either contract branch correctly.
 */
export interface FeedbackFailure extends FeedbackError {
  readonly status: "error"
  readonly error: true
}

/** Success envelope. */
export interface FeedbackSuccess<T> {
  readonly status: "ok"
  readonly data: T
}

/** Reply outcome for one item. */
export type ReplyOutcome = "posted" | "already_posted" | "pending" | "post_failed" | "issue_failed"

/** Issue outcome for one item (null for non-file items). */
export type IssueOutcome = "filed" | "deduplicated" | "failed" | null

/** Thread outcome for one item. */
export type ThreadOutcome =
  | "resolved"
  | "already_resolved"
  | "not_requested"
  | "not_applicable"
  | "reply_not_posted"
  | "thread_not_found"
  | "failed"

/** Per-item result of post_feedback_replies. */
export interface ReplyItemResult {
  readonly key: string
  readonly comment_kind: CommentKind
  readonly comment_id: number
  readonly decision: ReplyDecision
  readonly reply: ReplyOutcome
  readonly reply_id: number | null
  readonly reply_url: string | null
  readonly issue: IssueOutcome
  readonly issue_number: number | null
  readonly issue_url: string | null
  readonly thread: ThreadOutcome
  readonly thread_id: string | null
  readonly error: FeedbackError | null
}

/** post_feedback_replies success data. */
export interface PostFeedbackRepliesData {
  readonly pr_number: number
  readonly repo: string
  readonly complete: boolean
  readonly counts: {
    readonly total: number
    readonly posted: number
    readonly pending: number
    readonly failed: number
    readonly issues_filed: number
    readonly threads_resolved: number
    readonly threads_resolvable: number
  }
  readonly items: readonly ReplyItemResult[]
  readonly checkpoint_recorded: boolean
  readonly summary: string
  readonly warnings: readonly string[]
}

/** file_feedback_issue success data. */
export interface FileFeedbackIssueData {
  readonly pr_number: number
  readonly repo: string
  readonly key: string
  readonly issue_number: number
  readonly issue_url: string
  readonly deduplicated: boolean
  readonly attempts: number
}

/** post_issue_comment success data. */
export interface PostIssueCommentData {
  readonly repo: string
  readonly issue_number: number
  readonly comment_id: number | null
  readonly comment_url: string | null
}

/** create_child_issue success data. */
export interface CreateChildIssueData {
  readonly repo: string
  readonly issue_number: number
  readonly issue_url: string
  readonly attempts: number
}

/**
 * One rendered payload item: every feedback-triage 1.1.0 item field
 * present (nullable fields as null, never absent), with the
 * `comment_kind`/`comment_id` join identity stripped — the 1.1.0
 * schema is `additionalProperties: false`.
 */
export interface FeedbackTriagePayloadItem {
  readonly thread_id: string
  readonly reviewer: string
  readonly reviewer_role: FeedbackTriageReviewerRole
  readonly file: string | null
  readonly line: number | null
  readonly classification: FeedbackTriageClassification
  readonly tier: number
  readonly evidence: readonly string[]
  readonly recommendation: FeedbackTriageRecommendation
  readonly decision: ReplyDecision
  readonly decision_reasoning: string | null
  readonly commit_sha: string | null
  readonly filed_issue_number: number | null
  readonly filed_issue_url: string | null
  readonly divisor_agents_used: readonly string[]
  readonly tier2_unavailable: boolean
  readonly conflict_flag: boolean
}

/**
 * The derived summary counts (design D4): never passed in, always
 * counted from the items. `filed` and `replied` are always present in
 * the render even though the 1.1.0 schema marks them optional — the
 * Section 4.7 envelope template always showed them.
 */
export interface FeedbackTriageSummaryCounts {
  readonly total_items: number
  readonly accepted: number
  readonly modified: number
  readonly rejected: number
  readonly asked: number
  readonly filed: number
  readonly replied: number
  readonly tier1_count: number
  readonly tier2_count: number
  readonly divisor_agents_invoked: readonly string[]
}

/**
 * The feedback-triage artifact envelope: the standard schema wrapper
 * (producer identity, envelope version, timestamp, artifact type,
 * schema version, context) around the 1.1.0 payload. Field names and
 * order match the Section 4.7 envelope the hand-written artifacts
 * used, so the bytes are unchanged for the same inputs.
 */
export interface FeedbackTriageArtifactEnvelope {
  readonly hero: "cobalt-crush"
  readonly version: "1.0.0"
  readonly timestamp: string
  readonly artifact_type: "feedback-triage"
  readonly schema_version: "1.1.0"
  readonly context: {
    readonly branch: string
    readonly commit: string
    readonly backlog_item_id: string
  }
  readonly payload: {
    readonly pr_number: number
    readonly pr_url: string
    readonly branch: string
    readonly round: number
    readonly items: readonly FeedbackTriagePayloadItem[]
    readonly summary: FeedbackTriageSummaryCounts
  }
}

/** write_feedback_triage_artifact success data. */
export interface WriteFeedbackTriageArtifactData {
  /** Pull request the artifact was written for. */
  readonly pr_number: number
  /** Absolute path of the written artifact. */
  readonly path: string
  /** Round number the scan derived and the artifact recorded. */
  readonly round: number
  /** Byte size of the written artifact (UTF-8, including the newline). */
  readonly bytes: number
  /** The derived summary counts, as rendered into the payload. */
  readonly counts: FeedbackTriageSummaryCounts
  /** Whether the `uf.address-feedback` progress state was cleared. */
  readonly progress_cleared: boolean
  /** Non-fatal warnings (a failed progress clear after a durable write). */
  readonly warnings: readonly string[]
  /** The one-line outcome the command prints verbatim (DE-003). */
  readonly summary: string
}

/** Persisted per-item record under `state.replies[<key>]`. */
export interface ReplyRecord {
  comment_kind: CommentKind
  comment_id: number
  decision: ReplyDecision
  issue_number: number | null
  issue_url: string | null
  comment_posted: boolean
  reply_id: number | null
  reply_url: string | null
  thread_id: string | null
  thread_resolved: boolean
  updated_at: string
}

/** Dependencies of the feedback reply and artifact operations. */
export interface FeedbackReplyDeps {
  /** gh executor (argument array, no shell). */
  readonly gh: FeedbackGhExec
  /** Repository root: gh runs here and the feedback cache lives under it. */
  readonly cwd: string
  /** Clock for record and artifact timestamps. */
  readonly now?: () => Date
  /** Records a progress checkpoint for `uf.address-feedback`; throws on failure. */
  readonly recordCheckpoint?: (step: string, data: Record<string, unknown>) => void
  /**
   * Clears the `uf.address-feedback` progress state after the artifact
   * write (design D6); throws on failure. Wired by the factory to the
   * same branch-keyed state path the `clear_progress` tool uses.
   */
  readonly clearProgress?: () => void
}

// ---------------------------------------------------------------------------
// Envelope helpers
// ---------------------------------------------------------------------------

/** Build the whole-call failure envelope. */
export function feedbackFailure(code: string, message: string, retryable = false): FeedbackFailure {
  return { status: "error", error: true, code, message, retryable }
}

/** Build a success envelope. */
export function feedbackSuccess<T>(data: T): FeedbackSuccess<T> {
  return { status: "ok", data }
}

/** Typed failure carried through the call stack to the tool boundary. */
export class FeedbackOperationError extends Error {
  readonly code: string
  readonly retryable: boolean

  constructor(code: string, message: string, retryable = false) {
    super(message)
    this.name = "FeedbackOperationError"
    this.code = code
    this.retryable = retryable
  }

  /** The structured error view of this failure. */
  toError(): FeedbackError {
    return { code: this.code, message: this.message, retryable: this.retryable }
  }
}

/** Text of a value that may be a Buffer, string, or absent. */
function textOf(value: unknown): string {
  if (typeof value === "string") return value
  if (value instanceof Uint8Array) return Buffer.from(value).toString("utf-8")
  return ""
}

/**
 * Classify a thrown gh failure into the fetch-tool error codes:
 * `gh_not_found` (executable missing), `timeout`, `rate_limited`,
 * `gh_auth_failed`, or `api_error`. Timeouts and rate limits are
 * retryable; everything else is not.
 *
 * The decision table lives in the shared `classifyGhFailure` module
 * (`gh-error.ts`) so review-dispatch classifies identical failures
 * identically (FRR-FR-001, issue #1023 MEDIUM 3); this adapter
 * extracts the signals from the thrown record and maps the shared
 * codes onto this module's message templates.
 */
export function classifyGhError(err: unknown): FeedbackError {
  if (err instanceof FeedbackOperationError) return err.toError()
  const record = (typeof err === "object" && err !== null ? err : {}) as Record<string, unknown>
  const message = err instanceof Error ? err.message : String(err)
  const stderr = textOf(record.stderr).trim()
  const detail = stderr !== "" ? stderr : message
  const { code, retryable } = classifyGhFailure({
    stderr,
    message,
    // execFileSync reports the gh exit code as `status`.
    exitCode: typeof record.status === "number" ? record.status : undefined,
    spawnCode: typeof record.code === "string" ? record.code : undefined,
    signal: typeof record.signal === "string" ? record.signal : undefined,
  })
  switch (code) {
    case "gh_not_found":
      return { code, message: "gh CLI is not installed or not on PATH", retryable }
    case "timeout":
      return { code, message: `gh CLI timed out: ${detail}`, retryable }
    case "rate_limited":
      return { code, message: `GitHub API rate limited: ${detail}`, retryable }
    case "gh_auth_failed":
      return { code, message: `gh CLI is not authenticated: ${detail}`, retryable }
  }
  return { code: "api_error", message: `gh CLI failed: ${detail}`, retryable }
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

/** Cache key of one item: `<comment_kind>:<comment_id>`. */
export function itemKey(kind: CommentKind, commentId: number): string {
  return `${kind}:${commentId}`
}

/**
 * REST endpoint a reply posts to: the review-comment replies endpoint
 * for an inline thread, the issue-comments endpoint for a PR
 * conversation comment.
 */
export function replyEndpoint(repo: string, prNumber: number, item: Pick<ReplyItem, "comment_kind" | "comment_id">): string {
  if (item.comment_kind === "review") {
    return `repos/${repo}/pulls/${prNumber}/comments/${item.comment_id}/replies`
  }
  return `repos/${repo}/issues/${prNumber}/comments`
}

/** Final reply text: FILE replies lead with the filed issue reference. */
export function renderReplyBody(item: Pick<ReplyItem, "decision" | "body">, issueNumber: number | null): string {
  if (item.decision === "file" && issueNumber !== null) {
    return `Filed as #${issueNumber}: ${item.body}`
  }
  return item.body
}

/** Parse `gh issue create` output (the new issue's URL) into number and URL. */
export function parseIssueUrl(output: string): { number: number; url: string } | null {
  const match = /(https?:\/\/\S+\/issues\/(\d+))/.exec(output)
  if (match === null) return null
  const number = Number(match[2])
  if (!Number.isSafeInteger(number) || number < 1) return null
  return { number, url: match[1] }
}

/**
 * Render the one-line outcome the command shows (DE-003: tool-rendered,
 * not reconstructed from a template in prose).
 */
export function renderReplySummary(prNumber: number, items: readonly ReplyItemResult[], resolveThreads: boolean): string {
  const total = items.length
  const posted = items.filter((item) => item.reply === "posted" || item.reply === "already_posted").length
  const parts = [`Posted ${posted} of ${total} reply comments.`]
  const unposted = items
    .map((item, index) => ({ item, position: index + 1 }))
    .filter(({ item }) => item.reply !== "posted" && item.reply !== "already_posted")
  if (unposted.length > 0) {
    parts.push(`Items ${unposted.map(({ position }) => position).join(", ")} not posted.`)
  }
  const resolvable = items.filter((item) => isResolutionTarget(item))
  const resolved = resolvable.filter((item) => item.thread === "resolved" || item.thread === "already_resolved")
  if (resolveThreads) {
    parts.push(`Resolved ${resolved.length} of ${resolvable.length} addressed threads.`)
  } else {
    parts.push("Threads left open.")
  }
  const incomplete =
    unposted.length > 0 || (resolveThreads && resolved.length < resolvable.length)
  if (incomplete) {
    parts.push(`Re-run \`/uf.address-feedback ${prNumber}\` to retry.`)
  }
  return parts.join(" ")
}

/** Whether an item result is a thread the resolve decision applies to. */
function isResolutionTarget(item: Pick<ReplyItemResult, "comment_kind" | "decision">): boolean {
  return item.comment_kind === "review" && RESOLVABLE_DECISIONS.has(item.decision)
}

/** Validate an owner/repo string, or throw a FeedbackOperationError. */
function requireRepo(value: string): string {
  const trimmed = value.trim()
  if (!REPO_PATTERN.test(trimmed)) {
    throw new FeedbackOperationError(
      "repo_unresolved",
      `could not resolve the repository: gh returned ${JSON.stringify(trimmed.slice(0, 200))}`,
    )
  }
  return trimmed
}

/** Resolve owner/repo from the explicit argument or `gh repo view`. */
export function resolveRepo(deps: FeedbackReplyDeps, explicit: string | undefined): string {
  if (explicit !== undefined) return explicit
  let output: string
  try {
    output = deps.gh(["repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"], deps.cwd)
  } catch (err: unknown) {
    const classified = classifyGhError(err)
    throw new FeedbackOperationError(classified.code, classified.message, classified.retryable)
  }
  return requireRepo(output)
}

// ---------------------------------------------------------------------------
// Feedback cache (`.uf/feedback/pr-<N>/state.json`)
// ---------------------------------------------------------------------------

/** Absolute path of a PR's feedback cache file. */
export function feedbackStatePath(cwd: string, prNumber: number): string {
  return resolve(cwd, ".uf", "feedback", `pr-${prNumber}`, "state.json")
}

/** Type guard for a plain JSON object. */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

/** Whether an error is ENOENT. */
function isEnoent(err: unknown): boolean {
  return isPlainObject(err) && err.code === "ENOENT"
}

/**
 * The feedback cache: the whole state document, preserved as-is, plus
 * the tool-owned `replies` map. Other keys (assessment results the
 * command writes in Section 2.7) are never touched.
 */
export interface FeedbackState {
  readonly document: Record<string, unknown>
  readonly replies: Record<string, ReplyRecord>
}

/** Read the feedback cache. A missing file is an empty cache. */
export function readFeedbackState(cwd: string, prNumber: number): FeedbackState {
  const path = feedbackStatePath(cwd, prNumber)
  let raw: string
  try {
    if (lstatSync(path).isSymbolicLink()) {
      throw new FeedbackOperationError("state_error", "refusing to read a symlinked feedback cache")
    }
    raw = readFileSync(path, "utf-8")
  } catch (err: unknown) {
    if (isEnoent(err)) return { document: {}, replies: {} }
    if (err instanceof FeedbackOperationError) throw err
    throw new FeedbackOperationError("state_error", `could not read the feedback cache: ${String(err)}`)
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    throw new FeedbackOperationError(
      "state_error",
      `the feedback cache is not valid JSON; delete .uf/feedback/pr-${prNumber}/state.json and re-run`,
    )
  }
  if (!isPlainObject(parsed)) {
    throw new FeedbackOperationError("state_error", "the feedback cache is not a JSON object")
  }
  const replies = parsed[STATE_REPLIES_KEY]
  if (replies !== undefined && !isPlainObject(replies)) {
    throw new FeedbackOperationError("state_error", `the feedback cache "${STATE_REPLIES_KEY}" entry is not an object`)
  }
  return { document: parsed, replies: (replies ?? {}) as Record<string, ReplyRecord> }
}

/**
 * Write the feedback cache atomically (temp file, then rename) with
 * `0o600` files under `0o700` directories, refusing symlinked targets.
 */
export function writeFeedbackState(cwd: string, prNumber: number, state: FeedbackState): void {
  const path = feedbackStatePath(cwd, prNumber)
  const dir = resolve(path, "..")
  try {
    mkdirSync(dir, { recursive: true, mode: 0o700 })
    if (lstatSync(dir).isSymbolicLink()) {
      throw new FeedbackOperationError("state_error", "refusing to write into a symlinked feedback cache directory")
    }
    try {
      if (lstatSync(path).isSymbolicLink()) {
        throw new FeedbackOperationError("state_error", "refusing to overwrite a symlinked feedback cache")
      }
    } catch (err: unknown) {
      if (!isEnoent(err)) throw err
    }
    const document = { ...state.document, [STATE_REPLIES_KEY]: state.replies }
    const temp = `${path}.${process.pid}.${Date.now()}.tmp`
    writeFileSync(temp, `${JSON.stringify(document, null, 2)}\n`, { mode: 0o600 })
    renameSync(temp, path)
  } catch (err: unknown) {
    if (err instanceof FeedbackOperationError) throw err
    throw new FeedbackOperationError("state_error", `could not write the feedback cache: ${String(err)}`)
  }
}

// ---------------------------------------------------------------------------
// Feedback-triage artifact (`.uf/artifacts/feedback-triage/`)
// ---------------------------------------------------------------------------

/**
 * Payload item mirror of `schemas/feedback-triage/v1.1.0.schema.json`:
 * the required fields, the four enums, the nullable fields, and
 * `additionalProperties: false` (`.strict()`). Optional fields are
 * `.optional()`, not defaulted — the mirror validates artifacts as
 * they are rendered or read, not tool input. Parity is pinned
 * transitively (design D4): the vitest suite runs the checked-in
 * samples through this schema, and the Go CI test validates the
 * samples against the JSON Schema.
 */
export const FeedbackTriagePayloadItemSchema = z
  .object({
    thread_id: z.string(),
    reviewer: z.string(),
    reviewer_role: z.enum(FEEDBACK_TRIAGE_REVIEWER_ROLES),
    file: z.string().nullable().optional(),
    line: z.number().int().nullable().optional(),
    classification: z.enum(FEEDBACK_TRIAGE_CLASSIFICATIONS),
    tier: z.number().int(),
    evidence: z.array(z.string()),
    recommendation: z.enum(FEEDBACK_TRIAGE_RECOMMENDATIONS),
    decision: z.enum(REPLY_DECISIONS),
    decision_reasoning: z.string().nullable().optional(),
    commit_sha: z.string().nullable().optional(),
    filed_issue_number: z.number().int().nullable().optional(),
    filed_issue_url: z.string().nullable().optional(),
    divisor_agents_used: z.array(z.string()),
    tier2_unavailable: z.boolean().optional(),
    conflict_flag: z.boolean().optional(),
  })
  .strict()

/**
 * Payload summary mirror of the 1.1.0 summary schema: `filed` and
 * `replied` are optional (the 1.0.0 sample omits them and still
 * validates), everything else required, `additionalProperties: false`.
 */
export const FeedbackTriageSummaryPayloadSchema = z
  .object({
    total_items: z.number().int(),
    accepted: z.number().int(),
    modified: z.number().int(),
    rejected: z.number().int(),
    asked: z.number().int(),
    filed: z.number().int().optional(),
    replied: z.number().int().optional(),
    tier1_count: z.number().int(),
    tier2_count: z.number().int(),
    divisor_agents_invoked: z.array(z.string()),
  })
  .strict()

/**
 * The payload zod mirror of `schemas/feedback-triage/v1.1.0.schema.json`
 * (design D4). A rendered artifact that fails this validation is never
 * written; the mirror is the last gate before the atomic write.
 */
export const FeedbackTriagePayloadSchema = z
  .object({
    pr_number: z.number().int(),
    pr_url: z.string(),
    branch: z.string(),
    round: z.number().int(),
    items: z.array(FeedbackTriagePayloadItemSchema),
    summary: FeedbackTriageSummaryPayloadSchema,
  })
  .strict()

/** Absolute path of one PR's feedback-triage artifact round. */
export function feedbackTriageArtifactPath(cwd: string, prNumber: number, round: number): string {
  return resolve(cwd, FEEDBACK_TRIAGE_ARTIFACT_DIR, `pr-${prNumber}-round-${round}.json`)
}

/**
 * The next round number for a PR (design D3): the highest existing
 * `pr-<N>-round-<M>.json` plus one — not a file count, so gaps from
 * deleted files are handled. Unparsable names (including the temp
 * files of interrupted writes, which the anchored pattern cannot
 * match) are ignored; a missing directory is round 1, not an error;
 * any other read failure fails closed.
 *
 * @param cwd Repository root the artifact directory resolves against.
 * @param prNumber The PR the round number is derived for.
 * @returns The next round number, at least 1.
 */
export function nextRoundNumber(cwd: string, prNumber: number): number {
  const pattern = new RegExp(`^pr-${prNumber}-round-(\\d+)\\.json$`)
  let entries: string[]
  try {
    entries = readdirSync(resolve(cwd, FEEDBACK_TRIAGE_ARTIFACT_DIR))
  } catch (err: unknown) {
    if (isEnoent(err)) return 1
    throw new FeedbackOperationError(
      "artifact_error",
      `could not scan ${FEEDBACK_TRIAGE_ARTIFACT_DIR}/ for the next round: ${String(err)}`,
    )
  }
  let max = 0
  for (const name of entries) {
    const match = pattern.exec(name)
    if (match === null) continue
    const round = Number(match[1])
    if (round > max) max = round
  }
  return max + 1
}

/** The joined issue identity for one FILE item: what the reply run reported. */
export interface JoinedFiledIssue {
  readonly number: number | null
  readonly url: string | null
}

/**
 * Join the reply outcomes to the FILE items by comment identity
 * (design D2, DE-006). Each FILE item's `filed_issue_number`/
 * `filed_issue_url` come from the matching outcome — null when its
 * issue failed or the replies result is absent, recording reality
 * rather than an error. Every other item renders null for both, so
 * only FILE items appear in the map.
 *
 * @param input The validated artifact input, with its optional replies projection.
 * @returns FILE-item keys (`<comment_kind>:<comment_id>`) to their joined issue identity.
 */
export function joinFiledIssues(input: WriteFeedbackTriageArtifactInput): ReadonlyMap<string, JoinedFiledIssue> {
  const outcomes = new Map<string, RepliesResultItemProjection>()
  if (input.replies_result !== undefined) {
    for (const outcome of input.replies_result.items) {
      outcomes.set(itemKey(outcome.comment_kind, outcome.comment_id), outcome)
    }
  }
  const joined = new Map<string, JoinedFiledIssue>()
  for (const item of input.items) {
    if (item.decision !== "file") continue
    const key = itemKey(item.comment_kind, item.comment_id)
    const outcome = outcomes.get(key)
    joined.set(key, { number: outcome?.issue_number ?? null, url: outcome?.issue_url ?? null })
  }
  return joined
}

/**
 * Render the feedback-triage artifact envelope (design D4): the
 * envelope constants, the FILE-item join, the derived summary counts,
 * and the sorted `divisor_agents_invoked` union, with the
 * `comment_kind`/`comment_id` identity stripped per
 * `additionalProperties: false`. Items render in input order — the
 * order the command presents them, matching Section 4.6's posting
 * order. The envelope is validated by construction; the payload is
 * validated separately against the 1.1.0 mirror before anything is
 * written.
 *
 * @param input The validated artifact input.
 * @param round The round number the directory scan derived.
 * @param timestamp ISO 8601 write-time stamp for the envelope.
 * @returns The complete artifact envelope, ready to serialize.
 */
export function renderFeedbackTriageArtifact(
  input: WriteFeedbackTriageArtifactInput,
  round: number,
  timestamp: string,
): FeedbackTriageArtifactEnvelope {
  const joined = joinFiledIssues(input)
  const items: FeedbackTriagePayloadItem[] = input.items.map((item) => {
    const issue = item.decision === "file" ? joined.get(itemKey(item.comment_kind, item.comment_id)) : undefined
    return {
      thread_id: item.thread_id,
      reviewer: item.reviewer,
      reviewer_role: item.reviewer_role,
      file: item.file,
      line: item.line,
      classification: item.classification,
      tier: item.tier,
      evidence: item.evidence,
      recommendation: item.recommendation,
      decision: item.decision,
      decision_reasoning: item.decision_reasoning,
      commit_sha: item.commit_sha,
      filed_issue_number: issue?.number ?? null,
      filed_issue_url: issue?.url ?? null,
      divisor_agents_used: item.divisor_agents_used,
      tier2_unavailable: item.tier2_unavailable,
      conflict_flag: item.conflict_flag,
    }
  })
  const countDecision = (decision: ReplyDecision): number => items.filter((item) => item.decision === decision).length
  const summary: FeedbackTriageSummaryCounts = {
    total_items: items.length,
    accepted: countDecision("accept"),
    modified: countDecision("modify"),
    rejected: countDecision("reject"),
    asked: countDecision("ask"),
    filed: countDecision("file"),
    replied: countDecision("reply"),
    tier1_count: items.filter((item) => item.tier === 1).length,
    tier2_count: items.filter((item) => item.tier === 2).length,
    divisor_agents_invoked: [...new Set(items.flatMap((item) => item.divisor_agents_used))].sort(),
  }
  return {
    hero: FEEDBACK_TRIAGE_ARTIFACT_HERO,
    version: FEEDBACK_TRIAGE_ENVELOPE_VERSION,
    timestamp,
    artifact_type: FEEDBACK_TRIAGE_ARTIFACT_TYPE,
    schema_version: FEEDBACK_TRIAGE_SCHEMA_VERSION,
    context: {
      branch: input.branch,
      commit: input.commit,
      backlog_item_id: input.backlog_item_id,
    },
    payload: {
      pr_number: input.pr_number,
      pr_url: input.pr_url,
      branch: input.branch,
      round,
      items,
      summary,
    },
  }
}

/**
 * Render the one-line outcome the command shows (DE-003:
 * tool-rendered, not reconstructed from a template in prose).
 *
 * @param prNumber The PR the artifact was written for.
 * @param round The round number recorded in the artifact.
 * @param counts The derived summary counts.
 * @param path The absolute path the artifact was written to.
 * @returns The summary line.
 */
export function renderFeedbackTriageSummary(
  prNumber: number,
  round: number,
  counts: FeedbackTriageSummaryCounts,
  path: string,
): string {
  return [
    `Wrote feedback-triage round ${round} for PR ${prNumber}:`,
    `${counts.total_items} items`,
    `(${counts.accepted} accepted, ${counts.modified} modified, ${counts.rejected} rejected, ` +
      `${counts.asked} asked, ${counts.filed} filed, ${counts.replied} replied)`,
    `to ${path}.`,
  ].join(" ")
}

/**
 * Refuse a symlinked directory component between the repository
 * root and the artifact root's parent (review hardening): the
 * leaf-only lstat checks cannot see a symlinked `.uf` or
 * `.uf/artifacts` — `lstat` on the deeper resolved path follows
 * the link for every component above the leaf, and a missing leaf
 * swallows the check as ENOENT — so the recursive `mkdirSync`
 * could follow the link and create the artifact outside the
 * repository. Every existing component is lstat'ed itself (the
 * link, never its target); a missing component ends the walk
 * because nothing can exist below it, and any other stat failure
 * fails closed.
 */
function refuseSymlinkedArtifactParents(cwd: string, root: string): void {
  const segments = relative(cwd, root)
    .split(sep)
    .filter((segment) => segment !== "")
  let current = cwd
  for (const segment of segments) {
    current = join(current, segment)
    try {
      if (lstatSync(current).isSymbolicLink()) {
        throw new FeedbackOperationError(
          "artifact_error",
          `refusing to write through the symlinked parent ${current} of ${FEEDBACK_TRIAGE_ARTIFACT_DIR}`,
        )
      }
    } catch (err: unknown) {
      if (!isEnoent(err)) throw err
    }
  }
}

/**
 * Write the artifact file atomically (design D5, the
 * `writeFeedbackState` and `write_unleash_summary` precedent): `0o700`
 * recursive mkdir, symlink refusal on every path component from the
 * repository root down (the parent walk, the directory, and the
 * artifact path, all `lstat`), a `0o600` temp file with a
 * high-entropy name, then `renameSync`. The file name is derived
 * from a validated integer PR number and a computed round under the
 * fixed artifact root, so traversal is structurally impossible; the
 * resolved path is still prefix-checked against the root (belt and
 * braces). Filesystem failures fail closed as `artifact_error`.
 */
function writeArtifactFile(cwd: string, prNumber: number, round: number, contents: string): string {
  const root = resolve(cwd, FEEDBACK_TRIAGE_ARTIFACT_DIR)
  const path = feedbackTriageArtifactPath(cwd, prNumber, round)
  try {
    if (!path.startsWith(`${root}/`)) {
      throw new FeedbackOperationError(
        "artifact_error",
        `the resolved artifact path escapes ${FEEDBACK_TRIAGE_ARTIFACT_DIR}/: ${path}`,
      )
    }
    // Before mkdir can follow them: a symlinked `.uf` or
    // `.uf/artifacts` passes the leaf checks below — lstat on the
    // resolved root follows the link — and would escape the repo.
    refuseSymlinkedArtifactParents(cwd, dirname(root))
    try {
      if (lstatSync(root).isSymbolicLink()) {
        throw new FeedbackOperationError(
          "artifact_error",
          `refusing to write into a symlinked ${FEEDBACK_TRIAGE_ARTIFACT_DIR} directory`,
        )
      }
    } catch (err: unknown) {
      if (!isEnoent(err)) throw err
    }
    mkdirSync(root, { recursive: true, mode: 0o700 })
    try {
      if (lstatSync(path).isSymbolicLink()) {
        throw new FeedbackOperationError("artifact_error", "refusing to overwrite a symlinked feedback-triage artifact")
      }
    } catch (err: unknown) {
      if (!isEnoent(err)) throw err
    }
    // High-entropy suffix (review hardening): pid plus the
    // millisecond alone can collide on a same-millisecond retry,
    // letting one write clobber another's temp before the rename.
    const temp = `${path}.${process.pid}.${Date.now()}.${randomUUID()}.tmp`
    writeFileSync(temp, contents, { mode: 0o600 })
    renameSync(temp, path)
    return path
  } catch (err: unknown) {
    if (err instanceof FeedbackOperationError) throw err
    throw new FeedbackOperationError("artifact_error", `could not write the feedback-triage artifact: ${String(err)}`)
  }
}

/**
 * Render and write the feedback-triage 1.1.0 artifact in one call
 * (issue #1022, designs D2 through D7): join the reply outcomes to
 * the FILE items by comment identity, derive the summary counts, scan
 * the next round number, render the envelope, validate the payload
 * against the 1.1.0 zod mirror (a failure writes nothing), write it
 * atomically under `.uf/artifacts/feedback-triage/`, and clear the
 * `uf.address-feedback` progress state through the `clearProgress`
 * hook. A clear failure is a warning, not a failed call — the
 * artifact is already durable (design D6).
 *
 * @param input The validated artifact input (judgment products plus the optional replies projection).
 * @param deps Repository root, clock, and the optional clear-progress hook.
 * @returns The path, round, byte count, derived counts, and a rendered summary line.
 */
export function writeFeedbackTriageArtifact(
  input: WriteFeedbackTriageArtifactInput,
  deps: FeedbackReplyDeps,
): WriteFeedbackTriageArtifactData {
  const clock = deps.now ?? (() => new Date())
  const warnings: string[] = []
  const round = nextRoundNumber(deps.cwd, input.pr_number)
  const artifact = renderFeedbackTriageArtifact(input, round, clock().toISOString())
  const payload = FeedbackTriagePayloadSchema.safeParse(artifact.payload)
  if (!payload.success) {
    throw new FeedbackOperationError(
      "invalid_input",
      `the rendered feedback-triage payload failed 1.1.0 validation: ${payload.error.issues
        .map((issue) => `${issue.path.join(".")}: ${issue.message}`)
        .join("; ")}`,
    )
  }
  const contents = `${JSON.stringify(artifact, null, 2)}\n`
  const path = writeArtifactFile(deps.cwd, input.pr_number, round, contents)
  let progressCleared = false
  if (deps.clearProgress !== undefined) {
    try {
      deps.clearProgress()
      progressCleared = true
    } catch (err: unknown) {
      warnings.push(
        `could not clear the uf.address-feedback progress state: ${err instanceof Error ? err.message : String(err)}`,
      )
    }
  }
  return {
    pr_number: input.pr_number,
    path,
    round,
    bytes: Buffer.byteLength(contents, "utf-8"),
    counts: artifact.payload.summary,
    progress_cleared: progressCleared,
    warnings,
    summary: renderFeedbackTriageSummary(input.pr_number, round, artifact.payload.summary, path),
  }
}

// ---------------------------------------------------------------------------
// gh operations
// ---------------------------------------------------------------------------

/**
 * Run `fn` with a private `0o700` temp directory holding one `0o600`
 * file with `contents`; the directory is always removed afterwards.
 */
function withTempFile<T>(name: string, contents: string, fn: (path: string) => T): T {
  const dir = mkdtempSync(join(tmpdir(), "uf-feedback-"))
  try {
    const path = join(dir, name)
    writeFileSync(path, contents, { mode: 0o600 })
    return fn(path)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

/**
 * Create one issue, retrying once on failure. Ambiguous outcomes are
 * not retried because the issue may exist and a retry could file it
 * twice: a timeout (the create may have reached GitHub before the
 * client gave up) and unparseable output (the create succeeded but
 * the URL was lost) both fail fast and direct the operator to check
 * the repository first (design D3, issue #1023).
 */
function createIssue(
  deps: FeedbackReplyDeps,
  repo: string,
  title: string,
  body: string,
): { number: number; url: string; attempts: number } {
  let last: FeedbackError = { code: "api_error", message: "gh issue create did not run", retryable: false }
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    let output: string
    try {
      output = withTempFile("issue-body.md", body, (bodyPath) =>
        deps.gh(["issue", "create", "--repo", repo, "--title", title, "--body-file", bodyPath], deps.cwd),
      )
    } catch (err: unknown) {
      last = classifyGhError(err)
      // A timeout is an ambiguous outcome: the issue may have been
      // created server-side before the client gave up, so a retry
      // could file it twice. Fail fast and name the ambiguity
      // instead (FRR-FR-002).
      if (last.code === "timeout") {
        throw new FeedbackOperationError(
          "timeout",
          `gh issue create timed out; the issue may exist — check ${repo} for it before re-running: ${last.message}`,
          false,
        )
      }
      continue
    }
    const parsed = parseIssueUrl(output)
    if (parsed === null) {
      throw new FeedbackOperationError(
        "api_error",
        `gh issue create printed no issue URL; check ${repo} for the issue before re-running: ${output.trim().slice(0, 200)}`,
      )
    }
    return { ...parsed, attempts: attempt }
  }
  throw new FeedbackOperationError(
    last.code,
    `gh issue create failed twice: ${last.message}`,
    last.retryable,
  )
}

/** Post one reply through a temp-file JSON request body (`--input`). */
function postReply(deps: FeedbackReplyDeps, endpoint: string, body: string): { id: number | null; url: string | null } {
  let output: string
  try {
    output = withTempFile("reply.json", JSON.stringify({ body }), (inputPath) =>
      deps.gh(["api", "--method", "POST", endpoint, "--input", inputPath], deps.cwd),
    )
  } catch (err: unknown) {
    const classified = classifyGhError(err)
    throw new FeedbackOperationError(classified.code, classified.message, classified.retryable)
  }
  try {
    const parsed: unknown = JSON.parse(output)
    if (isPlainObject(parsed)) {
      const id = typeof parsed.id === "number" ? parsed.id : null
      const url = typeof parsed.html_url === "string" ? parsed.html_url : null
      return { id, url }
    }
  } catch {
    // The POST succeeded (exit 0); an unparseable body only loses the id.
  }
  return { id: null, url: null }
}

/** A review thread as the GraphQL lookup reports it. */
export interface ReviewThread {
  readonly id: string
  readonly isResolved: boolean
}

/**
 * Map every review comment's REST id (`databaseId`) to its thread,
 * paging through `reviewThreads`. This closes the REST-only ingest
 * gap: Phase 1.3 never sees thread node IDs.
 */
export function fetchReviewThreads(deps: FeedbackReplyDeps, repo: string, prNumber: number): Map<number, ReviewThread> {
  const [owner, name] = repo.split("/") as [string, string]
  const threads = new Map<number, ReviewThread>()
  let cursor: string | null = null
  for (let page = 0; page < MAX_THREAD_PAGES; page++) {
    const args = [
      "api",
      "graphql",
      "-f",
      `query=${REVIEW_THREADS_QUERY}`,
      "-f",
      `owner=${owner}`,
      "-f",
      `name=${name}`,
      "-F",
      `number=${prNumber}`,
    ]
    if (cursor !== null) args.push("-f", `cursor=${cursor}`)
    let output: string
    try {
      output = deps.gh(args, deps.cwd)
    } catch (err: unknown) {
      const classified = classifyGhError(err)
      throw new FeedbackOperationError(classified.code, `review thread lookup failed: ${classified.message}`, classified.retryable)
    }
    const connection = parseThreadPage(output)
    for (const node of connection.nodes) {
      for (const commentId of node.commentIds) {
        threads.set(commentId, { id: node.id, isResolved: node.isResolved })
      }
    }
    if (!connection.hasNextPage || connection.endCursor === null) return threads
    cursor = connection.endCursor
  }
  throw new FeedbackOperationError("api_error", `review thread lookup exceeded ${MAX_THREAD_PAGES} pages`)
}

/** Parse one reviewThreads page, failing closed on any unexpected shape. */
function parseThreadPage(output: string): {
  nodes: Array<{ id: string; isResolved: boolean; commentIds: number[] }>
  hasNextPage: boolean
  endCursor: string | null
} {
  const malformed = (): FeedbackOperationError =>
    new FeedbackOperationError("api_error", "review thread lookup returned an unexpected payload")
  let parsed: unknown
  try {
    parsed = JSON.parse(output)
  } catch {
    throw malformed()
  }
  const connection = dig(parsed, ["data", "repository", "pullRequest", "reviewThreads"])
  if (!isPlainObject(connection) || !Array.isArray(connection.nodes) || !isPlainObject(connection.pageInfo)) {
    throw malformed()
  }
  const nodes = connection.nodes.map((node: unknown) => {
    if (!isPlainObject(node) || typeof node.id !== "string" || typeof node.isResolved !== "boolean") {
      throw malformed()
    }
    const comments = dig(node, ["comments", "nodes"])
    const commentIds = Array.isArray(comments)
      ? comments
          .map((comment: unknown) => (isPlainObject(comment) ? comment.databaseId : undefined))
          .filter((id): id is number => typeof id === "number")
      : []
    return { id: node.id, isResolved: node.isResolved, commentIds }
  })
  const hasNextPage = connection.pageInfo.hasNextPage === true
  const endCursor = typeof connection.pageInfo.endCursor === "string" ? connection.pageInfo.endCursor : null
  return { nodes, hasNextPage, endCursor }
}

/** Walk a path of object keys, returning undefined on any miss. */
function dig(value: unknown, path: readonly string[]): unknown {
  let current: unknown = value
  for (const key of path) {
    if (!isPlainObject(current)) return undefined
    current = current[key]
  }
  return current
}

/**
 * Resolve one thread, retrying once on a retryable failure. The
 * mutation is idempotent, so the retry cannot double-apply.
 */
function resolveThread(deps: FeedbackReplyDeps, threadId: string): void {
  let last: FeedbackError = { code: "api_error", message: "resolveReviewThread did not run", retryable: false }
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      const output = deps.gh(
        ["api", "graphql", "-f", `query=${RESOLVE_THREAD_MUTATION}`, "-f", `threadId=${threadId}`],
        deps.cwd,
      )
      const resolved = dig(safeJson(output), ["data", "resolveReviewThread", "thread", "isResolved"])
      if (resolved === true) return
      throw new FeedbackOperationError("api_error", "resolveReviewThread did not report the thread resolved")
    } catch (err: unknown) {
      last = classifyGhError(err)
      if (!last.retryable) break
    }
  }
  throw new FeedbackOperationError(last.code, last.message, last.retryable)
}

/** JSON.parse that yields undefined instead of throwing. */
function safeJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

/** A fresh record for an item seen for the first time. */
function newRecord(item: Pick<ReplyItem, "comment_kind" | "comment_id" | "decision">, now: string): ReplyRecord {
  return {
    comment_kind: item.comment_kind,
    comment_id: item.comment_id,
    decision: item.decision,
    issue_number: null,
    issue_url: null,
    comment_posted: false,
    reply_id: null,
    reply_url: null,
    thread_id: null,
    thread_resolved: false,
    updated_at: now,
  }
}

/** A recorded issue number, when the cache holds a valid one. */
function recordedIssue(record: ReplyRecord | undefined): { number: number; url: string } | null {
  if (record === undefined) return null
  if (typeof record.issue_number === "number" && typeof record.issue_url === "string") {
    return { number: record.issue_number, url: record.issue_url }
  }
  return null
}

/**
 * File one finding as an issue (issue #998). A cached issue for the same
 * comment is returned without filing again; otherwise the issue is
 * created with one retry and recorded in the cache before returning.
 */
export function fileFeedbackIssue(input: FileFeedbackIssueInput, deps: FeedbackReplyDeps): FileFeedbackIssueData {
  const now = (deps.now ?? (() => new Date()))().toISOString()
  const key = itemKey(input.comment_kind, input.comment_id)
  const state = readFeedbackState(deps.cwd, input.pr_number)
  const repo = resolveRepo(deps, input.repo)
  const cached = recordedIssue(state.replies[key])
  if (cached !== null) {
    return {
      pr_number: input.pr_number,
      repo,
      key,
      issue_number: cached.number,
      issue_url: cached.url,
      deduplicated: true,
      attempts: 0,
    }
  }
  const created = createIssue(deps, repo, input.title, input.body)
  const record =
    state.replies[key] ??
    newRecord({ comment_kind: input.comment_kind, comment_id: input.comment_id, decision: "file" }, now)
  record.issue_number = created.number
  record.issue_url = created.url
  record.updated_at = now
  state.replies[key] = record
  writeFeedbackState(deps.cwd, input.pr_number, state)
  return {
    pr_number: input.pr_number,
    repo,
    key,
    issue_number: created.number,
    issue_url: created.url,
    deduplicated: false,
    attempts: created.attempts,
  }
}

/** Mutable per-item working result. */
type WorkingResult = {
  -readonly [K in keyof ReplyItemResult]: ReplyItemResult[K]
}

/**
 * Post one confirmed comment on an issue (the /uf.triage-issue 4.3
 * DE-005 extraction). The body is a human-confirmed judgment product
 * passed as a validated parameter; temp-file creation, 0o600
 * permissions, JSON serialization, `gh api --input`, and all-paths
 * cleanup are owned by the tool. The comment id/url are null only
 * when the POST succeeded but gh printed unparseable output.
 */
export function postIssueComment(input: PostIssueCommentInput, deps: FeedbackReplyDeps): PostIssueCommentData {
  const repo = resolveRepo(deps, input.repo)
  const posted = postReply(deps, `repos/${repo}/issues/${input.issue_number}/comments`, input.body)
  return {
    repo,
    issue_number: input.issue_number,
    comment_id: posted.id,
    comment_url: posted.url,
  }
}

/**
 * Create one confirmed child issue (the /uf.triage-issue 4.4 DE-005
 * extraction). Title and body are human-confirmed judgment products
 * passed as validated parameters; temp-file creation, 0o600
 * permissions, `gh issue create --body-file`, retry, and all-paths
 * cleanup are owned by the tool.
 */
export function createChildIssue(input: CreateChildIssueInput, deps: FeedbackReplyDeps): CreateChildIssueData {
  const repo = resolveRepo(deps, input.repo)
  const created = createIssue(deps, repo, input.title, input.body)
  return {
    repo,
    issue_number: created.number,
    issue_url: created.url,
    attempts: created.attempts,
  }
}

/**
 * File, post, and resolve in one call (issue #966).
 *
 * Order, per item in input order: file the issue (FILE items, once,
 * one retry; a failure skips that item's reply and thread and moves
 * on), then post the reply. A failed post stops posting (failure-stop):
 * the remaining items stay pending for the re-run, because a rate
 * limit or outage will fail them too. Then, when the author approved
 * resolution, one paginated reviewThreads lookup maps comments to
 * thread IDs and each addressed review thread with a posted reply is
 * resolved (one retry on retryable errors; failures are reported per
 * item and do not stop other resolutions). Every mutation is recorded
 * in the cache before the next starts. The `reply-comments` checkpoint
 * is recorded only when nothing is left to do.
 */
export function postFeedbackReplies(input: PostFeedbackRepliesInput, deps: FeedbackReplyDeps): PostFeedbackRepliesData {
  const clock = deps.now ?? (() => new Date())
  const state = readFeedbackState(deps.cwd, input.pr_number)
  const repo = resolveRepo(deps, input.repo)
  const warnings: string[] = []
  const persist = (): void => writeFeedbackState(deps.cwd, input.pr_number, state)

  const results: WorkingResult[] = []
  let stopped: FeedbackError | null = null

  for (const item of input.items) {
    const key = itemKey(item.comment_kind, item.comment_id)
    const record = state.replies[key] ?? newRecord(item, clock().toISOString())
    record.decision = item.decision
    state.replies[key] = record
    const cachedIssue = recordedIssue(record)
    const result: WorkingResult = {
      key,
      comment_kind: item.comment_kind,
      comment_id: item.comment_id,
      decision: item.decision,
      reply: "pending",
      reply_id: record.reply_id,
      reply_url: record.reply_url,
      issue: item.decision === "file" ? (cachedIssue !== null ? "deduplicated" : null) : null,
      issue_number: cachedIssue?.number ?? null,
      issue_url: cachedIssue?.url ?? null,
      thread: "not_requested",
      thread_id: record.thread_id,
      error: null,
    }
    results.push(result)

    if (record.comment_posted) {
      result.reply = "already_posted"
      continue
    }
    if (stopped !== null) {
      continue
    }

    if (item.decision === "file" && cachedIssue === null && item.issue !== undefined) {
      try {
        const created = createIssue(deps, repo, item.issue.title, item.issue.body)
        record.issue_number = created.number
        record.issue_url = created.url
        record.updated_at = clock().toISOString()
        persist()
        result.issue = "filed"
        result.issue_number = created.number
        result.issue_url = created.url
      } catch (err: unknown) {
        if (err instanceof FeedbackOperationError && err.code === "state_error") throw err
        result.issue = "failed"
        result.reply = "issue_failed"
        result.error = classifyGhError(err)
        continue
      }
    }

    try {
      const posted = postReply(deps, replyEndpoint(repo, input.pr_number, item), renderReplyBody(item, result.issue_number))
      record.comment_posted = true
      record.reply_id = posted.id
      record.reply_url = posted.url
      record.updated_at = clock().toISOString()
      persist()
      result.reply = "posted"
      result.reply_id = posted.id
      result.reply_url = posted.url
    } catch (err: unknown) {
      if (err instanceof FeedbackOperationError && err.code === "state_error") throw err
      result.reply = "post_failed"
      result.error = classifyGhError(err)
      stopped = result.error
    }
  }

  // Thread resolution.
  for (const result of results) {
    const record = state.replies[result.key] as ReplyRecord
    if (!isResolutionTarget(result)) {
      result.thread = "not_applicable"
    } else if (!input.resolve_threads) {
      result.thread = record.thread_resolved ? "already_resolved" : "not_requested"
    } else if (record.thread_resolved) {
      result.thread = "already_resolved"
    } else if (!record.comment_posted) {
      result.thread = "reply_not_posted"
    } else {
      result.thread = "failed" // provisional until the lookup below runs
    }
  }

  const toResolve = results.filter((result) => result.thread === "failed")
  if (toResolve.length > 0) {
    let threads: Map<number, ReviewThread> | null = null
    try {
      threads = fetchReviewThreads(deps, repo, input.pr_number)
    } catch (err: unknown) {
      const lookupError = classifyGhError(err)
      for (const result of toResolve) {
        result.error ??= lookupError
      }
    }
    if (threads !== null) {
      for (const result of toResolve) {
        const record = state.replies[result.key] as ReplyRecord
        const thread = threads.get(result.comment_id)
        if (thread === undefined) {
          result.thread = "thread_not_found"
          result.error ??= {
            code: "thread_not_found",
            message: `no review thread contains comment ${result.comment_id}`,
            retryable: false,
          }
          continue
        }
        record.thread_id = thread.id
        result.thread_id = thread.id
        try {
          if (!thread.isResolved) {
            resolveThread(deps, thread.id)
          }
          record.thread_resolved = true
          record.updated_at = clock().toISOString()
          persist()
          result.thread = thread.isResolved ? "already_resolved" : "resolved"
        } catch (err: unknown) {
          if (err instanceof FeedbackOperationError && err.code === "state_error") throw err
          result.thread = "failed"
          result.error ??= classifyGhError(err)
        }
      }
    }
  }

  const frozen = results as ReplyItemResult[]
  const posted = frozen.filter((result) => result.reply === "posted" || result.reply === "already_posted").length
  const resolvable = input.resolve_threads ? frozen.filter((result) => isResolutionTarget(result)) : []
  const resolved = resolvable.filter((result) => result.thread === "resolved" || result.thread === "already_resolved").length
  const complete = posted === frozen.length && resolved === resolvable.length
  const failed = frozen.filter((result) => result.error !== null).length

  let checkpointRecorded = false
  if (complete && deps.recordCheckpoint !== undefined) {
    try {
      deps.recordCheckpoint(REPLY_CHECKPOINT_STEP, {
        posted,
        total: frozen.length,
        threads_resolved: resolved,
        threads_resolvable: resolvable.length,
      })
      checkpointRecorded = true
    } catch (err: unknown) {
      warnings.push(`could not record the ${REPLY_CHECKPOINT_STEP} checkpoint: ${err instanceof Error ? err.message : String(err)}`)
    }
  }

  return {
    pr_number: input.pr_number,
    repo,
    complete,
    counts: {
      total: frozen.length,
      posted,
      pending: frozen.filter((result) => result.reply === "pending").length,
      failed,
      issues_filed: frozen.filter((result) => result.issue === "filed").length,
      threads_resolved: resolved,
      threads_resolvable: resolvable.length,
    },
    items: frozen,
    checkpoint_recorded: checkpointRecorded,
    summary: renderReplySummary(input.pr_number, frozen, input.resolve_threads),
    warnings,
  }
}
