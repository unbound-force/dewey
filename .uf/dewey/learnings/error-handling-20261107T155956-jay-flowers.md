---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:59:56Z
identity: error-handling-20261107T155956-jay-flowers
tier: draft
---

Before reporting "missing context in error wrap" against an outer wrapper, trace the full wrap chain to the innermost store-layer wrap. In dewey, GetStorePage wraps errors with the caller-requested name (`look up store page %q`), but the store layer's getPageWhere wraps with the exact key it actually queried (`get page %q`, store/store.go) — so a failed prefixed probe (e.g. compiled/gateway tried for a bare request "gateway") surfaces as `look up store page "gateway": get page "compiled/gateway": ...`, whic
