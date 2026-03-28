## ADDED Requirements

_None_

## MODIFIED Requirements

### Requirement: CLI Search Uses Vault Backend

The `dewey search` command MUST use the Obsidian vault backend (same data path as `dewey serve`) when the backend is `obsidian` (the default). The command MUST call `vault.Client.FullTextSearch()` to find matching blocks.

Previously: The search command always used the Logseq HTTP client (`client.New("", "")`), which returned zero results when Logseq was not running.

#### Scenario: Search with default backend finds local files
- **GIVEN** the user has `.md` files in the vault directory
- **WHEN** the user runs `dewey search "keyword" --vault .`
- **THEN** the command returns matching blocks from local `.md` files

#### Scenario: Search includes external-source content
- **GIVEN** the user has run `dewey index` and has external pages in `graph.db`
- **WHEN** the user runs `dewey search "keyword" --vault .`
- **THEN** the command returns matches from both local files and external sources

#### Scenario: Search with no matches
- **GIVEN** the user has `.md` files in the vault directory
- **WHEN** the user runs `dewey search "nonexistent-term-xyz" --vault .`
- **THEN** the command returns an error message `no results for "nonexistent-term-xyz"`

#### Scenario: Search without vault path
- **GIVEN** no `--vault` flag is provided and `DEWEY_VAULT_PATH` is not set
- **WHEN** the user runs `dewey search "keyword"`
- **THEN** the command returns a clear error about the missing vault path

## REMOVED Requirements

_None_
