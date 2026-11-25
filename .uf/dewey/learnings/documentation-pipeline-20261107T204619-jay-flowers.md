---
tag: documentation-pipeline
author: jay-flowers
category: reference
created_at: 2026-11-07T20:46:19Z
identity: documentation-pipeline-20261107T204619-jay-flowers
tier: draft
---

The dewey repo maintains no CHANGELOG.md (verified via glob CHANGELOG* at review time, Nov 2026): user-visible changes reach release notes through the conventional-commit-driven release pipeline (release.yml delegates to org-infra preflight + GoReleoser, which builds GitHub Releases from commit messages), not a repo file. The curator documentation-gap checklist item "Was CHANGELOG.md updated?" is therefore N/A for this repo — resolve it by confirming the change's commits use Conventional Commits format (fix:/feat:/docs:) instead of flagging a missing file update. On PR #158 (doctor-vertex-embedding) the commit "fix: diagnose Vertex embedding config without Ollama checks (#143)" satisfies this; a reviewer applying the generic checklist verbatim would produce a false MEDIUM finding.
