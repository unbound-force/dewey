---
tag: frontmatter-parsing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:12:14Z
identity: frontmatter-parsing-20261111T021214-jay-flowers
tier: draft
---

A line-based frontmatter guard that refuses only forms it recognizes, while the writer it guards is equally line-based, fails open on unrecognized-but-YAML-valid spellings: `key : value` (whitespace before the colon), `key:` followed by a blank line then indented children, and a root key with leading indentation all read as "unclaimed"; the writer cannot match the existing line either and appends a duplicate key. Duplicate keys make yaml.v3 reject the file, so re-ingestion skips it and the learning is silently lost after graph.db loss. A guarded rewrite must treat any form it cannot positively parse as claimed-and-differing, allow optional whitespace before the colon, and the seam test should feed the crafted file through both the writer and parseLearningFrontmatter, not just the guard. (serialize-supersession-writes, dewey #166.)
