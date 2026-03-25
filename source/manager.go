package source

import (
	"fmt"
	"time"
)

// Manager orchestrates fetching across all configured content sources.
// It checks refresh intervals, handles source failures gracefully
// (log warning, continue with others per FR-020), and reports summaries.
type Manager struct {
	sources []Source
	configs []SourceConfig
}

// FetchSummary reports the results of a fetch operation.
type FetchSummary struct {
	SourceID   string
	SourceType string
	Documents  int
	Errors     int
	Skipped    bool
	Error      string
}

// FetchResult is the aggregate result of fetching all sources.
type FetchResult struct {
	Summaries []FetchSummary
	TotalDocs int
	TotalErrs int
	TotalSkip int
}

// NewManager creates a Manager from source configurations, instantiating
// the appropriate [Source] implementation for each config entry (disk,
// github, or web). Unknown source types are logged as warnings and skipped.
// The basePath is used as the default directory for disk sources, and
// cacheDir is used for web source caching.
//
// Returns a Manager ready for [Manager.FetchAll] calls.
func NewManager(configs []SourceConfig, basePath, cacheDir string) *Manager {
	var sources []Source

	for _, cfg := range configs {
		src := createSource(cfg, basePath, cacheDir)
		if src != nil {
			sources = append(sources, src)
		}
	}

	return &Manager{
		sources: sources,
		configs: configs,
	}
}

// createSource instantiates a Source from a SourceConfig.
// It dispatches to per-type factory functions based on cfg.Type.
func createSource(cfg SourceConfig, basePath, cacheDir string) Source {
	switch cfg.Type {
	case "disk":
		return createDiskSource(cfg, basePath)
	case "github":
		return createGitHubSource(cfg)
	case "web":
		return createWebSource(cfg, cacheDir)
	default:
		logger.Warn("unknown source type, skipping", "type", cfg.Type, "id", cfg.ID)
		return nil
	}
}

// createDiskSource creates a DiskSource from config, resolving the path
// relative to basePath when no explicit path is configured.
func createDiskSource(cfg SourceConfig, basePath string) Source {
	path := "."
	if p, ok := cfg.Config["path"].(string); ok {
		path = p
	}
	if path == "." {
		path = basePath
	}
	return NewDiskSource(cfg.ID, cfg.Name, path)
}

// createGitHubSource creates a GitHubSource from config, extracting the
// org, repos list, and content type filters from the config map.
func createGitHubSource(cfg SourceConfig) Source {
	org, _ := cfg.Config["org"].(string)
	repos := extractStringList(cfg.Config["repos"])
	contentTypes := extractStringList(cfg.Config["content"])
	return NewGitHubSource(cfg.ID, cfg.Name, org, repos, contentTypes)
}

// createWebSource creates a WebSource from config, extracting URLs,
// crawl depth, rate limit, and cache directory.
func createWebSource(cfg SourceConfig, cacheDir string) Source {
	urls := extractStringList(cfg.Config["urls"])
	depth := 1
	if d, ok := cfg.Config["depth"].(int); ok {
		depth = d
	}
	if d, ok := cfg.Config["depth"].(float64); ok {
		depth = int(d)
	}
	rateLimit := defaultRateLimit
	if rl, ok := cfg.Config["rate_limit"].(string); ok {
		if d, err := time.ParseDuration(rl); err == nil {
			rateLimit = d
		}
	}
	return NewWebSource(cfg.ID, cfg.Name, urls, depth, rateLimit, cacheDir)
}

// extractStringList converts a config value to a string slice.
// It handles both []any (from JSON/YAML unmarshaling) and plain string values.
func extractStringList(v any) []string {
	switch val := v.(type) {
	case []any:
		var result []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}

// FetchAll fetches content from all configured sources and returns the
// aggregate result along with a map of source ID → fetched documents.
// If sourceName is non-empty, only that source is fetched. If force is
// true, refresh intervals are ignored and all sources are fetched
// regardless of when they were last refreshed.
//
// Source failures are non-fatal — each failure is logged as a warning
// and the fetch continues with remaining sources (FR-020). The returned
// [FetchResult] contains per-source summaries including document counts,
// error counts, and skip counts.
func (m *Manager) FetchAll(sourceName string, force bool, lastFetchedTimes map[string]time.Time) (*FetchResult, map[string][]Document) {
	result := &FetchResult{}
	allDocs := make(map[string][]Document)

	for _, src := range m.sources {
		meta := src.Meta()

		// Filter by source name if specified.
		if sourceName != "" && meta.ID != sourceName {
			continue
		}

		// Check refresh interval (skip if within interval and not forced).
		if !force {
			lastFetched, ok := lastFetchedTimes[meta.ID]
			if ok && !lastFetched.IsZero() {
				cfg := m.findConfig(meta.ID)
				if cfg != nil && cfg.RefreshInterval != "" {
					interval, err := ParseRefreshInterval(cfg.RefreshInterval)
					if err == nil && interval > 0 {
						if time.Since(lastFetched) < interval {
							logger.Info("source within refresh interval, skipping",
								"source", meta.ID, "interval", cfg.RefreshInterval)
							result.Summaries = append(result.Summaries, FetchSummary{
								SourceID:   meta.ID,
								SourceType: meta.Type,
								Skipped:    true,
							})
							result.TotalSkip++
							continue
						}
					}
				}
			}
		}

		// Fetch documents from source.
		logger.Info("fetching source", "source", meta.ID, "type", meta.Type)

		docs, err := src.List()
		if err != nil {
			// Source failures are non-fatal (FR-020).
			logger.Warn("source fetch failed, continuing with others",
				"source", meta.ID, "err", err)
			result.Summaries = append(result.Summaries, FetchSummary{
				SourceID:   meta.ID,
				SourceType: meta.Type,
				Errors:     1,
				Error:      err.Error(),
			})
			result.TotalErrs++
			continue
		}

		allDocs[meta.ID] = docs
		result.Summaries = append(result.Summaries, FetchSummary{
			SourceID:   meta.ID,
			SourceType: meta.Type,
			Documents:  len(docs),
		})
		result.TotalDocs += len(docs)

		logger.Info("source fetched",
			"source", meta.ID, "documents", len(docs))
	}

	return result, allDocs
}

// Sources returns the list of instantiated [Source] implementations
// created from the configurations passed to [NewManager]. Returns nil
// if no sources were successfully created.
func (m *Manager) Sources() []Source {
	return m.sources
}

// findConfig returns the SourceConfig for a given source ID.
func (m *Manager) findConfig(id string) *SourceConfig {
	for i := range m.configs {
		if m.configs[i].ID == id {
			return &m.configs[i]
		}
	}
	return nil
}

// FormatSummary returns a human-readable, multi-line summary of the fetch
// result including per-source status (documents fetched, errors, skips)
// and aggregate totals.
func (r *FetchResult) FormatSummary() string {
	var sb fmt.Stringer = &summaryBuilder{result: r}
	return sb.String()
}

type summaryBuilder struct {
	result *FetchResult
}

func (sb *summaryBuilder) String() string {
	var b string
	for _, s := range sb.result.Summaries {
		switch {
		case s.Skipped:
			b += fmt.Sprintf("  %s: skipped (within refresh interval)\n", s.SourceID)
		case s.Error != "":
			b += fmt.Sprintf("  %s: error (%s)\n", s.SourceID, s.Error)
		default:
			b += fmt.Sprintf("  %s: %d documents\n", s.SourceID, s.Documents)
		}
	}
	b += fmt.Sprintf("Total: %d documents, %d errors, %d skipped\n",
		sb.result.TotalDocs, sb.result.TotalErrs, sb.result.TotalSkip)
	return b
}
