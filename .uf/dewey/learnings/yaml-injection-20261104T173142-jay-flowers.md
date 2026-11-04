---
tag: yaml-injection
author: jay-flowers
category: gotcha
created_at: 2026-11-04T17:31:42Z
identity: yaml-injection-20261104T173142-jay-flowers
tier: draft
---

Control-character validation for YAML frontmatter (rejecting code points below 0x20, DEL, U+0085, U+2028, U+2029) blocks newline-based new-key injection but NOT indicator semantics: a value containing ' #' turns the rest of the line into a comment (value silently dropped), ': ' inside a plain scalar breaks the mapping (parse error), and a leading '{', '[', '*', '&', '!', or quote changes the parsed type to a collection, alias, anchor, or tag. In dewey PR 145, the compile path gated compiled_by/sources with exactly such a validator while rendering them unquoted, whereas the curate path added in the same change quoted scalars with %q — which closes the entire class because Go's %q emits a YAML-compatible double-quoted scalar with escapes. Prefer %q rendering at emit time over character-set validation alone; if a validator must be used, it needs to reject indicator sequences, not just control characters.
