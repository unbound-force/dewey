# Completion notes — serialize-supersession-writes (#166)

## Constitution alignment (task 5.3)

Assessed against the Unbound Force org constitution after implementation.

### I. Composability First — PASS

The change is internal to Dewey. The store claim uses SQLite JSON1
(`json_extract`) already in use via `GetPagesWithProperty`; no new
dependencies, no coupling to another Unbound Force tool, and `dewey
serve` on any vault is unchanged. `go.mod` and `go.sum` are untouched.

### II. Autonomous Collaboration — PASS

The conflict outcome rides the existing `store_learning` MCP result
field `supersedes_warning`. No new tool, no new input field, no runtime
coupling. The only schema-surface edits are two description strings
(`server.go` tool `Description`, `types/tools.go` `Supersedes`
jsonschema description) — no shape change.

### III. Observable Quality — PASS

A concurrent losing writer's result now carries `supersedes_warning`
naming the winning successor, where the prior behavior forked the chain
silently. The store row and the markdown backup are kept consistent by
the guarded store claim and the guarded backup replace, and a divergence
the guards detect (a backup already naming a different successor) is
surfaced as a warning rather than masked. A known, documented residual
is the store-first crash window (design Non-Goals/Risks), scoped out of
SSW-FR-004.

### IV. Testability — PASS

Every new piece is tested in isolation with no external services:
`ClaimSupersession` against in-memory SQLite (`store/store_test.go`);
the file guard against `t.TempDir()` backups; a sleep-free concurrent
race regression with 8 goroutines and a closed-channel start barrier
(`TestStoreLearning_ConcurrentSupersession`); a re-ingestion seam that
chains the real tools writer into `reIngestLearnings`
(`TestReIngestLearnings_SupersededByChainSurvivesGraphDBLoss`); and the
sequential regression guard. All pass under `-race -count=1`.

## Verification summary

- `go build ./...` — pass
- `go vet ./...` — pass
- `go test -race -count=1 ./...` — pass (all packages)
- `golangci-lint run ./...` — only 7 pre-existing staticcheck findings
  in unmodified files (`tools/learning.go`, `tools/learning_test.go`,
  `artifacts/`), present at the `aa27a30` baseline
- `openspec validate serialize-supersession-writes` — valid; no ERROR
  (only the repo-wide `originating_issue` warning and the informational
  archive note)

## Documentation

- README `#### Temporal Validity` — one paragraph on concurrent
  supersession.
- AGENTS.md — Store Learning API `supersedes` bullet extended; Recent
  Changes entry added.
- `server.go` / `types/tools.go` — tool/schema descriptions extended.
- Website sync: website#380 (docs) and website#392 (blog) commented;
  see task 5.2.

## Review advisories carried forward (non-blocking)

The iteration-3 spec review returned APPROVE WITH ADVISORIES (26
advisories, 0 blocking). Behavioural advisories were applied during
implementation:

- Race-test assertion corrected to the store-level invariant (accepts
  either a warned write-leg loser or a validation-error loser).
- `frontmatterScalar` fails closed on block-form values.
- README anchor corrected to the actual heading hierarchy.
- Blog determination executed and recorded.
- The `lint` capability-contract delta (lint's file leg now refuses to
  overwrite a backup already naming a successor) is implemented and
  tested; declaring it as a formal `lint-anchor-drift` MODIFIED delta is
  left for a follow-up spec-sync, since the review phase had closed.

## Post-approval CI fix (task 6.4)

- Extracted `decodeScalarValue` from `frontmatterScalar`
  (`tools/learning_temporal.go`) to clear PR #188's CI CRAPload ratchet
  failure (49 > 48); value semantics preserved byte-for-byte, both
  functions at 100% line coverage, local gate 47/48 PASS.
