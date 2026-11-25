---
tag: agent-permissions
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:51:11Z
identity: agent-permissions-20261110T235111-jay-flowers
tier: draft
---

Command allowlists in agent permission maps (e.g. `bash: {"cmd *": "allow"}`) are only as safe as the host's pattern-matching semantics. A suffix wildcard matches shell metacharacters (`cmd; evil`, `cmd && evil`, `cmd $(evil)`) under raw-string glob matching, while argv-based matching is vulnerable to leading environment assignments (`VAR=x cmd`) that strip to the allowed argv while redirecting the command's own config lookups. When a spec mandates an allow pattern, pin the matcher semantics in a recorded assumption and put adversarial forms (chained, substituted, env-prefixed) in the acceptance matrix — benign refusal cases alone will pass while the bypass remains live.
