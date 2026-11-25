---
tag: dewey-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-19T01:43:57Z
identity: dewey-testing-20261119T014357-jay-flowers
tier: draft
as_of: "dewey@2a7f0147afcf07964a3cb8ce9d52b7790e1a505e"
---

dewey's store.GetMeta returns an empty string both for a missing metadata key and for a key stored as empty, so a test asserting "the key was never written" via GetMeta cannot distinguish absence from an empty write. Assert absence with a direct SQL count against the metadata table instead — the pattern TestStatusCmd_StatusNeverStamps established: `SELECT COUNT(*) FROM metadata WHERE key IN (...)` must be 0. When a pair of keys is written together, a GetMeta-based workaround is to assert on the key whose zero-value encoding differs from empty (e.g., index_format zero-value writes as "0", not ""), as the tools-side assertBuildStamp relies on — but direct SQL is the precise check.
