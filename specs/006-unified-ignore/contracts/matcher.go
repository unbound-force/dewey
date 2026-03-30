// Package contracts defines the interface contracts for the ignore package.
// This file is a design artifact — it specifies the API surface that
// ignore/ignore.go must implement. It is NOT compiled into the binary.
//
// Contract: ignore.Matcher
//
// The Matcher evaluates whether a given path or name should be skipped
// during a filesystem walk. It is built once per walk from the union of:
//   1. .gitignore patterns (root-level only)
//   2. sources.yaml ignore patterns (per-source)
//   3. Hidden-directory baseline (names starting with ".")
//
// Thread safety: Matcher is immutable after construction and safe for
// concurrent use by multiple goroutines.

package contracts

// --- Constructor ---

// NewMatcher creates a Matcher from the union of .gitignore patterns
// (read from gitignorePath) and additional patterns (from sources.yaml).
//
// If gitignorePath does not exist or is unreadable, only extraPatterns
// and the hidden-directory baseline apply (no error).
//
// If .gitignore contains malformed lines, those lines are skipped with
// a logged warning. Parsing does not abort.
//
// Parameters:
//   - gitignorePath: absolute path to .gitignore file (may not exist)
//   - extraPatterns: additional patterns from sources.yaml ignore field
//
// Returns:
//   - *Matcher: ready-to-use matcher (never nil)
//   - error: only for unexpected I/O errors (not file-not-found)
//
// Example:
//   m, err := ignore.NewMatcher("/path/to/.gitignore", []string{"drafts"})
//   if err != nil { ... }
//   // m is ready to use even if .gitignore doesn't exist

// --- Matching Methods ---

// ShouldSkip reports whether the given name should be skipped during
// a filesystem walk. This is the primary method called from within
// filepath.Walk callbacks.
//
// Parameters:
//   - name: the base name of the file or directory (e.g., "node_modules")
//   - isDir: true if the entry is a directory
//
// Returns true if the name matches any ignore pattern (gitignore,
// extra patterns, or hidden-directory rule). Returns false if the
// name matches a negation pattern that overrides a previous match.
//
// Hidden directories (names starting with ".") always return true
// regardless of pattern configuration.
//
// Pattern evaluation order:
//   1. Hidden-directory check (always skip)
//   2. Negation patterns (if name matches a negation, return false)
//   3. Gitignore patterns (if name matches, return true)
//   4. Extra patterns (if name matches, return true)
//   5. Default: return false (don't skip)
//
// Example:
//   m.ShouldSkip("node_modules", true)   // true (matches gitignore)
//   m.ShouldSkip(".git", true)           // true (hidden directory)
//   m.ShouldSkip("README.md", false)     // false (no match)
//   m.ShouldSkip("important.md", false)  // false (negation: !important.md)

// ShouldSkipPath reports whether a file at the given relative path
// should be skipped. Used by the file watcher event handler where
// the full relative path is available but not the individual directory
// name from a Walk callback.
//
// Checks each path component (directory names) and the final filename
// against the ignore patterns. Returns true if any component matches.
//
// Parameters:
//   - relPath: slash-separated relative path (e.g., "node_modules/foo/bar.md")
//
// Returns true if any path component matches an ignore pattern.
//
// Example:
//   m.ShouldSkipPath("node_modules/package/README.md")  // true
//   m.ShouldSkipPath("docs/guide.md")                   // false
//   m.ShouldSkipPath(".git/config")                     // true

// --- DiskSource Extensions ---

// DiskSourceOption configures a DiskSource.
// Follows the same func(*DiskSource) pattern as vault.Option.
//
// Available options:
//   - WithIgnorePatterns(patterns []string) — additional ignore patterns
//   - WithRecursive(recursive bool) — enable/disable subdirectory traversal

// NewDiskSource signature extension:
//   func NewDiskSource(id, name, basePath string, opts ...DiskSourceOption) *DiskSource
//
// When recursive=false, DiskSource.List() returns only .md files
// directly in basePath (no subdirectory traversal).
//
// When ignore patterns are provided, they are passed to NewMatcher()
// as extraPatterns alongside the .gitignore at basePath root.

// --- Vault Extensions ---

// WithIgnorePatterns is a new vault.Option that provides additional
// ignore patterns to the vault client. These are combined with
// .gitignore patterns found at the vault root.
//
//   func WithIgnorePatterns(patterns []string) Option
//
// The vault builds a Matcher during Load() from:
//   1. .gitignore at vaultPath root
//   2. patterns from WithIgnorePatterns()
//
// The same Matcher is used by Load(), walkVault(), addWatcherDirs(),
// and handleEvent().
