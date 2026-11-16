---
description: Delete and rebuild all external source content in the Dewey index.
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

<protect>
## Instructions

**Warning**: This deletes all external source content and rebuilds
from scratch. Local vault content and stored learnings are preserved.

Call the Dewey MCP tool `reindex` with no parameters.

Display the returned summary showing pages deleted, sources
re-indexed, new page counts, and elapsed time.
</protect>
<!-- dewey:scaffold dev sha256:a1721e18995444e5dffe1f0df1db2a40d92e215772b1fbc59649b7bac66c7af0 -->
