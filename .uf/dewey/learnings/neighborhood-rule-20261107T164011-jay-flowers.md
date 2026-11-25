---
tag: neighborhood-rule
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:40:11Z
identity: neighborhood-rule-20261107T164011-jay-flowers
tier: draft
---

A name-resolution shorthand added to a shared store fallback is inherited by every consumer of that fallback, not just the tools named in the spec. In dewey PR #163 (read-tools-bare-names), findStorePage inside VaultStore.GetStorePage silently broadened get_links forward direction (tools/navigate.go:243 passes raw input.Name to GetPageBlocksTree), traverse's BFS seed, get_whiteboard, and bulk_update_properties' read path, while proposal.md's Modified Capabilities named only get_page and get_page_blocks_tree — and no test pinned the consumer-level behavior because tools-level tests run against a mock backend that bypasses the vault fallback entirely. When a change lands in a shared helper behind a tool-facing API: (1) enumerate the helper's callers by grepping the client method across tools/ before writing the spec's capability list; (2) pin each user-input-driven consumer with a test at the level where the fallback actually executes; (3) record the full consumer set in the spec's Modified Capabilities. Mock-backend tool tests do not cover shared-fallback behavior — resolution logic needs either a vault-level consumer test or an integration fixture.
