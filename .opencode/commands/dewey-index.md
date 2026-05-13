---
description: >
  Trigger an incremental re-index of all configured content sources.
  Updates the Dewey knowledge graph with new and changed content from
  disk, GitHub, web, and code sources without leaving the OpenCode session.
---

# Command: /dewey-index

## Description

Trigger an incremental re-index of all configured content sources.
Updates the Dewey knowledge graph with new and changed content from
disk, GitHub, web, and code sources without leaving the OpenCode session.

## Usage

```
/dewey-index
/dewey-index disk-website
```

## Instructions

Call the Dewey MCP tool `index` to re-index configured sources.

If the user provided a source ID argument (e.g., `disk-website`),
pass it as the `source_id` parameter to index only that source.

If no argument is provided, call `index` with no parameters to
index all configured sources.

Display the returned summary showing sources processed, pages
new/changed/deleted, embeddings generated, and elapsed time.
