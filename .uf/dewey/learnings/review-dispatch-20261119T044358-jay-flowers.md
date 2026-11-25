---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-19T04:43:58Z
identity: review-dispatch-20261119T044358-jay-flowers
tier: draft
as_of: "dewey@377610524098d68d9f685178d7f173f0a5558585"
---

When dispatched as a Divisor reviewer for a PR, the working tree may be checked out at the BASE ref rather than the PR head — cli.go and main.go lacked the diff's changes when read directly. Grep/read results therefore reflect base state: line numbers, helper presence, and test inventories are still valid for shared context (helpers the diff reuses, established patterns), but head-state content must be taken from the inline diff itself, and head line numbers must be computed from the diff hunk offsets (+start,count), not from file reads. Verify tree state before attributing read content to the PR.
