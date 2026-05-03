package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// compileLogger is the package-level structured logger for compile tool operations.
var compileLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/tools/compile",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// LearningEntry represents a learning with its metadata for clustering.
// Extracted from store.Page for use in the pure clustering function.
type LearningEntry struct {
	Identity  string    // e.g., "authentication-3"
	Tag       string    // topic tag
	Category  string    // decision/pattern/gotcha/context/reference
	CreatedAt time.Time // temporal ordering
	Content   string    // block content
}

// Cluster represents a group of related learnings for compilation.
// Each cluster produces one compiled article.
type Cluster struct {
	Topic       string          // Auto-generated topic name from dominant tag
	DominantTag string          // Most common tag in the cluster
	Learnings   []LearningEntry // Ordered by CreatedAt ascending
}

// Compile implements the dewey_compile MCP tool and CLI command.
// Dependencies are injected for testability (Dependency Inversion Principle).
//
// Design decision: The compile tool has two modes based on synthesizer
// availability. When a Synthesizer is injected, it calls the LLM to
// generate articles. When nil, it returns clusters with synthesis prompts
// as structured output for the calling agent to perform synthesis (D4).
type Compile struct {
	mu        sync.Mutex
	store     *store.Store
	embedder  embed.Embedder
	synth     llm.Synthesizer
	vaultPath string
}

// NewCompile creates a new Compile tool handler with the given dependencies.
// The store must be non-nil for the tool to function. The embedder and
// synthesizer may be nil — the tool degrades gracefully:
//   - nil embedder: clustering uses tags only (no semantic refinement)
//   - nil synthesizer: returns clusters + prompts without LLM synthesis
//
// vaultPath is the vault root directory (not the .uf/dewey/ workspace).
func NewCompile(s *store.Store, e embed.Embedder, synth llm.Synthesizer, vaultPath string) *Compile {
	return &Compile{store: s, embedder: e, synth: synth, vaultPath: vaultPath}
}

// compiledDir returns the path to the compiled articles directory.
// Articles are written to {vaultPath}/.uf/dewey/compiled/.
func (c *Compile) compiledDir() string {
	return filepath.Join(c.vaultPath, deweyWorkspaceDir, "compiled")
}

// Compile handles the dewey_compile MCP tool. Reads learnings from the
// store, clusters by tag, and produces compiled articles.
//
// When synthesizer is available: writes compiled articles to
// .uf/dewey/compiled/ and indexes them in the store.
//
// When synthesizer is nil: returns clusters with synthesis prompts
// as structured output for the calling agent to perform synthesis.
//
// Full rebuild (incremental empty): deletes existing compiled articles
// and rebuilds from all learnings.
//
// Incremental (identities provided): merges specified learnings into
// existing compiled articles by re-compiling affected tags.
func (c *Compile) Compile(ctx context.Context, req *mcp.CallToolRequest, input types.CompileInput) (*mcp.CallToolResult, any, error) {
	if c.store == nil {
		return errorResult("compile requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	// Non-blocking mutual exclusion: if another compile is running,
	// return immediately with an error rather than queuing.
	if !c.mu.TryLock() {
		return errorResult("compilation already in progress"), nil, nil
	}
	defer c.mu.Unlock()

	start := time.Now()

	if len(input.Incremental) > 0 {
		return c.compileIncremental(ctx, input.Incremental, start)
	}
	return c.compileAll(ctx, start)
}

// compileAll performs a full rebuild: deletes existing compiled articles
// and rebuilds from all learnings.
func (c *Compile) compileAll(ctx context.Context, start time.Time) (*mcp.CallToolResult, any, error) {
	compileLogger.Info("full compile starting")

	// List all learnings from the store.
	pages, err := c.store.ListLearningPages()
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list learnings: %v", err)), nil, nil
	}

	// Handle empty learnings case: produce an empty _index.md.
	if len(pages) == 0 {
		if err := c.writeEmptyIndex(); err != nil {
			return errorResult(fmt.Sprintf("failed to write empty index: %v", err)), nil, nil
		}
		result := map[string]any{
			"status":          "compiled",
			"articles":        []any{},
			"index":           filepath.Join(c.compiledDir(), "_index.md"),
			"total_learnings": 0,
			"total_articles":  0,
			"message":         "No learnings to compile.",
		}
		res, err := jsonTextResult(result)
		return res, nil, err
	}

	// Build learning entries from pages.
	entries, err := c.buildLearningEntries(pages)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to build learning entries: %v", err)), nil, nil
	}

	// Cluster learnings by tag.
	clusters := clusterLearnings(entries)

	// Full rebuild: delete existing compiled articles from store and filesystem.
	if _, err := c.store.DeletePagesBySource("compiled"); err != nil {
		compileLogger.Warn("failed to delete existing compiled pages", "err", err)
	}
	compiledDir := c.compiledDir()
	if err := os.RemoveAll(compiledDir); err != nil && !os.IsNotExist(err) {
		compileLogger.Warn("failed to remove compiled directory", "err", err)
	}

	// Process clusters: synthesize and write articles.
	return c.processClusters(ctx, clusters, len(entries), start)
}

// compileIncremental re-compiles only the tags affected by the specified
// learning identities. This is the same as compileAll but scoped to
// specific tags.
func (c *Compile) compileIncremental(ctx context.Context, identities []string, start time.Time) (*mcp.CallToolResult, any, error) {
	compileLogger.Info("incremental compile starting", "identities", len(identities))

	// Extract unique tags from the specified identities.
	// Look up each page from the store to get properties for reliable
	// tag extraction. Falls back to string-parsing if the page is not found.
	affectedTags := make(map[string]bool)
	for _, id := range identities {
		var properties string
		if page, err := c.store.GetPage("learning/" + id); err == nil && page != nil {
			properties = page.Properties
		}
		tag := extractTagFromIdentity(id, properties)
		if tag != "" {
			affectedTags[tag] = true
		}
	}

	if len(affectedTags) == 0 {
		return errorResult("no valid learning identities provided"), nil, nil
	}

	// List all learnings from the store.
	pages, err := c.store.ListLearningPages()
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list learnings: %v", err)), nil, nil
	}

	// Build learning entries from pages.
	entries, err := c.buildLearningEntries(pages)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to build learning entries: %v", err)), nil, nil
	}

	// Cluster all learnings by tag.
	allClusters := clusterLearnings(entries)

	// Filter to only affected clusters.
	var clusters []Cluster
	for _, cl := range allClusters {
		if affectedTags[cl.DominantTag] {
			clusters = append(clusters, cl)
		}
	}

	// Delete existing compiled pages for affected tags only.
	for _, cl := range clusters {
		pageName := "compiled/" + cl.DominantTag
		if err := c.store.DeletePage(pageName); err != nil {
			// Page may not exist yet — not an error.
			compileLogger.Debug("no existing compiled page to delete", "page", pageName)
		}
		// Remove the file if it exists.
		articlePath := filepath.Join(c.compiledDir(), cl.DominantTag+".md")
		_ = os.Remove(articlePath)
	}

	// Count total learnings across affected clusters for the summary.
	totalLearnings := 0
	for _, cl := range clusters {
		totalLearnings += len(cl.Learnings)
	}

	return c.processClusters(ctx, clusters, totalLearnings, start)
}

// processClusters handles the shared logic for both full and incremental
// compilation: synthesize articles, write to filesystem, persist in store.
func (c *Compile) processClusters(ctx context.Context, clusters []Cluster, totalLearnings int, start time.Time) (*mcp.CallToolResult, any, error) {
	// If no synthesizer is available, return clusters with prompts.
	if c.synth == nil || !c.synth.Available() {
		return c.returnPromptMode(clusters, totalLearnings, start)
	}

	// Synthesizer available: generate articles.
	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return errorResult(fmt.Sprintf("failed to create compiled directory: %v", err)), nil, nil
	}

	var articles []map[string]any
	for _, cl := range clusters {
		prompt := buildSynthesisPrompt(cl)
		compileLogger.Info("synthesizing article", "topic", cl.Topic, "learnings", len(cl.Learnings))

		synthesized, err := c.synth.Synthesize(ctx, prompt)
		if err != nil {
			compileLogger.Warn("synthesis failed, skipping cluster", "topic", cl.Topic, "err", err)
			continue
		}

		// Build the compiled article markdown.
		article := buildCompiledArticle(cl, synthesized)

		// Write to filesystem.
		articlePath := filepath.Join(compiledDir, cl.DominantTag+".md")
		if err := os.WriteFile(articlePath, []byte(article), 0o644); err != nil {
			compileLogger.Warn("failed to write article", "path", articlePath, "err", err)
			continue
		}

		// Persist in store with source_id="compiled" and tier="draft".
		if err := c.persistCompiledArticle(cl, article); err != nil {
			compileLogger.Warn("failed to persist compiled article", "topic", cl.Topic, "err", err)
		}

		// Collect source identities for the response.
		var sources []string
		for _, l := range cl.Learnings {
			sources = append(sources, l.Identity)
		}

		articles = append(articles, map[string]any{
			"path":           articlePath,
			"topic":          cl.Topic,
			"sources":        sources,
			"learning_count": len(cl.Learnings),
		})
	}

	// Generate _index.md.
	if err := c.writeIndex(clusters); err != nil {
		compileLogger.Warn("failed to write index", "err", err)
	}

	elapsed := time.Since(start)
	compileLogger.Info("compile complete",
		"articles", len(articles),
		"learnings", totalLearnings,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	result := map[string]any{
		"status":          "compiled",
		"articles":        articles,
		"index":           filepath.Join(compiledDir, "_index.md"),
		"total_learnings": totalLearnings,
		"total_articles":  len(articles),
		"message":         fmt.Sprintf("Compiled %d learnings into %d articles.", totalLearnings, len(articles)),
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// returnPromptMode returns clusters with synthesis prompts when no
// synthesizer is available. The calling agent can use these prompts
// to perform synthesis externally (D4: prompt-based delegation).
func (c *Compile) returnPromptMode(clusters []Cluster, totalLearnings int, start time.Time) (*mcp.CallToolResult, any, error) {
	var clusterResults []map[string]any
	for _, cl := range clusters {
		var learningResults []map[string]any
		for _, l := range cl.Learnings {
			learningResults = append(learningResults, map[string]any{
				"identity":   l.Identity,
				"category":   l.Category,
				"created_at": l.CreatedAt.UTC().Format(time.RFC3339),
				"content":    l.Content,
			})
		}

		clusterResults = append(clusterResults, map[string]any{
			"topic":                 cl.Topic,
			"dominant_tag":          cl.DominantTag,
			"learnings":             learningResults,
			"synthesis_prompt":      buildSynthesisPrompt(cl),
			"category_instructions": categoryInstructions(cl),
		})
	}

	elapsed := time.Since(start)
	result := map[string]any{
		"status":          "prompts_ready",
		"clusters":        clusterResults,
		"total_learnings": totalLearnings,
		"total_clusters":  len(clusters),
		"elapsed_ms":      elapsed.Milliseconds(),
		"message":         fmt.Sprintf("Clustered %d learnings into %d topics. Synthesis prompts ready for agent execution.", totalLearnings, len(clusters)),
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// buildLearningEntries converts store pages into LearningEntry values
// by loading block content and extracting metadata from page properties.
func (c *Compile) buildLearningEntries(pages []*store.Page) ([]LearningEntry, error) {
	var entries []LearningEntry
	for _, p := range pages {
		// Extract identity from page name: "learning/{identity}" → "{identity}".
		identity := strings.TrimPrefix(p.Name, "learning/")
		tag := extractTagFromIdentity(identity, p.Properties)

		// Load block content for this learning.
		blocks, err := c.store.GetBlocksByPage(p.Name)
		if err != nil {
			compileLogger.Warn("failed to load blocks", "page", p.Name, "err", err)
			continue
		}
		var contentParts []string
		for _, b := range blocks {
			if strings.TrimSpace(b.Content) != "" {
				contentParts = append(contentParts, b.Content)
			}
		}
		content := strings.Join(contentParts, "\n")

		// Parse created_at from page's CreatedAt (Unix milliseconds).
		createdAt := time.UnixMilli(p.CreatedAt)

		entries = append(entries, LearningEntry{
			Identity:  identity,
			Tag:       tag,
			Category:  p.Category,
			CreatedAt: createdAt,
			Content:   content,
		})
	}
	return entries, nil
}

// clusterLearnings groups learnings by tag and returns one cluster per
// topic. This is the v1 "tag-assisted" clustering — tags are the sole
// clustering dimension. Semantic refinement (splitting/merging by
// embedding distance) is a Phase 2 enhancement.
//
// Pure function — no side effects. Deterministic given the same input.
func clusterLearnings(entries []LearningEntry) []Cluster {
	// Group by tag.
	groups := make(map[string][]LearningEntry)
	for _, e := range entries {
		groups[e.Tag] = append(groups[e.Tag], e)
	}

	// Build clusters, sorting learnings by CreatedAt within each group.
	var clusters []Cluster
	for tag, learnings := range groups {
		sort.Slice(learnings, func(i, j int) bool {
			return learnings[i].CreatedAt.Before(learnings[j].CreatedAt)
		})

		// Topic name: capitalize first letter of tag, replace hyphens with spaces.
		topic := strings.ReplaceAll(tag, "-", " ")
		if len(topic) > 0 {
			topic = strings.ToUpper(topic[:1]) + topic[1:]
		}

		clusters = append(clusters, Cluster{
			Topic:       topic,
			DominantTag: tag,
			Learnings:   learnings,
		})
	}

	// Sort clusters by topic name for deterministic output.
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Topic < clusters[j].Topic
	})

	return clusters
}

// extractTagFromIdentity extracts the tag from a learning identity.
//
// If properties JSON contains a "tag" key, that value is returned directly.
// This is the preferred path for new-format identities where the tag is
// stored explicitly in page properties.
//
// Falls back to string-parsing for backward compatibility with old-format
// identities: "{tag}-{sequence}" (e.g., "authentication-3" → "authentication").
// Handles multi-segment tags like "vault-walker-2" → "vault-walker".
func extractTagFromIdentity(identity, properties string) string {
	// Preferred path: extract tag from properties JSON.
	if properties != "" {
		var props map[string]string
		if err := json.Unmarshal([]byte(properties), &props); err == nil {
			if tag, ok := props["tag"]; ok && tag != "" {
				return tag
			}
		}
	}

	// Fallback: parse tag from old-format identity string.
	// Find the last hyphen followed by a number — that's the sequence separator.
	lastHyphen := strings.LastIndex(identity, "-")
	if lastHyphen < 0 {
		return identity
	}
	// Verify the part after the last hyphen is a number.
	suffix := identity[lastHyphen+1:]
	if _, err := strconv.Atoi(suffix); err != nil {
		// Not a number — the whole string is the tag (unusual case).
		return identity
	}
	return identity[:lastHyphen]
}

// buildSynthesisPrompt generates the LLM prompt for synthesizing a
// compiled article from a cluster of learnings. The prompt includes
// category-aware instructions for how to handle different learning types.
func buildSynthesisPrompt(cl Cluster) string {
	var sb strings.Builder

	sb.WriteString("You are compiling learnings into a knowledge article.\n\n")
	sb.WriteString(fmt.Sprintf("Topic: %s\n\n", cl.Topic))

	// Add category-aware instructions.
	sb.WriteString(categoryInstructions(cl))
	sb.WriteString("\n")

	// Add learnings in chronological order.
	sb.WriteString("## Learnings (chronological order, oldest first)\n\n")
	for i, l := range cl.Learnings {
		cat := l.Category
		if cat == "" {
			cat = "context"
		}
		sb.WriteString(fmt.Sprintf("### Learning %d: %s\n", i+1, l.Identity))
		sb.WriteString(fmt.Sprintf("- **Category**: %s\n", cat))
		sb.WriteString(fmt.Sprintf("- **Date**: %s\n", l.CreatedAt.UTC().Format("2006-01-02")))
		sb.WriteString(fmt.Sprintf("- **Content**: %s\n\n", l.Content))
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Produce a markdown article with:\n")
	sb.WriteString("1. A `## Current State` section containing all current facts. ")
	sb.WriteString("Resolve contradictions by favoring the newest learning. ")
	sb.WriteString("Carry forward non-contradicted facts from older learnings.\n")
	sb.WriteString("2. Do NOT include a top-level heading (# Title) — it will be added automatically.\n")
	sb.WriteString("3. Write in a clear, factual style. No preamble or meta-commentary.\n")

	return sb.String()
}

// categoryInstructions returns category-specific instructions for the
// synthesis prompt based on the categories present in the cluster.
func categoryInstructions(cl Cluster) string {
	// Collect unique categories in the cluster.
	cats := make(map[string]bool)
	for _, l := range cl.Learnings {
		cat := l.Category
		if cat == "" {
			cat = "context"
		}
		cats[cat] = true
	}

	var sb strings.Builder
	sb.WriteString("## Category-Specific Resolution Rules\n\n")

	if cats["decision"] {
		sb.WriteString("- **decision**: Newer decisions supersede older ones on the same topic. ")
		sb.WriteString("Carry forward non-contradicted aspects of older decisions.\n")
	}
	if cats["pattern"] {
		sb.WriteString("- **pattern**: Accumulate all patterns. Patterns do not contradict each other — ")
		sb.WriteString("they represent different observations.\n")
	}
	if cats["gotcha"] {
		sb.WriteString("- **gotcha**: De-duplicate gotchas that describe the same issue. ")
		sb.WriteString("Keep the most detailed description.\n")
	}
	if cats["context"] {
		sb.WriteString("- **context**: Carry forward all contextual information unless explicitly ")
		sb.WriteString("superseded by a newer learning.\n")
	}
	if cats["reference"] {
		sb.WriteString("- **reference**: Preserve reference material as-is. Do not summarize or merge.\n")
	}

	return sb.String()
}

// buildCompiledArticle creates the full markdown content for a compiled
// article, including frontmatter, the synthesized current-state section,
// and a history table.
func buildCompiledArticle(cl Cluster, synthesized string) string {
	var sb strings.Builder

	// Frontmatter.
	sb.WriteString("---\n")
	sb.WriteString("tier: draft\n")
	sb.WriteString(fmt.Sprintf("compiled_at: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString("sources:\n")
	for _, l := range cl.Learnings {
		sb.WriteString(fmt.Sprintf("  - %s\n", l.Identity))
	}
	sb.WriteString(fmt.Sprintf("topic: %s\n", cl.DominantTag))
	sb.WriteString("---\n\n")

	// Title.
	sb.WriteString(fmt.Sprintf("# %s\n\n", cl.Topic))

	// Synthesized current state.
	sb.WriteString(strings.TrimSpace(synthesized))
	sb.WriteString("\n\n")

	// History table.
	sb.WriteString("## History\n\n")
	sb.WriteString("| Learning | Date | Category | Summary |\n")
	sb.WriteString("|----------|------|----------|--------|\n")
	for _, l := range cl.Learnings {
		cat := l.Category
		if cat == "" {
			cat = "context"
		}
		// Truncate content for the summary column (first 80 chars).
		summary := l.Content
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		// Remove newlines from summary for table formatting.
		summary = strings.ReplaceAll(summary, "\n", " ")
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			l.Identity,
			l.CreatedAt.UTC().Format("2006-01-02"),
			cat,
			summary,
		))
	}

	return sb.String()
}

// persistCompiledArticle stores a compiled article as a page in the store
// with source_id="compiled" and tier="draft". Also persists blocks and
// generates embeddings.
func (c *Compile) persistCompiledArticle(cl Cluster, articleContent string) error {
	pageName := "compiled/" + cl.DominantTag

	// Build sources list for properties.
	var sources []string
	for _, l := range cl.Learnings {
		sources = append(sources, l.Identity)
	}
	propsMap := map[string]any{
		"topic":       cl.DominantTag,
		"compiled_at": time.Now().UTC().Format(time.RFC3339),
		"sources":     sources,
		"tier":        "draft",
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	page := &store.Page{
		Name:         pageName,
		OriginalName: cl.Topic,
		SourceID:     "compiled",
		SourceDocID:  cl.DominantTag,
		Properties:   string(propsJSON),
		Tier:         "draft",
	}
	if err := c.store.InsertPage(page); err != nil {
		return fmt.Errorf("insert compiled page: %w", err)
	}

	// Parse and persist blocks.
	_, blocks := vault.ParseDocument(cl.DominantTag, articleContent)
	if err := vault.PersistBlocks(c.store, pageName, blocks, sql.NullString{}, 0); err != nil {
		return fmt.Errorf("persist compiled blocks: %w", err)
	}

	// Generate embeddings if available.
	if c.embedder != nil && c.embedder.Available() {
		vault.GenerateEmbeddings(c.store, c.embedder, pageName, blocks, nil)
	}

	return nil
}

// writeIndex generates the _index.md file listing all compiled articles.
func (c *Compile) writeIndex(clusters []Cluster) error {
	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return fmt.Errorf("create compiled directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Compiled Knowledge Index\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().UTC().Format(time.RFC3339)))

	sb.WriteString("| Topic | Articles | Sources |\n")
	sb.WriteString("|-------|----------|--------|\n")
	for _, cl := range clusters {
		sb.WriteString(fmt.Sprintf("| [%s](%s.md) | 1 | %d learnings |\n",
			cl.Topic, cl.DominantTag, len(cl.Learnings)))
	}

	indexPath := filepath.Join(compiledDir, "_index.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0o644)
}

// writeEmptyIndex generates an _index.md file indicating no learnings
// exist to compile. This satisfies invariant 6: "When no learnings exist,
// produce an empty _index.md with 'No learnings to compile'".
func (c *Compile) writeEmptyIndex() error {
	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return fmt.Errorf("create compiled directory: %w", err)
	}

	content := fmt.Sprintf("# Compiled Knowledge Index\n\n*Generated: %s*\n\nNo learnings to compile.\n",
		time.Now().UTC().Format(time.RFC3339))

	indexPath := filepath.Join(compiledDir, "_index.md")
	return os.WriteFile(indexPath, []byte(content), 0o644)
}
