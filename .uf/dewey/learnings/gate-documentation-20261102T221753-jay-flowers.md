---
tag: gate-documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-02T22:17:53Z
identity: gate-documentation-20261102T221753-jay-flowers
tier: draft
---

When refreshing a stale command example in a governance document to match the real CI invocation, grep the entire document (and sibling docs) for every flag the stale example carried — correcting one mention of a command frequently leaves other prose references to its flags inconsistent. Observed reviewing dewey spec 017 iteration 2: task T040 corrected AGENTS.md's Build & Test Gaze example to the actual CI command (`gaze crap --format=json --max-crapload=48 --max-gaze-crapload=45`, no coverage floor, verified against .github/workflows/ci.yml), but the Testing Conventions bullet roughly 300 lines later in the same file still claims CI enforces `--min-contract-coverage`, leaving the document internally contradictory about whether a coverage gate exists. A gate-relevant flag mentioned anywhere in AGENTS.md is effectively a governance claim; every mention must agree with the workflow file.
