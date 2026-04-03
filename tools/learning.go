package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

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

// StoreLearning handles the dewey_store_learning MCP tool. Persists a
// learning (insight, pattern, gotcha) into the knowledge graph with optional
// tags. The learning is parsed into blocks, stored in SQLite, and optionally
// embedded for semantic search.
//
// Returns a JSON result with the learning's UUID, page name, and a status
// message. Returns an MCP error result (not a Go error) if the input is
// invalid or the store is unavailable.
func (l *Learning) StoreLearning(ctx context.Context, req *mcp.CallToolRequest, input types.StoreLearningInput) (*mcp.CallToolResult, any, error) {
	if input.Information == "" {
		return errorResult("information parameter is required and must not be empty"), nil, nil
	}
	if l.store == nil {
		return errorResult("store_learning requires persistent storage. Configure --vault with a .dewey/ directory."), nil, nil
	}

	// Generate unique page name and document ID using Unix millisecond timestamp.
	// The learning/ namespace keeps learnings visually distinct from vault pages.
	timestamp := time.Now().UnixMilli()
	pageName := fmt.Sprintf("learning/%d", timestamp)
	docID := fmt.Sprintf("learning-%d", timestamp)

	// Build properties JSON with tags if provided (FR-004).
	properties := "{}"
	if input.Tags != "" {
		propsMap := map[string]string{"tags": input.Tags}
		propsJSON, err := json.Marshal(propsMap)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to marshal properties: %v", err)), nil, nil
		}
		properties = string(propsJSON)
	}

	// Compute a short content hash for deduplication support.
	hash := sha256.Sum256([]byte(input.Information))
	contentHash := fmt.Sprintf("%x", hash[:8])

	// Insert the page with source_id "learning" to distinguish from other
	// content sources (FR-003). This ensures learnings are never deleted by
	// dewey reindex (which only purges configured sources).
	page := &store.Page{
		Name:         pageName,
		OriginalName: pageName,
		SourceID:     "learning",
		SourceDocID:  docID,
		Properties:   properties,
		ContentHash:  contentHash,
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
		"uuid":    learningUUID,
		"page":    pageName,
		"message": "Learning stored successfully." + embeddingMsg,
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}
