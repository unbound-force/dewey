## 1. Anchor checks

- [x] 1.1 `tools/lint_anchors.go`: `parseAnchor`, `anchorChecker`,
  production git checker, `checkAnchors`
- [x] 1.2 `tools/lint.go`: wire the check, counters, summary keys

## 2. Fix

- [x] 2.1 `tools/lint_anchors.go`: `fixAnchorDrift` (store and
  frontmatter, tier untouched)
- [x] 2.2 `tools/lint.go`: `fixed.anchor_drift`
- [x] 2.3 fail closed: hex-only pins, `ValidLearningIdentity`,
  unreadable properties (#156 council findings)

## 3. Tests

- [x] 3.1 `tools/lint_anchors_test.go`: drift, unchanged, missing
  path, unknown pin, skipped locators and validities, fix, checker
  errors, non-repository vault

## 4. Docs and verification

- [x] 4.1 README lint section
- [x] 4.2 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [ ] 4.3 `/uf.review-council` (OpenCode), draft PR

## 5. Review feedback (PR #159)

- [x] 5.1 `anchor_checks` outcome; context, per-pass budget, and
  evidence limits re-checked at consumption; `--no-ext-diff
  --no-textconv`; `Head` on the anchor timeout
- [x] 5.2 learnings pinned to another repository are not checked;
  spec and D1 state the eligibility rule
- [x] 5.3 fix counts store writes and reports backup failures in
  `anchor_fix_warnings`; shared `updateLearningBackup` and
  `decodeLearningProperties` with the #146 flow
- [x] 5.4 tests: multi-anchor dedup, two-path unknown pin,
  anchor_missing-only fix negative, deleted-after-pin, corrupt
  properties, other-repository pin, post-failure state
- [x] 5.5 README MCP table and AGENTS.md; website#398 (docs),
  website#399 (blog)
