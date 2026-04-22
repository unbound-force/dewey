package curate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
)

// DocumentContent holds a document's content for the extraction prompt.
// Each document represents one indexed page from a configured source.
type DocumentContent struct {
	SourceID string
	PageName string
	Content  string
}

// KnowledgeFile represents a curated knowledge artifact with full metadata.
// Written as a markdown file with YAML frontmatter to the store's output directory.
//
// Invariants:
//  1. Tag is non-empty, lowercase, hyphenated
//  2. Category is one of: decision, pattern, gotcha, context, reference
//  3. Confidence is one of: high, medium, low, flagged
//  4. Sources is non-empty — every fact traces to its origin (FR-010)
//  5. Tier is always "curated"
//  6. Content is the markdown body text (not in YAML frontmatter)
type KnowledgeFile struct {
	Tag          string        `json:"tag"          yaml:"tag"`
	Category     string        `json:"category"     yaml:"category"`
	Confidence   string        `json:"confidence"   yaml:"confidence"`
	QualityFlags []QualityFlag `json:"quality_flags,omitempty" yaml:"quality_flags,omitempty"`
	Sources      []SourceRef   `json:"sources"      yaml:"sources"`
	StoreName    string        `json:"store"        yaml:"store"`
	CreatedAt    string        `json:"created_at"   yaml:"created_at"`
	Tier         string        `json:"tier"         yaml:"tier"`
	Content      string        `json:"content"      yaml:"-"`
}

// QualityFlag represents a quality issue detected during curation.
// Valid types: missing_rationale, implied_assumption, incongruent, unsupported_claim.
type QualityFlag struct {
	Type       string   `json:"type"                 yaml:"type"`
	Detail     string   `json:"detail"               yaml:"detail"`
	Sources    []string `json:"sources,omitempty"     yaml:"sources,omitempty"`
	Resolution string   `json:"resolution,omitempty"  yaml:"resolution,omitempty"`
}

// SourceRef traces a curated fact back to its source document.
type SourceRef struct {
	SourceID string `json:"source_id"          yaml:"source_id"`
	Document string `json:"document"           yaml:"document"`
	Section  string `json:"section,omitempty"  yaml:"section,omitempty"`
	Excerpt  string `json:"excerpt,omitempty"  yaml:"excerpt,omitempty"`
}

// CurationState tracks incremental curation progress per store.
// Persisted as .curation-state.json in the store's output directory.
type CurationState struct {
	LastCuratedAt     time.Time            `json:"last_curated_at"`
	SourceCheckpoints map[string]time.Time `json:"source_checkpoints"`
}

// CurationResult summarizes a curation run for a single store.
type CurationResult struct {
	StoreName     string `json:"store_name"`
	FilesCreated  int    `json:"files_created"`
	DocsProcessed int    `json:"docs_processed"`
	DocsSkipped   int    `json:"docs_skipped"`
	Error         string `json:"error,omitempty"`
}

// Pipeline is the main curation engine. It reads indexed content from the
// store, uses an LLM to extract structured knowledge, and writes curated
// markdown files.
//
// Design decision: Dependencies are injected (Dependency Inversion Principle)
// following the same pattern as tools.Compile. The synth may be nil — in
// that case, the pipeline returns extraction prompts without synthesis,
// enabling the calling agent to perform synthesis externally.
type Pipeline struct {
	store     *store.Store
	synth     llm.Synthesizer
	embedder  embed.Embedder
	vaultPath string
}

// NewPipeline creates a new curation pipeline.
// store must be non-nil. synth may be nil (returns prompts without synthesis).
// embedder may be nil (skips embedding generation for curated files).
func NewPipeline(s *store.Store, synth llm.Synthesizer, e embed.Embedder, vaultPath string) *Pipeline {
	return &Pipeline{store: s, synth: synth, embedder: e, vaultPath: vaultPath}
}

// CurateStore runs the full curation pipeline for a single store.
// Processes all documents from configured sources regardless of checkpoint.
// Returns the number of knowledge files created and any error.
func (p *Pipeline) CurateStore(ctx context.Context, cfg StoreConfig) (int, error) {
	return p.curateStoreInternal(ctx, cfg, false)
}

// CurateStoreIncremental runs incremental curation — only processes
// documents updated since the last checkpoint.
func (p *Pipeline) CurateStoreIncremental(ctx context.Context, cfg StoreConfig) (int, error) {
	return p.curateStoreInternal(ctx, cfg, true)
}

// curateStoreInternal is the shared implementation for full and incremental curation.
func (p *Pipeline) curateStoreInternal(ctx context.Context, cfg StoreConfig, incremental bool) (int, error) {
	if p.store == nil {
		return 0, fmt.Errorf("store is required for curation")
	}

	if len(cfg.Sources) == 0 {
		curateLogger.Warn("store has no sources, skipping", "store", cfg.Name)
		return 0, nil
	}

	storePath := ResolveStorePath(cfg, p.vaultPath)

	// Load curation checkpoint for incremental mode.
	var state CurationState
	if incremental {
		var err error
		state, err = LoadCurationState(storePath)
		if err != nil {
			curateLogger.Warn("failed to load curation state, running full curation",
				"store", cfg.Name, "err", err)
		}
	}

	// Load source content from the store.
	documents, skipped := p.loadSourceContent(cfg, state, incremental)
	if len(documents) == 0 {
		curateLogger.Info("no documents to process", "store", cfg.Name, "skipped", skipped)
		return 0, nil
	}

	// Build extraction prompt.
	prompt := p.BuildExtractionPrompt(documents)

	// If no synthesizer is available, return an error with the prompt
	// so the caller can perform synthesis externally.
	if p.synth == nil || !p.synth.Available() {
		return 0, fmt.Errorf("extraction_prompt:%s", prompt)
	}

	// Call LLM for extraction.
	curateLogger.Info("extracting knowledge", "store", cfg.Name, "documents", len(documents))
	response, err := p.synth.Synthesize(ctx, prompt)
	if err != nil {
		return 0, fmt.Errorf("LLM extraction failed: %w", err)
	}

	// Parse LLM response into knowledge files.
	files, err := ParseExtractionResponse(response)
	if err != nil {
		return 0, fmt.Errorf("parse extraction response: %w", err)
	}

	// Write knowledge files.
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return 0, fmt.Errorf("create store directory: %w", err)
	}

	filesCreated := 0
	for i, file := range files {
		file.StoreName = cfg.Name
		file.Tier = "curated"
		file.CreatedAt = time.Now().UTC().Format(time.RFC3339)

		if _, err := WriteKnowledgeFile(file, storePath, i+1); err != nil {
			curateLogger.Warn("failed to write knowledge file",
				"tag", file.Tag, "err", err)
			continue
		}
		filesCreated++
	}

	// Write index file.
	if err := writeStoreIndex(files, storePath); err != nil {
		curateLogger.Warn("failed to write index file", "err", err)
	}

	// Save curation checkpoint.
	newState := CurationState{
		LastCuratedAt:     time.Now(),
		SourceCheckpoints: make(map[string]time.Time),
	}
	for _, srcID := range cfg.Sources {
		newState.SourceCheckpoints[srcID] = time.Now()
	}
	if err := SaveCurationState(newState, storePath); err != nil {
		curateLogger.Warn("failed to save curation state", "err", err)
	}

	return filesCreated, nil
}

// loadSourceContent loads indexed documents for the configured source IDs.
// In incremental mode, only documents updated after the checkpoint are loaded.
// Returns the documents and the count of skipped (already-processed) documents.
func (p *Pipeline) loadSourceContent(cfg StoreConfig, state CurationState, incremental bool) ([]DocumentContent, int) {
	var documents []DocumentContent
	skipped := 0

	for _, srcID := range cfg.Sources {
		srcID = strings.TrimSpace(srcID)

		pages, err := p.store.ListPagesBySource(srcID)
		if err != nil {
			curateLogger.Warn("failed to list pages for source",
				"source", srcID, "err", err)
			continue
		}

		for _, page := range pages {
			// In incremental mode, skip pages that haven't been updated
			// since the last checkpoint for this source.
			if incremental {
				if checkpoint, ok := state.SourceCheckpoints[srcID]; ok {
					pageUpdated := time.UnixMilli(page.UpdatedAt)
					if !pageUpdated.After(checkpoint) {
						skipped++
						continue
					}
				}
			}

			// Load block content for this page.
			blocks, err := p.store.GetBlocksByPage(page.Name)
			if err != nil {
				curateLogger.Warn("failed to load blocks",
					"page", page.Name, "err", err)
				continue
			}

			var contentParts []string
			for _, b := range blocks {
				if strings.TrimSpace(b.Content) != "" {
					contentParts = append(contentParts, b.Content)
				}
			}

			if len(contentParts) > 0 {
				documents = append(documents, DocumentContent{
					SourceID: srcID,
					PageName: page.Name,
					Content:  strings.Join(contentParts, "\n"),
				})
			}
		}
	}

	return documents, skipped
}

// BuildExtractionPrompt builds the LLM prompt for knowledge extraction
// from the given documents. Exported for MCP tool mode (nil synthesizer).
//
// The prompt instructs the LLM to extract structured knowledge items with
// tags, categories, confidence scores, quality flags, and source traceability.
// The LLM is instructed to return a JSON array for reliable parsing.
func (p *Pipeline) BuildExtractionPrompt(documents []DocumentContent) string {
	var sb strings.Builder

	sb.WriteString("You are a knowledge curator. Analyze the following documents and extract key knowledge.\n\n")
	sb.WriteString("For each piece of knowledge, classify it as one of:\n")
	sb.WriteString("- **decision**: An explicit choice or agreement made\n")
	sb.WriteString("- **pattern**: An approach, technique, or practice that worked\n")
	sb.WriteString("- **gotcha**: A pitfall, warning, or thing to watch out for\n")
	sb.WriteString("- **context**: A stated constraint, fact, or background information\n")
	sb.WriteString("- **reference**: A link, citation, or external resource\n\n")

	sb.WriteString("For each extracted item, provide:\n")
	sb.WriteString("1. **tag**: A topic identifier (1-3 words, kebab-case, lowercase)\n")
	sb.WriteString("2. **category**: One of decision/pattern/gotcha/context/reference\n")
	sb.WriteString("3. **confidence**: One of:\n")
	sb.WriteString("   - high: Explicit statement, no contradictions, multiple sources agree\n")
	sb.WriteString("   - medium: Explicit but single-source\n")
	sb.WriteString("   - low: Implied or contradictions exist\n")
	sb.WriteString("   - flagged: Missing critical info or unresolvable contradictions\n")
	sb.WriteString("4. **quality_flags**: Array of quality issues (may be empty). Each flag has:\n")
	sb.WriteString("   - type: missing_rationale | implied_assumption | incongruent | unsupported_claim\n")
	sb.WriteString("   - detail: Description of the issue\n")
	sb.WriteString("   - sources: Source documents involved (optional)\n")
	sb.WriteString("   - resolution: Suggested resolution (optional)\n")
	sb.WriteString("5. **sources**: Array of source references. Each has:\n")
	sb.WriteString("   - source_id: The source identifier\n")
	sb.WriteString("   - document: The document name\n")
	sb.WriteString("   - excerpt: Relevant text from the document\n")
	sb.WriteString("6. **content**: The extracted knowledge as a clear, factual statement\n\n")

	sb.WriteString("Quality analysis rules:\n")
	sb.WriteString("- Flag 'missing_rationale' for decisions without explanation of why\n")
	sb.WriteString("- Flag 'implied_assumption' for unstated assumptions\n")
	sb.WriteString("- Flag 'incongruent' for contradictions between sources (include both source refs)\n")
	sb.WriteString("- Flag 'unsupported_claim' for facts without evidence\n")
	sb.WriteString("- For contradictions, prefer the newer source (temporal resolution)\n\n")

	sb.WriteString("Return a JSON array of objects. Example:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`[
  {
    "tag": "authentication",
    "category": "decision",
    "confidence": "high",
    "quality_flags": [],
    "sources": [{"source_id": "disk-meetings", "document": "sprint-planning", "excerpt": "Team decided OAuth2"}],
    "content": "Use OAuth2 for authentication (changed from API keys)."
  }
]
`)
	sb.WriteString("```\n\n")

	sb.WriteString("## Documents\n\n")
	for i, doc := range documents {
		sb.WriteString(fmt.Sprintf("### Document %d: %s (source: %s)\n\n", i+1, doc.PageName, doc.SourceID))
		sb.WriteString(doc.Content)
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString("Extract all knowledge items from the documents above. Return ONLY the JSON array, no other text.\n")

	return sb.String()
}

// ParseExtractionResponse parses the LLM's JSON response into KnowledgeFile structs.
// Handles JSON embedded in markdown code blocks (```json ... ```).
// Validates required fields and returns an error for completely malformed responses.
func ParseExtractionResponse(response string) ([]KnowledgeFile, error) {
	// Strip markdown code block fences if present.
	cleaned := strings.TrimSpace(response)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	var files []KnowledgeFile
	if err := json.Unmarshal([]byte(cleaned), &files); err != nil {
		return nil, fmt.Errorf("parse LLM response as JSON array: %w", err)
	}

	// Validate required fields.
	var valid []KnowledgeFile
	for _, f := range files {
		if f.Tag == "" {
			curateLogger.Warn("skipping knowledge item with empty tag")
			continue
		}
		if f.Category == "" {
			curateLogger.Warn("skipping knowledge item with empty category", "tag", f.Tag)
			continue
		}
		if f.Content == "" {
			curateLogger.Warn("skipping knowledge item with empty content", "tag", f.Tag)
			continue
		}
		// Default confidence to "medium" if not specified.
		if f.Confidence == "" {
			f.Confidence = "medium"
		}
		valid = append(valid, f)
	}

	return valid, nil
}

// WriteKnowledgeFile writes a curated knowledge file to the store's directory.
// Creates the directory if it doesn't exist. The file is written as markdown
// with YAML frontmatter containing all metadata fields.
// Returns the file path on success.
func WriteKnowledgeFile(file KnowledgeFile, storePath string, seq int) (string, error) {
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return "", fmt.Errorf("create store directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%d.md", file.Tag, seq)
	filePath := filepath.Join(storePath, filename)

	// Build YAML frontmatter using fmt.Sprintf for simple key-value pairs
	// and json.Marshal for complex nested structures (quality_flags, sources).
	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("tag: %s\n", file.Tag))
	buf.WriteString(fmt.Sprintf("category: %s\n", file.Category))
	buf.WriteString(fmt.Sprintf("confidence: %s\n", file.Confidence))
	buf.WriteString(fmt.Sprintf("created_at: %s\n", file.CreatedAt))
	if file.StoreName != "" {
		buf.WriteString(fmt.Sprintf("store: %s\n", file.StoreName))
	}
	buf.WriteString(fmt.Sprintf("tier: %s\n", file.Tier))

	// Write sources as YAML list.
	if len(file.Sources) > 0 {
		buf.WriteString("sources:\n")
		for _, src := range file.Sources {
			buf.WriteString(fmt.Sprintf("  - source_id: %s\n", src.SourceID))
			buf.WriteString(fmt.Sprintf("    document: %s\n", src.Document))
			if src.Section != "" {
				buf.WriteString(fmt.Sprintf("    section: %s\n", src.Section))
			}
			if src.Excerpt != "" {
				buf.WriteString(fmt.Sprintf("    excerpt: %q\n", src.Excerpt))
			}
		}
	}

	// Write quality flags as YAML list.
	if len(file.QualityFlags) > 0 {
		buf.WriteString("quality_flags:\n")
		for _, flag := range file.QualityFlags {
			buf.WriteString(fmt.Sprintf("  - type: %s\n", flag.Type))
			buf.WriteString(fmt.Sprintf("    detail: %q\n", flag.Detail))
			if len(flag.Sources) > 0 {
				buf.WriteString("    sources:\n")
				for _, s := range flag.Sources {
					buf.WriteString(fmt.Sprintf("      - %s\n", s))
				}
			}
			if flag.Resolution != "" {
				buf.WriteString(fmt.Sprintf("    resolution: %q\n", flag.Resolution))
			}
		}
	} else {
		buf.WriteString("quality_flags: []\n")
	}

	buf.WriteString("---\n\n")
	buf.WriteString(file.Content)
	buf.WriteString("\n")

	if err := os.WriteFile(filePath, []byte(buf.String()), 0o644); err != nil {
		return "", fmt.Errorf("write knowledge file %q: %w", filePath, err)
	}

	curateLogger.Debug("knowledge file written", "path", filePath, "tag", file.Tag)
	return filePath, nil
}

// LoadCurationState reads the curation checkpoint from the store's directory.
// Returns a zero-value CurationState if the file doesn't exist (first run).
func LoadCurationState(storePath string) (CurationState, error) {
	statePath := filepath.Join(storePath, ".curation-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return CurationState{SourceCheckpoints: make(map[string]time.Time)}, nil
		}
		return CurationState{}, fmt.Errorf("read curation state: %w", err)
	}

	var state CurationState
	if err := json.Unmarshal(data, &state); err != nil {
		return CurationState{}, fmt.Errorf("parse curation state: %w", err)
	}
	if state.SourceCheckpoints == nil {
		state.SourceCheckpoints = make(map[string]time.Time)
	}
	return state, nil
}

// SaveCurationState writes the curation checkpoint to the store's directory.
// Creates the directory if it doesn't exist.
func SaveCurationState(state CurationState, storePath string) error {
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal curation state: %w", err)
	}

	statePath := filepath.Join(storePath, ".curation-state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return fmt.Errorf("write curation state: %w", err)
	}
	return nil
}

// writeStoreIndex generates the _index.md file listing all knowledge files
// with their tags, categories, and confidence levels.
func writeStoreIndex(files []KnowledgeFile, storePath string) error {
	var sb strings.Builder
	sb.WriteString("# Knowledge Store Index\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().UTC().Format(time.RFC3339)))

	if len(files) == 0 {
		sb.WriteString("No knowledge files curated yet.\n")
	} else {
		sb.WriteString("| Tag | Category | Confidence | Quality Flags |\n")
		sb.WriteString("|-----|----------|------------|---------------|\n")
		for i, f := range files {
			flagCount := len(f.QualityFlags)
			sb.WriteString(fmt.Sprintf("| [%s](%s-%d.md) | %s | %s | %d |\n",
				f.Tag, f.Tag, i+1, f.Category, f.Confidence, flagCount))
		}
	}

	indexPath := filepath.Join(storePath, "_index.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0o644)
}
