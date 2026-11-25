---
tag: git-subprocess-security
author: jay-flowers
category: gotcha
created_at: 2026-11-10T15:33:16Z
identity: git-subprocess-security-20261110T153316-jay-flowers
tier: draft
---

Passing untrusted strings to git subprocesses needs four controls, not three: (1) validate revisions against ^[0-9a-fA-F]+$ — pure hex can never be parsed as an option, ref expression, or range; (2) place paths after `--` so they cannot be parsed as options; (3) disable repository-configured command execution with --no-ext-diff --no-textconv on git diff (diff.external and textconv otherwise run binaries chosen by whoever controls the vault — dewey PR #159 added a marker-file regression test proving both are
