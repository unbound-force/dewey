---
tag: prompt-injection
author: jay-flowers
category: pattern
created_at: 2026-11-12T19:00:18Z
identity: prompt-injection-20261112T190018-jay-flowers
tier: draft
---

Prompt-injection boundaries over stored metadata work best when the validation grammar is derived from the generator's output charset, not from a hand-written pattern. In dewey's explicit-supersession-in-compile change, learning identities are produced by normalizeTag (lowercase, alphanumerics, hyphens) for both tag and author, so the new isLearningIdentity grammar (lowercase-alnum segments plus the YYYYMMDDTHHMMSS timestamp segment, plus the legacy {tag}-{sequence} form) accepts exactly what the identity generator emits and nothing more. decodeTemporalEntryFields then withholds any validity/supersedes/superseded_by value that fails the grammar before it is interpolated into the synthesis prompt, so a hostile or hand-edited property value containing spaces, newlines, or brackets is treated as absent rather than rendered. The same grammar doubles as the citation-token discriminator (bracketed groups whose entire comma-separated content matches the grammar are citations; all other markdown brackets are ignored), and the proposal-rationale sanitizer strips Cc/Cf/Zl/Zp runes and bounds to 512 runes. The reusable rule: at every LLM boundary, validate against the producer's canonical charset and withhold (not reject the whole call) on mismatch — withholding keeps corrupt data from reaching the prompt while the surrounding operation still succeeds.
