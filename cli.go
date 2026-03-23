package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/client"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
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
// Performs full-text search and prints results to stdout.
func newSearchCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search [flags] QUERY",
		Short: "Full-text search across the graph",
		Long:  "Full-text search across all blocks in the knowledge graph.",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			if query == "" {
				return fmt.Errorf("query is required")
			}

			c := client.New("", "")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			queryLower := strings.ToLower(query)
			pages, err := c.GetAllPages(ctx)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			found := 0
			for _, pg := range pages {
				if found >= limit {
					break
				}
				if pg.Name == "" {
					continue
				}

				blocks, err := c.GetPageBlocksTree(ctx, pg.Name)
				if err != nil {
					continue
				}

				printSearchResults(blocks, queryLower, pg.OriginalName, limit, &found)
			}

			if found == 0 {
				return fmt.Errorf("no results for %q", query)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results")

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

// newStatusCmd creates the `dewey status` subcommand.
// Reports index health: page count, block count, source info.
// Supports --json flag for structured output.
func newStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Report index status",
		Long:         "Show Dewey index health: page count, block count, source info, and index path.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Find .dewey/ directory.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			deweyDir := filepath.Join(cwd, ".dewey")
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			dbPath := filepath.Join(deweyDir, "graph.db")
			var pageCount, blockCount, embeddingCount int
			var embeddingModel string
			embeddingAvailable := false

			// Read embedding model from config.yaml if it exists.
			configPath := filepath.Join(deweyDir, "config.yaml")
			if configData, err := os.ReadFile(configPath); err == nil {
				// Simple YAML parsing for model name — avoid adding a YAML dependency
				// just for status display. The config is already parsed elsewhere.
				for _, line := range strings.Split(string(configData), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "model:") {
						embeddingModel = strings.TrimSpace(strings.TrimPrefix(line, "model:"))
					}
				}
			}

			// Load per-source status from store.
			type sourceStatus struct {
				ID          string `json:"id"`
				Type        string `json:"type"`
				Status      string `json:"status"`
				PageCount   int    `json:"pageCount"`
				LastFetched string `json:"lastFetched,omitempty"`
				Error       string `json:"error,omitempty"`
			}
			var sources []sourceStatus

			// Open store if database exists.
			if _, err := os.Stat(dbPath); err == nil {
				s, err := store.New(dbPath)
				if err != nil {
					return fmt.Errorf("open store: %w", err)
				}
				defer func() { _ = s.Close() }()

				pages, err := s.ListPages()
				if err != nil {
					return fmt.Errorf("list pages: %w", err)
				}
				pageCount = len(pages)

				bc, err := s.CountBlocks()
				if err == nil {
					blockCount = bc
				}

				ec, err := s.CountEmbeddings()
				if err == nil {
					embeddingCount = ec
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
					sources = append(sources, ss)
				}
			}

			// Compute embedding coverage.
			var coverage float64
			if blockCount > 0 {
				coverage = float64(embeddingCount) / float64(blockCount) * 100
			}

			if jsonOutput {
				status := map[string]any{
					"path":               deweyDir,
					"pages":              pageCount,
					"blocks":             blockCount,
					"embeddings":         embeddingCount,
					"embeddingModel":     embeddingModel,
					"embeddingAvailable": embeddingAvailable,
					"embeddingCoverage":  coverage,
					"sources":            sources,
				}
				data, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal JSON: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dewey Index Status")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Path:       %s\n", deweyDir)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Pages:      %d\n", pageCount)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Blocks:     %d\n", blockCount)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Embeddings: %d\n", embeddingCount)
				if embeddingModel != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Model:      %s\n", embeddingModel)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Coverage:   %.1f%%\n", coverage)

				if len(sources) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nSources")
					for _, src := range sources {
						lastFetched := "never"
						if src.LastFetched != "" {
							lastFetched = src.LastFetched + " ago"
						}
						if src.Error != "" {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-15s %-8s %3d pages  %s  error: %s\n",
								src.ID, src.Status, src.PageCount, lastFetched, src.Error)
						} else {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-15s %-8s %3d pages  %s\n",
								src.ID, src.Status, src.PageCount, lastFetched)
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
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

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build or update the knowledge graph index",
		Long: `Build or update the knowledge graph and embedding indexes.
Fetches content from all configured sources and indexes it.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			deweyDir := filepath.Join(cwd, ".dewey")
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
			mgr := source.NewManager(configs, cwd, cacheDir)
			result, allDocs := mgr.FetchAll(sourceName, force, lastFetchedTimes)

			// Index fetched documents into store.
			totalIndexed := 0
			for sourceID, docs := range allDocs {
				for _, doc := range docs {
					// Upsert page from document.
					existing, _ := s.GetPage(doc.Title)
					if existing != nil {
						existing.ContentHash = doc.ContentHash
						existing.SourceID = sourceID
						existing.SourceDocID = doc.ID
						if err := s.UpdatePage(existing); err != nil {
							logger.Warn("failed to update page", "page", doc.Title, "err", err)
							continue
						}
					} else {
						page := &store.Page{
							Name:         doc.Title,
							OriginalName: doc.Title,
							SourceID:     sourceID,
							SourceDocID:  doc.ID,
							ContentHash:  doc.ContentHash,
							CreatedAt:    doc.FetchedAt.UnixMilli(),
							UpdatedAt:    doc.FetchedAt.UnixMilli(),
						}
						if doc.Properties != nil {
							propsJSON, _ := json.Marshal(doc.Properties)
							page.Properties = string(propsJSON)
						}
						if err := s.InsertPage(page); err != nil {
							logger.Warn("failed to insert page", "page", doc.Title, "err", err)
							continue
						}
					}
					totalIndexed++
				}

				// Update source record in store.
				existingSrc, _ := s.GetSource(sourceID)
				if existingSrc == nil {
					// Find config for this source.
					var srcType, srcName string
					for _, cfg := range configs {
						if cfg.ID == sourceID {
							srcType = cfg.Type
							srcName = cfg.Name
							break
						}
					}
					_ = s.InsertSource(&store.SourceRecord{
						ID:            sourceID,
						Type:          srcType,
						Name:          srcName,
						Status:        "active",
						LastFetchedAt: time.Now().UnixMilli(),
					})
				} else {
					_ = s.UpdateLastFetched(sourceID, time.Now().UnixMilli())
					_ = s.UpdateSourceStatus(sourceID, "active", "")
				}
			}

			// Report errors for failed sources.
			for _, summary := range result.Summaries {
				if summary.Error != "" {
					existingSrc, _ := s.GetSource(summary.SourceID)
					if existingSrc != nil {
						_ = s.UpdateSourceStatus(summary.SourceID, "error", summary.Error)
					}
				}
			}

			// Print summary.
			logger.Info("index complete",
				"documents", totalIndexed,
				"errors", result.TotalErrs,
				"skipped", result.TotalSkip,
			)

			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Index only the specified source")
	cmd.Flags().BoolVar(&force, "force", false, "Force full re-index, ignoring refresh intervals")

	return cmd
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

			switch sourceType {
			case "github":
				if org == "" {
					return fmt.Errorf("--org is required for github source")
				}
				if repos == "" {
					return fmt.Errorf("--repos is required for github source")
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

				sourceID := fmt.Sprintf("github-%s", org)
				newSource = source.SourceConfig{
					ID:              sourceID,
					Type:            "github",
					Name:            org,
					RefreshInterval: refresh,
					Config: map[string]any{
						"org":     org,
						"repos":   repoList,
						"content": contentTypes,
					},
				}

			case "web":
				if webURL == "" {
					return fmt.Errorf("--url is required for web source")
				}

				name := webName
				if name == "" {
					// Derive name from URL hostname.
					name = strings.TrimPrefix(webURL, "https://")
					name = strings.TrimPrefix(name, "http://")
					if idx := strings.Index(name, "/"); idx > 0 {
						name = name[:idx]
					}
				}

				if refresh == "" {
					refresh = "weekly"
				}

				sourceID := fmt.Sprintf("web-%s", name)
				newSource = source.SourceConfig{
					ID:              sourceID,
					Type:            "web",
					Name:            name,
					RefreshInterval: refresh,
					Config: map[string]any{
						"urls":  []string{webURL},
						"depth": depth,
					},
				}

			default:
				return fmt.Errorf("unknown source type %q (use github or web)", sourceType)
			}

			// Check for duplicate source.
			for _, src := range existing {
				if src.ID == newSource.ID {
					return fmt.Errorf("source %s already exists", newSource.ID)
				}
			}

			// Append and save.
			existing = append(existing, newSource)
			if err := source.SaveSourcesConfig(sourcesPath, existing); err != nil {
				return fmt.Errorf("save sources config: %w", err)
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
