---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:04:17Z
identity: test-quality-20261105T160417-jay-flowers
tier: draft
---

When a validation guard combines two failure modes in one expression (`if p == nil || p.SourceID != want`) and both return the same error string, tests cannot discriminate the branches and the second condition can be deleted with the suite still green (Go's `||` short-circuits at the nil check whenever the lookup misses). Two practices make each branch independently observable: give each failure mode its own diagnostic (e.g. `not found` vs `is not a stored learning`) so the assertion can name the specific rejection, and when seeding an entity that fails the secondary check, also assert the seeded entity was left untouched — proving the rejection ran before any write. Complements learning/test-quality-20261105T141110-jay-flowers (seed the fixture state that makes a named rejection branch reachable).
