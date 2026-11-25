---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T04:43:56Z
identity: test-quality-20261119T044356-jay-flowers
tier: draft
as_of: "dewey@377610524098d68d9f685178d7f173f0a5558585"
---

When a delta-spec acceptance scenario pins error-message content (e.g., "the result is an error naming the expected shape"), that clause is contract surface and needs a message-content assertion — a validator table test that asserts only error presence/absence per case leaves the clause unlocked, and a message regression passes the suite. The pattern to mirror is TestValidateValidity in tools/learning_temporal_test.go, which asserts strings.Contains(err.Error(), "stale-unverified") alongside the accept/reject table. Apply the same check to every sibling validator test (TestValidateCallerAsOf initially omitted it).
