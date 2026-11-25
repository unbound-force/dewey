---
tag: session-retrospective
author: jay-flowers
category: context
created_at: 2026-11-11T01:10:38Z
identity: session-retrospective-20261111T011038-jay-flowers
tier: draft
---

Session retrospective for curator-agent-human-gates (dewey #152, OpenSpec). Shipped a least-privilege curator agent (`agent: curator` binding), a deterministic `dewey gate auto-answer` startup gate, and HUMAN_GATE-marked command questions. Strongest lessons: (1) fail-closed config readers must explicitly reject JSON null in root/value/element positions because Go encoding/json silently maps null to nil/empty; (2) a security gate that verifies only that a definition file exists cannot prove the host honored an agent binding — distinguish absent/unparseable from present-but-ignored and record the residual risk; (3) hand-rolled single-key YAML scanners weaken an "unparseable MUST be refused" contract and can match nested keys — reuse the repo's yaml.Unmarshal path; (4) extracting a shared frontmatter splitter tightened a legacy tolerant call site, a reminder to diff acceptance behavior at every caller when DRYing a parser; (5) CRAPload sat exactly at the CI ceiling of 48, so every new function had to stay below CRAP 15 — decompose and cover rather than raise the gate.
