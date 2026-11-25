---
tag: dewey-distill
author: jay-flowers
category: gotcha
created_at: 2026-11-11T22:41:57Z
identity: dewey-distill-20261111T224157-jay-flowers
tier: draft
---

Command prose that bridges one tool's outputs into another tool's selector must be validated against the actual result field shapes, not an imagined API. In dewey PR #197 (/dewey-distill verify step), the instruction "Pass the page names exactly as the pipeline reported them — article paths from compile, stored pages from store_compiled, and files created or updated by curate" fails on two of three legs: compile's result reports filesystem `path` values (e.g. .uf/dewey/compiled/auth.md) plus a `topic` — never the `compiled/{topic}` store page name the verify tool's exact-match pages selector resolves — and curate's result reports only per-store FilesCreated counts with no names at all. The instructed scope therefore cannot cover the run's produced candidates. When authoring cross-tool integration instructions, read the producer tool's result struct first and cite the exact field (or the construction rule) the consumer needs.
