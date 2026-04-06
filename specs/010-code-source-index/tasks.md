# Tasks: Code Source Indexing & Manifest Generation

**Input**: Design documents from `/specs/010-code-source-index/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md, contracts/

**Tests**: Tests are included per spec requirement — the chunker package requires ≥80% coverage, code source ≥70%, and manifest command ≥70% (plan.md Coverage Strategy).

**Organization**: Tasks are grouped by implementation phase following the dependency chain: `chunker/` → `source/code.go` → `manifest command` → documentation → verification.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)
- Include exact file paths in descriptions

---

## Phase 1: Chunker Package (Foundation)

**Purpose**: Create the `chunker/` package with the interface, Block type, registry, and Go language implementation. This is the foundation — all subsequent phases depend on it.

**⚠️ CRITICAL**: No code source or manifest work can begin until this phase is complete.

### Chunker Interface & Registry

- [x] T001 [US4] Create `chunker/chunker.go` — Define `Chunker` interface (4 methods: `ChunkFile`, `IsTestFile`, `Extensions`, `Language`), `Block` struct (Heading, Content, Kind fields), and registry functions (`Register`, `Get`, `Languages`, `ExtensionMap`). Registry uses `sync.RWMutex`-protected map. `Register` panics on duplicate language (programming error). Per contracts/chunker-interface.md.

### Go Chunker Implementation

- [x] T002 [US1] [US2] Create `chunker/go_chunker.go` — Implement `GoChunker` struct satisfying `Chunker` interface. Core method `ChunkFile` uses `go/parser.ParseFile()` with `parser.ParseComments` to parse source, then extracts 7 block kinds via dedicated helper functions. Register via `init()`. Per contracts/go-chunker.md.

The 7 block kinds and their extraction helpers:

1. **Package doc** (Kind: `"package"`) — Extract `ast.File.Doc` + package name. Heading: `"package <name>"`.
2. **Exported func signatures** (Kind: `"func"`) — Extract `ast.FuncDecl` where `Recv == nil` and name is exported. Format signature via `go/format.Node()` with `Body` set to nil. Heading: `"func <Name>"`.
3. **Exported method signatures** (Kind: `"func"`) — Extract `ast.FuncDecl` where `Recv != nil` and name is exported. Format with receiver. Heading: `"func (<RecvType>) <Name>"`.
4. **Exported type declarations** (Kind: `"type"`) — Extract `ast.GenDecl` with `token.TYPE` containing exported `ast.TypeSpec`. Format full type including struct/interface fields. Heading: `"type <Name>"`.
5. **Exported const/var declarations** (Kind: `"const"` or `"var"`) — Extract `ast.GenDecl` with `token.CONST`/`token.VAR` containing exported specs. Heading: `"const <Name>"` or `"var <Name>"`.
6. **Cobra command definitions** (Kind: `"command"`) — Walk AST via `ast.Inspect` looking for `ast.CompositeLit` with type `cobra.Command`. Extract `Use`, `Short`, `Long` string fields from `ast.KeyValueExpr`. Heading: `"command: <Use>"`.
7. **MCP tool registrations** (Kind: `"tool"`) — Walk AST via `ast.Inspect` looking for `ast.CallExpr` with `mcp.AddTool` selector. Extract `Name`, `Description` from the `mcp.Tool` composite literal argument. Heading: `"tool: <Name>"`.

### Go Chunker Tests

- [x] T003 [P] [US1] [US2] Create `chunker/go_chunker_test.go` — Table-driven tests using Go source code string literals as input. Per plan.md test strategy and contracts/go-chunker.md invariants.

Required test cases:
- `TestGoChunker_PackageDoc` — file with package doc comment → Block with Kind "package"
- `TestGoChunker_ExportedFunc` — exported function with doc comment → Block with Kind "func", no body in Content
- `TestGoChunker_ExportedMethod` — method with receiver → Block with Kind "func", receiver in Heading
- `TestGoChunker_UnexportedFunc` — unexported function → not included in output
- `TestGoChunker_ExportedType` — struct type with fields → Block with Kind "type"
- `TestGoChunker_ExportedConst` — const declaration → Block with Kind "const"
- `TestGoChunker_ExportedVar` — var declaration → Block with Kind "var"
- `TestGoChunker_CobraCommand` — `&cobra.Command{Use: "serve", Short: "..."}` → Block with Kind "command"
- `TestGoChunker_MCPTool` — `mcp.AddTool(srv, &mcp.Tool{Name: "...", Description: "..."}, handler)` → Block with Kind "tool"
- `TestGoChunker_SyntaxError` — invalid Go source → returns error, no panic
- `TestGoChunker_EmptyFile` — file with no exports → returns empty slice (not nil)
- `TestGoChunker_NoDocComment` — exported func without doc comment → Block with empty doc prefix
- `TestGoChunker_IsTestFile` — `"foo_test.go"` → true, `"foo.go"` → false
- `TestGoChunker_Extensions` — returns `[".go"]`
- `TestGoChunker_Language` — returns `"go"`

**Checkpoint**: `go test ./chunker/...` passes. All 7 block kinds are extracted correctly. Syntax errors return errors without panics. Coverage ≥80%.

---

## Phase 2: Code Source (Integration with Indexing Pipeline)

**Purpose**: Create `CodeSource` in `source/code.go` and integrate with config validation and source manager. This connects the chunker to the existing indexing pipeline.

### Code Source Implementation

- [x] T004 [US1] [US2] Create `source/code.go` — Implement `CodeSource` struct satisfying `Source` interface (`List`, `Fetch`, `Diff`, `Meta`). Constructor `NewCodeSource(id, name, basePath string, languages []string, opts ...CodeSourceOption)` with functional options (`WithCodeIgnorePatterns`, `WithCodeInclude`, `WithCodeExclude`, `WithCodeRecursive`). `List()` walks basePath, filters by extension/language/ignore/include/exclude, calls chunker, formats blocks as markdown documents. Per contracts/code-source.md.

Key behaviors:
- `List()` produces one `Document` per source file with markdown-formatted content (heading per declaration)
- Skips test files via `chunker.IsTestFile()` (FR-014)
- Skips files with syntax errors with logged warning (FR-013)
- Logs warning for unsupported languages (FR-009)
- Respects `.gitignore` patterns (FR-007)
- Supports `include`/`exclude` glob patterns (FR-008)
- `Meta().Type` returns `"code"`

### Manager Integration

- [x] T005 [US1] Update `source/manager.go` — Add `case "code":` to `createSource()` switch statement. Implement `createCodeSource(cfg SourceConfig, basePath string) (Source, error)` that extracts `path`, `languages`, `include`, `exclude`, `ignore`, `recursive` from config map and returns `NewCodeSource(...)`. Per contracts/config-validation.md Manager Integration section.

### Config Validation

- [x] T006 [P] [US1] Update `source/config.go` — Add `type: code` validation to `validateSourceConfig()`. When `src.Type == "code"`: require non-nil config map, require non-empty `path` field, require non-empty `languages` field. Per contracts/config-validation.md Validation Rules.

### Code Source Tests

- [x] T007 [US1] [US2] Create `source/code_test.go` — Tests using `t.TempDir()` fixtures with `.go` files. Per plan.md test strategy.

Required test cases:
- `TestCodeSource_List` — directory with Go files → returns Documents with markdown content containing declarations
- `TestCodeSource_ListSkipsTestFiles` — directory with `*_test.go` → test files excluded from output
- `TestCodeSource_ListSkipsSyntaxErrors` — file with invalid Go → skipped, no error returned, other files still processed
- `TestCodeSource_ListRespectsInclude` — `include: ["cmd/"]` → only files under `cmd/` processed
- `TestCodeSource_ListRespectsExclude` — `exclude: ["vendor/"]` → vendor files skipped
- `TestCodeSource_Meta` — `Meta().Type` returns `"code"`
- `TestCodeSource_UnsupportedLanguage` — `languages: ["typescript"]` → warning logged, no error
- `TestCreateCodeSource` — manager creates CodeSource from config map
- `TestValidateCodeSourceConfig` — valid config passes, missing path/languages fails

**Checkpoint**: `go test ./source/...` passes. Code source integrates with existing pipeline. Config validation catches missing required fields. Coverage ≥70%.

---

## Phase 3: Manifest Command (CLI)

**Purpose**: Add `dewey manifest` CLI command that generates `.dewey/manifest.md` using the same chunker infrastructure.

### Manifest Command Implementation

- [x] T008 [US3] Add `newManifestCmd()` to `cli.go` — Cobra command registered on root. Walks `.go` files in vault path (CWD or `--vault` flag), runs Go chunker, collects blocks by Kind, generates `.dewey/manifest.md` with sections: CLI Commands (table), MCP Tools (table), Exported Packages (list). Creates `.dewey/` directory if needed. Omits empty sections. Includes generation timestamp. Per contracts/manifest-command.md.

### Manifest Command Tests

- [x] T009 [US3] Add `TestManifestCmd_*` tests to `cli_test.go` — Tests using `t.TempDir()` with sample `.go` files containing Cobra commands and MCP tool registrations.

Required test cases:
- `TestManifestCmd_GeneratesManifest` — directory with Go files → `.dewey/manifest.md` created with expected sections
- `TestManifestCmd_IncludesCobraCommands` — file with `cobra.Command` → CLI Commands table in manifest
- `TestManifestCmd_IncludesMCPTools` — file with `mcp.AddTool` → MCP Tools table in manifest
- `TestManifestCmd_OmitsEmptySections` — no commands found → CLI Commands section omitted
- `TestManifestCmd_Idempotent` — running twice produces identical output
- `TestManifestCmd_CreatesDirectory` — `.dewey/` doesn't exist → created automatically

**Checkpoint**: `go test ./...` passes including manifest tests. `dewey manifest` generates valid markdown. Coverage ≥70%.

---

## Phase 4: Integration & Documentation

**Purpose**: End-to-end validation and documentation updates.

### Integration Test

- [x] T010 [US1] [US2] Add end-to-end test: code source → index → search. Create a test that configures a `type: code` source pointing to a `t.TempDir()` with Go files, runs the indexing pipeline, and verifies blocks are stored and searchable. Validates SC-001 (CLI command discovery) and SC-002 (API discovery).

### Documentation

- [x] T011 [P] Update `AGENTS.md` — Add `chunker/` to Architecture section package listing. Add `type: code` to source type documentation. Add `dewey manifest` to CLI commands. Update Active Technologies with `go/parser`, `go/ast`, `go/token`, `go/format` (stdlib). Note the new package in Project Structure.

**Checkpoint**: All tests pass. Documentation reflects new capabilities.

---

## Phase 5: Verification

**Purpose**: Final CI parity gate before declaring implementation complete.

- [x] T012 Run CI parity gate — Execute the exact commands from `.github/workflows/ci.yml`: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`. All must pass. Verify no regressions in existing tests (SC-005). Check Gaze quality thresholds if available.

---

## Dependencies & Execution Order

### Phase Dependencies

```text
Phase 1 (chunker/) → Phase 2 (source/code.go) → Phase 3 (manifest command)
                                                → Phase 4 (integration + docs)
                                                                              → Phase 5 (verification)
```

- **Phase 1**: No dependencies — can start immediately
- **Phase 2**: Depends on Phase 1 (imports `chunker/` package)
- **Phase 3**: Depends on Phase 1 (imports `chunker/` package), independent of Phase 2
- **Phase 4**: T010 depends on Phase 2; T011 depends on all prior phases for accuracy
- **Phase 5**: Depends on all prior phases

### Within-Phase Dependencies

- **Phase 1**: T001 → T002 (GoChunker implements Chunker interface). T003 [P] can be written alongside T002 (same package, tests the implementation).
- **Phase 2**: T004 depends on T001+T002. T005 depends on T004. T006 [P] is independent (different file, validation only). T007 depends on T004+T005+T006.
- **Phase 3**: T008 → T009 (tests depend on implementation).
- **Phase 4**: T010 depends on Phase 2. T011 [P] is independent (documentation only).

### Parallel Opportunities

```text
# Phase 1 — after T001 completes:
T002 and T003 can be developed concurrently (implementation + tests)

# Phase 2 — after T004 completes:
T005 and T006 can run in parallel (different files)

# Phase 3 and Phase 2 can overlap:
T008 depends only on Phase 1, not Phase 2

# Phase 4:
T010 and T011 can run in parallel (different concerns)
```

### User Story Traceability

| Story | Tasks | Acceptance Scenarios |
|-------|-------|---------------------|
| US1 — Discover CLI Commands | T001, T002, T003, T004, T005, T006, T007, T010 | AS-1 (Cobra indexed), AS-2 (semantic search), AS-3 (language filter) |
| US2 — Search Exported APIs | T002, T003, T004, T007, T010 | AS-1 (func/type blocks), AS-2 (package docs), AS-3 (multi-line doc comments) |
| US3 — Generate Manifest | T008, T009 | AS-1 (CLI Commands section), AS-2 (MCP Tools section), AS-3 (Exported Packages), AS-4 (indexable by disk source) |
| US4 — Pluggable Language Support | T001, T006 | AS-1 (unsupported language warning), AS-2 (no pipeline changes for new language) |

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- The Go chunker (T002) is the most complex task — 7 block kinds with AST walking. Decompose into per-kind helper functions to keep CRAP scores < 30.
- All tests use standard library `testing` package only (no testify). Use `t.Errorf`/`t.Fatalf` directly.
- Test files use `t.TempDir()` for filesystem fixtures and Go source string literals for chunker input.
- Commit after each task or logical group. Mark `- [x]` immediately upon completion.
<!-- spec-review: passed -->
