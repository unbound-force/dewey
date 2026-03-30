# Feature Specification: Unified Ignore Support

**Feature Branch**: `006-unified-ignore`
**Created**: 2026-03-30
**Status**: Draft
**Input**: User description: "Unified ignore support for vault and source walkers with .gitignore respect and sources.yaml ignore/recursive config"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Automatic .gitignore Respect (Priority: P1)

A developer runs `dewey serve` in a Hugo website project that has `node_modules/` in its `.gitignore`. Today, dewey indexes all 374 markdown files inside `node_modules/`, causing the MCP server startup to exceed the 30-second timeout and fail to connect. The developer expects dewey to automatically skip directories that are already excluded from version control — the same files git ignores should be invisible to dewey without any configuration.

**Why this priority**: This is the root cause of the timeout bug. Respecting `.gitignore` is zero-config and solves the problem for every project that already has a `.gitignore`.

**Independent Test**: Create a vault with a `.gitignore` containing `node_modules/`, place markdown files inside `node_modules/`, run `dewey serve`, and verify those files are not indexed.

**Acceptance Scenarios**:

1. **Given** a vault with a `.gitignore` containing `node_modules/`, **When** `dewey serve` starts, **Then** no files inside `node_modules/` appear in the index
2. **Given** a vault with no `.gitignore`, **When** `dewey serve` starts, **Then** all non-hidden directories are indexed (existing behavior preserved)
3. **Given** an external disk source (`disk-website`) pointing to a directory with its own `.gitignore`, **When** `dewey index` runs, **Then** the source respects that directory's `.gitignore`
4. **Given** a `.gitignore` with a negation pattern (`!important.md`), **When** dewey walks the directory, **Then** negation patterns are respected (files are not skipped)

---

### User Story 2 - Sources.yaml Ignore Configuration (Priority: P2)

A developer wants to exclude directories that git tracks but dewey should not index — for example, a `drafts/` folder with work-in-progress markdown that shouldn't appear in the knowledge graph. The developer adds an `ignore` list to the source's configuration in `sources.yaml`. These patterns are applied in addition to `.gitignore` patterns (union merge — both apply).

**Why this priority**: This provides explicit control beyond what `.gitignore` covers, letting developers fine-tune what dewey indexes per source.

**Independent Test**: Add `ignore: [drafts]` to a disk source in `sources.yaml`, create markdown files in a `drafts/` directory, run `dewey serve` or `dewey index`, and verify those files are excluded.

**Acceptance Scenarios**:

1. **Given** a disk source with `ignore: [drafts]` in `sources.yaml`, **When** the source is indexed, **Then** all files under `drafts/` are skipped
2. **Given** a disk source with both `ignore` patterns and a `.gitignore` at the source root, **When** the source is indexed, **Then** both sets of patterns are applied (union — a directory is skipped if either source says to skip it)
3. **Given** a disk source with no `ignore` field, **When** the source is indexed, **Then** only `.gitignore` and hidden directories are skipped (backward compatible)

---

### User Story 3 - Non-Recursive Source Indexing (Priority: P3)

A developer has a `disk-org` source pointing to a parent directory (`../`) that contains multiple sibling repos. Today this source recursively walks all subdirectories, duplicating content from per-repo sources (`disk-gaze`, `disk-website`, etc.) and indexing junk like `node_modules/`. The developer sets `recursive: false` on the source to index only top-level markdown files in the parent directory (org-level READMEs, design docs) without descending into subdirectories.

**Why this priority**: Prevents massive content duplication and junk indexing for org-level sources. Lower priority because the immediate timeout is fixed by P1.

**Independent Test**: Set `recursive: false` on a disk source, place markdown files at the root and in subdirectories, run `dewey index`, and verify only root-level files are indexed.

**Acceptance Scenarios**:

1. **Given** a disk source with `recursive: false`, **When** the source is indexed, **Then** only `.md` files directly in the source root are included (no subdirectory traversal)
2. **Given** a disk source with no `recursive` field, **When** the source is indexed, **Then** all subdirectories are traversed (default `true`, backward compatible)
3. **Given** a disk source with `recursive: false`, **When** the source root contains subdirectories with `.md` files, **Then** those subdirectory files are not indexed

---

### User Story 4 - File Watcher Ignore Consistency (Priority: P4)

When a developer saves a file inside an ignored directory (e.g., `node_modules/`), the file watcher should not trigger a re-index. The watcher must apply the same ignore rules as the initial vault walk so that runtime behavior is consistent with startup behavior.

**Why this priority**: Without this, saving any file in an ignored directory would trigger unnecessary re-indexing, causing performance issues and inconsistent state.

**Independent Test**: Start `dewey serve` with a `.gitignore` excluding `node_modules/`, modify a file inside `node_modules/`, and verify no re-index event is triggered.

**Acceptance Scenarios**:

1. **Given** `dewey serve` is running with `.gitignore` excluding `node_modules/`, **When** a file inside `node_modules/` is modified, **Then** no re-index event fires
2. **Given** `dewey serve` is running with `ignore: [drafts]` in `sources.yaml`, **When** a file inside `drafts/` is modified, **Then** no re-index event fires
3. **Given** `dewey serve` is running, **When** a file in a non-ignored directory is modified, **Then** the re-index fires as expected (existing behavior preserved)

---

### Edge Cases

- What happens when `.gitignore` is malformed or contains syntax errors? Dewey logs a warning and falls back to hidden-directory-only skipping (existing behavior). Malformed lines are ignored, not fatal.
- What happens when `.gitignore` does not exist at the source root? Only hidden directories and `sources.yaml` ignore patterns apply. No error.
- What happens when a directory is ignored by `.gitignore` but an `ignore` pattern in `sources.yaml` tries to negate it? Union merge means both apply — there is no way to "un-ignore" via `sources.yaml` what `.gitignore` already excludes. This is intentional: `.gitignore` is the baseline, `sources.yaml` only adds exclusions.
- What happens when `recursive: false` is combined with `ignore` patterns? The `ignore` patterns are irrelevant since subdirectories are not traversed at all, but no error is raised.
- What happens when the local vault source (`disk-local`) has ignore patterns? They apply to `vault.Load()`, `walkVault()`, and the file watcher — the same walker paths used for the local vault at serve time.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: All filesystem walkers MUST respect `.gitignore` patterns found at the root of the directory being walked
- **FR-002**: Only the root-level `.gitignore` MUST be parsed — nested `.gitignore` files in subdirectories are not processed
- **FR-003**: The `.gitignore` parser MUST support directory patterns (e.g., `node_modules/`), file patterns (e.g., `*.log`), negation patterns (e.g., `!important.md`), comments (`#`), and blank lines
- **FR-004**: Hidden directories (names starting with `.`) MUST always be skipped regardless of ignore file contents (existing behavior preserved)
- **FR-005**: Disk sources in `sources.yaml` MUST support an optional `ignore` field — a list of directory or file patterns applied in addition to `.gitignore` (union merge)
- **FR-006**: Disk sources in `sources.yaml` MUST support an optional `recursive` field (boolean, default `true`) — when `false`, only files directly in the source root are indexed
- **FR-007**: The `dewey init` command MUST generate `sources.yaml` with `recursive: false` as the default for any source whose path is `"../"` or equivalent parent directory references
- **FR-008**: All four filesystem walkers MUST use the same ignore logic: `vault.Load()`, `vault.walkVault()`, `vault.addWatcherDirs()`, and `source.DiskSource.List()`
- **FR-009**: The file watcher event handler MUST apply the same ignore rules so that events from ignored directories do not trigger re-indexing
- **FR-010**: When `.gitignore` is absent or unreadable, the system MUST fall back to skipping hidden directories only (no error)
- **FR-011**: When `.gitignore` contains malformed lines, those lines MUST be skipped with a logged warning — parsing does not abort
- **FR-012**: The ignore logic MUST be shared between the `vault` and `source` packages without code duplication
- **FR-013**: The `dewey doctor` command MUST report ignored directory counts when running in verbose mode

### Key Entities

- **Ignore Matcher**: Evaluates whether a given directory name or file path should be skipped during a filesystem walk. Built once per walk from the union of `.gitignore` patterns, `sources.yaml` ignore patterns, and the hardcoded hidden-directory rule.
- **Source Config (extended)**: The existing `sources.yaml` disk source configuration, extended with `ignore` (list of patterns) and `recursive` (boolean) fields.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `dewey serve` on a project with `node_modules/` in `.gitignore` starts and responds to MCP requests within 5 seconds (down from 40+ seconds today)
- **SC-002**: `dewey index` on a source pointing to a directory with `.gitignore` produces zero pages from gitignored directories
- **SC-003**: Running `dewey status` after re-indexing the `disk-website` source shows the correct file count (approximately 137 pages, down from 443 today)
- **SC-004**: All existing tests continue to pass — no behavioral change for vaults without `.gitignore` or `sources.yaml` ignore configuration
- **SC-005**: The `disk-org` source with `recursive: false` indexes only top-level markdown files, eliminating duplicate content from per-repo sources

## Assumptions

- The `.gitignore` pattern syntax follows the git specification for directory and file matching. A minimal parser covering the patterns found in real Unbound Force repos (directory names, file globs, negation, comments) is sufficient — full git pathspec semantics (e.g., `**/` double-star matching) can be deferred if not needed.
- The `ignore` field in `sources.yaml` uses the same pattern syntax as `.gitignore` for consistency. Users do not need to learn a separate syntax.
- The `recursive` field defaults to `true` for backward compatibility. Only `dewey init` changes the default for parent-directory sources.
- The local vault walker (`vault.Load()`, `walkVault()`) reads its ignore configuration from the `disk-local` source entry in `sources.yaml`. If no `disk-local` entry exists, only `.gitignore` and hidden-directory skipping apply.
