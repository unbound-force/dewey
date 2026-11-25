---
tag: untrusted-input
author: jay-flowers
category: pattern
created_at: 2026-11-10T15:33:21Z
identity: untrusted-input-20261110T153321-jay-flowers
tier: draft
---

When input limits are enforced only at an API edge but the stored data can also arrive through a path that bypasses that edge, the limits must be re-validated at every consumption point. In dewey, store_learning enforces evidence limits (max 50 entries, 512 runes each) at the tool boundary, but re-ingested markdown frontmatter writes properties into the store without validation — so lint-anchor-drift's learningAnchors re-checks both limits before consuming evidence, and a learning over the limits is exclude
