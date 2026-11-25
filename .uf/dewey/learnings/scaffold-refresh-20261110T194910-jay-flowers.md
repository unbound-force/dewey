---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:49:10Z
identity: scaffold-refresh-20261110T194910-jay-flowers
tier: draft
---

Scaffold-refresh PRs that bump an artifact schema version and add a `schemas/<name>/<version>.schema.json` reference in command prose must also deploy that schema file into the consuming repo. After dewey PR #177 bumped feedback-triage to 1.1.0 and referenced `schemas/feedback-triage/v1.1.0.schema.json`, `.uf/schemas/` still had no feedback-triage/ directory — the new v1.1.0 fields (decision enum file/reply, filed_issue_number/url, filed/replied counts) were referenced but unvalidatable locally. Checkout-time check for uf-init refresh reviews: glob each schema path the refreshed commands reference against the deploying repository, not just against the upstream source of the assets.
