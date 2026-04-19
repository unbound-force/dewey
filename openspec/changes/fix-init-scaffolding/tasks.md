## 1. Restructure Init Flow

- [x] 1.1 In `cli.go` `newInitCmd()`: replace the early return at "already initialized" with a conditional block that only skips config/sources/gitignore creation. Slash command scaffolding must run unconditionally after.

## 2. Tests

- [x] 2.1 Add `TestInitCmd_ReInitScaffoldsNewCommands`: init once (creates everything), delete one slash command, init again, verify the deleted command is re-scaffolded
- [x] 2.2 Verify existing `TestInitCmd_Idempotent` still passes (re-init doesn't error)

## 3. Verification

- [x] 3.1 Run `go build ./...` and `go vet ./...`
- [x] 3.2 Run `go test -race -count=1 ./...` — all tests pass
<!-- spec-review: passed -->
