---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:10:07Z
identity: documentation-20261110T231007-jay-flowers
tier: draft
---

A spec's documentation tasks must cover every newly scaffolded user-facing artifact, not only the feature's headline workflow prose. A change that makes `dewey init` write a new file into user repositories changes the command's observable output; if the README command reference and the CLI table are not scheduled for update, users encounter an unexplained file. Treat scaffold output as a documentation surface in its own right.
