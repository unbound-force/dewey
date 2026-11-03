# Contract: OpenAI-Compatible Wire Protocol

**Feature**: `017-openai-compatible-providers` |
**Date**: 2026-11-02

This contract describes what Dewey sends to and expects from an
OpenAI-compatible gateway. It is the external interface consumed by
the feature; internal Go types are not part of it.

## Base URL

- `embedding.endpoint` and `synthesis.endpoint` are API base URLs.
- A missing scheme is normalized by prepending `http://`.
- When a resolved API key would transit an `http://` endpoint on a
  non-loopback host, Dewey warns at startup and in `dewey doctor`
  (FR-024); loopback hosts are exempt because local servers may use
  HTTP.
- Trailing slashes are removed.
- Dewey appends exactly one operation path: `/embeddings`,
  `/chat/completions`, or `/models`. If the endpoint already ends in
  an operation path, it is removed first, so the final URL never
  contains a doubled path (FR-010).
- An endpoint that is empty or whitespace-only is treated as missing
  and produces the FR-009 field-naming factory error.
- An endpoint with a non-HTTP(S) scheme (`file://`, `ftp://`) is
  rejected with a clear configuration error naming the endpoint
  field, rather than deferring to a request-time protocol error.
- An endpoint containing a query string or fragment is rejected with
  a clear configuration error naming the endpoint field.
- An endpoint with embedded userinfo (`user:pass@host`) is rejected
  with a clear configuration error naming the endpoint field;
  credentials in URLs are not accepted and would fall outside the
  key-redaction rule.

## Common Request Headers

| Header | Value | When |
|--------|-------|------|
| `Content-Type` | `application/json` | Every POST |
| `Authorization` | `Bearer <key>` | Only when a key resolves |

No other authentication scheme is supported. No key is sent when none
resolves.

## POST {base}/embeddings

### Request

```json
{
  "model": "openrouter/amazon/titan-embed-text-v2",
  "input": ["first text", "second text"]
}
```

- One request carries all inputs in a batch.
- `model` is sent verbatim.
- `input` preserves input order and UTF-8 content.

### Success Response

```json
{
  "data": [
    {"index": 1, "embedding": [0.12, -0.03]},
    {"index": 0, "embedding": [0.44, 0.01]}
  ]
}
```

Dewey requires exactly one entry per input, restores order by
`index`, and converts numbers to `float32`.

### Error Conditions

| Condition | Dewey behavior |
|-----------|----------------|
| Empty input list | Return empty result; no request |
| Entry count differs from input count | Descriptive error naming the provider |
| Duplicate or out-of-range index | Descriptive error naming the provider |
| Non-2xx status | Error with status and bounded excerpt |
| Malformed JSON | Parse error naming the provider |

## POST {base}/chat/completions

### Request

```json
{
  "model": "enmaas/rits/zai-org/glm-5-3",
  "messages": [{"role": "user", "content": "Synthesize ..."}],
  "stream": false
}
```

- `model` is sent verbatim; slashes and hyphens are not parsed.
- Only a single user message is sent.
- `stream` is always `false`.
- No `tools`, `functions`, `response_format`, or `max_tokens` are sent.

### Success Response

```json
{
  "choices": [
    {"message": {"role": "assistant", "content": "Compiled article"}}
  ]
}
```

Dewey reads `choices[0].message.content`.

### Error Conditions

| Condition | Dewey behavior |
|-----------|----------------|
| No choices, or empty or null content | Descriptive error; no article persisted |
| Non-2xx status | Error with status and bounded excerpt |
| Malformed JSON | Parse error naming the provider |

## GET {base}/models (Availability Probe)

- Sent by `Available()` only.
- Uses the same `Authorization` header rule as other requests.
- Any 2xx means available.
- Connection error, 401, 403, or any other status means unavailable.
- The configured model is not required to appear in the list.
- The result is cached for thirty seconds.
- The probe timeout is one second.
- The probe response body is drained, bounded by the same read limit
  as other requests, and closed even though only the status is used,
  so repeated probes cannot balloon memory or pin the connection
  pool.

A 401 or 403 when no key resolved produces an authentication error
that names the expected environment variables. A 401 or 403 when a key
resolved produces a "verify the key" error. Neither error contains
credential material.

## Rate Limiting

- Only HTTP 429 is retried.
- `Retry-After` with integer seconds is honored, capped at 60 seconds.
- Otherwise: exponential backoff from 1 second, doubling per attempt,
  plus or minus 25 percent jitter, capped at 60 seconds.
- Budget: initial attempt plus at most five retries.
- Context cancellation during a wait returns the context error
  promptly.
- A missing, negative, zero, non-numeric, or HTTP-date
  `Retry-After` value is treated as absent; exponential backoff
  applies.

## Timeouts

- Embedding requests time out after 30 seconds.
- Synthesis requests time out after 300 seconds, matching the Vertex
  synthesizer. The 013 llm-interface 120-second invariant governs
  the Ollama synthesizer; the divergence is recorded in
  `research.md` R12.
- The availability probe times out after one second.

## Errors

Error text follows this shape:

```text
openai-compatible <capability> error (HTTP <status>): <excerpt>
```

- `<capability>` is `embedding` or `synthesis`.
- Every occurrence of the resolved key is replaced with `[REDACTED]`
  before the excerpt is capped, so a key straddling the 512-byte
  boundary cannot leak a partial prefix.
- `<excerpt>` is capped at 512 bytes and marked when truncated.
- All C0 control characters (newlines, tabs, carriage returns,
  backspaces, and the rest of the C0 range) and ESC-initiated escape
  sequences are stripped or escaped from the excerpt so
  gateway-controlled bytes cannot forge log lines or corrupt terminal
  rendering.
- Request headers, bodies, and URLs never appear in errors or logs.

## Redirects

The HTTP client follows redirects under an explicit `CheckRedirect`
policy: the `Authorization` header is forwarded only when the
destination matches the initial request's host (port-insensitive,
case-insensitive) and scheme. Cross-host destinations, subdomains, and
same-host scheme downgrades (https to http) do not receive the header.

## Out of Scope

- Streaming responses.
- Tool calling, function calling, and JSON response formats.
- Automatic model discovery or model catalogue checks.
- Automatic splitting of oversized batches; gateway errors are
  surfaced instead.
- Anthropic-native or other non-OpenAI wire protocols.
