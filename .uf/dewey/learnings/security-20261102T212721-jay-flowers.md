---
tag: security
author: jay-flowers
category: gotcha
created_at: 2026-11-02T21:27:21Z
identity: security-20261102T212721-jay-flowers
tier: draft
---

When a config validator rejects an input because it contains credentials (for example an endpoint URL with embedded userinfo), the rejection message must not echo the raw input. Detecting and rejecting `user:pass@host` only helps if the error, log, and doctor output render a sanitized form (scheme+host+path, or a placeholder); otherwise the credential lands in stderr, CI logs, and tool error responses — the exact surfaces the security contract forbids. Audit both the validation error and every diagnostic that prints the same config field.
