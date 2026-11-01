---
tag: agent-orchestration
author: jay-flowers
category: pattern
created_at: 2026-11-01T18:31:10Z
identity: agent-orchestration-20261101T183110-jay-flowers
tier: draft
---

Running /uf.unleash end-to-end on opsx/mcp-internal-store-access (dewey #90/#91 combined fix) surfaced a reliable delegate-verify-recover pattern for flaky child sessions: when a uf_delegate_command child times out or 503s, the child has often already written its changes into the shared working tree before dying. Instead of re-delegating the full task, first check the tree (git diff --stat plus grep for the expected symbols and test names), then verify directly with go build/go vet/go test, and only delegate a tightly-scoped bounded completion for whatever pieces are actually missing (e.g., "add only the three missing tests"). This turned two timeouts and one 503 across phases 2-5 into zero wasted re-implementations. Two related observations: verify children's claims with piped-output exit codes masked (a `go test -v | tail` pipeline returns tail's exit code — re-run without the pipe or echo PIPESTATUS), and run `git status` + stage every new file before dispatching review-council on an uncommitted branch, because capture_review_diff only sweeps untracked files under the feature dir — new source files elsewhere (tools/persistent_storage.go) are invisible to reviewers and the manifest-delivered diff cannot compile standalone.
