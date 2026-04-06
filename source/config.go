// Package source provides pluggable content source implementations for Dewey.
package source

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SourceConfig represents a single source entry from .uf/dewey/sources.yaml.
type SourceConfig struct {
	ID              string         `yaml:"id"`
	Type            string         `yaml:"type"`
	Name            string         `yaml:"name"`
	Config          map[string]any `yaml:"config"`
	RefreshInterval string         `yaml:"refresh_interval,omitempty"`
}

// SourcesFile represents the top-level structure of .uf/dewey/sources.yaml.
type SourcesFile struct {
	Sources []SourceConfig `yaml:"sources"`
}

// LoadSourcesConfig reads and parses the sources configuration file at
// the given path (typically .uf/dewey/sources.yaml). Returns (nil, nil) if
// the file does not exist. Validates each source entry for required
// fields and type-specific configuration.
//
// Returns an error if the file cannot be read, the YAML is malformed,
// or any source entry fails validation.
func LoadSourcesConfig(path string) ([]SourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sources config: %w", err)
	}

	var file SourcesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse sources config: %w", err)
	}

	// Validate each source entry.
	for i, src := range file.Sources {
		if err := validateSourceConfig(&src); err != nil {
			return nil, fmt.Errorf("source %d (%s): %w", i, src.ID, err)
		}
	}

	return file.Sources, nil
}

// SaveSourcesConfig writes the sources configuration to the given path
// (typically .uf/dewey/sources.yaml) with a descriptive YAML header comment.
// Overwrites any existing file at the path.
//
// Returns an error if the configuration cannot be marshaled to YAML or
// the file cannot be written.
func SaveSourcesConfig(path string, sources []SourceConfig) error {
	file := SourcesFile{Sources: sources}
	data, err := yaml.Marshal(&file)
	if err != nil {
		return fmt.Errorf("marshal sources config: %w", err)
	}

	header := "# Dewey content sources\n# Each source provides documents for the knowledge graph index.\n\n"
	if err := os.WriteFile(path, []byte(header+string(data)), 0o644); err != nil {
		return fmt.Errorf("write sources config: %w", err)
	}
	return nil
}

// validateSourceConfig checks that a source config has all required fields.
func validateSourceConfig(src *SourceConfig) error {
	if src.ID == "" {
		return fmt.Errorf("missing required field: id")
	}
	if src.Type == "" {
		return fmt.Errorf("missing required field: type")
	}
	if src.Name == "" {
		return fmt.Errorf("missing required field: name")
	}

	switch src.Type {
	case "disk":
		// Disk sources require a path in config.
		if src.Config == nil {
			src.Config = map[string]any{"path": "."}
		}
	case "github":
		// GitHub sources require org and repos.
		if src.Config == nil {
			return fmt.Errorf("github source requires config with 'org' and 'repos'")
		}
		if _, ok := src.Config["org"]; !ok {
			return fmt.Errorf("github source requires 'org' in config")
		}
		if _, ok := src.Config["repos"]; !ok {
			return fmt.Errorf("github source requires 'repos' in config")
		}
	case "web":
		// Web sources require urls.
		if src.Config == nil {
			return fmt.Errorf("web source requires config with 'urls'")
		}
		if _, ok := src.Config["urls"]; !ok {
			return fmt.Errorf("web source requires 'urls' in config")
		}
	case "code":
		// Code sources require path and languages.
		if src.Config == nil {
			return fmt.Errorf("code source requires config with 'path' and 'languages'")
		}
		if p, ok := src.Config["path"].(string); !ok || p == "" {
			return fmt.Errorf("code source requires 'path' in config")
		}
		if langs := extractStringList(src.Config["languages"]); len(langs) == 0 {
			return fmt.Errorf("code source requires 'languages' in config")
		}
	default:
		return fmt.Errorf("unknown source type: %s", src.Type)
	}

	return nil
}

// ParseRefreshInterval converts a refresh interval string to a [time.Duration].
// Supports named intervals ("daily" = 24h, "weekly" = 168h, "hourly" = 1h)
// and Go duration strings (e.g., "1h", "30m"). Returns 0 for an empty
// string (no refresh interval). Returns an error if the string is not a
// recognized named interval and cannot be parsed as a Go duration.
func ParseRefreshInterval(interval string) (time.Duration, error) {
	if interval == "" {
		return 0, nil
	}

	switch strings.ToLower(interval) {
	case "daily":
		return 24 * time.Hour, nil
	case "weekly":
		return 7 * 24 * time.Hour, nil
	case "hourly":
		return time.Hour, nil
	default:
		d, err := time.ParseDuration(interval)
		if err != nil {
			return 0, fmt.Errorf("invalid refresh interval %q: %w", interval, err)
		}
		return d, nil
	}
}
