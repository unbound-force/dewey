---
tag: contract-enforcement
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:34:36Z
identity: contract-enforcement-20261103T183436-jay-flowers
tier: draft
---

Spec and contract artifacts that delegate a MUST to an implicit runtime default ("net/http's default policy strips Authorization on cross-host redirects") inherit the default's exact semantics — including the cases it silently does not cover — while claiming the broader guarantee. When a contract states a credential-handling MUST over a shared transport, enforce it explicitly in the transport (e.g. an http.Client.CheckRedirect comparing destination host AND scheme against via[0], following the codebase's existing host-comparison convention such as FR-007's hostname-with-port-stripped, case-insensitive, subdomains-do-not-match rule), and write the artifact sentence to describe the enforced policy, not the assumed default. The regression test must include the cases the default does not already handle — a redirect test using only unrelated hosts passes under the default and verifies the default, not the contract. Observed in dewey PR #144 (spec 017): research.md R8, the wire contract's Redirects section, and plan.md D7 all overstated stdlib stripping, and the sole redirect test exercised first.example→second.example, the one case stdlib strips.
