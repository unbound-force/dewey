---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:00:11Z
identity: path-safety-20261107T170011-jay-flowers
tier: draft
---

Creation-time hardening does not carry to rewrite paths. In dewey's learning pipeline, writeLearningFile creates files with O_CREATE|O_EXCL (symlink-refusing by construction), but markSupersededFile's later frontmatter rewrite used os.ReadFile + in-place os.WriteFile — both follow symlinks and truncate before writing, re-opening the file-type and atomicity channels the creation path closed. The package already had the correct rewrite discipline for durability-critical markdown: tools/compile.go writeArticle writes a dot-prefixed temp file, os.Remove's it on error, then os.Rename's it into place (a crash leaves an ignorable hidden .tmp, never a corrupted artifact), and vault.go has an atomicWrite helper doing the same. Review heuristic: when a change adds a rewrite path for an artifact whose creation path is hardened, check whether the hardening and the package's atomic-write precedent carry over; durability-critical rewrites need an os.Lstat/IsRegular (or O_NOFOLLOW) guard plus write-temp-then-rename, with the temp name dot-prefixed so crash leftovers stay invisible to *.md scans.
