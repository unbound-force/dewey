import { execSync } from "node:child_process"
import {
  readFileSync,
  writeFileSync,
  mkdtempSync,
  rmSync,
  existsSync,
} from "node:fs"
import { resolve, join } from "node:path"
import { tmpdir } from "node:os"
import { tool, type PluginModule } from "@opencode-ai/plugin"
import { classifyBranch } from "../../lib/branch-classification.js"
import { z } from "zod"
import {
  defaultGitExec,
  failure,
  normalizeModelName,
  sanitizeSlug,
  success,
  type GitExec,
} from "../../lib/uf-workflow-types.js"

// Re-export envelope helpers and shared utilities for consumer convenience.
export { defaultGitExec, failure, normalizeModelName, sanitizeSlug, success } from "../../lib/uf-workflow-types.js"
export type { GitExec, ToolFailure, ToolResult, ToolSuccess } from "../../lib/uf-workflow-types.js"

// ---------------------------------------------------------------------------
// Secret detection patterns
// ---------------------------------------------------------------------------

/** Filename patterns that indicate potential secret material. */
const SECRET_PATTERNS = [
  /^\.env$/,
  /^\.env\..+$/,
  /\.pem$/,
  /\.key$/,
  /^id_rsa/,
  /\.p12$/,
  /\.pfx$/,
  /\.jks$/,
  /\.keystore$/,
  /^credentials\.json$/,
  /^secrets\.json$/,
]

/** Test whether a filename matches any secret pattern. */
function isSecretCandidate(filename: string): boolean {
  const base = filename.split("/").pop() ?? filename
  return SECRET_PATTERNS.some((p) => p.test(base))
}

// ---------------------------------------------------------------------------
// Rev-list count parsing
// ---------------------------------------------------------------------------

/**
 * Parse `git rev-list --count` output as a finite non-negative
 * integer.
 *
 * The trimmed output must be entirely digits — git emits only
 * digits and a newline on success — so partially numeric output
 * ("2x") is rejected rather than prefix-parsed by parseInt.
 * Unparseable-but-successful output (empty, garbage, or partially
 * numeric) throws so callers route it through their
 * count-failure envelope — the `parseInt(x, 10) || 0` coercion
 * this replaces silently produced the 0 sentinel, making
 * "genuinely zero" indistinguishable from "failed to count" and
 * suppressing the gate the counts decide.
 *
 * @param output - Raw stdout of a `git rev-list --count` command.
 * @returns The parsed commit count.
 * @throws When the trimmed output is not entirely digits or does
 *   not parse as a finite integer.
 */
function parseRevCount(output: string): number {
  // parseInt is prefix-tolerant ("2x" parses as 2), so the full
  // trimmed token must be digits before parseInt runs; the
  // finiteness check stays as defense-in-depth for digit strings
  // long enough to overflow to Infinity.
  const trimmed = output.trim()
  const count = /^\d+$/.test(trimmed) ? parseInt(trimmed, 10) : NaN
  if (!Number.isFinite(count)) {
    throw new Error(
      `unparseable rev-list --count output: ${JSON.stringify(trimmed)}`,
    )
  }
  return count
}

// ---------------------------------------------------------------------------
// 1. uf_finale_startup — Branch safety and working tree context
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_startup tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_startup tool.
 */
export function createUfFinaleStartupTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Gather branch safety and working tree context for the finale " +
      "workflow. Returns branch name, type, uncommitted changes, " +
      "secret candidates, and unpushed commits.",
    execute: async () => {
      try {
        const branch = exec(["rev-parse", "--abbrev-ref", "HEAD"], cwd)
        // Delegates to the shared pure classifier (lib/branch-classification)
        // so the branch gate and this tool cannot drift apart (DRY; OpenSpec
        // change unleash-deterministic-tools). The output contract is
        // unchanged, including main/master -> branchType "other" + isMain.
        const { branchType, changeName, isMain } = classifyBranch(branch)

        // Check for uncommitted changes.
        let porcelainOutput = ""
        try {
          porcelainOutput = exec(["status", "--porcelain"], cwd)
        } catch {
          // Empty status is fine.
        }
        const hasChanges = porcelainOutput.length > 0

        // Scan changed files for secret candidates.
        const secretCandidates: string[] = []
        if (porcelainOutput) {
          const lines = porcelainOutput.split("\n").filter((l) => l.length > 0)
          for (const line of lines) {
            // git status --porcelain format: XY filename (or XY filename -> renamed)
            const filename = line.slice(3).split(" -> ").pop()?.trim() ?? ""
            if (filename && isSecretCandidate(filename)) {
              secretCandidates.push(filename)
            }
          }
        }

        // Unpushed commits (empty if no upstream).
        const unpushedCommits: Array<{ sha: string; subject: string }> = []
        try {
          const logOutput = exec(["log", "@{u}..HEAD", "--oneline"], cwd)
          if (logOutput) {
            const lines = logOutput.split("\n").filter((l) => l.length > 0)
            for (const line of lines) {
              const spaceIdx = line.indexOf(" ")
              if (spaceIdx > 0) {
                unpushedCommits.push({
                  sha: line.slice(0, spaceIdx),
                  subject: line.slice(spaceIdx + 1),
                })
              }
            }
          }
        } catch {
          // No upstream tracking — unpushedCommits stays empty.
        }

        const data = {
          branch,
          isMain,
          branchType,
          changeName,
          hasChanges,
          secretCandidates,
          unpushedCommits,
        }

        return { output: JSON.stringify(success(data)) }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "STARTUP_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 2. uf_finale_stage — Stage all changes
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_stage tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_stage tool.
 */
export function createUfFinaleStageTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Stage all changes in the working tree via `git add .` and return " +
      "the count of staged files.",
    execute: async () => {
      try {
        exec(["add", "."], cwd)
        const stagedOutput = exec(["diff", "--cached", "--name-only"], cwd)
        const files = stagedOutput
          .split("\n")
          .filter((f) => f.length > 0)
        return {
          output: JSON.stringify(
            success({ fileCount: files.length, files }),
          ),
        }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "STAGE_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 3. uf_finale_commit_context — Diff stat, recent log, normalized model
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_commit_context tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_commit_context tool.
 */
export function createUfFinaleCommitContextTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Gather commit context: cached diff stat, recent log, and " +
      "normalized model name for commit message authorship.",
    args: {
      rawModelId: z.string().describe(
        "Raw model identifier to normalize " +
          "(e.g. google-vertex-anthropic/claude-sonnet-4-20250514@default)",
      ),
    },
    execute: async (args: { rawModelId: string }) => {
      // Inner operations have their own error handling;
      // normalizeModelName is pure and cannot throw.
      let diffStat = ""
      try {
        diffStat = exec(["diff", "--cached", "--stat"], cwd)
      } catch {
        // No cached diff is possible.
      }

      let recentLog = ""
      try {
        recentLog = exec(["log", "-5", "--oneline"], cwd)
      } catch {
        // New repo with no commits.
      }

      const normalizedModel = normalizeModelName(args.rawModelId)

      return {
        output: JSON.stringify(
          success({ diffStat, recentLog, normalizedModel }),
        ),
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 4. uf_finale_base_state — PR base remote, branch, and ahead/behind counts
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_base_state tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_base_state tool.
 */
export function createUfFinaleBaseStateTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Resolve the PR base remote (upstream when configured, else origin) " +
      "and base branch (remote HEAD symbolic ref, main fallback), fetch " +
      "the base, and report how far HEAD is ahead of and behind the base.",
    execute: async () => {
      try {
        // Remote resolution: upstream preferred when listed, else origin.
        const remotes = exec(["remote"], cwd)
          .split("\n")
          .map((r) => r.trim())
          .filter((r) => r.length > 0)
        const remote = remotes.includes("upstream") ? "upstream" : "origin"

        // Base branch resolution: remote HEAD symbolic ref, main fallback.
        let baseBranch = "main"
        try {
          const symref = exec(
            ["symbolic-ref", `refs/remotes/${remote}/HEAD`],
            cwd,
          ).trim()
          const prefix = `refs/remotes/${remote}/`
          if (symref.startsWith(prefix) && symref.length > prefix.length) {
            baseBranch = symref.slice(prefix.length)
          }
        } catch {
          // Symbolic ref unset — the main fallback stands.
        }

        const baseRef = `${remote}/${baseBranch}`

        // Fetch the base branch — the tool's only side effect.
        try {
          exec(["fetch", remote, baseBranch], cwd)
        } catch (fetchErr: unknown) {
          const msg =
            fetchErr instanceof Error ? fetchErr.message : String(fetchErr)
          return {
            output: JSON.stringify(
              failure(`Could not fetch ${remote} ${baseBranch}: ${msg}`, {
                retryable: true,
                code: "BASE_SYNC_FETCH",
              }),
            ),
          }
        }

        // Ahead/behind counts versus the fetched base ref. Count
        // failures — exec throws and unparseable output alike —
        // route through a retryable failure envelope: behindBy
        // decides the step-4 sync gate and MUST NOT silently
        // read as 0.
        let aheadBy = 0
        let behindBy = 0
        try {
          const ahead = exec(["rev-list", `${baseRef}..HEAD`, "--count"], cwd)
          aheadBy = parseRevCount(ahead)
          const behind = exec(["rev-list", `HEAD..${baseRef}`, "--count"], cwd)
          behindBy = parseRevCount(behind)
        } catch (countErr: unknown) {
          const msg =
            countErr instanceof Error ? countErr.message : String(countErr)
          return {
            output: JSON.stringify(
              failure(`Could not count commits versus ${baseRef}: ${msg}`, {
                retryable: true,
                code: "BASE_SYNC_COUNT",
              }),
            ),
          }
        }

        const data = { remote, baseBranch, baseRef, aheadBy, behindBy }

        return { output: JSON.stringify(success(data)) }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "BASE_STATE_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 5. uf_finale_push_state — Upstream and divergence info
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_push_state tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_push_state tool.
 */
export function createUfFinalePushStateTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Check push state: whether an upstream exists, and if so, how " +
      "many commits ahead/behind and whether the branches have diverged.",
    execute: async () => {
      try {
        let upstreamExists = false
        try {
          exec(["rev-parse", "--abbrev-ref", "@{u}"], cwd)
          upstreamExists = true
        } catch {
          // No upstream tracking configured.
        }

        // Ahead/behind counts versus the tracking upstream. Count
        // failures — exec throws and unparseable output alike —
        // route through a retryable failure envelope, mirroring
        // the sibling uf_finale_base_state BASE_SYNC_COUNT fix:
        // diverged decides the step-5 push gate's divergence
        // warning and MUST NOT silently read as false through
        // swallowed zero counts.
        let aheadBy = 0
        let behindBy = 0
        let diverged = false

        if (upstreamExists) {
          try {
            const ahead = exec(["rev-list", "@{u}..HEAD", "--count"], cwd)
            aheadBy = parseRevCount(ahead)
            const behind = exec(["rev-list", "HEAD..@{u}", "--count"], cwd)
            behindBy = parseRevCount(behind)
          } catch (countErr: unknown) {
            const msg =
              countErr instanceof Error ? countErr.message : String(countErr)
            return {
              output: JSON.stringify(
                failure(`Could not count commits versus @{u}: ${msg}`, {
                  retryable: true,
                  code: "PUSH_SYNC_COUNT",
                }),
              ),
            }
          }
          diverged = aheadBy > 0 && behindBy > 0
        }

        const data = { upstreamExists, diverged, aheadBy, behindBy }

        return { output: JSON.stringify(success(data)) }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "PUSH_STATE_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 6. uf_finale_pr_context — PR metadata, fork info, templates, issue link
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_pr_context tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_pr_context tool.
 */
export function createUfFinalePrContextTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Gather PR context: existing PR, fork status, PR template sections, " +
      "commit log, diff stat, originating issue (tier-routing route " +
      "decision or .openspec.yaml), and unleash summary.",
    execute: async () => {
      try {
        // Existing PR — uses gh CLI directly (not GitExec).
        let existingPr: { number: number; url: string; title: string } | null =
          null
        try {
          const prJson = execSync(
            "gh pr view --json number,url,title",
            { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
          ).trim()
          const parsed = JSON.parse(prJson) as {
            number: number
            url: string
            title: string
          }
          existingPr = {
            number: parsed.number,
            url: parsed.url,
            title: parsed.title,
          }
        } catch {
          // No existing PR or gh not available.
        }

        // Fork detection — uses gh CLI directly.
        let isFork = false
        let forkParent: { owner: string; repo: string } | null = null
        try {
          const forkOutput = execSync(
            "gh repo view --json isFork --jq '.isFork'",
            { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
          ).trim()
          isFork = forkOutput === "true"
        } catch {
          // gh not available — assume not a fork.
        }

        if (isFork) {
          try {
            const parentJson = execSync(
              "gh repo view --json parent",
              { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
            ).trim()
            const parsed = JSON.parse(parentJson) as {
              parent?: { owner?: { login: string }; name: string }
            }
            if (parsed.parent) {
              forkParent = {
                owner: parsed.parent.owner?.login ?? "",
                repo: parsed.parent.name,
              }
            }
          } catch {
            // Could not fetch parent info.
          }
        }

        // PR template sections.
        const templateSections: string[] = []
        const templatePath = resolve(
          cwd,
          ".github/PULL_REQUEST_TEMPLATE.md",
        )
        try {
          const templateContent = readFileSync(templatePath, "utf-8")
          const headingRegex = /^## (.+)$/gm
          let match: RegExpExecArray | null
          while ((match = headingRegex.exec(templateContent)) !== null) {
            templateSections.push(match[1].trim())
          }
        } catch {
          // No template file — sections stay empty.
        }

        // Commit log between merge-base and HEAD.
        const commitLog: Array<{ sha: string; subject: string }> = []
        let diffStat = ""
        try {
          const mergeBase = exec(
            ["merge-base", "HEAD", "origin/main"],
            cwd,
          )
          // SHA validation — defense-in-depth against poisoned output.
          if (!/^[0-9a-f]{40}$/.test(mergeBase.trim())) {
            throw new Error("Invalid merge-base SHA")
          }
          const logOutput = exec(
            ["log", "--oneline", `${mergeBase}..HEAD`],
            cwd,
          )
          if (logOutput) {
            for (const line of logOutput.split("\n").filter((l) => l.length > 0)) {
              const spaceIdx = line.indexOf(" ")
              if (spaceIdx > 0) {
                commitLog.push({
                  sha: line.slice(0, spaceIdx),
                  subject: line.slice(spaceIdx + 1),
                })
              }
            }
          }
          diffStat = exec(["diff", "--stat", `${mergeBase}..HEAD`], cwd)
        } catch {
          // Could not compute merge-base — likely no origin/main.
        }

        // Originating issue from .openspec.yaml.
        let originatingIssue: number | null = null
        try {
          const openspecPath = resolve(cwd, ".openspec.yaml")
          const openspecContent = readFileSync(openspecPath, "utf-8")
          const issueMatch = openspecContent.match(
            /originating_issue:\s*(\d+)/,
          )
          if (issueMatch) {
            originatingIssue = parseInt(issueMatch[1], 10)
          }
        } catch {
          // No .openspec.yaml — field stays null.
        }

        // Tier-routing issue number (unleash-tier-routing). The route
        // decision records the originating issue in the branch-scoped
        // uf.unleash checkpoint state, which is more precise than the
        // repo-root .openspec.yaml — it takes precedence when present.
        try {
          const routeBranch = exec(
            ["rev-parse", "--abbrev-ref", "HEAD"],
            cwd,
          )
          const routeSlugged = sanitizeSlug(routeBranch)
          const statePath = resolve(
            cwd,
            `.uf/progress/${routeSlugged}/uf.unleash.json`,
          )
          const stateContent = readFileSync(statePath, "utf-8")
          const state = JSON.parse(stateContent) as {
            completed_steps?: unknown
            data?: { issue_number?: unknown }
          }
          // Fail closed: only a recorded tier-routing step carrying a
          // valid positive integer issue number overrides .openspec.yaml.
          if (
            Array.isArray(state.completed_steps) &&
            state.completed_steps.includes("tier-routing") &&
            typeof state.data?.issue_number === "number" &&
            Number.isInteger(state.data.issue_number) &&
            state.data.issue_number > 0
          ) {
            originatingIssue = state.data.issue_number
          }
        } catch {
          // No route-decision state — the .openspec.yaml value stands.
        }

        // Unleash summary artifact.
        let unleashSummary: Record<string, unknown> | null = null
        try {
          const branch = exec(
            ["rev-parse", "--abbrev-ref", "HEAD"],
            cwd,
          )
          const slugged = sanitizeSlug(branch)
          const summaryPath = resolve(
            cwd,
            `.uf/artifacts/unleash-summary/${slugged}.json`,
          )
          const content = readFileSync(summaryPath, "utf-8")
          unleashSummary = JSON.parse(content) as Record<string, unknown>
        } catch {
          // No unleash summary — field stays null.
        }

        const data = {
          existingPr,
          isFork,
          forkParent,
          templateSections,
          commitLog,
          diffStat,
          originatingIssue,
          unleashSummary,
        }

        return { output: JSON.stringify(success(data)) }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "PR_CONTEXT_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 7. uf_finale_create_pr — Create a PR with secure temp file lifecycle
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_create_pr tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_create_pr tool.
 */
export function createUfFinaleCreatePrTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Create a GitHub pull request. Uses a secure temp file for the PR " +
      "body to avoid shell injection. Optionally targets a fork parent repo.",
    args: {
      title: z.string().describe("PR title"),
      body: z.string().describe("PR body content (Markdown)"),
      targetRepo: z.string().optional().describe(
        "Optional target repository (owner/repo) for fork PRs",
      ),
    },
    execute: async (args: {
      title: string
      body: string
      targetRepo?: string
    }) => {
      // Step 0: Validate targetRepo format before any side effects.
      if (args.targetRepo) {
        if (!/^[a-zA-Z0-9._-]+\/[a-zA-Z0-9._-]+$/.test(args.targetRepo)) {
          return {
            output: JSON.stringify(
              failure(
                `Invalid targetRepo format: expected owner/repo`,
                { retryable: false, code: "INVALID_TARGET_REPO" },
              ),
            ),
          }
        }
      }

      let tempDir: string | null = null
      try {
        // Step 1: Create temp dir.
        try {
          tempDir = mkdtempSync(join(tmpdir(), "uf-finale-"))
        } catch (tmpErr: unknown) {
          const msg =
            tmpErr instanceof Error ? tmpErr.message : String(tmpErr)
          return {
            output: JSON.stringify(
              failure(`Could not create temp directory: ${msg}`, {
                code: "TEMP_FILE_ERROR",
              }),
            ),
          }
        }

        // Step 2: Write body to temp file with restricted permissions.
        const bodyPath = join(tempDir, "pr-body.md")
        try {
          writeFileSync(bodyPath, args.body, { mode: 0o600 })
        } catch (writeErr: unknown) {
          const msg =
            writeErr instanceof Error ? writeErr.message : String(writeErr)
          return {
            output: JSON.stringify(
              failure(`Could not write PR body to temp file: ${msg}`, {
                code: "TEMP_FILE_ERROR",
              }),
            ),
          }
        }

        // Step 3: Execute gh pr create.
        // targetRepo was validated in Step 0 before any side effects.
        const repoFlag = args.targetRepo
          ? ` --repo ${args.targetRepo}`
          : ""
        // Shell-escape the title to prevent injection via backticks,
        // $(), ${}, and double quotes.
        const escapedTitle = args.title
          .replace(/\\/g, "\\\\")
          .replace(/"/g, '\\"')
          .replace(/`/g, "\\`")
          .replace(/\$\(/g, "\\$(")
          .replace(/\$\{/g, "\\${")
        const cmd =
          `gh pr create --title "${escapedTitle}" --body-file "${bodyPath}"${repoFlag}`

        const output = execSync(cmd, {
          cwd,
          encoding: "utf-8",
          stdio: ["pipe", "pipe", "pipe"],
        }).trim()

        // Step 4: Parse output for PR number and URL.
        // gh pr create outputs the PR URL on success.
        const urlMatch = output.match(
          /https:\/\/github\.com\/[^\s]+\/pull\/(\d+)/,
        )
        const number = urlMatch ? parseInt(urlMatch[1], 10) : 0
        const url = urlMatch ? urlMatch[0] : output

        return {
          output: JSON.stringify(success({ number, url })),
        }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "PR_CREATE_ERROR" }),
          ),
        }
      } finally {
        // Step 5: Clean up temp file in finally block.
        if (tempDir) {
          try {
            rmSync(tempDir, { recursive: true, force: true })
          } catch {
            // Best-effort cleanup.
          }
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 8. uf_finale_ci_poll — Poll CI checks until completion or timeout
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_ci_poll tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_ci_poll tool.
 */
export function createUfFinaleCiPollTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Poll CI checks for a PR until all complete or timeout. Reports " +
      "pass/fail/conflict status with failed check details.",
    args: {
      prNumber: z.number().describe("PR number to poll CI checks for"),
      timeoutMinutes: z.number().optional().describe(
        "Timeout in minutes (default: 15)",
      ),
    },
    execute: async (args: {
      prNumber: number
      timeoutMinutes?: number
    }) => {
      // Runtime validation for prNumber to prevent shell injection.
      const pr = Math.floor(Number(args.prNumber))
      if (!Number.isFinite(pr) || pr < 1 || pr > 999999) {
        return {
          output: JSON.stringify(
            failure("Invalid PR number", {
              retryable: false,
              code: "INVALID_PR_NUMBER",
            }),
          ),
        }
      }

      const timeout = (args.timeoutMinutes ?? 15) * 60 * 1000
      const pollInterval = 30 * 1000
      const startTime = Date.now()

      try {
        // Check for merge conflicts first.
        try {
          const prViewOutput = execSync(
            `gh pr view ${pr} --json mergeable,mergeStateStatus`,
            { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
          ).trim()
          const prView = JSON.parse(prViewOutput) as {
            mergeable: string
            mergeStateStatus: string
          }
          if (prView.mergeable === "CONFLICTING") {
            return {
              output: JSON.stringify(
                success({
                  status: "conflict",
                  failedChecks: [],
                  mergeable: false,
                  workflowsConfigured: true,
                }),
              ),
            }
          }
        } catch {
          // Could not check mergeability — continue to check polling.
        }

        // Polling loop.
        while (Date.now() - startTime < timeout) {
          try {
            const checksOutput = execSync(
              `gh pr checks ${pr} --json name,state,conclusion,detailsUrl`,
              { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] },
            ).trim()

            const checks = JSON.parse(checksOutput) as Array<{
              name: string
              state: string
              conclusion: string
              detailsUrl: string
            }>

            if (checks.length === 0) {
              // No checks found — determine if workflows are configured.
              const workflowsConfigured = existsSync(
                resolve(cwd, ".github/workflows"),
              )

              if (workflowsConfigured) {
                return {
                  output: JSON.stringify(
                    success({
                      status: "no-checks-workflows-exist",
                      failedChecks: [],
                      mergeable: true,
                      workflowsConfigured: true,
                      skipReason:
                        "No CI checks detected but .github/workflows/ exists — " +
                        "checks may not be triggered for this PR type",
                    }),
                  ),
                }
              }

              // No checks and no workflow files.
              return {
                output: JSON.stringify(
                  success({
                    status: "no-checks-no-workflows",
                    failedChecks: [],
                    mergeable: true,
                    workflowsConfigured: false,
                    skipReason:
                      "No CI checks detected and no .github/workflows/ directory found",
                  }),
                ),
              }
            }

            // Check if all checks are complete.
            const pending = checks.filter(
              (c) => c.state === "PENDING" || c.state === "QUEUED" || c.state === "IN_PROGRESS",
            )

            if (pending.length === 0) {
              // All checks complete — determine pass/fail.
              const failed = checks.filter(
                (c) =>
                  c.conclusion !== "SUCCESS" &&
                  c.conclusion !== "NEUTRAL" &&
                  c.conclusion !== "SKIPPED",
              )

              if (failed.length === 0) {
                return {
                  output: JSON.stringify(
                    success({
                      status: "pass",
                      failedChecks: [],
                      mergeable: true,
                      workflowsConfigured: true,
                    }),
                  ),
                }
              }

              const failedChecks = failed.map((c) => ({
                name: c.name,
                conclusion: c.conclusion,
                url: c.detailsUrl,
              }))
              return {
                output: JSON.stringify(
                  success({
                    status: "fail",
                    failedChecks,
                    mergeable: false,
                    workflowsConfigured: true,
                  }),
                ),
              }
            }

            // Still pending — wait before the next poll.
            await new Promise((r) => setTimeout(r, pollInterval))
          } catch (pollErr: unknown) {
            const msg =
              pollErr instanceof Error ? pollErr.message : String(pollErr)
            return {
              output: JSON.stringify(
                failure(`gh CLI error: ${msg}`, {
                  retryable: true,
                  code: "GH_CLI_ERROR",
                }),
              ),
            }
          }
        }

        // Timeout reached.
        return {
          output: JSON.stringify(
            failure(
              `CI checks did not complete within ${args.timeoutMinutes ?? 15} minutes`,
              { retryable: true, code: "CI_POLL_TIMEOUT" },
            ),
          ),
        }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "CI_POLL_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// 9. uf_finale_return_to_main — Checkout main and pull
// ---------------------------------------------------------------------------

/**
 * Factory for the uf_finale_return_to_main tool.
 *
 * @param exec - Injectable git executor for command execution.
 * @param cwd - Working directory for git operations.
 * @returns The uf_finale_return_to_main tool.
 */
export function createUfFinaleReturnToMainTool(
  exec: GitExec,
  cwd: string,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Return to the main branch by checking out main and pulling " +
      "latest changes.",
    execute: async () => {
      try {
        exec(["checkout", "main"], cwd)
        exec(["pull"], cwd)
        return { output: JSON.stringify(success({ success: true })) }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err)
        return {
          output: JSON.stringify(
            failure(msg, { retryable: false, code: "RETURN_TO_MAIN_ERROR" }),
          ),
        }
      }
    },
  })
}

// ---------------------------------------------------------------------------
// Plugin entry point
// ---------------------------------------------------------------------------

/** UF Finale plugin — provides composite tools for the /uf.finale workflow. */
const UfFinalePlugin = {
  id: "uf-finale",
  server: async (input) => {
    const cwd = input.worktree || input.directory
    return {
      tool: {
        uf_finale_startup: createUfFinaleStartupTool(defaultGitExec, cwd),
        uf_finale_stage: createUfFinaleStageTool(defaultGitExec, cwd),
        uf_finale_commit_context: createUfFinaleCommitContextTool(defaultGitExec, cwd),
        uf_finale_base_state: createUfFinaleBaseStateTool(defaultGitExec, cwd),
        uf_finale_push_state: createUfFinalePushStateTool(defaultGitExec, cwd),
        uf_finale_pr_context: createUfFinalePrContextTool(defaultGitExec, cwd),
        uf_finale_create_pr: createUfFinaleCreatePrTool(defaultGitExec, cwd),
        uf_finale_ci_poll: createUfFinaleCiPollTool(defaultGitExec, cwd),
        uf_finale_return_to_main: createUfFinaleReturnToMainTool(defaultGitExec, cwd),
      },
    }
  },
} satisfies PluginModule

export default UfFinalePlugin
