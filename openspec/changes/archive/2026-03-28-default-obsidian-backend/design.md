## Context

Dewey inherits `logseq` as the default backend from
graphthulhu. This requires a running Logseq app. The
`obsidian` backend reads Markdown files directly.

## Changes

1. `main.go:69,106` -- flag help text: "logseq (default)" -> "obsidian (default)"
2. `main.go:157` -- `resolveBackendType()` fallback: return "obsidian" instead of "logseq"
3. `main.go:149` -- comment: update "falling back to logseq" -> "falling back to obsidian"
4. `server.go:351` -- health check default: "logseq" -> "obsidian"
5. `main_test.go` -- update assertions that expect "logseq" as default

The `logseq` backend remains available via `--backend logseq`.
