---
tag: learning-reingest
author: jay-flowers
category: pattern
created_at: 2026-11-18T21:43:01Z
identity: learning-reingest-20261118T214301-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
---

Re-ingesting file-backed pages (learnings, knowledge-store docs) MUST hash the same bytes on write and read. Store via `store_learning` wrote `sha256(information)` while re-ingestion compared `sha256(TrimSpace(body))`; for padded text the two diverge and cause a one-time spurious re-embed. Fix: hash `TrimSpace(information)` on write so the stored ContentHash equals the file-body hash compared at startup. Mirror the `knowledge-store-reindex` three-branch shape: missing → insert, hash match → cheap properties/tier backfill (no blocks/embeddings, no count), hash mismatch (including empty) → DeleteBlocksByPage → UpdatePage → PersistBlocks → GenerateEmbeddings, tier clamped to draft with warning for forged claims.
