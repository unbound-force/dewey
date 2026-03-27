## ADDED Requirements

### Requirement: Decomposed RenamePage

The `(*Client).RenamePage` method in `vault/vault.go` MUST decompose its file-rename, index-rebuild, and cleanup phases into private methods.

#### Scenario: File rename phase handles directory creation
- **GIVEN** a page being renamed to a path with non-existent parent directories
- **WHEN** the rename file phase executes
- **THEN** it MUST create the target directory and rename the file atomically

#### Scenario: Reindex phase reads and indexes the renamed file
- **GIVEN** a file has been successfully renamed
- **WHEN** the reindex phase executes
- **THEN** it MUST read the file content, index it under the new name, and rebuild backlinks

#### Scenario: Cleanup phase removes empty directories
- **GIVEN** a rename that leaves the old file's directory empty
- **WHEN** the cleanup phase executes
- **THEN** it MUST walk up from the old directory removing empty directories until it reaches the vault root or a non-empty directory

#### Scenario: RenamePage produces identical results after decomposition
- **GIVEN** a vault with known files and link structure
- **WHEN** `RenamePage` is called before and after refactoring
- **THEN** the resulting file structure, link updates, and index state MUST be identical

### Requirement: Decomposed Source Add Command

The `newSourceAddCmd` function in `cli.go` MUST extract per-type source config builders and save logic into private functions.

#### Scenario: GitHub source builder validates required fields
- **GIVEN** a call with `sourceType == "github"` and `--org` empty
- **WHEN** the builder is called
- **THEN** it MUST return an error indicating `--org is required`

#### Scenario: Web source builder derives name from URL
- **GIVEN** a call with `--url https://pkg.go.dev/std` and `--name` empty
- **WHEN** the builder is called
- **THEN** it MUST derive the name as `"pkg.go.dev"` from the URL hostname

#### Scenario: Save helper rejects duplicate sources
- **GIVEN** an existing sources list containing source ID `"github-org"`
- **WHEN** the save helper is called with a new source having the same ID
- **THEN** it MUST return an error indicating the source already exists

### Requirement: indexDocuments Test Coverage

The `indexDocuments` function in `cli.go` MUST have dedicated test coverage verifying page upsert and source record operations.

#### Scenario: Insert new page from document
- **GIVEN** an in-memory store with no existing pages
- **WHEN** `indexDocuments` is called with one document
- **THEN** the store MUST contain a page with the document's title, content hash, source ID, and source doc ID

#### Scenario: Update existing page
- **GIVEN** an in-memory store with an existing page
- **WHEN** `indexDocuments` is called with a document having the same title but different content hash
- **THEN** the page's content hash MUST be updated to the new value

#### Scenario: Source record creation
- **GIVEN** documents from a source not yet in the store
- **WHEN** `indexDocuments` completes
- **THEN** a source record MUST exist with the correct type and status

#### Scenario: Properties marshaling
- **GIVEN** a document with a non-nil Properties map
- **WHEN** `indexDocuments` inserts the page
- **THEN** the page's Properties field MUST contain valid JSON matching the original map

### Requirement: Whiteboard Contract Assertion Restructuring

Tests for `GetWhiteboard` in `tools/whiteboard_test.go` MUST structure assertions so that Gaze's contract mapper can trace them to observable side effects.

#### Scenario: Assertions trace to embedded pages
- **GIVEN** a mock backend with whiteboard blocks containing embedded page references
- **WHEN** the test calls `GetWhiteboard` and parses the result
- **THEN** the test MUST assert directly on the `embeddedPages` field of the parsed map with a dedicated `t.Errorf` call

#### Scenario: Assertions trace to connections
- **GIVEN** a mock backend with whiteboard blocks containing source/target connector properties
- **WHEN** the test calls `GetWhiteboard` and parses the result
- **THEN** the test MUST assert directly on the `connections` field with dedicated comparison assertions

### Requirement: Decomposed DiskSource Diff

The `(*DiskSource).Diff` method in `source/disk.go` MUST split its filesystem walk and hash comparison into private functions.

#### Scenario: Walk phase returns file inventory
- **GIVEN** a directory with markdown files and hidden directories
- **WHEN** the walk phase executes
- **THEN** it MUST return a map of relative paths to content hashes, skipping hidden directories and non-.md files

#### Scenario: Diff phase categorizes changes
- **GIVEN** current file hashes and stored hashes
- **WHEN** the diff phase executes
- **THEN** it MUST return changes categorized as added, modified, or deleted

#### Scenario: Diff produces identical results after decomposition
- **GIVEN** a disk source with known files and stored hashes
- **WHEN** `Diff` is called before and after refactoring
- **THEN** the returned changes MUST be identical

## MODIFIED Requirements

### Requirement: CI Quality Gate Headroom

The project MUST maintain increasing headroom below CI quality gates. Previously: CRAPload=10 (gate: 15), GazeCRAPload=33 (gate: 34).

Updated targets:
- CRAPload MUST be <= 7 (down from 10)
- GazeCRAPload MUST be <= 28 (down from 33)

## REMOVED Requirements

None.
