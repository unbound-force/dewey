---
tag: issue-filing
author: jay-flowers
category: pattern
created_at: 2026-11-02T20:37:37Z
identity: issue-filing-20261102T203737-jay-flowers
tier: draft
---

Issue-filing tasks in spec tasks.md should carry the duplicate-check step, not just the create command. Prepend to each `gh issue create` task: "search existing open issues first (`gh issue list --repo <repo> --search <keyword> --state open`) and reference a matching issue instead of filing a duplicate." Filings deferred to implementation time otherwise depend on implementer memory for duplicate hygiene. Precedent: unbound-force/website#113 already tracks dewey provider documentation sync from spec 016, so a new provider-feature filing task without a search step risks overlap.
