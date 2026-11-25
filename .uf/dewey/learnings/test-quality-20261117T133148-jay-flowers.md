---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-17T13:31:48Z
identity: test-quality-20261117T133148-jay-flowers
tier: draft
as_of: "dewey@588f9b8098461007f176527ccc683e8d643f76cb"
evidence:
  - "dewey#220"
  - ".uf/artifacts/dispatch/3ecf4c63-7eb3-409b-ac15-1f5552fb863e.json"
---

Executed-but-not-asserted coverage trap: when a change extends the contract of a multi-branch upserter (insert / content-changed update / unchanged-backfill), pre-existing branch tests keep executing the new line — branch coverage stays green — while no assertion pins the new behavior on that branch. Enumerate every write path and assert the new field on each. Seen in dewey PR #220 (knowledge-store properties fix): the insert and backfill paths of IndexKnowledgeStore got dedicated assertions via the pageValidity consumer seam, but the content-changed update path (existing.Properties = propsJSON) was only executed by TestAutoIndex_UpdatedContentReindexed, which asserts neither properties nor validity. The content-changed branch is the steady-state path for curation-rewritten files on every server restart, so a regression there would silently reproduce the exact defect the change fixes (stale validity metadata invisible to semantic_search filtering). Rule of thumb: when a delta spec's scenarios cover a subset of a requirement's paths, the uncovered paths still need tests — the requirement text ("MUST be stored ... when indexed") is the contract, not the scenario list.
