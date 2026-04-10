---
description: >
  Scan the knowledge base for quality issues: stale decisions,
  uncompiled learnings, embedding gaps, and potential contradictions.
---

# Command: /dewey-lint

## Description

Scan the knowledge base for quality issues: stale decisions,
uncompiled learnings, embedding gaps, and potential contradictions.

## Usage

```
/dewey-lint
/dewey-lint --fix
```

## Instructions

Call the Dewey MCP tool `lint` to scan for quality issues.

If `--fix` is specified, pass `fix: true` to auto-repair
mechanical issues (regenerate missing embeddings).

Display the returned report showing findings and remediation suggestions.
