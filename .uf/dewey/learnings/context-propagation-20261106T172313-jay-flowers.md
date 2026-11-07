---
tag: context-propagation
author: jay-flowers
category: decision
created_at: 2026-11-06T17:23:13Z
identity: context-propagation-20261106T172313-jay-flowers
tier: draft
---

Artifact-provenance stamping at store time should not be bound to the requester's connection lifetime. When a subprocess enriches an artifact whose write is not itself context-bound (dewey store_learning's as_of git pin, and resolveAuthor before it), threading the request context means a client disconnect still completes the store write but silently drops the provenance pin — the enrichment dies with the context while the artifact persists without it. For non-fatal, wall-clock-bounded enrichment (2-second timeout, failure merely omits the field), context.Background() with its own timeout is the defensible design, mirroring the pre-existing spec-pinned pattern; reserve request-context propagation for work whose result is returned to the requester. Widen the resolver signature (func() (string, error) -> func(ctx context.Context) (string, error)) only when a change reworks the resolver for its own reasons — dewey PR #156 deferred this to #106 multi-repo pinning — because the widening ripples through the exported WithAsOfResolver setter, the production resolver, prepareTemporal, gitOutput, and every test double (fixedAsOf plus inline doubles in tools and main tests, ~15 touchpoints). Folded-in gotcha: verify claimed contextless call paths against the code before citing them in review replies — dewey's reIngestLearnings restores as_of from frontmatter and never invokes the resolver, so the "Background fallback would remain" argument was posted against a call path that does not exist.
