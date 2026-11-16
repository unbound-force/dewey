---
description: Curate knowledge from indexed sources into structured knowledge stores.
---

# Command: /dewey-curate

## Description

Run the Dewey curation pipeline to extract decisions, facts, patterns,
and context from indexed sources. Uses LLM analysis to produce structured
knowledge files with quality flags and confidence scores.

## Usage

```
/dewey-curate
/dewey-curate --store team-decisions
/dewey-curate --force
```

<protect>
## Instructions

1. Call the `curate` MCP tool
2. If the tool returns extraction prompts (no local LLM), perform synthesis
3. Report the results: files created, quality flags, confidence distribution
</protect>
<!-- dewey:scaffold dev sha256:b816fa596ae726850a0b8d36a612966af827e509fbf8d2c6d56ca86b54a5e36e -->
