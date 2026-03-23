package source

import (
	"fmt"
	"testing"
	"time"
)

// mockSource is a test double for the Source interface.
type mockSource struct {
	id        string
	srcType   string
	name      string
	docs      []Document
	err       error
	listCalls int
}

func (m *mockSource) List() ([]Document, error) {
	m.listCalls++
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
	if src.listCalls != 0 {
		t.Errorf("List should not be called when within refresh interval")
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
	if src.listCalls != 1 {
		t.Errorf("List should be called once when forced")
	}
}
