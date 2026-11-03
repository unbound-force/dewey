## 1. Anchor checks

- [x] 1.1 `tools/lint_anchors.go`: `parseAnchor`, `anchorChecker`,
  production git checker, `checkAnchors`
- [x] 1.2 `tools/lint.go`: wire the check, counters, summary keys

## 2. Fix

- [x] 2.1 `tools/lint_anchors.go`: `fixAnchorDrift` (store and
  frontmatter, tier untouched)
- [x] 2.2 `tools/lint.go`: `fixed.anchor_drift`

## 3. Tests

- [x] 3.1 `tools/lint_anchors_test.go`: drift, unchanged, missing
  path, unknown pin, skipped locators and validities, fix, checker
  errors, non-repository vault

## 4. Docs and verification

- [x] 4.1 README lint section
- [x] 4.2 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [ ] 4.3 `/uf.review-council` (OpenCode), draft PR
