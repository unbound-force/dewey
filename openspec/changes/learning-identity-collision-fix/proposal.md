## Why

The current learning file naming scheme uses `{tag}-{seq}.md` where the sequence number is derived from a `COUNT(*)` query against the local SQLite database (`store.NextLearningSequence`). Each team member runs their own Dewey instance with their own `graph.db`, so their counters are independent. When two people store learnings with the same tag, they produce identical filenames (e.g., both generate `authentication-1.md`), causing git merge conflicts or silent overwrites when the `.uf/dewey/learnings/` directory is committed.

This is not a theoretical risk — it is guaranteed on any multi-person team that commits learnings to version control, which is the intended usage pattern for file-backed learnings (spec 015).

A secondary issue: the `COUNT(*)`-based sequence (rather than `MAX(seq)`) means deleting a learning from the database causes the counter to regress, potentially generating a filename that collides with an existing file on disk. `os.WriteFile` would silently overwrite the existing file.

## What Changes

Replace the `{tag}-{seq}` naming scheme with `{tag}-{timestamp}-{author}`:

- **timestamp**: UTC in `YYYYMMDDTHHMMSS` format (compact, filesystem-safe, sortable)
- **author**: normalized identifier resolved from `git config user.name` (primary), `DEWEY_AUTHOR` env var (override), or `"anonymous"` (fallback)

Example: `authentication-20260502T143022-alice` instead of `authentication-3`.

Remove `NextLearningSequence` entirely — uniqueness comes from timestamp + author, not a database counter.

## Capabilities

### New Capabilities
- `author resolution`: Dewey resolves an author identifier via a fallback chain: `DEWEY_AUTHOR` env var (highest priority) -> `git config user.name` (zero-config default) -> `"anonymous"` (never fails). The resolved author is included in learning file frontmatter and filenames.

### Modified Capabilities
- `store_learning`: Identity format changes from `{tag}-{seq}` to `{tag}-{timestamp}-{author}`. File naming, frontmatter, page names, and doc IDs all use the new format. The MCP tool response structure is unchanged — only the identity string value changes.
- `reIngestLearnings`: Updated to parse the `author` frontmatter field and handle both old-format (`{tag}-{seq}`) and new-format filenames for backward compatibility with existing learning files.

### Removed Capabilities
- `NextLearningSequence`: The COUNT-based sequence query on `store.Store` is removed. Timestamp + author provides globally unique identities without a database counter.

## Impact

**Files affected:**
- `store/store.go` — Remove `NextLearningSequence` method
- `tools/learning.go` — New identity format, author resolution function, updated frontmatter with `author:` field
- `tools/compile.go` — Update `extractTagFromIdentity` to handle both old-format (`{tag}-{seq}`) and new-format (`{tag}-{timestamp}-{author}`) identities
- `tools/compile_test.go` — Update `storeLearningDirect` helper and `TestExtractTagFromIdentity` for new format
- `tools/lint_test.go` — Update `storeLearningForLint` helper for new identity format
- `main.go` — Updated `learningFrontmatter` struct and `reIngestLearnings` to handle author field and both old/new filename formats
- `types/tools.go` — Update `Incremental` field jsonschema description example to new identity format
- `store/migrate_test.go` — Remove `NextLearningSequence` tests
- `tools/learning_test.go` — Update all identity format assertions (~13 tests), add author resolution tests
- `main_test.go` — Update re-ingestion tests for new frontmatter, add backward-compat tests for old-format files

**Backward compatibility:**
- Existing `{tag}-{seq}.md` files remain valid and are parsed correctly by `reIngestLearnings`
- No SQL schema changes — author is stored in the existing `properties` JSON column
- The `store_learning` MCP tool response structure is unchanged (new `author` field added)
- `extractTagFromIdentity` handles both old and new identity formats for compilation
- During transition, old Dewey instances can read new-format files via frontmatter `identity` field

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

Author resolution gracefully degrades: `DEWEY_AUTHOR` env var -> `git config user.name` -> `"anonymous"`. Dewey remains independently usable without git, without environment configuration, and without any other Unbound Force tool. No new mandatory dependencies are introduced.

### II. Autonomous Collaboration

**Assessment**: PASS

The `store_learning` MCP tool interface is unchanged — agents call it the same way and receive the same structured response format. Author resolution is internal to Dewey and requires no agent coordination or runtime coupling. The change eliminates a collision vector that could disrupt artifact-based collaboration when multiple agents or users store learnings concurrently.

### III. Observable Quality

**Assessment**: PASS

The new identity format improves provenance — each learning carries its author and creation timestamp directly in the filename and frontmatter metadata. This makes the learning corpus auditable at rest: a human or agent can inspect the `.uf/dewey/learnings/` directory and immediately see who created each learning and when, without querying the database.

### IV. Testability

**Assessment**: PASS

Author resolution is a pure function (inputs: git output, env var, fallback -> output: normalized string) — trivially testable in isolation without external services. The removal of `NextLearningSequence` eliminates the database dependency from identity generation, making the naming logic easier to test. Tests inject a known author string without requiring git or environment variable setup.
