/**
 * Reply posting, thread resolution, and issue filing for
 * `/uf.address-feedback` (OpenSpec change feedback-reply-tools,
 * issues #966 and #998).
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
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { z } from "zod"

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

/** Dependencies of the feedback reply operations. */
export interface FeedbackReplyDeps {
  /** gh executor (argument array, no shell). */
  readonly gh: FeedbackGhExec
  /** Repository root: gh runs here and the feedback cache lives under it. */
  readonly cwd: string
  /** Clock for record timestamps. */
  readonly now?: () => Date
  /** Records a progress checkpoint for `uf.address-feedback`; throws on failure. */
  readonly recordCheckpoint?: (step: string, data: Record<string, unknown>) => void
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
 */
export function classifyGhError(err: unknown): FeedbackError {
  if (err instanceof FeedbackOperationError) return err.toError()
  const record = (typeof err === "object" && err !== null ? err : {}) as Record<string, unknown>
  const message = err instanceof Error ? err.message : String(err)
  const stderr = textOf(record.stderr).trim()
  const detail = stderr !== "" ? stderr : message
  const lower = `${message}\n${stderr}`.toLowerCase()
  if (record.code === "ENOENT") {
    return { code: "gh_not_found", message: "gh CLI is not installed or not on PATH", retryable: false }
  }
  if (record.code === "ETIMEDOUT" || record.signal === "SIGTERM" || lower.includes("timed out")) {
    return { code: "timeout", message: `gh CLI timed out: ${detail}`, retryable: true }
  }
  if (lower.includes("rate limit")) {
    return { code: "rate_limited", message: `GitHub API rate limited: ${detail}`, retryable: true }
  }
  // gh exits 4 when authentication is required.
  if (record.status === 4 || lower.includes("gh auth login") || lower.includes("not logged")) {
    return { code: "gh_auth_failed", message: `gh CLI is not authenticated: ${detail}`, retryable: false }
  }
  return { code: "api_error", message: `gh CLI failed: ${detail}`, retryable: false }
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
 * Create one issue, retrying once on failure. Unparseable output is
 * not retried: the issue may exist, and a retry could file it twice.
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
