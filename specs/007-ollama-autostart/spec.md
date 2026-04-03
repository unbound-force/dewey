# Feature Specification: Ollama Auto-Start

**Feature Branch**: `007-ollama-autostart`
**Created**: 2026-03-30
**Status**: Draft
**Input**: GitHub issue #24 — Auto-start Ollama when dewey serve launches (Spec 021 FR-001–FR-006)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Zero-Config Semantic Search (Priority: P1)

A developer opens their project in OpenCode. The Dewey MCP server starts via `dewey serve`. Ollama is installed but not currently running. Instead of falling back to keyword-only mode (losing semantic search), Dewey detects that Ollama is available but not running, starts it as a managed subprocess, waits for it to be ready, and then accepts MCP tool calls with full semantic search capabilities. The developer never has to manually start Ollama — it just works.

**Why this priority**: This is the core value of the feature. Today, developers who forget to start Ollama (or whose system rebooted) silently lose semantic search capabilities. Auto-start eliminates this friction and makes the "happy path" automatic.

**Independent Test**: Stop Ollama. Start `dewey serve`. Verify Ollama is now running. Verify `dewey_semantic_search` returns embedding-based results.

**Acceptance Scenarios**:

1. **Given** Ollama is installed but not running, **When** `dewey serve` starts, **Then** Dewey starts Ollama as a managed subprocess and logs an informational message indicating Ollama was auto-started
2. **Given** Dewey has auto-started Ollama, **When** Ollama's health check (`GET /api/tags`) returns 200, **Then** Dewey proceeds with full semantic search capabilities (embedding generation and semantic query tools are available)
3. **Given** Dewey has auto-started Ollama, **When** a user calls `dewey_semantic_search`, **Then** the tool returns embedding-based similarity results (not a "no embeddings" error)

---

### User Story 2 - Respect External Ollama (Priority: P2)

A developer already has Ollama running — perhaps they use it for other AI tools, or they started it manually with custom configuration. When Dewey starts, it detects the existing Ollama instance and uses it without modification. Dewey does not start a second Ollama process, does not interfere with the running instance's configuration, and does not stop it when Dewey exits.

**Why this priority**: This prevents Dewey from disrupting a developer's existing Ollama setup. It's the "do no harm" scenario — if Ollama is already working, Dewey should just use it.

**Independent Test**: Start Ollama manually. Start `dewey serve`. Verify Dewey uses the existing instance (no second Ollama process spawned). Stop `dewey serve`. Verify Ollama is still running.

**Acceptance Scenarios**:

1. **Given** Ollama is already running at the configured endpoint, **When** `dewey serve` starts, **Then** Dewey detects the existing instance and uses it without starting a subprocess
2. **Given** Dewey is using an externally-managed Ollama, **When** `dewey serve` exits, **Then** the external Ollama continues running

---

### User Story 3 - Graceful Degradation Without Ollama (Priority: P3)

A developer does not have Ollama installed at all. When Dewey starts, it detects this and operates in keyword-only mode with a clear informational message. No error, no crash — just reduced functionality with an explanation of what's missing and how to install it.

**Why this priority**: This preserves dewey's composability principle — it works without Ollama, just with less capability. The log message helps developers who want semantic search know how to enable it.

**Independent Test**: Remove or rename the Ollama binary so it's not in PATH. Start `dewey serve`. Verify keyword-only mode with informational log. Verify all non-semantic MCP tools work normally.

**Acceptance Scenarios**:

1. **Given** Ollama is not installed (binary not in PATH) and not running at the configured endpoint, **When** `dewey serve` starts, **Then** Dewey operates in keyword-only mode and logs an informational message explaining that semantic search is unavailable
2. **Given** Dewey is in keyword-only mode, **When** a user calls `dewey_search` or `dewey_find_by_tag`, **Then** the tools work normally (structured queries are unaffected)

---

### User Story 4 - Leave Ollama Running on Exit (Priority: P4)

When Dewey auto-started Ollama and then Dewey exits (user closes OpenCode, process killed, etc.), Ollama should remain running. Other tools may depend on Ollama, and stopping it would disrupt their operation. Dewey starts Ollama when needed but does not own its shutdown lifecycle.

**Why this priority**: This is a safety constraint. Stopping Ollama on Dewey exit would break other Ollama consumers and create a confusing "Ollama keeps disappearing" experience.

**Independent Test**: Start `dewey serve` (which auto-starts Ollama). Stop `dewey serve`. Verify Ollama is still running.

**Acceptance Scenarios**:

1. **Given** Dewey auto-started Ollama as a managed subprocess, **When** `dewey serve` exits normally or is terminated, **Then** Ollama continues running
2. **Given** Dewey auto-started Ollama, **When** a second `dewey serve` starts in another project, **Then** it detects the already-running Ollama and uses it (US2 behavior)

---

### Edge Cases

- What happens when the Ollama binary exists but fails to start (e.g., port already in use by another process)? Dewey should log a warning with the error details and fall back to keyword-only mode. No crash.
- What happens when Ollama starts but takes longer than expected to become ready? Dewey should have a bounded wait (timeout) and fall back to keyword-only mode if the timeout expires.
- What happens when Ollama is running but the required embedding model is not pulled? This is the existing behavior — Dewey already checks for model availability after Ollama is detected. This feature does not change that flow.
- What happens when `--no-embeddings` flag is set? Dewey should skip the Ollama check entirely — if the user explicitly opted out of embeddings, there's no reason to start Ollama.
- What happens when the Ollama endpoint is configured to a non-default address (remote Ollama)? The auto-start logic should only attempt to start a local subprocess when the endpoint is `localhost` or `127.0.0.1`. For remote endpoints, only the health check applies.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `dewey serve` MUST check if Ollama is running at the configured endpoint on startup by making a health check request
- **FR-002**: If Ollama is not running and the `ollama` binary is in PATH, `dewey serve` MUST start `ollama serve` as a managed subprocess
- **FR-003**: `dewey serve` MUST wait for Ollama to be ready (health check passes) before accepting MCP tool calls that require embeddings, with a bounded timeout
- **FR-004**: If Ollama was auto-started by Dewey, `dewey serve` MUST leave it running on exit (do not send SIGTERM/SIGKILL to the subprocess on shutdown)
- **FR-005**: If Ollama is already running (not started by Dewey), `dewey serve` MUST use it without modification and without starting a second process
- **FR-006**: If Ollama is not available (not installed and not running), `dewey serve` MUST operate in keyword-only mode with an informational log message
- **FR-007**: If the `--no-embeddings` flag is set, `dewey serve` MUST skip the Ollama detection and auto-start logic entirely
- **FR-008**: Auto-start MUST only attempt to start a local subprocess when the configured endpoint is `localhost` or `127.0.0.1` — remote endpoints only receive health checks
- **FR-009**: `dewey doctor` MUST report the Ollama state (external, managed, unavailable) in the Embedding Layer section
- **FR-010**: The Ollama startup timeout MUST be configurable or have a reasonable default (30 seconds)

### Key Entities

- **Ollama State**: One of three states — `External` (already running, not managed by Dewey), `Managed` (auto-started by Dewey), or `Unavailable` (not installed and not running). The state determines behavior at startup and shutdown.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer with Ollama installed but not running can start `dewey serve` and get semantic search results within 10 seconds of Ollama's first startup, without any manual intervention
- **SC-002**: A developer with Ollama already running sees no change in behavior — no extra processes, no log warnings, same startup speed
- **SC-003**: A developer without Ollama installed sees a clear informational message and all non-semantic tools work normally
- **SC-004**: Stopping `dewey serve` after auto-starting Ollama leaves Ollama running — verified by `pgrep ollama` returning a result after dewey exits
- **SC-005**: All existing tests continue to pass — no behavioral change for the `--no-embeddings` flag or keyword-only mode

## Assumptions

- The Ollama health check endpoint is `GET /api/tags` at the configured endpoint (default `http://localhost:11434`). A 200 response indicates Ollama is ready.
- The `ollama serve` command starts the Ollama server on the default port. If the default port is in use by a non-Ollama process, the startup will fail and Dewey falls back to keyword-only mode.
- The subprocess is started with `os/exec.Command("ollama", "serve")` and detached so it outlives the Dewey process. On macOS/Linux, this means setting `SysProcAttr` to prevent the child from receiving signals when the parent exits.
- This feature does not change the existing embedding model availability check. If Ollama is running but the required model is not pulled, Dewey's existing error handling applies.
- The 30-second startup timeout is a reasonable default. Ollama typically starts in 1-3 seconds on modern hardware, but cold starts (downloading model layers) can take longer.
