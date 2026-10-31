/**
 * Result envelope types for uf-workflow plugin tools.
 *
 * Every tool response is wrapped in a discriminated union
 * so callers can branch on `status` without guessing.
 *
 * Error messages SHOULD NOT include file paths or stack
 * traces — keep them user-facing and sanitized.
 */

import { execFileSync } from "node:child_process"

/** Successful tool result with optional typed payload. */
export interface ToolSuccess<T = void> {
  readonly status: "ok"
  readonly data?: T
}

/** Failed tool result with structured error metadata. */
export interface ToolFailure {
  readonly status: "error"
  readonly message: string
  readonly retryable: boolean
  readonly code?: string
  /**
   * Child session id, present only on delegation failures
   * (DELEGATE_FAILED / DELEGATE_CANCELLED) where the executor
   * created a child session before failing. Omitted when no
   * child session ever existed.
   */
  readonly child_session_id?: string
}

/** Discriminated union returned by every tool handler. */
export type ToolResult<T = void> = ToolSuccess<T> | ToolFailure

/** Build a success envelope, optionally carrying data. */
export function success<T = void>(data?: T): ToolSuccess<T> {
  if (data === undefined) {
    return { status: "ok" } as ToolSuccess<T>
  }
  return { status: "ok", data }
}

/** Build a failure envelope. `retryable` defaults to false. */
export function failure(
  message: string,
  options?: { retryable?: boolean; code?: string },
): ToolFailure {
  const result: ToolFailure = {
    status: "error",
    message,
    retryable: options?.retryable ?? false,
    ...(options?.code !== undefined && { code: options.code }),
  }
  return result
}

// ---------------------------------------------------------------------------
// Shared utilities (DRY: used by both uf-workflow and uf-finale plugins)
// ---------------------------------------------------------------------------

/** Git execution function signature for testability (args array, no shell). */
export type GitExec = (args: string[], cwd: string) => string

/** Default git executor using execFileSync (no shell interpolation). */
export const defaultGitExec: GitExec = (args: string[], cwd: string): string =>
  execFileSync("git", args, { cwd, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] }).trim()

/** Maximum slug length after sanitization. */
const MAX_SLUG_LENGTH = 100

/**
 * Sanitize a string to a filesystem-safe slug (FR-004).
 * Replaces `/` with `-`, strips characters outside [a-zA-Z0-9._-],
 * collapses dot runs, and truncates to 100 characters.
 * Throws if the result is empty after sanitization.
 */
export function sanitizeSlug(input: string): string {
  const result = input
    .replace(/\//g, "-")
    .replace(/[^a-zA-Z0-9._-]/g, "")
    .replace(/\.{2,}/g, ".")
    .slice(0, MAX_SLUG_LENGTH)
  if (result === "") {
    const safe = input.slice(0, 100).replace(/[\x00-\x1f]/g, "?")
    throw new Error(
      `Invalid input: "${safe}" produces empty slug after sanitization`,
    )
  }
  return result
}

/**
 * Normalize a raw model identifier to a canonical short name.
 * Algorithm (6 steps):
 * 1. Strip everything before and including the last `/`
 * 2. Strip everything after and including the first `@`
 * 3. Strip trailing date suffix `-YYYYMMDD`
 * 4. Repeatedly strip chained trailing `-N` (only when preceded by digit)
 * 5. Validate result contains only `[a-zA-Z0-9._-]`
 * 6. Return `unknown-model` if empty or invalid
 */
export function normalizeModelName(raw: string): string {
  if (!raw || raw.trim() === "") return "unknown-model"

  // Step 1: Strip before last `/`
  let name = raw
  const lastSlash = name.lastIndexOf("/")
  if (lastSlash !== -1) {
    name = name.slice(lastSlash + 1)
  }

  // Step 2: Strip after first `@`
  const atIndex = name.indexOf("@")
  if (atIndex !== -1) {
    name = name.slice(0, atIndex)
  }

  // Step 3: Strip trailing `-YYYYMMDD`
  name = name.replace(/-\d{8}$/, "")

  // Step 4: Repeatedly strip trailing `-N` when preceded by digit
  while (/\d-\d$/.test(name)) {
    name = name.slice(0, -2)
  }

  // Step 5: Validate
  const isValid = /^[a-zA-Z0-9._-]+$/.test(name)

  // Step 6: Fallback
  if (!name || !isValid) return "unknown-model"

  return name
}
