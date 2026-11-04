package curate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/llm"
	"github.com/unbound-force/dewey/v3/sanitize"
	"github.com/unbound-force/dewey/v3/store"
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
	sb.WriteString("IMPORTANT: Be concise. Extract only the most significant and non-obvious knowledge items.\n")
	sb.WriteString("Aim for 3-8 items per document maximum. Skip obvious or generic facts.\n\n")
	sb.WriteString("For each piece of knowledge, classify it as one of:\n")
	sb.WriteString("- **decision**: An explicit choice or agreement made\n")
	sb.WriteString("- **pattern**: An approach, technique, or practice that worked\n")
	sb.WriteString("- **gotcha**: A pitfall, warning, or thing to watch out for\n")
	sb.WriteString("- **context**: A stated constraint, fact, or background information\n")
	sb.WriteString("- **reference**: A link, citation, or external resource\n\n")

	sb.WriteString("For each extracted item, provide:\n")
	sb.WriteString("1. **tag**: A topic identifier (1-3 words, kebab-case, lowercase)\n")
	sb.WriteString("2. **category**: One of decision/pattern/gotcha/context/reference\n")
	sb.WriteString("3. **confidence**: One of high/medium/low/flagged\n")
	sb.WriteString("4. **quality_flags**: Empty array [] unless there is a clear contradiction\n")
	sb.WriteString("5. **sources**: ONE source reference only (most relevant). Each has source_id, document, and a SHORT excerpt (max 20 words)\n")
	sb.WriteString("6. **content**: The extracted knowledge as ONE clear sentence (max 30 words)\n\n")

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
		fmt.Fprintf(&sb, "### Document %d: %s (source: %s)\n\n", i+1, doc.PageName, doc.SourceID)
		sb.WriteString(doc.Content)
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString("Extract all knowledge items from the documents above. Return ONLY the JSON array, no other text.\n")

	return sb.String()
}

// isValidKnowledgeType reports whether t is one of the documented quality
// flag types. A flag with an unexpected type is treated as unsafe: the type
// is interpolated into frontmatter as a plain scalar, so anything outside
// this enum (including values carrying control characters) is rejected.
func isValidKnowledgeType(t string) bool {
	switch t {
	case "missing_rationale", "implied_assumption", "incongruent", "unsupported_claim":
		return true
	}
	return false
}

// validateKnowledgeMetadata validates the fields that become filenames or
// single-line YAML scalars, and normalizes confidence. It is the single
// gate shared by LLM-extraction parsing and file writing so the two paths
// cannot drift apart (PR 145 review amendment). Callers decide the failure
// policy: extraction skips the item with a warning, writing returns the
// error.
func validateKnowledgeMetadata(f *KnowledgeFile) error {
	if f.Tag == "" || f.Category == "" || f.Content == "" {
		return fmt.Errorf("knowledge item has missing required fields (tag, category, content)")
	}
	if !sanitize.ValidTag(f.Tag) {
		return fmt.Errorf("knowledge tag %q is unsafe", f.Tag)
	}
	if !sanitize.SafeFrontmatterScalar(f.Category) || !sanitize.SafeFrontmatterScalar(f.Confidence) ||
		!sanitize.SafeFrontmatterScalar(f.StoreName) || !sanitize.SafeFrontmatterScalar(f.CreatedAt) ||
		!sanitize.SafeFrontmatterScalar(f.Tier) {
		return fmt.Errorf("knowledge metadata for %q contains control characters", f.Tag)
	}
	for _, src := range f.Sources {
		if !sanitize.SafeFrontmatterScalar(src.SourceID) || !sanitize.SafeFrontmatterScalar(src.Document) || !sanitize.SafeFrontmatterScalar(src.Section) {
			return fmt.Errorf("knowledge source metadata for %q contains control characters", f.Tag)
		}
	}
	for _, flag := range f.QualityFlags {
		if !isValidKnowledgeType(flag.Type) || !sanitize.SafeFrontmatterScalar(flag.Type) {
			return fmt.Errorf("knowledge quality flag type %q is invalid", flag.Type)
		}
		for _, s := range flag.Sources {
			if !sanitize.SafeFrontmatterScalar(s) {
				return fmt.Errorf("knowledge quality flag sources for %q contain unsafe values", f.Tag)
			}
		}
	}
	switch f.Confidence {
	case "high", "medium", "low", "flagged":
	default:
		f.Confidence = "medium"
	}
	return nil
}

// ParseExtractionResponse parses the LLM's JSON response into KnowledgeFile structs.
// Handles JSON embedded in markdown code blocks (```json ... ```).
// Validates required fields and returns an error for completely malformed responses.
// Silently skips individual items that cannot be parsed after fallback attempts.
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

	// Parse into a raw intermediate to tolerate quality_flags being a string
	// instead of an array (LLMs sometimes output "[]" or "none" as strings).
	var rawFiles []json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &rawFiles); err != nil {
		return nil, fmt.Errorf("parse LLM response as JSON array: %w", err)
	}

	var files []KnowledgeFile
	for _, raw := range rawFiles {
		// Try direct unmarshal first.
		var f KnowledgeFile
		if err := json.Unmarshal(raw, &f); err != nil {
			// quality_flags may be a string (e.g. "none") instead of an
			// array — some LLMs emit this. Drop the field and retry so
			// the rest of the item still parses; empty quality_flags is
			// an acceptable fallback for a value we can't model.
			var fields map[string]json.RawMessage
			if err2 := json.Unmarshal(raw, &fields); err2 != nil {
				curateLogger.Warn("skipping unparseable knowledge item", "err", err2)
				continue
			}
			delete(fields, "quality_flags")
			retry, err2 := json.Marshal(fields)
			if err2 != nil {
				curateLogger.Warn("skipping unparseable knowledge item", "err", err2)
				continue
			}
			if err2 := json.Unmarshal(retry, &f); err2 != nil {
				curateLogger.Warn("skipping unparseable knowledge item", "err", err2)
				continue
			}
		}
		files = append(files, f)
	}

	// Validate required fields.
	var valid []KnowledgeFile
	for _, f := range files {
		if err := validateKnowledgeMetadata(&f); err != nil {
			curateLogger.Warn("skipping knowledge item", "err", err, "tag", f.Tag)
			continue
		}
		valid = append(valid, f)
	}
	return valid, nil
}

// renderKnowledgeFileContent renders knowledge file frontmatter and body.
// Scalar fields are rendered with %q (double-quoted YAML scalars) so values
// containing YAML-significant characters such as ": " or " #" cannot
// restructure the document (PR 145 review amendment); the vault frontmatter
// parser and lint's manual parser both tolerate quoted scalars.
func renderKnowledgeFileContent(file KnowledgeFile) string {
	var buf strings.Builder
	buf.WriteString("---\n")
	fmt.Fprintf(&buf, "tag: %s\n", file.Tag)
	fmt.Fprintf(&buf, "category: %q\n", file.Category)
	fmt.Fprintf(&buf, "confidence: %q\n", file.Confidence)
	fmt.Fprintf(&buf, "created_at: %q\n", file.CreatedAt)
	if file.StoreName != "" {
		fmt.Fprintf(&buf, "store: %q\n", file.StoreName)
	}
	fmt.Fprintf(&buf, "tier: %q\n", file.Tier)
	buf.WriteString(renderKnowledgeSources(file.Sources))
	buf.WriteString(renderKnowledgeQualityFlags(file.QualityFlags))
	buf.WriteString("---\n\n")
	buf.WriteString(file.Content)
	buf.WriteString("\n")
	return buf.String()
}

// renderKnowledgeSources renders the optional sources YAML list.
func renderKnowledgeSources(sources []SourceRef) string {
	if len(sources) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("sources:\n")
	for _, src := range sources {
		fmt.Fprintf(&buf, "  - source_id: %q\n", src.SourceID)
		fmt.Fprintf(&buf, "    document: %q\n", src.Document)
		if src.Section != "" {
			fmt.Fprintf(&buf, "    section: %q\n", src.Section)
		}
		if src.Excerpt != "" {
			fmt.Fprintf(&buf, "    excerpt: %q\n", src.Excerpt)
		}
	}
	return buf.String()
}

// renderKnowledgeQualityFlags renders the quality_flags YAML list.
func renderKnowledgeQualityFlags(flags []QualityFlag) string {
	if len(flags) == 0 {
		return "quality_flags: []\n"
	}
	var buf strings.Builder
	buf.WriteString("quality_flags:\n")
	for _, flag := range flags {
		fmt.Fprintf(&buf, "  - type: %q\n", flag.Type)
		fmt.Fprintf(&buf, "    detail: %q\n", flag.Detail)
		if len(flag.Sources) > 0 {
			buf.WriteString("    sources:\n")
			for _, s := range flag.Sources {
				fmt.Fprintf(&buf, "      - %q\n", s)
			}
		}
		if flag.Resolution != "" {
			fmt.Fprintf(&buf, "    resolution: %q\n", flag.Resolution)
		}
	}
	return buf.String()
}

// WriteKnowledgeFile writes a curated knowledge file to the store's directory.
// Creates the directory if it doesn't exist. The file is written as markdown
// with YAML frontmatter containing all metadata fields.
// Returns the file path on success.
func WriteKnowledgeFile(file KnowledgeFile, storePath string, seq int) (string, error) {
	if err := validateKnowledgeMetadata(&file); err != nil {
		return "", err
	}
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return "", fmt.Errorf("create store directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%d.md", file.Tag, seq)
	filePath := filepath.Join(storePath, filename)
	if filepath.Base(filePath) != filename {
		return "", fmt.Errorf("invalid knowledge filename %q", filename)
	}

	content := renderKnowledgeFileContent(file)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
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
	fmt.Fprintf(&sb, "*Generated: %s*\n\n", time.Now().UTC().Format(time.RFC3339))

	if len(files) == 0 {
		sb.WriteString("No knowledge files curated yet.\n")
	} else {
		sb.WriteString("| Tag | Category | Confidence | Quality Flags |\n")
		sb.WriteString("|-----|----------|------------|---------------|\n")
		for i, f := range files {
			flagCount := len(f.QualityFlags)
			fmt.Fprintf(&sb, "| [%s](%s-%d.md) | %s | %s | %d |\n",
				f.Tag, f.Tag, i+1, f.Category, f.Confidence, flagCount)
		}
	}

	indexPath := filepath.Join(storePath, "_index.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0o644)
}
