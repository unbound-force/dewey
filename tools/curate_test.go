package tools

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

func TestCurateTool_NilStore(t *testing.T) {
	curateTool := NewCurate(nil, nil, nil, "", nil)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for nil store")
	}
}

func TestCurateTool_NoConfig(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Use a temp dir with no knowledge-stores.yaml.
	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}

	curateTool := NewCurate(s, nil, nil, dir, nil)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for missing config")
	}
}

func TestCurateTool_StoreNotFound(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create a config with one store.
	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: team-decisions
    sources: [disk-local]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	curateTool := NewCurate(s, nil, nil, dir, nil)

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{
		Store: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for store not found")
	}
}

func TestCurateTool_ConcurrentCallRejected(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer func() { _ = s.Close() }()

	dir := t.TempDir()
	deweyDir := filepath.Join(dir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create dewey dir: %v", err)
	}
	ksContent := `stores:
  - name: test
    sources: [disk-local]
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	mu := &sync.Mutex{}
	curateTool := NewCurate(s, nil, nil, dir, mu)

	// Lock the mutex to simulate an indexing operation in progress.
	mu.Lock()

	result, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected MCP error result for concurrent call")
	}

	mu.Unlock()
}
