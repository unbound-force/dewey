package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/client"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/parser"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// newJournalCmd creates the `dewey journal` subcommand.
// Appends a block to today's (or a specified date's) journal page.
func newJournalCmd() *cobra.Command {
	var date string

	cmd := &cobra.Command{
		Use:   "journal [flags] TEXT",
		Short: "Append block to today's journal",
		Long: `Appends a block to a Logseq journal page.
Prints the created block UUID on success.

Content can be provided as arguments or piped via stdin:
  dewey journal "my note"
  echo "my note" | dewey journal`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New("", "")
			content := readContentFromArgs(args)
			if content == "" {
				return fmt.Errorf("no content provided")
			}

			var t time.Time
			if date != "" {
				var err error
				t, err = time.Parse("2006-01-02", date)
				if err != nil {
					return fmt.Errorf("invalid date %q (use YYYY-MM-DD)", date)
				}
			} else {
				t = time.Now()
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pageName := findJournalPage(ctx, c, t)
			if pageName == "" {
				// No existing page found — use ordinal format (most common Logseq default).
				pageName = ordinalDate(t)
			}

			block, err := c.AppendBlockInPage(ctx, pageName, content)
			if err != nil {
				return fmt.Errorf("journal: %w", err)
			}

			if block != nil {
				fmt.Println(block.UUID)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&date, "date", "d", "", "Journal date (YYYY-MM-DD). Default: today")

	return cmd
}

// newAddCmd creates the `dewey add` subcommand.
// Appends a block to a named page.
func newAddCmd() *cobra.Command {
	var page string

	cmd := &cobra.Command{
		Use:   "add [flags] TEXT",
		Short: "Append block to a page",
		Long: `Appends a block to a Logseq page (creates page if needed).
Prints the created block UUID on success.

Content can be provided as arguments or piped via stdin:
  dewey add -p "My Page" "content here"
  echo "content" | dewey add --page "My Page"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if page == "" {
				return fmt.Errorf("--page is required")
			}

			c := client.New("", "")
			content := readContentFromArgs(args)
			if content == "" {
				return fmt.Errorf("no content provided")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			block, err := c.AppendBlockInPage(ctx, page, content)
			if err != nil {
				return fmt.Errorf("add: %w", err)
			}

			if block != nil {
				fmt.Println(block.UUID)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&page, "page", "p", "", "Page name (required)")

	return cmd
}

// newSearchCmd creates the `dewey search` subcommand.
// Performs full-text search using the vault backend (same data path as dewey serve).
func newSearchCmd() *cobra.Command {
	var limit int
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "search [flags] QUERY",
		Short: "Full-text search across the graph",
		Long:  "Full-text search across all blocks in the knowledge graph.",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			if query == "" {
				return fmt.Errorf("query is required")
			}

			// Resolve vault path using the shared resolver.
			vp, err := resolveVaultPath(vaultPath)
			if err != nil {
				return err
			}

			// Create vault client and load local .md files.
			var opts []vault.Option
			vc := vault.New(vp, opts...)
			if err := vc.Load(); err != nil {
				return fmt.Errorf("search: load vault: %w", err)
			}

			// If persistent store exists, load external-source pages from graph.db.
			deweyDir := filepath.Join(vp, ".dewey")
			if _, err := os.Stat(deweyDir); err == nil {
				dbPath := filepath.Join(deweyDir, "graph.db")
				s, err := store.New(dbPath)
				if err == nil {
					defer func() { _ = s.Close() }()
					vs := vault.NewVaultStore(s, vp, "disk-local")
					if n, err := vs.LoadExternalPages(vc); err != nil {
						logger.Warn("failed to load external pages", "err", err)
					} else if n > 0 {
						logger.Info("loaded external pages", "count", n)
					}
				}
			}

			// Build backlinks and search index.
			vc.BuildBacklinks()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			hits, err := vc.FullTextSearch(ctx, query, limit)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			if len(hits) == 0 {
				return fmt.Errorf("no results for %q", query)
			}

			for _, hit := range hits {
				fmt.Printf("%s | %s\n", hit.PageName, strings.ReplaceAll(hit.Content, "\n", " "))
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault")

	return cmd
}

// newInitCmd creates the `dewey init` subcommand.
// Initializes a .dewey/ directory with default configuration.
// Idempotent — running twice does not error (per CLI contract).
func newInitCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Dewey configuration",
		Long:  "Create .dewey/ directory with default config.yaml and sources.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultPath == "" {
				var err error
				vaultPath, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
			}

			deweyDir := filepath.Join(vaultPath, ".dewey")

			// Check if already initialized (idempotent).
			if _, err := os.Stat(deweyDir); err == nil {
				logger.Info("already initialized", "path", deweyDir)
				return nil
			}

			// Create .dewey/ directory.
			if err := os.MkdirAll(deweyDir, 0o755); err != nil {
				return fmt.Errorf("create .dewey/ directory: %w", err)
			}

			// Write default config.yaml.
			configPath := filepath.Join(deweyDir, "config.yaml")
			configContent := `# Dewey configuration
# See: https://github.com/unbound-force/dewey

embedding:
  model: granite-embedding:30m
  endpoint: http://localhost:11434
`
			if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
				return fmt.Errorf("write config.yaml: %w", err)
			}

			// Write default sources.yaml.
			sourcesPath := filepath.Join(deweyDir, "sources.yaml")
			sourcesContent := `# Dewey content sources
# Each source provides documents for the knowledge graph index.

sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "."
`
			if err := os.WriteFile(sourcesPath, []byte(sourcesContent), 0o644); err != nil {
				return fmt.Errorf("write sources.yaml: %w", err)
			}

			// Append .dewey/ to .gitignore if it exists and doesn't already contain it.
			gitignorePath := filepath.Join(vaultPath, ".gitignore")
			if _, err := os.Stat(gitignorePath); err == nil {
				content, err := os.ReadFile(gitignorePath)
				if err == nil && !strings.Contains(string(content), ".dewey/") {
					f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
					if err == nil {
						defer func() { _ = f.Close() }()
						// Ensure we start on a new line.
						if len(content) > 0 && content[len(content)-1] != '\n' {
							_, _ = f.WriteString("\n")
						}
						_, _ = f.WriteString(".dewey/\n")
					}
				}
			}

			logger.Info("initialized", "path", deweyDir)
			logger.Info("default config", "file", configPath)
			logger.Info("run 'dewey index' to build the initial index")

			return nil
		},
	}

	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to the vault root (default: current directory)")

	return cmd
}

// sourceStatus holds per-source metadata for status reporting.
type sourceStatus struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	PageCount   int    `json:"pageCount"`
	LastFetched string `json:"lastFetched,omitempty"`
	Error       string `json:"error,omitempty"`
}

// statusData holds all data needed to render the status output.
// Separates data collection from formatting to reduce cyclomatic complexity.
type statusData struct {
	PageCount          int
	BlockCount         int
	EmbeddingCount     int
	EmbeddingModel     string
	EmbeddingAvailable bool
	Sources            []sourceStatus
	IndexPath          string
}

// embeddingCoverage computes the percentage of blocks with embeddings.
func (d statusData) embeddingCoverage() float64 {
	if d.BlockCount > 0 {
		return float64(d.EmbeddingCount) / float64(d.BlockCount) * 100
	}
	return 0
}

// newStatusCmd creates the `dewey status` subcommand.
// Reports index health: page count, block count, source info.
// Supports --json flag for structured output.
func newStatusCmd() *cobra.Command {
	var jsonOutput bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Report index status",
		Long:         "Show Dewey index health: page count, block count, source info, and index path.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, ".dewey")
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			data, err := queryStoreStatus(deweyDir)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			if jsonOutput {
				return formatStatusJSON(data, w)
			}
			return formatStatusText(data, w)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// readEmbeddingModel extracts the embedding model name from config.yaml
// using simple line parsing to avoid a YAML dependency for status display.
func readEmbeddingModel(deweyDir string) string {
	configPath := filepath.Join(deweyDir, "config.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(configData), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "model:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "model:"))
		}
	}
	return ""
}

// queryStoreStatus opens the store at deweyDir, queries all counts and source
// records, and returns a populated statusData. The store is closed before
// returning. Returns a zero-value statusData (with IndexPath set) if the
// database does not yet exist.
func queryStoreStatus(deweyDir string) (statusData, error) {
	data := statusData{
		IndexPath:      deweyDir,
		EmbeddingModel: readEmbeddingModel(deweyDir),
	}

	dbPath := filepath.Join(deweyDir, "graph.db")
	if _, err := os.Stat(dbPath); err != nil {
		// Database does not exist yet — return zero counts.
		return data, nil
	}

	s, err := store.New(dbPath)
	if err != nil {
		return data, fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	pages, err := s.ListPages()
	if err != nil {
		return data, fmt.Errorf("list pages: %w", err)
	}
	data.PageCount = len(pages)

	if bc, err := s.CountBlocks(); err == nil {
		data.BlockCount = bc
	}
	if ec, err := s.CountEmbeddings(); err == nil {
		data.EmbeddingCount = ec
	}

	storedSources, _ := s.ListSources()
	for _, src := range storedSources {
		ss := sourceStatus{
			ID:     src.ID,
			Type:   src.Type,
			Status: src.Status,
			Error:  src.ErrorMessage,
		}
		pc, _ := s.CountPagesBySource(src.ID)
		ss.PageCount = pc
		if src.LastFetchedAt > 0 {
			elapsed := time.Since(time.UnixMilli(src.LastFetchedAt))
			ss.LastFetched = formatDuration(elapsed)
		}
		data.Sources = append(data.Sources, ss)
	}

	return data, nil
}

// formatStatusText writes human-readable status output to w.
func formatStatusText(data statusData, w io.Writer) error {
	_, _ = fmt.Fprintln(w, "Dewey Index Status")
	_, _ = fmt.Fprintf(w, "  Path:       %s\n", data.IndexPath)
	_, _ = fmt.Fprintf(w, "  Pages:      %d\n", data.PageCount)
	_, _ = fmt.Fprintf(w, "  Blocks:     %d\n", data.BlockCount)
	_, _ = fmt.Fprintf(w, "  Embeddings: %d\n", data.EmbeddingCount)
	if data.EmbeddingModel != "" {
		_, _ = fmt.Fprintf(w, "  Model:      %s\n", data.EmbeddingModel)
	}
	_, _ = fmt.Fprintf(w, "  Coverage:   %.1f%%\n", data.embeddingCoverage())

	if len(data.Sources) > 0 {
		_, _ = fmt.Fprintln(w, "\nSources")
		for _, src := range data.Sources {
			lastFetched := "never"
			if src.LastFetched != "" {
				lastFetched = src.LastFetched + " ago"
			}
			if src.Error != "" {
				_, _ = fmt.Fprintf(w, "  %-15s %-8s %3d pages  %s  error: %s\n",
					src.ID, src.Status, src.PageCount, lastFetched, src.Error)
			} else {
				_, _ = fmt.Fprintf(w, "  %-15s %-8s %3d pages  %s\n",
					src.ID, src.Status, src.PageCount, lastFetched)
			}
		}
	}

	return nil
}

// formatStatusJSON writes JSON-formatted status output to w.
func formatStatusJSON(data statusData, w io.Writer) error {
	status := map[string]any{
		"path":               data.IndexPath,
		"pages":              data.PageCount,
		"blocks":             data.BlockCount,
		"embeddings":         data.EmbeddingCount,
		"embeddingModel":     data.EmbeddingModel,
		"embeddingAvailable": data.EmbeddingAvailable,
		"embeddingCoverage":  data.embeddingCoverage(),
		"sources":            data.Sources,
	}
	out, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	_, _ = fmt.Fprintln(w, string(out))
	return nil
}

// --- Helpers ---

// readContentFromArgs gets content from positional args or stdin (if piped).
func readContentFromArgs(args []string) string {
	if len(args) > 0 {
		return strings.Join(args, " ")
	}

	// Only read stdin if it's piped (not a terminal).
	stat, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "" // stdin is a terminal, not piped
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// findJournalPage tries common Logseq journal date formats to find an existing page.
func findJournalPage(ctx context.Context, c *client.Client, t time.Time) string {
	names := []string{
		ordinalDate(t),
		t.Format("2006-01-02"),
		t.Format("January 2, 2006"),
	}

	for _, name := range names {
		page, err := c.GetPage(ctx, name)
		if err == nil && page != nil {
			return name
		}
	}
	return ""
}

// ordinalDate formats a time as "Jan 29th, 2026" (common Logseq journal default).
func ordinalDate(t time.Time) string {
	day := t.Day()
	suffix := "th"
	switch day {
	case 1, 21, 31:
		suffix = "st"
	case 2, 22:
		suffix = "nd"
	case 3, 23:
		suffix = "rd"
	}
	return fmt.Sprintf("%s %d%s, %d", t.Format("Jan"), day, suffix, t.Year())
}

// printSearchResults recursively prints matching blocks to stdout.
func printSearchResults(blocks []types.BlockEntity, query, pageName string, limit int, found *int) {
	for _, b := range blocks {
		if *found >= limit {
			return
		}
		if strings.Contains(strings.ToLower(b.Content), query) {
			fmt.Printf("%s | %s\n", pageName, b.Content)
			*found++
		}
		if len(b.Children) > 0 {
			printSearchResults(b.Children, query, pageName, limit, found)
		}
	}
}

// formatDuration formats a duration as a human-readable string (e.g., "2m", "4h", "3d").
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// --- Index command (T050) ---

// newIndexCmd creates the `dewey index` subcommand.
// Builds or updates the knowledge graph and embedding indexes.
// Per contracts/cli-commands.md.
func newIndexCmd() *cobra.Command {
	var sourceName string
	var force bool
	var noEmbeddings bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build or update the knowledge graph index",
		Long: `Build or update the knowledge graph and embedding indexes.
Fetches content from all configured sources and indexes it.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, ".dewey")
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			// Load sources config.
			sourcesPath := filepath.Join(deweyDir, "sources.yaml")
			configs, err := source.LoadSourcesConfig(sourcesPath)
			if err != nil {
				return fmt.Errorf("load sources config: %w", err)
			}

			// Open store.
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			// Auto-purge orphaned sources (FR-013, T017): compare configured
			// source IDs against source IDs in the store. Delete pages for
			// any source that no longer appears in sources.yaml.
			purgeOrphanedSources(s, configs)

			// Create embedder for embedding generation during indexing (R4).
			// Hard error: if Ollama is unavailable and --no-embeddings is not set,
			// indexing fails with an actionable error message.
			embedder, err := createIndexEmbedder(noEmbeddings)
			if err != nil {
				return err
			}

			// Build last-fetched times from store.
			lastFetchedTimes := make(map[string]time.Time)
			storedSources, _ := s.ListSources()
			for _, src := range storedSources {
				if src.LastFetchedAt > 0 {
					lastFetchedTimes[src.ID] = time.UnixMilli(src.LastFetchedAt)
				}
			}

			// Create source manager and fetch.
			cacheDir := filepath.Join(deweyDir, "cache")
			mgr := source.NewManager(configs, vp, cacheDir)
			result, allDocs := mgr.FetchAll(sourceName, force, lastFetchedTimes)

			totalIndexed, indexErr := indexDocuments(s, allDocs, configs, embedder)
			reportSourceErrors(s, result)

			if indexErr != nil {
				return fmt.Errorf("index failed: %w", indexErr)
			}

			logger.Info("index complete",
				"documents", totalIndexed,
				"errors", result.TotalErrs,
				"skipped", result.TotalSkip,
			)

			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Index only the specified source")
	cmd.Flags().BoolVar(&force, "force", false, "Re-fetch all sources, even if within their refresh interval")
	cmd.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip embedding generation (disables semantic search)")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// newReindexCmd creates the `dewey reindex` subcommand.
// Performs a clean re-index by removing the existing graph.db and all
// associated SQLite files (WAL, SHM) and lock files, then running a
// full index from scratch. This is the recommended way to rebuild the
// index after upgrading Dewey or when the index is corrupted.
func newReindexCmd() *cobra.Command {
	var noEmbeddings bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Delete and rebuild the index from scratch",
		Long: `Remove the existing graph.db and rebuild the index from scratch.

This is equivalent to manually deleting .dewey/graph.db and its WAL/SHM
files, then running 'dewey index --force'. Use this when:
  - Upgrading Dewey to a version with schema or indexing changes
  - The index is corrupted (UUID collisions, foreign key errors)
  - You want a guaranteed clean slate

The command removes: graph.db, graph.db-wal, graph.db-shm, .dewey.lock`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reindexStart := time.Now()
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, ".dewey")
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			logger.Info("reindex starting", "vault", vp, "deweyDir", deweyDir, "pid", os.Getpid())

			// Acquire the lock FIRST to prevent TOCTOU race conditions.
			// If another dewey process holds the lock, we fail immediately
			// instead of checking and then racing to remove files.
			lockPath := filepath.Join(deweyDir, ".dewey.lock")
			lockFile, lockErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
			if lockErr == nil {
				if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
					_ = lockFile.Close()
					return fmt.Errorf("database is locked by another dewey process — stop 'dewey serve' first")
				}
				// We hold the lock now — safe to remove files.
				// Release and close before removing the lock file itself.
				_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
				_ = lockFile.Close()
			}

			// Remove all database and lock files with per-file logging.
			filesToRemove := []string{
				filepath.Join(deweyDir, "graph.db"),
				filepath.Join(deweyDir, "graph.db-wal"),
				filepath.Join(deweyDir, "graph.db-shm"),
				lockPath,
			}

			for _, f := range filesToRemove {
				info, statErr := os.Stat(f)
				if os.IsNotExist(statErr) {
					logger.Debug("file not present, skipping", "file", filepath.Base(f))
					continue
				}
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				if err := os.Remove(f); err != nil {
					return fmt.Errorf("remove %s (size=%d): %w — stop 'dewey serve' first", filepath.Base(f), size, err)
				}
				logger.Info("removed", "file", filepath.Base(f), "size", size)
			}

			// Open a fresh store (creates new graph.db with schema).
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()
			logger.Info("store opened", "path", dbPath)

			// Load sources config.
			sourcesPath := filepath.Join(deweyDir, "sources.yaml")
			configs, err := source.LoadSourcesConfig(sourcesPath)
			if err != nil {
				return fmt.Errorf("load sources config: %w", err)
			}
			logger.Info("sources loaded", "count", len(configs))
			for _, cfg := range configs {
				logger.Debug("source config", "id", cfg.ID, "type", cfg.Type, "name", cfg.Name)
			}

			// Create embedder (hard error if unavailable, unless --no-embeddings).
			embedder, err := createIndexEmbedder(noEmbeddings)
			if err != nil {
				return err
			}

			// Fetch all sources (force mode — ignore refresh intervals).
			cacheDir := filepath.Join(deweyDir, "cache")
			mgr := source.NewManager(configs, vp, cacheDir)
			lastFetchedTimes := make(map[string]time.Time) // empty — force fetch all
			result, allDocs := mgr.FetchAll("", true, lastFetchedTimes)

			// Log what was fetched per source.
			for srcID, docs := range allDocs {
				logger.Info("source fetched for reindex", "source", srcID, "documents", len(docs))
			}

			totalIndexed, indexErr := indexDocuments(s, allDocs, configs, embedder)
			reportSourceErrors(s, result)

			if indexErr != nil {
				return fmt.Errorf("reindex failed: %w", indexErr)
			}

			// Verify final state.
			finalPages, _ := s.ListPages()
			elapsed := time.Since(reindexStart)
			logger.Info("reindex complete",
				"documents", totalIndexed,
				"pages_in_db", len(finalPages),
				"errors", result.TotalErrs,
				"elapsed", elapsed.Round(time.Millisecond),
			)

			return nil
		},
	}

	cmd.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip embedding generation (disables semantic search)")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// detectLockHolder checks if the .dewey.lock file is held by another process.
// Returns a description of the holder (e.g., "PID 12345") or empty string if unlocked.
func detectLockHolder(lockPath string) string {
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0o644)
	if err != nil {
		return "" // file doesn't exist or can't be opened — not locked
	}
	defer func() { _ = f.Close() }()

	// Try to acquire a non-blocking exclusive lock.
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock is held by another process. Try to find who.
		return fmt.Sprintf("lock file %s is held (try: lsof %s)", lockPath, lockPath)
	}
	// We got the lock — release it, no one is holding it.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return ""
}

// createIndexEmbedder creates an OllamaEmbedder for use during indexing,
// using the same environment variables as `dewey serve` (per research R4).
// When noEmbeddings is true, returns nil (no embedder). When false, checks
// availability and returns a hard error if the model is unavailable.
func createIndexEmbedder(noEmbeddings bool) (embed.Embedder, error) {
	if noEmbeddings {
		logger.Info("embeddings disabled via --no-embeddings")
		return nil, nil
	}

	embedModel := os.Getenv("DEWEY_EMBEDDING_MODEL")
	embedEndpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT")
	if embedModel == "" {
		embedModel = "granite-embedding:30m"
	}
	if embedEndpoint == "" {
		embedEndpoint = "http://localhost:11434"
	}

	embedder := embed.NewOllamaEmbedder(embedEndpoint, embedModel)
	if !embedder.Available() {
		return nil, fmt.Errorf("embedding model %q not available at %s\n\nTo fix:\n  ollama pull %s\n\nTo skip embeddings:\n  dewey index --no-embeddings",
			embedModel, embedEndpoint, embedModel)
	}
	logger.Info("embedding model available for indexing", "model", embedModel)
	return embedder, nil
}

// purgeOrphanedSources compares configured source IDs against source IDs
// stored in the database. Any source in the store that is not in the config
// has its pages deleted (FR-013 auto-purge).
func purgeOrphanedSources(s *store.Store, configs []source.SourceConfig) {
	configIDs := make(map[string]bool, len(configs))
	for _, cfg := range configs {
		configIDs[cfg.ID] = true
	}

	storedSources, err := s.ListSources()
	if err != nil {
		logger.Warn("failed to list stored sources for purge check", "err", err)
		return
	}

	for _, src := range storedSources {
		logger.Debug("checking source for orphan purge", "source", src.ID, "inConfig", configIDs[src.ID])
		if !configIDs[src.ID] {
			deleted, err := s.DeletePagesBySource(src.ID)
			if err != nil {
				logger.Warn("failed to purge orphaned source pages",
					"source", src.ID, "err", err)
				continue
			}
			if deleted > 0 {
				logger.Info("purged orphaned source",
					"source", src.ID, "pages_deleted", deleted)
			}
		}
	}
}

// indexDocuments upserts fetched documents into the persistent store with full
// content persistence: blocks, links, and embeddings are parsed and stored
// alongside page metadata (US2). Returns the total number of documents
// successfully indexed.
func indexDocuments(s *store.Store, allDocs map[string][]source.Document, configs []source.SourceConfig, embedder embed.Embedder) (int, error) {
	totalIndexed := 0
	for sourceID, docs := range allDocs {
		start := time.Now()
		var blockCount, linkCount, embedCount int

		logger.Info("indexing source", "source", sourceID, "documents", len(docs))

		for _, doc := range docs {
			// Namespace external page names: sourceID/docID (per research R6, T016).
			pageName := strings.ToLower(sourceID + "/" + doc.ID)
			logger.Debug("indexing document", "source", sourceID, "docID", doc.ID, "pageName", pageName, "contentHash", doc.ContentHash)

			// Parse document content into frontmatter and blocks (T011).
			// Use pageName (source-namespaced) as the docID seed so that identical
			// files across different sources produce unique block UUIDs (fixes #17).
			props, blocks := vault.ParseDocument(pageName, doc.Content)
			logger.Debug("parsed document", "page", pageName, "blocks", len(blocks), "uuidSeed", pageName)
			if len(blocks) > 0 {
				logger.Debug("first block UUID", "page", pageName, "uuid", blocks[0].UUID)
			}

			// Build properties JSON.
			propsJSON := ""
			if props != nil {
				data, _ := json.Marshal(props)
				propsJSON = string(data)
			} else if doc.Properties != nil {
				data, _ := json.Marshal(doc.Properties)
				propsJSON = string(data)
			}

			// Upsert page record.
			existing, _ := s.GetPage(pageName)
			logger.Debug("page upsert", "page", pageName, "isUpdate", existing != nil)
			if existing != nil {
				// Re-index: delete existing blocks and links first (FR-004 replace strategy, T012/T013).
				if err := s.DeleteBlocksByPage(pageName); err != nil {
					logger.Warn("failed to delete existing blocks for re-index", "page", pageName, "err", err)
				}
				if err := s.DeleteLinksByPage(pageName); err != nil {
					logger.Warn("failed to delete existing links for re-index", "page", pageName, "err", err)
				}

				existing.ContentHash = doc.ContentHash
				existing.SourceID = sourceID
				existing.SourceDocID = doc.ID
				existing.OriginalName = doc.Title
				existing.Properties = propsJSON
				if err := s.UpdatePage(existing); err != nil {
					logger.Warn("failed to update page", "page", pageName, "err", err)
					continue
				}
			} else {
				page := &store.Page{
					Name:         pageName,
					OriginalName: doc.Title,
					SourceID:     sourceID,
					SourceDocID:  doc.ID,
					Properties:   propsJSON,
					ContentHash:  doc.ContentHash,
					CreatedAt:    doc.FetchedAt.UnixMilli(),
					UpdatedAt:    doc.FetchedAt.UnixMilli(),
				}
				if err := s.InsertPage(page); err != nil {
					logger.Warn("failed to insert page", "page", pageName, "err", err)
					continue
				}
			}

			// Persist blocks (T012) — uses shared vault.PersistBlocks to avoid duplication.
			// Block persistence failures (e.g., UUID collisions) are hard errors —
			// silently dropping blocks corrupts the index.
			if err := vault.PersistBlocks(s, pageName, blocks, sql.NullString{}, 0); err != nil {
				return 0, fmt.Errorf("persist blocks for %s: %w", pageName, err)
			}
			blockCount += countBlocksRecursive(blocks)

			// Extract and persist links from blocks (T013) — uses shared vault.PersistLinks.
			if err := vault.PersistLinks(s, pageName, blocks); err != nil {
				return 0, fmt.Errorf("persist links for %s: %w", pageName, err)
			}
			linkCount += countLinksRecursive(blocks)

			// Generate and persist embeddings if embedder is available (T015).
			if embedder != nil && embedder.Available() {
				ec := generateIndexEmbeddings(s, embedder, pageName, blocks, nil)
				embedCount += ec
			}

			totalIndexed++
		}

		elapsed := time.Since(start)
		logger.Info("source indexing complete",
			"source", sourceID,
			"documents", len(docs),
			"blocks", blockCount,
			"links", linkCount,
			"embeddings", embedCount,
			"elapsed", elapsed.Round(time.Millisecond),
		)

		// Update source record in store.
		existingSrc, _ := s.GetSource(sourceID)
		if existingSrc == nil {
			var srcType, srcName string
			for _, cfg := range configs {
				if cfg.ID == sourceID {
					srcType = cfg.Type
					srcName = cfg.Name
					break
				}
			}
			if err := s.InsertSource(&store.SourceRecord{
				ID:            sourceID,
				Type:          srcType,
				Name:          srcName,
				Status:        "active",
				LastFetchedAt: time.Now().UnixMilli(),
			}); err != nil {
				logger.Warn("failed to insert source record", "source", sourceID, "err", err)
			}
		} else {
			if err := s.UpdateLastFetched(sourceID, time.Now().UnixMilli()); err != nil {
				logger.Warn("failed to update source last fetched", "source", sourceID, "err", err)
			}
			if err := s.UpdateSourceStatus(sourceID, "active", ""); err != nil {
				logger.Warn("failed to update source status", "source", sourceID, "err", err)
			}
		}
	}
	return totalIndexed, nil
}

// generateIndexEmbeddings creates vector embeddings for blocks during indexing.
// Delegates to the shared vault.GenerateEmbeddings function.
func generateIndexEmbeddings(s *store.Store, embedder embed.Embedder, pageName string, blocks []types.BlockEntity, headingPath []string) int {
	return vault.GenerateEmbeddings(s, embedder, pageName, blocks, headingPath)
}

// countBlocksRecursive returns the total number of blocks in a tree.
func countBlocksRecursive(blocks []types.BlockEntity) int {
	count := len(blocks)
	for _, b := range blocks {
		count += countBlocksRecursive(b.Children)
	}
	return count
}

// countLinksRecursive returns the total number of wikilinks in a block tree.
func countLinksRecursive(blocks []types.BlockEntity) int {
	count := 0
	for _, b := range blocks {
		parsed := parser.Parse(b.Content)
		count += len(parsed.Links)
		count += countLinksRecursive(b.Children)
	}
	return count
}

// reportSourceErrors updates source status for any sources that failed
// during the fetch phase.
func reportSourceErrors(s *store.Store, result *source.FetchResult) {
	for _, summary := range result.Summaries {
		if summary.Error != "" {
			existingSrc, _ := s.GetSource(summary.SourceID)
			if existingSrc != nil {
				if err := s.UpdateSourceStatus(summary.SourceID, "error", summary.Error); err != nil {
					logger.Warn("failed to update source error status", "source", summary.SourceID, "err", err)
				}
			}
		}
	}
}

// --- Doctor command ---

// newDoctorCmd creates the `dewey doctor` subcommand.
// Checks all Dewey prerequisites and reports pass/fail for each
// with actionable fix instructions.
func newDoctorCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Dewey prerequisites",
		Long:  "Run diagnostic checks for Dewey dependencies and report pass/fail with fix instructions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve vault path — defaults to CWD if neither
			// --vault flag nor OBSIDIAN_VAULT_PATH is set.
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			runDoctorChecks(w, vp)
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// doctorCounter tracks pass/warn/fail counts for the summary box.
type doctorCounter struct {
	pass int
	warn int
	fail int
}

// printCheck writes a formatted check line in the `uf doctor` style:
//
//	[PASS] name                description
//
// The name field is left-aligned and padded to 20 characters. The marker
// is one of PASS, WARN, or FAIL — the counter is incremented accordingly.
func (c *doctorCounter) printCheck(w io.Writer, marker, name, description string) {
	switch marker {
	case "PASS":
		c.pass++
	case "WARN":
		c.warn++
	case "FAIL":
		c.fail++
	}
	_, _ = fmt.Fprintf(w, "  [%s] %-20s%s\n", marker, name, description)
}

// humanSize converts a byte count to a human-readable string (B, KB, MB, GB).
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// printSummaryBox writes the `uf doctor` style summary box with emoji counters.
func printSummaryBox(w io.Writer, c *doctorCounter) {
	warnWord := "warnings"
	if c.warn == 1 {
		warnWord = "warning"
	}

	inner := fmt.Sprintf("   ✅ %d passed  ⚠️  %d %s  ❌ %d failed   ",
		c.pass, c.warn, warnWord, c.fail)

	// Use display width (not rune count) so emoji render correctly in terminals.
	boxWidth := runewidth.StringWidth(inner)

	topBorder := "╭" + strings.Repeat("─", boxWidth) + "╮"
	bottomBorder := "╰" + strings.Repeat("─", boxWidth) + "╯"

	_, _ = fmt.Fprintf(w, "%s\n│%s│\n%s\n", topBorder, inner, bottomBorder)
}

// runDoctorChecks executes comprehensive diagnostic checks in the style of
// `uf doctor` — grouped sections, [PASS]/[WARN]/[FAIL] markers, descriptions
// with paths, and Fix: hints for actionable remediation.
func runDoctorChecks(w io.Writer, vaultPath string) {
	dp := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	c := &doctorCounter{}

	embeddingCount := -1 // -1 = not queried; set by store section if available.
	dp("🩺 Dewey Doctor\n\n")

	// --- Environment ---
	dp("Environment\n")
	c.printCheck(w, "PASS", "vault", vaultPath)

	deweyBin, err := os.Executable()
	if err == nil {
		c.printCheck(w, "PASS", "dewey", fmt.Sprintf("v%s (%s)", version, deweyBin))
	} else {
		c.printCheck(w, "WARN", "dewey", "could not determine path")
	}
	dp("\n")

	// --- Workspace ---
	dp("Workspace\n")
	deweyDir := filepath.Join(vaultPath, ".dewey")
	if _, err := os.Stat(deweyDir); err == nil {
		c.printCheck(w, "PASS", ".dewey/", fmt.Sprintf("initialized (%s)", deweyDir))
	} else {
		c.printCheck(w, "FAIL", ".dewey/", "not found")
		dp("     Fix: dewey init --vault %s\n", vaultPath)
		dp("\n")
		printSummaryBox(w, c)
		return // No point continuing if not initialized.
	}

	// Config files.
	configPath := filepath.Join(deweyDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		c.printCheck(w, "PASS", "config.yaml", fmt.Sprintf("present (%s)", configPath))
	} else {
		c.printCheck(w, "WARN", "config.yaml", "not found (using defaults)")
	}

	sourcesPath := filepath.Join(deweyDir, "sources.yaml")
	if _, err := os.Stat(sourcesPath); err == nil {
		configs, loadErr := source.LoadSourcesConfig(sourcesPath)
		if loadErr != nil {
			c.printCheck(w, "FAIL", "sources.yaml", fmt.Sprintf("parse error: %v", loadErr))
		} else {
			c.printCheck(w, "PASS", "sources.yaml", fmt.Sprintf("%d sources (%s)", len(configs), sourcesPath))
		}
	} else {
		c.printCheck(w, "WARN", "sources.yaml", "not found (no external sources)")
	}

	// Log file.
	logPath := filepath.Join(deweyDir, "dewey.log")
	if info, err := os.Stat(logPath); err == nil {
		c.printCheck(w, "PASS", "dewey.log", fmt.Sprintf("%s (%s)", humanSize(info.Size()), logPath))
	} else {
		c.printCheck(w, "PASS", "dewey.log", "not present (created on dewey serve)")
	}
	dp("\n")

	// --- Database ---
	dp("Database\n")
	dbPath := filepath.Join(deweyDir, "graph.db")
	dbInfo, dbStatErr := os.Stat(dbPath)
	if dbStatErr != nil {
		c.printCheck(w, "FAIL", "graph.db", "not found")
		dp("     Fix: dewey index\n")
		dp("\n")
	} else {
		// Try to open the store and report contents.
		s, storeErr := store.New(dbPath)
		if storeErr != nil {
			c.printCheck(w, "FAIL", "graph.db", fmt.Sprintf("cannot open: %v", storeErr))
			if strings.Contains(storeErr.Error(), "another Dewey process") {
				dp("     Fix: Stop 'dewey serve' and retry, or run: dewey reindex\n")
			}
		} else {
			defer func() { _ = s.Close() }()

			allPages, _ := s.ListPages()
			c.printCheck(w, "PASS", "graph.db", fmt.Sprintf("%s, %d pages (%s)", humanSize(dbInfo.Size()), len(allPages), dbPath))

			// Sources in database.
			sources, _ := s.ListSources()
			if len(sources) > 0 {
				dp("\n")
				dp("Sources in Database\n")
				for _, src := range sources {
					pages, _ := s.ListPagesBySource(src.ID)
					lastFetched := "never"
					if src.LastFetchedAt > 0 {
						t := time.UnixMilli(src.LastFetchedAt)
						lastFetched = t.Format("2006-01-02 15:04")
					}
					if len(pages) > 0 {
						c.printCheck(w, "PASS", src.ID, fmt.Sprintf("%d pages (fetched: %s)", len(pages), lastFetched))
					} else {
						c.printCheck(w, "WARN", src.ID, fmt.Sprintf("0 pages (fetched: %s)", lastFetched))
						dp("     Fix: dewey reindex --no-embeddings\n")
					}
				}
			}

			// Query embedding count while store is still open.
			if ec, ecErr := s.CountEmbeddings(); ecErr == nil {
				embeddingCount = ec
			}
		}
	}
	dp("\n")

	// --- Embedding Layer ---
	embedEndpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT")
	embedModel := os.Getenv("DEWEY_EMBEDDING_MODEL")
	if embedEndpoint == "" {
		embedEndpoint = "http://localhost:11434"
	}
	if embedModel == "" {
		embedModel = "granite-embedding:30m"
	}

	dp("Embedding Layer (%s via %s)\n", embedModel, embedEndpoint)

	// Ollama reachability.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ollamaReachable := false
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, embedEndpoint+"/api/tags", nil)
	if reqErr == nil {
		resp, doErr := http.DefaultClient.Do(req)
		if doErr == nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == http.StatusOK {
				ollamaReachable = true
				c.printCheck(w, "PASS", "ollama", fmt.Sprintf("running (%s)", embedEndpoint))
			} else {
				c.printCheck(w, "FAIL", "ollama", fmt.Sprintf("status %d (%s)", resp.StatusCode, embedEndpoint))
				dp("     Fix: ollama serve\n")
			}
		} else {
			c.printCheck(w, "FAIL", "ollama", fmt.Sprintf("not reachable (%s)", embedEndpoint))
			dp("     Fix: brew install --cask ollama-app && open -a Ollama\n")
		}
	}

	// Model availability.
	if ollamaReachable {
		embedder := embed.NewOllamaEmbedder(embedEndpoint, embedModel)
		if embedder.Available() {
			c.printCheck(w, "PASS", "model", fmt.Sprintf("%s ready", embedModel))
		} else {
			c.printCheck(w, "FAIL", "model", fmt.Sprintf("%s not available", embedModel))
			dp("     Fix: ollama pull %s\n", embedModel)
		}
	} else {
		c.printCheck(w, "WARN", "model", fmt.Sprintf("%s skipped (ollama not reachable)", embedModel))
	}

	// Embedding count — uses value queried from the already-open store above.
	if embeddingCount > 0 {
		c.printCheck(w, "PASS", "embeddings", fmt.Sprintf("%d in database", embeddingCount))
	} else if embeddingCount == 0 {
		c.printCheck(w, "WARN", "embeddings", "0 in database")
		dp("     Fix: dewey reindex (with Ollama running)\n")
	}
	dp("\n")

	// --- MCP Server ---
	dp("MCP Server\n")

	// Check if a dewey serve process is running (consolidated lock check per D5).
	lockPath := filepath.Join(deweyDir, ".dewey.lock")
	if _, err := os.Stat(lockPath); err == nil {
		holder := detectLockHolder(lockPath)
		if holder != "" {
			c.printCheck(w, "PASS", "serve", fmt.Sprintf("running (%s)", holder))
		} else {
			c.printCheck(w, "PASS", "serve", "not running (stale lock file)")
		}
	} else {
		c.printCheck(w, "PASS", "serve", "not running (no lock)")
	}

	// Check opencode.json for MCP config.
	ocPath := filepath.Join(vaultPath, "opencode.json")
	if data, err := os.ReadFile(ocPath); err == nil {
		if strings.Contains(string(data), "dewey") {
			c.printCheck(w, "PASS", "opencode.json", fmt.Sprintf("dewey MCP configured (%s)", ocPath))
		} else {
			c.printCheck(w, "WARN", "opencode.json", "exists but no dewey MCP config")
			dp("     Fix: Add dewey MCP server to opencode.json\n")
		}
	} else {
		c.printCheck(w, "PASS", "opencode.json", "not found (optional)")
	}
	dp("\n")

	// --- Summary ---
	printSummaryBox(w, c)
}

// --- Source command (T051) ---

// newSourceCmd creates the `dewey source` subcommand group.
func newSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage content sources",
		Long:  "Add, list, and manage content sources for the knowledge graph.",
	}

	cmd.AddCommand(newSourceAddCmd())
	return cmd
}

// newSourceAddCmd creates the `dewey source add` subcommand.
// Per contracts/cli-commands.md.
func newSourceAddCmd() *cobra.Command {
	// GitHub flags.
	var org string
	var repos string
	var content string
	var refresh string

	// Web flags.
	var webURL string
	var webName string
	var depth int

	cmd := &cobra.Command{
		Use:   "add [github|web]",
		Short: "Add a content source",
		Long: `Add a content source to the configuration.

Examples:
  dewey source add github --org unbound-force --repos gaze,website
  dewey source add web --url https://pkg.go.dev/std --name go-stdlib --depth 2`,
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceType := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			deweyDir := filepath.Join(cwd, ".dewey")
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			sourcesPath := filepath.Join(deweyDir, "sources.yaml")

			// Load existing sources.
			existing, err := source.LoadSourcesConfig(sourcesPath)
			if err != nil {
				return fmt.Errorf("load sources config: %w", err)
			}

			var newSource source.SourceConfig
			var buildErr error

			switch sourceType {
			case "github":
				newSource, buildErr = buildGitHubSource(org, repos, content, refresh)
			case "web":
				newSource, buildErr = buildWebSource(webURL, webName, refresh, depth)
			default:
				return fmt.Errorf("unknown source type %q (use github or web)", sourceType)
			}
			if buildErr != nil {
				return buildErr
			}

			if err := saveSourceConfig(sourcesPath, existing, newSource); err != nil {
				return err
			}

			logger.Info("added source",
				"id", newSource.ID,
				"type", newSource.Type,
				"refresh", newSource.RefreshInterval,
			)
			logger.Info(fmt.Sprintf("run 'dewey index --source %s' to fetch content", newSource.ID))

			return nil
		},
	}

	// GitHub flags.
	cmd.Flags().StringVar(&org, "org", "", "GitHub organization name")
	cmd.Flags().StringVar(&repos, "repos", "", "Comma-separated list of repository names")
	cmd.Flags().StringVar(&content, "content", "", "Content types to fetch (default: issues,pulls,readme)")
	cmd.Flags().StringVar(&refresh, "refresh", "", "Refresh interval (default: daily for github, weekly for web)")

	// Web flags.
	cmd.Flags().StringVar(&webURL, "url", "", "Documentation URL to crawl")
	cmd.Flags().StringVar(&webName, "name", "", "Human-readable source name")
	cmd.Flags().IntVar(&depth, "depth", 1, "Crawl depth")

	return cmd
}

// buildGitHubSource validates inputs and creates a SourceConfig for a GitHub source.
func buildGitHubSource(org, repos, content, refresh string) (source.SourceConfig, error) {
	if org == "" {
		return source.SourceConfig{}, fmt.Errorf("--org is required for github source")
	}
	if repos == "" {
		return source.SourceConfig{}, fmt.Errorf("--repos is required for github source")
	}

	repoList := strings.Split(repos, ",")
	for i := range repoList {
		repoList[i] = strings.TrimSpace(repoList[i])
	}

	contentTypes := []string{"issues", "pulls", "readme"}
	if content != "" {
		contentTypes = strings.Split(content, ",")
		for i := range contentTypes {
			contentTypes[i] = strings.TrimSpace(contentTypes[i])
		}
	}

	if refresh == "" {
		refresh = "daily"
	}

	return source.SourceConfig{
		ID:              fmt.Sprintf("github-%s", org),
		Type:            "github",
		Name:            org,
		RefreshInterval: refresh,
		Config: map[string]any{
			"org":     org,
			"repos":   repoList,
			"content": contentTypes,
		},
	}, nil
}

// buildWebSource validates inputs and creates a SourceConfig for a web crawl source.
func buildWebSource(webURL, webName, refresh string, depth int) (source.SourceConfig, error) {
	if webURL == "" {
		return source.SourceConfig{}, fmt.Errorf("--url is required for web source")
	}

	name := webName
	if name == "" {
		name = strings.TrimPrefix(webURL, "https://")
		name = strings.TrimPrefix(name, "http://")
		if idx := strings.Index(name, "/"); idx > 0 {
			name = name[:idx]
		}
	}

	if refresh == "" {
		refresh = "weekly"
	}

	return source.SourceConfig{
		ID:              fmt.Sprintf("web-%s", name),
		Type:            "web",
		Name:            name,
		RefreshInterval: refresh,
		Config: map[string]any{
			"urls":  []string{webURL},
			"depth": depth,
		},
	}, nil
}

// saveSourceConfig checks for duplicate source IDs, appends the new source,
// and saves the updated config to the YAML file.
func saveSourceConfig(sourcesPath string, existing []source.SourceConfig, newSource source.SourceConfig) error {
	for _, src := range existing {
		if src.ID == newSource.ID {
			return fmt.Errorf("source %s already exists", newSource.ID)
		}
	}

	existing = append(existing, newSource)
	if err := source.SaveSourcesConfig(sourcesPath, existing); err != nil {
		return fmt.Errorf("save sources config: %w", err)
	}
	return nil
}
