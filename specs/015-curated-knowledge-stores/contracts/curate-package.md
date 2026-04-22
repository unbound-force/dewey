# Contract: curate/ Package

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21

## Package Overview

The `curate` package provides knowledge store configuration parsing and the curation pipeline for extracting structured knowledge from indexed sources. It is a leaf dependency — it imports `store`, `llm`, `embed`, and stdlib only.

## Types

### StoreConfig

```go
// StoreConfig represents a single knowledge store entry from knowledge-stores.yaml.
type StoreConfig struct {
    Name             string   `yaml:"name"`
    Sources          []string `yaml:"sources"`
    Path             string   `yaml:"path"`
    CurateOnIndex    bool     `yaml:"curate_on_index"`
    CurationInterval string   `yaml:"curation_interval"`
}
```

**Invariants**:
1. `Name` is non-empty and unique across stores
2. `Sources` is non-empty (stores with empty sources are skipped with a warning)
3. `Path` defaults to `.uf/dewey/knowledge/{Name}` when empty
4. `CurationInterval` defaults to `"10m"` when empty
5. `CurateOnIndex` defaults to `false`

### KnowledgeFile

```go
// KnowledgeFile represents a curated knowledge artifact with full metadata.
type KnowledgeFile struct {
    Tag          string        `yaml:"tag"`
    Category     string        `yaml:"category"`
    Confidence   string        `yaml:"confidence"`
    QualityFlags []QualityFlag `yaml:"quality_flags,omitempty"`
    Sources      []SourceRef   `yaml:"sources"`
    StoreName    string        `yaml:"store"`
    CreatedAt    string        `yaml:"created_at"`
    Tier         string        `yaml:"tier"`
    Content      string        `yaml:"-"`
}
```

**Invariants**:
1. `Tag` is non-empty, lowercase, hyphenated
2. `Category` is one of: decision, pattern, gotcha, context, reference
3. `Confidence` is one of: high, medium, low, flagged
4. `Sources` is non-empty — every fact traces to its origin (FR-010)
5. `Tier` is always `"curated"`
6. `Content` is the markdown body text (not in YAML frontmatter)

### QualityFlag

```go
// QualityFlag represents a quality issue detected during curation.
type QualityFlag struct {
    Type       string   `yaml:"type"`
    Detail     string   `yaml:"detail"`
    Sources    []string `yaml:"sources,omitempty"`
    Resolution string   `yaml:"resolution,omitempty"`
}
```

**Valid types**: `missing_rationale`, `implied_assumption`, `incongruent`, `unsupported_claim`

### SourceRef

```go
// SourceRef traces a curated fact back to its source document.
type SourceRef struct {
    SourceID string `yaml:"source_id"`
    Document string `yaml:"document"`
    Section  string `yaml:"section,omitempty"`
    Excerpt  string `yaml:"excerpt,omitempty"`
}
```

### CurationState

```go
// CurationState tracks incremental curation progress per store.
type CurationState struct {
    LastCuratedAt     time.Time            `json:"last_curated_at"`
    SourceCheckpoints map[string]time.Time `json:"source_checkpoints"`
}
```

### Pipeline

```go
// Pipeline is the main curation engine.
type Pipeline struct {
    store     *store.Store
    synth     llm.Synthesizer
    embedder  embed.Embedder
    vaultPath string
}
```

## Functions

### Config Functions

```go
// LoadKnowledgeStoresConfig reads and parses knowledge-stores.yaml.
// Returns (nil, nil) if the file does not exist.
// Returns an error if the file is malformed or validation fails.
func LoadKnowledgeStoresConfig(path string) ([]StoreConfig, error)

// ResolveStorePath returns the absolute path for a store's output directory.
// If cfg.Path is empty, defaults to {vaultPath}/.uf/dewey/knowledge/{cfg.Name}.
// If cfg.Path is relative, resolves against vaultPath.
func ResolveStorePath(cfg StoreConfig, vaultPath string) string

// ParseCurationInterval parses the curation interval string.
// Returns 10*time.Minute for empty string (default).
// Delegates to source.ParseRefreshInterval for parsing.
func ParseCurationInterval(interval string) (time.Duration, error)
```

### Pipeline Functions

```go
// NewPipeline creates a new curation pipeline.
// store must be non-nil. synth may be nil (returns prompts without synthesis).
// embedder may be nil (skips embedding generation for curated files).
func NewPipeline(s *store.Store, synth llm.Synthesizer, e embed.Embedder, vaultPath string) *Pipeline

// CurateStore runs the curation pipeline for a single store.
// Returns the number of knowledge files created and any error.
// When synth is nil, returns 0 files and a CurationPrompt in the error
// (the caller performs synthesis).
func (p *Pipeline) CurateStore(ctx context.Context, cfg StoreConfig) (int, error)

// CurateStoreIncremental runs incremental curation — only processes
// documents updated since the last checkpoint.
func (p *Pipeline) CurateStoreIncremental(ctx context.Context, cfg StoreConfig) (int, error)

// BuildExtractionPrompt builds the LLM prompt for knowledge extraction
// from the given documents. Exported for MCP tool mode (nil synthesizer).
func (p *Pipeline) BuildExtractionPrompt(documents []DocumentContent) string

// ParseExtractionResponse parses the LLM's JSON response into KnowledgeFile structs.
func ParseExtractionResponse(response string) ([]KnowledgeFile, error)

// WriteKnowledgeFile writes a curated knowledge file to the store's directory.
// Creates the directory if it doesn't exist.
func WriteKnowledgeFile(file KnowledgeFile, storePath string, seq int) (string, error)

// LoadCurationState reads the curation checkpoint from the store's directory.
// Returns a zero-value CurationState if the file doesn't exist.
func LoadCurationState(storePath string) (CurationState, error)

// SaveCurationState writes the curation checkpoint to the store's directory.
func SaveCurationState(state CurationState, storePath string) error
```

### Helper Types

```go
// DocumentContent holds a document's content for the extraction prompt.
type DocumentContent struct {
    SourceID string
    PageName string
    Content  string
}

// CurationResult summarizes a curation run.
type CurationResult struct {
    StoreName     string `json:"store_name"`
    FilesCreated  int    `json:"files_created"`
    DocsProcessed int    `json:"docs_processed"`
    DocsSkipped   int    `json:"docs_skipped"`
    Error         string `json:"error,omitempty"`
}
```

## Error Handling

- Config parsing errors are returned immediately (fail-fast)
- Missing source IDs in config log a warning and skip the source (graceful)
- LLM failures return an error but don't affect existing knowledge files (FR-009 AS4)
- File write failures return an error with the file path for diagnostics
- Checkpoint save failures log a warning (next run will re-process — safe)

## Testing Strategy

- **Config tests**: Valid/invalid YAML, missing fields, default values, source validation
- **Pipeline tests**: Use `llm.NoopSynthesizer` with pre-built JSON responses
- **File writing tests**: Use `t.TempDir()`, verify YAML frontmatter and content
- **Checkpoint tests**: Read/write/missing file scenarios
- **Prompt tests**: Verify prompt structure includes all document content
