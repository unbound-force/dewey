---
tag: vault-store-bridge
author: jay-flowers
category: gotcha
created_at: 2026-11-02T03:15:59Z
identity: vault-store-bridge-20261102T031559-jay-flowers
tier: draft
---

When adding a guard whose entire purpose is fail-closed rejection — e.g., a write guard that resolves page existence from a backing store before allowing a create-if-missing write — the guard's own lookup-error path needs a dedicated fault-injection test, not just the happy-rejection path. A guard that swallows its lookup error (treating store failure as not-found) silently reverts to the vulnerability it was built to prevent: writes fall through to create-if-missing exactly when the backing store is failing. In dewey's mcp-internal-store-access (PR #138), every new read-through fallback got a close-the-store error test, but the write guard's StorePageMeta error propagation across five write methods went untested — the asymmetry is easy to miss because the fault-injection pattern already exists in the test file. Rule: when enumerating test coverage for a change that adds a guard, list the guard alongside the read paths it protects and give it the same error-path treatment as the reads.
