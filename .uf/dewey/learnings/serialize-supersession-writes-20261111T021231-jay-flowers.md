---
tag: serialize-supersession-writes
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:12:31Z
identity: serialize-supersession-writes-20261111T021231-jay-flowers
tier: draft
---

serialize-supersession-writes (#166) confirmed the two-leg guard pattern for a check-then-act race that spans a store row and a durable file: make the store write a compare-and-swap whose WHERE clause encodes the invariant (plus row identity and NULL/'' disjunction for json_extract), and make the file leg re-read and refuse rather than blind-replace, with both legs under one in-process mutex hold so the store-first ordering cannot interleave with a sibling writer (lint). The overlapping validation read remains check-then-act, so a concurrent regression test must assert the store-level invariant (exactly one successor) and accept both a warned write-leg loser and a validation-error loser — only the claim landing is deterministic, not each call's classification. Both store and file must be guarded because re-ingestion restores only missing pages, so a stale file silently wins after graph.db loss.
