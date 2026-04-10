package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// validCategories defines the allowed values for the category field.
// Learnings without a category are treated as "context" during compilation.
var validCategories = map[string]bool{
	"decision":  true,
	"pattern":   true,
	"gotcha":    true,
	"context":   true,
	"reference": true,
}

// tagNormalizer strips characters that are not alphanumeric or hyphens.
var tagNormalizer = regexp.MustCompile(`[^a-z0-9-]`)

// Learning implements the dewey_store_learning MCP tool for persisting
// agent learnings (insights, patterns, gotchas) into the knowledge graph.
//
// Design decision: The embedder and store are injected as dependencies
// (Dependency Inversion Principle) following the same pattern as Semantic.
// This enables testing with mocks and supports graceful degradation when
// Ollama is unavailable — learnings are stored without embeddings and
// remain searchable via keyword search.
type Learning struct {
	embedder embed.Embedder
	store    *store.Store
}

// NewLearning creates a new Learning tool handler with the given embedder
// and store. The embedder may be nil — the tool stores learnings without
// embeddings when unavailable (graceful degradation). The store must be
// non-nil for the tool to function; a clear error is returned at call time
// if it is nil.
func NewLearning(e embed.Embedder, s *store.Store) *Learning {
	return &Learning{embedder: e, store: s}
}

// normalizeTag lowercases, trims whitespace, replaces spaces with hyphens,
// and strips non-alphanumeric characters (except hyphens) from a tag string.
// Example: "My Tag Name" → "my-tag-name".
func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.ToLower(tag)
	tag = strings.ReplaceAll(tag, " ", "-")
	tag = tagNormalizer.ReplaceAllString(tag, "")
	return tag
}

// resolveTag determines the effective tag from the input, applying the
// priority: tag > tags (first value) > "general". Returns the normalized tag.
func resolveTag(input types.StoreLearningInput) string {
	if input.Tag != "" {
		return normalizeTag(input.Tag)
	}
	// Backward compatibility: extract first tag from comma-separated Tags field.
	if input.Tags != "" {
		parts := strings.SplitN(input.Tags, ",", 2)
		first := strings.TrimSpace(parts[0])
		if first != "" {
			return normalizeTag(first)
		}
	}
	return "general"
}

// StoreLearning handles the dewey_store_learning MCP tool. Persists a
// learning into the knowledge graph with a required topic tag and optional
// category. The learning receives a {tag}-{sequence} identity (e.g.,
// "authentication-3") and is stored with tier "draft".
//
// Returns a JSON result with the learning's identity, page name, and
// status message. Returns an MCP error result (not a Go error) if the
// input is invalid or the store is unavailable.
//
// BREAKING CHANGE from spec 008: The `tags` parameter (plural, optional,
// comma-separated) is replaced by `tag` (singular, required). For backward
// compatibility, if `tags` is provided but `tag` is not, the first tag
// from the comma-separated list is used. If neither is provided, defaults
// to "general".
func (l *Learning) StoreLearning(ctx context.Context, req *mcp.CallToolRequest, input types.StoreLearningInput) (*mcp.CallToolResult, any, error) {
	if input.Information == "" {
		return errorResult("information parameter is required and must not be empty"), nil, nil
	}
	if l.store == nil {
		return errorResult("store_learning requires persistent storage. Configure --vault with a .uf/dewey/ directory."), nil, nil
	}

	// Validate category if provided — must be one of the allowed values.
	// Empty category is allowed (treated as "context" during compilation).
	if input.Category != "" && !validCategories[input.Category] {
		return errorResult(fmt.Sprintf(
			"invalid category %q. Valid categories: decision, pattern, gotcha, context, reference",
			input.Category,
		)), nil, nil
	}

	// Resolve the effective tag using priority: tag > tags > "general".
	tag := resolveTag(input)

	// Determine the next sequence number for this tag namespace.
	seq, err := l.store.NextLearningSequence(tag)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to determine learning sequence: %v", err)), nil, nil
	}

	// Build the {tag}-{sequence} identity and page name.
	identity := fmt.Sprintf("%s-%d", tag, seq)
	pageName := fmt.Sprintf("learning/%s", identity)
	docID := fmt.Sprintf("learning-%s", identity)

	// Build properties JSON with tag, category, and created_at (FR-004, FR-005).
	now := time.Now()
	propsMap := map[string]string{
		"tag":        tag,
		"created_at": now.UTC().Format(time.RFC3339),
	}
	if input.Category != "" {
		propsMap["category"] = input.Category
	}
	// Preserve backward-compatible tags field if provided.
	if input.Tags != "" {
		propsMap["tags"] = input.Tags
	}
	propsJSON, err := json.Marshal(propsMap)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to marshal properties: %v", err)), nil, nil
	}
	properties := string(propsJSON)

	// Compute a short content hash for deduplication support.
	hash := sha256.Sum256([]byte(input.Information))
	contentHash := fmt.Sprintf("%x", hash[:8])

	// Insert the page with source_id "learning" to distinguish from other
	// content sources (FR-003). This ensures learnings are never deleted by
	// dewey reindex (which only purges configured sources).
	// Tier is always "draft" for learnings. Category is set from input.
	page := &store.Page{
		Name:         pageName,
		OriginalName: pageName,
		SourceID:     "learning",
		SourceDocID:  docID,
		Properties:   properties,
		ContentHash:  contentHash,
		Tier:         "draft",
		Category:     input.Category,
	}
	if err := l.store.InsertPage(page); err != nil {
		return errorResult(fmt.Sprintf("failed to store learning: %v", err)), nil, nil
	}

	// Parse the learning text into blocks using the shared parsing pipeline.
	_, blocks := vault.ParseDocument(docID, input.Information)

	// Persist blocks to the store.
	if err := vault.PersistBlocks(l.store, pageName, blocks, sql.NullString{}, 0); err != nil {
		return errorResult(fmt.Sprintf("failed to persist learning blocks: %v", err)), nil, nil
	}

	// Generate embeddings if the embedder is available (FR-005, FR-009).
	// Graceful degradation: learnings are stored without embeddings when
	// Ollama is unavailable, remaining searchable via keyword search.
	var embeddingMsg string
	if l.embedder != nil && l.embedder.Available() {
		vault.GenerateEmbeddings(l.store, l.embedder, pageName, blocks, nil)
	} else {
		embeddingMsg = " Note: Embeddings were not generated (Ollama unavailable). The learning is stored and searchable via keyword search. Semantic search will be available after embeddings are generated."
	}

	// Return the first block's UUID as the learning identifier (FR-006).
	learningUUID := ""
	if len(blocks) > 0 {
		learningUUID = blocks[0].UUID
	}

	result := map[string]any{
		"uuid":       learningUUID,
		"identity":   identity,
		"page":       pageName,
		"tag":        tag,
		"category":   input.Category,
		"created_at": now.UTC().Format(time.RFC3339),
		"message":    "Learning stored successfully." + embeddingMsg,
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}
