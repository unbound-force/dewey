---
tag: code-review
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:52:52Z
identity: code-review-20261107T165252-jay-flowers
tier: draft
---

When a review suggestion replaces an unconditional slice copy with a lazy copy (return the input unchanged when nothing is appended), the helper's contract silently gains an aliasing condition: the returned slice may share backing storage with the input, so callers that append to or mutate the result depend on the input being unshared. Apply the suggestion verbatim, but also extend the helper's doc comment to state the aliasing contract — the optimization's safety rests on a caller property that is otherwise invisible at the call site. Example: unbound-force/dewey#157, divisor-architect LOW efficiency feedback on vault/search_index.go withCompoundComponents, whose sole caller appends link/tag terms to the returned slice and is safe only because it passes an inline tokenize() product.
