---
tag: gate-integrity
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:26:08Z
identity: gate-integrity-20261111T202608-jay-flowers
tier: draft
---

When a tool-rendered verdict surface gains new output lines derived from environment- or config-supplied values, the same increment must pin the sanitization of the new lines with a control-character fixture test — a sanitizer existing elsewhere in the render function does not automatically cover new call sites. gate-config-path-parity's TestRenderGateVerdict_MultiFile pins the new "Also checked:" lines with a carriage-return + ANSI-escape path (asserting safeValue's space-mapping and escape-stripping), so a future refactor that drops safeValue from the new render loop fails a test rather than shipping a control-character injection surface in a gate verdict that agents relay verbatim.
