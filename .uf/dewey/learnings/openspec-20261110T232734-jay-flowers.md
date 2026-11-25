---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:34Z
identity: openspec-20261110T232734-jay-flowers
tier: draft
---

An OpenSpec proposal that lists a capability under "Modified Capabilities" must ship that capability's MODIFIED delta in the same change (meta-prompt pack MP-012: one specs/<capability>/spec.md delta per capability touched). In repos like dewey where un-archived change dirs are the de facto capability registry (openspec/specs/ holds only a few capabilities; agents retrieve requirement text from changes/ via Dewey), a declaration without a delta leaves the canonical requirement text stale after the behavior changes. The delta is substantive, not ceremonial, whenever the modification makes an existing MUST conditionally false — e.g. learning-temporal-validity's "the target MUST gain superseded_by (the new identity)" no longer holds for a concurrent losing writer under serialize-supersession-writes' claim semantics. Repo precedent for both directions: mcp-internal-store-access answers "Modified Capabilities: None (no openspec/specs/ capability exists yet...)" when establishing a capability; doctor-vertex-embedding shows the numbering-header + FR-id + delta form for real modifications.
