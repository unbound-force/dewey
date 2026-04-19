## NEW Requirements

### Requirement: /dewey-store Slash Command

A `/dewey-store` slash command MUST exist at `.opencode/command/dewey-store.md` that instructs the agent to store pasted text as a Dewey learning via the `store_learning` MCP tool.

#### Scenario: Fully specified (tag and category provided)
- **GIVEN** the user types `/dewey-store --tag auth-design --category decision` followed by text
- **WHEN** the agent processes the command
- **THEN** `store_learning` is called immediately with the provided tag, category, and text
- **AND** the returned `{tag}-{sequence}` identity is displayed

#### Scenario: Text only (no flags)
- **GIVEN** the user types `/dewey-store` followed by text with no flags
- **WHEN** the agent processes the command
- **THEN** the agent suggests 2-3 tags ranked by specificity with reasoning
- **AND** the agent suggests a category with reasoning
- **AND** the agent waits for the user to confirm or override before calling `store_learning`

#### Scenario: Extract mode
- **GIVEN** the user types `/dewey-store --extract` followed by a long conversation
- **WHEN** the agent processes the command
- **THEN** the agent identifies multiple distinct facts/decisions in the text
- **AND** displays each proposed learning with tag and category in a numbered list
- **AND** waits for user confirmation before storing

#### Scenario: Post-store suggestion
- **GIVEN** one or more learnings have been stored
- **WHEN** the storage completes
- **THEN** the agent suggests running `/dewey-compile` to synthesize with existing knowledge
