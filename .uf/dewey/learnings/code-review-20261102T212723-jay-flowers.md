---
tag: code-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T21:27:23Z
identity: code-review-20261102T212723-jay-flowers
tier: draft
---

When adding a new leaf/transport package, run an explicit post-implementation sweep for exported symbols with zero production callers before declaring the package done. In Go, `staticcheck`/`unused` do not flag exported identifiers in non-main packages, and an internal (`package foo`) test file can reach unexported fields directly — so accessors and 'test-only' exported config fields added as test seams become invisible orphaned API. Dewey's Zero-Waste Mandate and go pack CS-002 make those review findings. Example: openaicompat shipped Client.Capability/BaseURL/EndpointField/HasAPIKey/Timeout/Redact plus Config.Timeout, none used in production, one with a GoDoc claiming a caller that did not exist.
