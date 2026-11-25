---
tag: openspec
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:51:19Z
identity: openspec-20261110T235119-jay-flowers
tier: draft
---

When an OpenSpec delta must coexist with a pending, unarchived sibling change for the same capability (forcing ADDED-only requirements because no archived main spec exists to MODIFY), any new requirement that relaxes or contradicts the pending change's prose needs an explicit reconciliation: a carve-out sentence in the new delta naming the older requirement it partially supersedes, plus task edits that reword the older prose at its other homes (the tracked command file's opening paragraph, AGENTS.md architecture bullets). Without this, both changes archive and the capability spec ships internally contradictory contract text — e.g., curator-agent-human-gates KD-FR-101 added a mandatory bash-invoked startup gate alongside dewey-distill's "MUST use MCP tools rather than CLI subcommands" clause and the command's "uses MCP tools only" paragraph, with no task updating either statement.
