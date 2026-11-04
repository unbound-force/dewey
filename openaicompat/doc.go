// Package openaicompat implements the shared transport policy for
// OpenAI-compatible gateways (OpenRouter, ENMaas, LiteLLM, vLLM, and
// self-hosted servers).
//
// It speaks the OpenAI v1 wire protocol over plain net/http: a JSON POST
// request to {base}/embeddings and an authenticated GET {base}/models
// availability probe. The package owns
// base-URL normalization, environment-only API-key resolution, 429 retry
// with backoff, credential-free error excerpts, and the cached
// availability probe exactly once so the embed and llm providers cannot
// drift on these security-sensitive rules (research.md R1).
//
// Import direction is deliberately one-way: embed imports openaicompat.
// openaicompat imports only the standard library and
// github.com/charmbracelet/log, so it stays a true leaf and introduces
// no dependency cycle. Synthesis traffic does not use this package: the
// llm provider owns its own transport.
package openaicompat
