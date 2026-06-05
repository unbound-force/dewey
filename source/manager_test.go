package source

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockSource is a test double for the Source interface.
// Thread-safe for concurrent use in tests with -race (FR-101).
type mockSource struct {
	id        string
	srcType   string
	name      string
	docs      []Document
	err       error
	listCalls atomic.Int64

	// listFn is an optional hook called during List(). When set, it is called
	// before returning docs/err. Used for synchronization barriers in
	// concurrency tests.
	listFn func()
}

func (m *mockSource) List() ([]Document, error) {
	m.listCalls.Add(1)
	if m.listFn != nil {
		m.listFn()
	}
	return m.docs, m.err
}

func (m *mockSource) Fetch(id string) (*Document, error) {
	for _, d := range m.docs {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, nil
}

func (m *mockSource) Diff() ([]Change, error) {
	return nil, nil
}

func (m *mockSource) Meta() SourceMetadata {
	return SourceMetadata{
		ID:   m.id,
		Type: m.srcType,
		Name: m.name,
	}
}

// Verify mockSource implements Source at compile time.
var _ Source = (*mockSource)(nil)

func TestManager_FetchAll_MultiSource(t *testing.T) {
	mgr := &Manager{
		sources: []Source{
			&mockSource{
				id:      "disk-local",
				srcType: "disk",
				name:    "local",
				docs: []Document{
					{ID: "page1.md", Title: "Page 1", SourceID: "disk-local"},
					{ID: "page2.md", Title: "Page 2", SourceID: "disk-local"},
				},
			},
			&mockSource{
				id:      "github-test",
				srcType: "github",
				name:    "test",
				docs: []Document{
					{ID: "issue/1", Title: "Issue 1", SourceID: "github-test"},
				},
			},
		},
		configs: []SourceConfig{
			{ID: "disk-local", Type: "disk", Name: "local"},
			{ID: "github-test", Type: "github", Name: "test"},
		},
	}

	result, allDocs := mgr.FetchAll("", false, nil)

	if result.TotalDocs != 3 {
		t.Errorf("total docs = %d, want 3", result.TotalDocs)
	}
	if len(allDocs) != 2 {
		t.Errorf("source count = %d, want 2", len(allDocs))
	}
	if len(allDocs["disk-local"]) != 2 {
		t.Errorf("disk docs = %d, want 2", len(allDocs["disk-local"]))
	}
	if len(allDocs["github-test"]) != 1 {
		t.Errorf("github docs = %d, want 1", len(allDocs["github-test"]))
	}

	// Verify per-source summaries in result.
	if len(result.Summaries) != 2 {
		t.Fatalf("summaries count = %d, want 2", len(result.Summaries))
	}
	for _, s := range result.Summaries {
		switch s.SourceID {
		case "disk-local":
			if s.SourceType != "disk" {
				t.Errorf("disk summary type = %q, want %q", s.SourceType, "disk")
			}
			if s.Documents != 2 {
				t.Errorf("disk summary docs = %d, want 2", s.Documents)
			}
			if s.Errors != 0 {
				t.Errorf("disk summary errors = %d, want 0", s.Errors)
			}
		case "github-test":
			if s.SourceType != "github" {
				t.Errorf("github summary type = %q, want %q", s.SourceType, "github")
			}
			if s.Documents != 1 {
				t.Errorf("github summary docs = %d, want 1", s.Documents)
			}
		default:
			t.Errorf("unexpected source ID in summaries: %q", s.SourceID)
		}
	}

	// Verify documents retain source attribution.
	for _, doc := range allDocs["disk-local"] {
		if doc.SourceID != "disk-local" {
			t.Errorf("disk doc source_id = %q, want %q", doc.SourceID, "disk-local")
		}
	}
	for _, doc := range allDocs["github-test"] {
		if doc.SourceID != "github-test" {
			t.Errorf("github doc source_id = %q, want %q", doc.SourceID, "github-test")
		}
	}

	// Verify no errors or skips.
	if result.TotalErrs != 0 {
		t.Errorf("total errors = %d, want 0", result.TotalErrs)
	}
	if result.TotalSkip != 0 {
		t.Errorf("total skipped = %d, want 0", result.TotalSkip)
	}
}

func TestManager_FetchAll_SourceFilter(t *testing.T) {
	mgr := &Manager{
		sources: []Source{
			&mockSource{id: "disk-local", srcType: "disk", name: "local", docs: []Document{{ID: "1"}}},
			&mockSource{id: "github-test", srcType: "github", name: "test", docs: []Document{{ID: "2"}}},
		},
		configs: []SourceConfig{
			{ID: "disk-local", Type: "disk", Name: "local"},
			{ID: "github-test", Type: "github", Name: "test"},
		},
	}

	result, allDocs := mgr.FetchAll("github-test", false, nil)

	if result.TotalDocs != 1 {
		t.Errorf("total docs = %d, want 1 (only github)", result.TotalDocs)
	}
	if _, ok := allDocs["disk-local"]; ok {
		t.Error("disk source should not be fetched when filtering by github-test")
	}
}

func TestManager_FetchAll_SourceFailureIsolation(t *testing.T) {
	mgr := &Manager{
		sources: []Source{
			&mockSource{
				id:      "disk-local",
				srcType: "disk",
				name:    "local",
				docs:    []Document{{ID: "1"}},
			},
			&mockSource{
				id:      "github-fail",
				srcType: "github",
				name:    "fail",
				err:     fmt.Errorf("network error"),
			},
			&mockSource{
				id:      "web-test",
				srcType: "web",
				name:    "test",
				docs:    []Document{{ID: "3"}},
			},
		},
		configs: []SourceConfig{
			{ID: "disk-local", Type: "disk", Name: "local"},
			{ID: "github-fail", Type: "github", Name: "fail"},
			{ID: "web-test", Type: "web", Name: "test"},
		},
	}

	result, allDocs := mgr.FetchAll("", false, nil)

	// Should have docs from disk and web, but not github.
	if result.TotalDocs != 2 {
		t.Errorf("total docs = %d, want 2 (github failed)", result.TotalDocs)
	}
	if result.TotalErrs != 1 {
		t.Errorf("total errors = %d, want 1", result.TotalErrs)
	}
	if _, ok := allDocs["github-fail"]; ok {
		t.Error("failed source should not have documents")
	}

	// Verify working sources still returned documents despite github failure.
	if len(allDocs["disk-local"]) != 1 {
		t.Errorf("disk docs = %d, want 1 (should not be affected by github failure)", len(allDocs["disk-local"]))
	}
	if len(allDocs["web-test"]) != 1 {
		t.Errorf("web docs = %d, want 1 (should not be affected by github failure)", len(allDocs["web-test"]))
	}

	// Verify per-source summaries record the failure details.
	if len(result.Summaries) != 3 {
		t.Fatalf("summaries count = %d, want 3", len(result.Summaries))
	}
	for _, s := range result.Summaries {
		if s.SourceID == "github-fail" {
			if s.Errors != 1 {
				t.Errorf("failed source summary errors = %d, want 1", s.Errors)
			}
			if s.Error == "" {
				t.Error("failed source summary should include error message")
			}
			if s.Documents != 0 {
				t.Errorf("failed source summary docs = %d, want 0", s.Documents)
			}
		}
	}
}

func TestManager_FetchAll_RefreshInterval(t *testing.T) {
	src := &mockSource{
		id:      "github-test",
		srcType: "github",
		name:    "test",
		docs:    []Document{{ID: "1"}},
	}

	mgr := &Manager{
		sources: []Source{src},
		configs: []SourceConfig{
			{ID: "github-test", Type: "github", Name: "test", RefreshInterval: "daily"},
		},
	}

	// Last fetched 1 hour ago — should be skipped (within daily interval).
	lastFetched := map[string]time.Time{
		"github-test": time.Now().Add(-1 * time.Hour),
	}

	result, _ := mgr.FetchAll("", false, lastFetched)

	if result.TotalSkip != 1 {
		t.Errorf("total skipped = %d, want 1", result.TotalSkip)
	}
	if got := src.listCalls.Load(); got != 0 {
		t.Errorf("List should not be called when within refresh interval, got %d calls", got)
	}
}

func TestManager_FetchAll_ForceIgnoresInterval(t *testing.T) {
	src := &mockSource{
		id:      "github-test",
		srcType: "github",
		name:    "test",
		docs:    []Document{{ID: "1"}},
	}

	mgr := &Manager{
		sources: []Source{src},
		configs: []SourceConfig{
			{ID: "github-test", Type: "github", Name: "test", RefreshInterval: "daily"},
		},
	}

	lastFetched := map[string]time.Time{
		"github-test": time.Now().Add(-1 * time.Hour),
	}

	result, _ := mgr.FetchAll("", true, lastFetched)

	if result.TotalDocs != 1 {
		t.Errorf("total docs = %d, want 1 (force should ignore interval)", result.TotalDocs)
	}
	if got := src.listCalls.Load(); got != 1 {
		t.Errorf("List should be called once when forced, got %d calls", got)
	}
}

func TestCreateDiskSource_ValidConfig(t *testing.T) {
	cfg := SourceConfig{
		ID:   "disk-local",
		Type: "disk",
		Name: "local",
		Config: map[string]any{
			"path": "/some/vault/path",
		},
	}

	src := createDiskSource(cfg, "/fallback/base")
	if src == nil {
		t.Fatal("createDiskSource returned nil for valid config")
	}

	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatalf("expected *DiskSource, got %T", src)
	}
	if ds.id != "disk-local" {
		t.Errorf("id = %q, want %q", ds.id, "disk-local")
	}
	if ds.name != "local" {
		t.Errorf("name = %q, want %q", ds.name, "local")
	}
	if ds.basePath != "/some/vault/path" {
		t.Errorf("basePath = %q, want %q", ds.basePath, "/some/vault/path")
	}
}

func TestCreateDiskSource_FallsBackToBasePath(t *testing.T) {
	cfg := SourceConfig{
		ID:     "disk-local",
		Type:   "disk",
		Name:   "local",
		Config: map[string]any{},
	}

	src := createDiskSource(cfg, "/fallback/base")
	if src == nil {
		t.Fatal("createDiskSource returned nil")
	}

	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("createDiskSource did not return *DiskSource")
	}
	if ds.basePath != "/fallback/base" {
		t.Errorf("basePath = %q, want %q (should fall back to basePath)", ds.basePath, "/fallback/base")
	}
}

func TestCreateDiskSource_DotPathFallsBackToBasePath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "disk-local",
		Type: "disk",
		Name: "local",
		Config: map[string]any{
			"path": ".",
		},
	}

	src := createDiskSource(cfg, "/fallback/base")
	if src == nil {
		t.Fatal("createDiskSource returned nil")
	}

	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("createDiskSource did not return *DiskSource")
	}
	if ds.basePath != "/fallback/base" {
		t.Errorf("basePath = %q, want %q (dot path should resolve to basePath)", ds.basePath, "/fallback/base")
	}
}

func TestCreateDiskSource_NilConfig(t *testing.T) {
	cfg := SourceConfig{
		ID:     "disk-local",
		Type:   "disk",
		Name:   "local",
		Config: nil,
	}

	src := createDiskSource(cfg, "/fallback/base")
	if src == nil {
		t.Fatal("createDiskSource returned nil for nil config")
	}

	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("createDiskSource did not return *DiskSource")
	}
	// With nil Config, path type assertion fails, defaults to ".", then to basePath.
	if ds.basePath != "/fallback/base" {
		t.Errorf("basePath = %q, want %q", ds.basePath, "/fallback/base")
	}
}

func TestCreateGitHubSource_ValidConfig(t *testing.T) {
	// Set token env var to avoid side effects from gh CLI lookup.
	t.Setenv("GITHUB_TOKEN", "test-token")

	cfg := SourceConfig{
		ID:   "github-gaze",
		Type: "github",
		Name: "gaze",
		Config: map[string]any{
			"org":     "unbound-force",
			"repos":   []any{"gaze", "dewey"},
			"content": []any{"issues", "readme"},
		},
	}

	src := createGitHubSource(cfg)
	if src == nil {
		t.Fatal("createGitHubSource returned nil for valid config")
	}

	gs, ok := src.(*GitHubSource)
	if !ok {
		t.Fatalf("expected *GitHubSource, got %T", src)
	}
	if gs.id != "github-gaze" {
		t.Errorf("id = %q, want %q", gs.id, "github-gaze")
	}
	if gs.name != "gaze" {
		t.Errorf("name = %q, want %q", gs.name, "gaze")
	}
	if gs.org != "unbound-force" {
		t.Errorf("org = %q, want %q", gs.org, "unbound-force")
	}
	if len(gs.repos) != 2 {
		t.Fatalf("repos count = %d, want 2", len(gs.repos))
	}
	if gs.repos[0] != "gaze" || gs.repos[1] != "dewey" {
		t.Errorf("repos = %v, want [gaze dewey]", gs.repos)
	}
	if len(gs.contentType) != 2 {
		t.Fatalf("contentType count = %d, want 2", len(gs.contentType))
	}
	if gs.contentType[0] != "issues" || gs.contentType[1] != "readme" {
		t.Errorf("contentType = %v, want [issues readme]", gs.contentType)
	}
}

func TestCreateGitHubSource_DefaultContentTypes(t *testing.T) {
	// Set token env var to avoid side effects from gh CLI lookup.
	t.Setenv("GITHUB_TOKEN", "test-token")

	cfg := SourceConfig{
		ID:   "github-test",
		Type: "github",
		Name: "test",
		Config: map[string]any{
			"org":   "myorg",
			"repos": []any{"repo1"},
		},
	}

	src := createGitHubSource(cfg)
	gs, ok := src.(*GitHubSource)
	if !ok {
		t.Fatal("createGitHubSource did not return *GitHubSource")
	}

	// When no content types specified, NewGitHubSource defaults to all three.
	if len(gs.contentType) != 3 {
		t.Fatalf("contentType count = %d, want 3 (default)", len(gs.contentType))
	}
	expected := []string{"issues", "pulls", "readme"}
	for i, want := range expected {
		if gs.contentType[i] != want {
			t.Errorf("contentType[%d] = %q, want %q", i, gs.contentType[i], want)
		}
	}
}

func TestCreateWebSource_ValidConfig(t *testing.T) {
	cfg := SourceConfig{
		ID:   "web-docs",
		Type: "web",
		Name: "documentation",
		Config: map[string]any{
			"urls":       []any{"https://example.com", "https://docs.example.com"},
			"depth":      2,
			"rate_limit": "500ms",
		},
	}

	src := createWebSource(cfg, "/tmp/cache")
	if src == nil {
		t.Fatal("createWebSource returned nil for valid config")
	}

	ws, ok := src.(*WebSource)
	if !ok {
		t.Fatalf("expected *WebSource, got %T", src)
	}
	if ws.id != "web-docs" {
		t.Errorf("id = %q, want %q", ws.id, "web-docs")
	}
	if ws.name != "documentation" {
		t.Errorf("name = %q, want %q", ws.name, "documentation")
	}
	if len(ws.urls) != 2 {
		t.Fatalf("urls count = %d, want 2", len(ws.urls))
	}
	if ws.urls[0] != "https://example.com" || ws.urls[1] != "https://docs.example.com" {
		t.Errorf("urls = %v, want [https://example.com https://docs.example.com]", ws.urls)
	}
	if ws.depth != 2 {
		t.Errorf("depth = %d, want 2", ws.depth)
	}
	if ws.rateLimit != 500*time.Millisecond {
		t.Errorf("rateLimit = %v, want 500ms", ws.rateLimit)
	}
	if ws.cacheDir != "/tmp/cache" {
		t.Errorf("cacheDir = %q, want %q", ws.cacheDir, "/tmp/cache")
	}
}

func TestCreateWebSource_DepthAsFloat64(t *testing.T) {
	// JSON/YAML unmarshaling may produce float64 for integer values.
	cfg := SourceConfig{
		ID:   "web-test",
		Type: "web",
		Name: "test",
		Config: map[string]any{
			"urls":  []any{"https://example.com"},
			"depth": float64(3),
		},
	}

	src := createWebSource(cfg, "")
	ws, ok := src.(*WebSource)
	if !ok {
		t.Fatal("createWebSource did not return *WebSource")
	}

	if ws.depth != 3 {
		t.Errorf("depth = %d, want 3 (from float64)", ws.depth)
	}
}

func TestCreateWebSource_DefaultDepthAndRateLimit(t *testing.T) {
	cfg := SourceConfig{
		ID:   "web-test",
		Type: "web",
		Name: "test",
		Config: map[string]any{
			"urls": []any{"https://example.com"},
		},
	}

	src := createWebSource(cfg, "")
	ws, ok := src.(*WebSource)
	if !ok {
		t.Fatal("createWebSource did not return *WebSource")
	}

	if ws.depth != 1 {
		t.Errorf("depth = %d, want 1 (default)", ws.depth)
	}
	if ws.rateLimit != defaultRateLimit {
		t.Errorf("rateLimit = %v, want %v (default)", ws.rateLimit, defaultRateLimit)
	}
}

func TestCreateSource_UnknownType(t *testing.T) {
	cfg := SourceConfig{
		ID:     "unknown-src",
		Type:   "ftp",
		Name:   "bad source",
		Config: map[string]any{},
	}

	src := createSource(cfg, "/base", "/cache")
	if src != nil {
		t.Errorf("expected nil for unknown source type, got %T", src)
	}
}

func TestExtractStringList_FromSliceAny(t *testing.T) {
	input := []any{"alpha", "beta", "gamma"}
	got := extractStringList(input)

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExtractStringList_FromSliceAnySkipsNonStrings(t *testing.T) {
	input := []any{"valid", 42, "also-valid", true}
	got := extractStringList(input)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (non-strings skipped)", len(got))
	}
	if got[0] != "valid" || got[1] != "also-valid" {
		t.Errorf("got = %v, want [valid also-valid]", got)
	}
}

func TestExtractStringList_FromString(t *testing.T) {
	got := extractStringList("single-value")

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0] != "single-value" {
		t.Errorf("got[0] = %q, want %q", got[0], "single-value")
	}
}

func TestExtractStringList_Nil(t *testing.T) {
	got := extractStringList(nil)
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestExtractStringList_EmptySlice(t *testing.T) {
	got := extractStringList([]any{})
	if got != nil {
		t.Errorf("got = %v, want nil for empty slice", got)
	}
}

func TestExtractStringList_UnsupportedType(t *testing.T) {
	got := extractStringList(42)
	if got != nil {
		t.Errorf("got = %v, want nil for unsupported type", got)
	}
}

// --- summaryBuilder.String Tests ---

func TestSummaryBuilder_String_MultipleSources(t *testing.T) {
	sb := &summaryBuilder{
		result: &FetchResult{
			Summaries: []FetchSummary{
				{SourceID: "disk-local", Documents: 5},
				{SourceID: "github-test", Documents: 3},
			},
			TotalDocs: 8,
			TotalErrs: 0,
			TotalSkip: 0,
		},
	}

	got := sb.String()

	// Verify each source appears in the output with document count.
	if !containsStr(got, "disk-local: 5 documents") {
		t.Errorf("output missing 'disk-local: 5 documents', got:\n%s", got)
	}
	if !containsStr(got, "github-test: 3 documents") {
		t.Errorf("output missing 'github-test: 3 documents', got:\n%s", got)
	}
	if !containsStr(got, "Total: 8 documents, 0 errors, 0 skipped") {
		t.Errorf("output missing totals line, got:\n%s", got)
	}
}

func TestSummaryBuilder_String_WithError(t *testing.T) {
	sb := &summaryBuilder{
		result: &FetchResult{
			Summaries: []FetchSummary{
				{SourceID: "disk-local", Documents: 2},
				{SourceID: "github-fail", Error: "network error", Errors: 1},
			},
			TotalDocs: 2,
			TotalErrs: 1,
			TotalSkip: 0,
		},
	}

	got := sb.String()

	if !containsStr(got, "github-fail: error (network error)") {
		t.Errorf("output missing error line for github-fail, got:\n%s", got)
	}
	if !containsStr(got, "disk-local: 2 documents") {
		t.Errorf("output missing disk-local line, got:\n%s", got)
	}
	if !containsStr(got, "Total: 2 documents, 1 errors, 0 skipped") {
		t.Errorf("output missing totals line, got:\n%s", got)
	}
}

func TestSummaryBuilder_String_WithSkipped(t *testing.T) {
	sb := &summaryBuilder{
		result: &FetchResult{
			Summaries: []FetchSummary{
				{SourceID: "disk-local", Documents: 1},
				{SourceID: "github-test", Skipped: true},
			},
			TotalDocs: 1,
			TotalErrs: 0,
			TotalSkip: 1,
		},
	}

	got := sb.String()

	if !containsStr(got, "github-test: skipped (within refresh interval)") {
		t.Errorf("output missing skipped line, got:\n%s", got)
	}
	if !containsStr(got, "Total: 1 documents, 0 errors, 1 skipped") {
		t.Errorf("output missing totals line, got:\n%s", got)
	}
}

func TestSummaryBuilder_String_EmptySummaries(t *testing.T) {
	sb := &summaryBuilder{
		result: &FetchResult{
			Summaries: nil,
			TotalDocs: 0,
			TotalErrs: 0,
			TotalSkip: 0,
		},
	}

	got := sb.String()

	if !containsStr(got, "Total: 0 documents, 0 errors, 0 skipped") {
		t.Errorf("output missing totals line for empty result, got:\n%s", got)
	}
}

func TestFormatSummary_DelegatesToSummaryBuilder(t *testing.T) {
	result := &FetchResult{
		Summaries: []FetchSummary{
			{SourceID: "disk-local", Documents: 3},
		},
		TotalDocs: 3,
		TotalErrs: 0,
		TotalSkip: 0,
	}

	got := result.FormatSummary()

	if !containsStr(got, "disk-local: 3 documents") {
		t.Errorf("FormatSummary output missing source line, got:\n%s", got)
	}
	if !containsStr(got, "Total: 3 documents, 0 errors, 0 skipped") {
		t.Errorf("FormatSummary output missing totals line, got:\n%s", got)
	}
}

// containsStr checks if s contains substr (helper for readability).
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestCreateDiskSource_RelativePath verifies that relative paths like
// "../sibling" are resolved against basePath.
func TestCreateDiskSource_RelativePath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "disk-sibling",
		Name: "sibling",
		Type: "disk",
		Config: map[string]any{
			"path": "../sibling",
		},
	}
	basePath := "/users/dev/projects/myrepo"
	src := createDiskSource(cfg, basePath)
	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("expected *DiskSource")
	}
	want := filepath.Join(basePath, "../sibling")
	if ds.basePath != want {
		t.Errorf("basePath = %q, want %q", ds.basePath, want)
	}
}

// TestCreateDiskSource_AbsolutePath verifies that absolute paths are
// passed through unchanged.
func TestCreateDiskSource_AbsolutePath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "disk-abs",
		Name: "absolute",
		Type: "disk",
		Config: map[string]any{
			"path": "/opt/data/docs",
		},
	}
	basePath := "/users/dev/projects/myrepo"
	src := createDiskSource(cfg, basePath)
	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("expected *DiskSource")
	}
	if ds.basePath != "/opt/data/docs" {
		t.Errorf("basePath = %q, want %q", ds.basePath, "/opt/data/docs")
	}
}

// TestCreateDiskSource_DotPath verifies that "." is resolved to basePath
// (existing behavior preserved).
func TestCreateDiskSource_DotPath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "disk-local",
		Name: "local",
		Type: "disk",
		Config: map[string]any{
			"path": ".",
		},
	}
	basePath := "/users/dev/projects/myrepo"
	src := createDiskSource(cfg, basePath)
	ds, ok := src.(*DiskSource)
	if !ok {
		t.Fatal("expected *DiskSource")
	}
	if ds.basePath != basePath {
		t.Errorf("basePath = %q, want %q", ds.basePath, basePath)
	}
}

// --- Concurrent fetching tests (FR-101) ---

// TestManager_FetchAll_ConcurrentSources verifies that multiple sources
// are fetched concurrently when more than one is eligible (FR-101 scenario 1).
func TestManager_FetchAll_ConcurrentSources(t *testing.T) {
	const numSources = 4

	// Barrier: all goroutines must reach List() before any can proceed.
	// This proves concurrent execution — sequential fetching would deadlock.
	var activeCount atomic.Int64
	barrier := make(chan struct{})

	sources := make([]Source, numSources)
	configs := make([]SourceConfig, numSources)
	for i := range numSources {
		id := fmt.Sprintf("src-%d", i)
		sources[i] = &mockSource{
			id:      id,
			srcType: "disk",
			name:    id,
			docs:    []Document{{ID: fmt.Sprintf("doc-%d", i), SourceID: id}},
			listFn: func() {
				if activeCount.Add(1) == numSources {
					close(barrier) // Last goroutine releases the barrier.
				}
				<-barrier // All goroutines wait here.
			},
		}
		configs[i] = SourceConfig{ID: id, Type: "disk", Name: id}
	}

	mgr := &Manager{sources: sources, configs: configs}
	result, allDocs := mgr.FetchAll("", true, nil)

	if result.TotalDocs != numSources {
		t.Errorf("total docs = %d, want %d", result.TotalDocs, numSources)
	}
	if len(allDocs) != numSources {
		t.Errorf("source count = %d, want %d", len(allDocs), numSources)
	}
	// Verify no errors or skips in the concurrent path.
	if result.TotalErrs != 0 {
		t.Errorf("total errors = %d, want 0", result.TotalErrs)
	}
	if result.TotalSkip != 0 {
		t.Errorf("total skipped = %d, want 0", result.TotalSkip)
	}
	// Verify per-source summaries were collected for all sources.
	if len(result.Summaries) != numSources {
		t.Errorf("summaries count = %d, want %d", len(result.Summaries), numSources)
	}
	// Verify each source has exactly 1 document in its summary.
	for _, s := range result.Summaries {
		if s.Documents != 1 {
			t.Errorf("source %s summary docs = %d, want 1", s.SourceID, s.Documents)
		}
		if s.Errors != 0 {
			t.Errorf("source %s summary errors = %d, want 0", s.SourceID, s.Errors)
		}
	}
}

// TestManager_FetchAll_SourceFailureDoesNotCancelOthers verifies that one
// source failure does not cancel other source fetches (FR-101 scenario 2).
func TestManager_FetchAll_SourceFailureDoesNotCancelOthers(t *testing.T) {
	sources := []Source{
		&mockSource{id: "src-a", srcType: "disk", name: "a", docs: []Document{{ID: "a1"}}},
		&mockSource{id: "src-b", srcType: "disk", name: "b", err: fmt.Errorf("network error")},
		&mockSource{id: "src-c", srcType: "disk", name: "c", docs: []Document{{ID: "c1"}}},
	}
	configs := []SourceConfig{
		{ID: "src-a", Type: "disk", Name: "a"},
		{ID: "src-b", Type: "disk", Name: "b"},
		{ID: "src-c", Type: "disk", Name: "c"},
	}

	mgr := &Manager{sources: sources, configs: configs}
	result, allDocs := mgr.FetchAll("", true, nil)

	// A and C should succeed, B should fail.
	if result.TotalDocs != 2 {
		t.Errorf("total docs = %d, want 2", result.TotalDocs)
	}
	if result.TotalErrs != 1 {
		t.Errorf("total errors = %d, want 1", result.TotalErrs)
	}
	if _, ok := allDocs["src-b"]; ok {
		t.Error("failed source should not have documents")
	}
	if len(allDocs["src-a"]) != 1 {
		t.Errorf("src-a docs = %d, want 1", len(allDocs["src-a"]))
	}
	if len(allDocs["src-c"]) != 1 {
		t.Errorf("src-c docs = %d, want 1", len(allDocs["src-c"]))
	}

	// Verify error is recorded in summaries.
	var foundError bool
	for _, s := range result.Summaries {
		if s.SourceID == "src-b" {
			foundError = true
			if s.Errors != 1 {
				t.Errorf("src-b summary errors = %d, want 1", s.Errors)
			}
			if s.Error == "" {
				t.Error("src-b summary should include error message")
			}
		}
	}
	if !foundError {
		t.Error("summary should contain entry for failed source src-b")
	}
}

// TestManager_FetchAll_SingleSourceBypassesConcurrency verifies that when
// only one source matches (via filter), concurrency overhead is skipped (FR-101 scenario 3).
func TestManager_FetchAll_SingleSourceBypassesConcurrency(t *testing.T) {
	sources := []Source{
		&mockSource{id: "src-a", srcType: "disk", name: "a", docs: []Document{{ID: "a1"}}},
		&mockSource{id: "src-b", srcType: "disk", name: "b", docs: []Document{{ID: "b1"}}},
		&mockSource{id: "src-c", srcType: "disk", name: "c", docs: []Document{{ID: "c1"}}},
		&mockSource{id: "src-d", srcType: "disk", name: "d", docs: []Document{{ID: "d1"}}},
		&mockSource{id: "src-e", srcType: "disk", name: "e", docs: []Document{{ID: "e1"}}},
	}
	configs := []SourceConfig{
		{ID: "src-a", Type: "disk", Name: "a"},
		{ID: "src-b", Type: "disk", Name: "b"},
		{ID: "src-c", Type: "disk", Name: "c"},
		{ID: "src-d", Type: "disk", Name: "d"},
		{ID: "src-e", Type: "disk", Name: "e"},
	}

	mgr := &Manager{sources: sources, configs: configs}
	result, allDocs := mgr.FetchAll("src-c", true, nil)

	if result.TotalDocs != 1 {
		t.Errorf("total docs = %d, want 1", result.TotalDocs)
	}
	if len(allDocs) != 1 {
		t.Errorf("source count = %d, want 1", len(allDocs))
	}
	if _, ok := allDocs["src-c"]; !ok {
		t.Error("filtered source should have documents")
	}

	// Verify other sources were not fetched.
	for _, src := range sources {
		ms, ok := src.(*mockSource)
		if !ok {
			t.Fatal("expected *mockSource")
		}
		if ms.id != "src-c" && ms.listCalls.Load() != 0 {
			t.Errorf("source %s should not have been fetched", ms.id)
		}
	}
}

// TestManager_FetchAll_ConcurrentResultAggregation verifies that document
// counts are accurate when sources are fetched concurrently.
func TestManager_FetchAll_ConcurrentResultAggregation(t *testing.T) {
	// Use a wait group to add timing uncertainty.
	var wg sync.WaitGroup
	wg.Add(1)

	sources := make([]Source, 10)
	configs := make([]SourceConfig, 10)
	expectedTotal := 0
	for i := range 10 {
		id := fmt.Sprintf("src-%d", i)
		docCount := i + 1
		expectedTotal += docCount
		docs := make([]Document, docCount)
		for j := range docCount {
			docs[j] = Document{ID: fmt.Sprintf("doc-%d-%d", i, j)}
		}
		sources[i] = &mockSource{
			id:      id,
			srcType: "disk",
			name:    id,
			docs:    docs,
			listFn: func() {
				wg.Wait() // All wait until released.
			},
		}
		configs[i] = SourceConfig{ID: id, Type: "disk", Name: id}
	}

	mgr := &Manager{sources: sources, configs: configs}

	// Release all sources to proceed simultaneously.
	wg.Done()
	result, allDocs := mgr.FetchAll("", true, nil)

	if result.TotalDocs != expectedTotal {
		t.Errorf("total docs = %d, want %d", result.TotalDocs, expectedTotal)
	}
	if len(allDocs) != 10 {
		t.Errorf("source count = %d, want 10", len(allDocs))
	}
}

// TestCreateCodeSource_RelativePath verifies that code sources also
// resolve relative paths against basePath.
func TestCreateCodeSource_RelativePath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "code-sibling",
		Name: "sibling-code",
		Type: "code",
		Config: map[string]any{
			"path":      "../replicator",
			"languages": []any{"go"},
		},
	}
	basePath := "/users/dev/projects/myrepo"
	src := createCodeSource(cfg, basePath)
	cs, ok := src.(*CodeSource)
	if !ok {
		t.Fatal("expected *CodeSource")
	}
	want := filepath.Join(basePath, "../replicator")
	if cs.basePath != want {
		t.Errorf("basePath = %q, want %q", cs.basePath, want)
	}
}
