## ADDED Requirements

### Requirement: Author Resolution

The `store_learning` tool MUST resolve an author identifier using a three-tier fallback chain:
1. `DEWEY_AUTHOR` environment variable (highest priority)
2. `git config --get user.name` subprocess output (default for git repos)
3. The literal string `"anonymous"` (never fails)

The resolved author MUST be normalized: lowercased, spaces replaced with hyphens, non-alphanumeric characters (except hyphens) stripped. If normalization produces an empty string, the result MUST fall back to `"anonymous"`. Author strings MUST be truncated to 64 characters after normalization.

The `resolveAuthor` function MUST accept an injectable function parameter for the git subprocess call, enabling tests to provide a mock without depending on the test runner's git configuration. Tests for the git config path MUST NOT depend on the test runner's actual git configuration.

The git subprocess MUST use `exec.CommandContext` with a bounded timeout of 2 seconds. If the subprocess exceeds the timeout, the fallback chain MUST proceed to the next tier (`"anonymous"`). Raw subprocess output MUST be trimmed of whitespace and newlines before normalization.

#### Scenario: Author from DEWEY_AUTHOR environment variable
- **GIVEN** the `DEWEY_AUTHOR` environment variable is set to `"Alice"`
- **WHEN** a learning is stored via `store_learning`
- **THEN** the resolved author MUST be `"alice"`
- **AND** the learning identity MUST end with `-alice`

#### Scenario: Author from git config (testable)
- **GIVEN** a git config resolver function that returns `"John Flowers"`
- **AND** `DEWEY_AUTHOR` is not set
- **WHEN** `resolveAuthor` is called
- **THEN** the resolved author MUST be `"john-flowers"`

#### Scenario: Anonymous fallback
- **GIVEN** the `DEWEY_AUTHOR` environment variable is not set
- **AND** `git config --get user.name` fails or returns empty
- **WHEN** a learning is stored via `store_learning`
- **THEN** the resolved author MUST be `"anonymous"`

#### Scenario: Empty or whitespace-only DEWEY_AUTHOR
- **GIVEN** the `DEWEY_AUTHOR` environment variable is set to `""` or `"   "`
- **WHEN** a learning is stored via `store_learning`
- **THEN** `DEWEY_AUTHOR` MUST be treated as unset
- **AND** the fallback chain MUST proceed to `git config --get user.name`

#### Scenario: Non-Latin author name normalizes to empty
- **GIVEN** the `DEWEY_AUTHOR` environment variable is set to `"田中太郎"` (CJK characters only)
- **WHEN** normalization strips all non-alphanumeric characters
- **THEN** the resolved author MUST be `"anonymous"` (empty-after-normalization fallback)

#### Scenario: Git subprocess timeout
- **GIVEN** the git subprocess hangs or takes longer than 2 seconds
- **AND** `DEWEY_AUTHOR` is not set
- **WHEN** `resolveAuthor` is called
- **THEN** the subprocess MUST be terminated after 2 seconds
- **AND** the resolved author MUST be `"anonymous"`

### Requirement: Author in Frontmatter

Learning markdown files MUST include an `author` field in YAML frontmatter.

#### Scenario: Author field present in new learning
- **GIVEN** the resolved author is `"alice"`
- **WHEN** a learning is stored and the markdown file is written
- **THEN** the frontmatter MUST contain `author: alice`

### Requirement: Author in MCP Response

The `store_learning` tool response MUST include the resolved author in the result JSON.

#### Scenario: Author in tool response
- **GIVEN** the resolved author is `"alice"`
- **WHEN** a learning is stored successfully
- **THEN** the JSON response MUST contain `"author": "alice"`

## MODIFIED Requirements

### Requirement: Learning Identity Format

The learning identity format MUST be `{tag}-{YYYYMMDDTHHMMSS}-{author}` where timestamp is UTC and author is the normalized resolved author. This supersedes spec 015 FR-001's `{tag}-{seq}` naming convention.

Previously: Identity format was `{tag}-{seq}` where seq was an auto-incrementing integer per tag from `NextLearningSequence`.

#### Scenario: New identity format
- **GIVEN** a tag `"authentication"`, timestamp `2026-05-02T14:30:22Z`, and author `"alice"`
- **WHEN** a learning identity is generated
- **THEN** the identity MUST be `"authentication-20260502T143022-alice"`
- **AND** the page name MUST be `"learning/authentication-20260502T143022-alice"`
- **AND** the filename MUST be `"authentication-20260502T143022-alice.md"`

#### Scenario: Sub-second collision avoidance
- **GIVEN** a learning file `"auth-20260502T143022-alice.md"` already exists on disk
- **WHEN** another learning with tag `"auth"` is stored by author `"alice"` in the same second
- **THEN** the filename MUST be `"auth-20260502T143022-alice-2.md"`
- **AND** the identity MUST be `"auth-20260502T143022-alice-2"`

#### Scenario: Collision suffix exhaustion
- **GIVEN** learning files `"auth-20260502T143022-alice.md"` through `"auth-20260502T143022-alice-99.md"` all exist on disk
- **WHEN** another learning with tag `"auth"` is stored by author `"alice"` in the same second
- **THEN** `store_learning` MUST return an error indicating the collision suffix limit was exceeded

### Requirement: Learning File Naming

Learning files MUST be written to `{vaultPath}/.uf/dewey/learnings/{identity}.md`. File creation MUST use `O_CREATE|O_EXCL` flags for atomic create-or-fail semantics.

Previously: Files were named `{tag}-{seq}.md`.

### Requirement: Tag Extraction from Identity (Compilation)

The `extractTagFromIdentity` function in `tools/compile.go` MUST handle both old-format (`{tag}-{seq}`) and new-format (`{tag}-{YYYYMMDDTHHMMSS}-{author}`) identities. The function SHOULD read the `tag` from the page's `properties` JSON column as the primary source, falling back to string parsing only when properties are unavailable.

Previously: The function assumed `{tag}-{seq}` format and used `strconv.Atoi` on the suffix to find the sequence separator.

#### Scenario: Tag extraction from new-format identity via properties
- **GIVEN** a learning page with identity `"authentication-20260502T143022-alice"` and properties `{"tag": "authentication"}`
- **WHEN** tag extraction is performed during compilation
- **THEN** the extracted tag MUST be `"authentication"`

#### Scenario: Tag extraction from old-format identity (backward compat)
- **GIVEN** a learning page with identity `"vault-walker-2"` and properties `{"tag": "vault-walker"}`
- **WHEN** tag extraction is performed during compilation
- **THEN** the extracted tag MUST be `"vault-walker"`

### Requirement: Learning Re-ingestion Backward Compatibility

The `reIngestLearnings` function MUST handle both old-format and new-format learning files.

Previously: Only handled `{tag}-{seq}.md` files with frontmatter fields: tag, category, created_at, identity, tier.

#### Scenario: Re-ingest old-format learning
- **GIVEN** a learning file `"authentication-3.md"` exists with frontmatter `identity: authentication-3` and no `author` field
- **WHEN** `reIngestLearnings` runs and the page is missing from the store
- **THEN** the learning MUST be re-ingested with an empty author in properties
- **AND** the page name MUST be `"learning/authentication-3"`

#### Scenario: Re-ingest new-format learning
- **GIVEN** a learning file `"authentication-20260502T143022-alice.md"` exists with frontmatter `identity: authentication-20260502T143022-alice` and `author: alice`
- **WHEN** `reIngestLearnings` runs and the page is missing from the store
- **THEN** the learning MUST be re-ingested with `"author": "alice"` in properties
- **AND** the page name MUST be `"learning/authentication-20260502T143022-alice"`

#### Scenario: Mixed old and new format files in same directory
- **GIVEN** a learnings directory contains 2 old-format files (`tag-1.md`, `tag-2.md`) and 2 new-format files (`tag-20260502T143022-alice.md`, `tag-20260502T143023-bob.md`)
- **WHEN** `reIngestLearnings` runs and all pages are missing from the store
- **THEN** all 4 files MUST be re-ingested successfully
- **AND** old-format files MUST have empty author in properties
- **AND** new-format files MUST have the correct author in properties

## REMOVED Requirements

### Requirement: NextLearningSequence

The `store.NextLearningSequence(tag string) (int, error)` method MUST be removed. Identity generation no longer requires a database counter.

Reason: The COUNT-based sequence is local to each SQLite database instance, causing guaranteed filename collisions across team members. Replaced by timestamp + author identity format.
