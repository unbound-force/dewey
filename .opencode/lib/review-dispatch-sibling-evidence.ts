import { Buffer } from "node:buffer"
import { execFile } from "node:child_process"
import { createHash } from "node:crypto"
import { mkdir, readFile, realpath, stat } from "node:fs/promises"
import { dirname, isAbsolute, relative, resolve } from "node:path"
import process from "node:process"

import { tool } from "@opencode-ai/plugin"
import { z } from "zod"

const SIBLING_REPOSITORIES_PATH = ".uf/sibling-repos.yaml"
const MAX_SIBLING_FILES = 20
const MAX_SIBLING_FILE_BYTES = 256 * 1024
const MAX_SIBLING_TOTAL_BYTES = 1024 * 1024

/** Exact opening delimiter for content that must remain untrusted sibling evidence. */
export const UNTRUSTED_SIBLING_EVIDENCE_BEGIN = "<!-- uf-untrusted-sibling-evidence:v1 -->"

/** Exact closing delimiter for content that must remain untrusted sibling evidence. */
export const UNTRUSTED_SIBLING_EVIDENCE_END = "<!-- /uf-untrusted-sibling-evidence -->"

/** Narrow YAML parsing boundary supplied by Bun in production and JSON fixtures in tests. */
export type SiblingYamlParser = (text: string) => unknown

function uniqueArray<T extends z.ZodType>(item: T, maximum: number): z.ZodArray<T> {
  return z
    .array(item)
    .max(maximum)
    .superRefine((values, context) => {
      if (new Set(values).size !== values.length) {
        context.addIssue({ code: "custom", message: "items must be unique" })
      }
    })
}

function boundedUTF8String(maximumBytes: number): z.ZodString {
  return z.string().min(1).superRefine((value, context) => {
    if (Buffer.byteLength(value, "utf8") > maximumBytes) {
      context.addIssue({ code: "custom", message: `must be at most ${maximumBytes} UTF-8 bytes` })
    }
  })
}

function validRelativeDeclarationPath(value: string): boolean {
  const segments = value.split("/")
  return (
    !value.startsWith("/") &&
    !/^[A-Za-z]:[\\/]/.test(value) &&
    !value.includes("\\") &&
    !value.includes("\0") &&
    segments.every((segment) => segment !== "" && segment !== "." && segment !== "..")
  )
}

function validLocalDeclarationPath(value: string): boolean {
  const normalized = value.replaceAll("\\", "/")
  const drive = /^[A-Za-z]:\//.test(normalized)
  const withoutRoot = normalized.startsWith("/") ? normalized.slice(1) : drive ? normalized.slice(3) : normalized
  const segments = withoutRoot.split("/")
  return !value.includes("\0") && segments.every((segment) => segment !== "" && segment !== "." && segment !== "..")
}

function normalizeGitHubRepositoryURL(value: string): string {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    throw new Error("must be an absolute HTTPS GitHub repository URL")
  }
  if (
    parsed.protocol !== "https:" ||
    parsed.hostname.toLowerCase() !== "github.com" ||
    parsed.port !== "" ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== ""
  ) {
    throw new Error("must be a credential-free HTTPS GitHub URL without port, query, or fragment")
  }
  const path = parsed.pathname.replace(/\/+$/, "").replace(/\.git$/, "")
  const segments = path.split("/").filter((segment) => segment !== "")
  if (segments.length !== 2 || segments.some((segment) => !/^[A-Za-z0-9_.-]+$/.test(segment))) {
    throw new Error("must identify one GitHub owner and repository")
  }
  return `https://github.com/${segments[0]}/${segments[1]}`
}

const GitHubRepositoryURLSchema = z
  .string()
  .min(1)
  .max(512)
  .superRefine((value, context) => {
    try {
      normalizeGitHubRepositoryURL(value)
    } catch (error: unknown) {
      context.addIssue({ code: "custom", message: error instanceof Error ? error.message : "invalid GitHub URL" })
    }
  })
const RepositoryRelativePathSchema = boundedUTF8String(512).refine(
  validRelativeDeclarationPath,
  "must be a safe target-root-relative path",
)
const LocalPathSchema = boundedUTF8String(512).refine(
  validLocalDeclarationPath,
  "must be a safe absolute or target-root-relative path",
)
const ContractGlobSchema = boundedUTF8String(256).refine(
  validRelativeDeclarationPath,
  "must be a safe repository-relative glob",
)
const SiblingSchema = z
  .object({
    name: z.string().regex(/^[a-z][a-z0-9-]{0,62}$/),
    url: GitHubRepositoryURLSchema,
    default_branch: z.string().min(1).max(128).regex(/^[ -~]+$/),
    role: z.enum(["downstream-consumer", "upstream-source", "sibling"]),
    fetch: z.enum(["clone-on-review", "contract-only"]),
    contracts: uniqueArray(ContractGlobSchema, 20).min(1),
    local_paths: uniqueArray(LocalPathSchema, 4).optional(),
    clone_path: RepositoryRelativePathSchema.optional(),
    cache_path: RepositoryRelativePathSchema.optional(),
    notes: boundedUTF8String(1024).optional(),
  })
  .strict()
const SiblingRepositoriesSchema = z
  .object({ version: z.literal(1), siblings: z.array(SiblingSchema).max(32) })
  .strict()
const AcquireSiblingEvidenceInputSchema = z.object({}).strict()

type SiblingDeclaration = z.output<typeof SiblingSchema>

/** Closed version 1 sibling repository declaration. */
export type SiblingRepositories = z.output<typeof SiblingRepositoriesSchema>

/** Empty, closed input accepted by the project-scoped acquisition boundary. */
export type AcquireSiblingEvidenceInput = z.input<typeof AcquireSiblingEvidenceInputSchema>

/** Result of one injected command without shell interpretation. */
export interface CommandResult {
  readonly stdout: string
  readonly stderr: string
}

/** Options supplied to an injected command runner. */
export interface CommandOptions {
  readonly cwd?: string
  readonly env?: Readonly<Record<string, string>>
  readonly timeout?: number
  readonly signal?: AbortSignal
}

/** Minimal filesystem metadata used by acquisition. */
export interface AcquisitionFileInfo {
  readonly size: number
  readonly isDirectory: () => boolean
  readonly isFile: () => boolean
}

/** Injectable filesystem and command operations for provider-free acquisition. */
export interface SiblingAcquisitionDependencies {
  readonly projectRoot: string
  readonly readText: (relativePath: string) => Promise<string>
  readonly parseYaml: SiblingYamlParser
  readonly realpath: (path: string) => Promise<string>
  readonly stat: (path: string) => Promise<AcquisitionFileInfo>
  readonly readFile: (path: string) => Promise<Uint8Array>
  readonly mkdir: (path: string, options: { readonly recursive: true }) => Promise<unknown>
  readonly runCommand: (command: string, args: readonly string[], options?: CommandOptions) => Promise<CommandResult>
}

/** Stable reason recorded when a candidate, network operation, or file is rejected. */
export interface SiblingEvidenceRejection {
  readonly scope: "candidate" | "network" | "file"
  readonly subject: string
  readonly reason_code: string
  readonly reason: string
}

/** Immutable metadata for one accepted sibling evidence file. */
export interface SiblingEvidenceFile {
  readonly sibling: string
  readonly commit: string
  readonly path: string
  readonly sha256: string
  readonly source_mode: "local" | "clone" | "cache" | "offline-cache"
}

/** Structured acquisition outcome for one declared sibling. */
export interface SiblingEvidenceRepositoryResult {
  readonly sibling: string
  readonly status: "available" | "unavailable"
  readonly commit: string | null
  readonly source_mode: "local" | "clone" | "cache" | "offline-cache" | null
  readonly source_candidate: string | null
  readonly evidence: readonly SiblingEvidenceFile[]
  readonly rejections: readonly SiblingEvidenceRejection[]
  readonly unavailable_reason: string | null
}

/** Deterministic output returned by acquire_sibling_evidence. */
export interface AcquireSiblingEvidenceResult {
  readonly version: 1
  readonly status: "ok" | "invalid-config"
  readonly prompt: string
  readonly siblings: readonly SiblingEvidenceRepositoryResult[]
  readonly errors: readonly string[]
}

type SiblingSourceMode = Exclude<SiblingEvidenceRepositoryResult["source_mode"], null>

interface CandidateDeclaration {
  readonly label: string
  readonly path: string
  readonly mode: Exclude<SiblingSourceMode, "offline-cache">
  readonly constrainedToProject: boolean
}

interface VerifiedCandidate extends CandidateDeclaration {
  readonly root: string
  readonly commit: string
}

interface EvidenceBudget {
  files: number
  bytes: number
}

interface CollectedEvidence {
  readonly files: readonly SiblingEvidenceFile[]
  readonly sections: readonly string[]
}

const CREDENTIAL_DISABLED_ENV: Readonly<Record<string, string>> = {
  GIT_TERMINAL_PROMPT: "0",
  GCM_INTERACTIVE: "never",
  GIT_ASKPASS: "",
  SSH_ASKPASS: "",
}

function formatValidationError(error: z.ZodError): string {
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
    if (character === "'" && !doubleQuoted) singleQuoted = !singleQuoted
    else if (character === '"' && !singleQuoted && line[index - 1] !== "\\") doubleQuoted = !doubleQuoted
    else if (character === "#" && !singleQuoted && !doubleQuoted) return line.slice(0, index)
  }
  return line
}

function rejectYamlReferences(text: string): void {
  for (const line of text.split(/\r?\n/)) {
    const content = stripYamlComment(line)
    if (/^\s*<<\s*:/.test(content) || /(^|[\s:[{,])[&*][A-Za-z0-9_-]+(?=$|[\s,\]}])/.test(content)) {
      throw new Error("sibling repositories contains forbidden YAML anchor, alias, or merge reference")
    }
  }
}

function parseYamlDocument(text: string, parser: SiblingYamlParser): unknown {
  rejectYamlReferences(text)
  try {
    return parser(text)
  } catch (error: unknown) {
    throw new Error(`parse sibling repositories: ${error instanceof Error ? error.message : "unknown parser error"}`)
  }
}

function compareLexical(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

/**
 * Parses and closed-validates a version 1 sibling repository declaration.
 * @param text YAML or JSON declaration text.
 * @param parser Injected YAML parser.
 * @returns The validated declaration in lexical sibling order.
 */
export function parseSiblingRepositories(text: string, parser: SiblingYamlParser): SiblingRepositories {
  const parsed = SiblingRepositoriesSchema.safeParse(parseYamlDocument(text, parser))
  if (!parsed.success) {
    throw new Error(`validate sibling repositories: ${formatValidationError(parsed.error)}`)
  }
  const names = new Set<string>()
  for (const sibling of parsed.data.siblings) {
    if (names.has(sibling.name)) throw new Error(`duplicate sibling entry for ${sibling.name}`)
    names.add(sibling.name)
  }
  return {
    version: 1,
    siblings: [...parsed.data.siblings].sort((left, right) => compareLexical(left.name, right.name)),
  }
}

function isPathWithin(root: string, candidate: string): boolean {
  const difference = relative(root, candidate)
  const parentPrefix = process.platform === "win32" ? "..\\" : "../"
  return difference === "" || (!difference.startsWith(parentPrefix) && difference !== ".." && !isAbsolute(difference))
}

function candidateDeclarations(sibling: SiblingDeclaration, projectRoot: string): CandidateDeclaration[] {
  const candidates: CandidateDeclaration[] = []
  for (const [index, path] of (sibling.local_paths ?? []).entries()) {
    candidates.push({
      label: `local_paths[${index}]`,
      path: isAbsolute(path) ? path : resolve(projectRoot, path),
      mode: "local",
      constrainedToProject: !isAbsolute(path),
    })
  }
  if (sibling.clone_path !== undefined) {
    candidates.push({
      label: "clone_path",
      path: resolve(projectRoot, sibling.clone_path),
      mode: "clone",
      constrainedToProject: true,
    })
  }
  if (sibling.cache_path !== undefined) {
    candidates.push({
      label: "cache_path",
      path: resolve(projectRoot, sibling.cache_path),
      mode: "cache",
      constrainedToProject: true,
    })
  }
  return candidates
}

const GIT_COMMAND_TIMEOUT_MS = 60_000

async function runGit(
  dependencies: SiblingAcquisitionDependencies,
  args: readonly string[],
  cwd?: string,
): Promise<CommandResult> {
  return dependencies.runCommand("git", args, { cwd, env: CREDENTIAL_DISABLED_ENV, timeout: GIT_COMMAND_TIMEOUT_MS })
}

function rejection(
  scope: SiblingEvidenceRejection["scope"],
  subject: string,
  reasonCode: string,
  reason: string,
): SiblingEvidenceRejection {
  return { scope, subject, reason_code: reasonCode, reason }
}

async function verifyCandidate(
  sibling: SiblingDeclaration,
  candidate: CandidateDeclaration,
  canonicalProjectRoot: string,
  dependencies: SiblingAcquisitionDependencies,
  rejections: SiblingEvidenceRejection[],
): Promise<VerifiedCandidate | null> {
  let canonicalRoot: string
  try {
    canonicalRoot = await dependencies.realpath(candidate.path)
  } catch {
    rejections.push(rejection("candidate", candidate.label, "candidate-missing", "candidate path is unavailable"))
    return null
  }
  if (candidate.constrainedToProject && !isPathWithin(canonicalProjectRoot, canonicalRoot)) {
    rejections.push(
      rejection("candidate", candidate.label, "candidate-path-escape", "candidate resolves outside the project root"),
    )
    return null
  }
  try {
    const information = await dependencies.stat(canonicalRoot)
    if (!information.isDirectory()) {
      rejections.push(
        rejection("candidate", candidate.label, "candidate-not-directory", "candidate is not a directory"),
      )
      return null
    }
    const repositoryRoot = (await runGit(dependencies, ["rev-parse", "--show-toplevel"], canonicalRoot)).stdout.trim()
    if ((await dependencies.realpath(repositoryRoot)) !== canonicalRoot) {
      rejections.push(
        rejection("candidate", candidate.label, "candidate-not-root", "candidate is not the repository root"),
      )
      return null
    }
    const status = (
      await runGit(dependencies, ["status", "--porcelain", "--untracked-files=normal"], canonicalRoot)
    ).stdout
    if (status.trim() !== "") {
      rejections.push(rejection("candidate", candidate.label, "candidate-dirty", "repository worktree is not clean"))
      return null
    }
    const origin = (await runGit(dependencies, ["remote", "get-url", "origin"], canonicalRoot)).stdout.trim()
    if (normalizeGitHubRepositoryURL(origin) !== normalizeGitHubRepositoryURL(sibling.url)) {
      rejections.push(
        rejection(
          "candidate",
          candidate.label,
          "candidate-origin-mismatch",
          "repository origin does not match the declaration",
        ),
      )
      return null
    }
    const commit = (await runGit(dependencies, ["rev-parse", "HEAD"], canonicalRoot)).stdout.trim()
    if (!/^[0-9a-f]{40}$/.test(commit)) {
      rejections.push(
        rejection(
          "candidate",
          candidate.label,
          "candidate-invalid-commit",
          "repository HEAD is not an immutable lowercase commit",
        ),
      )
      return null
    }
    return { ...candidate, root: canonicalRoot, commit }
  } catch {
    rejections.push(
      rejection("candidate", candidate.label, "candidate-unverifiable", "repository identity could not be verified"),
    )
    return null
  }
}

async function firstVerifiedCandidate(
  sibling: SiblingDeclaration,
  candidates: readonly CandidateDeclaration[],
  canonicalProjectRoot: string,
  dependencies: SiblingAcquisitionDependencies,
  rejections: SiblingEvidenceRejection[],
): Promise<VerifiedCandidate | null> {
  for (const candidate of candidates) {
    const verified = await verifyCandidate(sibling, candidate, canonicalProjectRoot, dependencies, rejections)
    if (verified !== null) return verified
  }
  return null
}

async function safeClonePath(
  sibling: SiblingDeclaration,
  canonicalProjectRoot: string,
  dependencies: SiblingAcquisitionDependencies,
): Promise<string> {
  if (sibling.clone_path === undefined) throw new Error("clone_path is required")
  const target = resolve(canonicalProjectRoot, sibling.clone_path)
  if (!isPathWithin(canonicalProjectRoot, target)) throw new Error("clone_path escapes the project root")
  await dependencies.mkdir(dirname(target), { recursive: true })
  const canonicalParent = await dependencies.realpath(dirname(target))
  if (!isPathWithin(canonicalProjectRoot, canonicalParent)) {
    throw new Error("clone_path parent escapes the project root")
  }
  return target
}

async function refreshClone(
  sibling: SiblingDeclaration,
  canonicalProjectRoot: string,
  dependencies: SiblingAcquisitionDependencies,
  rejections: SiblingEvidenceRejection[],
): Promise<VerifiedCandidate | null> {
  let clonePath: string
  try {
    clonePath = await safeClonePath(sibling, canonicalProjectRoot, dependencies)
  } catch {
    rejections.push(
      rejection("network", "clone_path", "clone-path-invalid", "clone destination is unavailable or unsafe"),
    )
    return null
  }
  let existing = false
  try {
    await dependencies.realpath(clonePath)
    existing = true
  } catch {
    existing = false
  }
  const candidate: CandidateDeclaration = {
    label: "clone_path",
    path: clonePath,
    mode: "clone",
    constrainedToProject: true,
  }
  try {
    if (existing) {
      const verified = await verifyCandidate(sibling, candidate, canonicalProjectRoot, dependencies, rejections)
      if (verified === null) return null
      const branchRef = `refs/heads/${sibling.default_branch}`
      const remoteRef = `refs/remotes/origin/${sibling.default_branch}`
      await runGit(
        dependencies,
        ["fetch", "--depth", "1", "--no-tags", "origin", `+${branchRef}:${remoteRef}`],
        clonePath,
      )
      await runGit(dependencies, ["checkout", "--detach", remoteRef], clonePath)
    } else {
      await runGit(dependencies, [
        "clone",
        "--depth",
        "1",
        "--single-branch",
        "--no-tags",
        "--branch",
        sibling.default_branch,
        "--",
        sibling.url,
        clonePath,
      ])
    }
  } catch {
    rejections.push(
      rejection(
        "network",
        "clone_path",
        "network-acquisition-failed",
        "fetch or clone failed with credential prompts disabled",
      ),
    )
    return null
  }
  return verifyCandidate(sibling, candidate, canonicalProjectRoot, dependencies, rejections)
}

function globExpression(glob: string): RegExp {
  let expression = "^"
  for (let index = 0; index < glob.length; index += 1) {
    const character = glob[index]!
    if (character === "*") {
      if (glob[index + 1] === "*") {
        while (glob[index + 1] === "*") index += 1
        expression += glob[index + 1] === "/" ? "(?:.*/)?" : ".*"
        if (glob[index + 1] === "/") index += 1
      } else expression += "[^/]*"
    } else if (character === "?") expression += "[^/]"
    else expression += character.replace(/[\\^$.*+?()[\]{}|]/g, "\\$&")
  }
  return new RegExp(`${expression}$`)
}

function matchingTrackedFiles(output: string, contracts: readonly string[]): string[] {
  const expressions = contracts.map(globExpression)
  const paths = output
    .split("\0")
    .filter((path) => path !== "")
    .map((path) => {
      if (!validRelativeDeclarationPath(path)) throw new Error("git returned an unsafe repository path")
      return path
    })
    .filter((path) => expressions.some((expression) => expression.test(path)))
  return [...new Set(paths)].sort(compareLexical)
}

function renderEvidenceSection(metadata: SiblingEvidenceFile, content: string): string {
  return [
    UNTRUSTED_SIBLING_EVIDENCE_BEGIN,
    JSON.stringify(metadata),
    content,
    UNTRUSTED_SIBLING_EVIDENCE_END,
  ].join("\n")
}

async function readEvidenceFile(
  repositoryPath: string,
  candidate: VerifiedCandidate,
  sourceMode: SiblingSourceMode,
  sibling: SiblingDeclaration,
  dependencies: SiblingAcquisitionDependencies,
  budget: EvidenceBudget,
  trackedPaths: ReadonlySet<string>,
  rejections: SiblingEvidenceRejection[],
): Promise<{ readonly metadata: SiblingEvidenceFile; readonly section: string } | null> {
  if (budget.files >= MAX_SIBLING_FILES) {
    rejections.push(
      rejection("file", repositoryPath, "file-count-limit", "the invocation file-count limit was reached"),
    )
    return null
  }
  let canonicalPath: string
  let information: AcquisitionFileInfo
  try {
    canonicalPath = await dependencies.realpath(resolve(candidate.root, repositoryPath))
    if (!isPathWithin(candidate.root, canonicalPath) || canonicalPath === candidate.root) {
      rejections.push(
        rejection(
          "file",
          repositoryPath,
          "file-symlink-escape",
          "matched path resolves outside the verified repository root",
        ),
      )
      return null
    }
    const canonicalRelativePath = relative(candidate.root, canonicalPath).replaceAll("\\", "/")
    if (!trackedPaths.has(canonicalRelativePath)) {
      rejections.push(
        rejection("file", repositoryPath, "file-target-untracked", "resolved file target is not tracked by the commit"),
      )
      return null
    }
    information = await dependencies.stat(canonicalPath)
  } catch {
    rejections.push(rejection("file", repositoryPath, "file-unverifiable", "matched path could not be canonicalized"))
    return null
  }
  if (!information.isFile()) {
    rejections.push(rejection("file", repositoryPath, "file-not-regular", "matched path is not a regular file"))
    return null
  }
  if (information.size > MAX_SIBLING_FILE_BYTES) {
    rejections.push(rejection("file", repositoryPath, "file-size-limit", "file exceeds the 256 KiB limit"))
    return null
  }
  if (budget.bytes + information.size > MAX_SIBLING_TOTAL_BYTES) {
    rejections.push(
      rejection("file", repositoryPath, "total-size-limit", "file would exceed the 1 MiB invocation limit"),
    )
    return null
  }
  let bytes: Uint8Array
  let content: string
  try {
    bytes = await dependencies.readFile(canonicalPath)
    if (bytes.byteLength !== information.size || bytes.byteLength > MAX_SIBLING_FILE_BYTES) {
      throw new Error("unstable file")
    }
    content = new TextDecoder("utf-8", { fatal: true }).decode(bytes)
  } catch {
    rejections.push(
      rejection("file", repositoryPath, "file-read-failed", "file bytes are unavailable, unstable, or not UTF-8"),
    )
    return null
  }
  if (content.includes(UNTRUSTED_SIBLING_EVIDENCE_BEGIN) || content.includes(UNTRUSTED_SIBLING_EVIDENCE_END)) {
    rejections.push(
      rejection("file", repositoryPath, "reserved-delimiter", "file contains a reserved evidence delimiter"),
    )
    return null
  }
  const metadata: SiblingEvidenceFile = {
    sibling: sibling.name,
    commit: candidate.commit,
    path: repositoryPath,
    sha256: createHash("sha256").update(bytes).digest("hex"),
    source_mode: sourceMode,
  }
  budget.files += 1
  budget.bytes += bytes.byteLength
  return { metadata, section: renderEvidenceSection(metadata, content) }
}

async function collectEvidence(
  sibling: SiblingDeclaration,
  candidate: VerifiedCandidate,
  sourceMode: SiblingSourceMode,
  dependencies: SiblingAcquisitionDependencies,
  budget: EvidenceBudget,
  rejections: SiblingEvidenceRejection[],
): Promise<CollectedEvidence> {
  const initialBudget = { ...budget }
  let tracked: string[]
  try {
    tracked = matchingTrackedFiles(
      (await runGit(dependencies, ["ls-files", "-z"], candidate.root)).stdout,
      sibling.contracts,
    )
  } catch {
    rejections.push(
      rejection("file", sibling.name, "tracked-files-unverifiable", "tracked repository paths could not be verified"),
    )
    return { files: [], sections: [] }
  }
  const files: SiblingEvidenceFile[] = []
  const sections: string[] = []
  const trackedPaths = new Set(tracked)
  for (const repositoryPath of tracked) {
    const evidence = await readEvidenceFile(
      repositoryPath,
      candidate,
      sourceMode,
      sibling,
      dependencies,
      budget,
      trackedPaths,
      rejections,
    )
    if (evidence !== null) {
      files.push(evidence.metadata)
      sections.push(evidence.section)
    }
  }
  try {
    const status = (
      await runGit(dependencies, ["status", "--porcelain", "--untracked-files=normal"], candidate.root)
    ).stdout
    const commit = (await runGit(dependencies, ["rev-parse", "HEAD"], candidate.root)).stdout.trim()
    if (status.trim() !== "" || commit !== candidate.commit) {
      budget.files = initialBudget.files
      budget.bytes = initialBudget.bytes
      rejections.push(
        rejection("candidate", candidate.label, "candidate-changed", "repository changed during evidence acquisition"),
      )
      return { files: [], sections: [] }
    }
  } catch {
    budget.files = initialBudget.files
    budget.bytes = initialBudget.bytes
    rejections.push(
      rejection(
        "candidate",
        candidate.label,
        "candidate-postcheck-failed",
        "repository could not be reverified after evidence reads",
      ),
    )
    return { files: [], sections: [] }
  }
  return { files, sections }
}

async function acquireOneSibling(
  sibling: SiblingDeclaration,
  canonicalProjectRoot: string,
  dependencies: SiblingAcquisitionDependencies,
  budget: EvidenceBudget,
): Promise<{ readonly result: SiblingEvidenceRepositoryResult; readonly sections: readonly string[] }> {
  const rejections: SiblingEvidenceRejection[] = []
  const candidates = candidateDeclarations(sibling, canonicalProjectRoot)
  let selected: VerifiedCandidate | null
  let sourceMode: SiblingSourceMode | null
  if (sibling.fetch === "clone-on-review") {
    selected = await refreshClone(sibling, canonicalProjectRoot, dependencies, rejections)
    if (selected === null) {
      selected = await firstVerifiedCandidate(sibling, candidates, canonicalProjectRoot, dependencies, rejections)
      sourceMode = selected === null ? null : "offline-cache"
    } else sourceMode = "clone"
  } else {
    selected = await firstVerifiedCandidate(sibling, candidates, canonicalProjectRoot, dependencies, rejections)
    sourceMode = selected?.mode ?? null
  }
  if (selected === null || sourceMode === null) {
    return {
      result: {
        sibling: sibling.name,
        status: "unavailable",
        commit: null,
        source_mode: null,
        source_candidate: null,
        evidence: [],
        rejections,
        unavailable_reason:
          sibling.fetch === "contract-only" ? "no-verified-candidate" : "network-failed-no-verified-candidate",
      },
      sections: [],
    }
  }
  const collected = await collectEvidence(sibling, selected, sourceMode, dependencies, budget, rejections)
  return {
    result: {
      sibling: sibling.name,
      status: "available",
      commit: selected.commit,
      source_mode: sourceMode,
      source_candidate: selected.label,
      evidence: collected.files,
      rejections,
      unavailable_reason: null,
    },
    sections: collected.sections,
  }
}

/**
 * Acquires deterministic, bounded sibling evidence without invoking an AI provider.
 * @param rawInput Closed empty tool input.
 * @param dependencies Injectable config, filesystem, and command operations.
 * @returns Structured provenance and exact untrusted-evidence prompt sections.
 */
export async function acquireSiblingEvidence(
  rawInput: AcquireSiblingEvidenceInput,
  dependencies: SiblingAcquisitionDependencies,
): Promise<AcquireSiblingEvidenceResult> {
  AcquireSiblingEvidenceInputSchema.parse(rawInput)
  let declaration: SiblingRepositories
  try {
    declaration = parseSiblingRepositories(
      await dependencies.readText(SIBLING_REPOSITORIES_PATH),
      dependencies.parseYaml,
    )
  } catch (error: unknown) {
    return {
      version: 1,
      status: "invalid-config",
      prompt: "",
      siblings: [],
      errors: [error instanceof Error ? error.message : "invalid sibling repository configuration"],
    }
  }
  let canonicalProjectRoot: string
  try {
    canonicalProjectRoot = await dependencies.realpath(dependencies.projectRoot)
  } catch {
    return {
      version: 1,
      status: "invalid-config",
      prompt: "",
      siblings: [],
      errors: ["project root could not be canonicalized"],
    }
  }
  const budget: EvidenceBudget = { files: 0, bytes: 0 }
  const siblings: SiblingEvidenceRepositoryResult[] = []
  const sections: string[] = []
  for (const sibling of declaration.siblings) {
    const acquired = await acquireOneSibling(sibling, canonicalProjectRoot, dependencies, budget)
    siblings.push(acquired.result)
    sections.push(...acquired.sections)
  }
  return { version: 1, status: "ok", prompt: sections.join("\n\n"), siblings, errors: [] }
}

/**
 * Creates production sibling acquisition dependencies rooted at one OpenCode worktree.
 * @param projectRoot Absolute project worktree path.
 * @param parseYaml Supported production YAML parser.
 * @returns Node-backed filesystem and no-shell command operations.
 */
export function createSiblingAcquisitionDependencies(
  projectRoot: string,
  parseYaml: SiblingYamlParser,
): SiblingAcquisitionDependencies {
  return {
    projectRoot,
    readText: async (relativePath: string): Promise<string> => readFile(resolve(projectRoot, relativePath), "utf8"),
    parseYaml,
    realpath,
    stat,
    readFile,
    mkdir,
    runCommand: async (command, args, options = {}): Promise<CommandResult> =>
      new Promise<CommandResult>((resolvePromise, rejectPromise) => {
        execFile(
          command,
          [...args],
          {
            cwd: options.cwd,
            env: { ...process.env, ...options.env },
            encoding: "utf8",
            maxBuffer: 2 * 1024 * 1024,
            timeout: options.timeout,
            signal: options.signal,
          },
          (error, stdout, stderr) => {
            if (error !== null) rejectPromise(error)
            else resolvePromise({ stdout, stderr })
          },
        )
      }),
  }
}

/**
 * Creates the acquire_sibling_evidence OpenCode tool with injectable I/O.
 * @param dependencies Config, filesystem, and command dependencies.
 * @returns A provider-free OpenCode tool definition with closed empty input.
 */
export function createAcquireSiblingEvidenceTool(
  dependencies: SiblingAcquisitionDependencies,
): ReturnType<typeof tool> {
  return tool({
    description:
      "Acquire clean, origin-verified, commit-pinned sibling contract files within strict path and size bounds; " +
      "return content only inside untrusted-evidence delimiters.",
    args: {},
    async execute(args): Promise<string> {
      return `${JSON.stringify(await acquireSiblingEvidence(args, dependencies), null, 2)}\n`
    },
  })
}
