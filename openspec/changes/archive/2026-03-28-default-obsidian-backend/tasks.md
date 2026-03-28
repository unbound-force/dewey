## 1. Change Default Backend

- [x] 1.1 Update `resolveBackendType()` fallback in `main.go:157`: return `"obsidian"` instead of `"logseq"`
- [x] 1.2 Update comment on `main.go:149`: "falling back to logseq" -> "falling back to obsidian"
- [x] 1.3 Update flag help text on `main.go:69` and `main.go:106`: "logseq (default)" -> "obsidian (default)"
- [x] 1.4 Update health check default in `server.go:351`: `"logseq"` -> `"obsidian"` (inverted the condition)

## 2. Update Tests

- [x] 2.1 Update `TestResolveBackendType_DefaultLogseq` in `main_test.go`: renamed to `TestResolveBackendType_DefaultObsidian`, changed expected value from `"logseq"` to `"obsidian"`
- [x] 2.2 Update the table test case `{"default logseq", "", "", "logseq"}` in `main_test.go:69`: changed to `{"default obsidian", "", "", "obsidian"}`
- [x] 2.3 No other assertions expect `"logseq"` as default (checked)

## 3. Verify

- [ ] 3.1 Run `go build ./...`
- [ ] 3.2 Run `go test -race -count=1 ./...`
