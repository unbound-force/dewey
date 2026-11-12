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

	"github.com/unbound-force/dewey/v3/curate"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
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
// created_at for stale decision testing. Uses new-format identities
// ({tag}-{timestamp}-{author}) and includes "tag" in properties JSON.
func storeLearningForLint(t *testing.T, s *store.Store, tag string, seq int, category string, content string, createdAt time.Time) {
	t.Helper()
	// Use new-format identity: {tag}-{YYYYMMDDTHHMMSS}-test.
	// The seq parameter is used to offset the timestamp for uniqueness.
	ts := createdAt.Add(time.Duration(seq) * time.Second).UTC().Format("20060102T150405")
	identity := fmt.Sprintf("%s-%s-test", tag, ts)
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
	lint := NewLint(nil, nil, "", 0)

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
	lint := NewLint(s, nil, "", 0)

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
	lint := NewLint(s, nil, "", 0)

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
			id, _ := finding["identity"].(string)
			if !strings.HasPrefix(id, "auth-config-") {
				t.Errorf("stale identity = %v, want prefix 'auth-config-'", finding["identity"])
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
	lint := NewLint(s, nil, "", 0)

	// Store a decision learning backdated 45 days.
	staleDate := time.Now().Add(-45 * 24 * time.Hour)
	storeLearningForLint(t, s, "auth-config", 1, "decision", "Use basic auth", staleDate)

	// Construct the page name using the same identity format as storeLearningForLint.
	ts := staleDate.Add(1 * time.Second).UTC().Format("20060102T150405")
	pageName := fmt.Sprintf("learning/auth-config-%s-test", ts)

	// Promote it to validated — should no longer be flagged.
	page, err := s.GetPage(pageName)
	if err != nil || page == nil {
		t.Fatalf("get page: %v", err)
	}
	page.Tier = "validated"
	if err := s.UpdatePage(page); err != nil {
		t.Fatalf("UpdatePage: %v", err)
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
	lint := NewLint(s, nil, "", 0)

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Auth content 1", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Auth content 2", now)
	storeLearningForLint(t, s, "deploy", 1, "pattern", "Deploy content", now)

	// Construct the identity for auth seq=1 to reference in compiled sources.
	authID1 := fmt.Sprintf("auth-%s-test", now.Add(1*time.Second).UTC().Format("20060102T150405"))

	// Create a compiled article that references auth seq=1 only.
	compiledPage := &store.Page{
		Name:         "compiled/auth",
		OriginalName: "Auth",
		SourceID:     "compiled",
		SourceDocID:  "auth",
		Properties:   fmt.Sprintf(`{"sources":["%s"],"topic":"auth"}`, authID1),
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

	// auth seq=2 and deploy seq=1 are uncompiled (auth seq=1 is referenced).
	if summary["uncompiled_learnings"] != float64(2) {
		t.Errorf("uncompiled_learnings = %v, want 2", summary["uncompiled_learnings"])
	}

	// Verify findings contain the expected uncompiled identities.
	findings, _ := parsed["findings"].([]any)
	uncompiledCount := 0
	foundAuthUncompiled := false
	foundDeployUncompiled := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "uncompiled" {
			uncompiledCount++
			id, _ := finding["identity"].(string)
			if strings.HasPrefix(id, "auth-") {
				foundAuthUncompiled = true
			}
			if strings.HasPrefix(id, "deploy-") {
				foundDeployUncompiled = true
			}
			// The referenced auth learning should NOT appear.
			if id == authID1 {
				t.Errorf("%s should NOT be reported as uncompiled (it's in compiled sources)", authID1)
			}
		}
	}
	if !foundAuthUncompiled {
		t.Error("expected an auth learning to be reported as uncompiled")
	}
	if !foundDeployUncompiled {
		t.Error("expected a deploy learning to be reported as uncompiled")
	}
}

// TestLint_EmbeddingGaps verifies that pages with blocks but no embeddings
// are reported as embedding gaps.
func TestLint_EmbeddingGaps(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil, "", 0)

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
	lint := NewLint(s, e, "", 0)

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
	lint := NewLint(s, e, "", 0)

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
	lint := NewLint(s, nil, "", 0) // nil embedder

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
	lint := NewLint(s, e, "", 0)

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
	lint := NewLint(s, nil, "", 0) // nil embedder

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
	lint := NewLint(s, e, "", 0)

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
	lint := NewLint(s, e, "", 0)

	// Store a fresh, non-decision learning — should not trigger any check.
	now := time.Now()
	storeLearningForLint(t, s, "test", 1, "pattern", "A pattern", now)

	// Construct the identity for test seq=1 to reference in compiled sources.
	testID := fmt.Sprintf("test-%s-test", now.Add(1*time.Second).UTC().Format("20060102T150405"))

	// Create a compiled article referencing it.
	compiledPage := &store.Page{
		Name:         "compiled/test",
		OriginalName: "Test",
		SourceID:     "compiled",
		SourceDocID:  "test",
		Properties:   fmt.Sprintf(`{"sources":["%s"],"topic":"test"}`, testID),
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

// --- Knowledge Store Lint Tests (T037) ---

// writeKnowledgeFile is a test helper that writes a curated knowledge file
// to the given store directory with the specified frontmatter fields.
func writeKnowledgeFile(t *testing.T, storePath, tag string, seq int, confidence string, qualityFlags []string) {
	t.Helper()

	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatalf("create store dir: %v", err)
	}

	filename := fmt.Sprintf("%s-%d.md", tag, seq)
	filePath := filepath.Join(storePath, filename)

	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("tag: %s\n", tag))
	buf.WriteString("category: decision\n")
	buf.WriteString(fmt.Sprintf("confidence: %s\n", confidence))
	buf.WriteString("tier: curated\n")
	buf.WriteString("created_at: 2025-01-01T00:00:00Z\n")
	buf.WriteString("store: test-store\n")

	if len(qualityFlags) > 0 {
		buf.WriteString("quality_flags:\n")
		for _, flag := range qualityFlags {
			buf.WriteString(fmt.Sprintf("  - type: %s\n", flag))
			buf.WriteString(fmt.Sprintf("    detail: \"Test %s flag\"\n", flag))
		}
	} else {
		buf.WriteString("quality_flags: []\n")
	}

	buf.WriteString("---\n\n")
	buf.WriteString("Test knowledge content.\n")

	if err := os.WriteFile(filePath, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write knowledge file: %v", err)
	}
}

// writeKnowledgeStoresConfig is a test helper that writes a knowledge-stores.yaml
// config file to the given dewey directory.
func writeKnowledgeStoresConfig(t *testing.T, deweyDir string, stores []curate.StoreConfig) {
	t.Helper()

	var buf strings.Builder
	buf.WriteString("stores:\n")
	for _, cfg := range stores {
		buf.WriteString(fmt.Sprintf("  - name: %s\n", cfg.Name))
		if len(cfg.Sources) > 0 {
			buf.WriteString("    sources:\n")
			for _, src := range cfg.Sources {
				buf.WriteString(fmt.Sprintf("      - %s\n", src))
			}
		}
		if cfg.Path != "" {
			buf.WriteString(fmt.Sprintf("    path: %s\n", cfg.Path))
		}
	}

	ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	if err := os.WriteFile(ksPath, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write knowledge-stores.yaml: %v", err)
	}
}

// TestLint_KnowledgeStoreMetrics verifies that lint reports knowledge store
// quality metrics including confidence distribution and quality flag counts.
func TestLint_KnowledgeStoreMetrics(t *testing.T) {
	s := newTestStore(t)

	// Set up vault directory structure.
	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	// Create a knowledge store with curated files.
	storePath := filepath.Join(deweyDir, "knowledge", "test-store")

	writeKnowledgeStoresConfig(t, deweyDir, []curate.StoreConfig{
		{Name: "test-store", Sources: []string{"disk-local"}, Path: storePath},
	})

	// Write curated files with various confidence levels and quality flags.
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)
	writeKnowledgeFile(t, storePath, "deploy", 1, "medium", nil)
	writeKnowledgeFile(t, storePath, "security", 1, "low", []string{"missing_rationale"})
	writeKnowledgeFile(t, storePath, "infra", 1, "flagged", []string{"implied_assumption", "unsupported_claim"})

	lint := NewLint(s, nil, vaultDir, 0)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// Verify knowledge quality issues are reported.
	// low-confidence (security-1) + flagged-confidence (infra-1) = 2 confidence findings
	// + 1 missing_rationale + 1 implied_assumption + 1 unsupported_claim = 3 flag findings
	// Total: 5 knowledge_quality findings.
	kqIssues, ok := summary["knowledge_quality_issues"]
	if !ok {
		t.Fatal("expected 'knowledge_quality_issues' in summary")
	}
	if kqIssues != float64(5) {
		t.Errorf("knowledge_quality_issues = %v, want 5", kqIssues)
	}

	// Verify knowledge store summaries are present.
	ksRaw, ok := summary["knowledge_stores"]
	if !ok {
		t.Fatal("expected 'knowledge_stores' in summary")
	}
	ksList, ok := ksRaw.([]any)
	if !ok {
		t.Fatalf("knowledge_stores is not an array: %T", ksRaw)
	}
	if len(ksList) != 1 {
		t.Fatalf("expected 1 knowledge store summary, got %d", len(ksList))
	}

	ks, ok := ksList[0].(map[string]any)
	if !ok {
		t.Fatal("knowledge store entry is not map[string]any")
	}
	if ks["name"] != "test-store" {
		t.Errorf("store name = %v, want 'test-store'", ks["name"])
	}
	if ks["file_count"] != float64(4) {
		t.Errorf("file_count = %v, want 4", ks["file_count"])
	}

	// Verify confidence distribution.
	confCounts, ok := ks["confidence_counts"].(map[string]any)
	if !ok {
		t.Fatal("expected confidence_counts map")
	}
	if confCounts["high"] != float64(1) {
		t.Errorf("high confidence = %v, want 1", confCounts["high"])
	}
	if confCounts["medium"] != float64(1) {
		t.Errorf("medium confidence = %v, want 1", confCounts["medium"])
	}
	if confCounts["low"] != float64(1) {
		t.Errorf("low confidence = %v, want 1", confCounts["low"])
	}
	if confCounts["flagged"] != float64(1) {
		t.Errorf("flagged confidence = %v, want 1", confCounts["flagged"])
	}

	// Verify quality flag type counts.
	flagTypes, ok := ks["quality_flag_types"].(map[string]any)
	if !ok {
		t.Fatal("expected quality_flag_types map")
	}
	if flagTypes["missing_rationale"] != float64(1) {
		t.Errorf("missing_rationale = %v, want 1", flagTypes["missing_rationale"])
	}
	if flagTypes["implied_assumption"] != float64(1) {
		t.Errorf("implied_assumption = %v, want 1", flagTypes["implied_assumption"])
	}
	if flagTypes["unsupported_claim"] != float64(1) {
		t.Errorf("unsupported_claim = %v, want 1", flagTypes["unsupported_claim"])
	}

	// Verify findings include knowledge_quality type.
	findings, _ := parsed["findings"].([]any)
	kqFindings := 0
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "knowledge_quality" {
			kqFindings++
		}
	}
	if kqFindings != 5 {
		t.Errorf("knowledge_quality findings = %d, want 5", kqFindings)
	}
}

// TestLint_StaleStore verifies that lint detects stale knowledge stores
// where sources have been updated since the last curation checkpoint.
func TestLint_StaleStore(t *testing.T) {
	s := newTestStore(t)

	// Set up vault directory structure.
	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	// Create a knowledge store config.
	storePath := filepath.Join(deweyDir, "knowledge", "team-decisions")

	writeKnowledgeStoresConfig(t, deweyDir, []curate.StoreConfig{
		{Name: "team-decisions", Sources: []string{"disk-meetings"}, Path: storePath},
	})

	// Write a curation checkpoint with an old timestamp.
	oldCheckpoint := curate.CurationState{
		LastCuratedAt: time.Now().Add(-24 * time.Hour),
		SourceCheckpoints: map[string]time.Time{
			"disk-meetings": time.Now().Add(-24 * time.Hour),
		},
	}
	if err := curate.SaveCurationState(oldCheckpoint, storePath); err != nil {
		t.Fatalf("save curation state: %v", err)
	}

	// Insert a source page that was updated AFTER the checkpoint.
	page := &store.Page{
		Name:         "meeting-notes/sprint-42",
		OriginalName: "Sprint 42",
		SourceID:     "disk-meetings",
		ContentHash:  "abc",
		UpdatedAt:    time.Now().UnixMilli(), // Updated now — after checkpoint
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	lint := NewLint(s, nil, vaultDir, 0)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// Verify stale knowledge store is detected.
	staleKS, ok := summary["stale_knowledge_stores"]
	if !ok {
		t.Fatal("expected 'stale_knowledge_stores' in summary")
	}
	if staleKS != float64(1) {
		t.Errorf("stale_knowledge_stores = %v, want 1", staleKS)
	}

	// Verify the finding details.
	findings, _ := parsed["findings"].([]any)
	foundStale := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "stale_knowledge" {
			foundStale = true
			desc, _ := finding["description"].(string)
			if !strings.Contains(desc, "team-decisions") {
				t.Errorf("description = %q, should mention store name", desc)
			}
			if !strings.Contains(desc, "disk-meetings") {
				t.Errorf("description = %q, should mention source name", desc)
			}
			remediation, _ := finding["remediation"].(string)
			if !strings.Contains(remediation, "dewey curate") {
				t.Errorf("remediation = %q, should mention 'dewey curate'", remediation)
			}
		}
	}
	if !foundStale {
		t.Error("expected a stale_knowledge finding")
	}
}

// TestLint_StaleStore_NeverCurated verifies that lint detects stores that
// have never been curated but have source content available.
func TestLint_StaleStore_NeverCurated(t *testing.T) {
	s := newTestStore(t)

	// Set up vault directory structure.
	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	// Create a knowledge store config — no curation state file exists.
	storePath := filepath.Join(deweyDir, "knowledge", "new-store")

	writeKnowledgeStoresConfig(t, deweyDir, []curate.StoreConfig{
		{Name: "new-store", Sources: []string{"disk-local"}, Path: storePath},
	})

	// Insert a source page so there's content to curate.
	page := &store.Page{
		Name:         "test-doc",
		OriginalName: "Test Doc",
		SourceID:     "disk-local",
		ContentHash:  "abc",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	lint := NewLint(s, nil, vaultDir, 0)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	staleKS, ok := summary["stale_knowledge_stores"]
	if !ok {
		t.Fatal("expected 'stale_knowledge_stores' in summary")
	}
	if staleKS != float64(1) {
		t.Errorf("stale_knowledge_stores = %v, want 1", staleKS)
	}

	// Verify the finding mentions "never been curated".
	findings, _ := parsed["findings"].([]any)
	foundNeverCurated := false
	for _, f := range findings {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "stale_knowledge" {
			desc, _ := finding["description"].(string)
			if strings.Contains(desc, "never been curated") {
				foundNeverCurated = true
			}
		}
	}
	if !foundNeverCurated {
		t.Error("expected a 'never been curated' finding")
	}
}

// TestLint_NoKnowledgeStores verifies that lint produces no knowledge
// store findings when no stores are configured.
func TestLint_NoKnowledgeStores(t *testing.T) {
	s := newTestStore(t)

	// Set up vault directory without knowledge-stores.yaml.
	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	lint := NewLint(s, nil, vaultDir, 0)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// No knowledge store metrics should appear.
	if _, ok := summary["knowledge_quality_issues"]; ok {
		t.Error("knowledge_quality_issues should not be present when no stores configured")
	}
	if _, ok := summary["stale_knowledge_stores"]; ok {
		t.Error("stale_knowledge_stores should not be present when no stores configured")
	}
	if _, ok := summary["knowledge_stores"]; ok {
		t.Error("knowledge_stores should not be present when no stores configured")
	}
}

// TestLint_KnowledgeStoreEmptyVaultPath verifies that lint skips knowledge
// store checks when vaultPath is empty.
func TestLint_KnowledgeStoreEmptyVaultPath(t *testing.T) {
	s := newTestStore(t)
	lint := NewLint(s, nil, "", 0) // empty vaultPath

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}

	parsed := parseLintResult(t, resultText(result))
	summary := lintSummary(t, parsed)

	// No knowledge store metrics should appear.
	if _, ok := summary["knowledge_quality_issues"]; ok {
		t.Error("knowledge_quality_issues should not be present with empty vaultPath")
	}
	if _, ok := summary["stale_knowledge_stores"]; ok {
		t.Error("stale_knowledge_stores should not be present with empty vaultPath")
	}
}

// --- Auto-Indexing Tests (T038) ---

// TestAutoIndex_KnowledgeStoreRegistered verifies that after curation,
// knowledge store files are auto-indexed into the store with the correct
// source ID and tier.
func TestAutoIndex_KnowledgeStoreRegistered(t *testing.T) {
	s := newTestStore(t)

	// Set up vault directory structure.
	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	// Create a knowledge store config.
	storePath := filepath.Join(deweyDir, "knowledge", "test-store")

	writeKnowledgeStoresConfig(t, deweyDir, []curate.StoreConfig{
		{Name: "test-store", Sources: []string{"disk-local"}, Path: storePath},
	})

	// Write curated knowledge files.
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)
	writeKnowledgeFile(t, storePath, "deploy", 1, "medium", []string{"missing_rationale"})

	// Create a Curate tool and run auto-indexing.
	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}
	indexed := curateTool.autoIndexKnowledgeStore(cfg)

	if indexed != 2 {
		t.Errorf("indexed = %d, want 2", indexed)
	}

	// Verify pages are in the store with correct source ID.
	sourceID := "knowledge-test-store"
	pages, err := s.ListPagesBySource(sourceID)
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages with source %q, got %d", sourceID, len(pages))
	}

	// Verify page properties.
	for _, p := range pages {
		if p.SourceID != sourceID {
			t.Errorf("page %q source_id = %q, want %q", p.Name, p.SourceID, sourceID)
		}
		if p.Tier != "curated" {
			t.Errorf("page %q tier = %q, want 'curated'", p.Name, p.Tier)
		}
		if !strings.HasPrefix(p.Name, "knowledge/test-store/") {
			t.Errorf("page name %q should start with 'knowledge/test-store/'", p.Name)
		}
	}

	// Verify blocks were persisted.
	for _, p := range pages {
		blocks, err := s.GetBlocksByPage(p.Name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", p.Name, err)
		}
		if len(blocks) == 0 {
			t.Errorf("page %q has no blocks", p.Name)
		}
	}
}

// TestAutoIndex_SourceIDFormat verifies the knowledge-{name} source ID format.
func TestAutoIndex_SourceIDFormat(t *testing.T) {
	got := knowledgeSourceID("team-decisions")
	want := "knowledge-team-decisions"
	if got != want {
		t.Errorf("knowledgeSourceID = %q, want %q", got, want)
	}

	got = knowledgeSourceID("my-store")
	want = "knowledge-my-store"
	if got != want {
		t.Errorf("knowledgeSourceID = %q, want %q", got, want)
	}
}

// TestAutoIndex_IdempotentReindex verifies that re-running auto-indexing
// with unchanged content doesn't create duplicate pages.
func TestAutoIndex_IdempotentReindex(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	storePath := filepath.Join(deweyDir, "knowledge", "test-store")
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)

	// First indexing.
	indexed1 := curateTool.autoIndexKnowledgeStore(cfg)
	if indexed1 != 1 {
		t.Errorf("first indexing = %d, want 1", indexed1)
	}

	// Second indexing with same content — should skip (content hash match).
	indexed2 := curateTool.autoIndexKnowledgeStore(cfg)
	if indexed2 != 0 {
		t.Errorf("second indexing = %d, want 0 (content unchanged)", indexed2)
	}

	// Verify only one page exists.
	sourceID := "knowledge-test-store"
	pages, err := s.ListPagesBySource(sourceID)
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}
}

// TestAutoIndex_UpdatedContentReindexed verifies that when a knowledge file's
// content changes, it is re-indexed with updated blocks.
func TestAutoIndex_UpdatedContentReindexed(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	storePath := filepath.Join(deweyDir, "knowledge", "test-store")
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)

	// First indexing.
	indexed1 := curateTool.autoIndexKnowledgeStore(cfg)
	if indexed1 != 1 {
		t.Errorf("first indexing = %d, want 1", indexed1)
	}

	// Modify the file content.
	writeKnowledgeFile(t, storePath, "auth", 1, "medium", []string{"missing_rationale"})

	// Second indexing — should re-index because content changed.
	indexed2 := curateTool.autoIndexKnowledgeStore(cfg)
	if indexed2 != 1 {
		t.Errorf("second indexing = %d, want 1 (content changed)", indexed2)
	}

	// Verify the page was updated (still only one page).
	sourceID := "knowledge-test-store"
	pages, err := s.ListPagesBySource(sourceID)
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}
}

// TestAutoIndex_CuratedTier verifies that auto-indexed pages have tier "curated".
func TestAutoIndex_CuratedTier(t *testing.T) {
	s := newTestStore(t)

	vaultDir := t.TempDir()
	deweyDir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	storePath := filepath.Join(deweyDir, "knowledge", "test-store")
	writeKnowledgeFile(t, storePath, "auth", 1, "high", nil)

	cfg := curate.StoreConfig{
		Name:    "test-store",
		Sources: []string{"disk-local"},
		Path:    storePath,
	}

	curateTool := NewCurate(s, nil, nil, vaultDir, nil, 0)
	curateTool.autoIndexKnowledgeStore(cfg)

	// Verify the page has tier "curated".
	page, err := s.GetPage("knowledge/test-store/auth-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page to exist")
	}
	if page.Tier != "curated" {
		t.Errorf("page tier = %q, want 'curated'", page.Tier)
	}
}

// TestLint_ParsesCurateRenderedQuotedFrontmatter covers the write→read
// seam between curate's renderer (%q-quoted frontmatter scalars) and
// lint's manual parser (unquoteScalar). Council round 4 found the quoted
// path had zero coverage — every other lint fixture writes unquoted
// frontmatter, so a regression here would silently misparse every new
// curated file (PR 145).
func TestLint_ParsesCurateRenderedQuotedFrontmatter(t *testing.T) {
	kf := curate.KnowledgeFile{
		Tag:        "authentication",
		Category:   "decision",
		Confidence: "high",
		CreatedAt:  "2026-11-04T17:00:00Z",
		StoreName:  "test-store",
		Tier:       "curated",
		QualityFlags: []curate.QualityFlag{
			{Type: "missing_rationale", Detail: "no rationale given"},
		},
		Content: "Body text for the curated knowledge item.",
	}
	path, err := curate.WriteKnowledgeFile(kf, t.TempDir(), 1)
	if err != nil {
		t.Fatalf("WriteKnowledgeFile: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The rendered frontmatter must quote its scalars; if this ever
	// changes, the seam this test covers has moved.
	if !strings.Contains(string(content), `category: "decision"`) {
		t.Errorf("fixture is not exercising the quoted-scalar path:\n%s", content)
	}

	fm, err := parseKnowledgeFrontmatter(string(content))
	if err != nil {
		t.Fatalf("parseKnowledgeFrontmatter: %v", err)
	}
	if fm.Tag != "authentication" {
		t.Errorf("tag = %q, want authentication", fm.Tag)
	}
	if fm.Category != "decision" {
		t.Errorf("category = %q, want decision without quote marks", fm.Category)
	}
	if fm.Confidence != "high" {
		t.Errorf("confidence = %q, want high without quote marks", fm.Confidence)
	}
	if fm.Tier != "curated" {
		t.Errorf("tier = %q, want curated without quote marks", fm.Tier)
	}
	if len(fm.QualityFlags) != 1 || fm.QualityFlags[0].Type != "missing_rationale" {
		t.Errorf("quality flags = %+v, want one missing_rationale", fm.QualityFlags)
	}
}

// seedCompiledForLint inserts a compiled article page directly into the
// store, mirroring what compile and store_compiled persist (SourceID
// "compiled"): page properties carry the declared sources, blocks carry
// the article body.
func seedCompiledForLint(t *testing.T, s *store.Store, name string, sources []string, content string) {
	t.Helper()
	props := map[string]any{
		"tag":        strings.TrimPrefix(name, "compiled/"),
		"created_at": "2026-06-01T00:00:00Z",
	}
	if len(sources) > 0 {
		props["sources"] = sources
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	page := &store.Page{
		Name:         name,
		OriginalName: name,
		SourceID:     "compiled",
		SourceDocID:  name,
		Properties:   string(propsJSON),
		Tier:         "draft",
		CreatedAt:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
		UpdatedAt:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage(%s): %v", name, err)
	}
	block := &store.Block{UUID: "block-" + name, PageName: name, Content: content, Position: 0}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock(%s): %v", block.UUID, err)
	}
}

// citationCoverageFindings extracts the citation_coverage findings from a
// parsed lint result, keyed by their description.
func citationCoverageFindings(t *testing.T, parsed map[string]any) []map[string]any {
	t.Helper()
	raw, _ := parsed["findings"].([]any)
	var out []map[string]any
	for _, f := range raw {
		finding, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if finding["type"] == "citation_coverage" {
			out = append(out, finding)
		}
	}
	return out
}

// TestLint_CitationCoverage_Findings verifies the citation_coverage kind
// over a store-backed compiled article (ES-FR-011): unresolved citations
// and uncited sources are named by identity, the article is named in the
// description, a superseded source is excused when uncited, and the
// summary carries the counter.
func TestLint_CitationCoverage_Findings(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	lint := NewLint(s, nil, "", 0)

	storeLearningWithProps(t, s, "auth-20260101T000000-alice", `{"tag":"auth","created_at":"2026-01-01T00:00:00Z"}`, "first", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260102T000000-alice", `{"tag":"auth","created_at":"2026-01-02T00:00:00Z"}`, "second", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260303T000000-alice", `{"tag":"auth","created_at":"2026-03-03T00:00:00Z","superseded_by":"auth-20260404T000000-alice","validity":"superseded"}`, "third", time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC))

	// The body cites the first source and an undeclared learning; the
	// second source is current but never cited; the third is superseded
	// and named in the History table, which accounts for it.
	body := "# Auth\n\nUse tokens [auth-20260101T000000-alice] and see [auth-20260909T000000-alice].\n\n## History\n\n| Learning | Created |\n| --- | --- |\n| auth-20260303T000000-alice | 2026-03-03 |\n"
	seedCompiledForLint(t, s, "compiled/auth", []string{
		"auth-20260101T000000-alice",
		"auth-20260102T000000-alice",
		"auth-20260303T000000-alice",
	}, body)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	parsed := parseLintResult(t, resultText(result))

	// The structured outcome reports a checked pass over the article.
	outcome, ok := parsed["citation_coverage_checks"].(map[string]any)
	if !ok {
		t.Fatalf("result missing citation_coverage_checks outcome: %v", parsed)
	}
	if outcome["status"] != "checked" {
		t.Errorf("outcome status = %v, want checked", outcome["status"])
	}
	if checked, _ := outcome["checked"].(float64); checked != 1 {
		t.Errorf("outcome checked = %v, want 1", outcome["checked"])
	}
	if skipped, present := outcome["skipped"]; present {
		t.Errorf("outcome skipped = %v, want absent on a clean pass", skipped)
	}

	findings := citationCoverageFindings(t, parsed)
	if len(findings) != 2 {
		t.Fatalf("expected 2 citation_coverage findings (unresolved + uncited), got %d: %v", len(findings), findings)
	}

	var unresolved, uncited map[string]any
	for _, f := range findings {
		desc, _ := f["description"].(string)
		switch {
		case strings.Contains(desc, "not among its declared sources"):
			unresolved = f
		case strings.Contains(desc, "uncited"):
			uncited = f
		}
	}
	if unresolved == nil || uncited == nil {
		t.Fatalf("expected one unresolved-citations and one uncited-sources finding, got %v", findings)
	}

	if got := identitiesOf(t, unresolved); !equalStrings(got, []string{"auth-20260909T000000-alice"}) {
		t.Errorf("unresolved identities = %v, want [auth-20260909T000000-alice]", got)
	}
	if got := identitiesOf(t, uncited); !equalStrings(got, []string{"auth-20260102T000000-alice"}) {
		t.Errorf("uncited identities = %v, want [auth-20260102T000000-alice] (superseded source excused)", got)
	}
	for _, f := range findings {
		if f["page"] != "compiled/auth" {
			t.Errorf("finding page = %v, want compiled/auth", f["page"])
		}
		desc, _ := f["description"].(string)
		if !strings.Contains(desc, "auth-20260102T000000-alice") && !strings.Contains(desc, "auth-20260909T000000-alice") {
			t.Errorf("description = %q, should name the violations", desc)
		}
	}

	summary := lintSummary(t, parsed)
	if summary["citation_coverage"] != float64(2) {
		t.Errorf("summary citation_coverage = %v, want 2", summary["citation_coverage"])
	}
}

// TestLint_CitationCoverage_CleanArticle verifies an article whose
// citations all resolve and whose current sources are each cited produces
// no findings — an uncited superseded source is accounted for.
func TestLint_CitationCoverage_CleanArticle(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	lint := NewLint(s, nil, "", 0)

	storeLearningWithProps(t, s, "auth-20260101T000000-alice", `{"tag":"auth","created_at":"2026-01-01T00:00:00Z"}`, "first", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260102T000000-alice", `{"tag":"auth","created_at":"2026-01-02T00:00:00Z"}`, "second", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260303T000000-alice", `{"tag":"auth","created_at":"2026-03-03T00:00:00Z","superseded_by":"auth-20260404T000000-alice","validity":"superseded"}`, "third", time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC))

	body := "# Auth\n\nUse tokens [auth-20260101T000000-alice] and rotate [auth-20260102T000000-alice].\n\n## History\n\n| Learning | Created |\n| --- | --- |\n| auth-20260303T000000-alice | 2026-03-03 |\n"
	seedCompiledForLint(t, s, "compiled/auth", []string{
		"auth-20260101T000000-alice",
		"auth-20260102T000000-alice",
		"auth-20260303T000000-alice",
	}, body)

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	parsed := parseLintResult(t, resultText(result))

	if findings := citationCoverageFindings(t, parsed); len(findings) != 0 {
		t.Errorf("expected no citation_coverage findings, got %v", findings)
	}
	summary := lintSummary(t, parsed)
	if _, ok := summary["citation_coverage"]; ok {
		t.Errorf("summary should not carry citation_coverage when clean, got %v", summary["citation_coverage"])
	}
}

// TestLint_CitationCoverage_StoreCompiledAuthored verifies the kind covers
// articles written by store_compiled, not only by compile.
func TestLint_CitationCoverage_StoreCompiledAuthored(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	storeLearningWithProps(t, s, "auth-20260101T000000-alice", `{"tag":"auth","created_at":"2026-01-01T00:00:00Z"}`, "first", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260102T000000-alice", `{"tag":"auth","created_at":"2026-01-02T00:00:00Z"}`, "second", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	c := NewCompile(s, nil, nil, t.TempDir(), nil, 0)
	input := types.StoreCompiledInput{
		Tag:     "authentication",
		Content: "# Authentication\n\nTokens [auth-20260101T000000-alice] rotate hourly.",
		Sources: []string{"auth-20260101T000000-alice", "auth-20260102T000000-alice"},
	}
	result, _, err := c.StoreCompiled(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("StoreCompiled: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreCompiled returned error: %s", resultText(result))
	}

	lint := NewLint(s, nil, "", 0)
	lintResult, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	parsed := parseLintResult(t, resultText(lintResult))

	findings := citationCoverageFindings(t, parsed)
	if len(findings) != 1 {
		t.Fatalf("expected 1 citation_coverage finding, got %d: %v", len(findings), findings)
	}
	if got := identitiesOf(t, findings[0]); !equalStrings(got, []string{"auth-20260102T000000-alice"}) {
		t.Errorf("uncited identities = %v, want [auth-20260102T000000-alice]", got)
	}
	if findings[0]["page"] != "compiled/authentication" {
		t.Errorf("finding page = %v, want compiled/authentication", findings[0]["page"])
	}
}

// TestLint_CitationCoverage_PreChangeArticleNotClearedByFix verifies the
// legacy-article scenario (ES-FR-011): a pre-change article with no
// citations is reported as intended signal, a file-only hand edit to
// .uf/dewey/compiled/ is invisible to the kind, and fix never repairs or
// removes the findings.
func TestLint_CitationCoverage_PreChangeArticleNotClearedByFix(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	storeLearningWithProps(t, s, "auth-20260101T000000-alice", `{"tag":"auth","created_at":"2026-01-01T00:00:00Z"}`, "first", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, "auth-20260102T000000-alice", `{"tag":"auth","created_at":"2026-01-02T00:00:00Z"}`, "second", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// Pre-change article: declared sources, no citations anywhere.
	body := "# Legacy\n\nUse basic auth.\n\n## History\n\n| Learning | Created |\n| --- | --- |\n"
	seedCompiledForLint(t, s, "compiled/legacy", []string{
		"auth-20260101T000000-alice",
		"auth-20260102T000000-alice",
	}, body)

	// A file-only hand edit on disk fixes the article — but compiled
	// articles have no re-ingestion path, so the store is what lint sees.
	vaultPath := t.TempDir()
	dir := filepath.Join(vaultPath, ".uf", "dewey", "compiled")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fixed := "# Legacy\n\n[auth-20260101T000000-alice] [auth-20260102T000000-alice]\n"
	if err := os.WriteFile(filepath.Join(dir, "legacy.md"), []byte(fixed), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	lint := NewLint(s, nil, vaultPath, 0)
	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{Fix: true})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	parsed := parseLintResult(t, resultText(result))

	findings := citationCoverageFindings(t, parsed)
	if len(findings) != 1 {
		t.Fatalf("expected 1 citation_coverage finding (both sources uncited), got %d: %v", len(findings), findings)
	}
	if got := identitiesOf(t, findings[0]); !equalStrings(got, []string{"auth-20260101T000000-alice", "auth-20260102T000000-alice"}) {
		t.Errorf("uncited identities = %v, want both declared sources (store version, hand edit invisible)", got)
	}
	// A citation-free article is one informational finding, not a
	// warning per source, so a pre-upgrade vault's first lint does not
	// bury drift and contradiction findings.
	if findings[0]["severity"] != "info" {
		t.Errorf("severity = %v, want info for a citation-free article", findings[0]["severity"])
	}
	if desc, _ := findings[0]["description"].(string); !strings.Contains(desc, "no per-claim citations") {
		t.Errorf("description = %q, want the no-citations wording", desc)
	}

	// fix never touches citation coverage: no repair path exists for it.
	fixedMap, ok := parsed["fixed"].(map[string]any)
	if !ok {
		t.Fatal("expected 'fixed' map in lint result with fix=true")
	}
	if _, ok := fixedMap["citation_coverage"]; ok {
		t.Errorf("fixed map must not carry citation_coverage: %v", fixedMap)
	}

	summary := lintSummary(t, parsed)
	if summary["citation_coverage"] != float64(1) {
		t.Errorf("summary citation_coverage = %v, want 1", summary["citation_coverage"])
	}
}

// TestLint_ContradictionRemediationNamesSupersede verifies the
// contradiction remediation text points at the declared-supersession
// workflow instead of the removed temporal merge.
func TestLint_ContradictionRemediationNamesSupersede(t *testing.T) {
	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	e := newMockEmbedder(true)
	lint := NewLint(s, e, "", 0)

	now := time.Now()
	storeLearningForLint(t, s, "auth", 1, "decision", "Use basic auth", now)
	storeLearningForLint(t, s, "auth", 2, "decision", "Switch to OAuth", now.Add(24*time.Hour))

	result, _, err := lint.Lint(context.Background(), nil, types.LintInput{})
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	parsed := parseLintResult(t, resultText(result))

	raw, _ := parsed["findings"].([]any)
	found := false
	for _, f := range raw {
		finding, ok := f.(map[string]any)
		if !ok || finding["type"] != "contradiction" {
			continue
		}
		found = true
		remediation, _ := finding["remediation"].(string)
		if !strings.Contains(remediation, "supersede_learning") {
			t.Errorf("remediation = %q, should name supersede_learning", remediation)
		}
		if strings.Contains(remediation, "temporal merge") {
			t.Errorf("remediation = %q, must not reference temporal merge", remediation)
		}
	}
	if !found {
		t.Fatal("expected a contradiction finding")
	}
}

// identitiesOf extracts a finding's identities as a string slice.
func identitiesOf(t *testing.T, finding map[string]any) []string {
	t.Helper()
	raw, _ := finding["identities"].([]any)
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		if s, ok := id.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// equalStrings compares two string slices for exact equality.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
