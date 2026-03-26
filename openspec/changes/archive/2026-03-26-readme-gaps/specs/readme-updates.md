## ADDED Requirements

### Requirement: Homebrew Cask Install Documentation

The README Install section MUST include `brew install --cask unbound-force/tap/dewey` as the primary macOS installation method, listed before `go install`.

#### Scenario: macOS developer installs via Homebrew
- **GIVEN** a developer reads the Install section of the README
- **WHEN** they look for the macOS installation command
- **THEN** they find `brew install --cask unbound-force/tap/dewey` as the first installation option

#### Scenario: Linux developer installs via go install
- **GIVEN** a developer on Linux reads the Install section
- **WHEN** they look for installation instructions
- **THEN** they find `go install github.com/unbound-force/dewey@latest` as an alternative to Homebrew

### Requirement: Persistence Documentation

The README MUST include a "Persistence" section explaining the `.dewey/` directory structure, the `graph.db` SQLite database, and incremental indexing behavior.

#### Scenario: Developer understands persistence
- **GIVEN** a developer has initialized Dewey with `dewey init`
- **WHEN** they read the Persistence section
- **THEN** they understand that Dewey stores its index in `.dewey/graph.db`, re-uses it across sessions, and only re-processes changed files

### Requirement: Content Sources Configuration Documentation

The README MUST include a "Content Sources" section with a complete `sources.yaml` example showing all three source types: local disk, GitHub API, and web crawl.

#### Scenario: Developer configures GitHub source
- **GIVEN** a developer wants to index GitHub issues from their organization
- **WHEN** they read the Content Sources section
- **THEN** they find a YAML example showing a GitHub source with `org`, `repos`, `content_types`, and `refresh` fields

#### Scenario: Developer configures web crawl source
- **GIVEN** a developer wants to index external documentation
- **WHEN** they read the Content Sources section
- **THEN** they find a YAML example showing a web source with `urls`, `depth`, and `refresh` fields

### Requirement: Semantic Search Setup Documentation

The README MUST include a "Semantic Search Setup" section documenting Ollama installation, embedding model pull, and verification.

#### Scenario: Developer sets up semantic search
- **GIVEN** a developer wants to use `dewey_semantic_search`
- **WHEN** they read the Semantic Search Setup section
- **THEN** they find step-by-step instructions for installing Ollama, pulling the `granite-embedding:30m` model, and verifying semantic search works

#### Scenario: Developer understands graceful degradation
- **GIVEN** a developer does not have Ollama installed
- **WHEN** they read the Semantic Search Setup section
- **THEN** they understand that all keyword-based tools continue to work without Ollama and that semantic search tools return clear error messages

## MODIFIED Requirements

### Requirement: Install Section Order

The Install section MUST list installation methods in order of preference: Homebrew cask (macOS), `go install` (cross-platform), build from source. Previously: only `go install` and build from source were listed.

## REMOVED Requirements

None.
