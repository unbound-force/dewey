package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// newTestStore creates an in-memory store for learning tests.
// Registers a cleanup function to close the store when the test completes.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// parseLearningResult unmarshals the JSON text from a CallToolResult into a map.
func parseLearningResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return parsed
}

// TestResolveAuthor verifies the three-tier author resolution fallback chain
// with at least 10 cases covering env var, git resolver, normalization, and
// edge cases. Uses t.Setenv for environment variable tests and injected mock
// git resolvers — never depends on the test runner's actual git configuration.
func TestResolveAuthor(t *testing.T) {
	tests := []struct {
		name        string
		envAuthor   string // DEWEY_AUTHOR env var value ("" means unset)
		setEnv      bool   // whether to set the env var at all
		gitResolver func() (string, error)
		want        string
	}{
		{
			name:      "a_env_var_set",
			envAuthor: "alice",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-user", nil
			},
			want: "alice",
		},
		{
			name:      "b_normalizes_spaces_and_special_chars",
			envAuthor: "Alice Bob!@#",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-user", nil
			},
			want: "alice-bob",
		},
		{
			name:      "c_git_resolver_error_returns_anonymous",
			envAuthor: "",
			setEnv:    false,
			gitResolver: func() (string, error) {
				return "", fmt.Errorf("git not installed")
			},
			want: "anonymous",
		},
		{
			name:      "d_git_resolver_empty_returns_anonymous",
			envAuthor: "",
			setEnv:    false,
			gitResolver: func() (string, error) {
				return "", nil
			},
			want: "anonymous",
		},
		{
			name:      "e_empty_dewey_author_falls_through",
			envAuthor: "",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-fallback", nil
			},
			want: "git-fallback",
		},
		{
			name:      "f_whitespace_only_dewey_author_falls_through",
			envAuthor: "   \t  ",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-fallback", nil
			},
			want: "git-fallback",
		},
		{
			name:      "g_cjk_all_special_normalizes_to_empty_anonymous",
			envAuthor: "日本語テスト",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "", fmt.Errorf("no git")
			},
			want: "anonymous",
		},
		{
			name:      "h_leading_trailing_hyphens_stripped",
			envAuthor: "--alice--",
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-user", nil
			},
			want: "alice",
		},
		{
			name:      "i_long_author_truncated_to_64",
			envAuthor: strings.Repeat("a", 100),
			setEnv:    true,
			gitResolver: func() (string, error) {
				return "git-user", nil
			},
			want: strings.Repeat("a", 64),
		},
		{
			name:      "j_git_resolver_newline_trimmed",
			envAuthor: "",
			setEnv:    false,
			gitResolver: func() (string, error) {
				return "git-user\n", nil
			},
			want: "git-user",
		},
		{
			name:        "k_nil_git_resolver_returns_anonymous",
			envAuthor:   "",
			setEnv:      false,
			gitResolver: nil,
			want:        "anonymous",
		},
		{
			name:      "l_git_resolver_whitespace_only_returns_anonymous",
			envAuthor: "",
			setEnv:    false,
			gitResolver: func() (string, error) {
				return "   ", nil
			},
			want: "anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("DEWEY_AUTHOR", tt.envAuthor)
			} else {
				// Ensure env var is unset for this test.
				t.Setenv("DEWEY_AUTHOR", "")
				// Unset it completely by setting to empty — resolveAuthor
				// treats empty as unset after TrimSpace.
			}

			got := resolveAuthor(tt.gitResolver)
			if got != tt.want {
				t.Errorf("resolveAuthor() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStoreLearning_Basic verifies the happy path: storing a learning with
// a valid information string and nil embedder returns a successful result
// containing a UUID, identity, and page name with the "learning/" prefix.
// Updated for learning-identity-collision-fix: identity format is now
// {tag}-{YYYYMMDDTHHMMSS}-{author}.
func TestStoreLearning_Basic(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "The vault walker must build its ignore matcher in New()",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Parse the JSON result to verify structure.
	parsed := parseLearningResult(t, resultText(result))

	// Assert UUID is present and non-empty.
	uuid, ok := parsed["uuid"].(string)
	if !ok || uuid == "" {
		t.Errorf("expected non-empty uuid in result, got %v", parsed["uuid"])
	}

	// Assert identity follows {tag}-{YYYYMMDDTHHMMSS}-{author} format with default tag "general".
	identity, ok := parsed["identity"].(string)
	if !ok {
		t.Fatalf("expected string identity in result, got %v", parsed["identity"])
	}
	generalPattern := regexp.MustCompile(`^general-\d{8}T\d{6}-testuser$`)
	if !generalPattern.MatchString(identity) {
		t.Errorf("identity %q does not match pattern general-{timestamp}-testuser", identity)
	}

	// Assert page name has "learning/" prefix and matches identity.
	page, ok := parsed["page"].(string)
	if !ok || page != "learning/"+identity {
		t.Errorf("expected page %q, got %q", "learning/"+identity, page)
	}

	// Assert tag defaults to "general".
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "general" {
		t.Errorf("expected tag %q, got %q", "general", tag)
	}

	// Assert author is present in the response.
	author, ok := parsed["author"].(string)
	if !ok || author != "testuser" {
		t.Errorf("expected author %q, got %v", "testuser", parsed["author"])
	}

	// Assert message indicates success.
	msg, ok := parsed["message"].(string)
	if !ok || !strings.Contains(msg, "stored successfully") {
		t.Errorf("expected success message, got %q", msg)
	}

	// Assert created_at is present and non-empty.
	createdAt, ok := parsed["created_at"].(string)
	if !ok || createdAt == "" {
		t.Errorf("expected non-empty created_at, got %v", parsed["created_at"])
	}
}

// TestStoreLearning_EmptyInformation verifies that an empty information
// string returns an error result mentioning "information".
func TestStoreLearning_EmptyInformation(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for empty information")
	}

	text := resultText(result)
	if !strings.Contains(text, "information") {
		t.Errorf("error message = %q, should mention 'information'", text)
	}
}

// TestStoreLearning_NilStore verifies that a nil store returns an error
// result mentioning persistent storage.
func TestStoreLearning_NilStore(t *testing.T) {
	l := NewLearning(nil, nil, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestStoreLearning_WithTag verifies that the tag parameter produces a
// {tag}-{YYYYMMDDTHHMMSS}-{author} identity and stores the tag in page properties.
func TestStoreLearning_WithTag(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "OAuth tokens should be rotated every 24 hours",
		Tag:         "authentication",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Assert identity starts with "authentication-" and matches timestamp-author pattern.
	identity, ok := parsed["identity"].(string)
	if !ok {
		t.Fatalf("expected string identity in result, got %v", parsed["identity"])
	}
	authPattern := regexp.MustCompile(`^authentication-\d{8}T\d{6}-testuser$`)
	if !authPattern.MatchString(identity) {
		t.Errorf("identity %q does not match pattern authentication-{timestamp}-testuser", identity)
	}

	// Assert page name matches.
	page, ok := parsed["page"].(string)
	if !ok || page != "learning/"+identity {
		t.Errorf("expected page %q, got %q", "learning/"+identity, page)
	}

	// Assert tag is returned.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "authentication" {
		t.Errorf("expected tag %q, got %q", "authentication", tag)
	}

	// Verify the page in the store has the correct properties.
	storedPage, err := s.GetPage("learning/" + identity)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["tag"] != "authentication" {
		t.Errorf("stored tag = %q, want %q", props["tag"], "authentication")
	}
	if props["created_at"] == "" {
		t.Error("expected non-empty created_at in properties")
	}
	if props["author"] != "testuser" {
		t.Errorf("stored author = %q, want %q", props["author"], "testuser")
	}

	// Verify tier is "draft".
	if storedPage.Tier != "draft" {
		t.Errorf("tier = %q, want %q", storedPage.Tier, "draft")
	}
}

// TestStoreLearning_EmptyTag verifies that when both tag and tags are empty,
// the default tag "general" is used (not an error).
func TestStoreLearning_EmptyTag(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "a learning without any tag",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with default tag, got error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Should default to "general" tag.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "general" {
		t.Errorf("expected default tag %q, got %q", "general", tag)
	}

	// Assert identity starts with "general-" and matches timestamp-author pattern.
	identity, ok := parsed["identity"].(string)
	if !ok {
		t.Fatalf("expected string identity in result, got %v", parsed["identity"])
	}
	generalPattern := regexp.MustCompile(`^general-\d{8}T\d{6}-testuser$`)
	if !generalPattern.MatchString(identity) {
		t.Errorf("identity %q does not match pattern general-{timestamp}-testuser", identity)
	}
}

// TestStoreLearning_BackwardCompat verifies that the deprecated Tags field
// (comma-separated) falls back to the first tag when Tag is empty.
func TestStoreLearning_BackwardCompat(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning with old tags field",
		Tags:        "gotcha, vault-walker, 006-unified-ignore",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Should use first tag from comma-separated list.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "gotcha" {
		t.Errorf("expected tag %q from backward-compat fallback, got %q", "gotcha", tag)
	}

	// Assert identity starts with "gotcha-" and matches timestamp-author pattern.
	identity, ok := parsed["identity"].(string)
	if !ok {
		t.Fatalf("expected string identity in result, got %v", parsed["identity"])
	}
	gotchaPattern := regexp.MustCompile(`^gotcha-\d{8}T\d{6}-testuser$`)
	if !gotchaPattern.MatchString(identity) {
		t.Errorf("identity %q does not match pattern gotcha-{timestamp}-testuser", identity)
	}

	// Verify the page in the store preserves the original tags in properties.
	pageName, _ := parsed["page"].(string)
	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["tags"] != "gotcha, vault-walker, 006-unified-ignore" {
		t.Errorf("tags = %q, want %q", props["tags"], "gotcha, vault-walker, 006-unified-ignore")
	}
}

// TestStoreLearning_TagPriorityOverTags verifies that when both Tag and Tags
// are provided, Tag takes priority.
func TestStoreLearning_TagPriorityOverTags(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning with both fields",
		Tag:         "deployment",
		Tags:        "gotcha, vault-walker",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Tag field should take priority over Tags.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "deployment" {
		t.Errorf("expected tag %q (Tag takes priority), got %q", "deployment", tag)
	}
}

// TestStoreLearning_WithCategory verifies that a valid category is stored
// correctly on the page and returned in the response.
func TestStoreLearning_WithCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Always rotate OAuth tokens after 24 hours",
		Tag:         "authentication",
		Category:    "decision",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Assert category is returned in the response.
	category, ok := parsed["category"].(string)
	if !ok || category != "decision" {
		t.Errorf("expected category %q, got %q", "decision", category)
	}

	// Verify the page in the store has the correct category.
	pageName, _ := parsed["page"].(string)
	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}
	if storedPage.Category != "decision" {
		t.Errorf("stored category = %q, want %q", storedPage.Category, "decision")
	}

	// Verify category is also in properties JSON.
	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["category"] != "decision" {
		t.Errorf("properties category = %q, want %q", props["category"], "decision")
	}
}

// TestStoreLearning_InvalidCategory verifies that an invalid category
// returns an MCP error result with a descriptive message.
func TestStoreLearning_InvalidCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning",
		Tag:         "test",
		Category:    "invalid-category",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid category")
	}

	text := resultText(result)
	if !strings.Contains(text, "invalid category") {
		t.Errorf("error message = %q, should mention 'invalid category'", text)
	}
	if !strings.Contains(text, "invalid-category") {
		t.Errorf("error message = %q, should include the invalid value", text)
	}
}

// TestStoreLearning_EmptyCategory verifies that an empty category is
// allowed and stored as an empty string.
func TestStoreLearning_EmptyCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning without category",
		Tag:         "test",
		Category:    "",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with empty category, got error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Empty category should be returned as empty string.
	category, ok := parsed["category"].(string)
	if !ok || category != "" {
		t.Errorf("expected empty category, got %q", category)
	}
}

// TestStoreLearning_AllValidCategories verifies that all valid category
// values are accepted.
func TestStoreLearning_AllValidCategories(t *testing.T) {
	categories := []string{"decision", "pattern", "gotcha", "context", "reference"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			s := newTestStore(t)
			l := NewLearning(nil, s, "")

			result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
				Information: "test learning for " + cat,
				Tag:         "test",
				Category:    cat,
			})
			if err != nil {
				t.Fatalf("StoreLearning error: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected success for category %q, got error: %s", cat, resultText(result))
			}

			parsed := parseLearningResult(t, resultText(result))
			if parsed["category"] != cat {
				t.Errorf("expected category %q, got %v", cat, parsed["category"])
			}
		})
	}
}

// TestStoreLearning_SequenceIncrement verifies that storing multiple
// learnings with the same tag produces distinct identities, all starting
// with the same tag prefix. Since timestamps may collide within the same
// second, collision suffixes (-2, -3) may be appended. A vault path is
// required for the file-based collision avoidance mechanism to work.
func TestStoreLearning_SequenceIncrement(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()
	l := NewLearning(nil, s, vaultPath)

	deployPattern := regexp.MustCompile(`^deployment-\d{8}T\d{6}-testuser(-\d+)?$`)
	identities := make(map[string]bool)

	for i := 0; i < 3; i++ {
		result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
			Information: strings.Repeat("learning content ", i+1), // unique content
			Tag:         "deployment",
		})
		if err != nil {
			t.Fatalf("StoreLearning[%d] error: %v", i, err)
		}
		if result.IsError {
			t.Fatalf("StoreLearning[%d] returned error: %s", i, resultText(result))
		}

		parsed := parseLearningResult(t, resultText(result))
		identity, ok := parsed["identity"].(string)
		if !ok {
			t.Fatalf("learning[%d] expected string identity, got %v", i, parsed["identity"])
		}
		if !deployPattern.MatchString(identity) {
			t.Errorf("learning[%d] identity %q does not match deployment-{timestamp}-testuser pattern", i, identity)
		}

		// Verify page name matches identity.
		page, ok := parsed["page"].(string)
		if !ok || page != "learning/"+identity {
			t.Errorf("learning[%d] page = %q, want %q", i, page, "learning/"+identity)
		}

		identities[identity] = true
	}

	// All three identities must be distinct.
	if len(identities) != 3 {
		t.Errorf("expected 3 distinct identities, got %d: %v", len(identities), identities)
	}

	// Verify all 3 pages exist in the store.
	pages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("expected 3 learning pages, got %d", len(pages))
	}
}

// TestStoreLearning_TagNormalization verifies that tags are normalized:
// lowercase, spaces replaced with hyphens, non-alphanumeric stripped.
func TestStoreLearning_TagNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputTag string
		wantTag  string
	}{
		{"uppercase", "Authentication", "authentication"},
		{"spaces", "My Tag Name", "my-tag-name"},
		{"leading trailing spaces", "  auth  ", "auth"},
		{"special chars", "auth@#$%enti!cation", "authentication"},
		{"mixed", "  My Tag!  ", "my-tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			l := NewLearning(nil, s, "")

			result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
				Information: "test learning",
				Tag:         tt.inputTag,
			})
			if err != nil {
				t.Fatalf("StoreLearning error: %v", err)
			}
			if result.IsError {
				t.Fatalf("StoreLearning returned error: %s", resultText(result))
			}

			parsed := parseLearningResult(t, resultText(result))
			tag, ok := parsed["tag"].(string)
			if !ok || tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

// TestStoreLearning_TierDraft verifies that all stored learnings have
// tier "draft" regardless of input.
func TestStoreLearning_TierDraft(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning for tier check",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))
	pageName, _ := parsed["page"].(string)

	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}
	if storedPage.Tier != "draft" {
		t.Errorf("tier = %q, want %q", storedPage.Tier, "draft")
	}
}

// TestStoreLearning_CreatedAtInResponse verifies that the response includes
// a non-empty created_at field in ISO 8601 format.
func TestStoreLearning_CreatedAtInResponse(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning for created_at",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))
	createdAt, ok := parsed["created_at"].(string)
	if !ok || createdAt == "" {
		t.Errorf("expected non-empty created_at, got %v", parsed["created_at"])
	}

	// Verify it contains a 'T' (ISO 8601 separator) and 'Z' (UTC).
	if !strings.Contains(createdAt, "T") || !strings.Contains(createdAt, "Z") {
		t.Errorf("created_at = %q, expected ISO 8601 format with T and Z", createdAt)
	}
}

// TestStoreLearning_EmbedderUnavailable verifies that when the embedder
// reports Available() == false, the learning is still stored successfully
// and the message mentions embeddings.
func TestStoreLearning_EmbedderUnavailable(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(false) // Available() returns false
	l := NewLearning(e, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "learning with unavailable embedder",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder unavailable, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	parsed := parseLearningResult(t, resultText(result))
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "Embeddings") && !strings.Contains(msg, "embeddings") {
		t.Errorf("message = %q, should mention embeddings", msg)
	}
}

// TestStoreLearning_NilEmbedder verifies that when the embedder is nil,
// the learning is still stored successfully and the message mentions
// embeddings not being generated.
func TestStoreLearning_NilEmbedder(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "") // nil embedder

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "learning with nil embedder",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder is nil, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	parsed := parseLearningResult(t, resultText(result))
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "Embeddings") && !strings.Contains(msg, "embeddings") {
		t.Errorf("message = %q, should mention embeddings", msg)
	}
}

// TestStoreLearning_Searchable verifies that a stored learning creates a
// page in the store that can be found by listing pages, with source_id
// set to "learning".
func TestStoreLearning_Searchable(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "searchable learning content",
		Tag:         "search-test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Extract the page name from the result.
	parsed := parseLearningResult(t, resultText(result))
	pageName, _ := parsed["page"].(string)

	// Verify the page exists in the store.
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}

	var found bool
	for _, p := range pages {
		if p.Name == pageName {
			found = true
			if p.SourceID != "learning" {
				t.Errorf("page %q source_id = %q, want %q", pageName, p.SourceID, "learning")
			}
			break
		}
	}
	if !found {
		t.Errorf("page %q not found in store after StoreLearning", pageName)
	}

	// Verify blocks were persisted for the page.
	blocks, err := s.GetBlocksByPage(pageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage(%q): %v", pageName, err)
	}
	if len(blocks) == 0 {
		t.Errorf("expected at least 1 block for page %q, got 0", pageName)
	}
}

// TestStoreLearning_FilterBySourceType verifies that the stored learning
// page has source_id = "learning", which enables filtering via
// dewey_semantic_search_filtered. This proves the learning is distinguishable
// from other content sources.
func TestStoreLearning_FilterBySourceType(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	// Store a learning.
	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "filterable learning",
		Tag:         "filter-test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Also insert a non-learning page to verify filtering.
	err = s.InsertPage(&store.Page{
		Name:         "regular-page",
		OriginalName: "regular-page",
		SourceID:     "disk-local",
		SourceDocID:  "regular.md",
		ContentHash:  "abc",
	})
	if err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// List pages by source "learning" — only the learning page should appear.
	learningPages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(learningPages) != 1 {
		t.Fatalf("expected 1 learning page, got %d", len(learningPages))
	}
	if learningPages[0].SourceID != "learning" {
		t.Errorf("source_id = %q, want %q", learningPages[0].SourceID, "learning")
	}

	// Verify the regular page is NOT in the learning source.
	diskPages, err := s.ListPagesBySource("disk-local")
	if err != nil {
		t.Fatalf("ListPagesBySource(disk-local): %v", err)
	}
	if len(diskPages) != 1 {
		t.Fatalf("expected 1 disk-local page, got %d", len(diskPages))
	}
	if diskPages[0].Name != "regular-page" {
		t.Errorf("disk page name = %q, want %q", diskPages[0].Name, "regular-page")
	}
}

// TestStoreLearning_DifferentTagSequences verifies that learnings with
// different tags produce identities with the correct tag prefix. All
// identities should contain the same author. A vault path is required
// for the file-based collision avoidance mechanism to work when multiple
// learnings are stored within the same second.
func TestStoreLearning_DifferentTagSequences(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()
	l := NewLearning(nil, s, vaultPath)

	authPattern := regexp.MustCompile(`^auth-\d{8}T\d{6}-testuser(-\d+)?$`)
	deployPattern := regexp.MustCompile(`^deploy-\d{8}T\d{6}-testuser(-\d+)?$`)

	// Store learning with tag "auth".
	result1, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "auth learning 1",
		Tag:         "auth",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result1.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result1))
	}

	// Store learning with tag "deploy".
	result2, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "deploy learning 1",
		Tag:         "deploy",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result2))
	}

	// Store another learning with tag "auth".
	result3, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "auth learning 2",
		Tag:         "auth",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result3.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result3))
	}

	parsed1 := parseLearningResult(t, resultText(result1))
	parsed2 := parseLearningResult(t, resultText(result2))
	parsed3 := parseLearningResult(t, resultText(result3))

	id1, _ := parsed1["identity"].(string)
	id2, _ := parsed2["identity"].(string)
	id3, _ := parsed3["identity"].(string)

	if !authPattern.MatchString(id1) {
		t.Errorf("first auth identity %q does not match auth-{timestamp}-testuser pattern", id1)
	}
	if !deployPattern.MatchString(id2) {
		t.Errorf("deploy identity %q does not match deploy-{timestamp}-testuser pattern", id2)
	}
	if !authPattern.MatchString(id3) {
		t.Errorf("second auth identity %q does not match auth-{timestamp}-testuser pattern", id3)
	}

	// All identities should contain the same author.
	for i, id := range []string{id1, id2, id3} {
		if !strings.Contains(id, "-testuser") {
			t.Errorf("identity[%d] %q should contain '-testuser'", i, id)
		}
	}

	// Auth identities should be distinct (different timestamps or collision suffixes).
	if id1 == id3 {
		t.Errorf("two auth identities should be distinct, both are %q", id1)
	}
}

// --- Phase 1 (015-curated-knowledge-stores): File-backed learning tests ---

// TestStoreLearning_DualWritesMarkdown verifies that storing a learning
// creates a markdown file alongside the SQLite record. The file should
// exist at {vaultPath}/.uf/dewey/learnings/{identity}.md.
func TestStoreLearning_DualWritesMarkdown(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()
	l := NewLearning(nil, s, vaultPath)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "OAuth tokens should be rotated every 24 hours",
		Tag:         "authentication",
		Category:    "decision",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Extract the identity from the response to find the file.
	parsed := parseLearningResult(t, resultText(result))
	identity, ok := parsed["identity"].(string)
	if !ok || identity == "" {
		t.Fatalf("expected non-empty identity in response, got %v", parsed["identity"])
	}

	// Verify the identity matches the new format.
	authPattern := regexp.MustCompile(`^authentication-\d{8}T\d{6}-testuser$`)
	if !authPattern.MatchString(identity) {
		t.Errorf("identity %q does not match authentication-{timestamp}-testuser pattern", identity)
	}

	// Verify the markdown file was created with the new-format filename.
	mdPath := filepath.Join(vaultPath, ".uf", "dewey", "learnings", identity+".md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Fatalf("expected markdown file at %s, but it does not exist", mdPath)
	}

	// Verify the response includes file_path.
	filePath, ok := parsed["file_path"].(string)
	if !ok || filePath == "" {
		t.Errorf("expected non-empty file_path in response, got %v", parsed["file_path"])
	}
	if !strings.Contains(filePath, "learnings/"+identity+".md") {
		t.Errorf("file_path = %q, expected to contain 'learnings/%s.md'", filePath, identity)
	}
}

// TestStoreLearning_MarkdownFormat verifies that the markdown file has
// correct YAML frontmatter with all required fields including author.
func TestStoreLearning_MarkdownFormat(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()
	l := NewLearning(nil, s, vaultPath)

	info := "Always use parameterized queries for SQL"
	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: info,
		Tag:         "security",
		Category:    "pattern",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Extract identity from response to find the file.
	parsed := parseLearningResult(t, resultText(result))
	identity, _ := parsed["identity"].(string)

	// Read the markdown file.
	mdPath := filepath.Join(vaultPath, ".uf", "dewey", "learnings", identity+".md")
	content, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	contentStr := string(content)

	// Verify YAML frontmatter structure.
	if !strings.HasPrefix(contentStr, "---\n") {
		t.Error("markdown file should start with '---'")
	}

	// Verify required frontmatter fields.
	requiredFields := []string{
		"tag: security",
		"author: testuser",
		"category: pattern",
		"identity: " + identity,
		"tier: draft",
		"created_at:",
	}
	for _, field := range requiredFields {
		if !strings.Contains(contentStr, field) {
			t.Errorf("markdown file missing frontmatter field %q", field)
		}
	}

	// Verify the body contains the learning information.
	if !strings.Contains(contentStr, info) {
		t.Errorf("markdown file body should contain the learning text")
	}
}

// TestStoreLearning_MarkdownFormatNoCategory verifies that when no category
// is provided, the category field is omitted from the YAML frontmatter.
func TestStoreLearning_MarkdownFormatNoCategory(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()
	l := NewLearning(nil, s, vaultPath)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "A learning without a category",
		Tag:         "general",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Extract identity from response to find the file.
	parsed := parseLearningResult(t, resultText(result))
	identity, _ := parsed["identity"].(string)

	mdPath := filepath.Join(vaultPath, ".uf", "dewey", "learnings", identity+".md")
	content, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Category should NOT appear in frontmatter when empty.
	if strings.Contains(string(content), "category:") {
		t.Error("frontmatter should not contain 'category:' when category is empty")
	}
}

// TestStoreLearning_FileWriteFailure verifies that when the file write
// fails (e.g., read-only directory), the store_learning call still
// succeeds — the SQLite write is the primary store.
func TestStoreLearning_FileWriteFailure(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	vaultPath := t.TempDir()

	// Create the learnings directory and make it read-only to force
	// file write failure.
	learningsDir := filepath.Join(vaultPath, ".uf", "dewey", "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(learningsDir, 0o444); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so t.TempDir() cleanup can remove it.
		_ = os.Chmod(learningsDir, 0o755)
	})

	l := NewLearning(nil, s, vaultPath)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "This learning should still be stored in SQLite",
		Tag:         "resilience",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// The operation should succeed despite file write failure.
	if result.IsError {
		t.Fatalf("expected success even when file write fails, got error: %s", resultText(result))
	}

	// Extract the page name from the response to look up in the store.
	parsed := parseLearningResult(t, resultText(result))
	pageName, ok := parsed["page"].(string)
	if !ok || pageName == "" {
		t.Fatalf("expected non-empty page in response, got %v", parsed["page"])
	}

	// Verify the learning was stored in SQLite using the page name from the response.
	page, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage(%q): %v", pageName, err)
	}
	if page == nil {
		t.Fatal("learning page should exist in store despite file write failure")
	}

	// Verify file_path is empty in the response (write failed).
	filePath, _ := parsed["file_path"].(string)
	if filePath != "" {
		t.Errorf("expected empty file_path when write fails, got %q", filePath)
	}
}

// TestStoreLearning_NoVaultPath verifies that when vaultPath is empty,
// no file is written but the learning is still stored in SQLite.
func TestStoreLearning_NoVaultPath(t *testing.T) {
	t.Setenv("DEWEY_AUTHOR", "testuser")
	s := newTestStore(t)
	l := NewLearning(nil, s, "")

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Learning without vault path",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Extract the page name from the response to look up in the store.
	parsed := parseLearningResult(t, resultText(result))
	pageName, ok := parsed["page"].(string)
	if !ok || pageName == "" {
		t.Fatalf("expected non-empty page in response, got %v", parsed["page"])
	}

	// Verify learning was stored in SQLite using the page name from the response.
	page, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage(%q): %v", pageName, err)
	}
	if page == nil {
		t.Fatal("learning page should exist in store")
	}

	// Verify file_path is empty (no vault path configured).
	filePath, _ := parsed["file_path"].(string)
	if filePath != "" {
		t.Errorf("expected empty file_path when vaultPath is empty, got %q", filePath)
	}
}
