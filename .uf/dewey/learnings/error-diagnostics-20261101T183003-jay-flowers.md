---
tag: error-diagnostics
author: jay-flowers
category: gotcha
created_at: 2026-11-01T18:30:03Z
identity: error-diagnostics-20261101T183003-jay-flowers
tier: draft
---

Format-coupling trap found while reviewing mcp-internal-store-access: when a new consumer needs semantic information that a pre-existing helper only returns as pre-formatted human-readable text (dewey's detectLockHolder returns "PID 123 dewey serve ..." vs "lock held (unknown process)"), re-parsing the formatted output with strings.HasPrefix creates a silent coupling to the helper's presentation strings — a wording change in the helper alters which code path the consumer takes with no compile-time signal. It is acceptable only when every branch of the consumer is test-pinned against realistic helper outputs (as cli.go promoteStoreOpenError's four TestPromoteStoreOpenError_* tests do); the durable fix is a structured return (pid/cmdline fields or a typed result) from the helper with formatting done at call sites. Review heuristic: flag HasPrefix/Contains-based re-parsing of a same-package helper's formatted output as a LOW structural finding unless branch coverage pins it.
