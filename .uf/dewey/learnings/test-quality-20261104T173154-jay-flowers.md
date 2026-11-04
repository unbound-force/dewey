---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-04T17:31:54Z
identity: test-quality-20261104T173154-jay-flowers
tier: draft
---

When a writer switches to defensive quoting (e.g., %q-rendered YAML frontmatter to prevent restructured documents) and a hand-rolled reader adds an unquoting pass in another package, test the seam, not just the halves. Package-local hand-written fixtures keep passing when the writer's format changes because they still exercise the old format; a yaml.v3 round-trip on the writer side does not cover a custom line-scanning parser on the reader side. Generate the reader's test fixture by calling the writer's actual renderer (or run the full write→read pipeline in one test) so the two sides cannot drift apart silently — observed in dewey PR 145, where lint's unquoteScalar path for curate's quoted frontmatter had no test exercising real curate output.
