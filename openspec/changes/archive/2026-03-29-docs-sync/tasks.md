# Tasks: docs-sync

## 1. README.md Fixes

- [x] 1.1 Fix `DEWEY_BACKEND` default from `logseq` to `obsidian` in the environment variables table in `README.md`
- [x] 1.2 Verify `brew install ollama` line is already updated to `brew install --cask ollama-app` in `README.md` (was fixed in v0.5.0 — confirm no regression) — VERIFIED: line 390
- [x] 1.3 Add `.dewey/dewey.log` auto-logging behavior to the README (mention it's created by `dewey serve`, truncated at 10 MB)
- [x] 1.4 Update the Logseq read-only MCP config example to note that Obsidian is the default backend

## 2. AGENTS.md Updates

- [x] 2.1 Add `dewey doctor`, `dewey reindex`, `dewey search` to the Architecture CLI description in `AGENTS.md`
- [x] 2.2 Add `vault/parse_export.go` to the Architecture package listing in `AGENTS.md` with description: "Exported parsing and persistence functions (ParseDocument, PersistBlocks, PersistLinks, GenerateEmbeddings)"
- [x] 2.3 Add `--verbose`, `--log-file`, `--no-embeddings` to the Build & Test Commands section or a new Global Flags section in `AGENTS.md`
- [x] 2.4 Update the `dewey index` command description in `AGENTS.md` to mention `--vault`, `--force`, `--no-embeddings` flags

## 3. Spec Artifacts

- [x] 3.1 Update `specs/004-unified-content-serve/spec.md` status from `Draft` to `Complete`

## 4. OpenSpec Archives

- [x] 4.1 Move `openspec/changes/ollama-hard-error/` to `openspec/changes/archive/2026-03-29-ollama-hard-error/`

## 5. Verification

- [x] 5.1 Run `go build ./...` to verify no code was accidentally changed
- [x] 5.2 Grep README.md and AGENTS.md for remaining references to `brew install ollama` (without `-app`) — should be zero — VERIFIED
- [x] 5.3 Grep for `DEWEY_BACKEND.*logseq` in README.md — should be zero (except in the Logseq-specific section) — VERIFIED: only correct reference at line 271
