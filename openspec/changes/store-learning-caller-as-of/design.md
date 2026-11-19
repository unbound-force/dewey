# Design: Caller-Supplied `as_of` on `store_learning`

## Context

See proposal. Constitution alignment: all four principles PASS.

## Goals / Non-Goals

### Goals

- A caller can pin a learning to the source and commit it observed.
- The git-HEAD stamp stays the default.
- Malformed pins never reach the store.

### Non-Goals

- Resolving or verifying that the commit exists (that needs the
  network for external repositories; `verify_candidates` and lint
  already check vault-repository pins locally).
- Anchor checks for `owner/repo@sha` pins against the vault. A pin
  naming another repository is skipped, as today.
- Changing `as_of` on `supersede_learning` or compiled articles.

## Decisions

### D1 Accept `owner/repo@<sha>` and `repo@<sha>`

`^(?:[A-Za-z0-9_.-]+/)?[A-Za-z0-9_.-]+@[0-9a-fA-F]{7,64}$`. The
`owner/repo` form is what callers pinning external sources need; the
bare form matches `productionAsOfResolver`'s output so a caller can pin
a vault learning to a specific earlier commit and stay eligible for
anchor checks. 7-64 hex covers abbreviated, SHA-1 and SHA-256 object
names. The repository alphabet is GitHub's; it also excludes
whitespace, `:` and URL shapes.

### D2 Validate with the other temporal inputs, before any write

The check sits in `prepareTemporal` next to `validateValidity`, so an
invalid `as_of` returns an error result and writes neither the page nor
the file (covered by the existing "writes nothing" test).

### D3 Caller value wins; resolver only as fallback

`prepareTemporal` stores the trimmed caller value and calls
`resolveAsOf` only when it is empty, so a caller pin costs no git
subprocess.

## Risks / Trade-offs

- A caller can pin a commit that does not exist. Accepted: the pin is
  a claim with the same trust as `evidence`, and learnings are always
  `draft` tier.
