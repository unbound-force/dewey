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

A learning is checked when it has `as_of` and its validity is not
`superseded` or `historical`. An evidence entry is an anchor when it
has the form `<repo>:<path>` and `<repo>` equals the `as_of` repo. A
trailing `:<line>` or `:<line>-<line>` suffix is stripped.

### D2 Git checker interface

```go
type anchorChecker interface {
    Head() (string, error)
    CommitExists(sha string) (bool, error)
    PathExists(sha, path string) (bool, error)
    PathChanged(fromSHA, toSHA, path string) (bool, error)
}
```

The production checker runs `git -C <vault>` with a 5-second timeout
per call: `rev-parse HEAD`, `cat-file -e <sha>^{commit}`,
`cat-file -e <sha>:<path>`, and `diff --quiet <from> <to> -- <path>`
(exit 1 means changed). A missing `HEAD` (not a git repository)
skips the anchor checks with one log line.

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
marked).

## Risks / Trade-offs

- **One subprocess per anchor.** Acceptable at current vault sizes;
  a batched `git diff --name-only` per distinct pin can come later.
- **Rename shows as drift.** A moved file reports drift at its old
  path, which correctly prompts re-verification.
