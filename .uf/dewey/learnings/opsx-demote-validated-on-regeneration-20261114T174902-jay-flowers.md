---
tag: opsx-demote-validated-on-regeneration
author: jay-flowers
category: reference
created_at: 2026-11-14T17:49:02Z
identity: opsx-demote-validated-on-regeneration-20261114T174902-jay-flowers
tier: draft
---

File-specific test knowledge for the content-bound tier contract: tools/compile_test.go's TestCompile_RecompileTierIsContentBound needs two compiles within one wall-clock second for its byte-identical leg because the article's compiled_at frontmatter has RFC 3339 second precision — the bounded 20-attempt retry loop re-promotes through the real promote gate on second-boundary crossings (modeling human re-review of changed bytes) and turns the pathological case into a hard failure instead of a hang; consecutive compiles are ~14 ms apart so it passes deterministically in practice. store/store_test.go's TestUpdatePage_PreservesValidatedTier must remain unmodified — it pins the PR 145 promote race the original UpdatePage comment describes. tools/curate_test.go's TestAutoIndex_ValidatedTierIsContentBound proves the skip-on-equality path with three signals: the indexed return count, tier, and content_hash/updated_at row equality (a tier assertion alone would NOT catch a removed skip because an equal-hash rewrite keeps validated via the store CASE rule).
