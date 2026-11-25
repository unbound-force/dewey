---
tag: dewey-doctor
author: jay-flowers
category: context
created_at: 2026-11-07T15:54:29Z
identity: dewey-doctor-20261107T155429-jay-flowers
tier: draft
---

In dewey, `dewey doctor` provider diagnostics (Embedding/Synthesis Layer) validate configuration through the provider factories (`embed.NewEmbedderFromConfig`, `llm.NewSynthesizerFromConfig`), which make no network calls: the Vertex constructors defer `google.FindDefaultCredentials` to a lazy `tokenFn` invoked only at request time. Consequence: doctor's Vertex config tests are deterministic regardless of ambient GCP credentials (GOOGLE_APPLICATION_CREDENTIALS, gcloud ADC, GCE metadata), and `dewey doctor` s
