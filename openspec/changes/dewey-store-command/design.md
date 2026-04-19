## Context

Developers paste ad-hoc knowledge (Slack DMs, meeting notes, decisions) into the OpenCode prompt. Today they must know the `store_learning` MCP tool API. A `/dewey-store` slash command makes this discoverable and adds intelligent tag/category suggestion.

## Goals / Non-Goals

### Goals
- Create a slash command that accepts pasted text and stores it as a Dewey learning
- Suggest tag and category when not provided, based on text analysis
- Support batch extraction from long conversations via `--extract`
- Suggest `/dewey-compile` after storing

### Non-Goals
- No new MCP tools — the command calls the existing `store_learning` tool
- No Go code changes
- No automated ingestion from external sources (that's a separate concern)

## Decisions

**D1: Three interaction modes.**
- **Fully specified**: `--tag` and `--category` provided → call `store_learning` immediately
- **Suggested**: No flags → agent analyzes text, suggests 2-3 tags and a category with reasoning, asks for confirmation
- **Extract**: `--extract` flag → agent identifies multiple distinct facts/decisions in the text, proposes each as a separate learning with tag/category, asks for confirmation, then stores all

**D2: Tag suggestion strategy.** The agent reads the pasted text and identifies the primary topic. It suggests 2-3 tags ranked by specificity: the most specific first (e.g., `oauth2-timeout`), then broader (e.g., `authentication`), then project-level (e.g., `sprint-review`). The user picks one or types their own.

**D3: Category suggestion strategy.** The agent classifies the text into one of the 5 categories based on content patterns:
- Contains "decided", "agreed", "confirmed", "will use" → `decision`
- Contains "watch out", "gotcha", "be careful", "trap" → `gotcha`
- Contains "pattern", "approach", "technique", "always" → `pattern`
- Contains a URL, reference, link, "see also" → `reference`
- Default → `context`

**D4: Post-store suggestion.** After successful storage, the command outputs the `{tag}-{sequence}` identity and suggests: "Run `/dewey-compile` to synthesize this with existing knowledge."

**D5: Extract mode confirmation.** In `--extract` mode, the agent shows all proposed learnings in a numbered list with tag/category before storing. The user can approve all, remove specific items, or edit tag/category for individual items.

## Risks / Trade-offs

**Trade-off: Tag suggestion quality.** The agent's tag suggestions depend on its ability to identify the topic from short, informal text. Slack messages may lack context. Mitigation: always show suggestions as options, never auto-store without confirmation in suggested mode.
