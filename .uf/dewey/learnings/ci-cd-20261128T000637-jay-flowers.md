---
tag: ci-cd
author: jay-flowers
category: gotcha
created_at: 2026-11-28T00:06:37Z
identity: ci-cd-20261128T000637-jay-flowers
tier: draft
as_of: "dewey@f19e39629dbe2929c37a90947e94b1f88efb9c0a"
---

A CI skip rule scoped to a path glob can bypass tests the glob's files are guarded by. In dewey, a gh-hud toolkit rollout rule skips build-and-test for PRs whose files are all under .opencode/, but cli_test.go holds the tracked .opencode/agents/{curator,distiller}.md mirrors byte-identical to the embedded deweyAgentDefinitions payloads in agents.go (payload equality, frontmatter shape, MCP allowlist parity). A scaffold refresh touching only .opencode/ merges with no test run and turns main red on the next push, blocking release preflight. When adding a path-scoped CI skip, check whether build-and-test is the only place an in-repo test asserts invariants over those paths, and either exclude the guarded subpaths or keep a lightweight guard job for chore PRs.
