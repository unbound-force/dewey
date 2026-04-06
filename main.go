package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/client"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/ignore"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/vault"
)

var version = "dev"

// deweyWorkspaceDir is the workspace directory name relative to the vault root.
// All Dewey runtime artifacts (graph.db, dewey.log, dewey.lock) live here.
// Uses the .uf/<tool>/ namespace per ecosystem convention (D1).
const deweyWorkspaceDir = ".uf/dewey"

// logger is the application-wide structured logger.
// Replaces fmt.Fprintf(os.Stderr, ...) per convention pack CS-008.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// fileLoggingEnabled tracks whether setupFileLogging has been called.
// Prevents double-setup when both --log-file and auto-serve logging apply.
var fileLoggingEnabled bool

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		logger.Error(err)
		os.Exit(1)
	}
}

// newRootCmd creates the root cobra command. When invoked without a subcommand,
// it starts the MCP server (backward compatible with graphthulhu behavior).
func newRootCmd() *cobra.Command {
	// Serve flags — declared at root level because the root command
	// doubles as the serve command for backward compatibility.
	var readOnly bool
	var backendType string
	var vaultPath string
	var dailyFolder string
	var httpAddr string
	var noEmbeddings bool
	var verbose bool
	var logFile string

	rootCmd := &cobra.Command{
		Use:   "dewey",
		Short: "Knowledge graph MCP server & CLI",
		Long:  fmt.Sprintf("dewey %s — Knowledge graph MCP server & CLI", version),
		// NOTE: Version is NOT set here to avoid Cobra's auto --version/-v flag
		// conflicting with our --verbose/-v persistent flag. Version is available
		// via the `dewey version` subcommand instead.
		// SilenceUsage prevents cobra from printing usage on every error.
		SilenceUsage: true,
		// SilenceErrors lets us handle error formatting ourselves.
		SilenceErrors: true,
		// PersistentPreRunE runs before any subcommand — sets debug logging
		// when --verbose is passed and configures file logging if --log-file is set.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				logger.SetLevel(log.DebugLevel)
				vault.SetLogLevel(log.DebugLevel)
				source.SetLogLevel(log.DebugLevel)
				ignore.SetLogLevel(log.DebugLevel)
				store.SetLogLevel(log.DebugLevel)
			}
			if logFile != "" {
				if err := setupFileLogging(logFile, verbose); err != nil {
					return fmt.Errorf("setup log file: %w", err)
				}
			}
			return nil
		},
		// RunE is the default action: start the MCP server.
		// This preserves backward compatibility — running `dewey` with no
		// subcommand starts the server, matching graphthulhu behavior.
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeServe(readOnly, backendType, vaultPath, dailyFolder, httpAddr, noEmbeddings)
		},
	}

	// Register serve flags on the root command so `dewey --backend obsidian`
	// works without the `serve` subcommand (backward compatible).
	rootCmd.Flags().BoolVar(&readOnly, "read-only", false, "Disable all write operations")
	rootCmd.Flags().StringVar(&backendType, "backend", "", "Backend type: obsidian (default) or logseq")
	rootCmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (required for obsidian backend)")
	rootCmd.Flags().StringVar(&dailyFolder, "daily-folder", "daily notes", "Daily notes subfolder name (obsidian only)")
	rootCmd.Flags().StringVar(&httpAddr, "http", "", "HTTP address to listen on (e.g. :8080)")
	rootCmd.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip embedding generation (disables semantic search)")

	// Persistent flags — inherited by all subcommands.
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Write logs to file (in addition to stderr)")

	// Add subcommands.
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newJournalCmd())
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newIndexCmd())
	rootCmd.AddCommand(newReindexCmd())
	rootCmd.AddCommand(newSourceCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newManifestCmd())

	return rootCmd
}

// setupFileLogging configures all package loggers to write to both stderr
// and the specified file. This is critical for diagnosing MCP server issues
// since the server runs as a child process of OpenCode with no visible stderr.
// maxLogSize is the threshold (10 MB) above which the log file is truncated
// on startup to prevent unbounded growth.
const maxLogSize = 10 * 1024 * 1024

func setupFileLogging(path string, verbose bool) error {
	// Truncate if the log file exceeds the size threshold.
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		if err := os.Truncate(path, 0); err == nil {
			// Write a marker so the user knows truncation occurred.
			_ = os.WriteFile(path, []byte("--- log truncated (exceeded 10 MB) ---\n"), 0o600)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	// Note: we intentionally don't close this file — it stays open for the
	// lifetime of the process. The OS reclaims it on exit.

	multi := io.MultiWriter(os.Stderr, f)
	level := log.InfoLevel
	if verbose {
		level = log.DebugLevel
	}

	// Replace all package loggers with multi-writer versions.
	newLogger := log.NewWithOptions(multi, log.Options{
		Prefix:          "dewey",
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	})
	*logger = *newLogger
	vault.SetLogOutput(multi, level)
	source.SetLogOutput(multi, level)
	ignore.SetLogOutput(multi, level)
	store.SetLogOutput(multi, level)

	fileLoggingEnabled = true
	logger.Info("file logging enabled", "path", path)
	return nil
}

// newServeCmd creates the `dewey serve` subcommand.
func newServeCmd() *cobra.Command {
	var readOnly bool
	var backendType string
	var vaultPath string
	var dailyFolder string
	var httpAddr string
	var noEmbeddings bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server",
		Long:  "Start the MCP server with stdio or HTTP transport.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeServe(readOnly, backendType, vaultPath, dailyFolder, httpAddr, noEmbeddings)
		},
	}

	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Disable all write operations")
	cmd.Flags().StringVar(&backendType, "backend", "", "Backend type: obsidian (default) or logseq")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (required for obsidian backend)")
	cmd.Flags().StringVar(&dailyFolder, "daily-folder", "daily notes", "Daily notes subfolder name (obsidian only)")
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP address to listen on (e.g. :8080)")
	cmd.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip embedding generation (disables semantic search)")

	return cmd
}

// executeServe contains the shared serve logic used by both the root command
// and the explicit `serve` subcommand. It acts as a thin orchestrator,
// delegating backend initialization, server creation, and transport to
// focused helper functions (decomposed per plan.md T009).
func executeServe(readOnly bool, backendType, vaultPath, dailyFolder, httpAddr string, noEmbeddings bool) error {
	serveStart := time.Now()
	bt := resolveBackendType(backendType)

	// Auto-enable file logging for serve if .uf/dewey/ exists and --log-file
	// wasn't already explicitly set. MCP servers run as child processes of AI
	// agents with no visible stderr — the log file is the only diagnostic output.
	if !fileLoggingEnabled {
		if vp, err := resolveVaultPath(vaultPath); err == nil {
			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); err == nil {
				logPath := filepath.Join(deweyDir, "dewey.log")
				if err := setupFileLogging(logPath, logger.GetLevel() == log.DebugLevel); err != nil {
					logger.Warn("auto-log setup failed", "path", logPath, "err", err)
				}
			}
		}
	}
	logger.Info("serve starting", "version", version, "backend", bt, "pid", os.Getpid())

	var b backend.Backend
	var srvOpts []serverOption

	switch bt {
	case "obsidian":
		ob, opts, cleanup, err := initObsidianBackend(vaultPath, dailyFolder, noEmbeddings)
		if err != nil {
			return err
		}
		defer cleanup()
		b = ob
		srvOpts = opts
	case "logseq":
		b = initLogseqBackend()
	default:
		return fmt.Errorf("unknown backend %q (use logseq or obsidian)", bt)
	}

	srv, toolCount := newServer(b, readOnly, srvOpts...)

	// Set up signal handling for graceful shutdown (SIGINT, SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Determine transport type for the "server ready" log line.
	transportType := "stdio"
	if httpAddr != "" {
		transportType = "http"
	}
	logger.Info("server ready", "transport", transportType, "tools", toolCount, "startup", time.Since(serveStart))

	if err := runServer(ctx, srv, httpAddr); err != nil {
		return err
	}

	// Log clean shutdown for stdio transport. HTTP transport already logs
	// "shutting down HTTP server" in its shutdown goroutine.
	if httpAddr == "" {
		logger.Info("server stopped", "transport", "stdio")
	}
	return nil
}

// resolveVaultPath resolves the vault path from a flag value, falling back
// to the OBSIDIAN_VAULT_PATH environment variable. Returns the absolute path.
// This is the SINGLE entry point for vault path resolution — all commands
// that accept a --vault flag must use this to prevent the relative path bug
// (vault.New requires absolute paths for file walking and UUID generation).
func resolveVaultPath(flagValue string) (string, error) {
	vp := flagValue
	if vp == "" {
		vp = os.Getenv("OBSIDIAN_VAULT_PATH")
	}
	if vp == "" {
		return "", fmt.Errorf("--vault or OBSIDIAN_VAULT_PATH required")
	}
	abs, err := filepath.Abs(vp)
	if err != nil {
		return "", fmt.Errorf("resolve vault path %q: %w", vp, err)
	}
	return abs, nil
}

// resolveVaultPathOrCwd resolves the vault path from a flag value, falling
// back to OBSIDIAN_VAULT_PATH env var, then to the current working directory.
// This is used by commands that operate on the .uf/dewey/ directory (index,
// reindex, status, doctor) where CWD is a reasonable default.
func resolveVaultPathOrCwd(flagValue string) (string, error) {
	vp := flagValue
	if vp == "" {
		vp = os.Getenv("OBSIDIAN_VAULT_PATH")
	}
	if vp == "" {
		vp = "."
	}
	abs, err := filepath.Abs(vp)
	if err != nil {
		return "", fmt.Errorf("resolve vault path %q: %w", vp, err)
	}
	return abs, nil
}

// resolveBackendType determines the backend type from the flag value,
// falling back to the DEWEY_BACKEND environment variable, then to "obsidian".
func resolveBackendType(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("DEWEY_BACKEND"); env != "" {
		return env
	}
	return "obsidian"
}

// OllamaState represents the lifecycle state of the Ollama embedding server.
type OllamaState int

const (
	// OllamaExternal indicates Ollama was already running (not started by Dewey).
	OllamaExternal OllamaState = iota
	// OllamaManaged indicates Ollama was auto-started by Dewey as a subprocess.
	OllamaManaged
	// OllamaUnavailable indicates Ollama is not running and could not be started.
	OllamaUnavailable
)

// String returns a human-readable label for the Ollama state.
func (s OllamaState) String() string {
	switch s {
	case OllamaExternal:
		return "external"
	case OllamaManaged:
		return "managed"
	case OllamaUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// ollamaStarter abstracts Ollama subprocess launching for testability.
type ollamaStarter interface {
	Start() error
}

// execOllamaStarter starts Ollama via os/exec with process group detachment.
// FR-004: Dewey starts Ollama but does not own its shutdown lifecycle.
type execOllamaStarter struct{}

func (s *execOllamaStarter) Start() error {
	cmd := exec.Command("ollama", "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start() // Start, not Run — don't wait for exit
}

// isLocalEndpoint reports whether the given endpoint URL refers to the
// local machine. Auto-start is only attempted for local endpoints.
func isLocalEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == ""
}

// ollamaHealthCheck reports whether Ollama is reachable and healthy at
// the given endpoint. It sends GET /api/tags with a 2-second timeout.
func ollamaHealthCheck(endpoint string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// ensureOllama checks if Ollama is running and optionally auto-starts it.
// When autoStart is false (e.g., dewey doctor), it only probes without
// starting a subprocess.
func ensureOllama(endpoint string, autoStart bool, starter ollamaStarter) (OllamaState, error) {
	// Step 1: Check if already running.
	if ollamaHealthCheck(endpoint) {
		return OllamaExternal, nil
	}

	// Step 2: If auto-start disabled or remote endpoint, report unavailable.
	if !autoStart || !isLocalEndpoint(endpoint) {
		return OllamaUnavailable, nil
	}

	// Step 3: Check if ollama binary is in PATH.
	if _, err := exec.LookPath("ollama"); err != nil {
		return OllamaUnavailable, nil
	}

	// Step 4: Start Ollama subprocess.
	if err := starter.Start(); err != nil {
		return OllamaUnavailable, fmt.Errorf("start ollama: %w", err)
	}
	logger.Info("auto-starting Ollama")

	// Step 5: Poll for readiness with bounded timeout.
	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 30 * time.Second
		logInterval  = 5 * time.Second
	)
	start := time.Now()
	lastLog := start
	deadline := start.Add(maxWait)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		if ollamaHealthCheck(endpoint) {
			logger.Info("Ollama is ready", "state", OllamaManaged)
			return OllamaManaged, nil
		}
		if time.Since(lastLog) >= logInterval {
			logger.Debug("waiting for Ollama", "elapsed", time.Since(start).Truncate(time.Second), "timeout", maxWait)
			lastLog = time.Now()
		}
	}

	return OllamaUnavailable, fmt.Errorf("ollama did not become ready within %s", maxWait)
}

// initObsidianBackend initializes the Obsidian/vault backend including:
//   - Vault path resolution from flag or OBSIDIAN_VAULT_PATH env var
//   - Persistent store initialization (optional, graceful degradation)
//   - Embedder initialization (hard error if unavailable, unless noEmbeddings is true)
//   - Vault creation and indexing (incremental or full)
//   - File watcher startup
//
// Returns the backend, server options, a cleanup func (for defers), and error.
// The cleanup func closes the store and vault client — callers must defer it.
func initObsidianBackend(vaultPath, dailyFolder string, noEmbeddings bool) (backend.Backend, []serverOption, func(), error) {
	vp, err := resolveVaultPath(vaultPath)
	if err != nil {
		return nil, nil, nil, err
	}
	logger.Info("vault path resolved", "path", vp)

	var srvOpts []serverOption

	// Initialize persistent store if .uf/dewey/ directory exists.
	// The store is optional — Dewey works without it (backward compat).
	var opts []vault.Option
	opts = append(opts, vault.WithDailyFolder(dailyFolder))

	// Read ignore patterns from sources.yaml disk-local config (T026).
	// This connects the sources.yaml "ignore" field to the vault walker
	// at serve time, so patterns configured via `dewey source` are
	// applied during incremental indexing and file watching.
	deweyDir := filepath.Join(vp, deweyWorkspaceDir)
	sourcesPath := filepath.Join(deweyDir, "sources.yaml")
	if configs, err := source.LoadSourcesConfig(sourcesPath); err == nil {
		for _, cfg := range configs {
			if cfg.Type != "disk" {
				continue
			}
			// Match the disk-local source: either by ID "disk-local" or
			// by being the first disk source with path ".".
			pathVal, _ := cfg.Config["path"].(string)
			if cfg.ID == "disk-local" || pathVal == "." {
				if patterns := extractIgnorePatterns(cfg.Config); len(patterns) > 0 {
					opts = append(opts, vault.WithIgnorePatterns(patterns))
					logger.Info("vault ignore patterns loaded from sources.yaml",
						"source", cfg.ID, "patterns", len(patterns))
				}
				break
			}
		}
	}

	var persistentStore *store.Store
	if _, err := os.Stat(deweyDir); err == nil {
		dbPath := filepath.Join(deweyDir, "graph.db")
		storeStart := time.Now()
		s, err := store.New(dbPath)
		if err != nil {
			logger.Warn("failed to open persistent store, continuing without persistence",
				"path", dbPath, "err", err)
		} else {
			persistentStore = s
			opts = append(opts, vault.WithStore(s))
			srvOpts = append(srvOpts, WithPersistentStore(s))
			logger.Info("persistent store opened", "path", dbPath, "elapsed", time.Since(storeStart))
		}
	}

	// Initialize embedder based on --no-embeddings flag.
	// When noEmbeddings is true, skip embedder creation entirely.
	// When false, require the embedding model to be available (hard error).
	embedModel := os.Getenv("DEWEY_EMBEDDING_MODEL")
	embedEndpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT")
	if embedModel == "" {
		embedModel = "granite-embedding:30m"
	}
	if embedEndpoint == "" {
		embedEndpoint = "http://localhost:11434"
	}

	var embedder *embed.OllamaEmbedder
	if noEmbeddings {
		logger.Info("embeddings disabled via --no-embeddings")
	} else {
		// Ensure Ollama is running (auto-start if needed).
		ollamaStart := time.Now()
		ollamaState, err := ensureOllama(embedEndpoint, true, &execOllamaStarter{})
		if err != nil {
			logger.Warn("ollama auto-start failed, continuing without embeddings", "err", err)
		}
		logger.Info("ollama state", "state", ollamaState, "endpoint", embedEndpoint, "elapsed", time.Since(ollamaStart))

		if ollamaState == OllamaUnavailable {
			// Graceful degradation: keyword-only mode when Ollama is not installed.
			logger.Info("semantic search unavailable — ollama not installed",
				"install", "brew install ollama")
		} else {
			// Ollama is running (External or Managed) — check model availability.
			// This remains a hard error: the user has Ollama but hasn't pulled the model.
			embedder = embed.NewOllamaEmbedder(embedEndpoint, embedModel)
			if !embedder.Available() {
				return nil, nil, nil, fmt.Errorf("embedding model %q not available at %s\n\nTo fix:\n  ollama pull %s\n\nTo skip embeddings:\n  dewey serve --no-embeddings",
					embedModel, embedEndpoint, embedModel)
			}
			logger.Info("embedding model available", "model", embedModel)
			srvOpts = append(srvOpts, WithEmbedder(embedder))
		}
	}

	vc := vault.New(vp, opts...)

	// Configure embedder on the vault store for indexing pipeline integration.
	if embedder != nil {
		if vs := vc.Store(); vs != nil {
			vs.SetEmbedder(embedder)
		}
	}

	// Index the vault — persistent (incremental) or in-memory.
	indexStart := time.Now()
	if err := indexVault(vc); err != nil {
		// Close store on error — caller won't get the cleanup func.
		if persistentStore != nil {
			_ = persistentStore.Close()
		}
		return nil, nil, nil, err
	}
	logger.Info("vault indexed", "elapsed", time.Since(indexStart))

	// Load external-source pages from store into the vault's in-memory index.
	// This must happen after indexVault() (which loads local pages) but before
	// BuildBacklinks() is called implicitly by the watcher, so external pages
	// participate in backlink and search index construction (FR-005, T022).
	if vs := vc.Store(); vs != nil {
		extCount, err := vs.LoadExternalPages(vc)
		if err != nil {
			logger.Warn("failed to load external pages", "err", err)
		} else if extCount > 0 {
			// Rebuild backlinks and search index to include external pages.
			vc.BuildBacklinks()
			logger.Info("external pages loaded into vault", "count", extCount)
		}
	}

	// Start file watcher.
	watchStart := time.Now()
	if err := vc.Watch(); err != nil {
		if persistentStore != nil {
			_ = persistentStore.Close()
		}
		return nil, nil, nil, fmt.Errorf("failed to start watcher: %w", err)
	}
	logger.Info("file watcher started", "elapsed", time.Since(watchStart))

	// Build cleanup func that closes vault client and persistent store.
	// Order matters: close vault first (stops watcher), then store.
	cleanup := func() {
		_ = vc.Close()
		if persistentStore != nil {
			_ = persistentStore.Close()
		}
	}

	return vc, srvOpts, cleanup, nil
}

// indexVault performs vault indexing using the appropriate strategy:
//   - If a persistent store is available, attempts incremental indexing first,
//     falling back to full re-index on validation failure or incremental error.
//   - If no store is available, uses in-memory-only loading with backlink building.
func indexVault(vc *vault.Client) error {
	vs := vc.Store()
	if vs != nil {
		// Use persistent indexing if store is available.
		if err := vs.ValidateStore(); err != nil {
			// Corruption detected — fall back to full re-index.
			logger.Warn("store validation failed, performing full re-index",
				"err", err)
			if err := vs.FullIndex(vc); err != nil {
				return fmt.Errorf("failed to full-index vault: %w", err)
			}
		} else {
			// Incremental index — load from store, re-index only changes.
			stats, err := vs.IncrementalIndex(vc)
			if err != nil {
				logger.Warn("incremental index failed, falling back to full index",
					"err", err)
				if err := vs.FullIndex(vc); err != nil {
					return fmt.Errorf("failed to full-index vault: %w", err)
				}
			} else {
				logger.Info("incremental index complete",
					"new", stats.New,
					"changed", stats.Changed,
					"deleted", stats.Deleted,
					"unchanged", stats.Unchanged,
				)
			}
		}
	} else {
		// No store — use existing in-memory-only behavior.
		if err := vc.Load(); err != nil {
			return fmt.Errorf("failed to load vault: %w", err)
		}
		vc.BuildBacklinks()
	}
	return nil
}

// initLogseqBackend initializes the Logseq backend by creating a client
// and checking whether the graph is under version control.
func initLogseqBackend() backend.Backend {
	lsClient := client.New("", "")
	checkGraphVersionControl(lsClient)
	return lsClient
}

// runServer runs the MCP server with either HTTP or stdio transport.
// For HTTP, it sets up graceful shutdown on context cancellation.
// For stdio, it passes the cancellable context directly to srv.Run.
func runServer(ctx context.Context, srv *mcp.Server, httpAddr string) error {
	if httpAddr != "" {
		// Streamable HTTP transport — serves multiple clients.
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return srv
		}, nil)

		httpSrv := &http.Server{
			Addr:    httpAddr,
			Handler: handler,
		}

		// Graceful shutdown: listen for context cancellation in a goroutine.
		go func() {
			<-ctx.Done()
			logger.Info("shutting down HTTP server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpSrv.Shutdown(shutdownCtx); err != nil {
				logger.Warn("HTTP server shutdown error", "err", err)
			}
		}()

		logger.Info("listening", "addr", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}

	// Default: stdio transport for MCP client integration.
	// Pass cancellable context so SIGINT/SIGTERM trigger clean shutdown.
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// newVersionCmd creates the `dewey version` subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(version)
		},
	}
}

// extractIgnorePatterns extracts the "ignore" field from a source config map
// as a string slice. YAML parsing delivers list values as []any, so this
// function handles the type assertion. Returns nil if the field is absent
// or not a list.
func extractIgnorePatterns(config map[string]any) []string {
	raw, ok := config["ignore"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		// Single string value.
		if s, ok := raw.(string); ok {
			return []string{s}
		}
		return nil
	}
	var patterns []string
	for _, item := range items {
		if s, ok := item.(string); ok {
			patterns = append(patterns, s)
		}
	}
	return patterns
}

// checkGraphVersionControl warns if the Logseq graph is not git-controlled.
// Best-effort: silently skips if Logseq is not running or the API is unreachable.
func checkGraphVersionControl(c *client.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	graph, err := c.GetCurrentGraph(ctx)
	if err != nil || graph == nil || graph.Path == "" {
		return
	}

	gitDir := filepath.Join(graph.Path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		logger.Warn("graph is not version controlled",
			"graph", graph.Name,
			"path", graph.Path,
		)
		logger.Warn("write operations cannot be undone",
			"suggestion", fmt.Sprintf("cd %s && git init && git add -A && git commit -m 'initial'", graph.Path),
		)
	}
}
