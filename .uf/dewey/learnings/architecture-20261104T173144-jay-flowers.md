---
tag: architecture
author: jay-flowers
category: gotcha
created_at: 2026-11-04T17:31:44Z
identity: architecture-20261104T173144-jay-flowers
tier: draft
---

When a package-leaf constraint forces duplicating a policy helper into a second package, replicate the helper's semantics exactly (with a parity test) rather than reimplementing a shorthand version. In dewey PR 145, llm.IsLoopbackHost was recreated as a case-sensitive string matcher while openaicompat.IsLoopbackHost resolves via net.ParseIP(...).IsLoopback(); both feed dewey doctor, so the same endpoint host can produce contradictory cleartext-key warnings across the embedding and synthesis sections (http://LOCALHOST warns only in synthesis). A leaf rule that forbids sharing the symbol does not forbid matching the behavior — stdlib-only parity is always available.
