---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:25:26Z
identity: test-quality-20261107T162526-jay-flowers
tier: draft
---

When a constructor gains a default resolver that probes the environment (e.g., git rev-parse against the vault path), every pre-existing test constructing the handler inherits the subprocess call unless it explicitly overrides the resolver. Empty-path early returns only protect tests that pass an empty path. To keep unit tests hermetic, either default the resolver to nil and wire the production resolver explicitly at the serve/call site, or add With<Resolver>(nil) to existing test constructions. Symptom to 
