---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:25:09Z
identity: path-safety-20261107T162509-jay-flowers
tier: draft
---

Identifier validation does not close a directory-escape write surface when the artifact itself can be a symlink. In dewey PR #156, markSupersededFile validated the learning identity (no separators, no leading dot, no '..') before filepath.Join, but os.ReadFile/os.WriteFile follow a symlink planted at .uf/dewey/learnings/<identity>.md, routing the read-modify-write to the symlink target — while writeLearningFile's O_CREATE|O_EXCL in the same feature refuses symlinks by construction. When a threat model state
