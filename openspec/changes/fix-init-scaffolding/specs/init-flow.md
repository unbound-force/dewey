## MODIFIED Requirements

### Requirement: dewey init Slash Command Scaffolding

`dewey init` MUST scaffold Dewey slash commands on every invocation, not just the first.

Previously: Early return on "already initialized" prevented scaffolding.

#### Scenario: Re-init scaffolds new commands
- **GIVEN** a repo with `.uf/dewey/` already initialized and `.opencode/` present
- **WHEN** the developer runs `dewey init`
- **THEN** any missing slash commands are created in `.opencode/command/`
- **AND** existing slash commands are NOT overwritten

#### Scenario: First init still works
- **GIVEN** a repo with no `.uf/dewey/` directory
- **WHEN** the developer runs `dewey init`
- **THEN** `.uf/dewey/` is created with config and sources
- **AND** slash commands are scaffolded into `.opencode/command/`
