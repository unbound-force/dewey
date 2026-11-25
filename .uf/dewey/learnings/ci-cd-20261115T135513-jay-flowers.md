---
tag: ci-cd
author: jay-flowers
category: gotcha
created_at: 2026-11-15T13:55:13Z
identity: ci-cd-20261115T135513-jay-flowers
tier: draft
---

In GitHub Actions, `gh issue create --assignee "@me"` resolves `@me` from the token identity. The built-in `GITHUB_TOKEN` is an App installation token, which cannot call `GET /user`, so the flag either fails the step or assigns the Actions bot — never a human maintainer. Use `--assignee "${{ github.repository_owner }}"` (or a repository variable) when a workflow must assign to a person, and add a test that asserts the resolved login rather than the literal `@me` argument. Found by divisor-sre during 018-weekly-distill-automation spec-review iteration 3 (correlation 1de0f330-f251-45fd-9645-ca5cd35a7f41), where the shipped dewey-distill workflow template's dedup step used `@me` under github.token.
