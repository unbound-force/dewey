---
tag: ci-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:54:52Z
identity: ci-parity-20261111T155452-jay-flowers
tier: draft
---

Dewey OpenSpec tasks.md checkpoints that claim "exact ci.yml Build/Vet/Test steps" must quote .github/workflows/ci.yml verbatim — `go build -v ./...`, `go vet ./...`, `go test -race -v -count=1 -coverprofile=coverage.out ./...` — because the CI Gaze threshold gate (`gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...`) consumes the coverage.out profile that only the test step's -coverprofile flag produces. A paraphrased `go test -race -count=1 ./...` checkpoint leaves a deferred CI-parity Gaze task without its coverage input. Sibling changes (curator-agent-human-gates, dewey-publish, mcp-internal-store-access) quote the commands verbatim for exactly this reason; spec-review iteration should diff claimed-exact checkpoint commands against the workflow file.
