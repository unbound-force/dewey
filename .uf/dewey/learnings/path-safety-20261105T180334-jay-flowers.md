---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-05T18:03:34Z
identity: path-safety-20261105T180334-jay-flowers
tier: draft
---

In dewey, .uf/dewey/learnings/ writes have two safety postures: writeLearningFile creates files with O_CREATE|O_EXCL, which fails on an existing symlink and therefore never writes through one, while markSupersededFile (learning-temporal-validity) rewrites existing backups in place with os.ReadFile + os.WriteFile, which both follow symlinks. Because learning files are git-synced and treated as untrusted, any new in-place-rewrite flow added next to an O_EXCL creation flow re-opens the symlink escape the creation path closes.
