---
description: Scan the knowledge base for quality issues.
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

<protect>
## Instructions

Call the Dewey MCP tool `lint` to scan for quality issues.

If --fix is specified, pass fix: true to auto-repair
mechanical issues (regenerate missing embeddings).

Display the returned report showing findings and remediation
suggestions.
</protect>
<!-- dewey:scaffold dev sha256:97ceb4de09a3146eaa4f3881586a2f24fb93d45e59611bfd937469c0435d4d45 -->
