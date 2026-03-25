package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- resolveBackendType tests (T014) ---

// TestResolveBackendType_FlagValue verifies the flag value is returned
// when non-empty, regardless of environment variable state.
func TestResolveBackendType_FlagValue(t *testing.T) {
	t.Setenv("DEWEY_BACKEND", "obsidian")

	got := resolveBackendType("logseq")
	if got != "logseq" {
		t.Errorf("resolveBackendType(%q) = %q, want %q", "logseq", got, "logseq")
	}
}

// TestResolveBackendType_EnvFallback verifies the DEWEY_BACKEND environment
// variable is used when the flag value is empty.
func TestResolveBackendType_EnvFallback(t *testing.T) {
	t.Setenv("DEWEY_BACKEND", "obsidian")

	got := resolveBackendType("")
	if got != "obsidian" {
		t.Errorf("resolveBackendType(%q) = %q, want %q", "", got, "obsidian")
	}
}

// TestResolveBackendType_DefaultLogseq verifies "logseq" is returned
// when both flag value and environment variable are empty.
func TestResolveBackendType_DefaultLogseq(t *testing.T) {
	t.Setenv("DEWEY_BACKEND", "")

	got := resolveBackendType("")
	if got != "logseq" {
		t.Errorf("resolveBackendType(%q) = %q, want %q", "", got, "logseq")
	}
}

// TestResolveBackendType_ArbitraryValue verifies arbitrary backend types
// are passed through without validation (validation happens in executeServe).
func TestResolveBackendType_ArbitraryValue(t *testing.T) {
	got := resolveBackendType("custom-backend")
	if got != "custom-backend" {
		t.Errorf("resolveBackendType(%q) = %q, want %q", "custom-backend", got, "custom-backend")
	}
}

// TestResolveBackendType_Table verifies all resolution precedence rules
// in a single table-driven test.
func TestResolveBackendType_Table(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      string
	}{
		{"flag takes precedence", "obsidian", "logseq", "obsidian"},
		{"env fallback", "", "obsidian", "obsidian"},
		{"default logseq", "", "", "logseq"},
		{"flag with no env", "obsidian", "", "obsidian"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEWEY_BACKEND", tc.envValue)

			got := resolveBackendType(tc.flagValue)
			if got != tc.want {
				t.Errorf("resolveBackendType(%q) with DEWEY_BACKEND=%q = %q, want %q",
					tc.flagValue, tc.envValue, got, tc.want)
			}
		})
	}
}

// --- initLogseqBackend tests (T014) ---

// TestInitLogseqBackend_ReturnsNonNil verifies initLogseqBackend returns
// a non-nil backend. The function creates a Logseq client and calls
// checkGraphVersionControl, which is best-effort and silently returns
// when the API is unreachable (no Logseq running in test).
func TestInitLogseqBackend_ReturnsNonNil(t *testing.T) {
	// Suppress log output from checkGraphVersionControl's HTTP error.
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b := initLogseqBackend()
	if b == nil {
		t.Fatal("initLogseqBackend() returned nil")
	}
}

// TestInitLogseqBackend_ImplementsBackend verifies the returned value
// satisfies the backend.Backend interface by checking it has the
// expected methods (compile-time check is implicit via return type,
// but this validates runtime behavior).
func TestInitLogseqBackend_ImplementsBackend(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b := initLogseqBackend()

	// The returned backend should be a *client.Client which implements
	// backend.Backend and backend.HasDataScript.
	if _, ok := b.(interface{ Ping(context.Context) error }); !ok {
		t.Error("initLogseqBackend() result does not have Ping method")
	}
}

// --- executeServe tests (T014) ---

// TestExecuteServe_UnknownBackend verifies executeServe returns an error
// for an unknown backend type.
func TestExecuteServe_UnknownBackend(t *testing.T) {
	err := executeServe(false, "unknown-backend", "", "", "")
	if err == nil {
		t.Fatal("executeServe with unknown backend should return error")
	}
	want := `unknown backend "unknown-backend" (use logseq or obsidian)`
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestExecuteServe_ObsidianRequiresVault verifies executeServe returns
// an error when obsidian backend is selected without vault path.
func TestExecuteServe_ObsidianRequiresVault(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT_PATH", "")

	err := executeServe(false, "obsidian", "", "", "")
	if err == nil {
		t.Fatal("executeServe with obsidian and no vault path should return error")
	}
	if got := err.Error(); got != "--vault or OBSIDIAN_VAULT_PATH required for obsidian backend" {
		t.Errorf("error = %q, want vault path required message", got)
	}
}

// --- runServer tests (T014) ---

// TestRunServer_HTTPTransport_InvalidAddr verifies runServer returns an
// error when given an invalid HTTP address that cannot be bound.
func TestRunServer_HTTPTransport_InvalidAddr(t *testing.T) {
	// Create a minimal server — we just need it to attempt ListenAndServe.
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "0.0.1"},
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use an address that will fail — port 0 on an invalid host.
	err := runServer(ctx, srv, "invalid-host:99999")
	if err == nil {
		t.Fatal("runServer with invalid address should return error")
	}
}

// TestRunServer_HTTPTransport_ContextCancellation verifies that the HTTP
// server shuts down gracefully when the context is cancelled.
func TestRunServer_HTTPTransport_ContextCancellation(t *testing.T) {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "0.0.1"},
		nil,
	)

	// Find a free port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, srv, addr)
	}()

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		// Should return nil (ErrServerClosed is swallowed by runServer).
		if err != nil {
			t.Errorf("runServer returned error after context cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not return after context cancellation (timeout)")
	}
}

// --- initObsidianBackend tests (T014) ---

// TestInitObsidianBackend_MissingVaultPath verifies initObsidianBackend
// returns an error when no vault path is provided via flag or env var.
func TestInitObsidianBackend_MissingVaultPath(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT_PATH", "")

	_, _, _, err := initObsidianBackend("", "daily notes")
	if err == nil {
		t.Fatal("initObsidianBackend with no vault path should return error")
	}
	if got := err.Error(); got != "--vault or OBSIDIAN_VAULT_PATH required for obsidian backend" {
		t.Errorf("error = %q, want vault path required message", got)
	}
}

// TestInitObsidianBackend_EnvVaultPath verifies initObsidianBackend uses
// the OBSIDIAN_VAULT_PATH environment variable when the flag is empty.
func TestInitObsidianBackend_EnvVaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", tmpDir)

	// Suppress log output from embedder availability check.
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b, opts, cleanup, err := initObsidianBackend("", "daily notes")
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}
	// Should have at least the embedder option.
	if len(opts) == 0 {
		t.Error("initObsidianBackend returned no server options")
	}
}

// TestInitObsidianBackend_WithPersistentStore verifies that when .dewey/
// exists in the vault path, a persistent store is initialized and included
// in the server options.
func TestInitObsidianBackend_WithPersistentStore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dewey/ directory to trigger store initialization.
	deweyDir := tmpDir + "/.dewey"
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .dewey: %v", err)
	}

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b, opts, cleanup, err := initObsidianBackend(tmpDir, "daily notes")
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}

	// With .dewey/ present, should have at least 2 options: persistent store + embedder.
	if len(opts) < 2 {
		t.Errorf("expected at least 2 server options (store + embedder), got %d", len(opts))
	}
}
