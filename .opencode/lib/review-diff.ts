import { Buffer } from "node:buffer"
import { randomUUID } from "node:crypto"
import { chmod, lstat, mkdir, open, readFile, unlink } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join, resolve } from "node:path"

import { tool } from "@opencode-ai/plugin"
import { z } from "zod"

import { defaultGitExec, type GitExec } from "./uf-workflow-types.js"

/** Valid base-ref source labels. */
export type BaseRefSource = "upstream/main" | "origin/main" | "main"

/** Ordered candidate refs to try (no network access needed). */
export const BASE_REF_CANDIDATES: readonly BaseRefSource[] = [
  "upstream/main",
  "origin/main",
  "main",
] as const

/** Base-ref resolution including the HEAD state and divergence signal. */
export interface BaseRefResolution {
  ref: BaseRefSource
  sha: string
  source: BaseRefSource
  /** HEAD commit id at resolution time ("" when HEAD cannot be resolved). */
  head_sha: string
  /**
   * True when the resolved base is NOT an ancestor of HEAD -- the base
   * contains commits HEAD lacks (stale fork, unrelated history). A diff
   * against such a base silently injects foreign-commit noise, so
   * consumers MUST NOT diff against it and should fall back to
   * uncommitted-local mode.
   */
  diverged: boolean
}

/**
 * Resolve the best available base ref and detect divergence.
 *
 * Tries upstream/main, origin/main, then main via the injected git
 * executor (no network access). Beyond the legacy ref/sha/source
 * fields, the resolution carries the current HEAD sha and a diverged
 * flag computed with `git merge-base --is-ancestor <base> HEAD`: the
 * flag is true when the base is not an ancestor of HEAD, meaning a
 * diff against the base would include changes HEAD never made.
 * When HEAD itself cannot be resolved, head_sha is "" and diverged
 * is false (undetermined, not diverged).
 *
 * Returns null when no candidate resolves.
 */
export function resolveBaseRefWithDivergence(exec: GitExec, cwd: string): BaseRefResolution | null {
  for (const candidate of BASE_REF_CANDIDATES) {
    try {
      const sha = exec(["rev-parse", "--verify", candidate], cwd)
      if (sha) {
        return {
          ref: candidate,
          sha,
          source: candidate,
          ...resolveHeadState(exec, cwd, sha),
        }
      }
    } catch {
      // Candidate not available, try next.
    }
  }
  return null
}

/** Compute head_sha and the divergence flag for a resolved base. */
function resolveHeadState(
  exec: GitExec,
  cwd: string,
  baseSha: string,
): Pick<BaseRefResolution, "head_sha" | "diverged"> {
  let headSha = ""
  try {
    headSha = exec(["rev-parse", "--verify", "HEAD"], cwd)
  } catch {
    return { head_sha: "", diverged: false }
  }
  try {
    exec(["merge-base", "--is-ancestor", baseSha, "HEAD"], cwd)
    return { head_sha: headSha, diverged: false }
  } catch {
    return { head_sha: headSha, diverged: true }
  }
}

/**
 * Per-file change counts assembled from `git diff --numstat` output and
 * synthesized counts for captured untracked/ignored files.
 */
export interface FileChange {
  path: string
  additions: number
  deletions: number
}

/** A file excluded from the captured diff, with the deterministic reason. */
export interface ExcludedFile {
  path: string
  reason:
    | "file-budget"
    | "file-symlink"
    | "file-too-large"
    | "total-bytes-budget"
    | "binary-file"
    | "stat-failed"
    | "read-failed"
}

/** Successful capture result. */
export interface ReviewDiffResult {
  mode: "base" | "uncommitted-local"
  base: BaseRefResolution | null
  fallback_reason?: string
  diff_path: string
  diff_bytes: number
  files: FileChange[]
  total_files: number
  total_additions: number
  total_deletions: number
  excluded: ExcludedFile[]
}

/** Failed capture result. */
export interface ReviewDiffError {
  error: true
  code: "invalid_input" | "git_error" | "diff_too_large" | "io_error"
  message: string
  retryable: boolean
}

/** Injected collaborators for captureReviewDiff (tests substitute fakes). */
export interface ReviewDiffDependencies {
  exec: GitExec
  cwd: string
  diffDir: string
  newId: () => string
  statFile: (path: string) => Promise<{ isSymlink: boolean; size: number }>
  readFile: (path: string) => Promise<string>
  writeDiffFile: (path: string, contents: string) => Promise<void>
}

export type CaptureReviewDiffInput = { feature_dir?: string }

/** Budgets mirroring the sibling-evidence caps. */
const MAX_CAPTURED_FILES = 200
const MAX_FILE_BYTES = 256 * 1024
const MAX_TOTAL_NEW_FILE_BYTES = 1024 * 1024
const MAX_DIFF_BYTES = 8 * 1024 * 1024

function isSafeRelativePath(value: string): boolean {
  if (value.startsWith("/") || /^[a-zA-Z]:[\\/]/.test(value)) {
    return false
  }
  return !value.split("/").includes("..")
}

function normalizeRelativePath(value: string): string {
  return value
    .split("/")
    .filter((segment) => segment !== "" && segment !== ".")
    .join("/")
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function listLines(text: string): string[] {
  return text.split("\n").filter(Boolean)
}

function countContentLines(content: string): number {
  if (content === "") {
    return 0
  }
  const lines = content.split("\n")
  if (lines[lines.length - 1] === "") {
    lines.pop()
  }
  return lines.length
}

function parseNumstat(text: string): FileChange[] {
  const files: FileChange[] = []
  for (const line of listLines(text)) {
    const match = /^(\d+|-)\t(\d+|-)\t(.+)$/.exec(line)
    if (match) {
      files.push({
        path: match[3],
        additions: match[1] === "-" ? 0 : Number(match[1]),
        deletions: match[2] === "-" ? 0 : Number(match[2]),
      })
    }
  }
  return files
}

function renderNewFileDiff(path: string, content: string): string {
  const lines = ["diff --git a/" + path + " b/" + path, "new file mode 100644", "--- /dev/null", "+++ b/" + path]
  const body = content.split("\n")
  if (body.length > 0 && body[body.length - 1] === "") {
    body.pop()
  }
  if (body.length === 0) {
    return lines.join("\n")
  }
  lines.push("@@ -0,0 +1," + body.length + " @@")
  for (const line of body) {
    lines.push("+" + line)
  }
  return lines.join("\n")
}

/**
 * Capture the full review diff in one deterministic pass.
 *
 * Resolves the base ref (with stale-fork divergence detection), assembles
 * tracked changes plus untracked and git-ignored files under the feature
 * dir, computes numstat, and writes a bounded patch file. When the base is
 * missing or diverged, falls back to uncommitted-local mode (git diff HEAD
 * + feature-dir files) with the fallback reason recorded.
 */
export async function captureReviewDiff(
  input: CaptureReviewDiffInput,
  deps: ReviewDiffDependencies,
): Promise<ReviewDiffResult | ReviewDiffError> {
  const base = resolveBaseRefWithDivergence(deps.exec, deps.cwd)
  let mode: "base" | "uncommitted-local"
  let fallbackReason: string | undefined
  if (base === null) {
    mode = "uncommitted-local"
    fallbackReason = "no resolvable base ref (tried " + BASE_REF_CANDIDATES.join(", ") + ")"
  } else if (base.diverged) {
    mode = "uncommitted-local"
    fallbackReason =
      "base " + base.ref + " is not an ancestor of HEAD (stale fork); diffing against it would inject foreign-commit noise"
  } else {
    mode = "base"
  }

  const diffTarget = mode === "base" && base !== null ? base.sha : "HEAD"
  let trackedDiff = ""
  let trackedNumstat = ""
  try {
    trackedDiff = deps.exec(["diff", "--no-renames", diffTarget], deps.cwd)
    trackedNumstat = deps.exec(["diff", "--no-renames", "--numstat", diffTarget], deps.cwd)
  } catch (error) {
    if (mode === "base") {
      return {
        error: true,
        code: "git_error",
        message: "git diff against " + diffTarget + " failed: " + errorMessage(error),
        retryable: false,
      }
    }
    fallbackReason = (fallbackReason ?? "uncommitted-local mode") + "; tracked diff unavailable (" + errorMessage(error) + ")"
  }

  const files = parseNumstat(trackedNumstat)
  const excluded: ExcludedFile[] = []
  const sections: string[] = trackedDiff === "" ? [] : [trackedDiff]

  if (input.feature_dir !== undefined) {
    const featureDir = normalizeRelativePath(input.feature_dir)
    let untracked: string[]
    let ignored: string[]
    try {
      untracked = listLines(deps.exec(["ls-files", "--others", "--exclude-standard", "--", featureDir], deps.cwd))
      ignored = listLines(
        deps.exec(["ls-files", "--others", "--ignored", "--exclude-standard", "--", featureDir], deps.cwd),
      )
    } catch (error) {
      return {
        error: true,
        code: "git_error",
        message: "git ls-files under " + featureDir + " failed: " + errorMessage(error),
        retryable: false,
      }
    }
    const seen = new Set(untracked)
    const candidates = [...untracked, ...ignored.filter((path) => !seen.has(path))].sort()
    let capturedCount = 0
    let totalNewBytes = 0
    for (const path of candidates) {
      if (capturedCount >= MAX_CAPTURED_FILES) {
        excluded.push({ path, reason: "file-budget" })
        continue
      }
      const absolute = resolve(deps.cwd, path)
      let stat: { isSymlink: boolean; size: number }
      try {
        stat = await deps.statFile(absolute)
      } catch {
        excluded.push({ path, reason: "stat-failed" })
        continue
      }
      if (stat.isSymlink) {
        excluded.push({ path, reason: "file-symlink" })
        continue
      }
      if (stat.size > MAX_FILE_BYTES) {
        excluded.push({ path, reason: "file-too-large" })
        continue
      }
      if (totalNewBytes + stat.size > MAX_TOTAL_NEW_FILE_BYTES) {
        excluded.push({ path, reason: "total-bytes-budget" })
        continue
      }
      let content: string
      try {
        content = await deps.readFile(absolute)
      } catch {
        excluded.push({ path, reason: "read-failed" })
        continue
      }
      if (content.includes("\0")) {
        excluded.push({ path, reason: "binary-file" })
        continue
      }
      sections.push(renderNewFileDiff(path, content))
      files.push({ path, additions: countContentLines(content), deletions: 0 })
      capturedCount += 1
      totalNewBytes += stat.size
    }
  }

  const combined = sections.join("\n")
  const diffBytes = Buffer.byteLength(combined, "utf8")
  if (diffBytes > MAX_DIFF_BYTES) {
    return {
      error: true,
      code: "diff_too_large",
      message: "captured diff exceeds " + MAX_DIFF_BYTES + " bytes",
      retryable: false,
    }
  }

  const diffPath = join(deps.diffDir, "review-diff-" + deps.newId() + ".patch")
  try {
    await deps.writeDiffFile(diffPath, combined)
  } catch (error) {
    return {
      error: true,
      code: "io_error",
      message: "failed to write diff file: " + errorMessage(error),
      retryable: false,
    }
  }

  const totalAdditions = files.reduce((sum, file) => sum + file.additions, 0)
  const totalDeletions = files.reduce((sum, file) => sum + file.deletions, 0)
  return {
    mode,
    base,
    ...(fallbackReason !== undefined ? { fallback_reason: fallbackReason } : {}),
    diff_path: diffPath,
    diff_bytes: diffBytes,
    files,
    total_files: files.length,
    total_additions: totalAdditions,
    total_deletions: totalDeletions,
    excluded,
  }
}

async function defaultWriteDiffFile(path: string, contents: string): Promise<void> {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 })
  try {
    await unlink(path)
  } catch {
    // Absent target is fine.
  }
  const handle = await open(path, "wx", 0o600)
  try {
    await handle.writeFile(Buffer.from(contents, "utf8"))
  } finally {
    await handle.close()
  }
  await chmod(path, 0o600)
}

/** Production dependencies for the capture tool. */
export function createReviewDiffDependencies(projectRoot: string): ReviewDiffDependencies {
  return {
    exec: defaultGitExec,
    cwd: projectRoot,
    diffDir: join(tmpdir(), "opencode"),
    newId: () => randomUUID(),
    statFile: async (path) => {
      const stats = await lstat(path)
      return { isSymlink: stats.isSymbolicLink(), size: stats.size }
    },
    readFile: (path) => readFile(path, "utf8"),
    writeDiffFile: defaultWriteDiffFile,
  }
}

const CaptureReviewDiffArgs = z.object({
  feature_dir: z
    .string()
    .min(1)
    .max(1024)
    .refine(isSafeRelativePath, {
      message: "feature_dir must be a repo-relative path without an absolute prefix or .. segments",
    })
    .optional(),
})

/** Tool wrapper around captureReviewDiff. */
export function createCaptureReviewDiffTool(deps: ReviewDiffDependencies) {
  return tool({
    description:
      "Capture the full review diff in one deterministic call. Resolves the base ref with stale-fork divergence detection (falling back to uncommitted-local mode with the reason recorded), assembles tracked changes plus untracked and git-ignored files under the feature dir, computes numstat, and saves a bounded patch file. Returns mode, base, diff path, per-file counts, and exclusions.",
    args: { feature_dir: CaptureReviewDiffArgs.shape.feature_dir },
    async execute(args: { feature_dir?: string }): Promise<{ readonly output: string }> {
      const parsed = CaptureReviewDiffArgs.safeParse(args)
      if (!parsed.success) {
        const message = parsed.error.issues
          .map((issue) => issue.path.join(".") + ": " + issue.message)
          .join("; ")
        return {
          output: JSON.stringify(
            { error: true, code: "invalid_input", message: "Invalid input: " + message, retryable: false },
            null,
            2,
          ),
        }
      }
      return { output: JSON.stringify(await captureReviewDiff(parsed.data, deps), null, 2) }
    },
  })
}
