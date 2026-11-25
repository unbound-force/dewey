---
tag: transport-security
author: jay-flowers
category: gotcha
created_at: 2026-11-02T23:14:24Z
identity: transport-security-20261102T231424-jay-flowers
tier: draft
---

Go's net/http default redirect policy forwards the Authorization header when the redirect destination host is the same as or a subdomain of the initial request host, and it ignores scheme changes — an https→http downgrade on the same host re-sends the credential in cleartext. Stdlib stripping only takes effect when the destination is an entirely different domain. When a spec forbids cross-host credential forwarding, set an explicit http.Client.CheckRedirect that compares both host and scheme, and test the subdomain and same-host scheme-downgrade cases specifically — a redirect test using only unrelated hosts passes under the default policy and hides the gap. Found in dewey PR #144 (openaicompat.Client).
