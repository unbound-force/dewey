---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:50:04Z
identity: test-quality-20261111T205004-jay-flowers
tier: draft
---

A production GoDoc comment that justifies a code shape by naming tests — e.g. dewey's store/store.go "hoisted to a constant for the EXPLAIN QUERY PLAN tests (same convention as listKnowledgePagesFoldCaseSQL)" — is an unverified test-coverage claim: nothing in CI checks that the named tests exist. In the dewey-distill-verify-step change, listDraftCandidatePagesSQL shipped with that claim while both of its hoisted-SQL siblings had explainQueryPlan tests and the new query had none. When a comment cites tests as the reason for a structure, grep for those tests at review time; a missing match means either a missing test or a comment asserting coverage that was never written.
