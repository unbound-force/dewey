---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-01T14:25:56Z
identity: spec-review-20261101T142556-jay-flowers
tier: draft
---

When a design Non-Goal defers work claiming it is 'noted in tasks as follow-up', verify the note actually exists in tasks.md — the cross-reference can be false. In dewey's mcp-internal-store-access change (reviewed 2026-11-01), design.md deferred the query_properties/FindByProperty store merge citing a tasks follow-up note that tasks.md never contained. The result: list_pages(has_property=...) became store-aware via the GetAllPages merge while query_properties stayed store-blind — two property-query surfaces diverging after a change whose purpose was unifying tool visibility of the store.
