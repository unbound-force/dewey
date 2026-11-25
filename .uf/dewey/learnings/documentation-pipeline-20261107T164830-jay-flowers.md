---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:48:30Z
identity: documentation-pipeline-20261107T164830-jay-flowers
tier: draft
---

In unbound-force/dewey, changes to CLI output format — including `dewey doctor` diagnostics — require a `docs`-labeled sync issue in the unbound-force/website repository per AGENTS.md (title format: "docs: sync dewey <feature> documentation"). Precedent: website#249 tracked the doctor Synthesis Layer change and referenced content/docs/dewey/commands/doctor.md. Tactical OpenSpec changes routinely miss this obligation because proposal.md Impact sections list only code files (the doctor-vertex-embedding change omitted both its own README update and the website sync). Reviews of output-changing dewey PRs must search unbound-force/website across all states for the sync issue before approving — an open provider-configuration issue (website#113) does not cover doctor output changes, and the originating bug issue (e.g., dewey#143) does not satisfy the requirement.
