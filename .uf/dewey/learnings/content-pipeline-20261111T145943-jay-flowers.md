---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:43Z
identity: content-pipeline-20261111T145943-jay-flowers
tier: draft
---

OpenSpec documentation tasks enumerated by what the change adds (new tool rows, counts, new sections) can leave falsified prose standing beside the new text, because implementation and review-fix loops execute tasks.md checkboxes literally. When a change reverses a documented behavior, the doc task must name the claim being removed — e.g. "rewrite the compile recency sentence in the dewey compile CLI section and Knowledge Compilation intro" — not just the additions ("compile citations, coverage, and proposals"). In explicit-supersession-in-compile (spec review), tasks 9.1/9.2 name every added surface, but no task owns correcting README's "resolve temporal contradictions — newer facts replace older ones" claims or AGENTS.md's insert-time-only supersedes bullet, both of which the declared-supersession policy (D1) falsifies. Authoring rule: for each behavior a change reverses, search the tasked doc files for the old claim and name its correction in the task.
