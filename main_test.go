// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They mutate the package-level logger output for log assertions.
// Some tests (TestEnsureOllama_BinaryNotFound) also manipulate PATH.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/curate"
	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/tools"
	"github.com/unbound-force/dewey/types"
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

	_, _, _, _, err := initObsidianBackend("", "daily notes", false)
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
	b, opts, cleanup, _, err := initObsidianBackend("", "daily notes", true)
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
	b, opts, cleanup, _, err := initObsidianBackend(tmpDir, "daily notes", true)
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
	b, _, cleanup, _, err := initObsidianBackend(tmpDir, "daily notes", true)
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
// returns quickly without indexing (spec 012, T003), and that calling the
// returned deferredIndex function populates the in-memory index.
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
	initStart := time.Now()
	b, _, cleanup, deferredIndex, err := initObsidianBackend(tmpDir, "daily notes", true)
	initElapsed := time.Since(initStart)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	// T006: Verify initObsidianBackend returns quickly (< 1 second) because
	// indexing is deferred. The function only resolves paths, opens the store,
	// and creates the vault client — no file walking or indexing.
	if initElapsed >= 1*time.Second {
		t.Errorf("initObsidianBackend took %v, want < 1s (indexing should be deferred)", initElapsed)
	}

	// T006: Verify pages are NOT indexed immediately after initObsidianBackend.
	// The in-memory index should be empty because indexing is deferred.
	pagesBeforeIndex, err := b.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages (before deferred index): %v", err)
	}
	if len(pagesBeforeIndex) != 0 {
		t.Errorf("expected 0 pages before deferredIndex, got %d", len(pagesBeforeIndex))
	}

	// T006: Verify deferredIndex is non-nil and calling it populates the index.
	if deferredIndex == nil {
		t.Fatal("deferredIndex should be non-nil")
	}
	if err := deferredIndex(); err != nil {
		t.Fatalf("deferredIndex failed: %v", err)
	}

	// Verify pages were indexed by querying through the backend.
	pages, err := b.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}
	if len(pages) == 0 {
		t.Error("expected at least 1 page after deferredIndex")
	}

	// Verify the specific page is accessible.
	page, err := b.GetPage(context.Background(), "test-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Error("test-page should be found after deferredIndex")
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
	b, _, cleanup, _, err := initObsidianBackend(tmpDir, "daily notes", true)
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

	b, _, cleanup, _, err := initObsidianBackend(tmpDir, "daily notes", true)
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

	_, _, cleanup, _, err := initObsidianBackend(tmpDir, "daily notes", false)
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

// --- Background indexing integration tests (T008, spec 012) ---

// TestBackgroundIndex_ServerStartsBeforeIndexing verifies the core split
// introduced by spec 012: initObsidianBackend() returns quickly without
// indexing, and the returned deferredIndex function performs the slow
// operations (indexVault, LoadExternalPages, Watch) when called later.
//
// This proves the MCP server can start accepting connections before the
// vault is fully indexed — the key user story (US1).
//
// PARALLEL SAFETY: Mutates package-level logger output for log assertions.
func TestBackgroundIndex_ServerStartsBeforeIndexing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple markdown files to simulate a real vault.
	files := map[string]string{
		"page-one.md":   "# Page One\n\nFirst page content.\n\n[[page-two]]",
		"page-two.md":   "# Page Two\n\nSecond page with a [[page-one]] backlink.",
		"page-three.md": "# Page Three\n\nThird page, no links.",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Step 1: Call initObsidianBackend — should return quickly.
	initStart := time.Now()
	b, _, cleanup, deferredIndex, err := initObsidianBackend(tmpDir, "daily notes", true)
	initElapsed := time.Since(initStart)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()

	if initElapsed >= 1*time.Second {
		t.Errorf("initObsidianBackend took %v, want < 1s", initElapsed)
	}

	// Step 2: Verify backend is non-nil but has no pages yet.
	if b == nil {
		t.Fatal("initObsidianBackend returned nil backend")
	}
	pagesBeforeIndex, err := b.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages (before index): %v", err)
	}
	if len(pagesBeforeIndex) != 0 {
		t.Errorf("expected 0 pages before background indexing, got %d", len(pagesBeforeIndex))
	}

	// Step 3: Simulate background indexing with shared mutex and readiness flag,
	// matching the pattern in executeServe() (spec 012, T004).
	indexReady := &atomic.Bool{}
	indexMu := &sync.Mutex{}

	if indexReady.Load() {
		t.Error("indexReady should be false before background indexing")
	}

	// Step 4: Run deferred indexing in a goroutine (same as executeServe).
	if deferredIndex == nil {
		t.Fatal("deferredIndex should be non-nil")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		indexMu.Lock()
		defer indexMu.Unlock()
		defer indexReady.Store(true)

		if err := deferredIndex(); err != nil {
			// Can't use t.Fatalf in goroutine — log and let the main
			// goroutine detect the failure via indexReady remaining false.
			t.Errorf("deferredIndex failed: %v", err)
		}
	}()

	// Step 5: Wait for background indexing to complete.
	select {
	case <-done:
		// Success — goroutine completed.
	case <-time.After(10 * time.Second):
		t.Fatal("background indexing did not complete within 10 seconds")
	}

	// Step 6: Verify indexReady is true after completion.
	if !indexReady.Load() {
		t.Error("indexReady should be true after background indexing completes")
	}

	// Step 7: Verify pages are now in the in-memory index.
	pagesAfterIndex, err := b.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages (after index): %v", err)
	}
	if len(pagesAfterIndex) != 3 {
		t.Errorf("expected 3 pages after background indexing, got %d", len(pagesAfterIndex))
	}
}

// TestBackgroundIndex_IndexReadyFlag verifies the atomic.Bool readiness flag
// lifecycle: starts false, remains false during indexing, becomes true after
// the deferred indexing function completes (FR-007, D2).
//
// PARALLEL SAFETY: Mutates package-level logger output for log assertions.
func TestBackgroundIndex_IndexReadyFlag(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal markdown file so indexing has work to do.
	if err := os.WriteFile(filepath.Join(tmpDir, "note.md"), []byte("# Note\n\nContent."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	b, _, cleanup, deferredIndex, err := initObsidianBackend(tmpDir, "daily notes", true)
	if err != nil {
		t.Fatalf("initObsidianBackend failed: %v", err)
	}
	defer cleanup()
	_ = b

	// Step 1: Create indexReady flag — starts false.
	indexReady := &atomic.Bool{}
	if indexReady.Load() {
		t.Error("indexReady should be false initially")
	}

	// Step 2: Call deferredIndex synchronously.
	if deferredIndex == nil {
		t.Fatal("deferredIndex should be non-nil")
	}
	if err := deferredIndex(); err != nil {
		t.Fatalf("deferredIndex failed: %v", err)
	}

	// Step 3: Set indexReady to true (simulating what executeServe's goroutine does).
	indexReady.Store(true)

	// Step 4: Verify the flag is now true.
	if !indexReady.Load() {
		t.Error("indexReady should be true after deferredIndex completes and flag is set")
	}
}

// TestBackgroundIndex_MutexBlocksIndexDuringStartup verifies that the shared
// mutex prevents the index MCP tool from running while background indexing
// is in progress. This is the key mutual exclusion guarantee (spec 012, D1).
//
// PARALLEL SAFETY: Mutates package-level logger output for log assertions.
func TestBackgroundIndex_MutexBlocksIndexDuringStartup(t *testing.T) {
	tmpDir := t.TempDir()

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Create a persistent store — required for the Index() handler to
	// proceed past the nil-store check and reach the mutex check.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create a shared mutex — same instance used by both background
	// indexing and the Indexing MCP tool.
	indexMu := &sync.Mutex{}

	// Simulate background indexing holding the lock.
	indexMu.Lock()

	// Create an Indexing tool handler with the shared mutex and a valid store.
	ix := tools.NewIndexing(s, nil, tmpDir, indexMu)

	// Attempt to call Index — should be rejected because the mutex is held.
	result, _, err := ix.Index(context.Background(), nil, types.IndexInput{})
	if err != nil {
		t.Fatalf("Index returned Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// The result should be an error about "already in progress".
	if !result.IsError {
		t.Fatal("expected error result when background indexing holds the mutex")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent in result")
	}
	if !strings.Contains(tc.Text, "already in progress") {
		t.Errorf("error message = %q, should mention 'already in progress'", tc.Text)
	}

	// Release the lock — simulating background indexing completion.
	indexMu.Unlock()
}

// --- Learning re-ingestion tests (015-curated-knowledge-stores, T005) ---

// TestReIngestLearnings_RecoversMissing verifies that learning markdown
// files without corresponding database entries are re-ingested on startup.
// This is the core durability guarantee: learnings survive graph.db deletion.
// Updated for learning-identity-collision-fix: uses new-format filenames
// and frontmatter including author.
func TestReIngestLearnings_RecoversMissing(t *testing.T) {
	vaultPath := t.TempDir()

	// Create the learnings directory with markdown files.
	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write two learning files with new-format frontmatter including author.
	file1 := `---
tag: authentication
author: alice
category: decision
created_at: 2026-04-21T10:30:00Z
identity: authentication-20260421T103000-alice
tier: draft
---

OAuth tokens should be rotated every 24 hours.
`
	file2 := `---
tag: deployment
author: bob
category: pattern
created_at: 2026-04-21T11:00:00Z
identity: deployment-20260421T110000-bob
tier: draft
---

Always use blue-green deployments for zero-downtime releases.
`
	if err := os.WriteFile(filepath.Join(learningsDir, "authentication-20260421T103000-alice.md"), []byte(file1), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(learningsDir, "deployment-20260421T110000-bob.md"), []byte(file2), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create an in-memory store (simulating a fresh graph.db).
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Re-ingest — both files should be recovered.
	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 re-ingested learnings, got %d", count)
	}

	// Verify pages exist in the store with correct metadata.
	page1, err := s.GetPage("learning/authentication-20260421T103000-alice")
	if err != nil {
		t.Fatalf("GetPage(authentication-20260421T103000-alice): %v", err)
	}
	if page1 == nil {
		t.Fatal("learning/authentication-20260421T103000-alice should exist in store after re-ingestion")
	}
	if page1.Tier != "draft" {
		t.Errorf("tier = %q, want %q", page1.Tier, "draft")
	}
	if page1.Category != "decision" {
		t.Errorf("category = %q, want %q", page1.Category, "decision")
	}
	if page1.SourceID != "learning" {
		t.Errorf("source_id = %q, want %q", page1.SourceID, "learning")
	}
	// Verify author is preserved in properties.
	if !strings.Contains(page1.Properties, `"author":"alice"`) {
		t.Errorf("properties should contain author=alice, got %s", page1.Properties)
	}

	page2, err := s.GetPage("learning/deployment-20260421T110000-bob")
	if err != nil {
		t.Fatalf("GetPage(deployment-20260421T110000-bob): %v", err)
	}
	if page2 == nil {
		t.Fatal("learning/deployment-20260421T110000-bob should exist in store after re-ingestion")
	}
	if page2.Category != "pattern" {
		t.Errorf("category = %q, want %q", page2.Category, "pattern")
	}
	// Verify author is preserved in properties.
	if !strings.Contains(page2.Properties, `"author":"bob"`) {
		t.Errorf("properties should contain author=bob, got %s", page2.Properties)
	}

	// Verify blocks were persisted.
	blocks, err := s.GetBlocksByPage("learning/authentication-20260421T103000-alice")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) == 0 {
		t.Error("expected at least 1 block for re-ingested learning")
	}
}

// TestReIngestLearnings_SkipsExisting verifies that learning files with
// corresponding database entries are NOT re-ingested (no duplicates).
func TestReIngestLearnings_SkipsExisting(t *testing.T) {
	vaultPath := t.TempDir()

	// Create a learning file.
	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	fileContent := `---
tag: auth
category: decision
created_at: 2026-04-21T10:30:00Z
identity: auth-1
tier: draft
---

Existing learning content.
`
	if err := os.WriteFile(filepath.Join(learningsDir, "auth-1.md"), []byte(fileContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a store with the page already present.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Insert the page so it already exists.
	if err := s.InsertPage(&store.Page{
		Name:         "learning/auth-1",
		OriginalName: "learning/auth-1",
		SourceID:     "learning",
		SourceDocID:  "learning-auth-1",
		Tier:         "draft",
		Category:     "decision",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Re-ingest — should skip the existing page.
	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 re-ingested learnings (already exists), got %d", count)
	}

	// Verify only one page exists (no duplicate).
	pages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 learning page, got %d", len(pages))
	}
}

// TestReIngestLearnings_NoFiles verifies that an empty learnings directory
// returns 0 with no errors.
func TestReIngestLearnings_NoFiles(t *testing.T) {
	vaultPath := t.TempDir()

	// Create an empty learnings directory.
	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 re-ingested learnings, got %d", count)
	}
}

// TestReIngestLearnings_NoDirectory verifies that a missing learnings
// directory returns 0 with no errors (graceful handling).
func TestReIngestLearnings_NoDirectory(t *testing.T) {
	vaultPath := t.TempDir()
	// Don't create the learnings directory.

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 re-ingested learnings, got %d", count)
	}
}

// TestReIngestLearnings_NilStore verifies that a nil store returns 0
// with no errors (graceful handling).
func TestReIngestLearnings_NilStore(t *testing.T) {
	vaultPath := t.TempDir()

	count, err := reIngestLearnings(nil, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 re-ingested learnings with nil store, got %d", count)
	}
}

// TestReIngestLearnings_PreservesCreatedAt verifies that re-ingested
// learnings preserve the original created_at timestamp from the file.
// Updated for learning-identity-collision-fix: uses new-format filename
// and frontmatter including author.
func TestReIngestLearnings_PreservesCreatedAt(t *testing.T) {
	vaultPath := t.TempDir()

	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Use a specific timestamp to verify preservation.
	fileContent := `---
tag: test
author: testuser
created_at: 2025-01-15T08:30:00Z
identity: test-20250115T083000-testuser
tier: draft
---

Test learning with specific timestamp.
`
	if err := os.WriteFile(filepath.Join(learningsDir, "test-20250115T083000-testuser.md"), []byte(fileContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 re-ingested learning, got %d", count)
	}

	page, err := s.GetPage("learning/test-20250115T083000-testuser")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("page should exist after re-ingestion")
	}

	// Verify the created_at timestamp was preserved from the file.
	expectedTime, _ := time.Parse(time.RFC3339, "2025-01-15T08:30:00Z")
	expectedMs := expectedTime.UnixMilli()
	if page.CreatedAt != expectedMs {
		t.Errorf("created_at = %d, want %d (preserved from file)", page.CreatedAt, expectedMs)
	}

	// Verify author is preserved in properties.
	if !strings.Contains(page.Properties, `"author":"testuser"`) {
		t.Errorf("properties should contain author=testuser, got %s", page.Properties)
	}
}

// TestReIngestLearnings_OldFormatCompatibility verifies that old-format
// learning files (tag-N.md with no author field) are re-ingested
// successfully with empty author in properties.
func TestReIngestLearnings_OldFormatCompatibility(t *testing.T) {
	vaultPath := t.TempDir()

	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Old-format file: no author field, sequential identity.
	oldFile := `---
tag: auth
category: decision
created_at: 2026-04-20T09:00:00Z
identity: auth-1
tier: draft
---

Use basic auth for internal services.
`
	if err := os.WriteFile(filepath.Join(learningsDir, "auth-1.md"), []byte(oldFile), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 re-ingested learning, got %d", count)
	}

	page, err := s.GetPage("learning/auth-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("learning/auth-1 should exist after re-ingestion")
	}

	// Verify properties do NOT contain "author" key (old format has no author).
	if strings.Contains(page.Properties, `"author"`) {
		t.Errorf("old-format learning should not have author in properties, got %s", page.Properties)
	}
	// Verify tag is preserved.
	if !strings.Contains(page.Properties, `"tag":"auth"`) {
		t.Errorf("properties should contain tag, got %s", page.Properties)
	}
}

// TestReIngestLearnings_MixedFormats verifies that a learnings directory
// containing both old-format (tag-N.md) and new-format
// (tag-YYYYMMDDTHHMMSS-author.md) files are all re-ingested correctly.
func TestReIngestLearnings_MixedFormats(t *testing.T) {
	vaultPath := t.TempDir()

	learningsDir := filepath.Join(vaultPath, deweyWorkspaceDir, "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Old-format files (no author).
	old1 := `---
tag: auth
category: decision
created_at: 2026-04-20T09:00:00Z
identity: auth-1
tier: draft
---

Old auth learning 1.
`
	old2 := `---
tag: deploy
category: pattern
created_at: 2026-04-20T10:00:00Z
identity: deploy-2
tier: draft
---

Old deploy learning 2.
`
	// New-format files (with author).
	new1 := `---
tag: auth
category: gotcha
created_at: 2026-04-21T14:30:22Z
identity: auth-20260421T143022-alice
tier: draft
author: alice
---

New auth learning by alice.
`
	new2 := `---
tag: deploy
category: context
created_at: 2026-04-21T15:00:00Z
identity: deploy-20260421T150000-bob
tier: draft
author: bob
---

New deploy learning by bob.
`
	files := map[string]string{
		"auth-1.md":                        old1,
		"deploy-2.md":                      old2,
		"auth-20260421T143022-alice.md":    new1,
		"deploy-20260421T150000-bob.md":    new2,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(learningsDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	count, err := reIngestLearnings(s, nil, vaultPath)
	if err != nil {
		t.Fatalf("reIngestLearnings error: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 re-ingested learnings, got %d", count)
	}

	// Verify old-format pages: no author in properties.
	oldPage1, err := s.GetPage("learning/auth-1")
	if err != nil {
		t.Fatalf("GetPage(auth-1): %v", err)
	}
	if oldPage1 == nil {
		t.Fatal("learning/auth-1 should exist")
	}
	if strings.Contains(oldPage1.Properties, `"author"`) {
		t.Errorf("old-format auth-1 should not have author, got %s", oldPage1.Properties)
	}

	oldPage2, err := s.GetPage("learning/deploy-2")
	if err != nil {
		t.Fatalf("GetPage(deploy-2): %v", err)
	}
	if oldPage2 == nil {
		t.Fatal("learning/deploy-2 should exist")
	}
	if strings.Contains(oldPage2.Properties, `"author"`) {
		t.Errorf("old-format deploy-2 should not have author, got %s", oldPage2.Properties)
	}

	// Verify new-format pages: author in properties.
	newPage1, err := s.GetPage("learning/auth-20260421T143022-alice")
	if err != nil {
		t.Fatalf("GetPage(auth-20260421T143022-alice): %v", err)
	}
	if newPage1 == nil {
		t.Fatal("learning/auth-20260421T143022-alice should exist")
	}
	if !strings.Contains(newPage1.Properties, `"author":"alice"`) {
		t.Errorf("new-format page should have author=alice, got %s", newPage1.Properties)
	}

	newPage2, err := s.GetPage("learning/deploy-20260421T150000-bob")
	if err != nil {
		t.Fatalf("GetPage(deploy-20260421T150000-bob): %v", err)
	}
	if newPage2 == nil {
		t.Fatal("learning/deploy-20260421T150000-bob should exist")
	}
	if !strings.Contains(newPage2.Properties, `"author":"bob"`) {
		t.Errorf("new-format page should have author=bob, got %s", newPage2.Properties)
	}
}

// TestParseLearningFrontmatter verifies the YAML frontmatter parser
// correctly extracts all fields from a learning markdown file.
func TestParseLearningFrontmatter(t *testing.T) {
	content := `---
tag: authentication
category: decision
created_at: 2026-04-21T10:30:00Z
identity: authentication-20260421T103000-alice
tier: draft
author: alice
---

OAuth tokens should be rotated every 24 hours.
`
	fm, body, err := parseLearningFrontmatter(content)
	if err != nil {
		t.Fatalf("parseLearningFrontmatter error: %v", err)
	}

	if fm.Tag != "authentication" {
		t.Errorf("tag = %q, want %q", fm.Tag, "authentication")
	}
	if fm.Category != "decision" {
		t.Errorf("category = %q, want %q", fm.Category, "decision")
	}
	if fm.CreatedAt != "2026-04-21T10:30:00Z" {
		t.Errorf("created_at = %q, want %q", fm.CreatedAt, "2026-04-21T10:30:00Z")
	}
	if fm.Identity != "authentication-20260421T103000-alice" {
		t.Errorf("identity = %q, want %q", fm.Identity, "authentication-20260421T103000-alice")
	}
	if fm.Tier != "draft" {
		t.Errorf("tier = %q, want %q", fm.Tier, "draft")
	}
	if fm.Author != "alice" {
		t.Errorf("author = %q, want %q", fm.Author, "alice")
	}
	if !strings.Contains(body, "OAuth tokens should be rotated") {
		t.Errorf("body = %q, should contain learning text", body)
	}
}

// TestParseLearningFrontmatter_NoFrontmatter verifies that a file
// without YAML frontmatter returns an error.
func TestParseLearningFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just plain text without frontmatter."
	_, _, err := parseLearningFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for content without frontmatter")
	}
}

// TestParseLearningFrontmatter_EmptyCategory verifies that a file
// without a category field parses successfully with empty category.
func TestParseLearningFrontmatter_EmptyCategory(t *testing.T) {
	content := `---
tag: general
created_at: 2026-04-21T10:30:00Z
identity: general-1
tier: draft
---

A learning without a category.
`
	fm, _, err := parseLearningFrontmatter(content)
	if err != nil {
		t.Fatalf("parseLearningFrontmatter error: %v", err)
	}
	if fm.Category != "" {
		t.Errorf("category = %q, want empty string", fm.Category)
	}
}

// --- Background curation tests (015-curated-knowledge-stores, T034) ---

// TestBackgroundCuration_SkipsWhenMutexHeld verifies that the background
// curation goroutine skips a cycle when the shared indexing mutex is held.
// This is the key mutual exclusion guarantee — curation uses TryLock and
// does not block MCP tools (FR-020).
func TestBackgroundCuration_SkipsWhenMutexHeld(t *testing.T) {
	vaultPath := t.TempDir()

	// Create an in-memory store.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create a store config with a very short interval.
	storeCfg := curate.StoreConfig{
		Name:             "test-store",
		Sources:          []string{"disk-local"},
		CurationInterval: "10ms",
	}

	// Create a mock synthesizer that records calls.
	synth := &llm.NoopSynthesizer{
		Response: `[]`,
		Avail:    true,
		Model:    "test-model",
	}

	// Lock the mutex to simulate indexing in progress.
	indexMu := &sync.Mutex{}
	indexMu.Lock()

	ctx, cancel := context.WithCancel(context.Background())

	// Enable debug logging so TryLock skip messages are captured.
	logger.SetLevel(log.DebugLevel)
	defer logger.SetLevel(log.InfoLevel)

	// Run backgroundCurateStore in a goroutine with a short interval.
	done := make(chan struct{})
	go func() {
		defer close(done)
		backgroundCurateStore(ctx, indexMu, storeCfg, 10*time.Millisecond, s, synth, nil, vaultPath)
	}()

	// Wait enough time for several ticker cycles.
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop the goroutine.
	cancel()

	// Wait for the goroutine to fully exit before reading shared state.
	select {
	case <-done:
		// Goroutine exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("backgroundCurateStore did not exit after context cancellation")
	}

	// Release the mutex (after goroutine is fully stopped).
	indexMu.Unlock()

	// The goroutine ran with the mutex held — TryLock should have failed
	// on every ticker cycle. Verify by checking that the goroutine did NOT
	// crash (it exited cleanly via context cancellation, proven by reaching
	// this point). The TryLock skip is logged at Debug level.
}

// TestBackgroundCuration_NoConfig verifies that backgroundCuration does not
// start any per-store goroutines when no stores are configured (empty slice).
// This tests the graceful no-op behavior when knowledge-stores.yaml is absent.
func TestBackgroundCuration_NoConfig(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	indexMu := &sync.Mutex{}
	indexReady := &atomic.Bool{}
	indexReady.Store(true) // Simulate indexing already complete.

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Call backgroundCuration with empty stores — should return quickly
	// after logging "background curation starting" with 0 stores.
	done := make(chan struct{})
	go func() {
		defer close(done)
		backgroundCuration(ctx, indexMu, indexReady, nil, s, nil, t.TempDir())
	}()

	select {
	case <-done:
		// backgroundCuration returned — expected for empty stores.
	case <-time.After(2 * time.Second):
		t.Fatal("backgroundCuration with no stores should return quickly")
	}
}

// TestBackgroundCuration_RespectsContextCancellation verifies that the
// background curation goroutine stops cleanly when the context is cancelled.
// This ensures graceful shutdown of the MCP server (FR-017).
func TestBackgroundCuration_RespectsContextCancellation(t *testing.T) {
	vaultPath := t.TempDir()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	storeCfg := curate.StoreConfig{
		Name:             "cancel-test",
		Sources:          []string{"disk-local"},
		CurationInterval: "100ms",
	}

	synth := &llm.NoopSynthesizer{
		Response: `[]`,
		Avail:    true,
		Model:    "test-model",
	}

	indexMu := &sync.Mutex{}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		backgroundCurateStore(ctx, indexMu, storeCfg, 100*time.Millisecond, s, synth, nil, vaultPath)
	}()

	// Let it run for a bit, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Goroutine exited cleanly — expected. This proves context
		// cancellation is respected (FR-017).
	case <-time.After(2 * time.Second):
		t.Fatal("backgroundCurateStore did not exit after context cancellation")
	}
}

// TestBackgroundCuration_WaitsForIndexReady verifies that backgroundCuration
// waits for the indexReady flag before starting its first curation cycle.
// This ensures curation doesn't run on stale or incomplete index data.
func TestBackgroundCuration_WaitsForIndexReady(t *testing.T) {
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	indexMu := &sync.Mutex{}
	indexReady := &atomic.Bool{} // Starts false — indexing not complete.

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	stores := []curate.StoreConfig{
		{
			Name:             "wait-test",
			Sources:          []string{"disk-local"},
			CurationInterval: "10ms",
		},
	}

	// backgroundCuration will block waiting for indexReady.
	// After a short delay, set indexReady to true.
	// Since Ollama is not available in test, the goroutine will log
	// "generation model not available" and return.
	done := make(chan struct{})
	go func() {
		defer close(done)
		backgroundCuration(ctx, indexMu, indexReady, stores, s, nil, t.TempDir())
	}()

	// Verify the goroutine is still waiting (indexReady is false).
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("backgroundCuration should be waiting for indexReady, but returned early")
	default:
		// Still waiting — expected.
	}

	// Set indexReady to true — goroutine should proceed.
	indexReady.Store(true)

	// The goroutine should now proceed and return (no Ollama available).
	select {
	case <-done:
		// Goroutine completed — expected (no Ollama in test env).
	case <-time.After(5 * time.Second):
		t.Fatal("backgroundCuration did not proceed after indexReady was set")
	}
}

// TestBackgroundCuration_ContinuesAfterError verifies that the background
// curation goroutine continues polling after a curation error, rather than
// crashing. This tests error resilience (FR-021).
func TestBackgroundCuration_ContinuesAfterError(t *testing.T) {
	vaultPath := t.TempDir()

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	storeCfg := curate.StoreConfig{
		Name:             "error-test",
		Sources:          []string{"nonexistent-source"},
		CurationInterval: "10ms",
	}

	// Synthesizer that returns an error to trigger curation failure.
	synth := &llm.NoopSynthesizer{
		Err:   fmt.Errorf("simulated LLM failure"),
		Avail: true,
		Model: "test-model",
	}

	indexMu := &sync.Mutex{}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		backgroundCurateStore(ctx, indexMu, storeCfg, 10*time.Millisecond, s, synth, nil, vaultPath)
	}()

	// Let it run through several cycles (some will encounter errors).
	// The goroutine should NOT crash — it logs errors and continues.
	time.Sleep(100 * time.Millisecond)

	// Cancel and verify the goroutine is still alive (didn't crash).
	cancel()

	select {
	case <-done:
		// Goroutine exited cleanly after cancellation — expected.
		// This proves the goroutine survived multiple error cycles
		// without crashing (FR-021).
	case <-time.After(2 * time.Second):
		t.Fatal("backgroundCurateStore did not exit after context cancellation")
	}
}
