---
tag: review-council
author: jay-flowers
category: decision
created_at: 2026-11-16T19:46:49Z
identity: review-council-20261116T194649-jay-flowers
tier: draft
as_of: "dewey@c065dd5b2329438999360970de26e597918508ef"
evidence:
  - "dewey:cli_test.go"
  - "dewey:agents.go"
  - "dewey#215"
---

The /uf.review-council code review for scaffold-bash-terminal-denies (dewey #215, 2026-11-16) returned APPROVE WITH ADVISORIES across five reviewers (adversary heavy tier, architect, curator, guard, testing). The notable advisory: assertDistillerFrontmatterShape in cli_test.go pins only the last-7 terminal-deny suffix plus allow presence — design D5's deliberate suffix-only pin to allow distiller evolution — so an unexpected bash entry inserted before the suffix would still pass the guard, weaker regression protection than the curator's length-exact nine-entry wantBash pin. Future changes adding distiller bash entries must consciously extend the guard rather than relying on it to catch unexpected entries.
