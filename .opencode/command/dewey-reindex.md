---
description: >
  Delete and rebuild all external source content in the Dewey index.
  Preserves local vault content and stored learnings. Use when the
  index appears stale or corrupted.
---

# Command: /dewey-reindex

## Description

Delete and rebuild all external source content in the Dewey index.
Preserves local vault content and stored learnings. Use when the
index appears stale or corrupted.

## Usage

```
/dewey-reindex
```

## Instructions

**Warning**: This deletes all external source content and rebuilds
from scratch. Local vault content and stored learnings are preserved.

Call the Dewey MCP tool `reindex` with no parameters.

Display the returned summary showing pages deleted, sources
re-indexed, new page counts, and elapsed time.
