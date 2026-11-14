# Capability: promotion-workflow

## ADDED Requirements

### Requirement: PW-FR-001 Promote edits frontmatter on a branch

`dewey promote` (CLI) and the `promote` MCP tool MUST promote a compiled or curated page by setting `tier: validated` in the page's file frontmatter on the current branch, so that a pull request can carry the promotion for human review. On success the file on disk MUST contain `tier: validated`. The file edit MUST be atomic: write a temporary file in the same directory as the target and rename it over the original, preserving the original file mode; a failed or interrupted edit MUST leave the original file unchanged and return an error.

#### Scenario: Promote writes file not transient state
- **GIVEN** a draft compiled page `compiled/auth.md` on a feature branch with `tier: draft` in its frontmatter
- **WHEN** the user runs `dewey promote compiled/auth.md` (or calls the MCP `promote` tool)
- **THEN** the file `compiled/auth.md` contains `tier: validated` in its frontmatter and `git diff` shows the promotion
#### Scenario: MCP promote mirrors CLI file edit

- **GIVEN** the MCP server is running on a feature branch with a draft compiled page
- **WHEN** an agent calls `promote` for that page
- **THEN** the same frontmatter edit occurs on disk as the CLI path

#### Scenario: Failed promote edit leaves the original file unchanged

- **GIVEN** a compiled page `compiled/auth.md` and a condition that makes the edit fail (e.g. the target directory is read-only)
- **WHEN** the user runs `dewey promote compiled/auth.md`
- **THEN** the command returns an error and `compiled/auth.md` is byte-identical to its pre-call content (atomic temp-file + rename; no partial write)

### Requirement: PW-FR-002 Promote does not write DB tier directly

`promote` MUST NOT set `pages.tier = 'validated'` (or equivalent in-memory tier) as the durable promotion mechanism. The database tier MUST remain a cache of the git rule (GTS-FR-005); any DB update performed for indexing convenience MUST be derived from the file-plus-ref check and MUST NOT outlive the next index.

#### Scenario: Promote does not bypass git gate
- **GIVEN** `promote` has edited the file on a feature branch
- **WHEN** ingestion runs on that branch before merge
- **THEN** the effective tier remains `draft` per GTS-FR-002 and no DB-only flag makes it appear `validated`

#### Scenario: Promote file change is observable as branch diff
- **GIVEN** a promotion has occurred on a branch
- **WHEN** the user inspects `git status` / PR diff
- **THEN** the promotion is visible as a file modification, not hidden local DB state

### Requirement: PW-FR-003 Branch-only promotion not visible as validated until merged

A page promoted on a branch that has not been merged to the default branch MUST continue to ingest as `draft` on all servers (including the promoting server's next index) until the default-branch ref contains the new blob. Merging the PR is the vouch.

#### Scenario: Pre-merge promotion stays draft
- **GIVEN** a branch promotion to `tier: validated` has not been merged to `origin/main`
- **WHEN** any server (including the author's) re-indexes
- **THEN** the effective tier is `draft`
#### Scenario: Post-merge promotion becomes validated

- **GIVEN** the promotion PR has been merged and `origin/main` has been fetched to include the new blob
- **WHEN** a server indexes after the fetch
- **THEN** the effective tier becomes `validated` (GTS-FR-001 satisfied)

### Requirement: PW-FR-004 Promote resolves a canonical, vault-contained path

`dewey promote` (CLI) and the `promote` MCP tool MUST resolve the page argument to a canonical, vault-contained path before performing any file edit. Resolution MUST start from the vault root, clean the joined path, resolve symlinks in every component to a fully resolved absolute path, and verify that the fully resolved path stays inside the vault root and under the `compiled/` or `knowledge/` prefixes. The tool MUST reject with an error, without reading or modifying any file: absolute paths, paths containing `..` segments before or after canonicalization, any resolved path that does not fall under the `compiled/` or `knowledge/` prefixes, and any path whose fully resolved form (following all symlinks) escapes the vault or the allowed prefixes. The final file mutation MUST use no-follow semantics on the final component (`O_NOFOLLOW` or a dirfd-relative handle opened after validation) so a symlink swapped between validation and edit (TOCTOU) is treated as validation failure and no file outside the allowed prefixes is modified.

#### Scenario: Path traversal is rejected

- **GIVEN** a caller invoking `dewey promote ../../.env`, `compiled/evil/../../secrets`, or an absolute path
- **WHEN** promote resolves the argument
- **THEN** the command returns a path-validation error, no file is modified, and no path outside `compiled/`/`knowledge/` is read or written

#### Scenario: Only compiled and knowledge prefixes are promotable

- **GIVEN** a learning file `learning/foo-20260502T143022-alice.md` outside the promotable prefixes
- **WHEN** the user runs `dewey promote learning/foo-20260502T143022-alice.md`
- **THEN** the command returns a path-validation error and the learning file is not edited (learnings are never promotable per IP-FR-002)

#### Scenario: Symlink escape is rejected and TOCTOU is not followed

- **GIVEN** a vault path where `compiled/link` is a symlink to `/etc/passwd` or to `../../secrets`, or where an intermediate directory component (e.g. `compiled/subdir`) is a symlink outside the vault, or where a symlink is swapped in between validation and edit
- **WHEN** promote resolves and opens the argument
- **THEN** the command returns a path-validation error, no file outside `compiled/`/`knowledge/` is modified, and the no-follow open rejects the swapped symlink as a validation failure

<!-- scaffolded by uf vdev -->
