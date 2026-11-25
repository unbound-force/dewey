---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:54:30Z
identity: dewey-doctor-20261107T155430-jay-flowers
tier: draft
---

dewey doctor provider diagnostics follow a fixed shape in cli.go runDoctorChecks: validate configuration through the provider factory (embed.NewEmbedderFromConfig / llm.NewSynthesizerFromConfig), which is network-free at construction — Vertex ADC discovery via google.FindDefaultCredentials is deferred to request time (defaultGetToken), so factory success means config completeness, not runtime reachability. On factory error print FAIL "config incomplete: <err>" (the factory error names the missing field); 
