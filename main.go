package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/vault"
)

var version = "dev"

// logger is the application-wide structured logger.
// Replaces fmt.Fprintf(os.Stderr, ...) per convention pack CS-008.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix: "dewey",
})

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
		// when --verbose is passed, affecting all three package loggers.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				logger.SetLevel(log.DebugLevel)
				vault.SetLogLevel(log.DebugLevel)
				source.SetLogLevel(log.DebugLevel)
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

	// Add subcommands.
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newJournalCmd())
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newIndexCmd())
	rootCmd.AddCommand(newSourceCmd())
	rootCmd.AddCommand(newDoctorCmd())

	return rootCmd
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
	bt := resolveBackendType(backendType)

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

	srv := newServer(b, readOnly, srvOpts...)

	// Set up signal handling for graceful shutdown (SIGINT, SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runServer(ctx, srv, httpAddr)
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
	vp := vaultPath
	if vp == "" {
		vp = os.Getenv("OBSIDIAN_VAULT_PATH")
	}
	if vp == "" {
		return nil, nil, nil, fmt.Errorf("--vault or OBSIDIAN_VAULT_PATH required for obsidian backend")
	}

	// Resolve to absolute path — vault.New requires absolute paths
	// for correct file walking and UUID seed generation.
	vp, err := filepath.Abs(vp)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve vault path: %w", err)
	}

	var srvOpts []serverOption

	// Initialize persistent store if .dewey/ directory exists.
	// The store is optional — Dewey works without it (backward compat).
	var opts []vault.Option
	opts = append(opts, vault.WithDailyFolder(dailyFolder))

	var persistentStore *store.Store
	deweyDir := filepath.Join(vp, ".dewey")
	if _, err := os.Stat(deweyDir); err == nil {
		dbPath := filepath.Join(deweyDir, "graph.db")
		s, err := store.New(dbPath)
		if err != nil {
			logger.Warn("failed to open persistent store, continuing without persistence",
				"path", dbPath, "err", err)
		} else {
			persistentStore = s
			opts = append(opts, vault.WithStore(s))
			srvOpts = append(srvOpts, WithPersistentStore(s))
			logger.Info("persistent store opened", "path", dbPath)
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
		embedder = embed.NewOllamaEmbedder(embedEndpoint, embedModel)
		if !embedder.Available() {
			return nil, nil, nil, fmt.Errorf("embedding model %q not available at %s\n\nTo fix:\n  ollama pull %s\n\nTo skip embeddings:\n  dewey serve --no-embeddings",
				embedModel, embedEndpoint, embedModel)
		}
		logger.Info("embedding model available", "model", embedModel)
		srvOpts = append(srvOpts, WithEmbedder(embedder))
	}

	vc := vault.New(vp, opts...)

	// Configure embedder on the vault store for indexing pipeline integration.
	if embedder != nil {
		if vs := vc.Store(); vs != nil {
			vs.SetEmbedder(embedder)
		}
	}

	// Index the vault — persistent (incremental) or in-memory.
	if err := indexVault(vc); err != nil {
		// Close store on error — caller won't get the cleanup func.
		if persistentStore != nil {
			_ = persistentStore.Close()
		}
		return nil, nil, nil, err
	}

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
	if err := vc.Watch(); err != nil {
		if persistentStore != nil {
			_ = persistentStore.Close()
		}
		return nil, nil, nil, fmt.Errorf("failed to start watcher: %w", err)
	}

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
