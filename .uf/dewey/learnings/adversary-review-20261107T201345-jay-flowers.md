---
tag: adversary-review
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:13:45Z
identity: adversary-review-20261107T201345-jay-flowers
tier: draft
---

For documentation-only diffs (markdown/spec artifacts, no code), the Adversary audit productively reduces to four checks: (1) secrets scan of added prose — issue references and repo names are fine, credentials are not; (2) gate-tampering check — diff hunks in AGENTS.md/tasks.md must not flip task checkbox states, alter protected gate sections, or weaken CI/security configuration, and recording a required gate's evidence (e.g., a website-docs-sync issue number in task 2.3) strengthens rather than weakens the audit trail; (3) content-preservation check when sections are moved or folded — every deleted line in the diff must reappear in the new location, since silently dropping documented risks or trade-offs is the doc-only equivalent of removing an error path; (4) clarification-marker syntax check — added spec/doc prose must not introduce literal NEEDS CLARIFICATION markers (MP-005). Verified on dewey PR #163 feedback-fix round (commits 2c4e58d, b25dc13, 94fc09e): all four checks passed, diff was clean.
