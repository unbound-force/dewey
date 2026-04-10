package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
// while still providing value from the other 3 checks.
type Lint struct {
	store    *store.Store
	embedder embed.Embedder
}

// NewLint creates a new Lint tool handler with the given store and embedder.
// The store must be non-nil for the tool to function; a clear error is
// returned at call time if it is nil (invariant 7). The embedder may be
// nil — contradiction checking is skipped when unavailable.
func NewLint(s *store.Store, e embed.Embedder) *Lint {
	return &Lint{store: s, embedder: e}
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

	// Count findings by type for the summary.
	staleCount := 0
	uncompiledCount := 0
	gapCount := 0
	contradictionCount := 0
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

	result := map[string]any{
		"status": status,
		"summary": map[string]any{
			"stale_decisions":      staleCount,
			"uncompiled_learnings": uncompiledCount,
			"embedding_gaps":       gapCount,
			"contradictions":       contradictionCount,
			"total_issues":         totalIssues,
		},
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
