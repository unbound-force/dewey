---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-26T21:42:21Z
identity: test-quality-20261126T214221-jay-flowers
tier: draft
as_of: "dewey@0504c191b50b8c9ec7e7609bcab67e6cbbc6fb63"
---

When golangci-lint reports SA1019 (use of a deprecated field) on a test whose purpose is to cover that deprecated field's backward-compat contract, the correct fix is a line-scoped `//nolint:staticcheck // SA1019: <reason>` directive on the exact usage line — nolint directives take golangci-lint linter names, not check ids, so `staticcheck` is the suppression target for SAxxxx findings. Keep the tests: deleting them or dropping the field coverage removes regression protection for the documented compat contract (the store_learning `tags`-plural fallback is a documented dewey contract), and excluding SA1019 in .golangci.yml would suppress the check repo-wide while touching protected linter configuration. Before applying, grep the repo for other usages of the deprecated identifier to confirm the deliberate-coverage set is complete — in dewey PR #283, tools/learning_test.go:411 and :466 were the only two StoreLearningInput.Tags usages, so two line-scoped directives fully resolved the lint failure with zero collateral suppression.
