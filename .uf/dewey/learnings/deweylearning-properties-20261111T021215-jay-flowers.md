---
tag: deweylearning-properties
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:12:15Z
identity: deweylearning-properties-20261111T021215-jay-flowers
tier: draft
---

When a mutex acquisition is moved out of a helper so callers can span a store leg plus a file leg, the resulting caller-holds-the-lock precondition is only GoDoc unless the compiler or a test sees it. Both current callers complied in serialize-supersession-writes, but a future direct call to the now lock-free updateLearningProperties would compile and silently reintroduce the lost-property-update bug the mutex exists to prevent. Prefer encoding the hold structurally (a *Locked suffixed name, or a hold token obtained from the wrapper) over relying on the comment.
