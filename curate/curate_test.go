package curate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
)

// mockLLMResponse returns a valid JSON array of knowledge items for testing.
func mockLLMResponse() string {
	items := []KnowledgeFile{
		{
			Tag:        "authentication",
			Category:   "decision",
			Confidence: "high",
			Sources: []SourceRef{
				{SourceID: "disk-meetings", Document: "sprint-planning", Excerpt: "Team decided OAuth2"},
			},
			Content: "Use OAuth2 for authentication (changed from API keys).",
		},
		{
			Tag:        "deployment",
			Category:   "pattern",
			Confidence: "medium",
			QualityFlags: []QualityFlag{
				{Type: "missing_rationale", Detail: "No explanation for choosing blue-green deployment"},
			},
			Sources: []SourceRef{
				{SourceID: "disk-meetings", Document: "architecture-review", Excerpt: "Blue-green deployment"},
			},
			Content: "Use blue-green deployment for zero-downtime releases.",
		},
	}
	data, _ := json.Marshal(items)
	return string(data)
}

// setupTestStore creates an in-memory store with test pages and blocks.
func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}

	// Insert test pages for source "disk-meetings".
	pages := []struct {
		name      string
		docID     string
		content   string
	}{
		{"disk-meetings/sprint-planning", "sprint-planning", "Team decided to use OAuth2 for authentication."},
		{"disk-meetings/architecture-review", "architecture-review", "We will use blue-green deployment for releases."},
	}

	for _, p := range pages {
		if err := s.InsertPage(&store.Page{
			Name:        p.name,
			SourceID:    "disk-meetings",
			SourceDocID: p.docID,
		}); err != nil {
			t.Fatalf("insert page %q: %v", p.name, err)
		}
		if err := s.InsertBlock(&store.Block{
			UUID:     "block-" + p.name,
			PageName: p.name,
			Content:  p.content,
			Position: 0,
		}); err != nil {
			t.Fatalf("insert block for %q: %v", p.name, err)
		}
	}

	return s
}

func TestBuildExtractionPrompt_IncludesAllDocuments(t *testing.T) {
	pipeline := NewPipeline(nil, nil, nil, "")

	documents := []DocumentContent{
		{SourceID: "disk-meetings", PageName: "sprint-planning", Content: "OAuth2 decision"},
		{SourceID: "disk-docs", PageName: "architecture", Content: "Deployment patterns"},
	}

	prompt := pipeline.BuildExtractionPrompt(documents)

	// Verify all documents are included in the prompt.
	if !strings.Contains(prompt, "OAuth2 decision") {
		t.Error("prompt missing first document content")
	}
	if !strings.Contains(prompt, "Deployment patterns") {
		t.Error("prompt missing second document content")
	}
	if !strings.Contains(prompt, "disk-meetings") {
		t.Error("prompt missing first source ID")
	}
	if !strings.Contains(prompt, "disk-docs") {
		t.Error("prompt missing second source ID")
	}
	// Verify prompt includes extraction instructions.
	if !strings.Contains(prompt, "knowledge curator") {
		t.Error("prompt missing curator instructions")
	}
	if !strings.Contains(prompt, "JSON array") {
		t.Error("prompt missing JSON format instruction")
	}
}

func TestParseExtractionResponse_ValidJSON(t *testing.T) {
	response := mockLLMResponse()

	files, err := ParseExtractionResponse(response)
	if err != nil {
		t.Fatalf("ParseExtractionResponse: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	// Verify first item.
	if files[0].Tag != "authentication" {
		t.Errorf("files[0].Tag = %q, want %q", files[0].Tag, "authentication")
	}
	if files[0].Category != "decision" {
		t.Errorf("files[0].Category = %q, want %q", files[0].Category, "decision")
	}
	if files[0].Confidence != "high" {
		t.Errorf("files[0].Confidence = %q, want %q", files[0].Confidence, "high")
	}
	if len(files[0].Sources) != 1 {
		t.Errorf("files[0].Sources len = %d, want 1", len(files[0].Sources))
	}

	// Verify second item has quality flags.
	if len(files[1].QualityFlags) != 1 {
		t.Errorf("files[1].QualityFlags len = %d, want 1", len(files[1].QualityFlags))
	}
}

func TestParseExtractionResponse_MarkdownCodeBlock(t *testing.T) {
	response := "```json\n" + mockLLMResponse() + "\n```"

	files, err := ParseExtractionResponse(response)
	if err != nil {
		t.Fatalf("ParseExtractionResponse with code block: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
}

func TestParseExtractionResponse_MalformedJSON(t *testing.T) {
	_, err := ParseExtractionResponse("this is not json at all")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestParseExtractionResponse_MissingFields(t *testing.T) {
	// Items with missing required fields should be skipped.
	response := `[
		{"tag": "", "category": "decision", "content": "test"},
		{"tag": "valid", "category": "", "content": "test"},
		{"tag": "valid", "category": "decision", "content": ""},
		{"tag": "valid", "category": "decision", "content": "good item"}
	]`

	files, err := ParseExtractionResponse(response)
	if err != nil {
		t.Fatalf("ParseExtractionResponse: %v", err)
	}
	// Only the last item should survive validation.
	if len(files) != 1 {
		t.Fatalf("got %d valid files, want 1", len(files))
	}
	if files[0].Tag != "valid" {
		t.Errorf("files[0].Tag = %q, want %q", files[0].Tag, "valid")
	}
}

func TestParseExtractionResponse_DefaultConfidence(t *testing.T) {
	response := `[{"tag": "test", "category": "context", "content": "no confidence set"}]`

	files, err := ParseExtractionResponse(response)
	if err != nil {
		t.Fatalf("ParseExtractionResponse: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Confidence != "medium" {
		t.Errorf("default confidence = %q, want %q", files[0].Confidence, "medium")
	}
}

func TestWriteKnowledgeFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	file := KnowledgeFile{
		Tag:        "authentication",
		Category:   "decision",
		Confidence: "high",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		StoreName:  "test-store",
		Tier:       "curated",
		Sources: []SourceRef{
			{SourceID: "disk-meetings", Document: "sprint-planning", Excerpt: "Team decided OAuth2"},
		},
		Content: "Use OAuth2 for authentication.",
	}

	path, err := WriteKnowledgeFile(file, dir, 1)
	if err != nil {
		t.Fatalf("WriteKnowledgeFile: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created at %q: %v", path, err)
	}

	// Read and verify content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)

	// Verify frontmatter fields.
	if !strings.Contains(content, "tag: authentication") {
		t.Error("missing tag in frontmatter")
	}
	if !strings.Contains(content, "category: decision") {
		t.Error("missing category in frontmatter")
	}
	if !strings.Contains(content, "confidence: high") {
		t.Error("missing confidence in frontmatter")
	}
	if !strings.Contains(content, "tier: curated") {
		t.Error("missing tier in frontmatter")
	}
	if !strings.Contains(content, "store: test-store") {
		t.Error("missing store in frontmatter")
	}
	if !strings.Contains(content, "source_id: disk-meetings") {
		t.Error("missing source_id in frontmatter")
	}
	// Verify body content.
	if !strings.Contains(content, "Use OAuth2 for authentication.") {
		t.Error("missing body content")
	}
}

func TestWriteKnowledgeFile_QualityFlags(t *testing.T) {
	dir := t.TempDir()

	file := KnowledgeFile{
		Tag:        "deployment",
		Category:   "pattern",
		Confidence: "low",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Tier:       "curated",
		QualityFlags: []QualityFlag{
			{
				Type:       "missing_rationale",
				Detail:     "No explanation for choosing blue-green",
				Sources:    []string{"architecture-review"},
				Resolution: "Ask team lead for rationale",
			},
		},
		Sources: []SourceRef{
			{SourceID: "disk-docs", Document: "architecture"},
		},
		Content: "Blue-green deployment for releases.",
	}

	path, err := WriteKnowledgeFile(file, dir, 1)
	if err != nil {
		t.Fatalf("WriteKnowledgeFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "type: missing_rationale") {
		t.Error("missing quality flag type in frontmatter")
	}
	if !strings.Contains(content, "No explanation for choosing blue-green") {
		t.Error("missing quality flag detail in frontmatter")
	}
}

func TestWriteKnowledgeFile_ConfidenceScoring(t *testing.T) {
	dir := t.TempDir()

	levels := []string{"high", "medium", "low", "flagged"}
	for i, level := range levels {
		file := KnowledgeFile{
			Tag:        "test",
			Category:   "context",
			Confidence: level,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			Tier:       "curated",
			Sources:    []SourceRef{{SourceID: "test", Document: "test"}},
			Content:    "Test content.",
		}

		path, err := WriteKnowledgeFile(file, dir, i+1)
		if err != nil {
			t.Fatalf("WriteKnowledgeFile(%s): %v", level, err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if !strings.Contains(string(data), "confidence: "+level) {
			t.Errorf("missing confidence %q in file", level)
		}
	}
}

func TestWriteKnowledgeFile_SourceTraceability(t *testing.T) {
	dir := t.TempDir()

	file := KnowledgeFile{
		Tag:        "auth",
		Category:   "decision",
		Confidence: "high",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Tier:       "curated",
		Sources: []SourceRef{
			{SourceID: "disk-meetings", Document: "sprint-planning", Excerpt: "Team decided OAuth2"},
			{SourceID: "disk-docs", Document: "architecture", Section: "Auth"},
		},
		Content: "OAuth2 decision.",
	}

	path, err := WriteKnowledgeFile(file, dir, 1)
	if err != nil {
		t.Fatalf("WriteKnowledgeFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)

	// Verify both sources are present.
	if !strings.Contains(content, "source_id: disk-meetings") {
		t.Error("missing first source_id")
	}
	if !strings.Contains(content, "source_id: disk-docs") {
		t.Error("missing second source_id")
	}
	if !strings.Contains(content, "document: sprint-planning") {
		t.Error("missing first document name")
	}
	if !strings.Contains(content, "section: Auth") {
		t.Error("missing section field")
	}
}

func TestCurationState_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	state := CurationState{
		LastCuratedAt: time.Now().UTC().Truncate(time.Second),
		SourceCheckpoints: map[string]time.Time{
			"disk-meetings": time.Now().UTC().Truncate(time.Second),
			"disk-docs":     time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
		},
	}

	if err := SaveCurationState(state, dir); err != nil {
		t.Fatalf("SaveCurationState: %v", err)
	}

	loaded, err := LoadCurationState(dir)
	if err != nil {
		t.Fatalf("LoadCurationState: %v", err)
	}

	if !loaded.LastCuratedAt.Equal(state.LastCuratedAt) {
		t.Errorf("LastCuratedAt = %v, want %v", loaded.LastCuratedAt, state.LastCuratedAt)
	}
	if len(loaded.SourceCheckpoints) != 2 {
		t.Errorf("SourceCheckpoints len = %d, want 2", len(loaded.SourceCheckpoints))
	}
}

func TestLoadCurationState_MissingFile(t *testing.T) {
	state, err := LoadCurationState("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if state.SourceCheckpoints == nil {
		t.Fatal("expected non-nil SourceCheckpoints map")
	}
	if !state.LastCuratedAt.IsZero() {
		t.Errorf("expected zero LastCuratedAt, got %v", state.LastCuratedAt)
	}
}

func TestCurateStore_WithNoopSynthesizer(t *testing.T) {
	s := setupTestStore(t)
	defer func() { _ = s.Close() }()

	dir := t.TempDir()

	synth := &llm.NoopSynthesizer{
		Response: mockLLMResponse(),
		Avail:    true,
		Model:    "test-model",
	}

	pipeline := NewPipeline(s, synth, nil, dir)

	cfg := StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-meetings"},
	}

	filesCreated, err := pipeline.CurateStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CurateStore: %v", err)
	}
	if filesCreated != 2 {
		t.Errorf("filesCreated = %d, want 2", filesCreated)
	}

	// Verify knowledge files were created.
	storePath := ResolveStorePath(cfg, dir)
	entries, err := os.ReadDir(storePath)
	if err != nil {
		t.Fatalf("read store dir: %v", err)
	}

	// Should have: authentication-1.md, deployment-2.md, _index.md, .curation-state.json
	mdCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdCount++
		}
	}
	// 2 knowledge files + 1 _index.md = 3
	if mdCount != 3 {
		t.Errorf("got %d .md files, want 3 (2 knowledge + 1 index)", mdCount)
	}
}

func TestCurateStore_NoSources(t *testing.T) {
	s := setupTestStore(t)
	defer func() { _ = s.Close() }()

	synth := &llm.NoopSynthesizer{
		Response: mockLLMResponse(),
		Avail:    true,
	}

	pipeline := NewPipeline(s, synth, nil, t.TempDir())

	cfg := StoreConfig{
		Name:    "empty-store",
		Sources: []string{},
	}

	filesCreated, err := pipeline.CurateStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CurateStore with no sources: %v", err)
	}
	if filesCreated != 0 {
		t.Errorf("filesCreated = %d, want 0", filesCreated)
	}
}

func TestCurateStore_NilSynthesizer(t *testing.T) {
	s := setupTestStore(t)
	defer func() { _ = s.Close() }()

	pipeline := NewPipeline(s, nil, nil, t.TempDir())

	cfg := StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-meetings"},
	}

	// With nil synthesizer, should return an error containing the prompt.
	_, err := pipeline.CurateStore(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error with nil synthesizer, got nil")
	}
	if !strings.HasPrefix(err.Error(), "extraction_prompt:") {
		t.Errorf("expected error starting with 'extraction_prompt:', got: %v", err)
	}
}

func TestCurateStore_IncrementalSkipsProcessed(t *testing.T) {
	s := setupTestStore(t)
	defer func() { _ = s.Close() }()

	dir := t.TempDir()

	synth := &llm.NoopSynthesizer{
		Response: mockLLMResponse(),
		Avail:    true,
	}

	pipeline := NewPipeline(s, synth, nil, dir)

	cfg := StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-meetings"},
	}

	// First run: full curation.
	filesCreated, err := pipeline.CurateStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first CurateStore: %v", err)
	}
	if filesCreated != 2 {
		t.Errorf("first run: filesCreated = %d, want 2", filesCreated)
	}

	// Second run: incremental — no new content, should create 0 files.
	filesCreated, err = pipeline.CurateStoreIncremental(context.Background(), cfg)
	if err != nil {
		t.Fatalf("incremental CurateStore: %v", err)
	}
	if filesCreated != 0 {
		t.Errorf("incremental run: filesCreated = %d, want 0", filesCreated)
	}
}

func TestCurateStore_IndexFile(t *testing.T) {
	s := setupTestStore(t)
	defer func() { _ = s.Close() }()

	dir := t.TempDir()

	synth := &llm.NoopSynthesizer{
		Response: mockLLMResponse(),
		Avail:    true,
	}

	pipeline := NewPipeline(s, synth, nil, dir)

	cfg := StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-meetings"},
	}

	if _, err := pipeline.CurateStore(context.Background(), cfg); err != nil {
		t.Fatalf("CurateStore: %v", err)
	}

	// Verify _index.md was created.
	storePath := ResolveStorePath(cfg, dir)
	indexPath := filepath.Join(storePath, "_index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read _index.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Knowledge Store Index") {
		t.Error("_index.md missing title")
	}
	if !strings.Contains(content, "authentication") {
		t.Error("_index.md missing authentication entry")
	}
	if !strings.Contains(content, "deployment") {
		t.Error("_index.md missing deployment entry")
	}
}

// --- Quality analysis tests (T026) ---

func TestParseExtractionResponse_QualityFlags(t *testing.T) {
	response := `[
		{
			"tag": "auth",
			"category": "decision",
			"confidence": "flagged",
			"quality_flags": [
				{
					"type": "missing_rationale",
					"detail": "Decision without explanation",
					"sources": ["sprint-planning"],
					"resolution": "Ask team lead"
				},
				{
					"type": "incongruent",
					"detail": "Contradicts architecture doc",
					"sources": ["sprint-planning", "architecture-review"]
				}
			],
			"sources": [{"source_id": "disk-meetings", "document": "sprint-planning"}],
			"content": "Use OAuth2."
		}
	]`

	files, err := ParseExtractionResponse(response)
	if err != nil {
		t.Fatalf("ParseExtractionResponse: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	if len(files[0].QualityFlags) != 2 {
		t.Fatalf("got %d quality flags, want 2", len(files[0].QualityFlags))
	}

	// Verify missing_rationale flag.
	flag0 := files[0].QualityFlags[0]
	if flag0.Type != "missing_rationale" {
		t.Errorf("flag[0].Type = %q, want %q", flag0.Type, "missing_rationale")
	}
	if flag0.Detail != "Decision without explanation" {
		t.Errorf("flag[0].Detail = %q, want %q", flag0.Detail, "Decision without explanation")
	}
	if flag0.Resolution != "Ask team lead" {
		t.Errorf("flag[0].Resolution = %q, want %q", flag0.Resolution, "Ask team lead")
	}

	// Verify incongruent flag has both source references.
	flag1 := files[0].QualityFlags[1]
	if flag1.Type != "incongruent" {
		t.Errorf("flag[1].Type = %q, want %q", flag1.Type, "incongruent")
	}
	if len(flag1.Sources) != 2 {
		t.Errorf("flag[1].Sources len = %d, want 2", len(flag1.Sources))
	}
}

func TestParseExtractionResponse_ConfidenceValidation(t *testing.T) {
	tests := []struct {
		confidence string
		wantConf   string
	}{
		{"high", "high"},
		{"medium", "medium"},
		{"low", "low"},
		{"flagged", "flagged"},
		{"", "medium"}, // default
	}

	for _, tt := range tests {
		response := `[{"tag": "test", "category": "context", "confidence": "` + tt.confidence + `", "content": "test"}]`
		files, err := ParseExtractionResponse(response)
		if err != nil {
			t.Fatalf("ParseExtractionResponse(%q): %v", tt.confidence, err)
		}
		if len(files) != 1 {
			t.Fatalf("got %d files, want 1", len(files))
		}
		if files[0].Confidence != tt.wantConf {
			t.Errorf("confidence %q → %q, want %q", tt.confidence, files[0].Confidence, tt.wantConf)
		}
	}
}
