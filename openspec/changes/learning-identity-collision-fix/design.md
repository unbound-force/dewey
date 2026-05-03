## Context

The `store_learning` MCP tool generates learning file identities using `{tag}-{seq}` where `seq` comes from `store.NextLearningSequence()` — a `COUNT(*)` query against the local SQLite database. Each team member's Dewey instance has an independent database, so independent counters. Two people storing learnings with the same tag produce identical filenames (e.g., `authentication-1.md`), causing git merge conflicts or silent overwrites.

The file-backed learning system (spec 015) is designed for git-committed durability, making this a guaranteed collision on multi-person teams.

## Goals / Non-Goals

### Goals
- Eliminate learning filename collisions across team members using the same vault
- Embed author provenance directly in learning filenames and frontmatter
- Maintain backward compatibility with existing `{tag}-{seq}.md` files
- Remove the `NextLearningSequence` database dependency from identity generation

### Non-Goals
- User authentication or access control — author is informational, not a security boundary
- Multi-user conflict resolution at the Dewey application level — git handles merge
- Changing the `store_learning` MCP tool interface (input parameters remain unchanged)
- Adding author resolution to any tool other than `store_learning`

## Decisions

### D1: Identity format — `{tag}-{timestamp}-{author}`

The new identity format is `{tag}-{YYYYMMDDTHHMMSS}-{author}`, for example `authentication-20260502T143022-alice`.

**Rationale**: Timestamp provides temporal ordering and near-uniqueness. Author provides per-person namespacing. Together they make collisions require the same tag, same second, same author — effectively impossible in practice. The format is filesystem-safe (no colons, spaces, or special characters), sortable, and human-readable.

**Alternatives rejected**:
- UUID/ULID suffix: eliminates collisions but sacrifices human readability and temporal sorting
- Content hash: deterministic but opaque and not sortable
- MAX(seq) + filesystem scan: reduces but doesn't eliminate cross-machine collisions

### D2: Author resolution chain

Author is resolved via a three-tier fallback chain:

1. `DEWEY_AUTHOR` environment variable (highest priority — explicit override)
2. `git config user.name` via `git config --get user.name` subprocess (zero-config default for git repos)
3. `"anonymous"` literal (never fails)

**Rationale**: `DEWEY_AUTHOR` as highest priority lets CI, containers, and non-git environments set identity explicitly. `git config user.name` covers the common case (most Dewey vaults are git repos) with zero configuration. `"anonymous"` ensures Dewey never fails — aligns with Composability First (no mandatory dependencies).

The author is resolved once per `StoreLearning` call, not cached globally. This keeps the implementation simple and testable. The cost of running `git config --get user.name` is negligible (< 5ms) for the learning storage path which is not latency-sensitive.

### D3: Author normalization

Author strings are normalized using the same rules as tag normalization: lowercase, spaces replaced with hyphens, non-alphanumeric characters (except hyphens) stripped. This reuses the existing `normalizeTag` function directly (no rename — keeping the existing name avoids scope creep).

If normalization produces an empty string (e.g., CJK-only names like `"田中太郎"`, or all-special-character values like `"@#$%"`), the result MUST fall back to `"anonymous"`. Empty or whitespace-only `DEWEY_AUTHOR` values MUST be treated as unset and fall through to the next tier in the resolution chain. Author strings MUST be truncated to 64 characters after normalization to prevent unreasonably long filenames.

Examples:
- `"John Flowers"` -> `"john-flowers"`
- `"Alice O'Brien"` -> `"alice-obrien"`
- `"bob"` -> `"bob"`
- `"田中太郎"` -> `"anonymous"` (empty after normalization, falls back)
- `""` or `"   "` -> treated as unset, falls through to next tier

**Rationale**: Filesystem safety (no spaces, special chars) and consistency with existing tag normalization. The same regex (`[^a-z0-9-]`) applies. The empty-string fallback ensures the identity format always has a valid author segment.

### D4: Timestamp precision and sub-second collisions

Timestamps use second-level precision (`YYYYMMDDTHHMMSS`) in UTC. Sub-second collisions (same tag, same second, same author) are handled by creating the file with `os.OpenFile` using `O_CREATE|O_EXCL` flags (atomic create-or-fail). If the file already exists, append `-2`, `-3` suffix up to a maximum of 99 attempts. If 99 attempts are exhausted, return an error. The collision suffix produces identities like `{tag}-{timestamp}-{author}-2`.

**Rationale**: Second precision is sufficient for human-driven learning storage. `O_CREATE|O_EXCL` provides atomic file creation, eliminating the TOCTOU race condition that would exist with a stat-then-write approach. The 99-attempt cap prevents unbounded loops in pathological cases. Millisecond precision was considered but adds visual noise to filenames for negligible benefit.

### D5: Backward compatibility for existing files

The `reIngestLearnings` function MUST handle both old-format (`{tag}-{seq}.md`) and new-format (`{tag}-{YYYYMMDDTHHMMSS}-{author}.md`) files. Detection is based on the `identity` field in the YAML frontmatter, not the filename pattern — the frontmatter is the source of truth.

Old-format files without an `author` field are ingested normally with an empty author in properties. No migration of existing files is performed.

### D6: `NextLearningSequence` removal

The `store.NextLearningSequence()` method is deleted entirely. No replacement query is needed — identity generation is now a pure function of (tag, timestamp, author). This simplifies the `store.Store` interface and removes a database round-trip from the learning storage path.

Any code that calls `NextLearningSequence` (only `tools/learning.go:134`) is updated to use the new identity generation.

### D7: Author in frontmatter, not in MCP tool input

The author is resolved internally by Dewey, not passed as an MCP tool parameter. The `StoreLearningInput` struct is unchanged. This keeps the MCP interface simple — agents don't need to know or care about author identity.

The resolved author is stored in:
- The filename: `{tag}-{timestamp}-{author}.md`
- The frontmatter: `author: {author}`
- The `properties` JSON column in SQLite (key: `"author"`)
- The MCP response: added to the result JSON as `"author"` field

### D8: Tag extraction from new identity format

The `extractTagFromIdentity` function in `tools/compile.go` parses learning identities to extract the tag for compilation clustering. The current implementation assumes `{tag}-{seq}` format and finds the last hyphen followed by a number. This will silently misparse the new `{tag}-{timestamp}-{author}` format.

The fix: read the `tag` from the page's `properties` JSON column (which already contains `"tag": "..."`) as the primary source. Fall back to string parsing only when properties are unavailable. This approach is more robust than pattern-matching on the identity string and naturally handles both old and new formats.

**Rationale**: Properties JSON is the authoritative source for the tag — it was set by `StoreLearning` at write time. Parsing the identity string is fragile and requires knowledge of the format. Using properties eliminates format coupling.

### D9: `resolveAuthor` testability via injectable git resolver

The `resolveAuthor` function accepts a function parameter for the git subprocess call, enabling tests to inject a mock that returns a known value. The production code passes a function that calls `exec.CommandContext("git", "config", "--get", "user.name")` with a 2-second context timeout. Tests pass a function that returns a predetermined string.

The subprocess MUST use `exec.CommandContext` with a 2-second timeout. Raw output MUST be trimmed of whitespace and newlines before normalization.

**Rationale**: Direct `exec.Command` calls make the git-present path untestable in isolation (would depend on the test runner's git configuration). A function parameter provides injection without introducing a full interface — appropriate for a single function with one call site.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `git config --get user.name` subprocess fails (git not installed) | Low | None | Fallback chain: env var -> anonymous. Dewey never fails. Subprocess uses 2-second timeout via `exec.CommandContext`. |
| `git config --get user.name` subprocess hangs (credential helper, network mount) | Very Low | Low | 2-second context timeout ensures bounded execution. Falls through to `"anonymous"`. |
| Two learnings in same second with same tag and author | Very Low | Low | Atomic file creation with `O_CREATE\|O_EXCL` + collision suffix (`-2`, `-3`), capped at 99 attempts. |
| TOCTOU race on collision suffix (concurrent MCP calls) | Very Low | Low | `O_CREATE\|O_EXCL` provides atomic create-or-fail, eliminating the race. Pre-existing condition (current `os.WriteFile` is also not atomic). |
| Longer filenames than before | Certain | Negligible | Format is `{tag}-{14 char timestamp}-{author}.md` vs `{tag}-{1-3 digits}.md`. Author capped at 64 chars. Well within filesystem limits. |
| Existing tests assume `{tag}-{seq}` identity format | Certain | Medium | ~13 tests need updating across `learning_test.go`, `compile_test.go`, `lint_test.go`, `main_test.go`. |
| Author normalization produces empty string (CJK, all-special-chars) | Low | None | Empty-after-normalization falls back to `"anonymous"`. Explicitly tested. |
| `extractTagFromIdentity` breaks with new format | Certain | High | Mitigated by D8 — switch to reading tag from properties JSON. Old format still supported via fallback parsing. |
| Transition period: old Dewey instances encounter new-format files | Low | None | `reIngestLearnings` uses `fm.Identity` from frontmatter (a string), not filename parsing. Old instances read new files correctly. |
