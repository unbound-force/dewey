---
tag: declarative-config-guards
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:13:52Z
identity: declarative-config-guards-20261111T201352-jay-flowers
tier: draft
---

Guard tests that keep N derived copies of a declarative artifact consistent (tracked file ↔ embedded string literal ↔ scaffolded copy ↔ prompt text) cannot detect an identifier that is wrong in every copy simultaneously: a typo or an upstream rename leaves all copies agreeing on the stale name while the glob or rule silently denies the real one, with green CI. Close the hole by anchoring one canonical copy's identifiers against the ground-truth registry that defines them — e.g., dewey PR #195's assertCuratorFrontmatterShape checks every frontmatter allowlisted tool name against registeredMCPToolNames(t), which parses the tool names registered in server.go, so an inert allow entry fails the test even when frontmatter, prompt body, and command copies all agree.
