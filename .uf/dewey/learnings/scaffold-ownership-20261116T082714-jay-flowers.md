---
tag: scaffold-ownership
author: jay-flowers
category: gotcha
created_at: 2026-11-16T08:27:14Z
identity: scaffold-ownership-20261116T082714-jay-flowers
tier: draft
as_of: "dewey@f6911abc6403998f6eaf1c2bb6d09007e516a2c6"
---

Byte-exact SHA-256 ownership markers (payload digest on the file's final line) make any byte-level transformation — git `core.autocrlf` checkout normalization, an editor adding or stripping a trailing newline, whitespace cleanup — look identical to an intentional user edit, so the file is classified `customized` and never refreshed again. Log a distinct reason (`digest-mismatch` vs `marker-absent` vs `malformed-marker` vs `symlink`) and document LF preservation, otherwise the refresh silently no-ops for whole classes of users.
