/**
 * Pure diff-manifest construction shared by review-dispatch tools.
 *
 * Above the 1500-line `DIFF_THRESHOLD`, `build_review_prompts` switches
 * from inlining the full diff into every reviewer prompt to serving a
 * manifest plus on-demand chunk retrieval (OpenSpec change
 * review-dispatch-diff-manifest, issue #877). This module owns the
 * single shared definition of the threshold, the line-counting rule
 * that feeds it, and the deterministic manifest builder the retrieval
 * tool validates against.
 *
 * This module is deliberately free of filesystem, clock, zod, and
 * crypto access: manifest construction is a pure function of the patch
 * text (and optional numstat counts), testable in isolation and usable
 * standalone by any tool (constitution Principles II and IV, design
 * D3/D4/D10).
 */

/**
 * Lines-of-diff threshold at which review dispatch switches delivery
 * modes: inline at or below, manifest strictly above. Value 1500 is
 * gate-fixed (RD-FR-032); the constant moved here from the plugin's
 * `index.ts` so prompt-assembly mode selection and the capture-path
 * `exceeds_threshold` computation share one definition.
 */
export const DIFF_THRESHOLD = 1500

/**
 * Per-chunk byte budget for manifest chunking and retrieval responses.
 * Matches the sibling-evidence per-file cap precedent (design D4):
 * bytes are what the bound actually protects, and byte budgets compose
 * with the manifest's byte-offset integrity checks.
 */
export const MAX_CHUNK_BYTES = 256 * 1024

/**
 * Schema version of the persisted manifest artifact
 * (`<diff_path>.manifest.json`). Bump only on a breaking shape change;
 * readers fail closed on unknown versions.
 */
export const MANIFEST_SCHEMA_VERSION = 1

/**
 * Persona-relevance scope hints, aligned with the scope names in
 * `reviewer-capabilities.yaml` (RD-FR-033). Hints are guidance for
 * retrieval prioritization only — they never filter, omit, or reorder
 * manifest entries.
 */
export type ReviewScopeHint =
  | "security"
  | "dependencies"
  | "standard"
  | "cli-ux"
  | "ci-cd"
  | "documentation"
  | "test-quality"

/**
 * One retrievable chunk of a file section: a byte range [start, end)
 * into the source patch content. Boundaries are hunk-aligned (they
 * fall on `@@` hunk headers or file-section boundaries) except for
 * mid-hunk splits of oversized single hunks, which are annotated.
 */
export interface DiffManifestChunk {
  /** Deterministic identifier: `<file-path>#<index>` (1-based, per file section). */
  readonly id: string
  /** Inclusive start byte offset into the patch content. */
  readonly byteStart: number
  /** Exclusive end byte offset into the patch content. */
  readonly byteEnd: number
  /** True when this chunk is part of a mid-hunk split of an oversized single hunk. */
  readonly midHunkSplit: boolean
}

/**
 * One file section of the patch: path, counts, flags, chunks, hints.
 * Entries appear in patch order — never filtered or reordered (RD-FR-033).
 */
export interface DiffManifestEntry {
  /** Repository-relative file path (post-rename for renamed files). */
  readonly path: string
  /** Added lines (numstat when provided, else derived from hunks). */
  readonly additions: number
  /** Deleted lines (numstat when provided, else derived from hunks). */
  readonly deletions: number
  /** True when the section declares binary content (`Binary files ... differ` / `GIT binary patch`). */
  readonly binary: boolean
  /** True when the section carries rename headers (`rename from`/`rename to`/`similarity index`). */
  readonly rename: boolean
  /** Retrievable chunks for this file section, in patch order. */
  readonly chunks: readonly DiffManifestChunk[]
  /** Persona-relevance scope hints (guidance only; at least one, default `standard`). */
  readonly hints: readonly ReviewScopeHint[]
}

/**
 * Optional per-file counts supplied by the caller when the capture
 * path already computed them (numstat or PR files API). Counts here
 * are authoritative over hunk-derived counts for the matching path;
 * sections without a matching numstat entry derive counts from hunks.
 */
export interface NumstatInput {
  readonly path: string
  readonly additions: number
  readonly deletions: number
}

/**
 * The persisted manifest artifact written by `build_review_prompts` in
 * manifest mode as `<diff_path>.manifest.json` (0o600). The digest and
 * diff path are computed at the plugin layer (crypto and filesystem
 * live there, not in this pure module); `read_diff_chunk` re-verifies
 * `diff_bytes` and `diff_sha256` against the diff before serving.
 */
export interface PersistedDiffManifest {
  readonly schema_version: number
  readonly diff_path: string
  readonly diff_bytes: number
  readonly diff_sha256: string
  readonly entries: readonly DiffManifestEntry[]
}

/**
 * Count the lines in a diff's content with a single shared rule
 * (RD-FR-032): CRLF and lone-CR terminators normalize to LF, and a
 * trailing terminator does not introduce an additional line. This is
 * the one definition both the capture-path `exceeds_threshold`
 * computation and prompt-assembly mode selection consume, so the two
 * cannot diverge at the 1500-line boundary.
 *
 * @param diffContent The full diff text.
 * @returns The number of real lines (0 for empty content).
 */
export function countDiffLines(diffContent: string): number {
  if (diffContent.length === 0) {
    return 0
  }
  // Normalize CRLF and lone CR to LF so terminator style cannot shift
  // the boundary (a CRLF diff counts identically to its LF twin).
  const normalized = diffContent.replace(/\r\n?/g, "\n")
  const segments = normalized.split("\n")
  // "a\nb\n" splits to 3 segments but is 2 lines; "a\nb" splits to 2
  // and is 2 lines. A trailing terminator never adds a line.
  return normalized.endsWith("\n") ? segments.length - 1 : segments.length
}

/**
 * Classify a repository-relative path into persona-relevance scope
 * hints (design D10). Deterministic path-pattern classification; a
 * path may carry several hints, and every path yields at least
 * `standard`. Command-surface files (`.opencode/commands/`, `cmd/`,
 * `cli/`) carry the `*.md` extension, so the cli-ux pattern takes
 * precedence over the documentation pattern for those paths — they
 * classify as cli-ux, not documentation.
 *
 * Hints are guidance for retrieval ordering only: the manifest lists
 * every file for every agent (explicit planning decision —
 * full-changeset visibility).
 *
 * @param path Repository-relative file path.
 * @returns Ordered unique scope hints; never empty.
 */
export function classifyPathHints(path: string): readonly ReviewScopeHint[] {
  const lower = path.toLowerCase()
  const base = lower.slice(lower.lastIndexOf("/") + 1)
  const hints: ReviewScopeHint[] = []
  let cliUx = false

  // ci-cd: workflow definitions and CI service files.
  if (
    lower.startsWith(".github/workflows/") ||
    lower.startsWith(".circleci/") ||
    [".gitlab-ci.yml", ".travis.yml", "jenkinsfile", "azure-pipelines.yml", "cloudbuild.yaml", ".drone.yml"].includes(
      base,
    )
  ) {
    hints.push("ci-cd")
  }

  // test-quality: test and fixture locations. Covers Go-style
  // `*_test.*` and spec-style `*_spec.*` basenames plus test/fixture
  // directories.
  if (
    /_test\.[^/]+$/.test(base) ||
    /_spec\.[^/]+$/.test(base) ||
    /(^|\/)(test|tests|__tests__|fixtures)(\/|$)/.test(lower)
  ) {
    hints.push("test-quality")
  }

  // cli-ux: command-surface paths (CLI entry points and command
  // definitions). Checked before documentation so command files that
  // carry the *.md extension classify as cli-ux, not documentation.
  if (/(^|\/)(cmd|cli)(\/|$)/.test(lower) || lower.startsWith(".opencode/commands/")) {
    hints.push("cli-ux")
    cliUx = true
  }

  // dependencies: lockfiles and dependency manifests.
  if (
    [
      "package.json",
      "package-lock.json",
      "yarn.lock",
      "pnpm-lock.yaml",
      "bun.lockb",
      "go.mod",
      "go.sum",
      "cargo.lock",
      "cargo.toml",
      "gemfile",
      "gemfile.lock",
      "pipfile",
      "pipfile.lock",
      "poetry.lock",
      "requirements.txt",
      "flake.lock",
    ].includes(base)
  ) {
    hints.push("dependencies")
  }

  // security: security-adjacent path segments (auth, crypto, secrets,
  // permissions). Segment equality avoids false hits (an "author"
  // directory is not "auth").
  if (
    lower
      .split("/")
      .some((segment) =>
        ["auth", "authentication", "crypto", "cryptography", "secret", "secrets", "security", "permission", "permissions"].includes(
          segment,
        ),
      )
  ) {
    hints.push("security")
  }

  // documentation: docs locations and Markdown files. Skipped for
  // paths already classified cli-ux (command files carry *.md; the
  // cli-ux pattern takes precedence over the documentation pattern).
  if (
    !cliUx &&
    (base.endsWith(".md") ||
      lower.startsWith("docs/") ||
      lower.includes("/docs/") ||
      lower.startsWith("openspec/") ||
      lower.startsWith("specs/"))
  ) {
    hints.push("documentation")
  }

  if (hints.length === 0) {
    hints.push("standard")
  }
  return hints
}

/**
 * Build the diff manifest entries for a patch: one entry per
 * `diff --git` file section (including the synthesized new-file
 * sections the capture tools emit for untracked files — they are
 * standard sections and parse naturally, design D3), in patch order.
 *
 * Chunking is hunk-aligned under the 256 KiB budget (design D4):
 * chunk boundaries fall on `@@` hunk headers or file-section
 * boundaries so retrieved content always carries complete hunk
 * context. A single hunk larger than the budget splits mid-hunk as a
 * last resort, line-aligned, with every resulting piece annotated
 * `midHunkSplit`. A single line larger than the budget (minified
 * content) byte-splits as the final last resort so no chunk ever
 * exceeds the cap. Chunk ids are deterministic (`<file-path>#<index>`,
 * 1-based within the file section; ids parse by splitting at the last
 * `#`, so paths containing `#` stay unambiguous).
 *
 * @param patchContent The full unified-diff text (`diff --git` format).
 * @param numstat Optional authoritative per-file counts; matched by
 *   exact path. Binary sections without a numstat entry carry 0/0
 *   counts (their content has no hunks to derive from).
 * @returns Manifest entries in patch order.
 */
export function buildDiffManifest(
  patchContent: string,
  numstat?: readonly NumstatInput[],
): readonly DiffManifestEntry[] {
  const lines = walkLines(patchContent)
  const counts = new Map<string, NumstatInput>()
  for (const entry of numstat ?? []) {
    counts.set(entry.path, entry)
  }

  const entries: DiffManifestEntry[] = []
  let sectionStart = -1
  for (let i = 0; i <= lines.length; i++) {
    if (i < lines.length && lines[i].text.startsWith("diff --git ")) {
      if (sectionStart >= 0) {
        entries.push(buildEntry(lines.slice(sectionStart, i), counts))
      }
      sectionStart = i
    } else if (i === lines.length && sectionStart >= 0) {
      entries.push(buildEntry(lines.slice(sectionStart, i), counts))
    }
  }
  return entries
}

/** A patch line plus its byte range (end exclusive, terminator included). */
interface PatchLine {
  readonly text: string
  readonly byteStart: number
  readonly byteEnd: number
}

/**
 * Split content into lines carrying byte offsets. Byte offsets are
 * UTF-8 (the retrieval tool slices the on-disk file, which is UTF-8);
 * an ASCII fast path keeps the common case cheap.
 */
function walkLines(content: string): PatchLine[] {
  const lines: PatchLine[] = []
  let lineStart = 0
  let byteStart = 0
  let i = 0
  while (i < content.length) {
    const ch = content[i]
    if (ch === "\n" || ch === "\r") {
      let terminatorBytes = 1
      if (ch === "\r" && content[i + 1] === "\n") {
        terminatorBytes = 2
      }
      const text = content.slice(lineStart, i)
      const textBytes = byteLength(text)
      lines.push({ text, byteStart, byteEnd: byteStart + textBytes + terminatorBytes })
      byteStart += textBytes + terminatorBytes
      i += terminatorBytes
      lineStart = i
    } else {
      i++
    }
  }
  if (lineStart < content.length) {
    const text = content.slice(lineStart)
    lines.push({ text, byteStart, byteEnd: byteStart + byteLength(text) })
  }
  return lines
}

/** UTF-8 byte length of a string (ASCII fast path). */
function byteLength(text: string): number {
  let ascii = true
  for (let i = 0; i < text.length; i++) {
    if (text.charCodeAt(i) > 0x7f) {
      ascii = false
      break
    }
  }
  if (ascii) {
    return text.length
  }
  return new TextEncoder().encode(text).length
}

/** Build one manifest entry from a file section's line span. */
function buildEntry(sectionLines: readonly PatchLine[], counts: ReadonlyMap<string, NumstatInput>): DiffManifestEntry {
  const headers = collectSectionHeaders(sectionLines)
  const path = resolveSectionPath(headers)

  let additions = 0
  let deletions = 0
  let inHunk = false
  for (const line of sectionLines) {
    if (line.text.startsWith("@@")) {
      inHunk = true
      continue
    }
    if (!inHunk) {
      continue
    }
    if (line.text.startsWith("+") && !line.text.startsWith("+++")) {
      additions++
    } else if (line.text.startsWith("-") && !line.text.startsWith("---")) {
      deletions++
    }
  }
  // Numstat counts are authoritative for the matching path; sections
  // without a match keep their hunk-derived counts (0/0 for binary
  // sections — no hunks to derive from).
  const numstat = counts.get(path)
  if (numstat !== undefined) {
    additions = numstat.additions
    deletions = numstat.deletions
  }

  return {
    path,
    additions,
    deletions,
    binary: headers.binary,
    rename: headers.rename,
    chunks: chunkSection(sectionLines, path),
    hints: classifyPathHints(path),
  }
}

/** Section header facts extracted from the lines before the first hunk. */
interface SectionHeaders {
  readonly diffLine: string | null
  readonly oldPath: string | null
  readonly newPath: string | null
  readonly renameFrom: string | null
  readonly renameTo: string | null
  readonly binary: boolean
  readonly rename: boolean
}

/** Collect header facts from a section's pre-hunk lines. */
function collectSectionHeaders(sectionLines: readonly PatchLine[]): SectionHeaders {
  let diffLine: string | null = null
  let oldPath: string | null = null
  let newPath: string | null = null
  let renameFrom: string | null = null
  let renameTo: string | null = null
  let binary = false
  let rename = false
  for (const line of sectionLines) {
    const text = line.text
    if (text.startsWith("@@")) {
      break // hunk content starts; headers are done
    }
    if (text.startsWith("diff --git ")) {
      diffLine = text
    } else if (text.startsWith("--- ")) {
      oldPath = stripPrefixPath(text.slice(4))
    } else if (text.startsWith("+++ ")) {
      newPath = stripPrefixPath(text.slice(4))
    } else if (text.startsWith("rename from ")) {
      renameFrom = text.slice("rename from ".length)
      rename = true
    } else if (text.startsWith("rename to ")) {
      renameTo = text.slice("rename to ".length)
      rename = true
    } else if (text.startsWith("similarity index")) {
      rename = true
    } else if (text.startsWith("Binary files") || text.startsWith("GIT binary patch")) {
      binary = true
    }
  }
  return { diffLine, oldPath, newPath, renameFrom, renameTo, binary, rename }
}

/** Strip the a//b prefix (or /dev/null marker) from a header path. */
function stripPrefixPath(raw: string): string | null {
  // Guard against tab-separated timestamps some diff flavors append.
  const trimmed = raw.split("\t")[0].trim()
  if (trimmed === "/dev/null" || trimmed === "") {
    return null
  }
  if (trimmed.startsWith("a/") || trimmed.startsWith("b/")) {
    return trimmed.slice(2)
  }
  return trimmed
}

/**
 * Resolve the entry path for a section. Priority: the `+++` new path
 * (the authoritative post-change path), then `rename to`, then the
 * `diff --git` b-side, then `rename from`, then the `---` old path
 * (deleted files carry their only path there).
 */
function resolveSectionPath(headers: SectionHeaders): string {
  if (headers.newPath !== null) {
    return headers.newPath
  }
  if (headers.renameTo !== null) {
    return headers.renameTo
  }
  const fromDiffLine = diffLineBPath(headers.diffLine)
  if (fromDiffLine !== null) {
    return fromDiffLine
  }
  if (headers.renameFrom !== null) {
    return headers.renameFrom
  }
  if (headers.oldPath !== null) {
    return headers.oldPath
  }
  return "<unknown>"
}

/**
 * Extract the b-side path from a `diff --git a/<old> b/<new>` line.
 * Paths with spaces are quoted by git (C-style); the simple split
 * below covers the unquoted common case, and callers fall back to
 * `+++`/`rename to` lines — which are more authoritative anyway.
 */
function diffLineBPath(diffLine: string | null): string | null {
  if (diffLine === null) {
    return null
  }
  const rest = diffLine.slice("diff --git ".length)
  const separator = rest.indexOf(" b/")
  if (separator === -1) {
    return null
  }
  const bPath = rest.slice(separator + 3)
  return bPath === "" ? null : bPath
}

/**
 * Chunk a file section under `MAX_CHUNK_BYTES` with hunk-aligned
 * boundaries (design D4). The section's header lines (everything
 * before the first `@@`) join the first chunk so retrieved content
 * carries the file headers. At every hunk header the walker decides:
 *
 * 1. A chunk that started mid-hunk (a split continuation) closes at
 *    the next hunk header so split pieces stay self-contained.
 * 2. Otherwise, a chunk carrying at least one unit closes when
 *    appending the entire next hunk would exceed the budget.
 *
 * A hunk whose lines overflow the budget mid-hunk splits line-aligned
 * as a last resort; every piece of a split (including the piece that
 * begins at the hunk header, since it ends mid-hunk) is annotated
 * `midHunkSplit`. A single line larger than the budget byte-splits so
 * no chunk ever exceeds the cap — refusing to serve oversized content
 * would blind reviewers to exactly the files most likely to hide
 * findings.
 */
function chunkSection(sectionLines: readonly PatchLine[], path: string): DiffManifestChunk[] {
  const chunks: DiffManifestChunk[] = []
  let chunkStart = 0
  let chunkBytes = 0
  let chunkStartedMidHunk = false

  const closeChunk = (endExclusive: number, midHunkSplit: boolean): void => {
    if (endExclusive <= chunkStart) {
      return
    }
    chunks.push({
      id: `${path}#${chunks.length + 1}`,
      byteStart: sectionLines[chunkStart].byteStart,
      byteEnd: sectionLines[endExclusive - 1].byteEnd,
      midHunkSplit,
    })
    chunkStart = endExclusive
    chunkBytes = 0
  }

  let i = 0
  while (i < sectionLines.length) {
    const isHunkHeader = sectionLines[i].text.startsWith("@@")
    if (isHunkHeader && i > chunkStart) {
      if (chunkStartedMidHunk || (chunkBytes > 0 && chunkBytes + hunkBytes(i, sectionLines) > MAX_CHUNK_BYTES)) {
        closeChunk(i, chunkStartedMidHunk)
        chunkStartedMidHunk = false
        continue // re-evaluate the same header with an empty chunk
      }
    }

    const line = sectionLines[i]
    const lineBytes = line.byteEnd - line.byteStart
    if (chunkBytes + lineBytes <= MAX_CHUNK_BYTES) {
      chunkBytes += lineBytes
      i++
      continue
    }

    // Byte overflow mid-hunk: the current hunk must split. The chunk
    // being closed ends mid-hunk (annotated), and the continuation
    // starts mid-hunk (tracked so its own close is annotated too).
    if (i > chunkStart) {
      closeChunk(i, true)
    }
    chunkStartedMidHunk = true
    if (lineBytes > MAX_CHUNK_BYTES) {
      // Pathological single line larger than the budget (minified
      // content): byte-split it as the final last resort.
      let offset = 0
      while (offset < lineBytes) {
        const pieceEnd = Math.min(offset + MAX_CHUNK_BYTES, lineBytes)
        chunks.push({
          id: `${path}#${chunks.length + 1}`,
          byteStart: line.byteStart + offset,
          byteEnd: line.byteStart + pieceEnd,
          midHunkSplit: true,
        })
        offset = pieceEnd
      }
      chunkStart = i + 1
      chunkBytes = 0
    } else {
      chunkBytes = lineBytes // the overflowing line opens the next piece
    }
    i++
  }
  closeChunk(sectionLines.length, chunkStartedMidHunk)
  return chunks
}

/** Byte span of the hunk starting at `startIdx` (through its last line). */
function hunkBytes(startIdx: number, sectionLines: readonly PatchLine[]): number {
  const end = unitEnd(startIdx, sectionLines)
  return sectionLines[end - 1].byteEnd - sectionLines[startIdx].byteStart
}

/** Index (exclusive) at which the hunk starting at `startIdx` ends. */
function unitEnd(startIdx: number, sectionLines: readonly PatchLine[]): number {
  for (let i = startIdx + 1; i < sectionLines.length; i++) {
    if (sectionLines[i].text.startsWith("@@")) {
      return i
    }
  }
  return sectionLines.length
}
