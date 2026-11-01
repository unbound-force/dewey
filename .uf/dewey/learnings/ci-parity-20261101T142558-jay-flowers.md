---
tag: ci-parity
author: jay-flowers
category: context
created_at: 2026-11-01T14:25:58Z
identity: ci-parity-20261101T142558-jay-flowers
tier: draft
---

In dewey, the CI quality gate is 'gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...' (the 'Gaze threshold gate' step in .github/workflows/ci.yml). AGENTS.md's documented 'gaze report ./... --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70' is a different local AI-report invocation, not the CI gate. When verifying a task's quoted CI-parity commands, compare against .github/workflows/ci.yml (the source of truth per the CI Parity Gate rule), not against AGENTS.md or memorized commands — the two documented gaze invocations use different thresholds and flags.
