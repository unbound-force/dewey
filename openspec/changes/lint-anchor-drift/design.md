## Context

`Lint.Lint` runs a fixed sequence of checks, counts findings by type,
and auto-fixes only mechanical issues (embedding gaps). Learnings
store `as_of: <repo>@<sha>` and `evidence: [...]` in properties
(#146). Locators use `<repo>:<path>`.

## Goals / Non-Goals

### Goals
- Detect cited-path drift and missing anchors deterministically.
- Let `fix` mark drifted learnings `stale-unverified`.

### Non-Goals
- Resolving `<repo>#<n>` (network).
- Multi-repository anchors (#106).
- Semantic re-verification of the claim itself.
- Removing the 30-day check.

## Decisions

### D1 Scope of a checkable anchor

A learning is checked when its `as_of` repo is the vault's repository
(the base name of `git rev-parse --show-toplevel`, the same name
`as_of` is written with), it has at least one anchor, and its
validity is not `superseded` or `historical`. An evidence entry is an
anchor when it has the form `<repo>:<path>` and `<repo>` equals the
`as_of` repo. A trailing `:<line>` or `:<line>-<line>` suffix is
stripped, and duplicate paths are checked once.

A learning with no anchor is not checked even when its pin is
unknown: an `as_of` naming a commit that cites only `<repo>#<n>` or
other repositories gives this repository nothing to compare, and
reporting it would flag every learning pinned elsewhere.

Evidence over `store_learning`'s limits (50 entries, 512 characters
each) excludes the learning: re-ingested properties bypass the API
validation, so the limits are re-checked where evidence is consumed.

### D2 Git checker interface

```go
type anchorChecker interface {
    Head(ctx context.Context) (string, error)
    Repo(ctx context.Context) (string, error)
    CommitExists(ctx context.Context, sha string) (bool, error)
    PathExists(ctx context.Context, sha, path string) (bool, error)
    PathChanged(ctx context.Context, fromSHA, toSHA, path string) (bool, error)
}
```

The production checker runs `git -C <vault>` with a 5-second timeout
per call (`anchorGitTimeout`): `rev-parse --verify HEAD`,
`rev-parse --show-toplevel`, `cat-file -e <sha>^{commit}`,
`cat-file -e <sha>:<path>`, and `diff --quiet --no-ext-diff
--no-textconv <from> <to> -- <path>` (exit 1 means changed; external
diff drivers and textconv filters come from repository config and are
never run). Every call derives from the lint request's context, and
the whole pass is bounded by `anchorCheckBudget` (60 seconds).

The outcome is reported in the result as `anchor_checks`:
`skipped` when there is no vault path or git finds no repository or
commit, `failed` (with the error) when git cannot answer, the request
is cancelled, or the budget runs out, and `checked` otherwise. A
skipped or failed pass contributes no findings.

### D3 Findings

One finding per learning and anchor. `anchor_missing` when the pin
commit is unknown (one finding for the learning) or the path is absent
at the pin; `anchor_drift` when the path changed between pin and
`HEAD`. Severity `warning`. Descriptions name the anchor and the
short commit range.

### D4 Fix

With `fix: true`, every learning with at least one `anchor_drift`
finding gets `validity: stale-unverified` in its properties and in
its markdown backup's frontmatter (when present). Tier and content
are untouched. The result reports `fixed.anchor_drift` (learnings
marked in the store).

The store is written first; a learning counts as fixed once that
succeeds. A backup failure afterward is reported in
`anchor_fix_warnings` rather than dropping the count, the same
contract #146's supersession flow reports as `supersedes_warning`.
Both flows write the backup through one helper,
`updateLearningBackup`, and decode properties through
`decodeLearningProperties`.

### D5 Fail closed on untrusted input

`as_of` and page names can arrive through re-ingested frontmatter
(the trust boundary #156 hardened). A non-hexadecimal pin is not a
pin, so it never reaches git as a revision. `fix` validates the
identity with `ValidLearningIdentity` and decodes properties before
writing anything: an unsafe identity or unreadable properties is an
error, and the store and backup stay as they were. A literal JSON
`null` decodes to an empty map.

## Risks / Trade-offs

- **One subprocess per anchor.** Acceptable at current vault sizes
  and bounded by the pass budget; a batched `git diff --name-only`
  per distinct pin can come later.
- **Rename shows as drift.** A moved file reports drift at its old
  path, which correctly prompts re-verification.
