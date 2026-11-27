package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/mattn/go-runewidth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/v3/artifacts"
	"github.com/unbound-force/dewey/v3/chunker"
	"github.com/unbound-force/dewey/v3/client"
	"github.com/unbound-force/dewey/v3/curate"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/ignore"
	"github.com/unbound-force/dewey/v3/llm"
	"github.com/unbound-force/dewey/v3/openaicompat"
	"github.com/unbound-force/dewey/v3/sanitize"
	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/tools"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
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
			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
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

// dcpJSONCContent is the template for the DCP configuration file.
// Enables protectTags so DCP honors <protect> blocks in slash commands.
const dcpJSONCContent = `{
  // DCP (Dynamic Context Pruning) configuration for OpenCode.
  // See: https://github.com/Opencode-DCP/opencode-dynamic-context-pruning
  "$schema": "https://raw.githubusercontent.com/Opencode-DCP/opencode-dynamic-context-pruning/master/dcp.schema.json",
  // Slash command files in .opencode/commands/ use <protect> tags
  // to mark execution-critical sections that must survive compression.
  "compress": {
    "protectTags": true
  }
}
`

// fileExists returns true if the given path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileContains returns true if the file at path exists and
// its content contains the given substring.
func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

// newInitCmd creates the `dewey init` subcommand.
// Initializes a .uf/dewey/ directory with default configuration.
// Idempotent — running twice does not error (per CLI contract).
func newInitCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Dewey configuration",
		Long:  "Create .uf/dewey/ directory with default config.yaml and sources.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultPath == "" {
				var err error
				vaultPath, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
			}

			deweyDir := filepath.Join(vaultPath, deweyWorkspaceDir)

			// Check if already initialized. If so, skip config/sources
			// creation but still run slash command scaffolding below.
			alreadyInitialized := false
			if _, err := os.Stat(deweyDir); err == nil {
				alreadyInitialized = true
				logger.Info("already initialized", "path", deweyDir)
			}

			if !alreadyInitialized {
				// Create .uf/dewey/ directory (MkdirAll creates .uf/ parent too — D3).
				if err := os.MkdirAll(deweyDir, 0o755); err != nil {
					return fmt.Errorf("create .uf/dewey/ directory: %w", err)
				}

				// Write default config.yaml.
				configPath := filepath.Join(deweyDir, "config.yaml")
				configContent := `# Dewey configuration
# See: https://github.com/unbound-force/dewey

embedding:
  model: granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env var
  # endpoint: http://localhost:11434  # Uncomment to override OLLAMA_HOST
  # max_chunk_chars: 12288  # Override with DEWEY_CHUNK_MAX_CHARS env var
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
      # ignore: [pattern1, pattern2]  # additional patterns beyond .gitignore
      # recursive: true               # set false to index only top-level files
`
				if err := os.WriteFile(sourcesPath, []byte(sourcesContent), 0o644); err != nil {
					return fmt.Errorf("write sources.yaml: %w", err)
				}

				// Write default knowledge-stores.yaml (T008, 015-curated-knowledge-stores).
				// Scaffolds a commented-out example store. Follows the same idempotency
				// pattern as sources.yaml — don't overwrite if file exists.
				ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
				ksContent := `# Knowledge store configuration
# Each store curates knowledge from indexed sources.
# Uncomment and customize the example below.

# stores:
#   - name: team-decisions
#     sources: [disk-local]
#     # path: .uf/dewey/knowledge/team-decisions  # default
#     # curate_on_index: false                     # default
#     # curation_interval: 10m                     # default
`
				if err := os.WriteFile(ksPath, []byte(ksContent), 0o644); err != nil {
					return fmt.Errorf("write knowledge-stores.yaml: %w", err)
				}

				// Append granular .uf/dewey/ runtime artifact patterns to .gitignore.
				// Only runtime artifacts (db, log, lock) are ignored — sources.yaml
				// and config.yaml remain trackable for team sharing.
				gitignorePath := filepath.Join(vaultPath, ".gitignore")
				if _, err := os.Stat(gitignorePath); err == nil {
					content, err := os.ReadFile(gitignorePath)
					if err == nil {
						text := string(content)
						switch {
						case strings.Contains(text, ".uf/dewey/graph.db"):
							// Current granular patterns already present — skip.
						case strings.Contains(text, ".dewey/graph.db"):
							// Old granular patterns — inform user to update.
							logger.Info("old .dewey/ gitignore patterns found — update to .uf/dewey/ patterns")
						case strings.Contains(text, ".dewey/"):
							// Legacy blanket pattern — don't modify, inform user.
							logger.Info("existing .dewey/ gitignore pattern found — update to .uf/dewey/ patterns")
						default:
							// No dewey patterns — append granular patterns.
							f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
							if err == nil {
								defer func() { _ = f.Close() }()
								if len(content) > 0 && content[len(content)-1] != '\n' {
									_, _ = f.WriteString("\n")
								}
								_, _ = f.WriteString(".uf/dewey/graph.db\n")
								_, _ = f.WriteString(".uf/dewey/graph.db-shm\n")
								_, _ = f.WriteString(".uf/dewey/graph.db-wal\n")
								_, _ = f.WriteString(".uf/dewey/dewey.log\n")
								_, _ = f.WriteString(".uf/dewey/dewey.lock\n")
							}
						}
					}
				}
			} // end if !alreadyInitialized

			// Scaffold Dewey-specific slash commands into .opencode/commands/
			// if the .opencode/ directory exists (composability — only scaffold
			// when OpenCode is present). Tool-owned refresh (DI-FR-002):
			// untouched scaffolds are refreshed to the embedded content;
			// customized files are preserved; a write failure fails init
			// naming the path (DI-FR-001, design R2).
			opencodeCmdDir := filepath.Join(vaultPath, ".opencode", "commands")
			if _, err := os.Stat(filepath.Join(vaultPath, ".opencode")); err == nil {
				// Migrate Dewey slash commands from old .opencode/command/ (singular)
				// to .opencode/commands/ (plural). Only moves Dewey-owned files.
				oldCmdDir := filepath.Join(vaultPath, ".opencode", "command")
				if _, err := os.Stat(oldCmdDir); err == nil {
					if err := os.MkdirAll(opencodeCmdDir, 0o755); err != nil {
						logger.Warn("failed to create commands dir for migration", "err", err)
					} else {
						for name := range deweySlashCommands {
							oldPath := filepath.Join(oldCmdDir, name)
							if _, err := os.Stat(oldPath); err != nil {
								continue // File not in old dir — nothing to migrate.
							}
							newPath := filepath.Join(opencodeCmdDir, name)
							if _, err := os.Stat(newPath); err == nil {
								// Already exists in new dir — remove old copy.
								if rmErr := os.Remove(oldPath); rmErr != nil {
									logger.Warn("failed to remove old slash command", "path", oldPath, "err", rmErr)
								} else {
									logger.Info("removed old slash command (already in new dir)", "name", name)
								}
								continue
							}
							// Move file from old to new directory.
							if mvErr := os.Rename(oldPath, newPath); mvErr != nil {
								logger.Warn("failed to migrate slash command", "name", name, "err", mvErr)
							} else {
								logger.Info("migrated slash command", "from", oldPath, "to", newPath)
							}
						}
						// Remove old directory if empty.
						entries, err := os.ReadDir(oldCmdDir)
						if err == nil && len(entries) == 0 {
							if rmErr := os.Remove(oldCmdDir); rmErr != nil {
								logger.Warn("failed to remove empty old command dir", "err", rmErr)
							} else {
								logger.Info("removed empty old command directory", "path", oldCmdDir)
							}
						}
					}
				}

				if err := scaffoldToolOwnedFiles(opencodeCmdDir, deweySlashCommands, scaffoldMarkdown, "slash command"); err != nil {
					return err
				}
			}

			// Scaffold Dewey-specific agent definitions into
			// .opencode/agents/ when the .opencode/ directory exists
			// (composability — only scaffold when OpenCode is present).
			// Tool-owned refresh through the shared scaffold discipline
			// (DI-FR-001/DI-FR-002): untouched scaffolds are refreshed to
			// the embedded content, customized files are preserved, and a
			// failed or partial write is rolled back and fails init naming
			// the target path.
			if fileExists(filepath.Join(vaultPath, ".opencode")) {
				agentsDir := filepath.Join(vaultPath, ".opencode", "agents")
				if err := scaffoldToolOwnedFiles(agentsDir, deweyAgentDefinitions, scaffoldMarkdown, "agent"); err != nil {
					return err
				}
			}

			// Scaffold DCP configuration (.opencode/dcp.jsonc) to enable
			// protectTags for slash command <protect> blocks.
			// Composability guard: only scaffold when .opencode/ exists.
			// Three-way idempotent logic:
			//   1. Skip if dcp.jsonc or dcp.json already has protectTags
			//   2. Warn if file exists without protectTags (don't overwrite)
			//   3. Create dcp.jsonc if neither file exists
			if fileExists(filepath.Join(vaultPath, ".opencode")) {
				dcpJSONC := filepath.Join(vaultPath, ".opencode", "dcp.jsonc")
				dcpJSON := filepath.Join(vaultPath, ".opencode", "dcp.json")

				switch {
				case fileContains(dcpJSONC, "protectTags") || fileContains(dcpJSON, "protectTags"):
					// Already configured — skip silently.
				case fileExists(dcpJSONC) || fileExists(dcpJSON):
					// File exists but missing protectTags — warn, don't overwrite.
					logger.Warn("DCP config exists but missing protectTags; add \"compress\": {\"protectTags\": true} to enable protect-tag support")
				default:
					// Neither file exists — scaffold dcp.jsonc.
					if writeErr := os.WriteFile(dcpJSONC, []byte(dcpJSONCContent), 0o644); writeErr != nil {
						logger.Warn("failed to scaffold DCP config", "path", dcpJSONC, "err", writeErr)
					} else {
						logger.Info("scaffolded DCP config", "path", dcpJSONC)
					}
				}
			}

			if !alreadyInitialized {
				logger.Info("initialized", "path", deweyDir)
				logger.Info("run 'dewey index' to build the initial index")
			} else {
				logger.Info("re-initialized", "path", deweyDir)
			}
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

// trustStatus reports the git-trust derivation state (GTS-FR-004
// observability): the default-branch comparison ref and how the vault's
// compiled/ and knowledge/ files compare against it. Shared by `dewey
// status` and `dewey doctor` through queryTrustStatus.
type trustStatus struct {
	// Ref is the effective <remote>/<default-branch> value that was
	// attempted, exactly as the winning precedence leg provided it.
	Ref string `json:"ref"`
	// Status classifies the ref resolution outcome (gitref.Status).
	Status string `json:"status"`
	// SHA is the fetched ref's commit SHA, empty when unresolved.
	SHA string `json:"sha,omitempty"`
	// Leg names the precedence source that supplied Ref.
	Leg string `json:"leg,omitempty"`
	// Reason carries the fallback cause when Status is not resolved.
	Reason string `json:"reason,omitempty"`
	// TrustFiles counts markdown files under compiled/ and knowledge/.
	TrustFiles int `json:"trust_files"`
	// ValidatedEligible counts files claiming canonical validated whose
	// working-tree blob equals the default-branch blob (GTS-FR-001 — they
	// index as validated).
	ValidatedEligible int `json:"validated_eligible"`
	// Differing counts files whose blob differs from the default branch
	// (working-tree edits or unmerged branches — they index as draft).
	Differing int `json:"differing"`
	// DifferPaths lists up to 10 vault-relative differing paths for
	// verbose diagnostics; never serialized.
	DifferPaths []string `json:"-"`
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
	Trust              *trustStatus
	// DeweyVersion is the stored `dewey_version` metadata key: the
	// resolved version of the binary that last completed an indexing
	// pass over this store, recorded verbatim. Empty when the key is
	// absent or there is no store (IBP-FR-003, design D5).
	DeweyVersion string
	// IndexFormat is the stored `index_format` metadata key; a missing
	// or non-decimal value counts as 0 (parseIndexFormat).
	IndexFormat int
	// ReindexRecommended is true iff IndexFormat is strictly lower than
	// this binary's IndexFormat (reindexDecision). False without a
	// store — there is nothing to rebuild.
	ReindexRecommended bool
	// ReindexReasons carries the reindex recommendation reasons; empty
	// and non-nil when not recommended so status JSON renders an array,
	// never null.
	ReindexReasons []string
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

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			data, err := queryStoreStatus(deweyDir)
			if err != nil {
				return err
			}

			// Trust-tier derivation state (git-derived tiers): the
			// default-branch ref and the compiled/knowledge blob
			// comparison behind every pages.tier in the index.
			data.Trust = queryTrustStatus(vp)

			// Embedding layer: the model resolved the way serve resolves
			// it, plus a diagnostic-only availability probe. Model and
			// availability come from one config resolution (design D3).
			data.EmbeddingModel, data.EmbeddingAvailable = queryEmbeddingStatus(deweyDir)

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

// queryStoreStatus opens the store at deweyDir, queries all counts, source
// records, and index-build provenance, and returns a populated statusData.
// The store is closed before returning. Returns a zero-value statusData
// (with IndexPath set) if the database does not yet exist — empty version,
// format 0, and no reindex recommendation: nothing has been built, so there
// is nothing to rebuild. The embedding model and availability are resolved
// separately by the caller via queryEmbeddingStatus.
func queryStoreStatus(deweyDir string) (statusData, error) {
	data := statusData{
		IndexPath: deweyDir,
		// Non-nil from the start so every return path carries reasons
		// as an array, never null — the same non-nil contract
		// reindexDecision guarantees for its own results.
		ReindexReasons: []string{},
	}

	dbPath := filepath.Join(deweyDir, "graph.db")
	if _, err := os.Stat(dbPath); err != nil {
		// Database does not exist yet — return zero counts and no
		// index provenance (no recommendation: nothing to rebuild).
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

	// Index-build provenance (IBP-FR-003, design D5): read-only — status
	// never stamps either key. GetMeta reads a missing key as "" with no
	// error, so an absent stamp flows through parseIndexFormat's
	// counts-as-zero rule; a genuine read failure degrades the same way
	// (over-recommending, the fail-safe direction) instead of failing the
	// command, matching the auxiliary counts above.
	if v, err := s.GetMeta("dewey_version"); err == nil {
		data.DeweyVersion = v
	}
	if f, err := s.GetMeta("index_format"); err == nil {
		data.IndexFormat = parseIndexFormat(f)
	}
	data.ReindexRecommended, data.ReindexReasons = reindexDecision(data.IndexFormat, IndexFormat)

	return data, nil
}

// queryEmbeddingStatus resolves the embedding model the way serve does
// (embed.ReadEmbeddingConfig: DEWEY_EMBEDDING_MODEL over per-vault over
// global config) and probes availability through the configured embedder's
// Available() method — the same probe the MCP health tool uses.
// Diagnostic-only: never auto-starts Ollama or spawns subprocesses (the
// doctor autoStart=false stance); a provider that cannot be constructed
// (e.g. vertex without project) reports unavailable while the model still
// comes from config. Relies on the embedders' built-in bounded probes so an
// unreachable provider cannot hang the command.
func queryEmbeddingStatus(deweyDir string) (model string, available bool) {
	cfg := embed.ReadEmbeddingConfig(deweyDir)
	embedder, err := embed.NewEmbedderFromConfig(cfg)
	if err != nil {
		// Construction failure is unavailable, not a command error, so a
		// misconfigured provider cannot fail `dewey status` (design D2).
		return cfg.Model, false
	}
	return cfg.Model, embedder.Available()
}

// parseIndexFormat returns the stored index format; missing or
// non-decimal values count as 0 (IBP-FR-003, design D5). It is the
// read-side inverse of recordIndexBuild's strconv.Itoa write, so a
// stamp written by this codebase round-trips exactly. Pure function
// over its input: no store access, no I/O, no side effects.
func parseIndexFormat(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// reindexDecision returns whether a reindex is recommended and why.
// Recommended iff stored < current. The reason is exactly
// "index format <stored>, this Dewey writes <current>" (IBP-FR-003,
// design D5); equal or newer stored formats are not flagged. When
// not recommended, the reason slice is empty and non-nil so status
// JSON renders an array, never null. Pure function over its inputs:
// no store access, no I/O, no side effects.
func reindexDecision(storedFormat, currentFormat int) (bool, []string) {
	if storedFormat >= currentFormat {
		return false, []string{}
	}
	return true, []string{
		fmt.Sprintf("index format %d, this Dewey writes %d", storedFormat, currentFormat),
	}
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

	// Index-build provenance (IBP-FR-005, design D5): the recorded
	// builder, verbatim, and when the stored format is older than this
	// binary's, the reindex recommendation. An absent stamp reads
	// "not recorded" rather than printing an empty value, so the line
	// stays meaningful for a fresh or pre-tracking index.
	indexedBy := data.DeweyVersion
	if indexedBy == "" {
		indexedBy = "not recorded"
	}
	_, _ = fmt.Fprintf(w, "  Indexed by: %s (index format %d)\n", indexedBy, data.IndexFormat)
	if data.ReindexRecommended {
		_, _ = fmt.Fprintf(w, "  Reindex:    %s\n", strings.Join(data.ReindexReasons, "; "))
	}

	if data.Trust != nil {
		_, _ = fmt.Fprintln(w, "\nTrust Tiers")
		switch {
		case data.Trust.Status == string(gitref.StatusResolved):
			_, _ = fmt.Fprintf(w, "  Ref:        %s @ %s (via %s)\n",
				data.Trust.Ref, shortRefSHA(data.Trust.SHA), data.Trust.Leg)
			_, _ = fmt.Fprintf(w, "  Files:      %d compiled/knowledge file(s): %d validated-eligible, %d differing from %s\n",
				data.Trust.TrustFiles, data.Trust.ValidatedEligible, data.Trust.Differing, data.Trust.Ref)
		default:
			_, _ = fmt.Fprintf(w, "  Ref:        %s (%s)\n", data.Trust.Ref, data.Trust.Status)
			_, _ = fmt.Fprintf(w, "  Files:      %d compiled/knowledge file(s) degrade to draft until the ref resolves\n",
				data.Trust.TrustFiles)
		}
	}

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
	// reindexReasons must always render as an array, never null
	// (IBP-FR-003): json.Marshal renders a nil slice as null, and a
	// statusData that never went through queryStoreStatus (a zero
	// value, a test fixture) carries nil reasons even though every
	// store-backed path seeds non-nil.
	reindexReasons := data.ReindexReasons
	if reindexReasons == nil {
		reindexReasons = []string{}
	}
	status := map[string]any{
		"path":               data.IndexPath,
		"pages":              data.PageCount,
		"blocks":             data.BlockCount,
		"embeddings":         data.EmbeddingCount,
		"embeddingModel":     data.EmbeddingModel,
		"embeddingAvailable": data.EmbeddingAvailable,
		"embeddingCoverage":  data.embeddingCoverage(),
		"deweyVersion":       data.DeweyVersion,
		"indexFormat":        data.IndexFormat,
		"reindexRecommended": data.ReindexRecommended,
		"reindexReasons":     reindexReasons,
		"sources":            data.Sources,
	}
	if data.Trust != nil {
		status["trust"] = data.Trust
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

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
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
			embedder, err := createIndexEmbedder(noEmbeddings, deweyDir)
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

			embCfg := embed.ReadEmbeddingConfig(deweyDir)
			// Tier-blob resolver (default-branch ref + batch blob map, one
			// ls-tree per index run per design D2) for compiled/curated
			// trust-tier derivation (IP-FR-001); an unresolvable ref
			// withholds the validated grant (GTS-FR-004) without failing
			// the index.
			tierBlobs := gitref.NewPageBlobResolver(context.Background(),
				gitref.Options{VaultDir: vp}, gitref.TrustPathspecs())
			indexResult, indexErr := vault.IndexDocuments(s, allDocs, configs, embedder, embCfg.MaxChunkChars, tierBlobs)
			reportSourceErrors(s, result)

			if indexErr != nil {
				return fmt.Errorf("index failed: %w", indexErr)
			}

			// Stamp the store with this binary's build provenance now that
			// the pass completed (IBP-FR-001, design D4). A failed pass above
			// returns early and never stamps; recording the stamp is part of
			// the command's job, so a stamp failure fails the command.
			if err := recordIndexBuild(s, resolveVersion(), IndexFormat); err != nil {
				return fmt.Errorf("record index build: %w", err)
			}

			logger.Info("index complete",
				"documents", indexResult.TotalIndexed,
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
// full index from scratch, then restoring learnings and knowledge-store
// pages from their files (restoreFileBackedPages, #223). This is the
// recommended way to rebuild the index after upgrading Dewey or when the
// index is corrupted.
func newReindexCmd() *cobra.Command {
	var noEmbeddings bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Delete and rebuild the index from scratch",
		Long: `Remove the existing graph.db and rebuild the index from scratch.

This is equivalent to manually deleting .uf/dewey/graph.db and its WAL/SHM
files, then running 'dewey index --force'. Use this when:
  - Upgrading Dewey to a version with schema or indexing changes
  - The index is corrupted (UUID collisions, foreign key errors)
  - You want a guaranteed clean slate

The command removes: graph.db, graph.db-wal, graph.db-shm, dewey.lock

After indexing the configured sources it re-ingests learnings from
.uf/dewey/learnings/ and re-indexes configured knowledge stores from
their files, so the rebuilt index is complete without starting
'dewey serve'.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reindexStart := time.Now()
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			logger.Info("reindex starting", "vault", vp, "deweyDir", deweyDir, "pid", os.Getpid())

			// Acquire the lock FIRST to prevent TOCTOU race conditions.
			// If another dewey process holds the lock, we fail immediately
			// instead of checking and then racing to remove files.
			lockPath := filepath.Join(deweyDir, "dewey.lock")
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
			embedder, err := createIndexEmbedder(noEmbeddings, deweyDir)
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

			embCfg := embed.ReadEmbeddingConfig(deweyDir)
			// Same per-index-run tier-blob resolver as `dewey index`
			// (design D2, IP-FR-001).
			tierBlobs := gitref.NewPageBlobResolver(context.Background(),
				gitref.Options{VaultDir: vp}, gitref.TrustPathspecs())
			indexResult, indexErr := vault.IndexDocuments(s, allDocs, configs, embedder, embCfg.MaxChunkChars, tierBlobs)
			reportSourceErrors(s, result)

			if indexErr != nil {
				return fmt.Errorf("reindex failed: %w", indexErr)
			}

			// Restore file-backed pages — learnings and knowledge stores —
			// that no source reaches, so the rebuilt index is complete
			// without a `dewey serve` start (#223). Runs under the same
			// exclusive use of graph.db as the rest of the rebuild.
			restoreFileBackedPages(s, embedder, vp, embCfg.MaxChunkChars)

			// Stamp the freshly rebuilt store with this binary's build
			// provenance (IBP-FR-001, design D4), mirroring `dewey index`:
			// only a completed rebuild stamps, and a stamp failure fails the
			// command.
			if err := recordIndexBuild(s, resolveVersion(), IndexFormat); err != nil {
				return fmt.Errorf("record index build: %w", err)
			}

			// Verify final state.
			finalPages, _ := s.ListPages()
			elapsed := time.Since(reindexStart)
			logger.Info("reindex complete",
				"documents", indexResult.TotalIndexed,
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

// detectLockHolder checks if the dewey.lock file is held by another process.
// Returns a description of the holder (e.g., "PID 12345 dewey serve --vault /path")
// or empty string if unlocked. When the lock is held but no PID was written
// (pre-T011 lock files), returns a generic "lock held" message.
func detectLockHolder(lockPath string) string {
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0o644)
	if err != nil {
		return "" // file doesn't exist or can't be opened — not locked
	}
	defer func() { _ = f.Close() }()

	// Try to acquire a non-blocking exclusive lock.
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock is held by another process. Read PID info written by T011.
		data, readErr := os.ReadFile(lockPath)
		if readErr == nil {
			firstLine := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
			if firstLine != "" {
				return fmt.Sprintf("PID %s", firstLine)
			}
		}
		// Lock held but no PID data (old-format lock file).
		return "lock held (unknown process)"
	}
	// We got the lock — release it, no one is holding it.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return ""
}

// createIndexEmbedder creates an Embedder for use during indexing.
// Reads provider configuration from the dewey workspace config.yaml,
// falling back to environment variables and Ollama defaults.
// When noEmbeddings is true, returns nil (no embedder). When the provider
// is Ollama, attempts auto-start. Returns nil on graceful degradation
// and a hard error only when the provider is available but the model is not.
func createIndexEmbedder(noEmbeddings bool, deweyDirs ...string) (embed.Embedder, error) {
	if noEmbeddings {
		logger.Info("embeddings disabled via --no-embeddings")
		return nil, nil
	}

	// Read config from the dewey workspace directory.
	// The config reader handles env var overrides and defaults.
	var deweyDir string
	if len(deweyDirs) > 0 {
		deweyDir = deweyDirs[0]
	}
	cfg := embed.ReadEmbeddingConfig(deweyDir)

	// For Ollama provider, ensure Ollama is running (auto-start if needed).
	if cfg.Provider == "ollama" || cfg.Provider == "" {
		ollamaState, err := ensureOllama(cfg.Endpoint, true, &execOllamaStarter{})
		if err != nil {
			logger.Warn("ollama auto-start failed, continuing without embeddings", "err", err)
		}
		logger.Info("ollama state", "state", ollamaState, "endpoint", cfg.Endpoint)

		if ollamaState == OllamaUnavailable {
			logger.Info("semantic search unavailable — ollama not installed",
				"install", "brew install ollama")
			return nil, nil
		}
	}

	if cfg.Provider == "openai-compatible" {
		logGatewayCredentialWarnings(openaicompat.CapabilityEmbedding, cfg.Endpoint, embed.HasAPIKeyField(deweyDir))
	}

	embedder, err := embed.NewEmbedderFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("embedding provider error: %w", err)
	}
	if !embedder.Available() {
		// Graceful degradation: keyword-only mode when the model is
		// unavailable. Matches the OllamaUnavailable path above — warn and
		// continue without embeddings instead of hard-exiting (D3). The
		// ollama pull hint is Ollama-only (FR-024).
		logger.Warn("embedding model not available, continuing without embeddings",
			embeddingDegradationFields(cfg, embedder)...)
		return nil, nil
	}
	logger.Info("embedding model available for indexing", "provider", cfg.Provider, "model", cfg.Model)
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
//	✅ name                description
//
// The name field is left-aligned and padded to 20 characters. The marker
// is one of PASS, WARN, or FAIL — the counter is incremented accordingly
// and the corresponding emoji (✅, ⚠️, ❌) is displayed.
func (c *doctorCounter) printCheck(w io.Writer, marker, name, description string) {
	var emoji string
	switch marker {
	case "PASS":
		c.pass++
		emoji = "✅"
	case "WARN":
		c.warn++
		emoji = "⚠️"
	case "FAIL":
		c.fail++
		emoji = "❌"
	case "INFO":
		emoji = "ℹ️"
	}
	_, _ = fmt.Fprintf(w, "  %s %-20s%s\n", emoji, name, description)
}

// isLegacyEmbeddingModel reports whether the given model name is a legacy
// embedding model that has a newer replacement available. Used by the doctor
// advisory to suggest upgrading.
func isLegacyEmbeddingModel(model string) bool {
	return model == "granite-embedding:30m"
}

// countExcludedDirs walks the vault directory and counts how many directories
// would be skipped by the ignore matcher. Used by the doctor verbose report
// to show the impact of .gitignore and ignore patterns.
func countExcludedDirs(vaultPath string, matcher *ignore.Matcher) int {
	count := 0
	_ = filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || path == vaultPath {
			return nil
		}
		if matcher.ShouldSkip(info.Name(), true) {
			count++
			return filepath.SkipDir
		}
		return nil
	})
	return count
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
// `uf doctor` — grouped sections, ✅/⚠️/❌ emoji markers, descriptions
// with paths, and Fix: hints for actionable remediation.
func runDoctorChecks(w io.Writer, vaultPath string) {
	dp := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	c := &doctorCounter{}

	// Lipgloss styles matching uf doctor (format.go).
	renderer := lipgloss.NewRenderer(w)
	boldStyle := renderer.NewStyle().Bold(true)
	section := func(name string) { _, _ = fmt.Fprintln(w, boldStyle.Render(name)) }

	embeddingCount := -1 // -1 = not queried; set by store section if available.
	dp("🩺 Dewey Doctor\n\n")

	// --- Environment ---
	section("Environment")
	c.printCheck(w, "PASS", "vault", vaultPath)

	deweyBin, err := os.Executable()
	if err == nil {
		c.printCheck(w, "PASS", "dewey", fmt.Sprintf("v%s (%s)", version, deweyBin))
	} else {
		c.printCheck(w, "WARN", "dewey", "could not determine path")
	}
	dp("\n")

	// --- Workspace ---
	section("Workspace")
	deweyDir := filepath.Join(vaultPath, deweyWorkspaceDir)
	if _, err := os.Stat(deweyDir); err == nil {
		c.printCheck(w, "PASS", ".uf/dewey/", fmt.Sprintf("initialized (%s)", deweyDir))
	} else {
		c.printCheck(w, "FAIL", ".uf/dewey/", "not found")
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

	// Verbose mode: report ignore rules and excluded directories.
	// Only shown when --verbose / -v is active (logger level is DebugLevel).
	if logger.GetLevel() == log.DebugLevel {
		matcher, matcherErr := ignore.NewMatcher(
			filepath.Join(vaultPath, ".gitignore"),
			nil, // no extra patterns — just .gitignore baseline
		)
		if matcherErr == nil {
			excludedDirs := countExcludedDirs(vaultPath, matcher)
			if excludedDirs > 0 {
				c.printCheck(w, "PASS", "ignore rules", fmt.Sprintf("%d directories excluded", excludedDirs))
			}
		}
	}
	dp("\n")

	// --- Database ---
	section("Database")
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
				section("Sources in Database")
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
						dp("     Fix: dewey reindex\n")
					}
				}
			}

			// --- Content Sanitization ---
			// Query pages with sanitization findings to report content
			// health across indexed sources (FR-SAN-009).
			sanitizePages, sanitizeErr := s.GetPagesWithProperty("sanitize_findings")
			if sanitizeErr == nil && len(sanitizePages) > 0 {
				dp("\n")
				section("Content Sanitization")

				// Aggregate finding counts by severity and source.
				type sourceFindings struct {
					total    int
					critical int
					high     int
					medium   int
					low      int
					info     int
				}
				bySource := make(map[string]*sourceFindings)

				for _, p := range sanitizePages {
					var props map[string]any
					if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
						continue
					}
					findingsRaw, ok := props["sanitize_findings"]
					if !ok {
						continue
					}
					// Re-marshal and unmarshal to get typed findings.
					findingsJSON, err := json.Marshal(findingsRaw)
					if err != nil {
						continue
					}
					var findings []sanitize.Finding
					if err := json.Unmarshal(findingsJSON, &findings); err != nil {
						continue
					}

					srcID := p.SourceID
					if srcID == "" {
						srcID = "unknown"
					}
					sf, exists := bySource[srcID]
					if !exists {
						sf = &sourceFindings{}
						bySource[srcID] = sf
					}
					for _, f := range findings {
						sf.total++
						switch f.Severity {
						case "critical":
							sf.critical++
						case "high":
							sf.high++
						case "medium":
							sf.medium++
						case "low":
							sf.low++
						case "info":
							sf.info++
						}
					}
				}

				// Display per-source breakdown.
				hasCritical := false
				for srcID, sf := range bySource {
					if sf.critical > 0 {
						hasCritical = true
					}
					desc := fmt.Sprintf("%d findings", sf.total)
					if sf.critical > 0 || sf.high > 0 {
						desc += fmt.Sprintf(" (%d critical, %d high)", sf.critical, sf.high)
					}
					if sf.critical > 0 {
						c.printCheck(w, "WARN", srcID, desc)
					} else {
						c.printCheck(w, "PASS", srcID, desc)
					}
				}

				// Summary line.
				if hasCritical {
					c.printCheck(w, "WARN", "sanitization", fmt.Sprintf("%d pages with findings", len(sanitizePages)))
				} else {
					c.printCheck(w, "PASS", "sanitization", fmt.Sprintf("%d pages with findings (no critical)", len(sanitizePages)))
				}
			} else if sanitizeErr == nil {
				// No pages with findings — clean.
				// Only show the section if there are indexed pages to scan.
				if len(allPages) > 0 {
					dp("\n")
					section("Content Sanitization")
					c.printCheck(w, "PASS", "sanitization", "no findings")
				}
			}

			// Query embedding count while store is still open.
			if ec, ecErr := s.CountEmbeddings(); ecErr == nil {
				embeddingCount = ec
			}
		}
	}
	dp("\n")

	// --- Trust Tiers ---
	// Git-derived trust tier state (GTS-FR-004 observability): the
	// default-branch comparison ref and the compiled/knowledge blob
	// comparison behind every pages.tier in the index.
	section("Trust Tiers")
	printTrustTierChecks(c, w, dp, vaultPath)
	dp("\n")

	// --- Embedding Layer ---
	embCfg := embed.ReadEmbeddingConfig(deweyDir)
	embedEndpoint := embCfg.Endpoint
	embedModel := embCfg.Model

	switch embCfg.Provider {
	case "ollama", "":
		section(fmt.Sprintf("Embedding Layer (%s via %s)", embedModel, embedEndpoint))

		// Ollama state check via ensureOllama (autoStart=false for doctor —
		// doctor is diagnostic-only, it should never start subprocesses).
		ollamaState, _ := ensureOllama(embedEndpoint, false, nil)
		ollamaReachable := false
		switch ollamaState {
		case OllamaExternal:
			ollamaReachable = true
			c.printCheck(w, "PASS", "ollama", fmt.Sprintf("running (external) (%s)", embedEndpoint))
		case OllamaUnavailable:
			// Distinguish between "not running" (binary exists) and "not installed" (optional).
			// Per composability principle: Ollama is optional, so absence is PASS, not FAIL.
			if _, lookErr := exec.LookPath("ollama"); lookErr == nil {
				c.printCheck(w, "WARN", "ollama", "not running")
				dp("     Fix: ollama serve\n")
			} else {
				c.printCheck(w, "PASS", "ollama", "not installed (optional)")
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

		// Legacy model advisory — inform users about the newer R2 model.
		if isLegacyEmbeddingModel(embedModel) {
			c.printCheck(w, "INFO", "model", "granite-embedding:30m has a newer replacement (Granite Embedding R2)")
			dp("     Tip: update embedding.model in config.yaml, then run: dewey reindex\n")
		}

	case "vertex":
		printVertexEmbeddingChecks(c, w, section, embCfg)

	case "openai-compatible":
		section(fmt.Sprintf("Embedding Layer (%s via %s)", displayGatewayValue(embedModel), displayGatewayValue(openaicompat.DisplayURL(embedEndpoint))))
		embedder, err := embed.NewEmbedderFromConfig(embCfg)
		if err != nil {
			c.printCheck(w, "FAIL", "config", fmt.Sprintf("incomplete: %s", err))
		} else {
			c.printCheck(w, "PASS", "config", fmt.Sprintf("provider=%s endpoint=%s model=%s", embCfg.Provider, openaicompat.DisplayURL(embedEndpoint), embedModel))
			if embedder.Available() {
				c.printCheck(w, "PASS", "provider", "ready")
			} else {
				c.printCheck(w, "WARN", "provider", embedderUnavailableReason(embedder))
			}
		}
		printGatewayCredentialChecks(c, w, openaicompat.CapabilityEmbedding, embCfg.Endpoint, embed.HasAPIKeyField(deweyDir))

	default:
		section(fmt.Sprintf("Embedding Layer (unknown provider: %s)", embCfg.Provider))
		c.printCheck(w, "FAIL", "provider", fmt.Sprintf("unknown provider %q (supported: ollama, vertex, openai-compatible)", embCfg.Provider))
	}

	// Embedding count — uses value queried from the already-open store above.
	if embeddingCount > 0 {
		c.printCheck(w, "PASS", "embeddings", fmt.Sprintf("%d in database", embeddingCount))
	} else if embeddingCount == 0 {
		c.printCheck(w, "WARN", "embeddings", "0 in database")
		if embCfg.Provider == "ollama" || embCfg.Provider == "" {
			dp("     Fix: dewey reindex (with Ollama running)\n")
		} else {
			dp("     Fix: dewey reindex\n")
		}
	}
	dp("\n")

	// --- Synthesis Layer ---
	synthCfg := llm.ReadSynthesisConfig(deweyDir)

	switch synthCfg.Provider {
	case "ollama", "":
		if synthCfg.Model == "" {
			// No synthesis configured — this is optional per Composability First.
			section("Synthesis Layer (not configured)")
			c.printCheck(w, "PASS", "synthesis", "not configured (optional)")
		} else {
			synthEndpoint := synthCfg.Endpoint
			if synthEndpoint == "" {
				synthEndpoint = llm.ResolveSynthesisEndpoint()
			}
			section(fmt.Sprintf("Synthesis Layer (%s via %s)", synthCfg.Model, synthEndpoint))

			// Ollama connectivity check (autoStart=false — doctor is diagnostic-only).
			synthOllamaState, _ := ensureOllama(synthEndpoint, false, nil)
			synthReachable := false
			// OllamaManaged is unreachable here because autoStart=false.
			switch synthOllamaState {
			case OllamaExternal:
				synthReachable = true
				c.printCheck(w, "PASS", "ollama", fmt.Sprintf("running (external) (%s)", synthEndpoint))
			case OllamaUnavailable:
				if _, lookErr := exec.LookPath("ollama"); lookErr == nil {
					c.printCheck(w, "WARN", "ollama", "not running")
					dp("     Fix: ollama serve\n")
				} else {
					c.printCheck(w, "PASS", "ollama", "not installed (optional)")
				}
			}

			// Model availability.
			if synthReachable {
				synth := llm.NewOllamaSynthesizer(synthEndpoint, synthCfg.Model)
				if synth.Available() {
					c.printCheck(w, "PASS", "model", fmt.Sprintf("%s ready", synthCfg.Model))
				} else {
					c.printCheck(w, "FAIL", "model", fmt.Sprintf("%s not available", synthCfg.Model))
					dp("     Fix: ollama pull %s\n", synthCfg.Model)
				}
			} else {
				c.printCheck(w, "WARN", "model", fmt.Sprintf("%s skipped (ollama not reachable)", synthCfg.Model))
			}
		}

	case "vertex":
		section(fmt.Sprintf("Synthesis Layer (%s via vertex)", synthCfg.Model))

		// Vertex config completeness — no live API calls (no GCP credentials required).
		_, synthErr := llm.NewSynthesizerFromConfig(synthCfg)
		if synthErr != nil {
			c.printCheck(w, "FAIL", "config", fmt.Sprintf("incomplete: %s", synthErr))
		} else {
			c.printCheck(w, "PASS", "config", fmt.Sprintf("project=%s region=%s", synthCfg.Project, synthCfg.Region))
		}

	case "openai-compatible":
		section(fmt.Sprintf("Synthesis Layer (%s via openai-compatible)", synthCfg.Model))

		// Configuration completeness only — no live API call and the key is
		// never printed.
		if synthCfg.Endpoint == "" {
			c.printCheck(w, "FAIL", "endpoint", "missing")
			dp("     Fix: set synthesis.endpoint in .uf/dewey/config.yaml\n")
		} else if u, err := llm.ValidateSynthesisEndpoint(synthCfg.Endpoint); err != nil {
			c.printCheck(w, "FAIL", "endpoint", "invalid URL (must be an absolute http(s) URL without credentials, query, or fragment)")
			dp("     Fix: use a plain absolute http(s) URL without credentials, query, or fragment\n")
		} else {
			// Print the sanitized endpoint (no userinfo/query) so credentials
			// embedded in the URL never reach terminal output or logs.
			c.printCheck(w, "PASS", "endpoint", u.Scheme+"://"+u.Host+u.Path)
			host := u.Hostname()
			loopback := llm.IsLoopbackHost(host)
			if u.Scheme == "http" && !loopback {
				c.printCheck(w, "WARN", "tls", "plaintext http endpoint; the API key and prompts are sent in cleartext")
				dp("     Fix: use https:// or a loopback endpoint\n")
			}
		}
		if synthCfg.Model == "" {
			c.printCheck(w, "FAIL", "model", "missing")
			dp("     Fix: set synthesis.model in .uf/dewey/config.yaml\n")
		} else {
			c.printCheck(w, "PASS", "model", synthCfg.Model)
		}
		if llm.ResolveSynthesisAPIKey(synthCfg.APIKey) == "" {
			c.printCheck(w, "FAIL", "credential", "missing")
			dp("     Fix: set DEWEY_SYNTHESIS_API_KEY (recommended) or synthesis.api_key; the per-vault\n")
			dp("     .uf/dewey/config.yaml is git-tracked (dewey init keeps it shareable), so keep secrets\n")
			dp("     in the env var or the global ~/.config/dewey/config.yaml\n")
		} else {
			c.printCheck(w, "PASS", "credential", "configured")
		}

	default:
		section(fmt.Sprintf("Synthesis Layer (unknown provider: %s)", synthCfg.Provider))
		c.printCheck(w, "FAIL", "provider", fmt.Sprintf("unknown provider %q (supported: ollama, vertex, openai-compatible)", synthCfg.Provider))
	}
	dp("\n")

	// --- MCP Server ---
	section("MCP Server")

	// Check if a dewey serve process is running (consolidated lock check per D5).
	lockPath := filepath.Join(deweyDir, "dewey.lock")
	if _, err := os.Stat(lockPath); err == nil {
		holder := detectLockHolder(lockPath)
		if holder != "" {
			c.printCheck(w, "PASS", "serve", fmt.Sprintf("running (%s)", holder))
		} else {
			c.printCheck(w, "PASS", "serve", "not running (stale lock file)")
			dp("     Fix: rm %s\n", lockPath)
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

	// Check DCP config for protectTags.
	// Composability: only check when .opencode/ exists (silent skip otherwise).
	if fileExists(filepath.Join(vaultPath, ".opencode")) {
		dcpJSONC := filepath.Join(vaultPath, ".opencode", "dcp.jsonc")
		dcpJSON := filepath.Join(vaultPath, ".opencode", "dcp.json")

		switch {
		case fileContains(dcpJSONC, "protectTags") || fileContains(dcpJSON, "protectTags"):
			c.printCheck(w, "PASS", "dcp.jsonc", "protectTags enabled")
		case fileExists(dcpJSONC) || fileExists(dcpJSON):
			c.printCheck(w, "WARN", "dcp.jsonc", "exists but protectTags not configured")
			dp("     Fix: Add \"compress\": {\"protectTags\": true} to your DCP config\n")
		default:
			c.printCheck(w, "WARN", "dcp.jsonc", "not found (protectTags disabled)")
			dp("     Fix: dewey init\n")
		}
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

			deweyDir := filepath.Join(cwd, deweyWorkspaceDir)
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

// --- Manifest command (T008) ---

// newManifestCmd creates the `dewey manifest` subcommand.
// Walks Go source files in the vault path, runs the chunker to extract
// declarations, and generates .uf/dewey/manifest.md with sections for CLI
// Commands, MCP Tools, and Exported Packages. Empty sections are omitted.
// Per contracts/manifest-command.md.
func newManifestCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Generate project manifest from source code",
		Long: `Walk Go source files and generate .uf/dewey/manifest.md with sections
for CLI Commands, MCP Tools, and Exported Packages.

The manifest uses the same chunker infrastructure as the code source indexer.
It is idempotent — running twice produces the same output for the same input.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			blocks, err := collectManifestBlocks(vp)
			if err != nil {
				return err
			}

			manifestContent := formatManifest(blocks)

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if err := os.MkdirAll(deweyDir, 0o755); err != nil {
				return fmt.Errorf("create .uf/dewey/ directory: %w", err)
			}

			manifestPath := filepath.Join(deweyDir, "manifest.md")
			if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			// Count items per section for the log message.
			cmdCount, toolCount, pkgCount := countManifestItems(blocks)
			logger.Info("manifest generated",
				"path", manifestPath,
				"commands", cmdCount,
				"tools", toolCount,
				"packages", pkgCount,
			)

			return nil
		},
	}

	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to vault (default: current directory)")

	return cmd
}

// collectManifestBlocks walks the vault directory for .go files, runs the
// Go chunker on each, and returns all extracted blocks. Skips test files,
// ignored directories, and files with syntax errors.
func collectManifestBlocks(vaultPath string) ([]chunker.Block, error) {
	// Build ignore matcher from .gitignore if present.
	gitignorePath := filepath.Join(vaultPath, ".gitignore")
	matcher, err := ignore.NewMatcher(gitignorePath, nil)
	if err != nil {
		return nil, fmt.Errorf("create ignore matcher: %w", err)
	}

	goChunker, ok := chunker.Get("go")
	if !ok {
		// Go chunker not registered — return empty blocks.
		// This can happen if the chunker package wasn't imported.
		logger.Warn("go chunker not registered — import _ \"github.com/unbound-force/dewey/v3/chunker\"")
		return nil, nil
	}

	var allBlocks []chunker.Block

	walkErr := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		// Skip ignored directories.
		if info.IsDir() {
			if matcher.ShouldSkip(info.Name(), true) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .go files.
		if filepath.Ext(path) != ".go" {
			return nil
		}

		// Skip test files (FR-014).
		if goChunker.IsTestFile(info.Name()) {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Warn("failed to read file", "path", path, "err", readErr)
			return nil
		}

		blocks, chunkErr := goChunker.ChunkFile(info.Name(), content)
		if chunkErr != nil {
			// Skip files with syntax errors — log warning, continue.
			logger.Warn("skipping file with syntax error", "path", path, "err", chunkErr)
			return nil
		}

		allBlocks = append(allBlocks, blocks...)
		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("walk vault: %w", walkErr)
	}

	return allBlocks, nil
}

// formatManifest generates the manifest markdown content from extracted blocks.
// Sections with no entries are omitted per contract invariant #5.
func formatManifest(blocks []chunker.Block) string {
	var commands, tools, packages []chunker.Block

	for _, b := range blocks {
		switch b.Kind {
		case "command":
			commands = append(commands, b)
		case "tool":
			tools = append(tools, b)
		case "package":
			packages = append(packages, b)
		}
	}

	var sb strings.Builder
	sb.WriteString("# Project Manifest\n\n")
	_, _ = fmt.Fprintf(&sb, "> Auto-generated by `dewey manifest` on %s\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	sb.WriteString("> Do not edit manually — regenerate with `dewey manifest`\n")

	if len(commands) == 0 && len(tools) == 0 && len(packages) == 0 {
		sb.WriteString("\nNo Go source files found or no declarations extracted.\n")
		return sb.String()
	}

	if len(commands) > 0 {
		sb.WriteString("\n## CLI Commands\n\n")
		sb.WriteString("| Command | Description |\n")
		sb.WriteString("|---------|-------------|\n")
		for _, c := range commands {
			// Extract command name and description from the block content.
			name, desc := parseCommandBlock(c)
			_, _ = fmt.Fprintf(&sb, "| `%s` | %s |\n", name, desc)
		}
	}

	if len(tools) > 0 {
		sb.WriteString("\n## MCP Tools\n\n")
		sb.WriteString("| Tool | Description |\n")
		sb.WriteString("|------|-------------|\n")
		for _, t := range tools {
			name, desc := parseToolBlock(t)
			_, _ = fmt.Fprintf(&sb, "| `%s` | %s |\n", name, desc)
		}
	}

	if len(packages) > 0 {
		sb.WriteString("\n## Exported Packages\n\n")
		for _, p := range packages {
			// Heading is "package <name>", content includes doc comment.
			_, _ = fmt.Fprintf(&sb, "### %s\n\n", p.Heading)
			// Extract just the doc comment portion (before the package declaration line).
			doc := extractPackageDocText(p.Content)
			if doc != "" {
				sb.WriteString(doc + "\n\n")
			}
		}
	}

	return sb.String()
}

// parseCommandBlock extracts the command name and short description from
// a command block. The Heading format is "command: <Use>", and the Content
// contains "CLI Command: <Use>\nShort: <desc>\n...".
func parseCommandBlock(b chunker.Block) (name, desc string) {
	name = strings.TrimPrefix(b.Heading, "command: ")
	// Extract Short description from content lines.
	for _, line := range strings.Split(b.Content, "\n") {
		if strings.HasPrefix(line, "Short: ") {
			desc = strings.TrimPrefix(line, "Short: ")
			return name, desc
		}
	}
	return name, ""
}

// parseToolBlock extracts the tool name and description from a tool block.
// The Heading format is "tool: <Name>", and the Content contains
// "MCP Tool: <Name>\nDescription: <desc>".
func parseToolBlock(b chunker.Block) (name, desc string) {
	name = strings.TrimPrefix(b.Heading, "tool: ")
	for _, line := range strings.Split(b.Content, "\n") {
		if strings.HasPrefix(line, "Description: ") {
			desc = strings.TrimPrefix(line, "Description: ")
			return name, desc
		}
	}
	return name, ""
}

// extractPackageDocText extracts the doc comment text from a package block's
// Content field. The content format is "// doc line\n// ...\npackage <name>".
// Returns the doc comment with // prefixes stripped, or empty string if none.
func extractPackageDocText(content string) string {
	lines := strings.Split(content, "\n")
	var docLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "//") {
			// Strip the "// " prefix to get clean doc text.
			text := strings.TrimPrefix(line, "// ")
			text = strings.TrimPrefix(text, "//") // bare "//" lines
			docLines = append(docLines, text)
		}
	}
	return strings.TrimSpace(strings.Join(docLines, "\n"))
}

// countManifestItems returns the count of commands, tools, and packages
// in the block list.
func countManifestItems(blocks []chunker.Block) (commands, tools, packages int) {
	for _, b := range blocks {
		switch b.Kind {
		case "command":
			commands++
		case "tool":
			tools++
		case "package":
			packages++
		}
	}
	return
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

// --- Compile command (T034, 013-knowledge-compile Phase 7) ---

// newCompileCmd creates the `dewey compile` subcommand.
// Synthesizes stored learnings into compiled knowledge articles.
// Uses OllamaSynthesizer from config when available; otherwise returns
// clusters with synthesis prompts for manual execution.
func newCompileCmd() *cobra.Command {
	var incremental []string
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile learnings into knowledge articles",
		Long: `Synthesize stored learnings into compiled knowledge articles.

Groups learnings by topic tag and produces current-state articles
with per-claim citations and history. Supersession is explicit, not
recency-driven: synthesis presents unresolved contradictions with both
positions, proposes supersessions for human confirmation, and reports
per-article citation coverage.

When a compile_model is configured in .uf/dewey/config.yaml, uses
Ollama for LLM synthesis. Otherwise, outputs clusters with synthesis
prompts for manual execution.

Examples:
  dewey compile                          # full rebuild
  dewey compile -i auth-3 -i auth-4      # incremental`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			// Open store.
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			// Create embedder (optional — used for semantic refinement).
			embedder, err := createIndexEmbedder(false, deweyDir)
			if err != nil {
				// Non-fatal for compile: proceed without embedder.
				logger.Warn("embedder unavailable, compiling without semantic refinement", "err", err)
			}

			// Create synthesizer from config.
			var synth llm.Synthesizer
			synthCfg := llm.ReadSynthesisConfig(deweyDir)
			if synthCfg.Model != "" {
				s2, synthErr := llm.NewSynthesizerFromConfig(synthCfg)
				if synthErr != nil {
					logger.Warn("synthesis provider error, returning prompts only", "err", synthErr)
				} else if s2.Available() {
					synth = s2
					logger.Info("synthesis model available", "provider", synthCfg.Provider, "model", synthCfg.Model)
				} else {
					logger.Warn("synthesis model not available, returning prompts only",
						"provider", synthCfg.Provider, "model", synthCfg.Model)
				}
			}

			// Create compile tool and run.
			compileEmbCfg := embed.ReadEmbeddingConfig(deweyDir)
			compile := tools.NewCompile(s, embedder, synth, vp, nil, compileEmbCfg.MaxChunkChars)
			input := types.CompileInput{Incremental: incremental}

			start := time.Now()
			result, _, mcpErr := compile.Compile(context.Background(), nil, input)
			if mcpErr != nil {
				return fmt.Errorf("compile failed: %w", mcpErr)
			}

			elapsed := time.Since(start)

			// Print result.
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						return fmt.Errorf("compile: %s", tc.Text)
					}
				}
				return fmt.Errorf("compile failed")
			}

			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					fmt.Println(tc.Text)
				}
			}

			logger.Info("compile complete", "elapsed", elapsed.Round(time.Millisecond))
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&incremental, "incremental", "i", nil,
		"Learning identities to compile incrementally (repeatable)")
	cmd.Flags().StringVar(&vaultPath, "vault", "",
		"Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// --- Curate command (T023, 015-curated-knowledge-stores Phase 3) ---

// newCurateCmd creates the `dewey curate` subcommand.
// Runs the curation pipeline to extract structured knowledge from indexed
// sources into knowledge store directories. Uses OllamaSynthesizer from
// config when available; otherwise returns extraction prompts.
func newCurateCmd() *cobra.Command {
	var storeName string
	var force bool
	var noEmbeddings bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "curate",
		Short: "Extract knowledge from indexed sources",
		Long: `Run the curation pipeline to extract structured knowledge from indexed
sources into knowledge store directories.

Reads knowledge-stores.yaml for store definitions, queries the index for
source content, uses an LLM to extract decisions/facts/patterns, and writes
curated markdown files to the store's output directory.

Examples:
  dewey curate                           # curate all stores
  dewey curate --store team-decisions    # curate one store
  dewey curate --force                   # re-curate all content`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			// Load knowledge stores config.
			ksPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
			stores, err := curate.LoadKnowledgeStoresConfig(ksPath)
			if err != nil {
				return fmt.Errorf("load knowledge stores config: %w", err)
			}
			if len(stores) == 0 {
				return fmt.Errorf("no knowledge stores configured — create .uf/dewey/knowledge-stores.yaml or run 'dewey init'")
			}

			// Filter to named store if specified.
			if storeName != "" {
				var found *curate.StoreConfig
				for i := range stores {
					if stores[i].Name == storeName {
						found = &stores[i]
						break
					}
				}
				if found == nil {
					return fmt.Errorf("knowledge store %q not found in configuration", storeName)
				}
				stores = []curate.StoreConfig{*found}
			}

			// Open store.
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			// Create embedder (optional).
			embedder, err := createIndexEmbedder(noEmbeddings, deweyDir)
			if err != nil {
				logger.Warn("embedder unavailable, curating without embeddings", "err", err)
			}

			// Create synthesizer from config.
			var synth llm.Synthesizer
			synthCfg := llm.ReadSynthesisConfig(deweyDir)
			if synthCfg.Model == "" {
				return fmt.Errorf("LLM unavailable. Configure synthesis provider in .uf/dewey/config.yaml.\nTo fix: Add 'synthesis:\\n  provider: ollama\\n  model: llama3.2:3b' or 'compile_model: llama3.2:3b'")
			}
			s2, synthErr := llm.NewSynthesizerFromConfig(synthCfg)
			if synthErr != nil {
				return fmt.Errorf("synthesis provider error: %w", synthErr)
			}
			if !s2.Available() {
				return fmt.Errorf("synthesis model %q not available (provider: %s)", synthCfg.Model, synthCfg.Provider)
			}
			synth = s2
			logger.Info("LLM model available for curation", "provider", synthCfg.Provider, "model", synthCfg.Model)

			// Create pipeline and run curation.
			pipeline := curate.NewPipeline(s, synth, embedder, vp)

			totalFiles := 0
			for _, cfg := range stores {
				fmt.Printf("  Curating store %q (%d sources)...\n", cfg.Name, len(cfg.Sources))

				var filesCreated int
				var curateErr error
				if force {
					filesCreated, curateErr = pipeline.CurateStore(context.Background(), cfg)
				} else {
					filesCreated, curateErr = pipeline.CurateStoreIncremental(context.Background(), cfg)
				}

				if curateErr != nil {
					fmt.Printf("  ❌ %s: %v\n", cfg.Name, curateErr)
					continue
				}

				fmt.Printf("  ✅ %s: %d knowledge files created\n", cfg.Name, filesCreated)
				totalFiles += filesCreated
			}

			fmt.Printf("\n  Curation complete: %d files created across %d stores\n", totalFiles, len(stores))
			return nil
		},
	}

	cmd.Flags().StringVarP(&storeName, "store", "s", "", "Curate only the named store (default: all stores)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Re-curate all content, ignoring checkpoints")
	cmd.Flags().BoolVar(&noEmbeddings, "no-embeddings", false, "Skip embedding generation for curated files")
	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// --- Lint command (T035, 013-knowledge-compile Phase 7) ---

// newLintCmd creates the `dewey lint` subcommand.
// Scans the knowledge base for quality issues and optionally auto-fixes them.
func newLintCmd() *cobra.Command {
	var fix bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Check knowledge base quality",
		Long: `Scan the knowledge base for quality issues:
  - Stale decisions (>30 days without review)
  - Uncompiled learnings (not in any compiled article)
  - Embedding gaps (pages with blocks but no embeddings)
  - Potential contradictions (similar learnings with same tag)
  - Citation coverage (compiled articles citing undeclared learnings
    or leaving current sources uncited)

Use --fix to auto-repair mechanical issues (e.g., regenerate
missing embeddings). Semantic issues require human intervention.

Exit code 0 if clean, 1 if issues found.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			// Open store.
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			// Create embedder (optional — used for fix mode and contradiction check).
			var embedder embed.Embedder
			if fix {
				embedder, err = createIndexEmbedder(false, deweyDir)
				if err != nil {
					logger.Warn("embedder unavailable, fix mode limited", "err", err)
				}
			}

			// Create lint tool and run.
			lintEmbCfg := embed.ReadEmbeddingConfig(deweyDir)
			lint := tools.NewLint(s, embedder, vp, lintEmbCfg.MaxChunkChars)
			input := types.LintInput{Fix: fix}

			result, _, mcpErr := lint.Lint(context.Background(), nil, input)
			if mcpErr != nil {
				return fmt.Errorf("lint failed: %w", mcpErr)
			}

			// Print result.
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						return fmt.Errorf("lint: %s", tc.Text)
					}
				}
				return fmt.Errorf("lint failed")
			}

			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					fmt.Println(tc.Text)
				}
			}

			// Parse result to determine exit code.
			// If any findings exist, exit with code 1.
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					var parsed map[string]any
					if err := json.Unmarshal([]byte(tc.Text), &parsed); err == nil {
						if status, ok := parsed["status"].(string); ok && status != "clean" {
							os.Exit(1)
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "Auto-fix mechanical issues (e.g., regenerate missing embeddings)")
	cmd.Flags().StringVar(&vaultPath, "vault", "",
		"Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// --- Promote command (T036, 013-knowledge-compile Phase 7) ---

// newPromoteCmd creates the `dewey promote` subcommand. Promotion is a
// frontmatter file edit on the current branch (git-is-the-source-of-record,
// PW-FR-001): the command writes tier: validated into the page file so a
// pull request can carry the promotion for human review. The database tier
// is a cache recomputed on the next index — promote never writes it.
func newPromoteCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "promote PAGE_NAME...",
		Short: "Promote draft pages to validated (frontmatter file edit)",
		Long: `Promote draft compiled articles or curated knowledge pages by writing
tier: validated into the page file's frontmatter on the current branch.
The file diff — carried by a pull request — is the promotion artifact for
human review; promote never writes the database tier, which stays a cache
recomputed on the next index.

A promotion takes effect for every server only after the pull request
merges to the default branch and the server re-indexes: a branch-only
promotion keeps indexing as draft (the merge is the vouch). Only pages
with tier=draft can be promoted, and only files under compiled/ or
knowledge/ are promotable — learnings stay draft forever and are never
promotable.

Page names resolve through a ladder: exact name, then case-insensitive
name, then bare tags against the compiled/, learning/, and knowledge/
namespaces, then source doc IDs. Ambiguous matches are refused. Multiple
names are promoted as an all-or-nothing batch: if any page cannot be
promoted, no file is changed.

Examples:
  dewey promote compiled/authentication
  dewey promote authentication
  dewey promote compiled/authentication knowledge/team/notes`,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			vp, err := resolveVaultPathOrCwd(vaultPath)
			if err != nil {
				return err
			}

			deweyDir := filepath.Join(vp, deweyWorkspaceDir)
			if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
				return fmt.Errorf("not initialized. Run 'dewey init' first")
			}

			// Open store.
			dbPath := filepath.Join(deweyDir, "graph.db")
			s, err := store.New(dbPath)
			if err != nil {
				return promoteStoreOpenError(deweyDir, err)
			}
			defer func() { _ = s.Close() }()

			// Create promote tool and run. A single argument preserves the
			// single-page result shape; multiple arguments use batch mode.
			// The vault root anchors the path validation every edit target
			// must pass (PW-FR-004).
			promote := tools.NewPromote(s, vp)
			input := types.PromoteInput{Page: args[0]}
			if len(args) > 1 {
				input = types.PromoteInput{Pages: args}
			}

			result, _, mcpErr := promote.Promote(context.Background(), nil, input)
			if mcpErr != nil {
				return fmt.Errorf("promote failed: %w", mcpErr)
			}

			// Print result.
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						return fmt.Errorf("promote: %s", tc.Text)
					}
				}
				return fmt.Errorf("promote failed")
			}

			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					fmt.Println(tc.Text)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&vaultPath, "vault", "",
		"Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// promoteStoreOpenError wraps a store.New failure from the promote command
// with lock-holder diagnostics (mcp-internal-store-access task 4.1, design
// D7). store.New creates and holds dewey.lock next to graph.db, so on
// failure that lock file is probed best-effort via detectLockHolder: when
// another Dewey process holds it, the returned error wraps the underlying
// failure, identifies the recorded holder, and points to the dewey_promote
// MCP tool as the supported write path while a server is running. When no
// holder can be identified (lock file absent, unheld, or unreadable), it
// degrades gracefully to the plain wrapped error.
func promoteStoreOpenError(deweyDir string, err error) error {
	holder := detectLockHolder(filepath.Join(deweyDir, "dewey.lock"))
	if holder == "" {
		// No holder identified (absent, unheld, or unreadable lock file):
		// degrade to the plain error rather than guessing at a cause.
		return fmt.Errorf("open store: %w", err)
	}
	holderDesc := fmt.Sprintf("another Dewey process (%s) appears to be holding the database lock", holder)
	if !strings.HasPrefix(holder, "PID ") {
		// Old-format lock with no recorded PID: detectLockHolder's fallback
		// ("lock held (unknown process)") is already a standalone clause, so
		// use it verbatim rather than nesting it inside parentheses.
		holderDesc = holder
	}
	return fmt.Errorf("open store: %w; %s. While the dewey server is running, use the dewey_promote MCP tool to promote pages, or stop the server first", err, holderDesc)
}

// newArtifactsCmd creates the `dewey artifacts` command group.
func newArtifactsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifacts",
		Short: "Publish knowledge artifacts to git",
		Long: `Plan and publish stranded knowledge artifacts (learnings, compiled
articles, and knowledge-store output) to git.

The flow is two-step with a mandatory human gate between the steps:

  1. dewey artifacts plan — scan the working tree, stashes, and unpushed
     commits for stranded artifacts and write a plan file.
  2. dewey artifacts sync — execute an approved plan on a chore branch.

Nothing is published until a human approves the plan with --approve.`,
	}

	cmd.AddCommand(newArtifactsPlanCmd())
	cmd.AddCommand(newArtifactsSyncCmd())

	return cmd
}

// resolveArtifactsContext resolves the vault root, git repository root, and
// configured knowledge-store paths for the artifacts commands. The vault
// resolves from the --vault flag, then OBSIDIAN_VAULT_PATH, then the current
// directory. Knowledge-store paths are resolved relative to the repository
// root; a missing knowledge-stores.yaml yields no extra stores (not an error).
func resolveArtifactsContext(vaultPath string) (repoRoot, vaultRoot string, storePaths []string, err error) {
	vaultAbs, err := resolveVaultPathOrCwd(vaultPath)
	if err != nil {
		return "", "", nil, err
	}
	// git resolves symlinks in its output; resolve them here too so store
	// paths computed relative to the repository root stay inside it when the
	// vault path traverses a symlink (common on macOS temporary directories).
	if resolved, resolveErr := filepath.EvalSymlinks(vaultAbs); resolveErr == nil {
		vaultAbs = resolved
	}

	repo, err := artifacts.ResolveRepoRoot(vaultAbs)
	if err != nil {
		return "", "", nil, err
	}

	storesPath := filepath.Join(vaultAbs, deweyWorkspaceDir, "knowledge-stores.yaml")
	if _, statErr := os.Stat(storesPath); statErr == nil {
		cfgs, cfgErr := curate.LoadKnowledgeStoresConfig(storesPath)
		if cfgErr != nil {
			return "", "", nil, fmt.Errorf("load knowledge stores config: %w", cfgErr)
		}
		for _, cfg := range cfgs {
			absStore := curate.ResolveStorePath(cfg, vaultAbs)
			rel, relErr := filepath.Rel(repo, absStore)
			if relErr != nil {
				return "", "", nil, fmt.Errorf("relativize store path %s: %w", absStore, relErr)
			}
			storePaths = append(storePaths, rel)
		}
	}

	return repo, vaultAbs, storePaths, nil
}

// newArtifactsPlanCmd creates the `dewey artifacts plan` subcommand.
func newArtifactsPlanCmd() *cobra.Command {
	var planJSON bool
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan a knowledge-artifact publish chore",
		Long: `Scans the working tree, stash entries, and unpushed commits for stranded
knowledge artifacts, screens every candidate, and assembles a publish plan.

The canonical plan JSON is written to a private (0600) temp file; the rendered
summary is relayed verbatim. Pass --json to emit the full plan JSON to stdout
(the plan file path is logged to stderr either way).`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, vaultRoot, storePaths, err := resolveArtifactsContext(vaultPath)
			if err != nil {
				return err
			}

			plan, err := artifacts.BuildPlan(repoRoot, vaultRoot, storePaths)
			if err != nil {
				return fmt.Errorf("build plan: %w", err)
			}

			payload, err := json.Marshal(plan)
			if err != nil {
				return fmt.Errorf("marshal plan: %w", err)
			}

			tmp, err := os.CreateTemp("", "dewey-plan-*.json")
			if err != nil {
				return fmt.Errorf("create plan file: %w", err)
			}
			if err := tmp.Chmod(0o600); err != nil {
				_ = tmp.Close()
				return fmt.Errorf("restrict plan file permissions: %w", err)
			}
			if _, err := tmp.Write(payload); err != nil {
				_ = tmp.Close()
				return fmt.Errorf("write plan file: %w", err)
			}
			if err := tmp.Close(); err != nil {
				return fmt.Errorf("close plan file: %w", err)
			}

			if planJSON {
				logger.Info("plan written", "path", tmp.Name())
				fmt.Println(string(payload))
			} else {
				fmt.Printf("plan written to %s\n\n", tmp.Name())
				fmt.Println(plan.RenderedSummary)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&planJSON, "json", false, "Emit the full plan JSON to stdout")
	cmd.Flags().StringVar(&vaultPath, "vault", "",
		"Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// newArtifactsSyncCmd creates the `dewey artifacts sync` subcommand.
func newArtifactsSyncCmd() *cobra.Command {
	var planPath string
	var approve bool
	var approvedBy string
	var resolutionsPath string
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "sync --plan <file>",
		Short: "Execute an approved knowledge-artifact publish",
		Long: `Executes a plan produced by dewey artifacts plan: materializes the planned
artifacts on a dedicated chore branch inside an OS-temp worktree, commits them
with a rendered commit message and PR body, and prints the sync result JSON
(chore branch, commit, worktree, PR body) verbatim.

Nothing is published without --approve; --approved-by records the human
approver in the commit trailer (defaults to git config user.name). If the plan
reported collisions, pass the reviewed resolutions file via --resolutions.`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, vaultRoot, storePaths, err := resolveArtifactsContext(vaultPath)
			if err != nil {
				return err
			}

			result, err := artifacts.RunSync(repoRoot, vaultRoot, storePaths, planPath, resolutionsPath, approve, approvedBy)
			if err != nil {
				return fmt.Errorf("sync: %w", err)
			}

			out, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("marshal sync result: %w", err)
			}
			fmt.Println(string(out))
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "",
		"Path to the plan file produced by dewey artifacts plan (required)")
	if err := cmd.MarkFlagRequired("plan"); err != nil {
		panic(fmt.Sprintf("mark plan flag required: %v", err))
	}
	cmd.Flags().BoolVar(&approve, "approve", false,
		"Approve the publish (required — the human gate between plan and sync)")
	cmd.Flags().StringVar(&approvedBy, "approved-by", "",
		"Approver identity recorded in the commit trailer (default: git config user.name)")
	cmd.Flags().StringVar(&resolutionsPath, "resolutions", "",
		"Path to a path-keyed JSON file of collision resolutions")
	cmd.Flags().StringVar(&vaultPath, "vault", "",
		"Path to vault (default: OBSIDIAN_VAULT_PATH or current directory)")

	return cmd
}

// displayGatewayValue renders an empty gateway configuration value as
// "not configured" for diagnostic output.
func displayGatewayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not configured"
	}
	return value
}

// embedderUnavailableReason returns the provider's credential-free reason
// when it implements embed.UnavailableReasoner, or a generic fallback.
func embedderUnavailableReason(e embed.Embedder) string {
	if reason := embed.UnavailableReasonOf(e); reason != "" {
		return reason
	}
	return "unavailable"
}

// gatewayCredentialWarnings returns the FR-024 credential-posture
// warnings for an openai-compatible provider. It never includes key
// material: only booleans derived from the resolved key.
func gatewayCredentialWarnings(capability openaicompat.Capability, endpoint string, apiKeyInConfig bool) []string {
	var warnings []string
	if openaicompat.InsecureEndpointForCredential(endpoint, openaicompat.ResolveAPIKey(capability, endpoint)) {
		warnings = append(warnings, "API key would transit http:// to a non-loopback host; use https")
	}
	if apiKeyInConfig {
		warnings = append(warnings, fmt.Sprintf("%s.api_key in config.yaml is ignored; keys are environment-only", capability))
	}
	return warnings
}

// queryTrustStatus resolves the default-branch comparison ref and surveys
// the vault's compiled/ and knowledge/ markdown files against it — the same
// derivation inputs ingestion uses (one bounded ref resolution, one batch
// ls-tree, per-file blob hashing). Read-only: no writes, no fetches, no
// state changes; an unresolvable ref yields the GTS-FR-004 fallback shape
// (every trust file counts as differing, validated withheld).
func queryTrustStatus(vp string) *trustStatus {
	ctx := context.Background()
	res := gitref.New(gitref.Options{VaultDir: vp}).Resolve(ctx)
	ts := &trustStatus{
		Ref:    res.Ref,
		Status: string(res.Status),
		SHA:    res.SHA,
		Leg:    string(res.Leg),
		Reason: res.Reason,
	}
	var blobs map[string]string
	if res.Resolved() {
		blobs = gitref.MapDefaultBranchBlobs(ctx, gitref.ExecRunner{}, vp, res, gitref.TrustPathspecs())
	}
	for _, namespace := range gitref.TrustPathspecs() {
		root := filepath.Join(vp, namespace)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil // unreadable or non-page entries are not status failures
			}
			rel, relErr := filepath.Rel(vp, p)
			if relErr != nil {
				return nil
			}
			content, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil
			}
			ts.TrustFiles++
			fileBlob, _ := gitref.FileBlobSHA(p)
			if refBlob := blobs[filepath.ToSlash(rel)]; refBlob != "" && fileBlob == refBlob {
				if frontmatterTierOf(content) == "validated" {
					ts.ValidatedEligible++
				}
			} else {
				ts.Differing++
				if len(ts.DifferPaths) < 10 {
					ts.DifferPaths = append(ts.DifferPaths, filepath.ToSlash(rel))
				}
			}
			return nil
		})
	}
	return ts
}

// printTrustTierChecks reports the trust-tier derivation state in the
// doctor layout: the default-branch comparison ref (with the GTS-FR-004
// fallback note and a fix hint when it cannot resolve) and the
// compiled/knowledge blob comparison (effective-tier observability —
// claimed file tier versus default-branch blob).
func printTrustTierChecks(c *doctorCounter, w io.Writer, dp func(string, ...any), vaultPath string) {
	ts := queryTrustStatus(vaultPath)

	if ts.Status == string(gitref.StatusResolved) {
		c.printCheck(w, "PASS", "default-branch ref",
			fmt.Sprintf("%s @ %s (via %s)", ts.Ref, shortRefSHA(ts.SHA), ts.Leg))
	} else {
		c.printCheck(w, "WARN", "default-branch ref",
			fmt.Sprintf("%s: %s", ts.Status, ts.Reason))
		dp("     Fix: %s\n", trustRefFix(ts.Status))
	}

	switch {
	case ts.TrustFiles == 0:
		c.printCheck(w, "PASS", "tier derivation", "no compiled/ or knowledge/ pages in the vault")
	case ts.Status == string(gitref.StatusResolved) && ts.Differing == 0:
		c.printCheck(w, "PASS", "tier derivation",
			fmt.Sprintf("%d file(s) match %s — %d validated-eligible", ts.TrustFiles, ts.Ref, ts.ValidatedEligible))
	case ts.Status == string(gitref.StatusResolved):
		c.printCheck(w, "INFO", "tier derivation",
			fmt.Sprintf("%d of %d file(s) differ from %s — they index as draft until merged and fetched (%d validated-eligible)",
				ts.Differing, ts.TrustFiles, ts.Ref, ts.ValidatedEligible))
	default:
		c.printCheck(w, "WARN", "tier derivation",
			fmt.Sprintf("%d file(s) degrade to draft — validated is withheld until the ref resolves", ts.TrustFiles))
	}

	// Verbose mode: name the differing files so a draft page is
	// explainable (working-tree edit vs unmerged branch vs missing file).
	if logger.GetLevel() == log.DebugLevel {
		for _, rel := range ts.DifferPaths {
			dp("     • %s (differs from %s — indexes as draft)\n", rel, ts.Ref)
		}
	}
}

// trustRefFix returns the actionable remediation for an unresolved
// default-branch ref by failure class.
func trustRefFix(status string) string {
	switch status {
	case string(gitref.StatusMissingRef):
		return "git fetch the remote (the comparison uses the locally fetched ref), then dewey index"
	case string(gitref.StatusNotARepo):
		return "initialize a git repository in the vault — trust tiers derive from git"
	case string(gitref.StatusInvalidConfig):
		return "correct default_branch_ref in .uf/dewey/config.yaml (or the global config / DEWEY_DEFAULT_BRANCH_REF)"
	default:
		return "check git availability and repository health (dewey -v doctor records the reason)"
	}
}

// shortRefSHA renders the first 7 hex characters of a commit SHA, or the
// value unchanged when shorter.
func shortRefSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// frontmatterTierOf extracts the top-level `tier:` value from leading YAML
// frontmatter for diagnostics. Line-oriented, matching the tier-key shape
// the promote edit and ingestion parse: a column-zero `tier:` inside the
// opening `---` block. Display-only — the derivation gate remains
// vault.DeriveEffectiveTier at ingest.
func frontmatterTierOf(content []byte) string {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "---" {
			return ""
		}
		if v, ok := strings.CutPrefix(line, "tier:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// printVertexEmbeddingChecks renders the Embedding Layer section for the
// Vertex provider (issue #143): configuration completeness (model,
// project, region) checked through the provider factory, which makes no
// network calls — mirroring the Synthesis Layer's Vertex branch. Vertex
// never gets Ollama reachability checks or `ollama pull` hints.
func printVertexEmbeddingChecks(c *doctorCounter, w io.Writer, section func(string), cfg embed.ProviderConfig) {
	section(fmt.Sprintf("Embedding Layer (%s via vertex)", cfg.Model))
	if _, err := embed.NewEmbedderFromConfig(cfg); err != nil {
		c.printCheck(w, "FAIL", "config", fmt.Sprintf("incomplete: %s", err))
		return
	}
	c.printCheck(w, "PASS", "config", fmt.Sprintf("project=%s region=%s model=%s", cfg.Project, cfg.Region, cfg.Model))
}

// printGatewayCredentialChecks renders the credential warnings in dewey
// doctor output.
func printGatewayCredentialChecks(c *doctorCounter, w io.Writer, capability openaicompat.Capability, endpoint string, apiKeyInConfig bool) {
	for _, warning := range gatewayCredentialWarnings(capability, endpoint, apiKeyInConfig) {
		c.printCheck(w, "WARN", "credentials", warning)
	}
}

// logGatewayCredentialWarnings logs the credential warnings at startup
// and index time.
func logGatewayCredentialWarnings(capability openaicompat.Capability, endpoint string, apiKeyInConfig bool) {
	for _, warning := range gatewayCredentialWarnings(capability, endpoint, apiKeyInConfig) {
		logger.Warn("gateway credential warning", "capability", capability, "warning", warning)
	}
}

// embeddingDegradationFields builds the structured log fields for a
// degraded embedding provider. The ollama pull hint appears only for
// Ollama, and the provider's reason is included when it implements
// embed.UnavailableReasoner (FR-015, FR-024).
func embeddingDegradationFields(cfg embed.ProviderConfig, embedder embed.Embedder) []any {
	fields := []any{"provider", cfg.Provider, "model", cfg.Model, "endpoint", openaicompat.DisplayURL(cfg.Endpoint)}
	if cfg.Provider == "ollama" || cfg.Provider == "" {
		fields = append(fields, "pull", fmt.Sprintf("ollama pull %s", cfg.Model))
	}
	if reason := embed.UnavailableReasonOf(embedder); reason != "" {
		fields = append(fields, "reason", reason)
	}
	return append(fields, "note", "semantic search is unavailable")
}
