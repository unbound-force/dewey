# Contract: LLM Synthesis Interface

**Package**: `llm`
**File**: `llm/llm.go`

## Interface: Synthesizer

```go
// Synthesizer generates natural language text from prompts. Used by the
// compile tool to synthesize compiled articles from clustered learnings.
// Implementations must be safe for concurrent use.
//
// Design decision: Separate from embed.Embedder because embedding and
// synthesis use different Ollama API endpoints (/api/embed vs /api/generate),
// different models (embedding model vs generation model), and have different
// error modes. Single Responsibility Principle.
type Synthesizer interface {
    // Synthesize generates text from a prompt. Returns the generated text
    // or an error if generation fails (model unavailable, timeout, etc.).
    Synthesize(ctx context.Context, prompt string) (string, error)

    // Available reports whether the generation model is loaded and ready.
    // Returns false if the provider is unreachable or the model is not pulled.
    Available() bool

    // ModelID returns the model identifier used for text generation.
    ModelID() string
}
```

## Implementation: OllamaSynthesizer

```go
// OllamaSynthesizer implements Synthesizer using Ollama's HTTP API.
// Calls POST /api/generate for text generation.
//
// Design decision: Uses the same HTTP client pattern as OllamaEmbedder
// but targets a different endpoint and model. The generation model
// (e.g., "llama3.2:3b") is typically larger than the embedding model
// (e.g., "granite-embedding:30m").
type OllamaSynthesizer struct {
    baseURL string
    model   string
    client  *http.Client
    // Cache model availability (same pattern as OllamaEmbedder).
    mu            sync.RWMutex
    available     bool
    lastCheck     time.Time
    checkInterval time.Duration
}

// NewOllamaSynthesizer creates an OllamaSynthesizer that connects to
// the Ollama API at baseURL using the specified generation model.
// Returns a ready-to-use synthesizer with a 120-second HTTP timeout
// (generation is slower than embedding) and 30-second availability
// cache interval.
func NewOllamaSynthesizer(baseURL, model string) *OllamaSynthesizer
```

### Ollama Generate API

```go
// generateRequest is the JSON body sent to POST /api/generate.
type generateRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Stream bool   `json:"stream"` // false for non-streaming
}

// generateResponse is the JSON response from POST /api/generate.
type generateResponse struct {
    Response string `json:"response"`
    Done     bool   `json:"done"`
}
```

### Synthesize Method

```go
// Synthesize generates text by calling Ollama's POST /api/generate
// endpoint with stream=false. Returns the complete generated text.
// Returns an error if the HTTP request fails, Ollama returns a non-200
// status, or the response cannot be parsed.
func (o *OllamaSynthesizer) Synthesize(ctx context.Context, prompt string) (string, error)
```

## Implementation: NoopSynthesizer

```go
// NoopSynthesizer is a test double that returns a fixed response.
// Used in tests to avoid requiring a running Ollama instance.
type NoopSynthesizer struct {
    Response string
    Err      error
    Model    string
    Avail    bool
}

func (n *NoopSynthesizer) Synthesize(_ context.Context, _ string) (string, error) {
    return n.Response, n.Err
}

func (n *NoopSynthesizer) Available() bool { return n.Avail }
func (n *NoopSynthesizer) ModelID() string { return n.Model }
```

## Configuration

The synthesizer is configured via `.uf/dewey/config.yaml`:

```yaml
compile:
  model: "llama3.2:3b"
  ollama_url: "http://localhost:11434"
```

When `compile.model` is empty or not configured, the compile tool operates in prompt-only mode (returns synthesis prompts without calling an LLM).

## Invariants

1. `Synthesize` MUST NOT stream — returns complete text in one call
2. `Synthesize` MUST respect context cancellation (use `http.NewRequestWithContext`)
3. `Available()` MUST cache results for 30 seconds (same pattern as `OllamaEmbedder`)
4. HTTP timeout MUST be 120 seconds (generation is slower than embedding)
5. Response body MUST be capped at 50MB (same safety limit as `OllamaEmbedder`)
6. `NoopSynthesizer` MUST be exported for use in tool tests
7. The `llm` package MUST NOT import any Dewey packages (leaf dependency)
