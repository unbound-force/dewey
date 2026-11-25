---
tag: fail-closed-pruning
author: jay-flowers
category: gotcha
created_at: 2026-11-19T04:41:25Z
identity: fail-closed-pruning-20261119T044125-jay-flowers
tier: draft
as_of: "dewey@2a7f0147afcf07964a3cb8ce9d52b7790e1a505e"
evidence:
  - "unbound-force/dewey:main.go"
  - "unbound-force/dewey:main_test.go"
  - "unbound-force/dewey#222"
---

In scan-then-prune startup passes (reIngestLearnings / IndexKnowledgeStore in dewey), the prune file set must be recorded from the directory entry before any per-file I/O. When the file-set key requires reading the file (the learning identity comes from frontmatter), a per-file read or parse failure silently excludes the file from the set and the prune tail deletes its page even though the file exists on disk — a transient fault (partial write during a pull, merge-conflict markers breaking YAML, per-file permission change on a listable directory) mass-deletes pages through the very tail the fail-closed scan-error guards were written to protect. Two safe shapes, both now in dewey: (a) entry-name keys (knowledge stores) record the base name from the directory entry BEFORE ReadFile — a listed-but-unreadable file stays in the set and its page survives; (b) read-required keys (learnings) count per-file read/parse failures and skip the prune tail entirely for the pass when any occurred (fail-closed at the pass level, retried next startup). Any scan-then-prune tail needs a test pinning the per-file read/parse-failure interaction, not just the directory-level scan error — the directory-level test alone gives false confidence that transient faults never prune.
