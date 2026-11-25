---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:40:18Z
identity: test-coverage-20261107T164018-jay-flowers
tier: draft
---

Shared-fallback changes need consumer-scoped tests, not spec-scoped tests. In dewey PR #163, bare-name resolution landed inside VaultStore.GetStorePage (design D1), so every vault-client read inherited it: get_links forward direction, traverse/bfs, find_connections, whiteboard, search snippets, journal, and bulk_update_properties reads all broadened silently beyond the two tools the spec named. The client-level path was tested, but no fixture pinned any incidental consumer and the capability list omitted them. Pattern: after placing a behavior change in a shared helper, grep for all callers of that helper and treat the caller set as the coverage surface — pin at least one incidental consumer (especially inherited tools protected by a backward-compatibility or neighborhood rule) with a fixture, and record the broadened surface in the change's capability documentation. Spec-scoped tests leave adjacent-tool behavior unpinned; the gap is cheap to close at implementation time and expensive to rediscover after a refactor.
