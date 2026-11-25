---
tag: governance-exception-provenance
author: jay-flowers
category: pattern
created_at: 2026-11-12T12:23:55Z
identity: governance-exception-provenance-20261112T122355-jay-flowers
tier: draft
---

A human-approved governance exception recorded inside a spec artifact should carry a falsifiable provenance pointer — the recorded decision mechanism's artifact location (dispatch-history directory, correlation ids, checkpoint keys) — so a reviewer can verify the authorization exists rather than trusting the claim. In dewey's explicit-supersession-in-compile, the Tier Exception section (new MCP tool on an OpenSpec change, against the AGENTS.md "new MCP tools route Speckit" table) cited the /uf.run-pipeline-plan pause-relay and pointed at .uf/artifacts/dispatch/ with preserved correlation ids; the Guard review then confirmed the dispatch artifacts exist for the branch and that the curator-allowlist extension (a Gatekeeping Value Protection item-5 surface) is both documented in the task and mechanically enforced by an existing cli_test.go assertion that derives its candidate set from the server registry. The pattern that made verification cheap: exception claim + named decision mechanism + on-disk record + an automated test that fails if the protected surface moves without the three copies moving together.
