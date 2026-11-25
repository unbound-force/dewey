---
tag: injection-safety
author: jay-flowers
category: pattern
created_at: 2026-11-09T16:54:33Z
identity: injection-safety-20261109T165433-jay-flowers
tier: draft
---

Command templates that state a shell-safety doctrine (quoting is defense-in-depth, not neutralization; charset gates are the enforcement) must apply it to every new interpolated value. Arbitrary untrusted prose — finding, reply, or issue text — cannot be charset-gated, so it must never be interpolated into a shell script body (heredoc or otherwise): a quoted heredoc still terminates on any line exactly matching its published delimiter, and everything after that line executes as shell. The safe pattern is to write untrusted text to a file through the host file-write tool and keep only fixed, fully-quoted commands in shell (gh issue create --title "$(cat \"$TITLE_FILE\")" --body-file "$BODY_FILE"); the durable end state is an argument-array exec tool with no string interpolation (unbound-force #965).
