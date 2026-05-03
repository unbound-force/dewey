package tools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// learningLogger is the package-level structured logger for learning tool operations.
var learningLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/tools/learning",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

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
// Each learning receives a {tag}-{YYYYMMDDTHHMMSS}-{author} identity
// (e.g., "authentication-20260502T143022-alice") where the author is
// resolved via a three-tier fallback: DEWEY_AUTHOR env var, git config
// user.name, or "anonymous".
//
// Design decision: The embedder and store are injected as dependencies
// (Dependency Inversion Principle) following the same pattern as Semantic.
// This enables testing with mocks and supports graceful degradation when
// Ollama is unavailable — learnings are stored without embeddings and
// remain searchable via keyword search.
//
// The vaultPath field is the vault root directory (not the .uf/dewey/
// workspace). Markdown files are written to {vaultPath}/.uf/dewey/learnings/.
type Learning struct {
	embedder  embed.Embedder
	store     *store.Store
	vaultPath string
}

// NewLearning creates a new Learning tool handler with the given embedder,
// store, and vault root path. The embedder may be nil — the tool stores
// learnings without embeddings when unavailable (graceful degradation).
// The store must be non-nil for the tool to function; a clear error is
// returned at call time if it is nil. The vaultPath is the vault root
// directory; markdown files are written to {vaultPath}/.uf/dewey/learnings/.
func NewLearning(e embed.Embedder, s *store.Store, vaultPath string) *Learning {
	return &Learning{embedder: e, store: s, vaultPath: vaultPath}
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
	//nolint:staticcheck // SA1019: intentionally reading deprecated field for backward compat
	if input.Tags != "" {
		parts := strings.SplitN(input.Tags, ",", 2) //nolint:staticcheck
		first := strings.TrimSpace(parts[0])
		if first != "" {
			return normalizeTag(first)
		}
	}
	return "general"
}

// maxAuthorLen is the maximum length for a normalized author string.
const maxAuthorLen = 64

// resolveAuthor determines the author identity using a three-tier fallback:
// 1. DEWEY_AUTHOR environment variable (if set and non-empty after trimming)
// 2. gitResolver function (e.g., `git config user.name`)
// 3. "anonymous" as the final fallback
//
// The result is normalized using normalizeTag (lowercase, strip special chars,
// replace spaces with hyphens), then leading/trailing hyphens are trimmed.
// If normalization produces an empty string (e.g., CJK-only names that are
// stripped entirely), the function falls back to "anonymous". The result is
// truncated to 64 characters.
//
// Design decision: The gitResolver is injected as a function parameter
// (Dependency Inversion Principle) so tests can provide mock resolvers
// without depending on the test runner's actual git configuration.
func resolveAuthor(gitResolver func() (string, error)) string {
	// Tier 1: DEWEY_AUTHOR environment variable.
	if envAuthor := strings.TrimSpace(os.Getenv("DEWEY_AUTHOR")); envAuthor != "" {
		normalized := strings.Trim(normalizeTag(envAuthor), "-")
		if normalized != "" {
			if len(normalized) > maxAuthorLen {
				normalized = normalized[:maxAuthorLen]
			}
			return normalized
		}
	}

	// Tier 2: Git resolver (e.g., git config user.name).
	if gitResolver != nil {
		if gitName, err := gitResolver(); err == nil {
			trimmed := strings.TrimSpace(gitName)
			if trimmed != "" {
				normalized := strings.Trim(normalizeTag(trimmed), "-")
				if normalized != "" {
					if len(normalized) > maxAuthorLen {
						normalized = normalized[:maxAuthorLen]
					}
					return normalized
				}
			}
		}
	}

	// Tier 3: Anonymous fallback.
	return "anonymous"
}

// productionGitResolver returns a git resolver function that uses
// exec.CommandContext with a 2-second timeout to run `git config user.name`.
// The output is trimmed of whitespace and newlines.
func productionGitResolver() func() (string, error) {
	return func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "git", "config", "user.name").Output()
		if err != nil {
			return "", fmt.Errorf("git config user.name: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// StoreLearning handles the dewey_store_learning MCP tool. Persists a
// learning into the knowledge graph with a required topic tag and optional
// category. The learning receives a {tag}-{YYYYMMDDTHHMMSS}-{author}
// identity (e.g., "authentication-20260502T143022-alice") and is stored
// with tier "draft". The author is resolved via [resolveAuthor] using a
// three-tier fallback: DEWEY_AUTHOR env var, git config user.name, or
// "anonymous".
//
// Returns a JSON result with the learning's identity, page name, author,
// and status message. Returns an MCP error result (not a Go error) if the
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

	// Resolve the author identity using three-tier fallback:
	// DEWEY_AUTHOR env var -> git config user.name -> "anonymous".
	author := resolveAuthor(productionGitResolver())

	// Build the {tag}-{timestamp}-{author} identity and page name.
	// Uses UTC time to ensure consistent ordering across time zones.
	now := time.Now()
	timestamp := now.UTC().Format("20060102T150405")
	identity := fmt.Sprintf("%s-%s-%s", tag, timestamp, author)
	pageName := fmt.Sprintf("learning/%s", identity)
	docID := fmt.Sprintf("learning-%s", identity)

	// Dual-write: attempt file creation first to resolve any sub-second
	// collisions before committing to the SQLite identity. The file write
	// uses O_CREATE|O_EXCL for atomic creation. If a file with the same
	// identity already exists, a suffix (-2, -3, ..., -99) is appended.
	// The identity, pageName, and docID are updated to match the final filename.
	var filePath string
	if l.vaultPath != "" {
		var finalIdentity string
		var fileErr error
		filePath, finalIdentity, fileErr = l.writeLearningFile(tag, author, input.Category, now, identity, input.Information)
		if fileErr != nil {
			// Collision avoidance exhausted (99 attempts) — return MCP error.
			return errorResult(fileErr.Error()), nil, nil
		}
		if finalIdentity != identity {
			// A collision suffix was added — update identity, pageName, and docID
			// to match the actual filename for consistency.
			identity = finalIdentity
			pageName = fmt.Sprintf("learning/%s", identity)
			docID = fmt.Sprintf("learning-%s", identity)
		}
	}

	// Build properties JSON with tag, category, author, and created_at (FR-004, FR-005).
	propsMap := map[string]string{
		"tag":        tag,
		"author":     author,
		"created_at": now.UTC().Format(time.RFC3339),
	}
	if input.Category != "" {
		propsMap["category"] = input.Category
	}
	// Preserve backward-compatible tags field if provided.
	if input.Tags != "" { //nolint:staticcheck // SA1019: intentionally reading deprecated field
		propsMap["tags"] = input.Tags //nolint:staticcheck
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
		"author":     author,
		"category":   input.Category,
		"created_at": now.UTC().Format(time.RFC3339),
		"file_path":  filePath,
		"message":    "Learning stored successfully." + embeddingMsg,
	}

	res, err := jsonTextResult(result)
	return res, nil, err
}

// maxCollisionAttempts is the maximum number of collision suffixes to try
// before giving up. Suffixes go from -2 to -99 (98 attempts + the original).
const maxCollisionAttempts = 99

// writeLearningFile writes a learning as a markdown file with YAML frontmatter
// to {vaultPath}/.uf/dewey/learnings/{identity}.md where identity is
// {tag}-{YYYYMMDDTHHMMSS}-{author}. Uses O_CREATE|O_EXCL for atomic file
// creation with sub-second collision avoidance: if the file already exists,
// appends -2, -3, ..., -99 to the identity until a unique filename is found.
// Returns the relative file path, the final identity (which may differ from
// the input if a collision suffix was added), and an error if collision
// avoidance is exhausted.
//
// The YAML frontmatter includes tag, author, category (if non-empty),
// created_at, identity, and tier fields.
//
// Design decision: Uses fmt.Sprintf for YAML frontmatter construction rather
// than yaml.Marshal to avoid importing gopkg.in/yaml.v3 for trivial key-value
// pairs. The frontmatter format is simple enough that string formatting is
// clearer and more predictable than marshaling.
func (l *Learning) writeLearningFile(tag, author, category string, createdAt time.Time, identity, information string) (string, string, error) {
	learningsDir := filepath.Join(l.vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		learningLogger.Warn("failed to create learnings directory", "path", learningsDir, "err", err)
		return "", identity, nil
	}

	// buildContent constructs the markdown file content with YAML frontmatter.
	buildContent := func(finalIdentity string) []byte {
		var buf strings.Builder
		buf.WriteString("---\n")
		buf.WriteString(fmt.Sprintf("tag: %s\n", tag))
		buf.WriteString(fmt.Sprintf("author: %s\n", author))
		if category != "" {
			buf.WriteString(fmt.Sprintf("category: %s\n", category))
		}
		buf.WriteString(fmt.Sprintf("created_at: %s\n", createdAt.UTC().Format(time.RFC3339)))
		buf.WriteString(fmt.Sprintf("identity: %s\n", finalIdentity))
		buf.WriteString("tier: draft\n")
		buf.WriteString("---\n\n")
		buf.WriteString(information)
		buf.WriteString("\n")
		return []byte(buf.String())
	}

	// Try the original identity first, then append collision suffixes.
	finalIdentity := identity
	for attempt := 1; attempt <= maxCollisionAttempts; attempt++ {
		filename := fmt.Sprintf("%s.md", finalIdentity)
		filePath := filepath.Join(learningsDir, filename)

		f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				// File exists — try next collision suffix.
				finalIdentity = fmt.Sprintf("%s-%d", identity, attempt+1)
				continue
			}
			// Non-collision error (e.g., permission denied) — best-effort, log and return.
			learningLogger.Warn("failed to create learning file", "path", filePath, "err", err)
			return "", identity, nil
		}

		// File created successfully — write content and close.
		content := buildContent(finalIdentity)
		if _, writeErr := f.Write(content); writeErr != nil {
			_ = f.Close()
			learningLogger.Warn("failed to write learning file content", "path", filePath, "err", writeErr)
			return "", finalIdentity, nil
		}
		if closeErr := f.Close(); closeErr != nil {
			learningLogger.Warn("failed to close learning file", "path", filePath, "err", closeErr)
		}

		relPath := filepath.Join(deweyWorkspaceDir, "learnings", filename)
		learningLogger.Debug("learning persisted to file", "path", relPath)
		return relPath, finalIdentity, nil
	}

	// All collision attempts exhausted.
	return "", identity, fmt.Errorf("failed to create learning file: %d collision attempts exhausted for identity %q", maxCollisionAttempts, identity)
}
