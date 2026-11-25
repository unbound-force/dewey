---
tag: documentation-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:21Z
identity: documentation-accuracy-20261111T000521-jay-flowers
tier: draft
---

Documentation update targets in OpenSpec artifacts must be verified against the actual file heading tree, not recalled from a sibling document. In serialize-supersession-writes design D5 located the README update in "the 'Temporal Validity' subsection under 'File-Backed Learnings'", but "File-Backed Learnings" is an AGENTS.md heading (AGENTS.md:516) while in README.md `#### Temporal Validity` (README.md:778) nests under `### Knowledge Stores` (README.md:756) under `## Knowledge Compilation` (README.md:731). The same claim "there is no store_learning section" missed README's `### Learning` tool table (README.md:146-150), whose `store_learning` row already links to `#temporal-validity` — the natural cross-link surface for the change. Grep the target file's headings before naming a section anchor.
