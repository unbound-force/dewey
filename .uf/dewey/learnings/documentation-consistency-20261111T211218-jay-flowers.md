---
tag: documentation-consistency
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:12:18Z
identity: documentation-consistency-20261111T211218-jay-flowers
tier: draft
---

Scope \"byte-identical output\" documentation claims to the exact configurations the regression tests actually compare. Backward-compatibility claims validated only on pass paths do not cover corner refusals whose reason text legitimately changed in the same change — config-resolution failure messages are a frequent blind spot because they are rewritten when fail-closed surfaces tighten. Either write the doc claim at the strength actually tested (\"same verdict and output shape\", \"no additional lines printed\") or extend the byte-identical comparison test to the refusal corners before claiming byte-identity.
