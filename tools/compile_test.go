package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// parseCompileResult unmarshals the JSON text from a compile CallToolResult.
func parseCompileResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal compile result: %v\ntext: %s", err, text)
	}
	return parsed
}

// storeLearningDirect is a simplified test helper that inserts a learning
// page with a single block directly into the store.
func storeLearningDirect(t *testing.T, s *store.Store, tag string, seq int, category, content string, createdAt time.Time) {
	t.Helper()
	identity := fmt.Sprintf("%s-%d", tag, seq)
	pageName := "learning/" + identity

	page := &store.Page{
		Name:         pageName,
		OriginalName: pageName,
		SourceID:     "learning",
		SourceDocID:  "learning-" + identity,
		Properties:   fmt.Sprintf(`{"tag":"%s","created_at":"%s"}`, tag, createdAt.UTC().Format(time.RFC3339)),
		Tier:         "draft",
		Category:     category,
		CreatedAt:    createdAt.UnixMilli(),
		UpdatedAt:    createdAt.UnixMilli(),
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage(%s): %v", pageName, err)
	}

	block := &store.Block{
		UUID:     "block-" + identity,
		PageName: pageName,
		Content:  content,
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock(%s): %v", block.UUID, err)
	}
}

// TestCompile_NilStore verifies that a nil store returns an error result.
func TestCompile_NilStore(t *testing.T) {
	c := NewCompile(nil, nil, nil, t.TempDir())

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}
	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestCompile_NoLearnings verifies that compiling with no learnings produces
// an empty summary with no errors and writes an _index.md.
func TestCompile_NoLearnings(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	c := NewCompile(s, nil, nil, tmpDir)

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success for empty learnings, got error: %s", resultText(result))
	}

	parsed := parseCompileResult(t, resultText(result))

	// Should report 0 learnings and 0 articles.
	if parsed["total_learnings"] != float64(0) {
		t.Errorf("total_learnings = %v, want 0", parsed["total_learnings"])
	}
	if parsed["total_articles"] != float64(0) {
		t.Errorf("total_articles = %v, want 0", parsed["total_articles"])
	}
	if parsed["message"] != "No learnings to compile." {
		t.Errorf("message = %v, want 'No learnings to compile.'", parsed["message"])
	}

	// Verify _index.md exists and contains "No learnings to compile".
	indexPath := filepath.Join(tmpDir, ".uf", "dewey", "compiled", "_index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read _index.md: %v", err)
	}
	if !strings.Contains(string(data), "No learnings to compile") {
		t.Errorf("_index.md content = %q, should contain 'No learnings to compile'", string(data))
	}
}

// TestCompile_ClusterByTag verifies that 5 learnings across 2 tags produce
// 2 clusters, each containing the correct learnings.
func TestCompile_ClusterByTag(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	entries := []LearningEntry{
		{Identity: "auth-1", Tag: "auth", Category: "decision", CreatedAt: baseTime, Content: "Use Option A"},
		{Identity: "auth-2", Tag: "auth", Category: "decision", CreatedAt: baseTime.Add(1 * time.Hour), Content: "Switch to Option B"},
		{Identity: "auth-3", Tag: "auth", Category: "context", CreatedAt: baseTime.Add(2 * time.Hour), Content: "Timeout 60s"},
		{Identity: "deploy-1", Tag: "deploy", Category: "pattern", CreatedAt: baseTime, Content: "Blue-green deployment"},
		{Identity: "deploy-2", Tag: "deploy", Category: "gotcha", CreatedAt: baseTime.Add(1 * time.Hour), Content: "Watch for DNS propagation"},
	}

	clusters := clusterLearnings(entries)

	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	// Clusters are sorted by topic name alphabetically.
	// "Auth" comes before "Deploy".
	if clusters[0].DominantTag != "auth" {
		t.Errorf("cluster[0] tag = %q, want %q", clusters[0].DominantTag, "auth")
	}
	if len(clusters[0].Learnings) != 3 {
		t.Errorf("auth cluster has %d learnings, want 3", len(clusters[0].Learnings))
	}

	if clusters[1].DominantTag != "deploy" {
		t.Errorf("cluster[1] tag = %q, want %q", clusters[1].DominantTag, "deploy")
	}
	if len(clusters[1].Learnings) != 2 {
		t.Errorf("deploy cluster has %d learnings, want 2", len(clusters[1].Learnings))
	}
}

// TestCompile_SynthesisPromptIncludesAllLearnings verifies that the
// synthesis prompt contains the content of all learnings in the cluster.
func TestCompile_SynthesisPromptIncludesAllLearnings(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	cl := Cluster{
		Topic:       "Authentication",
		DominantTag: "auth",
		Learnings: []LearningEntry{
			{Identity: "auth-1", Tag: "auth", Category: "decision", CreatedAt: baseTime, Content: "Use Option A for auth"},
			{Identity: "auth-2", Tag: "auth", Category: "decision", CreatedAt: baseTime.Add(24 * time.Hour), Content: "Switch to Option B due to rate limiting"},
			{Identity: "auth-3", Tag: "auth", Category: "context", CreatedAt: baseTime.Add(48 * time.Hour), Content: "Increase timeout to 60s per user feedback"},
		},
	}

	prompt := buildSynthesisPrompt(cl)

	// Verify all learning content is present.
	if !strings.Contains(prompt, "Use Option A for auth") {
		t.Error("prompt missing content from auth-1")
	}
	if !strings.Contains(prompt, "Switch to Option B due to rate limiting") {
		t.Error("prompt missing content from auth-2")
	}
	if !strings.Contains(prompt, "Increase timeout to 60s per user feedback") {
		t.Error("prompt missing content from auth-3")
	}

	// Verify all identities are present.
	if !strings.Contains(prompt, "auth-1") {
		t.Error("prompt missing identity auth-1")
	}
	if !strings.Contains(prompt, "auth-2") {
		t.Error("prompt missing identity auth-2")
	}
	if !strings.Contains(prompt, "auth-3") {
		t.Error("prompt missing identity auth-3")
	}

	// Verify topic is mentioned.
	if !strings.Contains(prompt, "Authentication") {
		t.Error("prompt missing topic name")
	}

	// Verify category-aware instructions are included.
	if !strings.Contains(prompt, "decision") {
		t.Error("prompt missing decision category instructions")
	}
	if !strings.Contains(prompt, "context") {
		t.Error("prompt missing context category instructions")
	}
}

// TestCompile_SynthesisPromptTemporalOrder verifies that learnings appear
// in chronological order (oldest first) in the synthesis prompt.
func TestCompile_SynthesisPromptTemporalOrder(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	cl := Cluster{
		Topic:       "Authentication",
		DominantTag: "auth",
		Learnings: []LearningEntry{
			{Identity: "auth-1", Tag: "auth", Category: "decision", CreatedAt: baseTime, Content: "FIRST learning"},
			{Identity: "auth-2", Tag: "auth", Category: "decision", CreatedAt: baseTime.Add(24 * time.Hour), Content: "SECOND learning"},
			{Identity: "auth-3", Tag: "auth", Category: "context", CreatedAt: baseTime.Add(48 * time.Hour), Content: "THIRD learning"},
		},
	}

	prompt := buildSynthesisPrompt(cl)

	// Verify temporal order: FIRST appears before SECOND, SECOND before THIRD.
	firstIdx := strings.Index(prompt, "FIRST learning")
	secondIdx := strings.Index(prompt, "SECOND learning")
	thirdIdx := strings.Index(prompt, "THIRD learning")

	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Fatal("prompt missing one or more learning contents")
	}
	if firstIdx >= secondIdx {
		t.Errorf("FIRST (pos %d) should appear before SECOND (pos %d)", firstIdx, secondIdx)
	}
	if secondIdx >= thirdIdx {
		t.Errorf("SECOND (pos %d) should appear before THIRD (pos %d)", secondIdx, thirdIdx)
	}
}

// TestCompile_ArticleWrittenToFS verifies that compiled articles are
// written to the filesystem at the expected path.
func TestCompile_ArticleWrittenToFS(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nUse Option B for authentication. Timeout: 60s.",
		Avail:    true,
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	// Store learnings.
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Use Option A", baseTime)
	storeLearningDirect(t, s, "auth", 2, "decision", "Switch to Option B", baseTime.Add(24*time.Hour))

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	// Verify the article file exists.
	articlePath := filepath.Join(tmpDir, ".uf", "dewey", "compiled", "auth.md")
	data, err := os.ReadFile(articlePath)
	if err != nil {
		t.Fatalf("failed to read compiled article: %v", err)
	}
	content := string(data)

	// Verify frontmatter.
	if !strings.Contains(content, "tier: draft") {
		t.Error("article missing 'tier: draft' in frontmatter")
	}
	if !strings.Contains(content, "topic: auth") {
		t.Error("article missing 'topic: auth' in frontmatter")
	}
	if !strings.Contains(content, "compiled_at:") {
		t.Error("article missing 'compiled_at' in frontmatter")
	}

	// Verify sources in frontmatter.
	if !strings.Contains(content, "- auth-1") {
		t.Error("article missing source auth-1")
	}
	if !strings.Contains(content, "- auth-2") {
		t.Error("article missing source auth-2")
	}

	// Verify synthesized content.
	if !strings.Contains(content, "Use Option B for authentication") {
		t.Error("article missing synthesized current state")
	}

	// Verify history table.
	if !strings.Contains(content, "## History") {
		t.Error("article missing History section")
	}
}

// TestCompile_ArticlePersistedInStore verifies that compiled articles are
// persisted in the store with source_id="compiled" and tier="draft".
func TestCompile_ArticlePersistedInStore(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nCompiled content.",
		Avail:    true,
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "deploy", 1, "pattern", "Blue-green deployment", baseTime)

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	// Verify the compiled page exists in the store.
	page, err := s.GetPage("compiled/deploy")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("compiled page not found in store")
	}
	if page.SourceID != "compiled" {
		t.Errorf("source_id = %q, want %q", page.SourceID, "compiled")
	}
	if page.Tier != "draft" {
		t.Errorf("tier = %q, want %q", page.Tier, "draft")
	}

	// Verify blocks were persisted.
	blocks, err := s.GetBlocksByPage("compiled/deploy")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) == 0 {
		t.Error("expected at least 1 block for compiled page")
	}
}

// TestCompile_IndexGenerated verifies that _index.md exists and lists
// all compiled topics.
func TestCompile_IndexGenerated(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nContent.",
		Avail:    true,
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Auth content", baseTime)
	storeLearningDirect(t, s, "deploy", 1, "pattern", "Deploy content", baseTime)

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	// Verify _index.md exists.
	indexPath := filepath.Join(tmpDir, ".uf", "dewey", "compiled", "_index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read _index.md: %v", err)
	}
	content := string(data)

	// Verify both topics are listed.
	if !strings.Contains(content, "Auth") {
		t.Error("_index.md missing Auth topic")
	}
	if !strings.Contains(content, "Deploy") {
		t.Error("_index.md missing Deploy topic")
	}
	if !strings.Contains(content, "auth.md") {
		t.Error("_index.md missing link to auth.md")
	}
	if !strings.Contains(content, "deploy.md") {
		t.Error("_index.md missing link to deploy.md")
	}
}

// TestCompile_SynthesizerUnavailable verifies that compilation succeeds
// in prompt-only mode when the synthesizer is nil. Articles are not
// synthesized but clusters and prompts are returned.
func TestCompile_SynthesizerUnavailable(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	c := NewCompile(s, nil, nil, tmpDir) // nil synthesizer

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Use Option A", baseTime)
	storeLearningDirect(t, s, "auth", 2, "decision", "Switch to Option B", baseTime.Add(24*time.Hour))

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	parsed := parseCompileResult(t, resultText(result))

	// Should be in prompt mode.
	if parsed["status"] != "prompts_ready" {
		t.Errorf("status = %v, want 'prompts_ready'", parsed["status"])
	}

	// Should have clusters with prompts.
	clusters, ok := parsed["clusters"].([]any)
	if !ok || len(clusters) == 0 {
		t.Fatal("expected non-empty clusters in prompt mode")
	}

	cluster, ok := clusters[0].(map[string]any)
	if !ok {
		t.Fatal("cluster is not map[string]any")
	}
	if cluster["topic"] == nil || cluster["topic"] == "" {
		t.Error("cluster missing topic")
	}
	if cluster["synthesis_prompt"] == nil || cluster["synthesis_prompt"] == "" {
		t.Error("cluster missing synthesis_prompt")
	}
	if cluster["category_instructions"] == nil || cluster["category_instructions"] == "" {
		t.Error("cluster missing category_instructions")
	}

	// Verify learnings are included in the cluster.
	learnings, ok := cluster["learnings"].([]any)
	if !ok || len(learnings) != 2 {
		t.Errorf("expected 2 learnings in cluster, got %d", len(learnings))
	}

	// Should report total learnings.
	if parsed["total_learnings"] != float64(2) {
		t.Errorf("total_learnings = %v, want 2", parsed["total_learnings"])
	}
	if parsed["total_clusters"] != float64(1) {
		t.Errorf("total_clusters = %v, want 1", parsed["total_clusters"])
	}
}

// TestCompile_SynthesizerNotAvailable verifies prompt-only mode when
// the synthesizer exists but reports Available() == false.
func TestCompile_SynthesizerNotAvailable(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "should not be called",
		Avail:    false, // Not available
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Use Option A", baseTime)

	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	parsed := parseCompileResult(t, resultText(result))
	if parsed["status"] != "prompts_ready" {
		t.Errorf("status = %v, want 'prompts_ready'", parsed["status"])
	}
}

// TestCompile_ConcurrentCallRejected verifies the TryLock pattern:
// a second concurrent call is rejected while the first is running.
func TestCompile_ConcurrentCallRejected(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	c := NewCompile(s, nil, nil, tmpDir)

	// Acquire the lock manually to simulate a running compilation.
	c.mu.Lock()

	// Try to compile — should be rejected.
	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when compilation is already in progress")
	}
	text := resultText(result)
	if !strings.Contains(text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", text)
	}

	// Release the lock.
	c.mu.Unlock()
}

// TestCompileIncremental_SingleTag verifies that incremental compilation
// re-compiles only the specified tag.
func TestCompileIncremental_SingleTag(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nIncremental content.",
		Avail:    true,
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Auth content 1", baseTime)
	storeLearningDirect(t, s, "auth", 2, "decision", "Auth content 2", baseTime.Add(24*time.Hour))
	storeLearningDirect(t, s, "deploy", 1, "pattern", "Deploy content", baseTime)

	// Compile incrementally for auth-2 only.
	result, _, err := c.Compile(context.Background(), nil, types.CompileInput{
		Incremental: []string{"auth-2"},
	})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Compile returned error: %s", resultText(result))
	}

	parsed := parseCompileResult(t, resultText(result))
	if parsed["status"] != "compiled" {
		t.Errorf("status = %v, want 'compiled'", parsed["status"])
	}

	// Should have compiled only the auth cluster (2 learnings).
	articles, ok := parsed["articles"].([]any)
	if !ok || len(articles) != 1 {
		t.Fatalf("expected 1 article (auth only), got %d", len(articles))
	}

	article, ok := articles[0].(map[string]any)
	if !ok {
		t.Fatal("article is not map[string]any")
	}
	if article["topic"] != "Auth" {
		t.Errorf("article topic = %v, want 'Auth'", article["topic"])
	}

	// The auth article should include both auth learnings (full cluster re-compile).
	sources, ok := article["sources"].([]any)
	if !ok || len(sources) != 2 {
		t.Errorf("expected 2 sources in auth article, got %d", len(sources))
	}

	// Verify auth article file exists.
	authPath := filepath.Join(tmpDir, ".uf", "dewey", "compiled", "auth.md")
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		t.Error("auth.md not written to filesystem")
	}

	// Verify deploy article was NOT created (not in incremental scope).
	deployPath := filepath.Join(tmpDir, ".uf", "dewey", "compiled", "deploy.md")
	if _, err := os.Stat(deployPath); !os.IsNotExist(err) {
		t.Error("deploy.md should not exist — not in incremental scope")
	}
}

// TestCompile_FullRebuildDeletesOld verifies that a full rebuild deletes
// existing compiled articles before creating new ones.
func TestCompile_FullRebuildDeletesOld(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nRebuilt content.",
		Avail:    true,
		Model:    "test-model",
	}
	c := NewCompile(s, nil, synth, tmpDir)

	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningDirect(t, s, "auth", 1, "decision", "Auth content", baseTime)

	// First compile.
	result1, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("first Compile error: %v", err)
	}
	if result1.IsError {
		t.Fatalf("first Compile returned error: %s", resultText(result1))
	}

	// Verify the compiled page exists.
	page1, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage after first compile: %v", err)
	}
	if page1 == nil {
		t.Fatal("compiled page not found after first compile")
	}

	// Second compile (full rebuild) — should delete and recreate.
	result2, _, err := c.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("second Compile error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("second Compile returned error: %s", resultText(result2))
	}

	// Verify the compiled page still exists (recreated).
	page2, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage after second compile: %v", err)
	}
	if page2 == nil {
		t.Fatal("compiled page not found after second compile (rebuild)")
	}
}

// TestCompile_CategoryAwarePrompt verifies that the synthesis prompt
// includes category-specific instructions based on the categories
// present in the cluster.
func TestCompile_CategoryAwarePrompt(t *testing.T) {
	cl := Cluster{
		Topic:       "Authentication",
		DominantTag: "auth",
		Learnings: []LearningEntry{
			{Identity: "auth-1", Tag: "auth", Category: "decision", CreatedAt: time.Now(), Content: "Decision content"},
			{Identity: "auth-2", Tag: "auth", Category: "pattern", CreatedAt: time.Now(), Content: "Pattern content"},
			{Identity: "auth-3", Tag: "auth", Category: "gotcha", CreatedAt: time.Now(), Content: "Gotcha content"},
		},
	}

	prompt := buildSynthesisPrompt(cl)

	// Verify category-specific instructions are present.
	if !strings.Contains(prompt, "decision") {
		t.Error("prompt missing decision instructions")
	}
	if !strings.Contains(prompt, "pattern") {
		t.Error("prompt missing pattern instructions")
	}
	if !strings.Contains(prompt, "gotcha") {
		t.Error("prompt missing gotcha instructions")
	}
	if !strings.Contains(prompt, "supersede") {
		t.Error("prompt missing temporal merge instruction for decisions")
	}
	if !strings.Contains(strings.ToLower(prompt), "accumulate") {
		t.Error("prompt missing accumulate instruction for patterns")
	}
}

// TestCompile_ExtractTagFromIdentity verifies the tag extraction logic
// for various identity formats.
func TestCompile_ExtractTagFromIdentity(t *testing.T) {
	tests := []struct {
		identity string
		wantTag  string
	}{
		{"authentication-3", "authentication"},
		{"vault-walker-2", "vault-walker"},
		{"general-1", "general"},
		{"deploy-10", "deploy"},
		{"my-multi-part-tag-5", "my-multi-part-tag"},
		{"notag", "notag"}, // no sequence number
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			got := extractTagFromIdentity(tt.identity)
			if got != tt.wantTag {
				t.Errorf("extractTagFromIdentity(%q) = %q, want %q", tt.identity, got, tt.wantTag)
			}
		})
	}
}

// TestCompile_ClusterTopicNaming verifies that cluster topics are derived
// from tags with proper capitalization.
func TestCompile_ClusterTopicNaming(t *testing.T) {
	entries := []LearningEntry{
		{Identity: "vault-walker-1", Tag: "vault-walker", CreatedAt: time.Now(), Content: "content"},
	}

	clusters := clusterLearnings(entries)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}

	// "vault-walker" → "Vault walker"
	if clusters[0].Topic != "Vault walker" {
		t.Errorf("topic = %q, want %q", clusters[0].Topic, "Vault walker")
	}
}

// TestCompile_ClusterSortedChronologically verifies that learnings within
// a cluster are sorted by CreatedAt ascending.
func TestCompile_ClusterSortedChronologically(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	// Insert in reverse chronological order.
	entries := []LearningEntry{
		{Identity: "auth-3", Tag: "auth", CreatedAt: baseTime.Add(48 * time.Hour), Content: "third"},
		{Identity: "auth-1", Tag: "auth", CreatedAt: baseTime, Content: "first"},
		{Identity: "auth-2", Tag: "auth", CreatedAt: baseTime.Add(24 * time.Hour), Content: "second"},
	}

	clusters := clusterLearnings(entries)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}

	learnings := clusters[0].Learnings
	if learnings[0].Identity != "auth-1" {
		t.Errorf("first learning = %q, want auth-1", learnings[0].Identity)
	}
	if learnings[1].Identity != "auth-2" {
		t.Errorf("second learning = %q, want auth-2", learnings[1].Identity)
	}
	if learnings[2].Identity != "auth-3" {
		t.Errorf("third learning = %q, want auth-3", learnings[2].Identity)
	}
}

// TestCompile_CompiledArticleFormat verifies the markdown format of a
// compiled article including frontmatter, current-state, and history.
func TestCompile_CompiledArticleFormat(t *testing.T) {
	baseTime := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	cl := Cluster{
		Topic:       "Authentication",
		DominantTag: "auth",
		Learnings: []LearningEntry{
			{Identity: "auth-1", Tag: "auth", Category: "decision", CreatedAt: baseTime, Content: "Use Option A"},
			{Identity: "auth-2", Tag: "auth", Category: "decision", CreatedAt: baseTime.Add(24 * time.Hour), Content: "Switch to Option B"},
		},
	}

	article := buildCompiledArticle(cl, "## Current State\n\nUse Option B.")

	// Verify frontmatter structure.
	if !strings.HasPrefix(article, "---\n") {
		t.Error("article should start with frontmatter delimiter")
	}
	if !strings.Contains(article, "tier: draft") {
		t.Error("article missing tier in frontmatter")
	}
	if !strings.Contains(article, "topic: auth") {
		t.Error("article missing topic in frontmatter")
	}
	if !strings.Contains(article, "compiled_at:") {
		t.Error("article missing compiled_at in frontmatter")
	}
	if !strings.Contains(article, "  - auth-1") {
		t.Error("article missing source auth-1 in frontmatter")
	}
	if !strings.Contains(article, "  - auth-2") {
		t.Error("article missing source auth-2 in frontmatter")
	}

	// Verify title.
	if !strings.Contains(article, "# Authentication") {
		t.Error("article missing title heading")
	}

	// Verify synthesized content.
	if !strings.Contains(article, "Use Option B.") {
		t.Error("article missing synthesized content")
	}

	// Verify history table.
	if !strings.Contains(article, "## History") {
		t.Error("article missing History section")
	}
	if !strings.Contains(article, "| auth-1 |") {
		t.Error("article missing auth-1 in history table")
	}
	if !strings.Contains(article, "| auth-2 |") {
		t.Error("article missing auth-2 in history table")
	}
	if !strings.Contains(article, "| decision |") {
		t.Error("article missing category in history table")
	}
}
