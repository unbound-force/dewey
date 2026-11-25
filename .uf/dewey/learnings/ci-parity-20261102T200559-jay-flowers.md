---
tag: ci-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:59Z
identity: ci-parity-20261102T200559-jay-flowers
tier: draft
---

In dewey, .github/workflows/ci.yml is the source of truth for Gaze gate thresholds: `gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...` with no coverage floor. AGENTS.md's Build & Test local example (--max-gaze-crapload=18 --min-contract-coverage=70) is stale relative to the workflow. When a spec artifact cites gate thresholds or coverage targets, verify them against the workflow file, not AGENTS.md — and beware cross-references that a same-changeset doc-refresh task (like 017's T040) will invalidate.
