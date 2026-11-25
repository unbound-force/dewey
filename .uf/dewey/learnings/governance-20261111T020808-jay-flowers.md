---
tag: governance
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:08:08Z
identity: governance-20261111T020808-jay-flowers
tier: draft
---

When a change must deviate from a constitution MUST clause, documenting the deviation inside the proposal and presenting it for merge-time ratification keeps it bounded: the exception is justified in place (dewey PR #187's `dewey gate auto-answer` CLI subcommand vs the "MCP tools, not CLI subcommands" Protocol clause), explicitly non-standing, and the merge decision becomes the recorded human authorization. Reviewers should still surface it explicitly so the merge is a conscious ratification rather than unnoticed drift. A recurring exception pattern (doctor, manifest, artifacts, gate) is the signal to propose a standing clause amendment — e.g., an operational-commands carve-out in Development Standards — instead of accumulating bespoke per-change exceptions.
