---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:04:15Z
identity: test-quality-20261105T160415-jay-flowers
tier: draft
---

Go short-circuit collapses table-driven rejection cases: in `if a == nil || a.Attr != want`, a "wrong attribute" case run against an empty store exits through the nil branch and the attribute check never executes — the case's label then overstates coverage. Two defenses when writing or reviewing such tests: (1) seed the discriminator fixture so each named branch is reachable (on dewey PR #156, s.InsertPage of a learning/-namespaced page with SourceID "disk" made resolveSupersededTarget's SourceID rejection reachable; ListLearningPages filtering source_id='learning' kept the shared table's zero-pages assertion valid), and (2) verify by mutation — temporarily delete the guarded condition and confirm a test fails; a named case that survives deletion is false coverage signal, not protection.
