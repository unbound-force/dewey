---
tag: prompt-injection
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:49:54Z
identity: prompt-injection-20261111T204954-jay-flowers
tier: draft
---

When a slash-command instructs an agent to relay tool output verbatim, error-message text is an untrusted-input channel just like structured data fields — and it is the one most often forgotten. Git error wraps interpolate pathspec arguments verbatim (e.g. fmt.Errorf("git %s: %w", strings.Join(args, " "), err) in dewey's gitAnchorChecker.run), so an attacker-controlled evidence locator or page name can ride instruction-like text into the "reason" a curator agent relays at a human gate. In the dewey-distill-verify-step review (issue #151), the command's presentation rules correctly demanded code-span rendering and "untrusted data" labeling for evidence locators but only said "relay the tool's reason verbatim" for failure reasons. Rule of thumb: every string that originates in indexed/re-ingested content — structured fields AND error/reason text — needs the same untrusted-data framing (code span, explicit label) in command files that present it to humans or agents.
