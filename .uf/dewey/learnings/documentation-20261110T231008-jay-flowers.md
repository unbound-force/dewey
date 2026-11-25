---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:10:08Z
identity: documentation-20261110T231008-jay-flowers
tier: draft
---

When a change defines a contract consumed by sibling or external repositories (for example a question marker that an auto-answer plugin must honor), the contract grammar must be scheduled for publication in public documentation. Defining it only in an OpenSpec design artifact is insufficient because design.md moves under openspec/changes/archive/ after archive, leaving external implementers without a stable public reference.
