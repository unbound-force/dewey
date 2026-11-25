---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:00:05Z
identity: path-safety-20261107T170005-jay-flowers
tier: draft
---

Dewey's file-backed durability copies need rewrite-time guards that creation-time protections do not provide: os.WriteFile follows symlinks and truncates before writing, so a later in-place rewrite of a file originally created with O_CREATE|O_EXCL (symlink-refusing) reopens the file-type channel and exposes the artifact to crash/ENOSPC corruption — defeating the copy's purpose of surviving graph.db loss (seen in markSupersededFile, tools/learning_temporal.go:253). Guard pattern, applicable to every later-rewritten artifact class (learnings, compiled/, knowledge/): os.Lstat + Mode().IsRegular() refusal, then write-temp-in-same-directory + os.Rename. Rename replaces a final-path symlink instead of following it, and CreateTemp's 0600 preserves a tight mode; name temps with a dot prefix and no .md suffix so an orphaned temp is invisible to re-ingestion.
