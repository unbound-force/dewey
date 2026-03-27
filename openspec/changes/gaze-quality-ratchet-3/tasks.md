# Tasks

## 1. Decompose `RenamePage` (vault/vault.go)

- [x] 1.1 Extract `renameFile(oldPath, newAbsPath string) error` — creates target directory with `os.MkdirAll` and renames the file with `os.Rename`
- [x] 1.2 Extract `reindexRenamed(newRelPath, newAbsPath string)` — reads the renamed file content, stats it, and calls `indexFileCore` + `rebuildLinksLocked`
- [x] 1.3 Extract `cleanupEmptyDirs(startDir, vaultAbs string)` — walks up from startDir removing empty directories until reaching vaultAbs or a non-empty directory
- [x] 1.4 Refactor `RenamePage` into: validate → compute paths → renameFile → updateLinksAcrossVaultLocked → removePageFromIndexLocked → reindexRenamed → cleanupEmptyDirs
- [x] 1.5 Run `go test -race -count=1 ./vault/...` and verify all existing tests pass
- [x] 1.6 Run `gaze crap --format=json ./vault/...` and verify `RenamePage` CRAP score dropped below 15

## 2. Decompose `newSourceAddCmd` (cli.go)

- [x] 2.1 Extract `buildGitHubSource(org, repos, content, refresh string) (source.SourceConfig, error)` — validates required fields, parses comma-separated lists, returns populated SourceConfig
- [x] 2.2 Extract `buildWebSource(webURL, webName, refresh string, depth int) (source.SourceConfig, error)` — validates URL, derives name from hostname if empty, returns populated SourceConfig
- [x] 2.3 Extract `saveSourceConfig(sourcesPath string, existing []source.SourceConfig, newSource source.SourceConfig) error` — checks for duplicate source IDs, appends, and saves to YAML
- [x] 2.4 Refactor `newSourceAddCmd` RunE into orchestrator: get cwd → load existing → switch type → build config → save → log
- [x] 2.5 Run `go test -race -count=1 ./...` and verify all tests pass

## 3. Add tests for `indexDocuments` (cli.go)

- [x] 3.1 Add `TestIndexDocuments_InsertNew` — create in-memory store, call `indexDocuments` with one document, verify `s.GetPage()` returns page with correct title, content hash, source ID, and source doc ID
- [x] 3.2 Add `TestIndexDocuments_UpdateExisting` — insert a page first, call `indexDocuments` with same title but different hash, verify page's content hash was updated
- [x] 3.3 Add `TestIndexDocuments_SourceRecord` — call `indexDocuments` with documents from a new source, verify `s.GetSource()` returns a source record with correct type and status
- [x] 3.4 Add `TestIndexDocuments_WithProperties` — call with document having non-nil Properties map, verify page's Properties field is valid JSON matching the map
- [x] 3.5 Run `go test -race -count=1 ./...` and verify all tests pass

## 4. Restructure whiteboard test assertions (tools/whiteboard_test.go)

- [x] 4.1 Refactor `TestGetWhiteboard` assertions to use direct map field access with dedicated `t.Errorf` calls for `embeddedPages`, `connections`, `elementCount`, and `elements` fields
- [x] 4.2 Add explicit type assertion checks: verify `embeddedPages` is `[]any`, `connections` is `[]any`, each connection has `source` and `target` string fields
- [x] 4.3 Run `go test -race -count=1 ./tools/...` and verify all tests pass

## 5. Decompose `(*DiskSource).Diff` (source/disk.go)

- [x] 5.1 Extract `walkDiskFiles(basePath string) (map[string]string, error)` — walks directory, skips hidden dirs and non-.md files, returns relPath → content hash map
- [x] 5.2 Extract `diffFileChanges(currentFiles, storedHashes map[string]string, fetcher func(string) (*Document, error)) []Change` — compares hashes, calls fetcher for added/modified files, returns categorized changes
- [x] 5.3 Refactor `Diff` into orchestrator: walkDiskFiles → diffFileChanges → return
- [x] 5.4 Add unit test for `walkDiskFiles` — verify it skips hidden dirs and non-.md files, returns correct hashes for .md files
- [x] 5.5 Run `go test -race -count=1 ./source/...` and verify all tests pass
- [x] 5.6 Run `gaze crap --format=json ./source/...` and verify `Diff` CRAP score dropped below 15

## 6. Verification

- [x] 6.1 Run full CI-equivalent checks: `go build ./... && go vet ./... && go test -race -count=1 ./...` — all 11 packages pass
- [x] 6.2 Gaze threshold gate: CRAPload=6 (pass, gate 15), GazeCRAPload=31 (pass, gate 34)
- [x] 6.3 CRAPload = 6 (target ≤7, EXCEEDED TARGET)
- [x] 6.4 GazeCRAPload = 31 (target ≤28 not fully met, improved from 33, CI gate passes comfortably)
- [x] 6.5 All tests pass including tool count assertions — 40 tools registered
- [x] 6.6 Constitution alignment verified: Composability First (no new deps), Observable Quality (Gaze gates pass with significant headroom), Testability (all packages pass -race)
