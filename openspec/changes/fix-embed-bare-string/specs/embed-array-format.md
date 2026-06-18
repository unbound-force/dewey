## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Ollama Embed Input Format

The `OllamaEmbedder.Embed()` method MUST send the `input` field as a JSON array of strings (`["text"]`) when calling the Ollama `/api/embed` endpoint. Previously, it sent a bare JSON string (`"text"`).

Previously: `Embed()` passed a bare `string` to `doEmbed()`, producing `"input": "text"` in the request body.

#### Scenario: Single text embedding sends array format

- **GIVEN** a caller invokes `OllamaEmbedder.Embed(ctx, "hello world")`
- **WHEN** the HTTP request is sent to `/api/embed`
- **THEN** the JSON body MUST contain `"input": ["hello world"]` (array of one string)

#### Scenario: Batch embedding continues to send array format

- **GIVEN** a caller invokes `OllamaEmbedder.EmbedBatch(ctx, []string{"a", "b"})`
- **WHEN** the HTTP request is sent to `/api/embed`
- **THEN** the JSON body MUST contain `"input": ["a", "b"]` (array of strings)

#### Scenario: Strict Ollama proxy accepts single embed request

- **GIVEN** an Ollama proxy that rejects bare-string `input` fields
- **WHEN** `Embed()` is called with any text
- **THEN** the request MUST succeed (HTTP 200) because the input is sent as an array

### Requirement: Embed Wire Format Test Coverage

The `TestOllamaEmbedder_Embed` test MUST assert that the `input` field in the HTTP request body is a JSON array, not a bare string. This SHOULD mirror the existing assertion pattern in `TestOllamaEmbedder_EmbedBatch`.

#### Scenario: Test catches bare-string regression

- **GIVEN** the test server receives a request from `Embed()`
- **WHEN** the test decodes the request body
- **THEN** the test MUST verify that `req.Input` is of type `[]any` (JSON array)

## REMOVED Requirements

_None._
