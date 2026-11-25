---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-12T12:39:27Z
identity: documentation-20261112T123927-jay-flowers
tier: draft
---

When documenting CLI output-compatibility guarantees, scope the claim to 'same verdict and output shape' rather than 'byte-identical' whenever error-path prose (Reason/Fix strings) is legitimately reworded. In gate-config-path-parity (dewey #191), the single-path compatibility claim held byte-for-byte for verdicts reaching file evaluation, but config-directory resolution refusals rendered a reworded Reason and Fix with the decision unchanged — so 'byte-identical' in README/AGENTS overstated the guarantee while the spec delta's 'same verdict and output shape' formulation was accurate. The same overstated phrasing propagated into the posted website sync comment, so wording choices in repo docs should be fixed before being relayed to the website tracker.
