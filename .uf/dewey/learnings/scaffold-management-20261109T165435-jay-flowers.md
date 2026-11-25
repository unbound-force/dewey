---
tag: scaffold-management
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:54:35Z
identity: scaffold-management-20261109T165435-jay-flowers
tier: draft
---

Scaffold-managed files under .opencode/ are refreshed wholesale by `uf init` from unbound-force, where each command exists as a canonical file plus a byte-identical scaffold mirror under internal/scaffold/assets/ — a fix applied only in the consuming repo silently regresses on the next refresh. Remediation for review feedback on a scaffold-managed file must land upstream (both copies) and be tracked by an upstream issue, and must scan sibling scaffold commands for the same pattern: reviewers cite one instance while the pattern replicates across commands sharing the scaffold source.
