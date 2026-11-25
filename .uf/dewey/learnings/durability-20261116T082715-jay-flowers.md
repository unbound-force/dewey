---
tag: durability
author: jay-flowers
category: pattern
created_at: 2026-11-16T08:27:15Z
identity: durability-20261116T082715-jay-flowers
tier: draft
as_of: "dewey@f6911abc6403998f6eaf1c2bb6d09007e516a2c6"
---

For tool-owned scaffold refresh, temp-file-and-rename gives atomic visibility but not durability: without fsync of the temp file before rename (and of the directory after), a crash can persist the rename while losing the data, leaving a truncated file whose digest no longer matches. A digest-based classifier then reads that corruption as a user customization and preserves it forever. Pair atomic replace with `tmp.Sync()` or a post-rename digest re-read if the classifier treats mismatch as fail-safe.
