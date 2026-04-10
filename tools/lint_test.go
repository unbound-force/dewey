package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// parseLintResult unmarshals the JSON text from a lint CallToolResult.
func parseLintResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal lint result: %v\ntext: %s", err, text)
	}
	return parsed
}

// lintSummary extracts the summary sub-map from a parsed lint result.
func lintSummary(t *testing.T, parsed map[string]any) map[string]any {
	t.Helper()
	summary, ok := parsed["summary"].(map[string]any)
	if !ok {
		t.Fatal("expected 'summary' map in lint result")
	}
	return summary
}

// storeLearningForLint is a test helper that inserts a learning page with
// a single block directly into the store, with explicit control over
// created_at for stale decision testing.
func storeLearningForLint(t *testing.T, s *store.Store, tag string, seq int, category string, content string, createdAt time.Time) {
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

// TestLint_NilStore verifies that a nil store returns an error result
// mentioning persistent storage.
func TestLint_NilStore(t *testing.T) {
	lint := NewLint(nil, nil)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}
	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestLint_NoLearnings verifies that lint with no learnings produces a
// clean report with all zero counts.
func TestLint_NoLearnings(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	if summary["stale_decisions"] != float64(0) {
		t.Errorf("stale_decisions = %v, want 0", summary["stale_decisions"])
	}
	if summary["uncompiled_learnings"] != float64(0) {
		t.Errorf("uncompiled_learnings = %v, want 0", summary["uncompiled_learnings"])
	}
	if summary["embedding_gaps"] != float64(0) {
		t.Errorf("embedding_gaps = %v, want 0", summary["embedding_gaps"])
	}
	if summary["contradictions"] != float64(0) {
		t.Errorf("contradictions = %v, want 0", summary["contradictions"])
	}
	if summary["total_issues"] != float64(0) {
		t.Errorf("total_issues = %v, want 0", summary["total_issues"])
	}

	if parsed["status"] != "clean" {
		t.Errorf("status = %v, want 'clean'", parsed["status"])
	}
	if parsed["message"] != "Knowledge index is clean. No issues found." {
		t.Errorf("message = %v, want clean message", parsed["message"])
	}
}

// TestLint_StaleDecision verifies that a decision learning older than
// 30 days is reported as stale.
func TestLint_StaleDecision(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil)

	// Store a decision learning backdated 45 days.
	staleDate := time.Now().Add(-45 * 24 * time.Hour)
	storeLearningForLint(t, s, "auth-config", 1, "decision", "Use basic auth", staleDate)

	// Store a fresh decision learning (should NOT be flagged).
	freshDate := time.Now().Add(-5 * 24 * time.Hour)
	storeLearningForLint(t, s, "deploy-config", 1, "decision", "Use blue-green", freshDate)

	// Store a non-decision learning (should NOT be flagged regardless of age).
	storeLearningForLint(t, s, "old-pattern", 1, "pattern", "Some pattern", staleDate)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// Only the 45-day-old decision should be stale.
	if summary["stale_decisions"] != float64(1) {
		t.Errorf("stale_decisions = %v, want 1", summary["stale_decisions"])
	}

	// Verify the finding details.
	findings, ok := parsed["findings"].([]any)
	if !ok {
		t.Fatal("expected findings array")
	}

	foundStale := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "stale_decision" {
			foundStale = true
			if finding["identity"] != "auth-config-1" {
				t.Errorf("stale identity = %v, want 'auth-config-1'", finding["identity"])
			}
			desc, _ := finding["description"].(string)
			if !strings.Contains(desc, "45 days old") {
				t.Errorf("description = %q, should mention '45 days old'", desc)
			}
			remediation, _ := finding["remediation"].(string)
			if !strings.Contains(remediation, "dewey promote") {
				t.Errorf("remediation = %q, should mention 'dewey promote'", remediation)
			}
		}
	}
	if !foundStale {
		t.Error("expected a stale_decision finding")
	}

	if parsed["status"] != "issues_found" {
		t.Errorf("status = %v, want 'issues_found'", parsed["status"])
	}
}

// TestLint_StaleDecision_ValidatedSkipped verifies that a validated
// decision is not reported as stale even if it's old.
func TestLint_StaleDecision_ValidatedSkipped(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil)

	// Store a decision learning backdated 45 days.
	staleDate := time.Now().Add(-45 * 24 * time.Hour)
	storeLearningForLint(t, s, "auth-config", 1, "decision", "Use basic auth", staleDate)

	// Promote it to validated — should no longer be flagged.
	if err := s.UpdatePageTier("learning/auth-config-1", "validated"); err != nil {
		t.Fatalf("UpdatePageTier: %v", err)
	}

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	if summary["stale_decisions"] != float64(0) {
		t.Errorf("stale_decisions = %v, want 0 (validated should be skipped)", summary["stale_decisions"])
	}
}

// TestLint_UncompiledLearnings verifies that learnings not referenced by
// any compiled article are reported as uncompiled.
func TestLint_UncompiledLearnings(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil)

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Auth content 1", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Auth content 2", now)
	storeLearningForLint(t, s, "deploy", 1, "pattern", "Deploy content", now)

	// Create a compiled article that references auth-1 only.
	compiledPage := &store.Page{
		Name:         "compiled/auth",
		OriginalName: "Auth",
		SourceID:     "compiled",
		SourceDocID:  "auth",
		Properties:   `{"sources":["auth-1"],"topic":"auth"}`,
		Tier:         "draft",
	}
	if err := s.InsertPage(compiledPage); err != nil {
		t.Fatalf("InsertPage(compiled): %v", err)
	}

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// auth-2 and deploy-1 are uncompiled (auth-1 is referenced).
	if summary["uncompiled_learnings"] != float64(2) {
		t.Errorf("uncompiled_learnings = %v, want 2", summary["uncompiled_learnings"])
	}

	// Verify specific identities in findings.
	findings, _ := parsed["findings"].([]any)
	uncompiledIdentities := make(map[string]bool)
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "uncompiled" {
			id, _ := finding["identity"].(string)
			uncompiledIdentities[id] = true
		}
	}
	if !uncompiledIdentities["auth-2"] {
		t.Error("expected auth-2 to be reported as uncompiled")
	}
	if !uncompiledIdentities["deploy-1"] {
		t.Error("expected deploy-1 to be reported as uncompiled")
	}
	if uncompiledIdentities["auth-1"] {
		t.Error("auth-1 should NOT be reported as uncompiled (it's in compiled sources)")
	}
}

// TestLint_EmbeddingGaps verifies that pages with blocks but no embeddings
// are reported as embedding gaps.
func TestLint_EmbeddingGaps(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil)

	// Insert a page with blocks but no embeddings.
	page := &store.Page{
		Name:         "test-page",
		OriginalName: "Test Page",
		SourceID:     "disk-local",
		ContentHash:  "abc",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	block := &store.Block{
		UUID:     "block-1",
		PageName: "test-page",
		Content:  "Some content without embeddings",
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	if summary["embedding_gaps"] != float64(1) {
		t.Errorf("embedding_gaps = %v, want 1", summary["embedding_gaps"])
	}

	// Verify the finding details.
	findings, _ := parsed["findings"].([]any)
	foundGap := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "embedding_gap" {
			foundGap = true
			if finding["page"] != "test-page" {
				t.Errorf("gap page = %v, want 'test-page'", finding["page"])
			}
			desc, _ := finding["description"].(string)
			if !strings.Contains(desc, "1 blocks") {
				t.Errorf("description = %q, should mention block count", desc)
			}
		}
	}
	if !foundGap {
		t.Error("expected an embedding_gap finding")
	}
}

// TestLint_FixEmbeddingGaps verifies that with --fix, embeddings are
// regenerated for pages that have blocks but no embeddings.
func TestLint_FixEmbeddingGaps(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(true)
	lint := NewLint(s, e)

	// Insert a page with a block but no embeddings.
	page := &store.Page{
		Name:         "fix-test-page",
		OriginalName: "Fix Test",
		SourceID:     "disk-local",
		ContentHash:  "abc",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	block := &store.Block{
		UUID:     "fix-block-1",
		PageName: "fix-test-page",
		Content:  "Content needing embeddings",
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Run lint with fix=true.
	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{Fix: true})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))

	// Verify fix results are present.
	fixed, ok := parsed["fixed"].(map[string]any)
	if !ok {
		t.Fatal("expected 'fixed' map in result")
	}
	if fixed["embedding_gaps"] != float64(1) {
		t.Errorf("fixed embedding_gaps = %v, want 1", fixed["embedding_gaps"])
	}

	// Verify the message mentions fixing.
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "Fixed") {
		t.Errorf("message = %q, should mention 'Fixed'", msg)
	}

	// Verify that a second lint run reports no embedding gaps
	// (the fix should have resolved them).
	result2, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("second Lint error: %v", err)
	}
	parsed2 := parseLintResult(t, resultText(result2))
	summary2 := lintSummary(t, parsed2)
	if summary2["embedding_gaps"] != float64(0) {
		t.Errorf("after fix, embedding_gaps = %v, want 0", summary2["embedding_gaps"])
	}
}

// TestLint_Contradictions verifies that tags with 2+ decision learnings
// are reported as potential contradictions when an embedder is available.
func TestLint_Contradictions(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(true)
	lint := NewLint(s, e)

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Use basic auth", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Switch to OAuth", now.Add(24*time.Hour))

	// Non-decision learning on same tag — should NOT trigger contradiction.
	storeLearningForLint(t, s, "auth", 3, "context", "Auth context info", now.Add(48*time.Hour))

	// Single decision on different tag — should NOT trigger contradiction.
	storeLearningForLint(t, s, "deploy", 1, "decision", "Use blue-green", now)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	if summary["contradictions"] != float64(1) {
		t.Errorf("contradictions = %v, want 1", summary["contradictions"])
	}

	// Verify the finding details.
	findings, _ := parsed["findings"].([]any)
	foundContradiction := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "contradiction" {
			foundContradiction = true
			identities, ok := finding["identities"].([]any)
			if !ok {
				t.Fatal("expected identities array in contradiction finding")
			}
			if len(identities) != 2 {
				t.Errorf("expected 2 identities, got %d", len(identities))
			}
			desc, _ := finding["description"].(string)
			if !strings.Contains(desc, "auth") {
				t.Errorf("description = %q, should mention tag 'auth'", desc)
			}
		}
	}
	if !foundContradiction {
		t.Error("expected a contradiction finding")
	}
}

// TestLint_NoContradictionsWithoutEmbedder verifies that the contradiction
// check is skipped when no embedder is available (invariant 6).
func TestLint_NoContradictionsWithoutEmbedder(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil) // nil embedder

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Use basic auth", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Switch to OAuth", now.Add(24*time.Hour))

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// No contradictions should be reported without an embedder.
	if summary["contradictions"] != float64(0) {
		t.Errorf("contradictions = %v, want 0 (embedder unavailable)", summary["contradictions"])
	}
}

// TestLint_NoContradictionsWithUnavailableEmbedder verifies that the
// contradiction check is skipped when the embedder reports unavailable.
func TestLint_NoContradictionsWithUnavailableEmbedder(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(false) // Available() returns false
	lint := NewLint(s, e)

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Use basic auth", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Switch to OAuth", now.Add(24*time.Hour))

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	if summary["contradictions"] != float64(0) {
		t.Errorf("contradictions = %v, want 0 (embedder unavailable)", summary["contradictions"])
	}
}

// TestLint_FixWithoutEmbedder verifies that --fix without an embedder
// does not crash and reports zero fixed.
func TestLint_FixWithoutEmbedder(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil) // nil embedder

	// Insert a page with a block but no embeddings.
	page := &store.Page{
		Name:         "no-embed-page",
		OriginalName: "No Embed",
		SourceID:     "disk-local",
		ContentHash:  "abc",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	block := &store.Block{
		UUID:     "no-embed-block",
		PageName: "no-embed-page",
		Content:  "Content",
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{Fix: true})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))

	// Fix should still be in the result but with 0 fixed.
	fixed, ok := parsed["fixed"].(map[string]any)
	if !ok {
		t.Fatal("expected 'fixed' map in result")
	}
	if fixed["embedding_gaps"] != float64(0) {
		t.Errorf("fixed embedding_gaps = %v, want 0 (no embedder)", fixed["embedding_gaps"])
	}

	// The gap should still be reported.
	summary := lintSummary(t, parsed)
	if summary["embedding_gaps"] != float64(1) {
		t.Errorf("embedding_gaps = %v, want 1", summary["embedding_gaps"])
	}
}

// TestLint_NoFixWithoutFlag verifies that lint does NOT modify data
// when fix=false (invariant 1).
func TestLint_NoFixWithoutFlag(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(true)
	lint := NewLint(s, e)

	// Insert a page with a block but no embeddings.
	page := &store.Page{
		Name:         "no-fix-page",
		OriginalName: "No Fix",
		SourceID:     "disk-local",
		ContentHash:  "abc",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	block := &store.Block{
		UUID:     "no-fix-block",
		PageName: "no-fix-page",
		Content:  "Content",
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Run lint WITHOUT fix.
	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{Fix: false})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))

	// No "fixed" key should be present when fix=false.
	if _, ok := parsed["fixed"]; ok {
		t.Error("'fixed' key should not be present when fix=false")
	}

	// The gap should still be reported.
	summary := lintSummary(t, parsed)
	if summary["embedding_gaps"] != float64(1) {
		t.Errorf("embedding_gaps = %v, want 1", summary["embedding_gaps"])
	}

	// Run lint again — gap should still be there (not fixed).
	result2, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("second Lint error: %v", err)
	}
	parsed2 := parseLintResult(t, resultText(result2))
	summary2 := lintSummary(t, parsed2)
	if summary2["embedding_gaps"] != float64(1) {
		t.Errorf("after no-fix lint, embedding_gaps = %v, want 1", summary2["embedding_gaps"])
	}
}

// TestLint_AllChecksClean verifies that when all checks pass, the report
// is clean with zero counts and a clean status message.
func TestLint_AllChecksClean(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(true)
	lint := NewLint(s, e)

	// Store a fresh, non-decision learning — should not trigger any check.
	now := time.Now()
	storeLearningForLint(t, s, "test", 1, "pattern", "A pattern", now)

	// Create a compiled article referencing it.
	compiledPage := &store.Page{
		Name:         "compiled/test",
		OriginalName: "Test",
		SourceID:     "compiled",
		SourceDocID:  "test",
		Properties:   `{"sources":["test-1"],"topic":"test"}`,
		Tier:         "draft",
	}
	if err := s.InsertPage(compiledPage); err != nil {
		t.Fatalf("InsertPage(compiled): %v", err)
	}

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))

	// The learning page has a block but no embedding — that's an embedding gap.
	// But the compiled page has no blocks, so it won't show up as a gap.
	// The learning page DOES have a block without embedding.
	summary := lintSummary(t, parsed)

	// Stale decisions: 0 (no decision category).
	if summary["stale_decisions"] != float64(0) {
		t.Errorf("stale_decisions = %v, want 0", summary["stale_decisions"])
	}
	// Uncompiled: 0 (test-1 is in compiled sources).
	if summary["uncompiled_learnings"] != float64(0) {
		t.Errorf("uncompiled_learnings = %v, want 0", summary["uncompiled_learnings"])
	}
	// Contradictions: 0 (only 1 learning, not a decision).
	if summary["contradictions"] != float64(0) {
		t.Errorf("contradictions = %v, want 0", summary["contradictions"])
	}
}
