## Why

Dewey defaults to the Logseq backend, which requires a
running Logseq application. No Dewey user runs Logseq.
The Obsidian backend reads Markdown files directly from
disk, which is what every user wants. The wrong default
causes silent failures in every OpenCode session.

## What Changes

Change the default backend from `logseq` to `obsidian`
in all locations: main.go flag defaults,
resolveBackendType() fallback, server.go health check,
and help text.

## Impact

- `main.go` -- flag help text, resolveBackendType()
- `main_test.go` -- test assertions for default
- `server.go` -- health check default
- CLI help text

## Constitution Alignment

N/A -- one-line default change.
