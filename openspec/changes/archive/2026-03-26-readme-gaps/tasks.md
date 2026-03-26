## 1. Preparation

- [x] 1.1 Run `dewey --help`, `dewey serve --help`, `dewey init --help`, `dewey index --help`, `dewey status --help`, `dewey source --help` to capture v0.2.0 CLI reference
- [x] 1.2 Read the current README.md to identify exact insertion points for each new section

## 2. Homebrew Cask Install

- [x] 2.1 Add Homebrew cask install section to README.md Install section — `brew install --cask unbound-force/tap/dewey` as the first option, before `go install`
- [x] 2.2 Add a note that Homebrew cask is macOS-only and `go install` works on all platforms

## 3. Persistence Documentation

- [x] 3.1 Add "Persistence" section to README.md between "Configuration" and "CLI Commands" explaining: `.dewey/` directory, `graph.db` SQLite database, incremental indexing (only changed files reprocessed), near-instant startup after first index

## 4. Content Sources Guide

- [x] 4.1 Add "Content Sources" section to README.md after "CLI Commands" with a complete `sources.yaml` example showing all 3 source types (local disk with path, GitHub with org/repos/content_types/refresh, web crawl with urls/depth/refresh)
- [x] 4.2 Add a note explaining the `dewey source add` command as an alternative to editing YAML directly, and `dewey index --force` for full rebuild

## 5. Semantic Search Setup

- [x] 5.1 Add "Semantic Search Setup" section to README.md after "Content Sources" with: Ollama install (`brew install ollama` or from ollama.ai), model pull (`ollama pull granite-embedding:30m`), verification (`dewey status` shows embedding coverage)
- [x] 5.2 Add a note about graceful degradation: all 37 keyword-based tools work without Ollama; only the 3 semantic search tools require it

## 6. Verification

- [x] 6.1 Cross-reference all documented commands, flags, and config keys against `dewey --help` output — verify zero inaccuracies
- [x] 6.2 Run `go build ./...` and `go vet ./...` to confirm no code was accidentally modified
- [x] 6.3 Verify README renders correctly in GitHub markdown preview (tables, code blocks, headings)
- [x] 6.4 Verify constitution alignment: Composability First (Dewey documented as standalone), Observable Quality (`dewey status` documented for verification)
