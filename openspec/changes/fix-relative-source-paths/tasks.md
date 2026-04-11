## 1. Fix Path Resolution

- [x] 1.1 In `source/manager.go` `createDiskSource()` (~line 84): replace `if path == "." { path = basePath }` with path resolution that handles both `.` and other relative paths via `filepath.Join(basePath, path)` when `!filepath.IsAbs(path)`
- [x] 1.2 In `source/manager.go` `createCodeSource()` (~line 114): apply the same fix
- [x] 1.3 Add DEBUG log: `logger.Debug("resolved source path", "source", cfg.ID, "raw", rawPath, "resolved", path)` in both functions

## 2. Tests

- [x] 2.1 Add `TestCreateDiskSource_RelativePath` in `source/manager_test.go`: verify `path: "../sibling"` resolves to `basePath/../sibling`
- [x] 2.2 Add `TestCreateDiskSource_AbsolutePath` in `source/manager_test.go`: verify absolute path passes through unchanged
- [x] 2.3 Add `TestCreateCodeSource_RelativePath` in `source/manager_test.go`: verify same behavior for code sources

## 3. Verification

- [x] 3.1 Run `go build ./...` and `go vet ./...`
- [x] 3.2 Run `go test -race -count=1 ./...` — all tests pass
