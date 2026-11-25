---
tag: error-handling
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:25:11Z
identity: error-handling-20261107T162511-jay-flowers
tier: draft
---

An in-place os.WriteFile on a disaster-recovery artifact creates a crash window that defeats the artifact's purpose: os.WriteFile truncates before writing, so a crash or disk-full mid-rewrite destroys the only surviving copy, and the corrupted file then fails parsing at recovery time and is skipped. When code must mutate a backup/recovery file (dewey's learning markdown backups during supersession), write the updated content to a temp file in the same directory and os.Rename over the target — rename is atom
