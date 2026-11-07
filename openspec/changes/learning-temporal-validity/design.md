## Context

`StoreLearning` writes a page (properties JSON, tier `draft`) and a
markdown backup with YAML frontmatter; `reIngestLearnings` restores
missing pages from those files at startup. Properties are consumed as
`map[string]any` everywhere (`vault/vault_store.go`, `tools/lint.go`,
`cli.go`), so list-valued properties are safe. `StoreLearning` and
`reIngestLearnings` are already long; the Gaze CRAPload gate has
three functions of headroom (45/48 on main).

## Goals / Non-Goals

### Goals
- Pin learnings to the code state they were stored against.
- Carry evidence locators and an explicit validity status.
- Record supersession two-way without deleting history.
- Round-trip every field through markdown backup and re-ingestion.

### Non-Goals
- Verifying evidence or detecting drift (#147).
- Ranking or filtering by validity (#148).
- Multi-repo pins (#106); per-locator pins can come later.
- Changing tiers or tier semantics.

## Decisions

### D1 Properties, not columns

All fields live in the `properties` JSON column. No migration; read
tools surface them through the existing properties passthrough.

### D2 `as_of` format and resolution

`as_of` is `<repo>@<sha>`: `repo` is the basename of
`git rev-parse --show-toplevel` run in the vault root, `sha` is the
full `git rev-parse HEAD`. Resolved by an injected function (the
`resolveAuthor` pattern) using `exec.CommandContext` with a 2-second
timeout. Any failure omits `as_of` and logs at debug; storing never
fails because git is unavailable. The pin is HEAD, not the working
tree: uncommitted changes are not represented, documented as such.
The resolver runs under `context.Background()` rather than a request
context: `as_of` is artifact provenance that must not depend on the
requester's connection lifetime, the 2-second timeout bounds the
subprocess cost, and failure only omits the pin. Threading a context
would widen the `AsOfResolver` signature across every resolver and
test double; revisit with multi-repo pinning (#106).

### D3 Validation

- `validity` must be one of `current`, `stale-unverified`,
  `superseded`, `historical`.
- `evidence`: at most 50 entries, each non-empty after trimming and at
  most 512 characters; entries are trimmed and exact duplicates
  dropped, order preserved.
- `supersedes`: an identity (`<tag>-<ts>-<author>`) or page name
  (`learning/<identity>`); must resolve to an existing page with
  source `learning`; must not already carry `superseded_by` (one
  successor per learning — chains, not forks).
- Validation runs before any write, so an invalid call writes nothing.

### D4 Supersession link

After the new learning's page is inserted, the target's properties
gain `superseded_by: <new identity>` and `validity: superseded`
(store `UpdatePage`). Its markdown backup, when present, has those
two frontmatter keys set or replaced in place — body untouched. A
failure to update the target after the new learning is stored is
logged and reported in the response (`supersedes_warning`); the new
learning is not rolled back. The target's tier is unchanged.

### D5 Frontmatter encoding

New keys are written after `tier:`. Strings use double-quoted YAML
(`%q`, which is YAML-compatible for these values); `evidence` is a
block sequence of quoted strings. `learningFrontmatter` gains
`AsOf`, `Evidence []string`, `Validity`, `Supersedes`,
`SupersededBy`.

### D6 CRAP budget

Logic goes into small pure helpers in `tools/learning_temporal.go`
(validation, property building, frontmatter rendering and in-place
key update) and one re-ingestion helper in `main.go`, each fully
tested, so `StoreLearning` and `reIngestLearnings` gain only a few
straight-line calls.

## Risks / Trade-offs

- **HEAD-only pin.** A learning about uncommitted work pins the
  previous commit. Acceptable: drift detection compares cited paths
  between the pin and later HEADs; the uncommitted window is small.
- **Re-ingested `validity: superseded` is trusted.** Unlike tier,
  validity is not a trust claim, so re-ingestion keeps it as written.
- **Concurrent supersession** of the same target is not serialized;
  the second writer overwrites `superseded_by`, silently forking the
  chain (no `supersedes_warning`). The store lock serializes
  processes; the unserialized window is concurrent in-process tool
  calls under streamable HTTP sharing one store handle. Tracked in
  #166 — candidate fix is a compare-and-swap update or per-target
  mutex covering both write legs (store and markdown file).
