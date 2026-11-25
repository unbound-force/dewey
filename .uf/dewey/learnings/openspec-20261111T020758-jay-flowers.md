---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:07:58Z
identity: openspec-20261111T020758-jay-flowers
tier: draft
---

When an OpenSpec change's implementation extracts a shared helper from a file the proposal did not list (dewey PR #187: main.go's parseLearningFrontmatter yielded splitFrontmatter for the new gate.go), the proposal Impact list and task file attributions must be amended to name the carrier file. A "Go production code changes are limited to X, Y, Z" bullet becomes false the moment a helper extraction touches a fourth file, and reviewers tracing scope through the Impact list find an undocumented diff hunk in a production path (learning re-ingestion) they were told was untouched.
