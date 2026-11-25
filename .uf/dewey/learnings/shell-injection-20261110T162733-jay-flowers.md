---
tag: shell-injection
author: jay-flowers
category: gotcha
created_at: 2026-11-10T16:27:33Z
identity: shell-injection-20261110T162733-jay-flowers
tier: draft
---

A quoted heredoc neutralizes shell expansion but not content-based termination: it ends at any line that exactly matches the delimiter, and command templates publish their delimiters, so any templated shell block that routes untrusted text (issue titles, finding bodies, reply text) through a heredoc is a command-injection vector even when fully quoted. The durable fix keeps only fully-quoted tool invocations in the shell (a quoted $(cat ...) is not re-parsed) and moves untrusted content through the host's file-write tool or an argument-array exec API, so the content never enters a shell parse. Verified in dewey PR #167: the uf.address-feedback.md heredoc breakout was fixed in a6b988b with mktemp + host file-write tool + fully-quoted gh/git invocations, leaving no delimiter for untrusted text to terminate.
