---
tag: subprocess-context
author: jay-flowers
category: decision
created_at: 2026-11-06T17:23:09Z
identity: subprocess-context-20261106T172309-jay-flowers
tier: draft
---

Dewey PR #156 feedback triage (tools/learning_temporal.go gitOutput): a git subprocess helper running under context.WithTimeout(context.Background(), 2s) with defer cancel() and exec.CommandContext(...).Output() was flagged to thread the caller's context; the deferral was assessed sound on verified grounds: (1) the fixed 2s timeout already bounds worst-case subprocess life past caller cancellation to two seconds, and Output() waits so the process is reaped — no leak; (2) failure only omits the non-fatal as_of field, the same path as any git failure, with tested failure branches; (3) partial context threading is incoherent when the surrounding handler's durable writes (writeLearningFile takes no context) are not cancellation-gated — cancelling the cheapest, most harmless step while the writes proceed misleads more than it saves; (4) threading required widening the injected resolver type func() (string, error) to func(context.Context) (string, error) across the handler field, WithAsOfResolver, productionAsOfResolver, resolveAsOf, and every test double. Rule of thumb: for time-bounded, non-critical subprocess collaborators whose failure is non-fatal, a local fixed timeout under context.Background() is acceptable — thread caller context when the resolver signature is widened anyway (dewey #106, multi-repo pinning). Verification corollary: check deferral rationales against the diff before relying on them — reIngestLearnings restores as_of from file frontmatter and never invokes the resolver, so the claimed startup caller without a request context did not exist; the only invocation (StoreLearning) does have a request context, and the deferral still stands on the bounded-cost, coherent-cancellation, and signature-churn grounds.
