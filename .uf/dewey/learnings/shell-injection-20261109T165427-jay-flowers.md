---
tag: shell-injection
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:54:27Z
identity: shell-injection-20261109T165427-jay-flowers
tier: draft
---

A quoted heredoc delimiter (`cat > "$FILE" << 'EOF'`) disables variable and command expansion but NOT content-based termination: the first body line that exactly equals the delimiter ends the heredoc, and every remaining intended-content line executes as a shell command. In agent command templates that embed untrusted text (reviewer comments, finding bodies, issue text) into bash blocks, a published or predictable delimiter is therefore an injection vector — any external reviewer can plant the breakout line, and a reviewer quoting the template's own heredoc trips it accidentally. The durable pattern: write untrusted text to a file through the host file-write tool (no shell parsing) and pass it to the CLI as data (`--body-file "$FILE"`, or a quoted `$(cat "$FILE")` whose substitution output is never re-parsed as shell). A per-invocation runtime-random delimiter is the weaker fix; an argument-array exec tool removes the class entirely.
