## 1. Module Declaration

- [x] 1.1 Update `go.mod` module line from `module github.com/unbound-force/dewey` to `module github.com/unbound-force/dewey/v3`

## 2. Import Path Rewrite

- [x] 2.1 Rewrite all internal import paths in `.go` files from `github.com/unbound-force/dewey/` to `github.com/unbound-force/dewey/v3/` (64 files, 181 import lines, 15 subpackages)
- [x] 2.2 Run `go mod tidy` to update `go.sum` checksums (the diff will be large since the module path change affects all checksums — this is expected)
- [x] 2.3 Verify no stale import references remain: grep `.go` files for `"github.com/unbound-force/dewey/` without `/v3` — zero matches expected (excluding `go.sum`, spec artifacts, and historical documentation)

## 3. Documentation Updates

- [x] 3.1 Update README.md `go install` command to `go install github.com/unbound-force/dewey/v3@latest`
- [x] 3.2 Update all module path references in AGENTS.md (line 8 module declaration and Active Technologies import paths) to use `github.com/unbound-force/dewey/v3`
- [x] 3.3 Create GitHub issue in `unbound-force/website` documenting the `go install` path change from `github.com/unbound-force/dewey@latest` to `github.com/unbound-force/dewey/v3@latest`

## 4. Verification

- [x] 4.1 Run `go build ./...` — must succeed with no import resolution errors
- [x] 4.2 Run `go vet ./...` — must pass clean
- [x] 4.3 Run `go test -race -count=1 ./...` — all tests must pass
- [x] 4.4 Review diff for unintended substitutions in string literals, comments, or test fixtures

## 5. Constitution Alignment Verification

- [x] 5.1 Confirm Composability First: `go.mod` declares `module github.com/unbound-force/dewey/v3` and `go build ./...` succeeds
- [x] 5.2 Confirm Testability: full test suite passes with rewritten imports (covered by task 4.3)

## 6. Post-Release Verification

- [ ] 6.1 After tagging the next v3.x release, run `go install github.com/unbound-force/dewey/v3@latest` from a clean environment and verify the installed binary reports the correct version
<!-- spec-review: passed -->
<!-- code-review: passed -->
