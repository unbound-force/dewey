## 1. Create the Slash Command

- [x] 1.1 Create `.opencode/command/dewey-store.md` with frontmatter, description, usage examples, and full instruction set covering all 3 modes (fully specified, suggested, extract)

## 2. Scaffold Slash Commands in `dewey init`

- [x] 2.1 In `cli.go` `newInitCmd()`: after creating `.uf/dewey/` directory and config files, scaffold all Dewey-specific slash commands into `.opencode/command/` if the `.opencode/` directory exists
- [x] 2.2 Embed the slash command content as Go string constants or use `embed` directive for the command files: `dewey-store.md`, `dewey-index.md`, `dewey-reindex.md`, `dewey-compile.md`, `dewey-lint.md`
- [x] 2.3 Only write command files that don't already exist (idempotent — don't overwrite user customizations)
- [x] 2.4 Log each scaffolded file at INFO level: `logger.Info("scaffolded slash command", "path", path)`

## 3. Tests

- [x] 3.1 Add `TestInitCmd_ScaffoldsSlashCommands` in `cli_test.go`: create temp dir with `.opencode/` directory, run init, verify all 5 command files exist
- [x] 3.2 Add `TestInitCmd_SkipsExistingSlashCommands` in `cli_test.go`: create temp dir with `.opencode/command/dewey-store.md` already present (custom content), run init, verify the file was NOT overwritten
- [x] 3.3 Add `TestInitCmd_NoOpenCodeDir` in `cli_test.go`: create temp dir WITHOUT `.opencode/`, run init, verify no slash commands are created (graceful skip)

## 4. Verification

- [x] 4.1 Run `go build ./...` and `go vet ./...`
- [x] 4.2 Run `go test -race -count=1 ./...` — all tests pass
<!-- spec-review: passed -->
