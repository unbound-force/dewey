/**
 * Hierarchical delegation budgets and child-session activity probing
 * shared by every executeAgentSession caller (OpenSpec change
 * delegation-timeout-budgets, design D1/D2, issue #930). Compaction
 * accounting is scoped per session (OpenSpec change
 * compaction-storm-detector-sums-compactio design D1/D2; DT-FR-002 /
 * DELEG-FR-002, issues #1079 and #1125). Running tool-call parts
 * fold into the progress fingerprint and ride the sample per
 * idle-timeout-aborts-children-waiting-on (DT-FR-013, issue #1117):
 * a quiet-but-healthy long tool call counts as activity within the
 * DT-FR-014 deferral bound, and the collected parts select the abort
 * envelope's running_tool (DT-FR-015).
 *
 * A child session created by the executor records its absolute
 * deadline here; a nested executor call made from inside that child
 * (or from a session the host created beneath it) resolves the
 * deadline and clamps its own wall clock to the remaining budget, so
 * nested work never outlives its parent. The activity probe
 * fingerprints a child's session subtree so the executor can tell a
 * working child from a stalled one. Compaction markers (context
 * compression summaries and synthetic continuation prompts) are
 * structurally classified and excluded from progress, and the probe
 * exposes the per-session compaction accounting -- the highest
 * per-session run of consecutive markers and the session holding it
 * -- shared by the idle watchdog and the compaction-storm detector.
 * The run accrues only for counted sessions: executor-managed
 * descendants -- sessions another executor registered a run deadline
 * for -- are excluded because their own detector is always armed,
 * while the probed root stays counted.
 *
 * Per-task delegation children inherit idle semantics only through
 * this registry (OpenSpec change per-task-delegation-budgets,
 * design D2, PTB-FR-002, issue #1080): a registry entry carries the
 * idle window alongside the deadline it has always held, a nested
 * per-task call resolves the nearest registered ancestor's entry in
 * a single ancestry walk, and the applicability rule combines an
 * explicit window with the inherited one without introducing any
 * new global idle default -- top-level callers without a budgeted
 * ancestor keep their unchanged behavior.
 */

import { type PluginInput } from "@opencode-ai/plugin"

// ── Constants ────────────────────────────────────────────────

/** Time reserved for the parent to receive and act on a nested failure. */
export const BUDGET_RESERVE_MS = 60_000

/** Smallest clamped budget worth spawning a nested child for. */
export const MIN_NESTED_BUDGET_MS = 30_000

/** Maximum parentID hops walked when resolving an inherited deadline. */
export const MAX_ANCESTRY_DEPTH = 8

/** Maximum sessions the activity probe fingerprints per poll. */
export const MAX_PROBE_SESSIONS = 32

/** Window within which observed progress marks a child as active. */
export const ACTIVE_WINDOW_MS = 120_000

/** Default interval between activity probes. */
export const DEFAULT_IDLE_POLL_MS = 30_000

/**
 * Fixed deferral cap for the idle watcher while a tool part is running
 * (DT-FR-014): with no observable tool deadline, the idle abort fires
 * no later than idle_timeout plus this cap, measured from the last
 * fingerprint change. Pinned at 15 minutes, inside the spec's
 * 5-15 minute (300,000-900,000 ms) range.
 */
export const IDLE_DEFERRAL_CAP_MS = 900_000

/**
 * Grace period added to an observable tool deadline before the idle
 * watcher may abort (DT-FR-014): the tool's own timeout is
 * model-handled -- the model receives the tool error and reacts -- so
 * idle evaluation waits this long past the deadline. Fixed at 30
 * seconds, under the spec's 60,000 ms cap. Applies only to the
 * deadline path; the cap path is already measured from the last
 * fingerprint change and needs no grace.
 */
export const IDLE_TOOL_GRACE_MS = 30_000

/**
 * Maximum length of a text that may still match the system-reminder
 * compaction-summary wrapper. Real compaction summaries measure in the
 * tens of thousands of characters, so the bound only exists to keep a
 * pathological legitimate text that merely starts and ends with the
 * wrapper tags from matching (design D3: matchers are anchored and
 * length-bounded, never bare substrings).
 */
export const MAX_SYNTHETIC_WRAPPER_LENGTH = 65_536

/**
 * Maximum length of a text that may still match the synthetic
 * continuation prompt. The prompt is one sentence, optionally prefixed
 * by opencode's documented large-media overflow preamble (about 460
 * characters total); anything longer cannot be the injected prompt.
 */
export const MAX_CONTINUATION_PROMPT_LENGTH = 4_096

/**
 * Validation pattern for a running part's tool name (DT-FR-015,
 * Constitution V): the name originates from model-controlled message
 * parts, so it only reaches envelopes and logs when it matches. A
 * failing value reports as null and is never logged verbatim.
 */
export const RUNNING_TOOL_NAME_PATTERN = /^[A-Za-z0-9._:-]{1,128}$/

/**
 * Tool names whose completion is read-only inspection and therefore
 * never counts as substantive progress (DT-FR-002: file reads,
 * searches, and other inspection MUST NOT count). Every other tool --
 * including project MCP tools -- defaults to non-read: the spec's
 * exclusion list is explicit, and an unknown tool that completes has
 * usually performed durable work worth resetting the storm window
 * for. Read tools are the exception, not the rule.
 */
export const READ_ONLY_TOOLS: ReadonlySet<string> = new Set(["read", "grep", "glob", "list", "ls", "webfetch"])

const REGISTRY_KEY = Symbol.for("unbound-force.delegation-budget")
const PROBE_REGISTRY_KEY = Symbol.for("unbound-force.delegation-budget.probe")

type Client = PluginInput["client"]

// ── Deadline registry ────────────────────────────────────────

/**
 * The budget registered for one child session: the absolute
 * wall-clock deadline the executor enforces plus the idle window its
 * run was armed with, when any (per-task-delegation-budgets design
 * D2). The idle window rides the same entry as the deadline so a
 * nested per-task call inherits both in one ancestry walk; null
 * means the registering run had no idle watchdog, which inherits as
 * "off" rather than as a default.
 */
export interface RegisteredBudget {
  /** Absolute deadline in epoch milliseconds. */
  readonly deadlineMs: number
  /** Idle window in milliseconds; null when the run has no idle watchdog. */
  readonly idleMs: number | null
}

/**
 * Process-wide deadline registry (design D1). Stored on globalThis
 * under a registered symbol so the separate plugin modules that import
 * this library share one registry even if the host loads the library
 * more than once. Holds only session ids, epoch milliseconds, and
 * nullable idle windows.
 */
function registry(): Map<string, RegisteredBudget> {
  const holder = globalThis as unknown as Record<symbol, Map<string, RegisteredBudget> | undefined>
  let map = holder[REGISTRY_KEY]
  if (map === undefined) {
    map = new Map<string, RegisteredBudget>()
    holder[REGISTRY_KEY] = map
  }
  return map
}

/**
 * Record a child session's absolute deadline (epoch ms). The optional
 * idle window (per-task-delegation-budgets design D2) rides the same
 * registry entry so nested per-task children can inherit the
 * watchdog; omit it for a run with no idle watchdog.
 *
 * @param sessionID - The child session id.
 * @param deadlineMs - Absolute deadline in epoch milliseconds.
 * @param idleMs - The run's idle window in milliseconds, when it carries one.
 */
export function registerDeadline(sessionID: string, deadlineMs: number, idleMs?: number): void {
  registry().set(sessionID, { deadlineMs, idleMs: idleMs ?? null })
}

/** Remove a child session's registered budget once its run has ended. */
export function unregisterDeadline(sessionID: string): void {
  registry().delete(sessionID)
}

/** The registered deadline for a session, or undefined. */
export function registeredDeadline(sessionID: string): number | undefined {
  return registry().get(sessionID)?.deadlineMs
}

/**
 * The full budget registered for a session -- deadline plus idle
 * window -- without walking any ancestry.
 *
 * @param sessionID - The session id to look up.
 * @returns The registered entry, or undefined when the session has none.
 */
export function registeredBudget(sessionID: string): RegisteredBudget | undefined {
  return registry().get(sessionID)
}

/**
 * Resolve the budget a nested call inherits: the registered entry of
 * `sessionID`, else of the nearest registered ancestor found by
 * walking `parentID` links up to MAX_ANCESTRY_DEPTH hops. The entry
 * carries the ancestor's deadline and, for per-task children of a
 * budgeted command-path parent, the idle window that ancestor's run
 * was armed with (per-task-delegation-budgets design D2, PTB-FR-002).
 * Any lookup failure (or a client without `session.get`) yields
 * null, which leaves top-level behavior unchanged.
 *
 * @param client - Plugin client used to walk parentID links.
 * @param sessionID - The session the nested call runs from.
 * @param directory - Project directory scoping the session lookups.
 * @param signal - Abort signal relayed to each lookup request.
 * @returns The inherited budget entry, or null when no ancestor is registered.
 */
export async function resolveInheritedBudget(
  client: Client,
  sessionID: string,
  directory: string,
  signal: AbortSignal,
): Promise<RegisteredBudget | null> {
  let current: string | undefined = sessionID
  for (let depth = 0; current !== undefined && depth <= MAX_ANCESTRY_DEPTH; depth++) {
    const budget = registeredBudget(current)
    if (budget !== undefined) {
      return budget
    }
    if (depth === MAX_ANCESTRY_DEPTH || typeof client.session?.get !== "function") {
      return null
    }
    try {
      const response = await client.session.get({ path: { id: current }, query: { directory }, signal })
      current = response.data?.parentID
    } catch {
      return null
    }
  }
  return null
}

/**
 * Resolve only the deadline a nested call inherits, delegating the
 * shared ancestry walk to resolveInheritedBudget. Whether the idle
 * watchdog applies on top of the resolved budget is
 * resolveIdleApplicability's concern, not this function's.
 *
 * @param client - Plugin client used to walk parentID links.
 * @param sessionID - The session the nested call runs from.
 * @param directory - Project directory scoping the session lookups.
 * @param signal - Abort signal relayed to each lookup request.
 * @returns The inherited deadline, or null when no ancestor is registered.
 */
export async function resolveInheritedDeadline(
  client: Client,
  sessionID: string,
  directory: string,
  signal: AbortSignal,
): Promise<number | null> {
  const budget = await resolveInheritedBudget(client, sessionID, directory, signal)
  return budget === null ? null : budget.deadlineMs
}

/** Outcome of clamping a requested wall clock to an inherited budget. */
export interface BudgetClamp {
  readonly effectiveMs: number
  readonly clamped: boolean
  readonly exhausted: boolean
}

/**
 * Clamp a requested wall clock to the inherited deadline minus
 * BUDGET_RESERVE_MS. Exhausted when less than MIN_NESTED_BUDGET_MS
 * would remain; a null deadline passes the request through.
 */
export function clampToBudget(requestedMs: number, inheritedDeadline: number | null, now: number): BudgetClamp {
  if (inheritedDeadline === null) {
    return { effectiveMs: requestedMs, clamped: false, exhausted: false }
  }
  const available = inheritedDeadline - now - BUDGET_RESERVE_MS
  if (available < MIN_NESTED_BUDGET_MS) {
    return { effectiveMs: Math.max(0, available), clamped: true, exhausted: true }
  }
  return available < requestedMs
    ? { effectiveMs: available, clamped: true, exhausted: false }
    : { effectiveMs: requestedMs, clamped: false, exhausted: false }
}

// ── Idle applicability (per-task inheritance) ─────────────────

/** Origin of the effective idle window for a delegated run. */
export type IdleSource = "explicit" | "inherited" | "none"

/** How the idle watchdog applies to one delegated run. */
export interface IdleApplicability {
  /** The effective idle window in milliseconds; undefined when the watchdog is off. */
  readonly idleMs: number | undefined
  /** Where the effective window came from. */
  readonly source: IdleSource
}

/**
 * Decide how the idle watchdog applies to a delegated run
 * (per-task-delegation-budgets design D2, PTB-FR-002). Precedence: a
 * window the caller explicitly passed wins; otherwise a budgeted
 * ancestor's registered idle window is inherited through the same
 * registry entry as its deadline; otherwise the watchdog stays off.
 * Introducing no global default is the point -- a top-level caller
 * without a budgeted ancestor behaves exactly as before, which is
 * the alternative design D2 rejects (a per-task idle default outside
 * the registry would be a second source of truth and would change
 * top-level behavior). Whether an explicitly passed value is sane is
 * the tool schema's concern; like the executor's armed check, any
 * defined value counts as explicit.
 *
 * @param explicitIdleMs - The idle window the caller passed, when it passed one.
 * @param inherited - The budget resolved from the registry, null when unbudgeted.
 * @returns The effective idle window and where it came from.
 */
export function resolveIdleApplicability(
  explicitIdleMs: number | undefined,
  inherited: RegisteredBudget | null,
): IdleApplicability {
  if (explicitIdleMs !== undefined) {
    return { idleMs: explicitIdleMs, source: "explicit" }
  }
  if (inherited !== null && inherited.idleMs !== null) {
    return { idleMs: inherited.idleMs, source: "inherited" }
  }
  return { idleMs: undefined, source: "none" }
}

// ── Activity probe ───────────────────────────────────────────

/**
 * Compaction-accounting counters shared by the idle watchdog and the
 * compaction-storm detector (compaction-storm-detector-sums-compactio
 * design D1/D2; DT-FR-002 requires one accounting for both
 * consumers). Accounting accrues per session: markers observed in
 * different subtree sessions are never summed, and only substantive
 * progress in the same session resets a session's run. The storm
 * fields describe counted sessions only -- executor-managed
 * descendants never accrue (design D2) -- while the observability
 * totals stay subtree-wide (issue #1125).
 */
export interface ProbeCounters {
  /**
   * Highest run of consecutive compaction markers accrued by a single
   * counted session (the probe root or an unrecognized descendant)
   * since that session's last substantive progress.
   */
  readonly maxSessionCompactionsSinceProgress: number
  /**
   * The session holding the highest run; null exactly when the
   * maximum is zero. Ties resolve to the first session in the fold's
   * observation order.
   */
  readonly stormSessionID: string | null
  /** Substantive non-read progress observations across all probes. */
  readonly nonReadProgressCount: number
  /** Total compaction markers observed across all probes. */
  readonly compactionCount: number
}

/** One tool-call part observed with `state.status === "running"` (DT-FR-013). */
export interface RunningToolPart {
  /** Part identity; partId-keyed changes in the running set are progress. */
  readonly partId: string
  /** Always "running"; carried so the hashed triple matches the spec. */
  readonly status: "running"
  /** The part's tool name, empty string when the part carries none. */
  readonly tool: string
  /**
   * Observable tool deadline (epoch ms) read from the running state's
   * `metadata.deadline_ms`, or null when none. The session store
   * exposes no tool deadline today (design D2), so this is a
   * contingency key: when a host begins populating it, DT-FR-014's
   * deadline path activates without further probe changes.
   */
  readonly deadlineMs: number | null
}

/** One activity sample of a child's session subtree. */
export interface ActivitySample extends ProbeCounters {
  /** Opaque fingerprint; any change between samples is progress. */
  readonly fingerprint: string
  /** The latest part of some subtree session is a running question tool call. */
  readonly pendingQuestion: boolean
  /**
   * Running tool-call parts observed across the subtree (DT-FR-013),
   * sorted by (partId, tool). A non-empty set defers the idle abort
   * within the DT-FR-014 bound and selects the abort envelope's
   * running_tool (DT-FR-015).
   */
  readonly runningTools: readonly RunningToolPart[]
}

/** Structural kind of a host-injected synthetic text shape (design D3). */
export type SyntheticTextKind = "compaction-summary" | "continuation-prompt"

interface PartLike {
  readonly id?: unknown
  readonly type?: unknown
  readonly tool?: unknown
  readonly text?: unknown
  readonly metadata?: unknown
  readonly state?: {
    readonly status?: unknown
    readonly output?: unknown
    readonly title?: unknown
    readonly metadata?: unknown
  }
}

interface MessageLike {
  readonly info?: {
    readonly id?: unknown
    readonly role?: unknown
    readonly mode?: unknown
    readonly agent?: unknown
    readonly time?: { readonly completed?: unknown }
    readonly error?: unknown
  }
  readonly parts?: readonly PartLike[]
}

/** Serialized length of a running tool's metadata; 0 when absent or unserializable. */
function metadataLength(metadata: unknown): number {
  if (metadata === undefined || metadata === null) {
    return 0
  }
  try {
    return JSON.stringify(metadata)?.length ?? 0
  } catch {
    return 0
  }
}

const SYSTEM_REMINDER_OPEN = "<system-reminder>"
const SYSTEM_REMINDER_CLOSE = "</system-reminder>"
// Both injected variants of opencode's continuation prompt share this
// closing sentence: the bare prompt and the large-media overflow
// preamble prefixed to it. Anchoring on the shared suffix (plus the
// length bound) matches both without hardcoding the volatile preamble.
const CONTINUATION_PROMPT_SUFFIX =
  "continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed."

/**
 * Classify a text against the known host-injected synthetic shapes: a
 * system-reminder-wrapped compaction summary or the synthetic
 * continuation prompt. Matchers are structural per design D3 --
 * anchored at the wrapper edges or the prompt's closing sentence,
 * case-insensitive, and length-bounded -- so legitimate prose that
 * merely mentions compaction or reminders never matches.
 *
 * @param text - The text to classify.
 * @returns The matched shape, or null when the text is not a known synthetic wrapper.
 */
export function classifySyntheticText(text: string): SyntheticTextKind | null {
  const trimmed = text.trim()
  if (trimmed.length === 0) {
    return null
  }
  const lowered = trimmed.toLowerCase()
  if (
    trimmed.length <= MAX_SYNTHETIC_WRAPPER_LENGTH &&
    lowered.startsWith(SYSTEM_REMINDER_OPEN) &&
    lowered.endsWith(SYSTEM_REMINDER_CLOSE)
  ) {
    return "compaction-summary"
  }
  if (trimmed.length <= MAX_CONTINUATION_PROMPT_LENGTH && lowered.endsWith(CONTINUATION_PROMPT_SUFFIX)) {
    return "continuation-prompt"
  }
  return null
}

/**
 * Whether a part-level metadata object carries a boolean flag set to true.
 * Narrowing helper for the untyped part metadata the probe observes.
 */
function partMetadataFlag(metadata: unknown, flag: string): boolean {
  if (typeof metadata !== "object" || metadata === null) {
    return false
  }
  return (metadata as Record<string, unknown>)[flag] === true
}

/**
 * Whether a message structurally matches a compaction marker: a
 * host-injected context-compression artifact. Matches the opencode
 * storage shapes (a `compaction` part, a message generated by the
 * `compaction` agent, a part flagged `compaction_continue`) and the
 * text shapes (a user-role system-reminder wrapper or the synthetic
 * continuation prompt on the last part, mirroring the fingerprint's
 * view), so the classification survives host-side representation
 * drift. Never matches bare words like "compaction" appearing in
 * legitimate prose (design D3).
 *
 * @param message - The latest message of a subtree session, if any.
 * @returns True when the message is a compaction/synthetic marker.
 */
export function isCompactionMarker(message: MessageLike | undefined): boolean {
  if (message === undefined) {
    return false
  }
  if (message.info?.mode === "compaction" || message.info?.agent === "compaction") {
    return true
  }
  const parts = message.parts ?? []
  for (const part of parts) {
    if (part?.type === "compaction") {
      return true
    }
    if (partMetadataFlag(part?.metadata, "compaction_continue")) {
      return true
    }
  }
  if (message.info?.role === "user") {
    const last = parts[parts.length - 1]
    if (typeof last?.text === "string" && classifySyntheticText(last.text) !== null) {
      return true
    }
  }
  return false
}

/**
 * Whether a message's latest part is the successful completion of a
 * non-read tool call -- the observable form of substantive progress
 * (file writes, edits, shell commands, and checkpoint records are all
 * completed tool calls). Read-only tools never count whatever their
 * outcome, and a non-read tool call that failed or has not completed
 * does not count (DT-FR-002 / DELEG-FR-002: only successful
 * completions reset a session's storm window).
 *
 * @param message - The latest message of a subtree session, if any.
 * @returns True when the message records completed non-read work.
 */
export function isSubstantiveProgress(message: MessageLike | undefined): boolean {
  if (message === undefined) {
    return false
  }
  const parts = message.parts ?? []
  const last = parts[parts.length - 1]
  if (last?.type !== "tool" || typeof last.tool !== "string" || READ_ONLY_TOOLS.has(last.tool)) {
    return false
  }
  return last.state?.status === "completed"
}

/** Deterministic order for running parts: by partId, then tool name. */
function compareRunningTools(a: RunningToolPart, b: RunningToolPart): number {
  if (a.partId !== b.partId) {
    return a.partId < b.partId ? -1 : 1
  }
  return a.tool === b.tool ? 0 : a.tool < b.tool ? -1 : 1
}

/**
 * Read an observable tool deadline (epoch ms) from a running state's
 * metadata. The session store exposes no tool deadline today (design
 * D2), so `metadata.deadline_ms` is a contingency key: only a finite
 * number is honored; anything else reads as no deadline observable.
 */
function observableToolDeadline(metadata: unknown): number | null {
  if (typeof metadata !== "object" || metadata === null) {
    return null
  }
  const deadline = (metadata as Record<string, unknown>).deadline_ms
  return typeof deadline === "number" && Number.isFinite(deadline) ? deadline : null
}

/**
 * Collect a message's running tool-call parts (DT-FR-013): every part
 * whose `state.status` is `"running"`, sorted by (partId, tool) so the
 * hashed triple set and the envelope's deterministic selection stay
 * stable. The latest message of each walked session is the observable
 * surface -- a running part lives on the message being streamed, and a
 * new message appears only after the current one settles.
 *
 * @param message - The latest message of a subtree session, if any.
 * @returns The sorted running parts; empty when none or no message.
 */
export function collectRunningTools(message: MessageLike | undefined): RunningToolPart[] {
  const running: RunningToolPart[] = []
  for (const part of message?.parts ?? []) {
    if (part?.state?.status !== "running") {
      continue
    }
    running.push({
      partId: String(part.id ?? ""),
      status: "running",
      tool: typeof part.tool === "string" ? part.tool : "",
      deadlineMs: observableToolDeadline(part.state.metadata),
    })
  }
  return running.sort(compareRunningTools)
}

/**
 * Select the abort envelope's running_tool (DT-FR-015): the tool name
 * of the running part with the lexicographically smallest partId,
 * null when no part is running. The name is model-controlled input,
 * so the selected value is validated against
 * RUNNING_TOOL_NAME_PATTERN before it is returned; a failure reports
 * null -- no fall-through to another part, keeping the selection rule
 * deterministic -- and the raw value never reaches the caller to be
 * logged.
 *
 * @param runningTools - The running parts observed at abort time.
 * @returns The validated tool name, or null when none or invalid.
 */
export function selectRunningTool(runningTools: readonly RunningToolPart[]): string | null {
  let selected: RunningToolPart | undefined
  for (const part of runningTools) {
    if (selected === undefined || compareRunningTools(part, selected) < 0) {
      selected = part
    }
  }
  if (selected === undefined) {
    return null
  }
  return RUNNING_TOOL_NAME_PATTERN.test(selected.tool) ? selected.tool : null
}

/**
 * Whether the DT-FR-014 idle-deferral bound has expired at `at`. With
 * no running parts the deferral is vacuous -- a true stall between
 * tool calls stays subject to the plain idle window. While a tool
 * runs, the bound is the earliest observable tool deadline plus
 * IDLE_TOOL_GRACE_MS, or -- when no running part exposes a deadline --
 * idle_timeout plus IDLE_DEFERRAL_CAP_MS measured from the last
 * fingerprint change, so a wedged tool can never hold idle forever.
 * The wall clock remains the absolute backstop either way.
 *
 * @param runningTools - The running parts observed in the latest sample.
 * @param idleMilliseconds - The configured idle window; undefined when the idle watchdog is disabled.
 * @param lastFingerprintChangeAt - When the fingerprint last changed (epoch ms).
 * @param at - The evaluation time (epoch ms).
 * @returns True when the idle watcher may abort past the idle window.
 */
export function idleDeferralExpired(
  runningTools: readonly RunningToolPart[],
  idleMilliseconds: number | undefined,
  lastFingerprintChangeAt: number,
  at: number,
): boolean {
  if (runningTools.length === 0) {
    return true
  }
  if (idleMilliseconds === undefined) {
    return false
  }
  let earliestDeadline: number | null = null
  for (const part of runningTools) {
    if (part.deadlineMs !== null && (earliestDeadline === null || part.deadlineMs < earliestDeadline)) {
      earliestDeadline = part.deadlineMs
    }
  }
  return earliestDeadline === null
    ? at - lastFingerprintChangeAt >= idleMilliseconds + IDLE_DEFERRAL_CAP_MS
    : at >= earliestDeadline + IDLE_TOOL_GRACE_MS
}

/**
 * Stable identity of a marker message: message id, part count, and
 * last part id. Deliberately excludes streamed text length so a
 * compaction summary that grows while it generates stays one counted
 * marker, not one per poll; message ids are append-only in practice,
 * so per-session last-key tracking cannot double count an event.
 */
function markerKey(message: MessageLike): string {
  const parts = message.parts ?? []
  const last = parts[parts.length - 1]
  return `${String(message.info?.id ?? "")}:${parts.length}:${String(last?.id ?? "")}`
}

/**
 * Fingerprint one session's latest message: message id, completion,
 * error presence, part count, and the last part's id, type, tool
 * status, title, and text/output/metadata length. Streaming grows the
 * text length, and a running tool streams through its state metadata
 * (bash keeps live output in `state.metadata.output`; the running
 * state has no `output` field), so a long command that keeps printing
 * counts as progress. The sorted set of running `(partId, status,
 * tool)` triples is appended with a `~` separator (DT-FR-013), so a
 * completion, a new running part, or a tool swap on the same partId
 * reads as progress while messages without running parts keep their
 * historical fingerprint. Compaction markers collapse to one constant
 * per session so marker-to-marker churn leaves the fingerprint
 * unchanged (DT-FR-002); probeSubtreeActivity additionally substitutes
 * the session's remembered pre-marker print so a marker's arrival
 * after a real message also reads as no change.
 */
export function messageFingerprint(sessionID: string, message: MessageLike | undefined): string {
  if (message === undefined) {
    return `${sessionID}:-`
  }
  if (isCompactionMarker(message)) {
    return `${sessionID}:synthetic`
  }
  const parts = message.parts ?? []
  const last = parts[parts.length - 1]
  const textLength = typeof last?.text === "string" ? last.text.length : 0
  const outputLength = typeof last?.state?.output === "string" ? last.state.output.length : 0
  const streamedLength = textLength + outputLength + metadataLength(last?.state?.metadata)
  const base = [
    sessionID,
    String(message.info?.id ?? ""),
    String(message.info?.time?.completed ?? ""),
    message.info?.error === undefined ? "" : "e",
    String(parts.length),
    String(last?.id ?? ""),
    String(last?.type ?? ""),
    String(last?.state?.status ?? ""),
    typeof last?.state?.title === "string" ? last.state.title : "",
    String(streamedLength),
  ].join(":")
  // The `~` prefix cannot collide with the base fields (colon-joined)
  // or the subtree-level `|` join of per-session prints, and sorted,
  // comma-joined triples keep the hash deterministic (DT-FR-013).
  const running = collectRunningTools(message)
  if (running.length === 0) {
    return base
  }
  return `${base}~${running.map((part) => `${part.partId}:${part.status}:${part.tool}`).join(",")}`
}

function isPendingQuestion(message: MessageLike | undefined): boolean {
  const parts = message?.parts ?? []
  const last = parts[parts.length - 1]
  return last?.type === "tool" && last.tool === "question" && last.state?.status === "running"
}

/** One session's latest message as observed during a probe walk. */
interface ProbeObservation {
  readonly id: string
  readonly latest: MessageLike | undefined
}

/**
 * Per-session probe memory: the last real print, the last observed
 * marker and progress identities, and the session's count of
 * consecutive compactions since its last substantive progress. The
 * count accrues only for counted sessions; a managed descendant's
 * identities still update -- so nothing is counted retroactively
 * once its executor run ends -- but its count never grows.
 */
interface ProbeSessionState {
  lastPrint: string | undefined
  lastMarkerKey: string | undefined
  lastProgressKey: string | undefined
  consecutiveCompactions: number
}

/**
 * Persistent compaction accounting for one probed subtree root
 * (design D1/D2): per-session memories, subtree-wide observability
 * totals, and the fold-computed storm signal. The max and its holder
 * are derived at fold time from that fold's observation order so
 * every reader of the recorded state sees the same tie-break, and
 * only counted sessions contribute to them.
 */
interface ProbeState {
  readonly sessions: Map<string, ProbeSessionState>
  nonReadProgressCount: number
  compactionCount: number
  maxSessionCompactionsSinceProgress: number
  stormSessionID: string | null
}

/**
 * Process-wide probe-state registry. Stored on globalThis under a
 * registered symbol for the same reason as the deadline registry:
 * separately loaded plugin modules share one accounting per probed
 * child. Keyed by the probed root session id.
 */
function probeRegistry(): Map<string, ProbeState> {
  const holder = globalThis as unknown as Record<symbol, Map<string, ProbeState> | undefined>
  let map = holder[PROBE_REGISTRY_KEY]
  if (map === undefined) {
    map = new Map<string, ProbeState>()
    holder[PROBE_REGISTRY_KEY] = map
  }
  return map
}

/**
 * Snapshot a probe state's counters. Shared by the fold and
 * registeredProbeCounters so both read paths expose one shape and
 * one tie-break.
 */
function snapshotCounters(state: ProbeState): ProbeCounters {
  return {
    maxSessionCompactionsSinceProgress: state.maxSessionCompactionsSinceProgress,
    stormSessionID: state.stormSessionID,
    nonReadProgressCount: state.nonReadProgressCount,
    compactionCount: state.compactionCount,
  }
}

/**
 * The recorded compaction accounting for a probed subtree root.
 *
 * @param rootID - The root session id to look up.
 * @returns A snapshot of the counters, or undefined when the root has no probe state.
 */
export function registeredProbeCounters(rootID: string): ProbeCounters | undefined {
  const state = probeRegistry().get(rootID)
  if (state === undefined) {
    return undefined
  }
  return snapshotCounters(state)
}

/**
 * Remove a subtree root's probe accounting once its run has ended.
 * Mirrors unregisterDeadline; the executor drops both when a child
 * finishes so state for finished children cannot accumulate.
 *
 * @param rootID - The root session id whose probe state should be dropped.
 */
export function unregisterProbeState(rootID: string): void {
  probeRegistry().delete(rootID)
}

/** Fingerprint contribution and running parts from one session's latest message. */
interface ProbeContribution {
  readonly print: string
  readonly runningTools: RunningToolPart[]
}

/**
 * Fingerprint contribution of one session's latest message. A marker
 * contributes the session's remembered pre-marker print (or the
 * absent-message placeholder when no real message was ever observed),
 * so neither a marker's arrival nor marker-to-marker churn reads as
 * subtree progress (DT-FR-002: compaction events MUST NOT count as
 * activity changes for the idle clock). Running tool parts are
 * collected from whatever message is latest, marker or not, because
 * DT-FR-014's deferral keys off any running tool in the subtree while
 * the print keeps markers excluded.
 */
function probeContribution(state: ProbeState | undefined, sessionID: string, latest: MessageLike | undefined): ProbeContribution {
  const runningTools = collectRunningTools(latest)
  if (!isCompactionMarker(latest)) {
    return { print: messageFingerprint(sessionID, latest), runningTools }
  }
  return { print: state?.sessions.get(sessionID)?.lastPrint ?? `${sessionID}:-`, runningTools }
}

/**
 * Whether one fold observation accrues toward this probe's storm
 * count (design D2). The probe root is this executor's direct child
 * and is counted unconditionally -- this executor registered its
 * deadline, yet the #1079 root-storm protection must survive the
 * managed-descendant exclusion. Any other session is
 * executor-managed when a run deadline is registered for it: another
 * executeAgentSession run created it and watches it with its own
 * always-armed detector, so its storms are that run's to abort.
 * Unrecognized descendants (no visible deadline, for example a
 * session from another host process) stay counted per session,
 * failing toward detection rather than silence.
 */
function isCountedSession(rootID: string, sessionID: string): boolean {
  return sessionID === rootID || registeredDeadline(sessionID) === undefined
}

/**
 * Fold one completed walk's observations into the root's persistent
 * accounting, in walk order (newest session first). Applied only
 * after the walk succeeds so a failed probe advances nothing
 * (DT-FR-002: a failed probe changes no clock). Marker increments
 * and substantive resets are identity-gated and scoped per session:
 * re-observing the same marker or the same completed tool is a no-op,
 * a new marker grows only its own session's consecutive run, and
 * substantive progress resets only the session that produced it --
 * so a quiet session's stale latest message masks nothing, and
 * parallel read-heavy siblings never sum into one storm signal
 * (issue #1125). Substantive progress is a completed non-read tool
 * call (reads and failed calls never reset; DELEG-FR-002).
 *
 * Executor-managed descendants accrue nothing toward any storm count
 * (design D2): their markers update the identity gates and the
 * subtree-wide observability totals, but only counted sessions grow
 * a consecutive run, and the probe root is counted unconditionally.
 * Marker keys are recorded for every observed session, managed or
 * not, so a marker first seen while managed is never counted
 * retroactively after that run's deadline is unregistered -- counting
 * depends on management status at first observation, which is
 * exactly the run window. The storm signal is the highest
 * per-session run among counted sessions with its holder, and ties
 * resolve to the first session in observation order.
 */
function foldProbeObservations(rootID: string, observations: readonly ProbeObservation[]): ProbeCounters {
  const registry = probeRegistry()
  let state = registry.get(rootID)
  if (state === undefined) {
    state = {
      sessions: new Map(),
      nonReadProgressCount: 0,
      compactionCount: 0,
      maxSessionCompactionsSinceProgress: 0,
      stormSessionID: null,
    }
    registry.set(rootID, state)
  }
  for (const observation of observations) {
    let session = state.sessions.get(observation.id)
    if (session === undefined) {
      session = {
        lastPrint: undefined,
        lastMarkerKey: undefined,
        lastProgressKey: undefined,
        consecutiveCompactions: 0,
      }
      state.sessions.set(observation.id, session)
    }
    if (isCompactionMarker(observation.latest)) {
      const key = markerKey(observation.latest)
      if (session.lastMarkerKey !== key) {
        // The key and the observability total are recorded for every
        // observed session; only the consecutive count is gated on
        // counted sessions, so a managed run's last marker is never
        // re-counted after its deadline is unregistered at run end.
        session.lastMarkerKey = key
        if (isCountedSession(rootID, observation.id)) {
          session.consecutiveCompactions += 1
        }
        state.compactionCount += 1
      }
      continue
    }
    const print = messageFingerprint(observation.id, observation.latest)
    session.lastPrint = print
    if (isSubstantiveProgress(observation.latest) && session.lastProgressKey !== print) {
      session.lastProgressKey = print
      session.consecutiveCompactions = 0
      state.nonReadProgressCount += 1
    }
  }
  let maxSessionCompactions = 0
  let stormSessionID: string | null = null
  // The storm signal is the highest per-session run among counted
  // sessions, never a subtree sum; the strictly-greater comparison
  // resolves ties to the first session in this fold's observation
  // order (the newest-first walk).
  for (const observation of observations) {
    if (!isCountedSession(rootID, observation.id)) {
      continue
    }
    const consecutive = state.sessions.get(observation.id)?.consecutiveCompactions ?? 0
    if (consecutive > maxSessionCompactions) {
      maxSessionCompactions = consecutive
      stormSessionID = observation.id
    }
  }
  state.maxSessionCompactionsSinceProgress = maxSessionCompactions
  state.stormSessionID = stormSessionID
  return snapshotCounters(state)
}

/**
 * Sample the activity of `rootID` and its descendant sessions
 * (depth-first over `session.children`, newest child first, capped at
 * MAX_PROBE_SESSIONS; two requests per session). Depth-first matters:
 * a long-lived child accumulates finished direct children (classifier
 * and reviewer sessions), and a breadth-first walk would spend the cap
 * on them before reaching the active grandchild under the newest one.
 * Returns null when the client lacks the probe endpoints or any
 * request fails, so callers never mistake an unreachable API for a
 * stalled child; a failed probe also leaves the compaction accounting
 * untouched because the fold runs only after a successful walk.
 *
 * The sample carries the compaction accounting (design D1/D2):
 * compaction markers never change the fingerprint, and the counters
 * expose the highest per-session run of consecutive markers and its
 * holder for the idle watchdog and the storm detector to share
 * (DT-FR-002 / DELEG-FR-002). Only counted sessions accrue that run
 * -- executor-managed descendants are excluded while the probe root
 * stays counted -- and the observability totals stay subtree-wide.
 * It also carries the subtree's running tool-call parts (DT-FR-013):
 * the triples are already folded into each session's fingerprint
 * print, and the collected set feeds the idle watcher's bounded
 * deferral (DT-FR-014) and the abort envelope's running_tool
 * selection (DT-FR-015). Accounting persists across probes per root
 * id and per session; callers drop it with unregisterProbeState when
 * the run ends.
 */
export async function probeSubtreeActivity(
  client: Client,
  rootID: string,
  directory: string,
  signal: AbortSignal,
): Promise<ActivitySample | null> {
  if (typeof client.session?.children !== "function" || typeof client.session?.messages !== "function") {
    return null
  }
  try {
    const stack = [rootID]
    const seen = new Set<string>()
    const prints: string[] = []
    const runningTools: RunningToolPart[] = []
    const observations: ProbeObservation[] = []
    let pendingQuestion = false
    // Read-only during the walk: foldProbeObservations is the only
    // writer, and it runs only after the walk succeeds.
    const state = probeRegistry().get(rootID)
    while (stack.length > 0 && seen.size < MAX_PROBE_SESSIONS) {
      const id = stack.pop() as string
      if (seen.has(id)) {
        continue
      }
      seen.add(id)
      const messages = await client.session.messages({ path: { id }, query: { directory, limit: 1 }, signal })
      if (messages.data === undefined) {
        return null
      }
      const latest = messages.data[messages.data.length - 1] as MessageLike | undefined
      observations.push({ id, latest })
      const contribution = probeContribution(state, id, latest)
      prints.push(contribution.print)
      runningTools.push(...contribution.runningTools)
      pendingQuestion ||= isPendingQuestion(latest)
      const children = await client.session.children({ path: { id }, query: { directory }, signal })
      if (children.data === undefined) {
        return null
      }
      // The child count rides in the fingerprint so a newly spawned
      // session registers as progress even past the session cap. Children
      // are pushed oldest first so the newest (the active one) pops next.
      prints.push(`${id}#${children.data.length}`)
      const oldestFirst = [...children.data].sort((a, b) => (a.time?.created ?? 0) - (b.time?.created ?? 0))
      for (const child of oldestFirst) {
        stack.push(child.id)
      }
    }
    return {
      fingerprint: prints.join("|"),
      pendingQuestion,
      runningTools: runningTools.sort(compareRunningTools),
      ...foldProbeObservations(rootID, observations),
    }
  } catch {
    return null
  }
}
