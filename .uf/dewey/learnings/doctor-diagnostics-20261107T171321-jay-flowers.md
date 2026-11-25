---
tag: doctor-diagnostics
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:13:21Z
identity: doctor-diagnostics-20261107T171321-jay-flowers
tier: draft
---

Dewey doctor's provider-aware diagnostics validate cloud provider configuration through the provider factory constructors, which are network-free by construction: embed.NewEmbedderFromConfig checks project/region for vertex (embed/provider.go) and NewVertexEmbedder checks model (embed/vertex.go), only building an http.Client — OAuth token acquisition is deferred to request time (defaultGetToken), so doctor and tests never need GCP credentials or network (Constitution IV). When adding a new doctor provider branch, mirror the established vertex shape (cli.go Synthesis Layer case "vertex" and printVertexEmbeddingChecks): section header "(model via provider)", factory call for validation, FAIL "incomplete: <err>" naming the missing field, PASS with field=value pairs — and never Ollama reachability checks or "ollama pull" hints for non-Ollama providers. Note for output-slicing tests: a section slice from "Embedding Layer" to "Synthesis Layer" also contains the provider-aware "Embedding count" fix-hint block (cli.go ~1545), which only stays Ollama-free for non-Ollama providers because that pre-existing branch is already provider-aware.
