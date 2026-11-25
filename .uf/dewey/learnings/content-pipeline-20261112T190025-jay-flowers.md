---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-12T19:00:25Z
identity: content-pipeline-20261112T190025-jay-flowers
tier: draft
---

When a change's website-sync determination finds the affected tutorial surfaces are still-unwritten open issues, extending those issues' scope by comment satisfies the tutorial-coverage requirement without duplicate tracking — provided the change's tasks.md record names every issue number, comment ID, and deliberate no-issue reason. explicit-supersession-in-compile (dewey #150) filed new docs website#409 and blog website#410, but extended website#376 (distill tutorial) and website#393 (store_learning supersession tutorial) by scope-extension comments naming the change instead of filing a third tutorial issue; the curator verified all four surfaces deterministically via read-only `gh issue view`. This refines the three-issue convention (docs + blog + tutorial, from dewey-publish tasks 7.3+7.8): the unit of verification is "every affected website surface is tracked somewhere with the change referenced," not "three new issues exist." The tasks.md 9.4 record also demonstrates the adjacent-surface scoping step — checking blog #392/tutorial #393 for insert-time-only supersession wording and the tutorials directory for a compile walkthrough — before deciding what to file.
