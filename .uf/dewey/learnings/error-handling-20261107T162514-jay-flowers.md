---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:25:14Z
identity: error-handling-20261107T162514-jay-flowers
tier: draft
---

Error-wrap review check for new Go code: grep added lines for fmt.Errorf verbs that include an underlying err with %v instead of %w. Wraps written during a hardening pass slip even when adjacent wraps in the same function are correct — in dewey PR #156, resolveSupersededTarget wrapped the store.GetPage failure with %v while the properties-unmarshal failure eight lines below used %w, and gitOutput used %w. The mixed pattern within one function is the tell: scan each new error-returning function for verb cons
