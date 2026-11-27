---
tag: allowlist-guards
author: jay-flowers
category: pattern
created_at: 2026-11-27T21:00:30Z
identity: allowlist-guards-20261127T210030-jay-flowers
tier: draft
as_of: "dewey@f77093acbbe0468d0a190f8705f89f51eec3b341"
---

When removing an allowed command from a scaffolded agent's permission.bash map, strengthen the frontmatter shape guard with a negative assertion — fail if any key outside the intended allow set carries "allow" — rather than only deleting the entry from the expected list. dewey's assertDistillerFrontmatterShape (PR #286) does this: wantAllowed lists exactly the two startup gates and a second loop fails on any other allowed key, so the removed "gh issue comment" allow cannot silently reappear in either shipped copy (tracked file or agents.go embedded mirror).
