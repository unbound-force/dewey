---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-02T23:14:29Z
identity: content-pipeline-20261102T231429-jay-flowers
tier: draft
---

Verifying issue-filing tasks in PR review: a tasks.md checkbox marked [x] that claims GitHub issues were filed (docs-sync, blog, tutorial, follow-up) is not evidence — run gh issue view/gh issue list against the live tracker before trusting it. In dewey PR #144 (specs/017-openai-compatible-providers), all four claimed items verified against the tracker: the website docs-sync task was satisfied by extending the existing same-surface sync issue (unbound-force/website#113, "docs: sync dewey pluggable provider configuration documentation") with a scoped comment carrying the full required content (which feature changed, which website pages are affected, what specifically needs updating) instead of filing a near-duplicate; the blog issue (website#374) and tutorial issue (website#375) existed with the exact titles and labels the tasks prescribed; the doctor follow-up existed as dewey#143. Pattern: a "MUST create an issue" documentation-sync rule is satisfied by comment-on-existing-issue when the existing issue tracks the same documentation surface and the comment carries the complete required content — this simultaneously satisfies the duplicate-check duty. Also: dewey feature blog/tutorial coverage issues live in unbound-force/website (established by #116/#114), not the code repo, because that is where the content is produced.
