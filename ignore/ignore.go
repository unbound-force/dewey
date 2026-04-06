// Package ignore provides gitignore-compatible pattern matching for filesystem
// walkers. It evaluates whether a given path or name should be skipped during
// a directory walk, supporting .gitignore patterns, extra patterns from
// sources.yaml, and a hardcoded hidden-directory baseline.
//
// Thread safety: Matcher is immutable after construction and safe for
// concurrent use by multiple goroutines.
package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// logger is the package-level structured logger for ignore operations.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/ignore",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// SetLogLevel sets the logging level for the ignore package.
// Use log.DebugLevel for verbose output during diagnostics.
func SetLogLevel(level log.Level) {
	logger.SetLevel(level)
}

// SetLogOutput replaces the ignore package logger with one that writes to
// the given writer at the given level. Used to enable file logging.
func SetLogOutput(w io.Writer, level log.Level) {
	newLogger := log.NewWithOptions(w, log.Options{
		Prefix:          "dewey/ignore",
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	})
	*logger = *newLogger
}

// pattern represents a single parsed ignore rule from a .gitignore file
// or an extra pattern from sources.yaml configuration.
type pattern struct {
	value   string // the cleaned pattern string (prefix/suffix markers stripped)
	negated bool   // true if the original line started with '!' (un-ignore)
	dirOnly bool   // true if the original line ended with '/' (match directories only)
	isGlob  bool   // true if the pattern contains glob metacharacters (*, ?, [)
}

// Matcher evaluates whether filesystem entries should be skipped during
// directory walks. It is built once from the union of .gitignore patterns,
// sources.yaml ignore patterns, and a hardcoded hidden-directory baseline.
//
// The zero value is not useful; use NewMatcher to construct a Matcher.
type Matcher struct {
	patterns []pattern
}

// NewMatcher creates a Matcher from the union of .gitignore patterns
// (read from gitignorePath) and additional patterns (from sources.yaml).
//
// If gitignorePath does not exist or is unreadable, only extraPatterns
// and the hidden-directory baseline apply (no error returned).
//
// If .gitignore contains malformed glob patterns, those lines are skipped
// with a logged warning. Parsing does not abort.
//
// Parameters:
//   - gitignorePath: path to the .gitignore file itself (not a directory)
//   - extraPatterns: additional patterns from sources.yaml ignore field
//
// The returned Matcher is never nil.
func NewMatcher(gitignorePath string, extraPatterns []string) (*Matcher, error) {
	m := &Matcher{}

	// Read .gitignore if it exists. Absence is not an error — many
	// directories won't have one, and the Matcher still works with
	// only the hidden-directory baseline and extra patterns.
	data, err := os.ReadFile(gitignorePath)
	if err == nil {
		gitPatterns, parseErr := parsePatterns(string(data))
		if parseErr != nil {
			return nil, fmt.Errorf("parse gitignore %s: %w", gitignorePath, parseErr)
		}
		m.patterns = append(m.patterns, gitPatterns...)
	}
	// If the file doesn't exist or is unreadable, we proceed silently.
	// This matches the contract: "no error when gitignore is absent."

	// Append extra patterns from sources.yaml. These are parsed with
	// the same rules as .gitignore lines, providing a consistent syntax
	// for users (FR-005: union merge semantics).
	if len(extraPatterns) > 0 {
		extraLines := strings.Join(extraPatterns, "\n")
		extras, parseErr := parsePatterns(extraLines)
		if parseErr != nil {
			return nil, fmt.Errorf("parse extra patterns: %w", parseErr)
		}
		m.patterns = append(m.patterns, extras...)
	}

	return m, nil
}

// parsePatterns parses gitignore-formatted text into a slice of patterns.
// It handles blank lines, comments, negation prefixes, directory suffixes,
// and glob detection. Malformed glob patterns are logged and skipped.
func parsePatterns(content string) ([]pattern, error) {
	var patterns []pattern
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()

		// Skip blank lines.
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip comment lines (starting with '#').
		if strings.HasPrefix(line, "#") {
			continue
		}

		p := pattern{}

		// Handle negation prefix ('!'). Negation means "un-ignore this
		// entry even if a previous pattern matched it." Strip the prefix
		// and record the flag.
		if strings.HasPrefix(line, "!") {
			p.negated = true
			line = strings.TrimPrefix(line, "!")
		}

		// Handle directory suffix ('/'). A trailing slash means this
		// pattern should only match directories, not files. Strip the
		// suffix and record the flag.
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}

		// Detect glob metacharacters. If the pattern contains *, ?, or [
		// it needs filepath.Match evaluation rather than exact string comparison.
		if strings.ContainsAny(line, "*?[") {
			p.isGlob = true

			// Validate the glob pattern. filepath.Match returns ErrBadPattern
			// for malformed patterns (e.g., unclosed brackets). We skip these
			// with a warning rather than aborting the entire parse.
			_, err := filepath.Match(line, "")
			if err != nil {
				logger.Warn("skipping malformed glob pattern", "pattern", line, "error", err)
				continue
			}
		}

		p.value = line
		patterns = append(patterns, p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan patterns: %w", err)
	}

	return patterns, nil
}

// ShouldSkip reports whether the given name should be skipped during
// a filesystem walk. This is the primary method called from within
// filepath.Walk callbacks.
//
// Hidden directories (names starting with ".") always return true
// regardless of pattern configuration — this baseline cannot be
// overridden by negation patterns.
//
// For non-hidden entries, patterns are evaluated in order with
// last-match-wins semantics (matching gitignore behavior):
//   - dirOnly patterns are skipped when isDir is false
//   - glob patterns use filepath.Match for evaluation
//   - non-glob patterns use exact string comparison
//   - negation patterns set matched=false; regular patterns set matched=true
func (m *Matcher) ShouldSkip(name string, isDir bool) bool {
	// Hidden directory baseline: always skip directories starting with "."
	// This is a hardcoded rule that cannot be overridden by any pattern,
	// preserving existing behavior (FR-004).
	if isDir && strings.HasPrefix(name, ".") {
		return true
	}

	// Walk patterns in order. Last matching pattern wins, following
	// gitignore's precedence rules. This means a later negation can
	// override an earlier match, and a later match can override an
	// earlier negation.
	matched := false
	for _, p := range m.patterns {
		// Directory-only patterns don't apply to files.
		if p.dirOnly && !isDir {
			continue
		}

		var hit bool
		if p.isGlob {
			// filepath.Match cannot return an error here because we
			// validated the pattern during parsing (malformed patterns
			// were skipped). The only error case is ErrBadPattern.
			hit, _ = filepath.Match(p.value, name)
		} else {
			hit = p.value == name
		}

		if hit {
			if p.negated {
				matched = false
			} else {
				matched = true
			}
		}
	}

	return matched
}

// ShouldSkipPath reports whether a file at the given relative path
// should be skipped. Used by the file watcher event handler where
// the full relative path is available but not the individual directory
// name from a Walk callback.
//
// The path is normalized to forward slashes and split into components.
// Each directory component is checked with ShouldSkip(component, true),
// and the final component (filename) is checked with ShouldSkip(component, false).
// Returns true if any component matches an ignore pattern.
func (m *Matcher) ShouldSkipPath(relPath string) bool {
	// Normalize path separators to forward slashes before splitting.
	// This ensures consistent behavior across platforms (Windows uses
	// backslashes, but gitignore patterns always use forward slashes).
	normalized := filepath.ToSlash(relPath)

	parts := strings.Split(normalized, "/")
	if len(parts) == 0 {
		return false
	}

	// Check each directory component (all except the last).
	for _, dir := range parts[:len(parts)-1] {
		if dir == "" {
			continue
		}
		if m.ShouldSkip(dir, true) {
			return true
		}
	}

	// Check the final component as a filename.
	filename := parts[len(parts)-1]
	if filename == "" {
		return false
	}
	return m.ShouldSkip(filename, false)
}
