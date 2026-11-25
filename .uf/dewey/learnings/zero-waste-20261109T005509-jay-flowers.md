---
tag: zero-waste
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:55:09Z
identity: zero-waste-20261109T005509-jay-flowers
tier: draft
---

When a new change needs the same store+backup write sequence as a recently-landed sibling flow in the same package, extract the shared mechanics in the same change instead of copying the sequence. Tell-tale: identical inline comments across files (PR #159's markStaleUnverified/markStaleFile copied #146's linkSuperseded/markSupersededFile flow including the comment 'a map of JSON-decoded values always marshals'). The copies diverge immediately — the new side flipped the failure contract from structured warning (supersedes_warning) to error and hardened the fail-closed ordering only on its own side, leaving two drifting implementations of a data-write path. Remedy shape: one helper owns the mechanics (decode properties into a non-nil map → mutate → UpdatePage → backup frontmatter write via setFrontmatterKeys) with a single documented partial-failure contract and fail-closed validation before any write; each caller maps the contract to its tool-level semantics (warning string vs logged error), preserving both specs' observable behavior.
