---
tag: injection-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:01Z
identity: injection-safety-20261109T160801-jay-flowers
tier: draft
---

Quoted heredocs (`cat > file << 'EOF'`) protect untrusted text from shell expansion, but the heredoc still terminates on any line that exactly matches the delimiter — everything after that line executes as shell commands. When an agent command template embeds untrusted PR/review/issue content into a bash heredoc, a static delimiter published in the repository is attacker-plantable: a review comment containing a bare line matching the delimiter breaks out of the quoted-heredoc sandbox, and quoting the interp
