package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// parsePromoteResult unmarshals the JSON text from a promote CallToolResult.
func parsePromoteResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal promote result: %v\ntext: %s", err, text)
	}
	return parsed
}

// promoteDraftContent is the on-disk shape of a draft compiled page.
const promoteDraftContent = "---\ntier: draft\n---\n\n# Title\n\nBody.\n"

// newPromoteVault creates a vault root whose pages exist on disk as given:
// pages maps a vault-relative slash path to file content.
func newPromoteVault(t *testing.T, pages map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range pages {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	return dir
}

// readVaultFile returns the current content of a vault-relative file.
func readVaultFile(t *testing.T, vaultDir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", rel, err)
	}
	return string(data)
}

// TestPromote_NilStore verifies that a nil store returns an error result
// mentioning persistent storage.
func TestPromote_NilStore(t *testing.T) {
	p := NewPromote(nil, "")

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/auth-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}
	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestPromote_Success verifies the happy path (PW-FR-001): promoting a
// draft compiled page writes `tier: validated` into the file's frontmatter
// on the current branch, leaves the rest of the file byte-preserved, and
// never touches the database tier (PW-FR-002 — pages.tier is a cache
// recomputed on the next index).
func TestPromote_Success(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{"compiled/authentication.md": promoteDraftContent})
	insertPromotePage(t, s, "compiled/authentication", "compiled", "authentication", "draft")
	p := NewPromote(s, vaultDir)

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/authentication",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parsePromoteResult(t, resultText(result))

	// Verify response fields.
	if parsed["page"] != "compiled/authentication" {
		t.Errorf("page = %v, want 'compiled/authentication'", parsed["page"])
	}
	if parsed["previous_tier"] != "draft" {
		t.Errorf("previous_tier = %v, want 'draft'", parsed["previous_tier"])
	}
	if parsed["new_tier"] != "validated" {
		t.Errorf("new_tier = %v, want 'validated'", parsed["new_tier"])
	}
	if parsed["file"] != "compiled/authentication.md" {
		t.Errorf("file = %v, want 'compiled/authentication.md'", parsed["file"])
	}
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "promoted to validated tier") {
		t.Errorf("message = %q, should mention 'promoted to validated tier'", msg)
	}
	if !strings.Contains(msg, "pull request") {
		t.Errorf("message = %q, should direct the human to the pull-request flow", msg)
	}

	// PW-FR-001: the file on disk now says tier: validated, and every other
	// frontmatter line plus the body is preserved byte-for-byte.
	content := readVaultFile(t, vaultDir, "compiled/authentication.md")
	want := "---\ntier: validated\n---\n\n# Title\n\nBody.\n"
	if content != want {
		t.Errorf("file content = %q, want %q", content, want)
	}

	// PW-FR-002: the database tier is untouched — still draft until the
	// next index re-derives it from the git rule.
	storedPage, err := s.GetPage("compiled/authentication")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage.Tier != "draft" {
		t.Errorf("stored tier = %q, want 'draft' (promote must not write the DB tier)", storedPage.Tier)
	}
}

// TestPromote_CreatesFrontmatterWhenAbsent verifies the file-edit shapes
// for files with no frontmatter and frontmatter without a tier key
// (PW-FR-001: `tier: validated` is written into the frontmatter, creating
// the block when absent).
func TestPromote_CreatesFrontmatterWhenAbsent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no frontmatter",
			content: "# Just a body\n",
			want:    "---\ntier: validated\n---\n# Just a body\n",
		},
		{
			name:    "frontmatter without tier key",
			content: "---\ntag: auth\n---\n\nBody.\n",
			want:    "---\ntier: validated\ntag: auth\n---\n\nBody.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			vaultDir := newPromoteVault(t, map[string]string{"compiled/auth.md": tt.content})
			insertPromotePage(t, s, "compiled/auth", "compiled", "auth", "draft")

			result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "compiled/auth"})
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected success, got error: %s", resultText(result))
			}
			if got := readVaultFile(t, vaultDir, "compiled/auth.md"); got != tt.want {
				t.Errorf("file content = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPromote_VaultPathUnresolvable verifies that a resolvable draft target
// with no configured vault root fails before any file edit.
func TestPromote_VaultPathUnresolvable(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/auth", "compiled", "auth", "draft")

	result, _, err := NewPromote(s, "").Promote(context.Background(), nil, types.PromoteInput{Page: "compiled/auth"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for unresolvable vault path")
	}
	if !strings.Contains(resultText(result), "vault path is not configured") {
		t.Errorf("error = %q, want vault-path error", resultText(result))
	}
}

// TestPromote_AlreadyValidated verifies that promoting an already-validated
// page returns an error.
func TestPromote_AlreadyValidated(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s, "")

	// Insert a validated page.
	page := &store.Page{
		Name:         "learning/auth-1",
		OriginalName: "learning/auth-1",
		SourceID:     "learning",
		SourceDocID:  "learning-auth-1",
		Tier:         "validated",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/auth-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for already-validated page")
	}

	text := resultText(result)
	if !strings.Contains(text, "validated") {
		t.Errorf("error message = %q, should mention 'validated'", text)
	}
	if !strings.Contains(text, "Only 'draft' pages") {
		t.Errorf("error message = %q, should mention 'Only draft pages'", text)
	}
}

// TestPromote_AuthoredPage verifies that promoting an authored page
// returns an error (invariant 2: authored pages must not be promotable).
func TestPromote_AuthoredPage(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s, "")

	// Insert an authored page (default tier).
	page := &store.Page{
		Name:         "specs/001-core",
		OriginalName: "specs/001-core",
		SourceID:     "disk-local",
		SourceDocID:  "specs-001-core",
		Tier:         "authored",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "specs/001-core",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for authored page")
	}

	text := resultText(result)
	if !strings.Contains(text, "authored") {
		t.Errorf("error message = %q, should mention 'authored'", text)
	}
	if !strings.Contains(text, "cannot be promoted") {
		t.Errorf("error message = %q, should mention 'cannot be promoted'", text)
	}
}

// TestPromote_PageNotFound verifies that promoting a non-existent page
// returns an error.
func TestPromote_PageNotFound(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s, "")

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/nonexistent-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for non-existent page")
	}

	text := resultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("error message = %q, should mention 'not found'", text)
	}
	if !strings.Contains(text, "nonexistent-1") {
		t.Errorf("error message = %q, should include the page name", text)
	}
}

// TestPromote_EmptyPageName verifies that an empty page name returns
// an error.
func TestPromote_EmptyPageName(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s, "")

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for empty page name")
	}

	text := resultText(result)
	if !strings.Contains(text, "required") {
		t.Errorf("error message = %q, should mention 'required'", text)
	}
}

// TestPromote_DatabaseRowUntouched verifies the PW-FR-002 contract from
// the store side: a successful promotion changes neither the page's tier
// nor its updated_at — the database tier is a cache recomputed on the next
// index, never written by promote.
func TestPromote_DatabaseRowUntouched(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{"compiled/old-page.md": promoteDraftContent})
	insertPromotePage(t, s, "compiled/old-page", "compiled", "old-page", "draft")
	before, _ := s.GetPage("compiled/old-page")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/old-page",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	after, err := s.GetPage("compiled/old-page")
	if err != nil || after == nil {
		t.Fatalf("GetPage: %v, %v", err, after)
	}
	if after.Tier != "draft" {
		t.Errorf("tier = %q, want draft (promote never writes the DB tier)", after.Tier)
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Errorf("updated_at moved: before=%d after=%d (promote never touches the DB row)", before.UpdatedAt, after.UpdatedAt)
	}
}

// TestPromote_LearningFileNeverPromotable verifies the IP-FR-002/PW-FR-004
// learning rejection: a draft learning page resolves through the ladder but
// is refused at file resolution — learnings stay draft forever, and the
// learning file is not edited.
func TestPromote_LearningFileNeverPromotable(t *testing.T) {
	s := newTestStore(t)
	const learningRel = "learning/auth-20260101T000000-alice.md"
	vaultDir := newPromoteVault(t, map[string]string{learningRel: promoteDraftContent})
	insertPromotePage(t, s, "learning/auth-20260101T000000-alice", "learning", "auth-20260101T000000-alice", "draft")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/auth-20260101T000000-alice",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for learning page")
	}
	text := resultText(result)
	if !strings.Contains(text, "learning pages are never promotable") {
		t.Errorf("error = %q, want the learning-rejection message", text)
	}
	if got := readVaultFile(t, vaultDir, learningRel); got != promoteDraftContent {
		t.Errorf("learning file was modified: %q", got)
	}
}

// --- Batch promotion and page resolution (dewey-distill, #91) ---

func insertPromotePage(t *testing.T, s *store.Store, name, sourceID, sourceDocID, tier string) {
	t.Helper()
	if err := s.InsertPage(&store.Page{
		Name:         name,
		OriginalName: name,
		SourceID:     sourceID,
		SourceDocID:  sourceDocID,
		Tier:         tier,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

func insertPromotePageTier(t *testing.T, s *store.Store, name, tier string) {
	t.Helper()
	if err := s.InsertPage(&store.Page{
		Name:         name,
		OriginalName: name,
		SourceID:     "compiled",
		SourceDocID:  strings.TrimPrefix(name, "compiled/"),
		Tier:         tier,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

// seedPromotableVault inserts a draft store page and its on-disk draft
// file for each vault-relative page name (without .md), returning the
// vault root.
func seedPromotableVault(t *testing.T, s *store.Store, names ...string) string {
	t.Helper()
	pages := make(map[string]string, len(names))
	for _, name := range names {
		pages[name+".md"] = promoteDraftContent
		insertPromotePage(t, s, name, "compiled", strings.TrimPrefix(name, "compiled/"), "draft")
	}
	return newPromoteVault(t, pages)
}

func TestPromote_BareCompiledTag(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/gateway")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "gateway"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}

	if got := readVaultFile(t, vaultDir, "compiled/gateway.md"); !strings.Contains(got, "tier: validated") {
		t.Errorf("file = %q, want tier: validated", got)
	}
	if !strings.Contains(resultText(result), "compiled/gateway") {
		t.Errorf("result should report the resolved page name: %s", resultText(result))
	}
}

func TestPromote_CaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/Gateway")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "gateway"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}

	if got := readVaultFile(t, vaultDir, "compiled/Gateway.md"); !strings.Contains(got, "tier: validated") {
		t.Errorf("file = %q, want tier: validated", got)
	}
}

func TestPromote_KnowledgeSuffix(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{"knowledge/team/authentication.md": promoteDraftContent})
	insertPromotePage(t, s, "knowledge/team/authentication", "knowledge-team", "authentication", "draft")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "authentication"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}
	if got := readVaultFile(t, vaultDir, "knowledge/team/authentication.md"); !strings.Contains(got, "tier: validated") {
		t.Errorf("file = %q, want tier: validated", got)
	}
}

func TestPromote_Ambiguous(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "learning/gateway", "learning", "gateway", "draft")
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	result, _, err := NewPromote(s, "").Promote(context.Background(), nil, types.PromoteInput{Page: "gateway"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected ambiguity error result")
	}
	if !strings.Contains(resultText(result), "ambiguous") {
		t.Errorf("result = %q, want ambiguity", resultText(result))
	}

	// Neither page may be promoted.
	for _, name := range []string{"learning/gateway", "compiled/gateway"} {
		page, _ := s.GetPage(name)
		if page.Tier != "draft" {
			t.Errorf("%s tier = %q, want draft", name, page.Tier)
		}
	}
}

func TestPromote_BatchSuccess(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{
		"compiled/one.md":       promoteDraftContent,
		"compiled/three.md":     promoteDraftContent,
		"knowledge/team/two.md": promoteDraftContent,
		"learning/not-this.md":  promoteDraftContent,
	})
	insertPromotePage(t, s, "compiled/one", "compiled", "one", "draft")
	insertPromotePage(t, s, "knowledge/team/two", "knowledge-team", "two", "draft")
	insertPromotePage(t, s, "compiled/three", "compiled", "three", "draft")
	insertPromotePage(t, s, "learning/not-this", "learning", "not-this", "draft")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{"one", "knowledge/team/two", "compiled/three"},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}

	text := resultText(result)
	for _, want := range []string{`"status": "promoted"`, `"total_promoted": 3`, "compiled/one", "knowledge/team/two", "compiled/three"} {
		if !strings.Contains(text, want) {
			t.Errorf("result missing %q: %s", want, text)
		}
	}
	for _, rel := range []string{"compiled/one.md", "knowledge/team/two.md", "compiled/three.md"} {
		if got := readVaultFile(t, vaultDir, rel); !strings.Contains(got, "tier: validated") {
			t.Errorf("%s = %q, want tier: validated", rel, got)
		}
	}
	// An unrequested learning file must remain untouched.
	if got := readVaultFile(t, vaultDir, "learning/not-this.md"); got != promoteDraftContent {
		t.Errorf("learning/not-this.md was modified: %q", got)
	}
}

func TestPromote_BatchFailClosed(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/one")
	insertPromotePage(t, s, "compiled/two", "compiled", "two", "validated")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{"compiled/one", "compiled/two"},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected batch failure result")
	}
	text := resultText(result)
	if !strings.Contains(text, `"status": "failed"`) || !strings.Contains(text, "compiled/two") {
		t.Errorf("result = %s, want per-page failure", text)
	}

	// The valid draft page's file must not be edited (no partial promotion).
	if got := readVaultFile(t, vaultDir, "compiled/one.md"); got != promoteDraftContent {
		t.Errorf("compiled/one.md was edited: %q, want unchanged", got)
	}
	page, _ := s.GetPage("compiled/one")
	if page.Tier != "draft" {
		t.Errorf("compiled/one tier = %q, want draft (no partial promotion)", page.Tier)
	}
}

func TestPromote_BatchDuplicateDeduped(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/one")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{"one", "one", "compiled/one"},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), `"total_promoted": 1`) {
		t.Errorf("result = %s, want total_promoted 1", resultText(result))
	}
}

func TestPromote_BothFieldsRejected(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/one")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Page:  "compiled/one",
		Pages: []string{"compiled/one"},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error")
	}
	if got := readVaultFile(t, vaultDir, "compiled/one.md"); got != promoteDraftContent {
		t.Errorf("compiled/one.md was edited: %q", got)
	}
}

func TestPromote_BatchEmptyElementRejected(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/one")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{"compiled/one", "  "},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error for blank page name")
	}
	if got := readVaultFile(t, vaultDir, "compiled/one.md"); got != promoteDraftContent {
		t.Errorf("compiled/one.md was edited: %q", got)
	}
}

func TestPromote_StoreErrorNotMasked(t *testing.T) {
	s := newTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	result, _, err := NewPromote(s, "").Promote(context.Background(), nil, types.PromoteInput{Page: "compiled/gateway"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	text := resultText(result)
	if strings.Contains(text, "Page 'compiled/gateway' not found.") {
		t.Errorf("store error was masked as not found: %q", text)
	}
	if !strings.Contains(text, "failed to look up page") {
		t.Errorf("result = %q, want resolution error", text)
	}
}

// TestPromote_StoreCompiledRoundTrip covers the DE-007 write→read seam:
// store_compiled then promote by bare tag resolves the compiled page and
// edits the workspace file store_compiled wrote (.uf/dewey/compiled/).
func TestPromote_StoreCompiledRoundTrip(t *testing.T) {
	s := newTestStore(t)
	tmpDir := t.TempDir()
	compile := NewCompile(s, nil, nil, tmpDir, nil, 0)

	// Declared sources must map to existing learnings (ES-FR-012).
	storeLearningWithProps(t, s, "gateway-20260101T000000-test", `{"tag":"gateway","created_at":"2026-01-01T00:00:00Z"}`, "gateway learning", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	result, _, err := compile.StoreCompiled(context.Background(), nil, types.StoreCompiledInput{
		Tag:     "gateway",
		Content: "# Gateway\n\nCompiled gateway knowledge.",
		Sources: []string{"gateway-20260101T000000-test"},
		Model:   "model-x",
	})
	if err != nil {
		t.Fatalf("StoreCompiled: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreCompiled returned error: %s", resultText(result))
	}

	promoteResult, _, err := NewPromote(s, tmpDir).Promote(context.Background(), nil, types.PromoteInput{Page: "gateway"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if promoteResult.IsError {
		t.Fatalf("Promote returned error: %s", resultText(promoteResult))
	}

	// The workspace file store_compiled wrote carries the promotion.
	parsed := parsePromoteResult(t, resultText(promoteResult))
	if parsed["file"] != ".uf/dewey/compiled/gateway.md" {
		t.Errorf("file = %v, want .uf/dewey/compiled/gateway.md", parsed["file"])
	}
	edited := readVaultFile(t, tmpDir, ".uf/dewey/compiled/gateway.md")
	if !strings.Contains(edited, "tier: validated") {
		t.Errorf("workspace file = %q, want tier: validated", edited)
	}

	// PW-FR-002: the database tier stays a draft cache until the next index.
	page, err := s.GetPage("compiled/gateway")
	if err != nil || page == nil {
		t.Fatalf("GetPage: %v (page=%v)", err, page)
	}
	if page.Tier != "draft" {
		t.Errorf("tier = %q, want draft (promote must not write the DB tier)", page.Tier)
	}
}

// TestPromote_EmptyPagesSlice verifies that an explicitly empty batch is
// rejected instead of silently doing nothing.
func TestPromote_EmptyPagesSlice(t *testing.T) {
	s := newTestStore(t)
	result, _, err := NewPromote(s, "").Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error for empty pages")
	}
	if !strings.Contains(resultText(result), "at least one") {
		t.Errorf("result = %q, want an empty-batch error", resultText(result))
	}
}

// --- Promote internal-store regression tests (mcp-internal-store-access task 3.3) ---

// TestPromote_PromotesInternalPagesByExactName verifies the #91 MCP path
// for compiled pages: promote locates internal store pages by exact name
// and edits their file's frontmatter, reporting the previous and new tier.
// Learning pages, by contrast, are refused — learnings stay draft forever
// (IP-FR-002) even when addressed by exact name.
func TestPromote_PromotesInternalPagesByExactName(t *testing.T) {
	tests := []struct {
		name    string
		page    string
		wantErr string // empty means success
	}{
		{"compiled page", "compiled/gateway", ""},
		{"learning page is refused", "learning/auth-20260101-alice", "learning pages are never promotable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			pages := map[string]string{tt.page + ".md": promoteDraftContent}
			vaultDir := newPromoteVault(t, pages)
			p := NewPromote(s, vaultDir)
			insertPromotePageTier(t, s, tt.page, "draft")

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.page})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}

			if tt.wantErr != "" {
				if !result.IsError {
					t.Fatalf("expected error, got success: %s", resultText(result))
				}
				if !strings.Contains(resultText(result), tt.wantErr) {
					t.Errorf("error = %q, want %q", resultText(result), tt.wantErr)
				}
				if got := readVaultFile(t, vaultDir, tt.page+".md"); got != promoteDraftContent {
					t.Errorf("file was edited: %q", got)
				}
				return
			}

			if result.IsError {
				t.Fatalf("expected success, got error: %s", resultText(result))
			}

			parsed := parsePromoteResult(t, resultText(result))
			if parsed["page"] != tt.page {
				t.Errorf("page = %v, want %q", parsed["page"], tt.page)
			}
			if parsed["previous_tier"] != "draft" {
				t.Errorf("previous_tier = %v, want 'draft'", parsed["previous_tier"])
			}
			if parsed["new_tier"] != "validated" {
				t.Errorf("new_tier = %v, want 'validated'", parsed["new_tier"])
			}
			if got := readVaultFile(t, vaultDir, tt.page+".md"); !strings.Contains(got, "tier: validated") {
				t.Errorf("file = %q, want tier: validated", got)
			}
		})
	}
}

// TestPromote_ShorthandResolution verifies bare-name resolution: when the
// exact name misses and the input contains no "/", promote tries
// "compiled/{name}" then "learning/{name}" — the first existing candidate
// that is promotable wins. A bare name resolving only to a learning page is
// refused (learnings are never promotable, IP-FR-002). When no candidate
// exists the not-found error names the original input (spec: Promote Name
// Resolution; issue #91 records agents calling promote with the bare tag).
func TestPromote_ShorthandResolution(t *testing.T) {
	tests := []struct {
		name     string
		seed     []string // draft pages to seed before the call
		input    string   // page name passed to promote
		wantPage string   // resolved page expected on success; empty means error
		wantErr  string   // expected error fragment on failure
	}{
		{
			name:     "bare name resolves to compiled page",
			seed:     []string{"compiled/gateway"},
			input:    "gateway",
			wantPage: "compiled/gateway",
		},
		{
			name:    "learning-only candidate is refused",
			seed:    []string{"learning/gateway"},
			input:   "gateway",
			wantErr: "learning pages are never promotable",
		},
		{
			name:  "no candidate reports the original name as not found",
			seed:  []string{"compiled/unrelated"},
			input: "gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			pages := make(map[string]string, len(tt.seed))
			for _, page := range tt.seed {
				pages[page+".md"] = promoteDraftContent
			}
			vaultDir := newPromoteVault(t, pages)
			p := NewPromote(s, vaultDir)
			for _, page := range tt.seed {
				insertPromotePageTier(t, s, page, "draft")
			}

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.input})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}

			if tt.wantPage == "" && tt.wantErr == "" {
				if !result.IsError {
					t.Fatalf("expected error result, got success: %s", resultText(result))
				}
				text := resultText(result)
				if !strings.Contains(text, "not found") {
					t.Errorf("error message = %q, should mention 'not found'", text)
				}
				if !strings.Contains(text, "'"+tt.input+"'") {
					t.Errorf("error message = %q, should name the original input %q", text, tt.input)
				}
				return
			}

			if tt.wantErr != "" {
				if !result.IsError {
					t.Fatalf("expected error result, got success: %s", resultText(result))
				}
				if !strings.Contains(resultText(result), tt.wantErr) {
					t.Errorf("error = %q, want %q", resultText(result), tt.wantErr)
				}
				return
			}

			if result.IsError {
				t.Fatalf("expected success, got error: %s", resultText(result))
			}
			parsed := parsePromoteResult(t, resultText(result))
			if parsed["page"] != tt.wantPage {
				t.Errorf("page = %v, want %q", parsed["page"], tt.wantPage)
			}
			if got := readVaultFile(t, vaultDir, tt.wantPage+".md"); !strings.Contains(got, "tier: validated") {
				t.Errorf("file = %q, want tier: validated", got)
			}
		})
	}
}

// TestPromote_StoreErrorSurfaces verifies that a failing store lookup is
// reported as a lookup error and never masked as "not found" (spec:
// Promote Error Distinction — the #91 bug class). The underlying database
// is closed after creation so GetPage fails, mirroring the pattern in
// vault.TestGetPage_StoreErrorSurfaces.
func TestPromote_StoreErrorSurfaces(t *testing.T) {
	s := newTestStore(t)
	// Close the underlying database so every lookup fails. The t.Cleanup
	// close registered by newTestStore is a safe no-op — database/sql
	// Close is idempotent.
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	p := NewPromote(s, "")

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/gateway",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when the store lookup fails")
	}

	text := resultText(result)
	if !strings.Contains(text, "failed to look up page") {
		t.Errorf("error message = %q, should mention 'failed to look up page'", text)
	}
	if !strings.Contains(text, "compiled/gateway") {
		t.Errorf("error message = %q, should name the queried page", text)
	}
	if strings.Contains(text, "not found") {
		t.Errorf("error message = %q, a store failure must not be masked as not-found", text)
	}
}

// TestPromote_NonDraftRejectionKeepsTier verifies the tier invariant from
// both directions: authored and validated pages are rejected with the
// tier-invariant error naming the resolved page, and the rejected attempt
// leaves the stored tier unchanged. The rejection also applies to pages
// located via the shorthand (spec: Promote Locates Internal Store Pages,
// non-draft scenario).
func TestPromote_NonDraftRejectionKeepsTier(t *testing.T) {
	tests := []struct {
		name      string
		seed      string // page to seed
		tier      string // tier to seed
		input     string // page name passed to promote
		wantInErr string // resolved page name the error must report
	}{
		{
			name:      "authored page",
			seed:      "specs/001-core",
			tier:      "authored",
			input:     "specs/001-core",
			wantInErr: "specs/001-core",
		},
		{
			name:      "validated page",
			seed:      "compiled/gateway",
			tier:      "validated",
			input:     "compiled/gateway",
			wantInErr: "compiled/gateway",
		},
		{
			name:      "shorthand-resolved authored page",
			seed:      "compiled/gateway",
			tier:      "authored",
			input:     "gateway",
			wantInErr: "compiled/gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			p := NewPromote(s, "")
			insertPromotePageTier(t, s, tt.seed, tt.tier)

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.input})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error result for %q-tier page", tt.tier)
			}

			text := resultText(result)
			if !strings.Contains(text, "cannot be promoted") {
				t.Errorf("error message = %q, should mention 'cannot be promoted'", text)
			}
			if !strings.Contains(text, "'"+tt.tier+"'") {
				t.Errorf("error message = %q, should mention the current tier %q", text, tt.tier)
			}
			if !strings.Contains(text, tt.wantInErr) {
				t.Errorf("error message = %q, should name the resolved page %q", text, tt.wantInErr)
			}

			// The rejected attempt must leave the stored tier untouched.
			stored, err := s.GetPage(tt.seed)
			if err != nil {
				t.Fatalf("GetPage: %v", err)
			}
			if stored == nil {
				t.Fatalf("page %q missing after rejected promote", tt.seed)
			}
			if stored.Tier != tt.tier {
				t.Errorf("stored tier = %q, want unchanged %q", stored.Tier, tt.tier)
			}
		})
	}
}

// --- PW-FR-004 path validation ---

// TestPromote_ArgumentValidationRejected verifies the raw-argument half of
// PW-FR-004: absolute paths and `..` segments in the requested page are
// rejected before any store lookup or filesystem access, and no file is
// modified.
func TestPromote_ArgumentValidationRejected(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "parent traversal", input: "../../.env"},
		{name: "dot-dot only", input: ".."},
		{name: "traversal after a prefix", input: "compiled/evil/../../secrets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			pages := map[string]string{"compiled/one.md": promoteDraftContent, ".env": "SECRET=1\n"}
			vaultDir := newPromoteVault(t, pages)
			insertPromotePageTier(t, s, "compiled/one", "draft")

			result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: tt.input})
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected path-validation error, got success: %s", resultText(result))
			}
			text := resultText(result)
			if !strings.Contains(text, "invalid page path") {
				t.Errorf("error = %q, want a path-validation error", text)
			}
			if !strings.Contains(text, tt.input) {
				t.Errorf("error = %q, should name the rejected argument", text)
			}
			// No file was modified — the outside vault file and the
			// seeded page are both unchanged.
			if got := readVaultFile(t, vaultDir, ".env"); got != "SECRET=1\n" {
				t.Errorf(".env was modified: %q", got)
			}
			if got := readVaultFile(t, vaultDir, "compiled/one.md"); got != promoteDraftContent {
				t.Errorf("compiled/one.md was modified: %q", got)
			}
		})
	}
}

// TestPromote_AbsolutePathRejected verifies the absolute-path rejection on
// its own, including a resolvable path inside the vault.
func TestPromote_AbsolutePathRejected(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/one")

	abs := filepath.Join(vaultDir, "compiled", "one.md")
	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: abs})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(resultText(result), "absolute paths are not allowed") {
		t.Errorf("error = %q, want absolute-path rejection", resultText(result))
	}
	if got := readVaultFile(t, vaultDir, "compiled/one.md"); got != promoteDraftContent {
		t.Errorf("file was modified: %q", got)
	}
}

// TestCheckPromoteRelPath pins the canonicalization half of PW-FR-004:
// after path.Clean, a relative path that escapes the vault ("..") or sits
// outside the promotable prefixes is rejected — the cleaned form must pass
// the same checks as the raw form.
func TestCheckPromoteRelPath(t *testing.T) {
	tests := []struct {
		rel     string
		wantErr string // empty means accepted
	}{
		{rel: "compiled/auth.md", wantErr: ""},
		{rel: "knowledge/team/auth.md", wantErr: ""},
		{rel: ".uf/dewey/compiled/auth.md", wantErr: ""},
		{rel: ".uf/dewey/knowledge/auth.md", wantErr: ""},
		{rel: "compiled.md", wantErr: "only files under compiled/ or knowledge/"},
		{rel: "compiled-notes/auth.md", wantErr: "only files under compiled/ or knowledge/"},
		{rel: "notes/regular.md", wantErr: "only files under compiled/ or knowledge/"},
		{rel: "compiled/../secrets.md", wantErr: "only files under compiled/ or knowledge/"},
		{rel: "compiled/evil/../../secrets.md", wantErr: "only files under compiled/ or knowledge/"},
		{rel: "compiled/../../../etc/passwd", wantErr: "'..' segments are not allowed"},
		{rel: "../secrets.md", wantErr: "'..' segments are not allowed"},
		{rel: "..", wantErr: "'..' segments are not allowed"},
		{rel: "/etc/passwd", wantErr: "absolute paths are not allowed"},
		{rel: "", wantErr: "empty"},
		{rel: ".", wantErr: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			cleaned, err := checkPromoteRelPath(tt.rel)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkPromoteRelPath(%q): %v", tt.rel, err)
				}
				if cleaned == "" || strings.HasPrefix(cleaned, "/") {
					t.Errorf("cleaned = %q, want a relative path", cleaned)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkPromoteRelPath(%q) = %q, want error %q", tt.rel, cleaned, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("checkPromoteRelPath(%q) error = %v, want %q", tt.rel, err, tt.wantErr)
			}
		})
	}
}

// TestPromote_OutsidePrefixRejected verifies that a store page whose only
// files live outside the promotable prefixes is refused with the
// no-promotable-file error, and nothing outside compiled//knowledge/ is
// read or written.
func TestPromote_OutsidePrefixRejected(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{
		"notes/regular.md": promoteDraftContent,
	})
	insertPromotePage(t, s, "notes/regular", "disk-local", "notes/regular.md", "draft")

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "notes/regular"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-promotable prefix")
	}
	if !strings.Contains(resultText(result), "no promotable file found") {
		t.Errorf("error = %q, want no-promotable-file error", resultText(result))
	}
	if got := readVaultFile(t, vaultDir, "notes/regular.md"); got != promoteDraftContent {
		t.Errorf("notes/regular.md was modified: %q", got)
	}
}

// TestPromote_SymlinkEscapeRejected verifies the filesystem half of
// PW-FR-004: a symlink whose resolved form escapes the vault root or the
// allowed prefixes is rejected without following it — neither a
// final-component symlink to an outside file nor an intermediate directory
// symlink outside the vault may direct an edit.
func TestPromote_SymlinkEscapeRejected(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("---\ntier: draft\n---\nsecret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}
	inner := filepath.Join(outside, "inner.md")
	if err := os.WriteFile(inner, []byte(promoteDraftContent), 0o644); err != nil {
		t.Fatalf("WriteFile inner: %v", err)
	}

	s := newTestStore(t)
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Final-component symlink pointing outside the vault.
	if err := os.Symlink(secret, filepath.Join(vaultDir, "compiled", "escape.md")); err != nil {
		t.Fatalf("Symlink final: %v", err)
	}
	// Intermediate-directory symlink pointing outside the vault; its
	// target exists, so resolution reaches the escape check rather than
	// a missing-file skip.
	if err := os.Symlink(outside, filepath.Join(vaultDir, "compiled", "escedir")); err != nil {
		t.Fatalf("Symlink dir: %v", err)
	}
	insertPromotePage(t, s, "compiled/escape", "compiled", "escape", "draft")
	insertPromotePage(t, s, "compiled/escedir/inner", "compiled", "escedir/inner", "draft")

	for _, input := range []string{"compiled/escape", "compiled/escedir/inner"} {
		result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: input})
		if err != nil {
			t.Fatalf("Promote(%s): %v", input, err)
		}
		if !result.IsError {
			t.Fatalf("expected symlink-escape rejection for %s, got: %s", input, resultText(result))
		}
	}

	// No file outside the allowed prefixes was modified.
	for _, path := range []string{secret, inner} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		want := promoteDraftContent
		if path == secret {
			want = "---\ntier: draft\n---\nsecret\n"
		}
		if string(got) != want {
			t.Errorf("outside file %s was modified: %q", path, string(got))
		}
	}
}

// TestPromote_NoFollowFinalComponent pins the TOCTOU half of PW-FR-004 at
// the read seam: after validation, the target is opened with O_NOFOLLOW,
// so a symlink swapped onto the final component is a validation failure
// (ELOOP), never a followed read.
func TestPromote_NoFollowFinalComponent(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	real := filepath.Join(vaultDir, "compiled", "real.md")
	if err := os.WriteFile(real, []byte(promoteDraftContent), 0o644); err != nil {
		t.Fatalf("WriteFile real: %v", err)
	}
	link := filepath.Join(vaultDir, "compiled", "link.md")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// A symlink on the final component of the fully resolved path is
	// refused by the no-follow open even when it points inside the
	// allowed prefixes — the swapped-symlink TOCTOU shape.
	if _, _, err := readPromoteTarget(link); err == nil {
		t.Fatal("expected ELOOP-style error opening a final-component symlink with O_NOFOLLOW")
	}

	// The fully resolved path of a within-prefix alias opens fine — the
	// no-follow guard targets the post-validation swap, not legitimate
	// resolution.
	content, mode, err := readPromoteTarget(real)
	if err != nil {
		t.Fatalf("readPromoteTarget(real): %v", err)
	}
	if string(content) != promoteDraftContent || mode != 0o644 {
		t.Errorf("read = (%q, %v), want the real file content and mode", string(content), mode)
	}
}

// --- PW-FR-001 atomic edit ---

// TestPromote_AtomicFailureLeavesOriginalUnchanged verifies the PW-FR-001
// atomicity contract: when the edit cannot be applied (read-only target
// directory), the command errors and the original file is byte-identical
// to its pre-call content — no partial write.
func TestPromote_AtomicFailureLeavesOriginalUnchanged(t *testing.T) {
	s := newTestStore(t)
	vaultDir := seedPromotableVault(t, s, "compiled/auth")

	// Make the target directory read-only after seeding: the file remains
	// readable (preparation succeeds) but no temp file can be created in
	// the directory (the atomic apply fails).
	dir := filepath.Join(vaultDir, "compiled")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{Page: "compiled/auth"})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for unwritable target directory")
	}
	if got := readVaultFile(t, vaultDir, "compiled/auth.md"); got != promoteDraftContent {
		t.Errorf("file changed after failed edit: %q, want byte-identical %q", got, promoteDraftContent)
	}
}

// TestPromote_BatchApplyFailureRollsBack verifies the batch all-or-nothing
// contract at the apply step: when one edit fails mid-batch, the edits
// already applied are restored from their in-memory originals, so a failed
// batch leaves no promotable file changed.
func TestPromote_BatchApplyFailureRollsBack(t *testing.T) {
	s := newTestStore(t)
	vaultDir := newPromoteVault(t, map[string]string{
		"compiled/good.md":   promoteDraftContent,
		"compiled/ro/bad.md": promoteDraftContent,
	})
	insertPromotePage(t, s, "compiled/good", "compiled", "good", "draft")
	insertPromotePage(t, s, "compiled/ro/bad", "compiled", "ro/bad", "draft")

	// The second target's directory is read-only, so its apply fails after
	// the first target's edit has already landed — exercising the rollback.
	roDir := filepath.Join(vaultDir, "compiled", "ro")
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatalf("Chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	result, _, err := NewPromote(s, vaultDir).Promote(context.Background(), nil, types.PromoteInput{
		Pages: []string{"compiled/good", "compiled/ro/bad"},
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for failed batch apply, got:\n%s", resultText(result))
	}

	parsed := parsePromoteResult(t, resultText(result))
	if parsed["status"] != "failed" {
		t.Errorf("status = %v, want failed", parsed["status"])
	}
	if !strings.Contains(resultText(result), "rolled back") {
		t.Errorf("result = %q, want the rollback report", resultText(result))
	}

	// Both files are unchanged: the failing edit never landed and the
	// already-applied edit was restored.
	for _, rel := range []string{"compiled/good.md", "compiled/ro/bad.md"} {
		if got := readVaultFile(t, vaultDir, rel); got != promoteDraftContent {
			t.Errorf("%s changed after rolled-back batch: %q", rel, got)
		}
	}
}
