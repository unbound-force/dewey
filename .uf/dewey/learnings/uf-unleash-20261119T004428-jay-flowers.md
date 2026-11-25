---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-19T00:44:28Z
identity: uf-unleash-20261119T004428-jay-flowers
tier: draft
as_of: "dewey@f7c7f99f3d8f90dbf6d681b99a18c9f38162988c"
evidence:
  - "dewey#224"
  - "dewey:openspec/changes/record-which-dewey-built-an-index-and-sa/tasks.md"
---

uf_delegate_command's default 600-second wall clock is too short for test-writing delegations in the dewey repo: package main's suite alone takes ~11s per run, and authoring hundreds of lines of table-driven tests plus verification exceeds 10 minutes (observed twice in dewey#224: task 2.4 timed out mid-work and task 4.2 left only a stray strconv import). Pass an explicit timeout — 1_800_000 ms, the prompt-only maximum — for any delegation expected to author substantial test code and run package suites. A timed-out child's partial work is not lost: re-dispatch with resume context describing the uncommitted state and the longer timeout, and the child completes it (the 4.2 resume kept the stranded import and built on it).
