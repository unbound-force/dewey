# Feature Specification: Structured Diagnostics

**Feature Branch**: `009-structured-diagnostics`
**Created**: 2026-04-04
**Status**: Draft
**Input**: Structured diagnostic logging optimized for AI-assisted debugging with ISO 8601 timestamps, phase timing, component prefixes, PID lock files, and server ready marker

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Diagnose MCP Startup Timeout (Priority: P1)

An AI agent is helping a developer debug why `dewey serve` exceeds OpenCode's 30-second MCP connection timeout. Today, the log file has no timestamps, no phase timing, and no "server ready" marker — the AI cannot determine which startup phase is slow, how long each phase took, or whether the server ever became ready. With structured diagnostics, the AI reads the log file and immediately identifies: "Store opened in 42ms, incremental index took 28.3s (42 new pages with embeddings), server never reached ready state within the timeout."

**Why this priority**: MCP startup timeout is the most common and most frustrating diagnostic scenario. It has occurred multiple times across different repos (website, unbound-force) and is the hardest to debug without phase timing.

**Independent Test**: Start `dewey serve --verbose` on a vault, capture the log output, and verify every startup phase has a timestamp and elapsed duration. Verify a "server ready" line appears with total startup time.

**Acceptance Scenarios**:

1. **Given** `dewey serve` starts normally, **When** the log file is read, **Then** every line has an ISO 8601 timestamp with millisecond precision
2. **Given** `dewey serve` completes startup, **When** the log file is read, **Then** the store open, indexing, external page loading, and watcher phases each report their elapsed duration as a structured field
3. **Given** `dewey serve` completes startup, **When** the log file is read, **Then** a "server ready" line appears with transport type, tool count, and total startup duration
4. **Given** `dewey serve` times out during Ollama polling, **When** the log file is read, **Then** progress messages appear every few seconds showing elapsed time vs timeout

---

### User Story 2 - Identify Lock File Holder (Priority: P2)

An AI agent encounters "another Dewey process is using this database" when trying to start `dewey serve`. Today, the error message suggests `lsof` as a workaround but doesn't identify the holding process. With structured diagnostics, the lock file contains the PID and command name of the holder, and both the error message and `dewey doctor` report it: "locked by PID 12345 (dewey serve --vault /path)."

**Why this priority**: Lock conflicts are the second most common diagnostic scenario after startup timeouts. They occur when a previous `dewey serve` didn't exit cleanly, or when multiple OpenCode sessions target the same vault.

**Independent Test**: Start `dewey serve` (acquires lock). Attempt a second `dewey serve` on the same vault. Verify the error message includes the PID and command of the first process. Run `dewey doctor` and verify it reports the same information.

**Acceptance Scenarios**:

1. **Given** a dewey process holds the lock, **When** a second process attempts to acquire the lock, **Then** the error message includes the PID and command name of the holding process
2. **Given** `dewey doctor` runs on a vault with a held lock, **When** the MCP Server section is reached, **Then** it reports "running (PID 12345, dewey serve)" instead of "try: lsof"
3. **Given** a stale lock file exists (no process holds it), **When** `dewey doctor` runs, **Then** it reports the lock is stale and suggests removing it

---

### User Story 3 - Trace Which Package Emitted a Log (Priority: P3)

An AI agent reads a dewey log file and sees multiple lines from different subsystems — vault indexing, source loading, ignore pattern matching, main server logic. Today, all lines show `dewey:` as the prefix, making it impossible to determine which package emitted a message. With component-prefixed logging, the AI can immediately filter by package: "All `dewey/vault:` messages show normal indexing, but `dewey/ignore:` has a warning about a malformed glob pattern."

**Why this priority**: Component discrimination helps AI agents narrow their investigation scope. It's less critical than timing (P1) and lock identification (P2) because most debugging focuses on the main startup path.

**Independent Test**: Start `dewey serve --verbose`, trigger activities across vault, source, and ignore packages, and verify each log line has a distinct component prefix.

**Acceptance Scenarios**:

1. **Given** `dewey serve --verbose` is running, **When** an AI reads the log, **Then** vault messages use `dewey/vault:`, source messages use `dewey/source:`, ignore messages use `dewey/ignore:`, and main server messages use `dewey:`
2. **Given** the `--verbose` flag is set, **When** the `ignore` package encounters a malformed glob pattern, **Then** a DEBUG-level warning appears in the log file (not just stderr)

---

### User Story 4 - Surface Silent File Skips (Priority: P4)

An AI agent is debugging why a specific markdown file is not appearing in search results. Today, the vault walker silently skips unreadable files and directories with `return nil`. With structured diagnostics, `--verbose` mode logs each skipped file with its path and the reason, letting the AI grep for the missing file: "Skipping docs/private.md: permission denied."

**Why this priority**: This is a niche diagnostic scenario but has zero visibility today. Seven locations in the codebase silently discard filesystem errors.

**Independent Test**: Create a vault with an unreadable file (permissions 000), run `dewey serve --verbose`, and verify the log contains a DEBUG-level message about the skipped file.

**Acceptance Scenarios**:

1. **Given** `dewey serve --verbose` is running, **When** the vault walker encounters an unreadable file, **Then** a DEBUG-level log line appears with `path` and `err` structured fields
2. **Given** `dewey serve` is running without `--verbose`, **When** the vault walker encounters an unreadable file, **Then** no log line appears (silent, existing behavior)

---

### Edge Cases

- What happens when the lock file contains a PID of a dead process? The lock is stale — `flock` will succeed. `dewey doctor` should detect this and report the stale lock with the dead PID for context.
- What happens when timestamps are enabled in piped/non-TTY output? Timestamps appear regardless of terminal detection. This is intentional — log files are the primary consumer and always need timestamps.
- What happens when the log file is truncated at 10 MB? Existing truncation behavior is preserved. Timestamps make the truncated file more useful because you can see when the truncation boundary falls.
- What happens in tests? Test helpers should not produce timestamped output unless explicitly testing timestamp behavior. Phase timing should be disable-able or mockable for deterministic tests.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: All logger instances MUST include ISO 8601 timestamps with millisecond precision on every log line
- **FR-002**: Each startup phase (store open, Ollama check, incremental/full index, external page load, watcher setup) MUST report its elapsed duration as an `elapsed` structured field on its completion log line
- **FR-003**: A "server ready" log line MUST be emitted when the MCP server is fully initialized, including transport type, tool count, and total startup duration
- **FR-004**: Logger prefixes MUST differentiate the emitting package: `dewey` (main), `dewey/vault` (vault), `dewey/source` (source), `dewey/ignore` (ignore)
- **FR-005**: The `ignore` package MUST be wired to `--verbose` level propagation and file logging output, matching the existing `vault` and `source` packages
- **FR-006**: The lock file MUST contain the PID and command name of the acquiring process, readable by `dewey doctor` and lock error messages
- **FR-007**: Lock acquisition and release MUST be logged at DEBUG level with the PID
- **FR-008**: `dewey doctor` MUST report the PID from the lock file when a lock is held, replacing the current "try: lsof" suggestion
- **FR-009**: `dewey doctor` MUST suggest removing stale lock files when detected (lock file exists but no process holds it)
- **FR-010**: Filesystem walker errors that are currently silently discarded MUST be logged at DEBUG level with `path` and `err` structured fields
- **FR-011**: Ollama readiness polling MUST log progress at DEBUG level every 5 seconds with elapsed time and timeout
- **FR-012**: The resolved vault path MUST be logged at startup
- **FR-013**: The MCP server shutdown MUST be logged for both stdio and HTTP transports

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An AI agent can determine which startup phase caused a timeout by reading only the log file — phase timing, timestamps, and "server ready" marker are sufficient to diagnose without additional commands
- **SC-002**: An AI agent can identify the process holding a lock by reading the lock error message or `dewey doctor` output — no need to run `lsof` or `ps`
- **SC-003**: An AI agent can determine which package emitted every log line by inspecting the component prefix — zero ambiguous lines
- **SC-004**: All existing tests continue to pass after the logging changes — no behavioral regressions
- **SC-005**: The `.dewey/dewey.log` file contains all diagnostic information needed for post-mortem analysis of any startup failure — timestamps, phases, durations, component, and errors

## Assumptions

- ISO 8601 format is `2006-01-02T15:04:05.000Z07:00` (Go reference time with milliseconds). This is unambiguous, sortable, and timezone-aware.
- Component prefixes use `/` as separator (`dewey/vault`) following standard Go package path conventions.
- Lock file PID is written as `{pid} {command}\n` (e.g., `12345 dewey serve --vault /path`). Simple text format, parseable with a single `strings.SplitN`.
- Phase timing uses `time.Since()` with the existing `logger.Info()` calls — no new messages, just adding an `elapsed` field to existing ones.
- The "server ready" marker is a new INFO-level log line, not a change to an existing one.
- Test assertions that match on log output may need updating if they check for exact prefix strings. Tests that capture log output via `bytes.Buffer` will see the new timestamps and prefixes.
