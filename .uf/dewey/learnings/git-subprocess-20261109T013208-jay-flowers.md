---
tag: git-subprocess
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:32:08Z
identity: git-subprocess-20261109T013208-jay-flowers
tier: draft
---

git diff honors repository-local configuration: `diff.external` from .git/config and `diff.<driver>.textconv` from .gitattributes can execute vault-configured commands when a diff is generated. Any subprocess git-diff probe against a repository treated as untrusted (e.g., dewey's vault in the lint anchor checks, tools/lint_anchors.go PathChanged) must pass `--no-ext-diff` and `--no-textconv` so the probe cannot invoke repository-configured commands. `--quiet` suppresses diff output, but its effect on driver invocation is an undocumented implementation detail of git's diff plumbing, not a security guarantee — the property must be enforced by explicit flags. Non-diff probes (rev-parse, cat-file -e) do not invoke this machinery, so they need no suppression.
