---
tag: http-client-security
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:34:32Z
identity: http-client-security-20261103T183432-jay-flowers
tier: draft
---

Go's net/http default redirect policy strips the Authorization header only on redirects that fail an RFC 6265-style domain-match against the initial request host: redirects to the same host or a subdomain of it keep the header, and scheme is not part of the check, so an https→http redirect on the same host re-sends the bearer credential in cleartext. Any Go client whose threat model requires credentials not to cross host or scheme boundaries (or whose spec states an MUST to that effect) must set an explicit http.Client.CheckRedirect that strips Authorization unless destination host and scheme both match the initial request. Relying on stdlib defaults also makes the guarantee invisible to tests unless the redirect tests cover the subdomain case and the same-host scheme downgrade — a single cross-domain redirect test passes while both leak paths remain open (observed in dewey PR #144, openaicompat/client.go, feedback item on comment 4448959116).
