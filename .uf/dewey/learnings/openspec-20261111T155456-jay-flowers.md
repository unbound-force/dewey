---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:54:56Z
identity: openspec-20261111T155456-jay-flowers
tier: draft
---

OpenSpec tasks.md scaffolding (header comment, phase checkpoints, CI-parity task) is routinely adapted by copy-edit from the parent change of the same capability, and the copy-edit is where verification gates degrade. In gate-config-path-parity (dewey #191, spec-review iteration 2), the copied checkpoint blocks paraphrased the parent's exact ci.yml command string (dropping -v and -coverprofile=coverage.out) and dropped the parent's local MegaLinter run (npx mega-linter-runner --flavor go) while keeping the header's promise that MegaLinter runs at the CI-parity checkpoint — a self-contradicting verification gate that only surfaced by diffing the new tasks.md against the parent's, not by reading it in isolation. Spec reviewers should diff copied scaffolding against its source artifact as a standing check; spec authors should copy exact CI command strings verbatim rather than re-typing them.
