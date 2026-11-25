---
tag: quickstart-validation
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:52Z
identity: quickstart-validation-20261102T200552-jay-flowers
tier: draft
---

A quickstart scenario's expected log output is only valid if the invoked command actually executes that code path — verify by tracing the command to the construction site before writing the expectation. In dewey, `dewey status` only queries store counts (newStatusCmd reads the SQLite store and greps config for the model name); provider construction and degradation logging live in the serve path and `createIndexEmbedder`. A quickstart section that expects provider-degradation logs from `dewey status` validates nothing — the correct command is one that builds the embedder (`dewey index`, `dewey doctor`, or serve).
