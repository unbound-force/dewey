---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:34:48Z
identity: test-quality-20261103T183448-jay-flowers
tier: draft
---

Go redirect-policy tests cannot rely on httptest loopback servers for the redirect shapes that expose header-forwarding gaps: subdomain redirects (api.gateway.example.com after gateway.example.com) and same-host scheme downgrades (https→http) are unreachable on loopback, where every server is a distinct host:port over http. Use a stub http.RoundTripper keyed on req.URL.Host instead — return the redirect Location for the initial host, capture the redirected request's Authorization header, and any destination URL can be synthesized with no DNS or network (dewey's openaicompat.captureTransport, PR #144). Pair it with the fix-side precedent already in the repo: source/web.go sets an explicit http.Client.CheckRedirect and source/web_test.go verifies the policy function directly. Discipline when testing stdlib-default behavior: enumerate test scenarios from the requirement's own wording (spec 017: "Authorization MUST NOT be forwarded across hosts"), not from the default policy's documented happy case — the PR's single cross-domain redirect test passed while the spec MUST was violated in every untested redirect class.
