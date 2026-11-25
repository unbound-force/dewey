---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:10:06Z
identity: openspec-20261110T231006-jay-flowers
tier: draft
---

An OpenSpec change spanning a new capability and an existing one can accidentally give two capabilities ownership of the same command contract. Designate one owner (e.g., the distillation capability owns command binding/gates; the agent capability owns identity, tool boundary, and marker token) and cross-reference rather than restating, or the deltas drift and later changes update one side only.
