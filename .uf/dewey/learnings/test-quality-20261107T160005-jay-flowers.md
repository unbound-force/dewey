---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:00:05Z
identity: test-quality-20261107T160005-jay-flowers
tier: draft
---

Resolution ladders behind read fallbacks need coexistence fixtures for every precedence pair, not just per-step hits. A test matrix that covers each lookup's hit (exact, compiled/<name>, learning/<name>) plus one adjacent-pair order (compiled over learning) still leaves the exact-vs-prefix cell unlocked: reordering the ladder to try prefixes first passes the whole suite. When writing or reviewing tests for an ordered name-resolution chain, enumerate the full precedence matrix and add at least one fixture wh
