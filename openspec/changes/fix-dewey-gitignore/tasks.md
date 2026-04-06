## 1. Update .gitignore Append Logic

- [x] 1.1 In `cli.go` `newInitCmd()` (~line 269-283): replace the `.dewey/` pattern with granular runtime artifact patterns: `.dewey/graph.db`, `.dewey/graph.db-shm`, `.dewey/graph.db-wal`, `.dewey/dewey.log`, `.dewey/.dewey.lock`
- [x] 1.2 Update the `strings.Contains` check (~line 273) to detect both old (`.dewey/`) and new (`.dewey/graph.db`) patterns to prevent duplicate entries on re-init
- [x] 1.3 When the old `.dewey/` pattern is detected, log an informational message: `logger.Info("existing .dewey/ gitignore pattern found — update manually to track sources.yaml")`

## 2. Update Tests

- [x] 2.1 Update `TestInitCmd_GitignoreAppend` in `cli_test.go` to verify the new granular patterns are written instead of `.dewey/`
- [x] 2.2 Update `TestInitCmd_GitignoreAlreadyPresent` in `cli_test.go` to verify that when `.dewey/graph.db` is already in `.gitignore`, no duplicates are added
- [x] 2.3 Add `TestInitCmd_GitignoreLegacyPattern` in `cli_test.go` to verify that when `.dewey/` already exists, the log message is emitted and no modification is made

## 3. Verification

- [x] 3.1 Run `go build ./...` and `go vet ./...`
- [x] 3.2 Run `go test -race -count=1 ./...` — all tests pass
- [x] 3.3 Verify constitution alignment: no new dependencies (Composability), configuration trackable improves auditability (Observable Quality)
