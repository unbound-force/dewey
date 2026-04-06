## Context

`dewey init` appends `.dewey/` to `.gitignore` at `cli.go:269-283`. This blanket pattern ignores all files in `.dewey/` including `sources.yaml` and `config.yaml` which should be team-shareable.

## Goals / Non-Goals

### Goals
- Replace `.dewey/` with granular patterns ignoring only runtime artifacts
- Existing repos with `.dewey/` continue to work (no forced migration)
- New repos get the correct granular pattern from `dewey init`

### Non-Goals
- Auto-migrating existing `.gitignore` files (too risky, opt-in only)
- Changing the `.dewey/` directory structure
- Modifying `uf init` scaffold behavior (separate repo)

## Decisions

**D1: Granular ignore list.** Replace `.dewey/` with:
```
.dewey/graph.db
.dewey/graph.db-shm
.dewey/graph.db-wal
.dewey/dewey.log
.dewey/.dewey.lock
```
This matches the exact list from issue #23. The `cache/` entry is omitted since no cache directory currently exists.

**D2: Check for existing `.dewey` patterns before appending.** The current code checks `!strings.Contains(string(content), ".dewey/")`. Update to also check for `".dewey/graph.db"` to avoid duplicate entries if `dewey init` is run twice after the fix.

**D3: No automatic migration.** If `.dewey/` already exists in `.gitignore`, do not modify it. Log an informational message suggesting the developer update manually.

## Risks / Trade-offs

**Risk: Accidental commit of graph.db.** If a developer removes `.dewey/` from `.gitignore` without adding the granular patterns, they could commit the 24+ MB database. Mitigation: the granular patterns are always added by `dewey init`, and the patterns are specific enough that manual removal would be intentional.

**Trade-off: No cache/ pattern.** Issue #23 suggests `.dewey/cache/` but no such directory exists today. Adding it preemptively would be speculative. Can be added when a cache feature is implemented.
