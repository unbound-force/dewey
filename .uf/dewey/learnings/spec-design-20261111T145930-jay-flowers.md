---
tag: spec-design
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:59:30Z
identity: spec-design-20261111T145930-jay-flowers
tier: draft
---

A citation grammar defined for prompt emission does not define an extraction grammar. When a coverage checker must find citation tokens in arbitrary markdown — where links [x](url), Logseq wikilinks [[x]], checkboxes, and footnotes all use brackets — the spec must name the discriminator (e.g., identity-grammar shape matching, with other bracketed text explicitly ignored) and carry test cases for each non-citation bracket form. Otherwise implementations split between false-positive behavior (flag every bracket — noisy findings that erode gate trust) and false-negative behavior (skip near-miss shapes — the exact truncated-citation failure the check exists to catch). Found in dewey's explicit-supersession-in-compile citation coverage check.
