// Package curate provides knowledge store configuration parsing and the
// curation pipeline for extracting structured knowledge from indexed sources.
// It is a leaf dependency — it imports store, llm, embed, and stdlib only.
package curate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/unbound-force/dewey/source"
	"gopkg.in/yaml.v3"
)

// curateLogger is the package-level structured logger for curate operations.
var curateLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/curate",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// StoreConfig represents a single knowledge store entry from knowledge-stores.yaml.
//
// Invariants:
//  1. Name is non-empty and unique across stores
//  2. Sources is non-empty (stores with empty sources are skipped with a warning)
//  3. Path defaults to .uf/dewey/knowledge/{Name} when empty
//  4. CurationInterval defaults to "10m" when empty
//  5. CurateOnIndex defaults to false
type StoreConfig struct {
	Name             string   `yaml:"name"`
	Sources          []string `yaml:"sources"`
	Path             string   `yaml:"path"`
	CurateOnIndex    bool     `yaml:"curate_on_index"`
	CurationInterval string   `yaml:"curation_interval"`
}

// knowledgeStoresFile is the top-level structure of knowledge-stores.yaml.
type knowledgeStoresFile struct {
	Stores []StoreConfig `yaml:"stores"`
}

// LoadKnowledgeStoresConfig reads and parses knowledge-stores.yaml at the given path.
// Returns (nil, nil) if the file does not exist — this is not an error condition
// since knowledge stores are optional.
// Returns an error if the file is malformed or validation fails.
//
// Design decision: Returns (nil, nil) for missing file rather than an error
// because knowledge stores are an optional feature. This follows the same
// pattern as source.LoadSourcesConfig (Consistency Principle).
func LoadKnowledgeStoresConfig(path string) ([]StoreConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read knowledge stores config: %w", err)
	}

	var file knowledgeStoresFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse knowledge stores config: %w", err)
	}

	// Apply defaults and validate.
	seen := make(map[string]bool)
	for i := range file.Stores {
		cfg := &file.Stores[i]

		// Validate name is non-empty.
		if cfg.Name == "" {
			return nil, fmt.Errorf("store %d: name is required", i)
		}

		// Validate name is unique.
		if seen[cfg.Name] {
			return nil, fmt.Errorf("store %d: duplicate store name %q", i, cfg.Name)
		}
		seen[cfg.Name] = true

		// Apply default curation interval.
		if cfg.CurationInterval == "" {
			cfg.CurationInterval = "10m"
		}

		// Validate curation interval parses correctly.
		if _, err := ParseCurationInterval(cfg.CurationInterval); err != nil {
			return nil, fmt.Errorf("store %q: %w", cfg.Name, err)
		}

		// Stores with empty sources are valid but will be skipped during curation.
		// Log a warning but don't fail — the user may be configuring incrementally.
		if len(cfg.Sources) == 0 {
			curateLogger.Warn("store has no sources, will be skipped during curation",
				"store", cfg.Name)
		}
	}

	return file.Stores, nil
}

// ResolveStorePath returns the absolute path for a store's output directory.
// If cfg.Path is empty, defaults to {vaultPath}/.uf/dewey/knowledge/{cfg.Name}.
// If cfg.Path is absolute, returns it as-is.
// If cfg.Path is relative, resolves against vaultPath.
func ResolveStorePath(cfg StoreConfig, vaultPath string) string {
	if cfg.Path == "" {
		return filepath.Join(vaultPath, ".uf", "dewey", "knowledge", cfg.Name)
	}
	if filepath.IsAbs(cfg.Path) {
		return cfg.Path
	}
	return filepath.Join(vaultPath, cfg.Path)
}

// ParseCurationInterval parses the curation interval string into a time.Duration.
// Returns 10*time.Minute for an empty string (default interval).
// Delegates to source.ParseRefreshInterval for parsing named intervals
// ("daily", "weekly", "hourly") and Go duration strings ("10m", "1h").
//
// Design decision: Delegates to source.ParseRefreshInterval rather than
// duplicating parsing logic (DRY principle). The curation interval uses
// the same format as source refresh intervals.
func ParseCurationInterval(interval string) (time.Duration, error) {
	if interval == "" {
		return 10 * time.Minute, nil
	}
	d, err := source.ParseRefreshInterval(interval)
	if err != nil {
		return 0, fmt.Errorf("invalid curation interval: %w", err)
	}
	// ParseRefreshInterval returns 0 for empty string, but we already
	// handled that case above. A zero duration from a non-empty string
	// is unexpected — treat as the default.
	if d == 0 {
		return 10 * time.Minute, nil
	}
	return d, nil
}

// ValidateConfig checks that source IDs referenced in store configs exist
// in the provided list of known source IDs. Returns a list of warning
// messages for unknown source IDs. Does not return an error — missing
// sources are warnings, not failures, because sources may be added later.
func ValidateConfig(stores []StoreConfig, sourceIDs []string) []string {
	known := make(map[string]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		known[id] = true
	}

	var warnings []string
	for _, cfg := range stores {
		for _, srcID := range cfg.Sources {
			srcID = strings.TrimSpace(srcID)
			if !known[srcID] {
				warnings = append(warnings,
					fmt.Sprintf("store %q references unknown source %q", cfg.Name, srcID))
			}
		}
	}
	return warnings
}
