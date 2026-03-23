package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/client"
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

	rootCmd := &cobra.Command{
		Use:     "dewey",
		Short:   "Knowledge graph MCP server & CLI",
		Long:    fmt.Sprintf("dewey %s — Knowledge graph MCP server & CLI", version),
		Version: version,
		// SilenceUsage prevents cobra from printing usage on every error.
		SilenceUsage: true,
		// SilenceErrors lets us handle error formatting ourselves.
		SilenceErrors: true,
		// RunE is the default action: start the MCP server.
		// This preserves backward compatibility — running `dewey` with no
		// subcommand starts the server, matching graphthulhu behavior.
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeServe(readOnly, backendType, vaultPath, dailyFolder, httpAddr)
		},
	}

	// Register serve flags on the root command so `dewey --backend obsidian`
	// works without the `serve` subcommand (backward compatible).
	rootCmd.Flags().BoolVar(&readOnly, "read-only", false, "Disable all write operations")
	rootCmd.Flags().StringVar(&backendType, "backend", "", "Backend type: logseq (default) or obsidian")
	rootCmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (required for obsidian backend)")
	rootCmd.Flags().StringVar(&dailyFolder, "daily-folder", "daily notes", "Daily notes subfolder name (obsidian only)")
	rootCmd.Flags().StringVar(&httpAddr, "http", "", "HTTP address to listen on (e.g. :8080)")

	// Add subcommands.
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newJournalCmd())
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// newServeCmd creates the `dewey serve` subcommand.
func newServeCmd() *cobra.Command {
	var readOnly bool
	var backendType string
	var vaultPath string
	var dailyFolder string
	var httpAddr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server",
		Long:  "Start the MCP server with stdio or HTTP transport.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeServe(readOnly, backendType, vaultPath, dailyFolder, httpAddr)
		},
	}

	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Disable all write operations")
	cmd.Flags().StringVar(&backendType, "backend", "", "Backend type: logseq (default) or obsidian")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (required for obsidian backend)")
	cmd.Flags().StringVar(&dailyFolder, "daily-folder", "daily notes", "Daily notes subfolder name (obsidian only)")
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP address to listen on (e.g. :8080)")

	return cmd
}

// executeServe contains the shared serve logic used by both the root command
// and the explicit `serve` subcommand.
func executeServe(readOnly bool, backendType, vaultPath, dailyFolder, httpAddr string) error {
	// Resolve backend from flag or environment.
	bt := backendType
	if bt == "" {
		bt = os.Getenv("DEWEY_BACKEND")
	}
	if bt == "" {
		bt = "logseq"
	}

	var b backend.Backend
	switch bt {
	case "obsidian":
		vp := vaultPath
		if vp == "" {
			vp = os.Getenv("OBSIDIAN_VAULT_PATH")
		}
		if vp == "" {
			return fmt.Errorf("--vault or OBSIDIAN_VAULT_PATH required for obsidian backend")
		}
		vc := vault.New(vp, vault.WithDailyFolder(dailyFolder))
		if err := vc.Load(); err != nil {
			return fmt.Errorf("failed to load vault: %w", err)
		}
		vc.BuildBacklinks()

		// Start file watcher.
		if err := vc.Watch(); err != nil {
			return fmt.Errorf("failed to start watcher: %w", err)
		}
		defer vc.Close()

		b = vc
	case "logseq":
		lsClient := client.New("", "")
		checkGraphVersionControl(lsClient)
		b = lsClient
	default:
		return fmt.Errorf("unknown backend %q (use logseq or obsidian)", bt)
	}

	srv := newServer(b, readOnly)

	if httpAddr != "" {
		// Streamable HTTP transport — serves multiple clients.
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return srv
		}, nil)
		logger.Info("listening", "addr", httpAddr)
		if err := http.ListenAndServe(httpAddr, handler); err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	} else {
		// Default: stdio transport for MCP client integration.
		if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			return fmt.Errorf("server error: %w", err)
		}
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
