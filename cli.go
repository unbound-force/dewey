package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unbound-force/dewey/client"
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
	switch {
	case day == 1 || day == 21 || day == 31:
		suffix = "st"
	case day == 2 || day == 22:
		suffix = "nd"
	case day == 3 || day == 23:
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
