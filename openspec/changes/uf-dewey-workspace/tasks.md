## 1. Introduce Workspace Constant

- [x] 1.1 Add `const deweyWorkspaceDir = ".uf/dewey"` to `main.go` (near the top, after imports)
- [x] 1.2 Replace all hardcoded `".dewey"` path segments in `main.go` with `deweyWorkspaceDir` (5 references)
- [x] 1.3 Replace all hardcoded `".dewey"` path segments in `cli.go` with `deweyWorkspaceDir` (32 references — use the constant directly or `filepath.Join(vp, deweyWorkspaceDir)`)

## 2. Rename Lock File

- [x] 2.1 In `store/store.go`: change lock file name from `.dewey.lock` to `dewey.lock` (~3 references)
- [x] 2.2 In `cli.go`: update `detectLockHolder()` and doctor references to use `dewey.lock` instead of `.dewey.lock`

## 3. Update .gitignore Patterns

- [x] 3.1 In `cli.go` `newInitCmd()`: update the granular gitignore patterns from `.dewey/*` to `.uf/dewey/*`
- [x] 3.2 In `cli.go` `newInitCmd()`: update the legacy pattern detection from `.dewey/` to check for both old (`.dewey/`) and new (`.uf/dewey/`) patterns

## 4. Update Non-Main Package References

- [x] 4.1 In `source/config.go`: update any `.dewey` path references (4 references)
- [x] 4.2 In `vault/vault.go` and `vault/vault_store.go`: update any `.dewey` path references (2 references)
- [x] 4.3 In `tools/learning.go`: update any `.dewey` path references (1 reference)

## 5. Update Tests

- [x] 5.1 Update `cli_test.go`: replace all `.dewey` fixture paths with `.uf/dewey` (66 references)
- [x] 5.2 Update `main_test.go`: replace `.dewey` fixture paths (5 references)
- [x] 5.3 Update `integration_test.go`: replace `.dewey` fixture paths (16 references)
- [x] 5.4 Update `vault/vault_store_test.go`: replace `.dewey` fixture path (1 reference)

## 6. Update Documentation

- [x] 6.1 Update `README.md`: replace all `.dewey/` references with `.uf/dewey/` (~15 references)
- [x] 6.2 Update `AGENTS.md`: replace all `.dewey/` references with `.uf/dewey/`
- [x] 6.3 Update `.specify/memory/constitution.md`: replace `.dewey/` references with `.uf/dewey/` (2 references)

## 7. Verification

- [x] 7.1 Run `go build ./...` and `go vet ./...`
- [x] 7.2 Run `go test -race -count=1 ./...` — all tests pass
- [x] 7.3 Verify no remaining `.dewey` references in Go source: `rg '\.dewey[^W]' --type go` should return 0 results (excluding `deweyWorkspaceDir` definition)
- [x] 7.4 Verify constitution alignment: workspace path change is transparent to MCP tools (Autonomous Collaboration), no new dependencies (Composability)
