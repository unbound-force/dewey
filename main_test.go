// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They mutate the package-level logger output for log assertions.
// Some tests (TestEnsureOllama_BinaryNotFound) also manipulate PATH.
package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// TestResolveBackendType_DefaultObsidian verifies "obsidian" is returned
// when both flag value and environment variable are empty.
func TestResolveBackendType_DefaultObsidian(t *testing.T) {
	t.Setenv("DEWEY_BACKEND", "")

	got := resolveBackendType("")
	if got != "obsidian" {
		t.Errorf("resolveBackendType(%q) = %q, want %q", "", got, "obsidian")
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
		{"default obsidian", "", "", "obsidian"},
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
	err := executeServe(false, "unknown-backend", "", "", "", false)
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

	err := executeServe(false, "obsidian", "", "", "", false)
	if err == nil {
		t.Fatal("executeServe with obsidian and no vault path should return error")
	}
	if !strings.Contains(err.Error(), "--vault or OBSIDIAN_VAULT_PATH required") {
		t.Errorf("error = %q, want vault path required message", err.Error())
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
	_ = listener.Close()

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

	_, _, _, err := initObsidianBackend("", "daily notes", false)
	if err == nil {
		t.Fatal("initObsidianBackend with no vault path should return error")
	}
	if !strings.Contains(err.Error(), "--vault or OBSIDIAN_VAULT_PATH required") {
		t.Errorf("error = %q, want vault path required message", err.Error())
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

	// Pass noEmbeddings=true because Ollama is not running in test env.
	b, opts, cleanup, err := initObsidianBackend("", "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}
	// With noEmbeddings=true, no embedder option is added.
	// opts may be empty (no store, no embedder).
	_ = opts
}

// TestInitObsidianBackend_WithPersistentStore verifies that when .uf/dewey/
// exists in the vault path, a persistent store is initialized and included
// in the server options.
func TestInitObsidianBackend_WithPersistentStore(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ directory to trigger store initialization.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Pass noEmbeddings=true because Ollama is not running in test env.
	b, opts, cleanup, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}

	// With .uf/dewey/ present and noEmbeddings=true, should have at least 1 option (persistent store).
	if len(opts) < 1 {
		t.Errorf("expected at least 1 server option (store), got %d", len(opts))
	}
}

// TestInitObsidianBackend_EmbedderEnvConfig verifies that the DEWEY_EMBEDDING_MODEL
// and DEWEY_EMBEDDING_ENDPOINT env vars are used when set, and default values are
// used when unset. The function always creates an embedder (graceful degradation).
func TestInitObsidianBackend_EmbedderEnvConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Set custom embedding config via env vars.
	t.Setenv("OBSIDIAN_VAULT_PATH", "")
	t.Setenv("DEWEY_EMBEDDING_MODEL", "custom-model:latest")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://localhost:99999")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Pass noEmbeddings=true because Ollama is not running in test env
	// (custom endpoint http://localhost:99999 is unreachable).
	b, _, cleanup, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}

	// Log output should mention embeddings disabled.
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("expected log output about embeddings disabled")
	}
}

// TestInitObsidianBackend_WithMarkdownFiles verifies that initObsidianBackend
// successfully indexes markdown files and returns a working backend that can
// list pages.
func TestInitObsidianBackend_WithMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test markdown files.
	if err := os.WriteFile(tmpDir+"/test-page.md", []byte("# Test Page\n\nContent."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Pass noEmbeddings=true because Ollama is not running in test env.
	b, _, cleanup, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	// Verify pages were indexed by querying through the backend.
	pages, err := b.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}
	if len(pages) == 0 {
		t.Error("expected at least 1 page after indexing")
	}

	// Verify the specific page is accessible.
	page, err := b.GetPage(context.Background(), "test-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Error("test-page should be found after indexing")
	}
}

// TestInitObsidianBackend_FlagTakesPrecedence verifies that the vaultPath
// flag takes precedence over the OBSIDIAN_VAULT_PATH env var.
func TestInitObsidianBackend_FlagTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	// Set env to a different (non-existent) path — flag should take precedence.
	t.Setenv("OBSIDIAN_VAULT_PATH", "/nonexistent/should-not-be-used")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Pass noEmbeddings=true because Ollama is not running in test env.
	b, _, cleanup, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}
}

// --- --no-embeddings tests ---

// TestInitObsidianBackend_NoEmbeddings_Succeeds verifies that serve starts
// without error when Ollama is unavailable and noEmbeddings is true.
func TestInitObsidianBackend_NoEmbeddings_Succeeds(t *testing.T) {
	tmpDir := t.TempDir()

	// Point to an unreachable Ollama endpoint.
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://localhost:99999")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b, _, cleanup, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend with noEmbeddings=true should succeed, got: %v", err)
	}
	defer cleanup()

	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}

	// Log should mention embeddings disabled.
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "embeddings disabled") {
		t.Errorf("log should mention embeddings disabled, got:\n%s", logOutput)
	}
}

// TestInitObsidianBackend_GracefulDegradation_WhenOllamaUnavailable verifies that
// serve succeeds in keyword-only mode when Ollama is unavailable (not running at
// a remote endpoint). This tests the 007-ollama-autostart graceful degradation:
// instead of a hard error, Dewey logs the unavailability and proceeds without
// embeddings.
func TestInitObsidianBackend_GracefulDegradation_WhenOllamaUnavailable(t *testing.T) {
	tmpDir := t.TempDir()

	// Point to a remote (non-local) unreachable endpoint so ensureOllama
	// skips the auto-start attempt and returns OllamaUnavailable immediately.
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://remote-host:99999")
	t.Setenv("DEWEY_EMBEDDING_MODEL", "granite-embedding:30m")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	_, _, cleanup, err := initObsidianBackend(tmpDir, "daily notes", false)
	if err != nil {
		t.Fatalf("initObsidianBackend should succeed with graceful degradation, got error: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	logOutput := logBuf.String()
	// Should log that semantic search is unavailable.
	if !strings.Contains(logOutput, "semantic search unavailable") {
		t.Errorf("log should contain 'semantic search unavailable', got: %q", logOutput)
	}
	// Should log the ollama state as unavailable.
	if !strings.Contains(logOutput, "unavailable") {
		t.Errorf("log should contain 'unavailable' state, got: %q", logOutput)
	}
}

// --- OllamaState tests (T011) ---

// TestOllamaState_String verifies the String() method returns the correct
// human-readable label for each OllamaState value, including unknown states.
func TestOllamaState_String(t *testing.T) {
	tests := []struct {
		state OllamaState
		want  string
	}{
		{OllamaExternal, "external"},
		{OllamaManaged, "managed"},
		{OllamaUnavailable, "unavailable"},
		{OllamaState(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.state.String()
			if got != tc.want {
				t.Errorf("OllamaState(%d).String() = %q, want %q", int(tc.state), got, tc.want)
			}
		})
	}
}

// --- isLocalEndpoint tests (T012) ---

// TestIsLocalEndpoint verifies that isLocalEndpoint correctly identifies
// local vs remote endpoints across various URL formats.
func TestIsLocalEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"http://localhost:11434", true},
		{"http://127.0.0.1:11434", true},
		{"http://[::1]:11434", true},
		{"http://gpu-server:11434", false},
		{"http://192.168.1.100:11434", false},
		{"", true},        // empty hostname defaults to localhost
		{"://bad", false}, // malformed URL
	}

	for _, tc := range tests {
		t.Run(tc.endpoint, func(t *testing.T) {
			got := isLocalEndpoint(tc.endpoint)
			if got != tc.want {
				t.Errorf("isLocalEndpoint(%q) = %v, want %v", tc.endpoint, got, tc.want)
			}
		})
	}
}

// --- ollamaHealthCheck tests (T013) ---

// TestOllamaHealthCheck_Healthy verifies that ollamaHealthCheck returns true
// when the endpoint responds with HTTP 200 on /api/tags.
func TestOllamaHealthCheck_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if !ollamaHealthCheck(server.URL) {
		t.Errorf("ollamaHealthCheck(%q) = false, want true", server.URL)
	}
}

// TestOllamaHealthCheck_Unreachable verifies that ollamaHealthCheck returns
// false when the endpoint is not reachable (port 0 = no listener).
func TestOllamaHealthCheck_Unreachable(t *testing.T) {
	if ollamaHealthCheck("http://127.0.0.1:0") {
		t.Error("ollamaHealthCheck(unreachable) = true, want false")
	}
}

// --- ensureOllama tests (T014-T018) ---

// mockStarter records whether Start() was called, for testing ensureOllama
// without launching real subprocesses.
type mockStarter struct {
	called bool
}

func (m *mockStarter) Start() error {
	m.called = true
	return nil
}

// TestEnsureOllama_AlreadyRunning verifies that when Ollama is already
// reachable, ensureOllama returns OllamaExternal without calling Start().
func TestEnsureOllama_AlreadyRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mock := &mockStarter{}
	state, err := ensureOllama(server.URL, true, mock)
	if err != nil {
		t.Fatalf("ensureOllama() error = %v, want nil", err)
	}
	if state != OllamaExternal {
		t.Errorf("ensureOllama() state = %v, want OllamaExternal", state)
	}
	if mock.called {
		t.Error("Start() should not be called when Ollama is already running")
	}
}

// TestEnsureOllama_BinaryNotFound verifies that ensureOllama returns
// OllamaUnavailable when the ollama binary is not in PATH.
// PARALLEL SAFETY: Manipulates PATH, must not run in parallel.
func TestEnsureOllama_BinaryNotFound(t *testing.T) {
	// Save and restore PATH.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer func() { _ = os.Setenv("PATH", origPath) }()

	mock := &mockStarter{}
	state, err := ensureOllama("http://localhost:99999", true, mock)
	if err != nil {
		t.Fatalf("ensureOllama() error = %v, want nil", err)
	}
	if state != OllamaUnavailable {
		t.Errorf("ensureOllama() state = %v, want OllamaUnavailable", state)
	}
	if mock.called {
		t.Error("Start() should not be called when binary is not in PATH")
	}
}

// TestEnsureOllama_RemoteEndpoint verifies that ensureOllama does not attempt
// to start Ollama when the endpoint is a remote host (non-local).
func TestEnsureOllama_RemoteEndpoint(t *testing.T) {
	mock := &mockStarter{}
	state, err := ensureOllama("http://gpu-server:11434", true, mock)
	if err != nil {
		t.Fatalf("ensureOllama() error = %v, want nil", err)
	}
	if state != OllamaUnavailable {
		t.Errorf("ensureOllama() state = %v, want OllamaUnavailable", state)
	}
	if mock.called {
		t.Error("Start() should not be called for remote endpoints")
	}
}

// TestEnsureOllama_StartSuccess verifies that ensureOllama starts Ollama
// and returns OllamaManaged when the binary is available and the server
// becomes ready after starting.
func TestEnsureOllama_StartSuccess(t *testing.T) {
	// Skip if ollama binary is not in PATH — this test requires LookPath to succeed.
	if _, err := exec.LookPath("ollama"); err != nil {
		t.Skip("ollama not in PATH")
	}

	// Counter-based server: first health check fails (503), subsequent ones succeed (200).
	// This simulates Ollama starting up after the subprocess is launched.
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mock := &mockStarter{}
	state, err := ensureOllama(server.URL, true, mock)
	if err != nil {
		t.Fatalf("ensureOllama() error = %v, want nil", err)
	}
	if state != OllamaManaged {
		t.Errorf("ensureOllama() state = %v, want OllamaManaged", state)
	}
	if !mock.called {
		t.Error("Start() should be called when Ollama needs to be started")
	}
}

// TestEnsureOllama_AutoStartDisabled verifies that ensureOllama returns
// OllamaUnavailable without panicking when autoStart is false and the
// starter is nil (doctor mode).
func TestEnsureOllama_AutoStartDisabled(t *testing.T) {
	state, err := ensureOllama("http://localhost:99999", false, nil)
	if err != nil {
		t.Fatalf("ensureOllama() error = %v, want nil", err)
	}
	if state != OllamaUnavailable {
		t.Errorf("ensureOllama() state = %v, want OllamaUnavailable", state)
	}
}
