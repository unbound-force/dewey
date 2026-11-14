package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/llm"
	"github.com/unbound-force/dewey/v3/sanitize"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
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
	// Identity is the learning's identity: the current
	// "{tag}-{YYYYMMDDTHHMMSS}-{author}" form or a legacy
	// "{tag}-{sequence}" form (e.g., "authentication-3").
	Identity string
	// Tag is the learning's topic tag, the sole clustering dimension.
	Tag string
	// Category is the learning's category: decision, pattern, gotcha,
	// context, or reference. Empty is treated as "context".
	Category string
	// CreatedAt is the learning's creation time, used for chronological
	// ordering within a cluster.
	CreatedAt time.Time
	// Content is the learning's block content.
	Content string
	// Validity is the temporal-validity token decoded from the learning's
	// page properties — one of current, stale-unverified, superseded, or
	// historical. Empty when the properties carry no validity key or the
	// value failed decode-boundary validation (ES-FR-003).
	Validity string
	// Supersedes is the identity of the learning this learning declares
	// replaced, normalized with supersedesIdentity and validated against
	// the learning-identity grammar. Empty when absent or withheld at the
	// decode boundary (ES-FR-003).
	Supersedes string
	// SupersededBy is the identity recorded as this learning's successor,
	// normalized with supersedesIdentity and validated against the
	// learning-identity grammar. Empty when absent or withheld at the
	// decode boundary (ES-FR-003).
	SupersededBy string
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
	mu            *sync.Mutex
	store         *store.Store
	embedder      embed.Embedder
	synth         llm.Synthesizer
	vaultPath     string
	maxChunkChars int // max chars per embedding chunk; 0 uses embed.DefaultMaxChunkChars
}

// NewCompile creates a new Compile tool handler with the given dependencies.
// The store must be non-nil for the tool to function. The embedder and
// synthesizer may be nil — the tool degrades gracefully:
//   - nil embedder: clustering uses tags only (no semantic refinement)
//   - nil synthesizer: returns clusters + prompts without LLM synthesis
//
// vaultPath is the vault root directory (not the .uf/dewey/ workspace).
// mu is the pipeline-wide index mutex shared with indexing and curation; pass
// nil to create a private mutex (used by CLI invocations and isolated tests).
// The maxChunkChars parameter controls the maximum character length per
// embedding chunk; pass 0 to use embed.DefaultMaxChunkChars.
func NewCompile(s *store.Store, e embed.Embedder, synth llm.Synthesizer, vaultPath string, mu *sync.Mutex, maxChunkChars int) *Compile {
	if mu == nil {
		mu = &sync.Mutex{}
	}
	return &Compile{mu: mu, store: s, embedder: e, synth: synth, vaultPath: vaultPath, maxChunkChars: maxChunkChars}
}

// effectiveMaxChunkChars returns the configured max chunk chars, falling back
// to embed.DefaultMaxChunkChars when no override is set.
func (c *Compile) effectiveMaxChunkChars() int {
	if c.maxChunkChars > 0 {
		return c.maxChunkChars
	}
	return embed.DefaultMaxChunkChars
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
		return errorResult(persistentStorageError("compile").Error()), nil, nil
	}

	// Non-blocking mutual exclusion on the pipeline-wide index mutex: if an
	// index, curation, or another compile is running, return immediately
	// rather than queuing.
	if !c.mu.TryLock() {
		return errorResult("compilation already in progress (indexing or curation holds the index lock)"), nil, nil
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

	// Handle empty learnings case. In prompt mode there is nothing to
	// synthesize, so leave existing compiled artifacts untouched. Only a real
	// synthesizer may clear and replace compiled output.
	if len(pages) == 0 {
		if c.synth == nil || !c.synth.Available() {
			return c.returnPromptMode(nil, 0, start, synthesisUnavailableReason(c.synth))
		}
		retained := c.clearCompiled()
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
		if len(retained) > 0 {
			result["retained_validated"] = retained
			result["message"] = fmt.Sprintf("No learnings to compile. %d validated article(s) retained.", len(retained))
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

	// Process clusters: synthesize and write articles. Existing articles are
	// only replaced after synthesis succeeds, so a provider failure never
	// destroys published knowledge.
	return c.processClusters(ctx, clusters, len(entries), start, true)
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

	// Count total learnings across affected clusters for the summary.
	totalLearnings := 0
	for _, cl := range clusters {
		totalLearnings += len(cl.Learnings)
	}

	return c.processClusters(ctx, clusters, totalLearnings, start, false)
}

// synthesizedCluster is a successfully synthesized article awaiting
// replacement, carrying the supersession proposals parsed out of its
// synthesized text (design D4).
type synthesizedCluster struct {
	cluster   Cluster
	article   string
	tier      string
	model     string
	proposals []SupersessionProposal
	dropped   []DroppedProposal
}

// processClusters handles the shared logic for both full and incremental
// compilation: synthesize articles, replace them atomically per topic, and
// persist the successful ones. A full rebuild additionally removes topics
// that no longer have learnings, while leaving articles for failed topics in
// place. When every cluster fails, nothing is changed.
func (c *Compile) processClusters(ctx context.Context, clusters []Cluster, totalLearnings int, start time.Time, full bool) (*mcp.CallToolResult, any, error) {
	// Prompt mode never mutates existing compiled artifacts.
	if c.synth == nil || !c.synth.Available() {
		return c.returnPromptMode(clusters, totalLearnings, start, synthesisUnavailableReason(c.synth))
	}

	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return errorResult(fmt.Sprintf("failed to create compiled directory: %v", err)), nil, nil
	}

	produced, failures := c.synthesizeClusters(ctx, clusters, c.synth.ModelID())
	if len(produced) == 0 && len(failures) > 0 {
		return compileFailureResult(totalLearnings, failures)
	}

	var retainedValidated []string
	if full {
		retainedValidated = c.removeStaleTopics(compiledDir, clusters)
	}

	articles, replaceFailures := c.replaceArticles(compiledDir, produced)
	failures = append(failures, replaceFailures...)

	// Supersession proposals and dropped entries ride the compiled result
	// for the review gate (ES-FR-006, design D8). The keys are always
	// present in compiled mode — prompt mode carries neither field — so
	// the gate can tell "compiled, nothing proposed" from "prompt mode".
	run := newProposalRun()
	for i := range produced {
		kept, linkDropped := filterProposalsByLinkState(c.store, run, produced[i].proposals)
		produced[i].proposals = kept
		produced[i].dropped = append(produced[i].dropped, linkDropped...)
	}
	proposedSupersessions, droppedProposals := proposalResultFields(produced)

	if err := c.writeIndex(); err != nil {
		compileLogger.Warn("failed to write index", "err", err)
		failures = append(failures, map[string]string{"topic": "_index", "error": err.Error()})
	}

	elapsed := time.Since(start)
	compileLogger.Info("compile complete",
		"articles", len(articles),
		"failed", len(failures),
		"learnings", totalLearnings,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	result := map[string]any{
		"status":                 "compiled",
		"articles":               articles,
		"index":                  filepath.Join(compiledDir, "_index.md"),
		"total_learnings":        totalLearnings,
		"total_articles":         len(articles),
		"proposed_supersessions": proposedSupersessions,
		"dropped_proposals":      droppedProposals,
		"message":                fmt.Sprintf("Compiled %d learnings into %d articles.", totalLearnings, len(articles)),
	}
	if len(failures) > 0 {
		result["status"] = "partial"
		result["failed"] = failures
		result["total_failed"] = len(failures)
		result["message"] = fmt.Sprintf("Compiled %d learnings into %d articles; %d topic(s) failed.", totalLearnings, len(articles), len(failures))
	}
	if len(retainedValidated) > 0 {
		result["retained_validated"] = retainedValidated
	}
	res, err := jsonTextResult(result)
	if err != nil {
		return nil, nil, err
	}
	if len(failures) > 0 {
		res.IsError = true
	}
	return res, nil, nil
}

// synthesizeClusters runs the provider for every cluster and returns the
// successful articles plus per-topic failures. Every article carries tier
// draft (VTB-FR-003): compile never looks up the existing page's tier, and
// preserving a validated tier across byte-identical regeneration is
// exclusively the store's content-bound UpdatePage rule (VTB-FR-001). The
// synthesizing model is recorded.
func (c *Compile) synthesizeClusters(ctx context.Context, clusters []Cluster, model string) ([]synthesizedCluster, []map[string]string) {
	produced := make([]synthesizedCluster, 0, len(clusters))
	failures := make([]map[string]string, 0)
	for _, cl := range clusters {
		compileLogger.Info("synthesizing article", "topic", cl.Topic, "learnings", len(cl.Learnings))
		synthesized, err := c.synth.Synthesize(ctx, buildSynthesisPrompt(cl))
		if err != nil {
			compileLogger.Warn("synthesis failed", "topic", cl.Topic, "err", err)
			failures = append(failures, map[string]string{"topic": cl.DominantTag, "error": err.Error()})
			continue
		}

		// Compile is a machine write path and must never write tier
		// "validated" (VTB-FR-003). Preservation of an existing validated
		// tier is exclusively the store's content-bound rule (VTB-FR-001):
		// byte-identical article bytes keep validated; changed bytes land
		// at draft.
		tier := "draft"
		// Parse, validate, and strip the trailing Supersession Proposals
		// section before the article is built, so stored articles never
		// contain it (ES-FR-005) and the proposals ride the result to the
		// review gate (ES-FR-006). Invalid entries are dropped and
		// reported, never fatal to the article.
		sourceIDs := make([]string, len(cl.Learnings))
		for i, l := range cl.Learnings {
			sourceIDs[i] = l.Identity
		}
		synthesized, proposals, dropped := parseSupersessionProposals(synthesized, sourceIDs)
		produced = append(produced, synthesizedCluster{
			cluster:   cl,
			article:   buildCompiledArticle(cl, synthesized, tier, model),
			tier:      tier,
			model:     model,
			proposals: proposals,
			dropped:   dropped,
		})
	}
	return produced, failures
}

// removeStaleTopics deletes compiled articles for topics that no longer have
// learnings. Failed topics are in clusters and therefore retained.
// Human-validated articles are never deleted; their tags are returned so the
// compile result can report them as retained.
func (c *Compile) removeStaleTopics(compiledDir string, clusters []Cluster) []string {
	currentTopics := make(map[string]struct{}, len(clusters))
	for _, cl := range clusters {
		currentTopics[cl.DominantTag] = struct{}{}
	}

	existingPages, err := c.store.ListPagesBySource(store.SourceCompiled)
	if err != nil {
		compileLogger.Warn("failed to list compiled pages for stale cleanup", "err", err)
		return nil
	}
	var retained []string
	for _, p := range existingPages {
		tag := p.SourceDocID
		if tag == "" {
			tag = strings.TrimPrefix(p.Name, store.CompiledPrefix)
		}
		if _, ok := currentTopics[tag]; ok {
			continue
		}
		if p.Tier == "validated" {
			retained = append(retained, tag)
			continue
		}
		if !sanitize.ValidTag(tag) {
			compileLogger.Warn("skipping stale compiled page with unsafe tag", "page", p.Name, "tag", tag)
			continue
		}
		stalePath := filepath.Join(compiledDir, tag+".md")
		if !pathWithin(compiledDir, stalePath) {
			compileLogger.Warn("skipping stale compiled path outside the compiled directory", "path", stalePath)
			continue
		}
		if err := c.store.DeletePage(p.Name); err != nil {
			// Keep the file when the store row survives so the store and the
			// filesystem cannot drift apart.
			compileLogger.Warn("failed to delete stale compiled page", "page", p.Name, "err", err)
			continue
		}
		if err := os.Remove(stalePath); err != nil && !os.IsNotExist(err) {
			compileLogger.Warn("failed to remove stale compiled file", "path", stalePath, "err", err)
		}
	}
	return retained
}

// replaceArticles stages each article, persists the page and blocks, then
// renames the staged file over the previous article. A failure at any step
// leaves the previous page and file intact.
func (c *Compile) replaceArticles(compiledDir string, produced []synthesizedCluster) ([]map[string]any, []map[string]string) {
	articles := make([]map[string]any, 0, len(produced))
	failures := make([]map[string]string, 0)

	for _, sc := range produced {
		tag := sc.cluster.DominantTag
		if !sanitize.ValidTag(tag) {
			failures = append(failures, map[string]string{"topic": tag, "error": "invalid tag: only letters, digits, '-' and '_' are allowed"})
			continue
		}
		articlePath := filepath.Join(compiledDir, tag+".md")
		if !pathWithin(compiledDir, articlePath) {
			failures = append(failures, map[string]string{"topic": tag, "error": "refusing to write outside the compiled directory"})
			continue
		}

		if err := c.writeArticle(articlePath, sc); err != nil {
			compileLogger.Warn("failed to write article", "topic", tag, "err", err)
			failures = append(failures, map[string]string{"topic": tag, "error": err.Error()})
			continue
		}

		var sources []string
		for _, l := range sc.cluster.Learnings {
			sources = append(sources, l.Identity)
		}
		// Per-article citation coverage (ES-FR-010): computed on the
		// written article, with each source's supersession status
		// resolved here from the decoded temporal fields so the pure
		// helper performs no store lookup.
		covSources := make([]CitationSource, len(sc.cluster.Learnings))
		for i, l := range sc.cluster.Learnings {
			covSources[i] = CitationSource{
				Identity:   l.Identity,
				Superseded: supersededFrom(l.SupersededBy, l.Validity),
			}
		}
		coverage := CheckCitationCoverage(CoverageOptions{Article: sc.article, Sources: covSources})
		articles = append(articles, map[string]any{
			"path":           articlePath,
			"topic":          sc.cluster.Topic,
			"sources":        sources,
			"learning_count": len(sc.cluster.Learnings),
			"tier":           sc.tier,
			"compiled_by":    sc.model,
			"coverage":       coverageResultMap(coverage),
		})
	}
	return articles, failures
}

// writeArticle stages an article beside its destination, persists the page
// and blocks, then atomically renames the staged file into place.
//
// Ordering rationale: the store row is written before the file is renamed
// on purpose. A crash between the two leaves a hidden .tmp file (ignored
// by the next compile) rather than a file the store does not know about,
// and the next compile re-persists the page anyway — the store, not the
// filesystem, is the source of truth for search, so it must never lag
// behind the file (PR 145 review amendment).
func (c *Compile) writeArticle(articlePath string, sc synthesizedCluster) error {
	tmpPath := filepath.Join(filepath.Dir(articlePath), "."+filepath.Base(articlePath)+".tmp")
	if err := os.WriteFile(tmpPath, []byte(sc.article), 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := c.persistCompiledArticle(sc.cluster, sc.article, sc.tier, sc.model); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, articlePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// compileFailureResult renders the total-failure result, which leaves all
// existing artifacts untouched.
func compileFailureResult(totalLearnings int, failures []map[string]string) (*mcp.CallToolResult, any, error) {
	payload := map[string]any{
		"status":          "failed",
		"failed":          failures,
		"articles":        []any{},
		"total_learnings": totalLearnings,
		"total_articles":  0,
		"total_failed":    len(failures),
		"message":         "synthesis failed for every topic; existing compiled articles were left unchanged.",
	}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, nil, err
	}
	res.IsError = true
	return res, nil, nil
}

// clearCompiled removes all non-validated compiled pages and their article
// files, retaining human-validated articles in both the store and the
// filesystem. Used only when there are no learnings left to compile. It
// returns the tags of retained validated articles so callers can report
// them.
func (c *Compile) clearCompiled() []string {
	deleted, err := c.store.DeletePagesBySourceExcludingTier(store.SourceCompiled, "validated")
	if err != nil {
		compileLogger.Warn("failed to delete existing compiled pages", "err", err)
	}
	compiledDir := c.compiledDir()
	for _, name := range deleted {
		tag := strings.TrimPrefix(name, store.CompiledPrefix)
		if !sanitize.ValidTag(tag) {
			continue
		}
		stalePath := filepath.Join(compiledDir, tag+".md")
		if !pathWithin(compiledDir, stalePath) {
			continue
		}
		if err := os.Remove(stalePath); err != nil && !os.IsNotExist(err) {
			compileLogger.Warn("failed to remove compiled file", "path", stalePath, "err", err)
		}
	}
	// The index is regenerated by writeEmptyIndex; remove the directory only
	// when no validated article files are retained.
	if err := os.Remove(filepath.Join(compiledDir, "_index.md")); err != nil && !os.IsNotExist(err) {
		compileLogger.Warn("failed to remove compiled index", "err", err)
	}
	retained := c.retainedValidatedTopics()
	if len(retained) == 0 {
		if err := os.Remove(compiledDir); err != nil && !os.IsNotExist(err) {
			compileLogger.Warn("failed to remove compiled directory", "err", err)
		}
	}
	return retained
}

// retainedValidatedTopics lists the tags of compiled pages still marked
// validated after a cleanup pass. They are reported in the compile result so
// callers can see that human-reviewed articles were retained, not lost.
func (c *Compile) retainedValidatedTopics() []string {
	pages, err := c.store.ListPagesBySource(store.SourceCompiled)
	if err != nil {
		compileLogger.Warn("failed to list retained compiled pages", "err", err)
		return nil
	}
	var retained []string
	for _, p := range pages {
		if p.Tier != "validated" {
			continue
		}
		tag := p.SourceDocID
		if tag == "" {
			tag = strings.TrimPrefix(p.Name, store.CompiledPrefix)
		}
		if sanitize.ValidTag(tag) {
			retained = append(retained, tag)
		}
	}
	return retained
}

// returnPromptMode returns clusters with synthesis prompts when no
// synthesizer is available. The calling agent can use these prompts
// to perform synthesis externally (D4: prompt-based delegation).
func (c *Compile) returnPromptMode(clusters []Cluster, totalLearnings int, start time.Time, reason string) (*mcp.CallToolResult, any, error) {
	clusterResults := make([]map[string]any, 0, len(clusters))
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
	message := fmt.Sprintf("Clustered %d learnings into %d topics. Synthesis prompts ready for agent execution. Synthesis unavailable: %s.", totalLearnings, len(clusters), reason)
	if len(clusters) == 0 {
		message = "No learnings to compile. Synthesis unavailable: " + reason
	}
	result := map[string]any{
		"status":             "prompts_ready",
		"clusters":           clusterResults,
		"total_learnings":    totalLearnings,
		"total_clusters":     len(clusters),
		"elapsed_ms":         elapsed.Milliseconds(),
		"unavailable_reason": reason,
		"message":            message,
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// temporalEntryFields holds the temporal-validity metadata decoded onto a
// LearningEntry. The zero value renders no temporal lines: every field is
// either a validated, prompt-safe value or empty.
type temporalEntryFields struct {
	Validity     string
	Supersedes   string
	SupersededBy string
}

// decodeTemporalEntryFields decodes the #146 temporal keys (validity,
// supersedes, superseded_by) from a learning's page properties into
// prompt-safe entry fields, applying the ES-FR-003 decode boundary:
// string values are trimmed, the two link values are normalized with
// supersedesIdentity and must match the learning-identity grammar (both
// the {tag}-{YYYYMMDDTHHMMSS}-{author} and the legacy {tag}-{sequence}
// form), and validity is accepted only from the known token set. A value
// that is absent, not a string, or fails validation is returned empty so
// a hostile or corrupt property can never inject prompt text. A nil map
// (unreadable properties) yields the zero value.
func decodeTemporalEntryFields(props map[string]any) temporalEntryFields {
	var fields temporalEntryFields
	if v, ok := props["validity"].(string); ok {
		v = strings.TrimSpace(v)
		if validValidities[v] {
			fields.Validity = v
		}
	}
	if v, ok := props["supersedes"].(string); ok {
		if id := supersedesIdentity(v); isLearningIdentity(id) {
			fields.Supersedes = id
		}
	}
	if v, ok := props["superseded_by"].(string); ok {
		if id := supersedesIdentity(v); isLearningIdentity(id) {
			fields.SupersededBy = id
		}
	}
	return fields
}

// buildLearningEntries converts store pages into LearningEntry values
// by loading block content and extracting metadata from page properties.
// The temporal-validity properties are decoded through
// decodeTemporalEntryFields, which normalizes and shape-validates them at
// the decode boundary so only prompt-safe values reach the entry.
func (c *Compile) buildLearningEntries(pages []*store.Page) ([]LearningEntry, error) {
	var entries []LearningEntry
	for _, p := range pages {
		// Extract identity from page name: "learning/{identity}" → "{identity}".
		identity := strings.TrimPrefix(p.Name, store.LearningPrefix)
		tag := extractTagFromIdentity(identity, p.Properties)

		// Decode the temporal keys at the trust boundary; a value that
		// fails validation is treated as absent, never rendered
		// (ES-FR-003).
		props, propsErr := decodeLearningProperties(p.Properties)
		if propsErr != nil {
			compileLogger.Warn("unreadable learning properties; temporal fields treated as absent",
				"page", p.Name, "err", propsErr)
		}
		temporal := decodeTemporalEntryFields(props)

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
			Identity:     identity,
			Tag:          tag,
			Category:     p.Category,
			CreatedAt:    createdAt,
			Content:      content,
			Validity:     temporal.Validity,
			Supersedes:   temporal.Supersedes,
			SupersededBy: temporal.SupersededBy,
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
// Per-learning temporal metadata (validity, supersedes, superseded-by)
// renders as entry lines only when the learning carries a decoded value,
// so a legacy learning without temporal properties renders exactly as
// before (ES-FR-003, design D2).
//
// Unresolved contradictions are surfaced, never silently resolved: the
// instructions direct both positions into an `## Open Conflicts` section,
// each with its citations, and pick no winner — replacement authority is
// the declared supersession link or validity, not recency (ES-FR-001).
//
// Every factual claim ends with a bracketed citation of the learning
// identity — or comma-separated identities — it derives from: the
// instructions show both citation forms and both identity grammars the
// store holds (the current {tag}-{YYYYMMDDTHHMMSS}-{author} form and the
// legacy {tag}-{sequence} form) and state that bracketed text which is
// not a learning identity is ignored (ES-FR-004, design D3). The article
// MAY end with a strict machine-parseable `## Supersession Proposals`
// section — one entry per proposed pair, each carrying successor, target,
// and rationale — justified only when one learning clearly replaces
// another, never by recency alone; the section is parsed and stripped
// from the stored article (ES-FR-005, design D4).
func buildSynthesisPrompt(cl Cluster) string {
	var sb strings.Builder

	sb.WriteString("You are compiling learnings into a knowledge article.\n\n")
	fmt.Fprintf(&sb, "Topic: %s\n\n", cl.Topic)

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
		fmt.Fprintf(&sb, "### Learning %d: %s\n", i+1, l.Identity)
		fmt.Fprintf(&sb, "- **Category**: %s\n", cat)
		fmt.Fprintf(&sb, "- **Date**: %s\n", l.CreatedAt.UTC().Format("2006-01-02"))
		// Temporal lines render only when the entry carries a decoded
		// value, so a legacy learning renders exactly as before
		// (ES-FR-003). Direct interpolation is safe here because the
		// decode boundary (decodeTemporalEntryFields) already withheld
		// every value that is not a known validity token or an identity
		// matching the learning-identity grammar — a hostile or corrupt
		// property cannot reach this rendering.
		if l.Validity != "" {
			fmt.Fprintf(&sb, "- **Validity**: %s\n", l.Validity)
		}
		if l.Supersedes != "" {
			fmt.Fprintf(&sb, "- **Supersedes**: %s\n", l.Supersedes)
		}
		if l.SupersededBy != "" {
			fmt.Fprintf(&sb, "- **Superseded by**: %s\n", l.SupersededBy)
		}
		fmt.Fprintf(&sb, "- **Content**: %s\n\n", l.Content)
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Produce a markdown article with:\n")
	sb.WriteString("1. A `## Current State` section containing all current facts. ")
	sb.WriteString("Carry forward non-contradicted facts from older learnings.\n")
	sb.WriteString("2. Do NOT include a top-level heading (# Title) — it will be added automatically.\n")
	sb.WriteString("3. Write in a clear, factual style. No preamble or meta-commentary.\n")
	// ES-FR-001: an unresolved contradiction is surfaced, never silently
	// resolved — the resolution authority is the declared link, which only
	// a human can add between two existing learnings. The citation format
	// gets its own instruction (ES-FR-004), so this clause names citations
	// without pinning their shape.
	sb.WriteString("4. An `## Open Conflicts` section when two learnings contradict and neither a ")
	sb.WriteString("declared supersession link nor `validity: superseded` resolves the ")
	sb.WriteString("contradiction: present both positions, each with its citations. Do not pick a winner.\n")
	// ES-FR-004: every factual claim carries a bracketed citation of the
	// learning identity — or comma-separated identities — it derives from
	// (design D3). The instruction shows both citation forms and names both
	// identity grammars the store holds — the current
	// {tag}-{YYYYMMDDTHHMMSS}-{author} form and the legacy
	// {tag}-{sequence} form — so a citation of a pre-#146 learning parses
	// too, and states that bracketed text which is not a learning identity
	// is ignored, so ordinary markdown brackets are not discouraged.
	sb.WriteString("5. End every factual claim with a bracketed citation of the learning identity — ")
	sb.WriteString("or comma-separated identities — the claim derives from, for example ")
	sb.WriteString("`[auth-20261101T000000-alice]` or `[auth-20261001T000000-alice, auth-20261101T000000-bob]`. ")
	sb.WriteString("An identity is either `{tag}-{YYYYMMDDTHHMMSS}-{author}` (for example ")
	sb.WriteString("`auth-20261101T000000-alice`) or the legacy `{tag}-{sequence}` (for example `auth-1`); ")
	sb.WriteString("copy identities exactly from the learnings above. Bracketed text that is not a ")
	sb.WriteString("learning identity is ignored.\n")
	// ES-FR-005: the article MAY end with a strict machine-parseable
	// `## Supersession Proposals` section (design D4) — one entry per
	// proposed pair, each carrying successor, target, and rationale. A
	// proposal is justified only when one learning clearly replaces the
	// other; recency alone is not grounds — the same authority rule the
	// Open Conflicts instruction states. The shape is strict because
	// compile parses and strips the section before storing the article;
	// entries that miss it are dropped and reported, never kept.
	sb.WriteString("6. An optional `## Supersession Proposals` section at the end of the article when ")
	sb.WriteString("one learning clearly replaces another — recency alone is not grounds for a proposal. ")
	sb.WriteString("Omit the section entirely when no learning clearly replaces another. ")
	sb.WriteString("Use exactly this shape, one entry per proposed pair:\n\n")
	sb.WriteString("```\n## Supersession Proposals\n\n- successor: <identity>\n  target: <identity>\n  rationale: <single line>\n```\n")

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
		sb.WriteString("- **decision**: A decision is replaced only when a declared supersession ")
		sb.WriteString("link or `validity: superseded` says so. ")
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
		sb.WriteString("- **context**: Carry forward all contextual information unless a declared ")
		sb.WriteString("supersession link or `validity: superseded` says otherwise.\n")
	}
	if cats["reference"] {
		sb.WriteString("- **reference**: Preserve reference material as-is. Do not summarize or merge.\n")
	}

	return sb.String()
}

// buildCompiledArticle creates the full markdown content for a compiled
// article, including frontmatter, the synthesized current-state section,
// and a history table.
func buildCompiledArticle(cl Cluster, synthesized, tier, model string) string {
	if tier == "" {
		tier = "draft"
	}
	if model != "" && !sanitize.SafeFrontmatterScalar(model) {
		compileLogger.Warn("omitting compiled_by with unsafe characters from article frontmatter")
		model = ""
	}
	var sb strings.Builder

	// Frontmatter.
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "tier: %s\n", tier)
	fmt.Fprintf(&sb, "compiled_at: %s\n", time.Now().UTC().Format(time.RFC3339))
	if model != "" {
		fmt.Fprintf(&sb, "compiled_by: %s\n", model)
	}
	sb.WriteString("sources:\n")
	for _, l := range cl.Learnings {
		fmt.Fprintf(&sb, "  - %s\n", l.Identity)
	}
	fmt.Fprintf(&sb, "topic: %s\n", cl.DominantTag)
	sb.WriteString("---\n\n")

	// Title.
	fmt.Fprintf(&sb, "# %s\n\n", cl.Topic)

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
		// Truncate content for the summary column (first 80 runes).
		// Use []rune to avoid slicing mid-multibyte character (e.g. em-dash
		// U+2014 is 3 bytes; byte-slicing at boundary produces invalid UTF-8).
		summary := l.Content
		if runes := []rune(summary); len(runes) > 80 {
			summary = string(runes[:80]) + "..."
		}
		// Remove newlines from summary for table formatting.
		summary = strings.ReplaceAll(summary, "\n", " ")
		fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
			l.Identity,
			l.CreatedAt.UTC().Format("2006-01-02"),
			cat,
			summary,
		)
	}

	return sb.String()
}

// upsertCompiledPage inserts a new compiled page or updates the existing one.
// UpdatePage preserves the page's blocks until persistCompiledArticle replaces
// them explicitly.
func (c *Compile) upsertCompiledPage(page *store.Page) error {
	existing, err := c.store.GetPage(page.Name)
	if err != nil {
		return fmt.Errorf("look up existing compiled page: %w", err)
	}
	if existing != nil {
		if err := c.store.UpdatePage(page); err != nil {
			return fmt.Errorf("update compiled page: %w", err)
		}
		return nil
	}
	if err := c.store.InsertPage(page); err != nil {
		return fmt.Errorf("insert compiled page: %w", err)
	}
	return nil
}

// contentHash returns the hex encoding of the first 8 bytes of the SHA-256
// digest of b — the truncated-hash convention the curation re-index and
// learning re-ingestion paths also record (a 16-character string). It is a
// change-detection fingerprint for the store's content-bound tier rule
// (VTB-FR-001, VTB-FR-002), not a cryptographic integrity check: the 8-byte
// truncation accepts a 2^-64 collision probability by design. The helper is
// scoped to the compiled article write paths (persistCompiledArticle,
// StoreCompiled); vault and disk re-ingestion keep their own hashing
// conventions and must not use it.
func contentHash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// persistCompiledArticle stores a compiled article as a page in the store
// with source_id="compiled" and the supplied tier (always draft from the
// compile path — a machine write never carries validated; preserving an
// existing validated tier is exclusively the store's content-bound rule,
// VTB-FR-001). Also persists blocks and generates embeddings.
func (c *Compile) persistCompiledArticle(cl Cluster, articleContent, tier, model string) error {
	if tier == "" {
		tier = "draft"
	}
	if model != "" && !sanitize.SafeFrontmatterScalar(model) {
		compileLogger.Warn("omitting compiled_by with unsafe characters from page properties")
		model = ""
	}
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
		"tier":        tier,
	}
	if model != "" {
		propsMap["compiled_by"] = model
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	// The hash covers the exact article bytes persisted here and written to
	// disk by writeArticle, so the store's content-bound tier rule
	// (VTB-FR-001) preserves validated only when regeneration is
	// byte-identical (VTB-FR-002).
	page := &store.Page{
		Name:         pageName,
		OriginalName: cl.Topic,
		SourceID:     "compiled",
		SourceDocID:  cl.DominantTag,
		Properties:   string(propsJSON),
		ContentHash:  contentHash([]byte(articleContent)),
		Tier:         tier,
	}
	if err := c.upsertCompiledPage(page); err != nil {
		return err
	}

	// Replace blocks for the page (UpdatePage leaves the old blocks in place).
	if err := c.store.DeleteBlocksByPage(pageName); err != nil {
		return fmt.Errorf("clear compiled blocks: %w", err)
	}

	// Parse and persist blocks.
	_, blocks := vault.ParseDocument(cl.DominantTag, articleContent)
	if err := vault.PersistBlocks(c.store, pageName, blocks, sql.NullString{}, 0); err != nil {
		return fmt.Errorf("persist compiled blocks: %w", err)
	}

	// Generate embeddings if available.
	if c.embedder != nil && c.embedder.Available() {
		vault.GenerateEmbeddings(c.store, c.embedder, pageName, blocks, nil, c.effectiveMaxChunkChars())
	}

	return nil
}

// writeIndex generates _index.md from the compiled pages currently in the
// store. Deriving it from the store (rather than from the clusters produced by
// this run) keeps the index correct for incremental compiles and for topics
// whose synthesis failed but whose previous article was retained.
func (c *Compile) writeIndex() error {
	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return fmt.Errorf("create compiled directory: %w", err)
	}

	pages, err := c.store.ListPagesBySource("compiled")
	if err != nil {
		return fmt.Errorf("list compiled pages: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Compiled Knowledge Index\n\n")
	fmt.Fprintf(&sb, "*Generated: %s*\n\n", time.Now().UTC().Format(time.RFC3339))

	sb.WriteString("| Topic | Articles | Sources |\n")
	sb.WriteString("|-------|----------|--------|\n")
	for _, p := range pages {
		if !sanitize.ValidTag(p.SourceDocID) {
			continue
		}
		topic := p.OriginalName
		if topic == "" {
			topic = p.SourceDocID
		}
		sourceCount := 0
		var props map[string]any
		if err := json.Unmarshal([]byte(p.Properties), &props); err == nil {
			if sources, ok := props["sources"].([]any); ok {
				sourceCount = len(sources)
			}
		}
		fmt.Fprintf(&sb, "| [%s](%s.md) | 1 | %d learnings |\n",
			topic, p.SourceDocID, sourceCount)
	}

	indexPath := filepath.Join(compiledDir, "_index.md")
	return os.WriteFile(indexPath, []byte(sb.String()), 0o644)
}

// pathWithin reports whether target resolves inside base (lexically). It is a
// defense-in-depth check for filenames derived from tags.
func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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

// resolveCompiledSources resolves store_compiled's declared sources
// against existing learning pages (#93, ES-FR-012). Each source is
// normalized like a declared citation source and looked up by its
// learning page; resolved sources return as CitationSources with their
// supersession status decoded from page properties. Sources that resolve
// to no learning — or whose lookup fails — are returned in unresolved
// (fail closed: existence cannot be verified).
func (c *Compile) resolveCompiledSources(sources []string) (covSources []CitationSource, unresolved []string) {
	covSources = make([]CitationSource, 0, len(sources))
	for _, s := range sources {
		id := supersedesIdentity(s)
		if id == "" {
			unresolved = append(unresolved, s)
			continue
		}
		page, err := c.store.GetPage(store.LearningPrefix + id)
		if err != nil || page == nil {
			unresolved = append(unresolved, id)
			continue
		}
		covSources = append(covSources, CitationSource{
			Identity:   id,
			Superseded: learningSuperseded(page),
		})
	}
	return covSources, unresolved
}

// buildStoredArticle renders a store_compiled article: frontmatter (tier,
// compiled_at, optional compiled_by and sources, topic) followed by the
// trimmed article content. Extracted from StoreCompiled to keep that
// method's complexity bounded once coverage and proposal handling joined
// the write path.
func buildStoredArticle(tag, content, model, tier string, sources []string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "tier: %s\n", tier)
	fmt.Fprintf(&sb, "compiled_at: %s\n", time.Now().UTC().Format(time.RFC3339))
	if model != "" {
		fmt.Fprintf(&sb, "compiled_by: %s\n", model)
	}
	if len(sources) > 0 {
		sb.WriteString("sources:\n")
		for _, s := range sources {
			fmt.Fprintf(&sb, "  - %s\n", s)
		}
	}
	fmt.Fprintf(&sb, "topic: %s\n", tag)
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(content))
	sb.WriteString("\n")
	return sb.String()
}

// StoreCompiled persists a compiled article that was synthesized by the calling
// agent. This closes the loop on the nil-synthesizer MCP path: the agent calls
// compile → gets prompts → synthesizes → calls store_compiled to persist.
// Returns the shared persistent-storage error when the store is nil (degraded,
// store-less mode) instead of panicking, and does so before any article file
// is written (spec: Degraded-Mode Safety).
func (c *Compile) StoreCompiled(_ context.Context, _ *mcp.CallToolRequest, input types.StoreCompiledInput) (*mcp.CallToolResult, any, error) {
	if c.store == nil {
		return errorResult(persistentStorageError("store_compiled").Error()), nil, nil
	}
	// Non-blocking mutual exclusion on the pipeline-wide index mutex, mirroring
	// Compile: the validated-refusal check below is check-then-act, so it must
	// not interleave with a compile that rewrites the same pages.
	if !c.mu.TryLock() {
		return errorResult("store_compiled already in progress (indexing, curation, or compile holds the index lock)"), nil, nil
	}
	defer c.mu.Unlock()
	if input.Tag == "" {
		return errorResult("tag is required"), nil, nil
	}
	if input.Content == "" {
		return errorResult("content is required"), nil, nil
	}

	// Validate tag to prevent path traversal (FR-010).
	if !sanitize.ValidTag(input.Tag) {
		return errorResult("invalid tag: must contain only alphanumeric characters, hyphens, and underscores"), nil, nil
	}

	// Validate model to prevent YAML frontmatter injection.
	if input.Model != "" && !sanitize.SafeFrontmatterScalar(input.Model) {
		return errorResult("invalid model: must not contain control characters"), nil, nil
	}

	// Validate sources to prevent YAML frontmatter injection.
	for _, s := range input.Sources {
		if !sanitize.SafeFrontmatterScalar(s) {
			return errorResult("invalid source: must not contain control characters"), nil, nil
		}
	}

	// Filter empty source entries.
	var sources []string
	for _, s := range input.Sources {
		if s != "" {
			sources = append(sources, s)
		}
	}

	// Never overwrite human-validated content with agent-generated text.
	pageName := "compiled/" + input.Tag
	tier := "draft"
	existing, err := c.store.GetPage(pageName)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to look up existing compiled page: %v", err)), nil, nil
	}
	if existing != nil && existing.Tier == "validated" {
		return errorResult(fmt.Sprintf(
			"compiled page '%s' is validated; refusing to overwrite reviewed content. Store the updated article under a new tag instead.",
			pageName)), nil, nil
	}

	// Every declared source must map to an existing learning (#93,
	// ES-FR-012). Refuse before any page or file is written, naming the
	// unresolved sources. Checked after the validated refusal so the
	// human-reviewed-content invariant keeps precedence over input
	// validation.
	covSources, unresolved := c.resolveCompiledSources(sources)
	if len(unresolved) > 0 {
		return errorResult(fmt.Sprintf(
			"unresolved sources (no matching learning): %s",
			strings.Join(unresolved, ", "))), nil, nil
	}

	// Parse, validate, and strip the trailing Supersession Proposals
	// section (ES-FR-013): the stored article never contains it, and the
	// proposals ride the result for the review gate, exactly as on the
	// compile path.
	content, proposals, droppedEntries := parseSupersessionProposals(input.Content, sources)
	proposals, linkDropped := filterProposalsByLinkState(c.store, newProposalRun(), proposals)
	droppedEntries = append(droppedEntries, linkDropped...)
	articleContent := buildStoredArticle(input.Tag, content, input.Model, tier, sources)

	// Per-article citation coverage (ES-FR-013), computed on the stored
	// article with the resolved sources.
	coverage := CheckCitationCoverage(CoverageOptions{Article: articleContent, Sources: covSources})

	// Write to filesystem.
	compiledDir := c.compiledDir()
	if err := os.MkdirAll(compiledDir, 0o755); err != nil {
		return errorResult(fmt.Sprintf("failed to create compiled directory: %v", err)), nil, nil
	}
	articlePath := filepath.Join(compiledDir, input.Tag+".md")
	tmpPath := filepath.Join(compiledDir, "."+input.Tag+".md.tmp")
	if err := os.WriteFile(tmpPath, []byte(articleContent), 0o644); err != nil {
		return errorResult(fmt.Sprintf("failed to write compiled article: %v", err)), nil, nil
	}

	// Persist in store.
	propsMap := map[string]any{
		"topic":       input.Tag,
		"compiled_at": time.Now().UTC().Format(time.RFC3339),
		"tier":        tier,
	}
	if input.Model != "" {
		propsMap["compiled_by"] = input.Model
	}
	if len(sources) > 0 {
		propsMap["sources"] = sources
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to marshal properties: %v", err)), nil, nil
	}

	// The hash covers the exact article bytes staged for the article file
	// above, so the store's content-bound tier rule (VTB-FR-001) sees this
	// write as content-bearing (VTB-FR-002): identical bytes hash
	// identically across both compiled write paths.
	page := &store.Page{
		Name:         pageName,
		OriginalName: input.Tag,
		SourceID:     "compiled",
		SourceDocID:  input.Tag,
		Properties:   string(propsJSON),
		ContentHash:  contentHash([]byte(articleContent)),
		Tier:         tier,
	}
	if existing != nil {
		if err := c.store.UpdatePage(page); err != nil {
			_ = os.Remove(tmpPath)
			return errorResult(fmt.Sprintf("failed to update compiled page: %v", err)), nil, nil
		}
	} else if err := c.store.InsertPage(page); err != nil {
		_ = os.Remove(tmpPath)
		return errorResult(fmt.Sprintf("failed to insert compiled page: %v", err)), nil, nil
	}

	// Replace blocks before publishing the file so a block failure leaves the
	// previous file in place.
	if err := c.store.DeleteBlocksByPage(pageName); err != nil {
		_ = os.Remove(tmpPath)
		return errorResult(fmt.Sprintf("failed to replace compiled blocks: %v", err)), nil, nil
	}
	_, blocks := vault.ParseDocument(input.Tag, articleContent)
	if err := vault.PersistBlocks(c.store, pageName, blocks, sql.NullString{}, 0); err != nil {
		_ = os.Remove(tmpPath)
		return errorResult(fmt.Sprintf("failed to persist compiled blocks: %v", err)), nil, nil
	}

	// Publish the article only after the store write succeeds.
	if err := os.Rename(tmpPath, articlePath); err != nil {
		_ = os.Remove(tmpPath)
		return errorResult(fmt.Sprintf("failed to publish compiled article: %v", err)), nil, nil
	}

	// Generate embeddings if available.
	if c.embedder != nil && c.embedder.Available() {
		vault.GenerateEmbeddings(c.store, c.embedder, pageName, blocks, nil, c.effectiveMaxChunkChars())
	}

	compileLogger.Info("stored compiled article", "tag", input.Tag, "path", articlePath)

	// The review-gate fields mirror compile's shape (ES-FR-013); on this
	// path topic names the stored page, since store_compiled has no
	// cluster.
	proposedSupersessions, droppedProposals := proposalMaps(proposals, droppedEntries, pageName)
	result := map[string]any{
		"status":                 "stored",
		"tag":                    input.Tag,
		"page":                   pageName,
		"path":                   articlePath,
		"sources":                sources,
		"tier":                   tier,
		"coverage":               coverageResultMap(coverage),
		"proposed_supersessions": proposedSupersessions,
		"dropped_proposals":      droppedProposals,
	}
	if input.Model != "" {
		result["compiled_by"] = input.Model
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}
