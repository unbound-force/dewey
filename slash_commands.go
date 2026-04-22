package main

// deweySlashCommands contains the slash command definitions that
// `dewey init` scaffolds into `.opencode/command/` when the
// `.opencode/` directory exists. Each entry maps a filename to
// the command content. Files that already exist are not overwritten
// to preserve user customizations.
var deweySlashCommands = map[string]string{
	"dewey-store.md": `---
description: Store pasted text as a Dewey learning with intelligent tag and category suggestions.
---

# Command: /dewey-store

## Description

Store ad-hoc knowledge (Slack DMs, meeting notes, decisions,
observations) as a Dewey learning. Supports three modes:
fully specified, suggested (agent analyzes text and proposes
tag/category), and extract (breaks a long conversation into
multiple learnings).

## Usage

` + "```" + `
/dewey-store --tag auth-design --category decision
Sarah confirmed we need OAuth2 + SAML for enterprise.

/dewey-store
Just talked to the team — we're switching to PostgreSQL.

/dewey-store --extract
[paste a long Slack conversation or meeting transcript]
` + "```" + `

## Instructions

### 1. Parse Input

Read the user's message after ` + "`/dewey-store`" + `.

**Check for flags**:
- ` + "`--tag <value>`" + `: Topic tag for the learning
- ` + "`--category <value>`" + `: One of decision, pattern, gotcha, context, reference
- ` + "`--extract`" + `: Enable multi-learning extraction mode

Everything after the flags is the **text to store**.

If no text is provided, ask:
> "What knowledge would you like to store? Paste the text."

### 2. Determine Mode

**Mode A: Fully Specified** (both --tag and --category provided)
Call store_learning immediately. Skip to Step 5.

**Mode B: Suggested** (no --tag or no --category)
Proceed to Step 3.

**Mode C: Extract** (--extract flag present)
Proceed to Step 4.

### 3. Analyze and Suggest (Mode B)

**Tag suggestion**: Suggest 2-3 tags ranked by specificity.

**Category suggestion**: Classify based on content:
- "decided", "agreed", "confirmed" → decision
- "watch out", "gotcha", "careful" → gotcha
- "pattern", "approach", "technique" → pattern
- URL, "see also", "reference" → reference
- Default → context

Wait for user confirmation before calling store_learning.

### 4. Multi-Learning Extraction (Mode C)

Identify distinct pieces of knowledge. Present as numbered
list with tag/category for each. Wait for confirmation.

### 5. Post-Store

Display the returned identity and suggest /dewey-compile.
`,

	"dewey-index.md": `---
description: Trigger an incremental re-index of all configured content sources.
---

# Command: /dewey-index

## Description

Trigger an incremental re-index of all configured content sources.
Updates the Dewey knowledge graph with new and changed content from
disk, GitHub, web, and code sources without leaving the OpenCode session.

## Usage

` + "```" + `
/dewey-index
/dewey-index disk-website
` + "```" + `

## Instructions

Call the Dewey MCP tool ` + "`index`" + ` to re-index configured sources.

If the user provided a source ID argument (e.g., disk-website),
pass it as the source_id parameter to index only that source.

If no argument is provided, call index with no parameters to
index all configured sources.

Display the returned summary showing sources processed, pages
new/changed/deleted, embeddings generated, and elapsed time.
`,

	"dewey-reindex.md": `---
description: Delete and rebuild all external source content in the Dewey index.
---

# Command: /dewey-reindex

## Description

Delete and rebuild all external source content in the Dewey index.
Preserves local vault content and stored learnings. Use when the
index appears stale or corrupted.

## Usage

` + "```" + `
/dewey-reindex
` + "```" + `

## Instructions

**Warning**: This deletes all external source content and rebuilds
from scratch. Local vault content and stored learnings are preserved.

Call the Dewey MCP tool ` + "`reindex`" + ` with no parameters.

Display the returned summary showing pages deleted, sources
re-indexed, new page counts, and elapsed time.
`,

	"dewey-compile.md": `---
description: Synthesize stored learnings into compiled knowledge articles.
---

# Command: /dewey-compile

## Description

Synthesize stored learnings into compiled knowledge articles.
Groups learnings by topic, resolves contradictions temporally,
and produces current-state articles with history.

## Usage

` + "```" + `
/dewey-compile
` + "```" + `

## Instructions

Call the Dewey MCP tool ` + "`compile`" + ` to synthesize stored learnings.

Display the returned summary showing topics compiled, articles
generated, and elapsed time.
`,

	"dewey-curate.md": `---
description: Curate knowledge from indexed sources into structured knowledge stores.
---

# Command: /dewey-curate

## Description

Run the Dewey curation pipeline to extract decisions, facts, patterns,
and context from indexed sources. Uses LLM analysis to produce structured
knowledge files with quality flags and confidence scores.

## Usage

` + "```" + `
/dewey-curate
/dewey-curate --store team-decisions
/dewey-curate --force
` + "```" + `

## Instructions

1. Call the ` + "`curate`" + ` MCP tool
2. If the tool returns extraction prompts (no local LLM), perform synthesis
3. Report the results: files created, quality flags, confidence distribution
`,

	"dewey-lint.md": `---
description: Scan the knowledge base for quality issues.
---

# Command: /dewey-lint

## Description

Scan the knowledge base for quality issues: stale decisions,
uncompiled learnings, embedding gaps, and potential contradictions.

## Usage

` + "```" + `
/dewey-lint
/dewey-lint --fix
` + "```" + `

## Instructions

Call the Dewey MCP tool ` + "`lint`" + ` to scan for quality issues.

If --fix is specified, pass fix: true to auto-repair
mechanical issues (regenerate missing embeddings).

Display the returned report showing findings and remediation
suggestions.
`,
}
