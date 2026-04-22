package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/curate"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// staleDecisionThreshold is the age after which a decision learning is
// considered stale and should be reviewed. Hardcoded per contract invariant 3.
const staleDecisionThreshold = 30 * 24 * time.Hour

// lintLogger is the package-level structured logger for lint tool operations.
var lintLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/tools/lint",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Finding represents a single lint issue detected in the knowledge index.
// Each finding includes an actionable remediation suggestion (invariant 5).
type Finding struct {
	Type        string   `json:"type"`                 // stale_decision, uncompiled, embedding_gap, contradiction
	Severity    string   `json:"severity"`             // info, warning, error
	Identity    string   `json:"identity,omitempty"`   // for single-page findings
	Identities  []string `json:"identities,omitempty"` // for contradictions (pair)
	Page        string   `json:"page,omitempty"`       // page name
	Similarity  float64  `json:"similarity,omitempty"` // for contradictions
	Description string   `json:"description"`          // human-readable description
	Remediation string   `json:"remediation"`          // actionable fix suggestion
}

// Lint implements the dewey_lint MCP tool and CLI command.
// Dependencies are injected for testability (Dependency Inversion Principle).
//
// Design decision: The embedder is optional — when nil, the contradiction
// check is skipped (invariant 6). This enables lint to run without Ollama
// while still providing value from the other 3 checks. The vaultPath is
// optional — when empty, knowledge store quality checks are skipped.
type Lint struct {
	store     *store.Store
	embedder  embed.Embedder
	vaultPath string
}

// NewLint creates a new Lint tool handler with the given store, embedder,
// and vault path. The store must be non-nil for the tool to function; a
// clear error is returned at call time if it is nil (invariant 7). The
// embedder may be nil — contradiction checking is skipped when unavailable.
// The vaultPath may be empty — knowledge store quality checks are skipped.
func NewLint(s *store.Store, e embed.Embedder, vaultPath string) *Lint {
	return &Lint{store: s, embedder: e, vaultPath: vaultPath}
}

// Lint handles the dewey_lint MCP tool. Scans the index for knowledge
// quality issues and optionally auto-repairs mechanical problems.
//
// Checks performed:
//  1. Stale decisions: learnings with category "decision" older than 30 days
//     and not validated
//  2. Uncompiled learnings: learnings not referenced by any compiled article
//  3. Embedding gaps: pages with blocks but no embeddings
//  4. Semantic contradictions: learning pairs with high similarity but
//     potentially different conclusions (same tag, different content)
//
// When fix=true, auto-repairs embedding gaps by regenerating embeddings.
// Semantic issues (contradictions, staleness) require human/agent judgment.
func (l *Lint) Lint(ctx context.Context, req *mcp.CallToolRequest, input types.LintInput) (*mcp.CallToolResult, any, error) {
	if l.store == nil {
		return errorResult("lint requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	lintLogger.Info("lint starting", "fix", input.Fix)

	var allFindings []Finding

	// Check 1: Stale decisions.
	staleFindings, err := l.checkStaleDecisions()
	if err != nil {
		lintLogger.Warn("stale decision check failed", "err", err)
	} else {
		allFindings = append(allFindings, staleFindings...)
	}

	// Check 2: Uncompiled learnings.
	uncompiledFindings, err := l.checkUncompiledLearnings()
	if err != nil {
		lintLogger.Warn("uncompiled learnings check failed", "err", err)
	} else {
		allFindings = append(allFindings, uncompiledFindings...)
	}

	// Check 3: Embedding gaps.
	gapFindings, err := l.checkEmbeddingGaps()
	if err != nil {
		lintLogger.Warn("embedding gaps check failed", "err", err)
	} else {
		allFindings = append(allFindings, gapFindings...)
	}

	// Check 4: Contradictions (requires embedder — invariant 6).
	contradictionFindings, err := l.checkContradictions()
	if err != nil {
		lintLogger.Warn("contradiction check failed", "err", err)
	} else {
		allFindings = append(allFindings, contradictionFindings...)
	}

	// Check 5: Knowledge store quality metrics (requires vaultPath — FR-025).
	knowledgeQualityFindings, knowledgeStoreSummaries, err := l.checkKnowledgeQuality()
	if err != nil {
		lintLogger.Warn("knowledge quality check failed", "err", err)
	} else {
		allFindings = append(allFindings, knowledgeQualityFindings...)
	}

	// Check 6: Stale knowledge stores (requires vaultPath — FR-026).
	staleKnowledgeFindings, err := l.checkStaleKnowledgeStores()
	if err != nil {
		lintLogger.Warn("stale knowledge store check failed", "err", err)
	} else {
		allFindings = append(allFindings, staleKnowledgeFindings...)
	}

	// Count findings by type for the summary.
	staleCount := 0
	uncompiledCount := 0
	gapCount := 0
	contradictionCount := 0
	knowledgeQualityCount := 0
	staleKnowledgeCount := 0
	for _, f := range allFindings {
		switch f.Type {
		case "stale_decision":
			staleCount++
		case "uncompiled":
			uncompiledCount++
		case "embedding_gap":
			gapCount++
		case "contradiction":
			contradictionCount++
		case "knowledge_quality":
			knowledgeQualityCount++
		case "stale_knowledge":
			staleKnowledgeCount++
		}
	}
	totalIssues := len(allFindings)

	// Auto-fix embedding gaps if requested (invariant 2: only mechanical issues).
	fixedEmbeddings := 0
	if input.Fix && len(gapFindings) > 0 {
		fixed, fixErr := l.fixEmbeddingGaps(ctx, gapFindings)
		if fixErr != nil {
			lintLogger.Warn("embedding gap fix failed", "err", fixErr)
		} else {
			fixedEmbeddings = fixed
		}
	}

	// Build the response.
	status := "clean"
	if totalIssues > 0 {
		status = "issues_found"
	}

	message := "Knowledge index is clean. No issues found."
	if totalIssues > 0 {
		message = fmt.Sprintf("Found %d issues.", totalIssues)
		if fixedEmbeddings > 0 {
			message += fmt.Sprintf(" Fixed %d embedding gaps.", fixedEmbeddings)
		}
	}

	summary := map[string]any{
		"stale_decisions":      staleCount,
		"uncompiled_learnings": uncompiledCount,
		"embedding_gaps":       gapCount,
		"contradictions":       contradictionCount,
		"total_issues":         totalIssues,
	}

	// Add knowledge store metrics to summary when stores are configured (FR-025).
	if knowledgeQualityCount > 0 || len(knowledgeStoreSummaries) > 0 {
		summary["knowledge_quality_issues"] = knowledgeQualityCount
	}
	if staleKnowledgeCount > 0 {
		summary["stale_knowledge_stores"] = staleKnowledgeCount
	}
	if len(knowledgeStoreSummaries) > 0 {
		summary["knowledge_stores"] = knowledgeStoreSummaries
	}

	result := map[string]any{
		"status":   status,
		"summary":  summary,
		"findings": allFindings,
		"message":  message,
	}

	// Include fix results when fix was requested.
	if input.Fix {
		result["fixed"] = map[string]any{
			"embedding_gaps": fixedEmbeddings,
		}
	}

	lintLogger.Info("lint complete",
		"stale", staleCount,
		"uncompiled", uncompiledCount,
		"gaps", gapCount,
		"contradictions", contradictionCount,
		"knowledge_quality", knowledgeQualityCount,
		"stale_knowledge", staleKnowledgeCount,
		"fixed", fixedEmbeddings,
	)

	res, err := jsonTextResult(result)
	return res, nil, err
}

// checkStaleDecisions finds decision learnings older than the staleness
// threshold (30 days) that have not been validated.
func (l *Lint) checkStaleDecisions() ([]Finding, error) {
	pages, err := l.store.ListLearningPages()
	if err != nil {
		return nil, fmt.Errorf("list learning pages: %w", err)
	}

	now := time.Now()
	var findings []Finding

	for _, p := range pages {
		// Only check decision-category learnings.
		if p.Category != "decision" {
			continue
		}
		// Skip validated pages — they've been reviewed.
		if p.Tier == "validated" {
			continue
		}

		// Parse created_at from page properties (ISO 8601).
		createdAt := parseCreatedAtFromProperties(p)
		if createdAt.IsZero() {
			// Fall back to the page's CreatedAt timestamp (Unix ms).
			createdAt = time.UnixMilli(p.CreatedAt)
		}

		age := now.Sub(createdAt)
		if age > staleDecisionThreshold {
			identity := strings.TrimPrefix(p.Name, "learning/")
			days := int(age.Hours() / 24)
			findings = append(findings, Finding{
				Type:        "stale_decision",
				Severity:    "warning",
				Identity:    identity,
				Description: fmt.Sprintf("Decision learning '%s' is %d days old and not validated.", identity, days),
				Remediation: fmt.Sprintf("Review and either validate with `dewey promote %s` or store an updated decision.", p.Name),
			})
		}
	}

	return findings, nil
}

// checkUncompiledLearnings finds learnings not referenced by any
// compiled article's sources list.
func (l *Lint) checkUncompiledLearnings() ([]Finding, error) {
	learningPages, err := l.store.ListLearningPages()
	if err != nil {
		return nil, fmt.Errorf("list learning pages: %w", err)
	}

	// Get all compiled articles to check their sources.
	compiledPages, err := l.store.ListPagesBySource("compiled")
	if err != nil {
		return nil, fmt.Errorf("list compiled pages: %w", err)
	}

	// Build a set of all learning identities referenced by compiled articles.
	compiledSources := make(map[string]bool)
	for _, cp := range compiledPages {
		sources := extractSourcesFromProperties(cp)
		for _, src := range sources {
			compiledSources[src] = true
		}
	}

	var findings []Finding
	for _, p := range learningPages {
		identity := strings.TrimPrefix(p.Name, "learning/")
		if !compiledSources[identity] {
			findings = append(findings, Finding{
				Type:        "uncompiled",
				Severity:    "info",
				Identity:    identity,
				Description: fmt.Sprintf("Learning '%s' has not been compiled into any article.", identity),
				Remediation: "Run `dewey compile` to compile all learnings.",
			})
		}
	}

	return findings, nil
}

// checkEmbeddingGaps finds pages with blocks but no embeddings.
func (l *Lint) checkEmbeddingGaps() ([]Finding, error) {
	pages, err := l.store.PagesWithoutEmbeddings()
	if err != nil {
		return nil, fmt.Errorf("pages without embeddings: %w", err)
	}

	var findings []Finding
	for _, p := range pages {
		// Count blocks for the description.
		blocks, _ := l.store.GetBlocksByPage(p.Name)
		blockCount := len(blocks)

		findings = append(findings, Finding{
			Type:        "embedding_gap",
			Severity:    "warning",
			Page:        p.Name,
			Description: fmt.Sprintf("Page '%s' has %d blocks but no embeddings.", p.Name, blockCount),
			Remediation: "Run `dewey lint --fix` to regenerate embeddings, or `dewey index` to re-index.",
		})
	}

	return findings, nil
}

// checkContradictions finds learning pairs with high semantic similarity
// within the same tag namespace, suggesting potential contradictions.
//
// Design decision: This check requires an embedder to be available.
// When the embedder is nil or unavailable, the check is skipped entirely
// (invariant 6). The heuristic reports tags with 2+ decision-type learnings
// as potentially contradicting — run `dewey compile` to resolve via
// temporal merge.
func (l *Lint) checkContradictions() ([]Finding, error) {
	// Skip if embedder is unavailable (invariant 6).
	if l.embedder == nil || !l.embedder.Available() {
		return nil, nil
	}

	pages, err := l.store.ListLearningPages()
	if err != nil {
		return nil, fmt.Errorf("list learning pages: %w", err)
	}

	// Group decision learnings by tag for pairwise comparison.
	tagGroups := make(map[string][]*store.Page)
	for _, p := range pages {
		if p.Category != "decision" {
			continue
		}
		tag := extractTagFromProperties(p)
		if tag == "" {
			continue
		}
		tagGroups[tag] = append(tagGroups[tag], p)
	}

	var findings []Finding
	for tag, group := range tagGroups {
		if len(group) < 2 {
			continue
		}

		// Report tags with 2+ decision-type learnings as potentially contradicting.
		var identities []string
		for _, p := range group {
			identities = append(identities, strings.TrimPrefix(p.Name, "learning/"))
		}

		findings = append(findings, Finding{
			Type:        "contradiction",
			Severity:    "warning",
			Identities:  identities,
			Description: fmt.Sprintf("Tag '%s' has %d decision learnings that may contain contradicting information.", tag, len(group)),
			Remediation: "Run `dewey compile` to resolve contradictions via temporal merge, or review manually.",
		})
	}

	return findings, nil
}

// fixEmbeddingGaps regenerates embeddings for pages that have blocks
// but no embeddings. Requires an available embedder.
func (l *Lint) fixEmbeddingGaps(ctx context.Context, gaps []Finding) (int, error) {
	if l.embedder == nil || !l.embedder.Available() {
		return 0, fmt.Errorf("embedder unavailable — cannot fix embedding gaps")
	}

	fixed := 0
	for _, gap := range gaps {
		if gap.Type != "embedding_gap" {
			continue
		}

		pageName := gap.Page
		blocks, err := l.store.GetBlocksByPage(pageName)
		if err != nil {
			lintLogger.Warn("failed to get blocks for embedding fix", "page", pageName, "err", err)
			continue
		}

		// Convert store.Block to types.BlockEntity for GenerateEmbeddings.
		var blockEntities []types.BlockEntity
		for _, b := range blocks {
			blockEntities = append(blockEntities, types.BlockEntity{
				UUID:    b.UUID,
				Content: b.Content,
			})
		}

		count := vault.GenerateEmbeddings(l.store, l.embedder, pageName, blockEntities, nil)
		if count > 0 {
			fixed++
			lintLogger.Info("regenerated embeddings", "page", pageName, "embeddings", count)
		}
	}

	return fixed, nil
}

// parseCreatedAtFromProperties extracts the created_at timestamp from page
// properties JSON. Returns zero time if the property is missing or unparseable.
func parseCreatedAtFromProperties(p *store.Page) time.Time {
	if p.Properties == "" {
		return time.Time{}
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return time.Time{}
	}

	createdAtStr, ok := props["created_at"].(string)
	if !ok || createdAtStr == "" {
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return time.Time{}
	}
	return t
}

// extractTagFromProperties extracts the tag from page properties JSON.
func extractTagFromProperties(p *store.Page) string {
	if p.Properties == "" {
		return ""
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return ""
	}

	tag, _ := props["tag"].(string)
	return tag
}

// knowledgeStoreSummary holds per-store metrics for the lint report.
type knowledgeStoreSummary struct {
	Name             string         `json:"name"`
	FileCount        int            `json:"file_count"`
	ConfidenceCounts map[string]int `json:"confidence_counts"`
	QualityFlagTypes map[string]int `json:"quality_flag_types"`
}

// knowledgeFrontmatter holds the parsed YAML frontmatter from a curated
// knowledge file. Used by lint to extract confidence and quality flags.
type knowledgeFrontmatter struct {
	Tag          string `yaml:"tag"`
	Category     string `yaml:"category"`
	Confidence   string `yaml:"confidence"`
	Tier         string `yaml:"tier"`
	QualityFlags []struct {
		Type string `yaml:"type"`
	} `yaml:"quality_flags"`
}

// checkKnowledgeQuality scans knowledge store directories for curated files,
// parses frontmatter to count confidence levels and quality flag types, and
// reports findings for low-confidence or flagged content (FR-025).
//
// Returns findings, per-store summaries, and any error. When no knowledge
// stores are configured or vaultPath is empty, returns (nil, nil, nil).
func (l *Lint) checkKnowledgeQuality() ([]Finding, []knowledgeStoreSummary, error) {
	if l.vaultPath == "" {
		return nil, nil, nil
	}

	deweyDir := filepath.Join(l.vaultPath, deweyWorkspaceDir)
	ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	stores, err := curate.LoadKnowledgeStoresConfig(ksPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load knowledge stores config: %w", err)
	}
	if len(stores) == 0 {
		return nil, nil, nil
	}

	var findings []Finding
	var summaries []knowledgeStoreSummary

	for _, cfg := range stores {
		storePath := curate.ResolveStorePath(cfg, l.vaultPath)

		entries, err := os.ReadDir(storePath)
		if err != nil {
			if os.IsNotExist(err) {
				// Store directory doesn't exist yet — no curated files.
				continue
			}
			lintLogger.Warn("failed to read knowledge store directory",
				"store", cfg.Name, "path", storePath, "err", err)
			continue
		}

		summary := knowledgeStoreSummary{
			Name:             cfg.Name,
			ConfidenceCounts: make(map[string]int),
			QualityFlagTypes: make(map[string]int),
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			// Skip index and state files.
			if entry.Name() == "_index.md" {
				continue
			}

			filePath := filepath.Join(storePath, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				lintLogger.Warn("failed to read knowledge file",
					"path", filePath, "err", err)
				continue
			}

			fm, err := parseKnowledgeFrontmatter(string(content))
			if err != nil {
				continue
			}

			summary.FileCount++

			// Count confidence levels.
			if fm.Confidence != "" {
				summary.ConfidenceCounts[fm.Confidence]++
			}

			// Count quality flag types.
			for _, flag := range fm.QualityFlags {
				if flag.Type != "" {
					summary.QualityFlagTypes[flag.Type]++
				}
			}

			// Report findings for low-confidence or flagged content.
			if fm.Confidence == "low" || fm.Confidence == "flagged" {
				findings = append(findings, Finding{
					Type:     "knowledge_quality",
					Severity: severityForConfidence(fm.Confidence),
					Page:     entry.Name(),
					Description: fmt.Sprintf("Knowledge file '%s' in store '%s' has %s confidence.",
						entry.Name(), cfg.Name, fm.Confidence),
					Remediation: "Review the source material and update or validate the knowledge file.",
				})
			}

			// Report findings for quality flags.
			for _, flag := range fm.QualityFlags {
				if flag.Type != "" {
					findings = append(findings, Finding{
						Type:     "knowledge_quality",
						Severity: "info",
						Page:     entry.Name(),
						Description: fmt.Sprintf("Knowledge file '%s' in store '%s' has quality flag: %s.",
							entry.Name(), cfg.Name, flag.Type),
						Remediation: "Review the flagged content and resolve the quality issue.",
					})
				}
			}
		}

		if summary.FileCount > 0 {
			summaries = append(summaries, summary)
		}
	}

	return findings, summaries, nil
}

// checkStaleKnowledgeStores detects knowledge stores whose mapped sources
// have been updated since the last curation checkpoint (FR-026).
//
// A store is "stale" when any of its configured sources has pages with
// updated_at timestamps newer than the store's last curation checkpoint.
func (l *Lint) checkStaleKnowledgeStores() ([]Finding, error) {
	if l.vaultPath == "" {
		return nil, nil
	}

	deweyDir := filepath.Join(l.vaultPath, deweyWorkspaceDir)
	ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	stores, err := curate.LoadKnowledgeStoresConfig(ksPath)
	if err != nil {
		return nil, fmt.Errorf("load knowledge stores config: %w", err)
	}
	if len(stores) == 0 {
		return nil, nil
	}

	var findings []Finding

	for _, cfg := range stores {
		if len(cfg.Sources) == 0 {
			continue
		}

		storePath := curate.ResolveStorePath(cfg, l.vaultPath)

		// Load the curation checkpoint.
		state, err := curate.LoadCurationState(storePath)
		if err != nil {
			lintLogger.Warn("failed to load curation state",
				"store", cfg.Name, "err", err)
			continue
		}

		// If no checkpoint exists (zero LastCuratedAt), the store has never
		// been curated. Check if there are any source documents to curate.
		if state.LastCuratedAt.IsZero() {
			// Check if any sources have content.
			hasContent := false
			for _, srcID := range cfg.Sources {
				count, err := l.store.CountPagesBySource(strings.TrimSpace(srcID))
				if err != nil {
					continue
				}
				if count > 0 {
					hasContent = true
					break
				}
			}
			if hasContent {
				findings = append(findings, Finding{
					Type:     "stale_knowledge",
					Severity: "warning",
					Description: fmt.Sprintf("Knowledge store '%s' has never been curated but has source content available.",
						cfg.Name),
					Remediation: fmt.Sprintf("Run `dewey curate --store %s` to curate knowledge from sources.", cfg.Name),
				})
			}
			continue
		}

		// Check each source for updates since the checkpoint.
		checkpointMs := state.LastCuratedAt.UnixMilli()
		for _, srcID := range cfg.Sources {
			srcID = strings.TrimSpace(srcID)

			latestUpdated, err := l.store.LatestUpdatedAtBySource(srcID)
			if err != nil {
				lintLogger.Warn("failed to check source freshness",
					"store", cfg.Name, "source", srcID, "err", err)
				continue
			}

			if latestUpdated > checkpointMs {
				findings = append(findings, Finding{
					Type:     "stale_knowledge",
					Severity: "warning",
					Description: fmt.Sprintf("Knowledge store '%s' is stale — source '%s' has been updated since last curation.",
						cfg.Name, srcID),
					Remediation: fmt.Sprintf("Run `dewey curate --store %s` to process new content.", cfg.Name),
				})
				// One stale finding per store is enough — break after first.
				break
			}
		}
	}

	return findings, nil
}

// parseKnowledgeFrontmatter extracts YAML frontmatter from a curated
// knowledge file. Returns the parsed frontmatter or an error if the
// file doesn't have valid frontmatter.
func parseKnowledgeFrontmatter(content string) (knowledgeFrontmatter, error) {
	var fm knowledgeFrontmatter

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return fm, fmt.Errorf("no YAML frontmatter found")
	}

	if err := json.Unmarshal([]byte(parts[1]), &fm); err != nil {
		// JSON unmarshal won't work for YAML — use a simpler approach.
		// Parse key-value pairs manually from the frontmatter.
		fm = parseKnowledgeFrontmatterManual(parts[1])
	}

	return fm, nil
}

// parseKnowledgeFrontmatterManual parses YAML frontmatter by scanning
// for known keys. This avoids importing gopkg.in/yaml.v3 in the tools
// package (which would add a dependency the tools package doesn't currently
// have — the curate package handles YAML parsing).
//
// Design decision: Manual parsing over YAML library import to maintain
// the tools package's minimal dependency footprint. The frontmatter
// format is well-defined and simple enough for line-by-line parsing.
func parseKnowledgeFrontmatterManual(yamlContent string) knowledgeFrontmatter {
	var fm knowledgeFrontmatter

	lines := strings.Split(yamlContent, "\n")
	inQualityFlags := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect quality_flags section.
		if strings.HasPrefix(trimmed, "quality_flags:") {
			rest := strings.TrimPrefix(trimmed, "quality_flags:")
			rest = strings.TrimSpace(rest)
			if rest == "[]" {
				// Empty quality flags.
				inQualityFlags = false
				continue
			}
			inQualityFlags = true
			continue
		}

		// Parse quality flag entries.
		if inQualityFlags {
			if strings.HasPrefix(trimmed, "- type:") {
				flagType := strings.TrimSpace(strings.TrimPrefix(trimmed, "- type:"))
				fm.QualityFlags = append(fm.QualityFlags, struct {
					Type string `yaml:"type"`
				}{Type: flagType})
				continue
			}
			// Indented lines within a flag entry (detail, sources, etc.) — skip.
			if strings.HasPrefix(trimmed, "detail:") || strings.HasPrefix(trimmed, "sources:") ||
				strings.HasPrefix(trimmed, "resolution:") || strings.HasPrefix(trimmed, "- ") {
				continue
			}
			// Non-indented line ends the quality_flags section.
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
				inQualityFlags = false
			}
		}

		// Parse top-level key-value pairs.
		if !inQualityFlags {
			if strings.HasPrefix(trimmed, "tag:") {
				fm.Tag = strings.TrimSpace(strings.TrimPrefix(trimmed, "tag:"))
			} else if strings.HasPrefix(trimmed, "category:") {
				fm.Category = strings.TrimSpace(strings.TrimPrefix(trimmed, "category:"))
			} else if strings.HasPrefix(trimmed, "confidence:") {
				fm.Confidence = strings.TrimSpace(strings.TrimPrefix(trimmed, "confidence:"))
			} else if strings.HasPrefix(trimmed, "tier:") {
				fm.Tier = strings.TrimSpace(strings.TrimPrefix(trimmed, "tier:"))
			}
		}
	}

	return fm
}

// severityForConfidence returns the lint severity for a given confidence level.
func severityForConfidence(confidence string) string {
	switch confidence {
	case "flagged":
		return "warning"
	case "low":
		return "info"
	default:
		return "info"
	}
}

// extractSourcesFromProperties extracts the sources list from compiled
// article properties JSON.
func extractSourcesFromProperties(p *store.Page) []string {
	if p.Properties == "" {
		return nil
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return nil
	}

	sourcesRaw, ok := props["sources"]
	if !ok {
		return nil
	}

	// Sources may be stored as []any (from JSON unmarshal).
	sourcesSlice, ok := sourcesRaw.([]any)
	if !ok {
		return nil
	}

	var sources []string
	for _, s := range sourcesSlice {
		if str, ok := s.(string); ok {
			sources = append(sources, str)
		}
	}
	return sources
}
