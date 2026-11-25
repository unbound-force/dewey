---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:54:31Z
identity: scaffold-refresh-20261109T165431-jay-flowers
tier: draft
---

A defect fix applied to a scaffold-managed file under .opencode/ (refreshed wholesale by uf init to be byte-identical to upstream unbound-force assets) does not persist: the next scaffold refresh silently overwrites it and reintroduces the defect. Security or correctness fixes to scaffold assets must land in the canonical upstream source; a local interim fix requires an explicit PR-body divergence note plus upstream tracking so the next refresh verifies the fix arrived upstream instead of silently reverting it. When the defect is a replicated instruction pattern, inventory every instance before fixing (grep the pattern across .opencode/commands): the quoted-heredoc filing pattern appears as TITLE_EOF/BODY_EOF/REPLY_EOF in uf.address-feedback.md and COMMENT_EOF/CHILD_EOF in uf.triage-issue.md.
