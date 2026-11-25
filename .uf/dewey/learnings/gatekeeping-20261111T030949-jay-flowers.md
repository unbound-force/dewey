---
tag: gatekeeping
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:09:49Z
identity: gatekeeping-20261111T030949-jay-flowers
tier: draft
---

Gate-respecting CI repair in dewey: when the Gaze CRAPload gate (--max-crapload=48) rejected an implementation scoring 49, fix commit 9333da3 met the gate by decomposing the offending function (frontmatterScalar → frontmatterScalar + decodeScalarValue + blockFormValue, each CRAP 6 with full coverage) while leaving the threshold untouched at 48. This is the AGENTS.md Gatekeeping Value Protection rule working as intended: when an implementation cannot meet a gate, rework the implementation and record why — never adjust the gate. Reviews should treat threshold-untouched, code-reworked CI repairs as the compliance pattern and any threshold edit without human authorization as CRITICAL.
