---
tag: documentation-sync
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:56:48Z
identity: documentation-sync-20261107T155648-jay-flowers
tier: draft
---

Dewey changes that modify user-facing MCP tool behavior require a docs sync issue in unbound-force/website (AGENTS.md 'Website Documentation Sync', constitution Development Workflow) — an in-repo README update does not satisfy it. Curator reviews verify via `gh issue list --repo unbound-force/website --label docs --state open` (the per-change sync series — website#41, #113, #132, #355, #361, #371, #380 — makes a missing entry visible) and put a ready-to-run `gh issue create` command in the finding, because 
