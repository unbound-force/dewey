// Package sanitize inspects content entering the Dewey knowledge graph for
// security threats, structural anomalies, and data integrity signals. It
// produces structured findings that annotate documents without blocking
// indexing by default (warn mode). Blocking is opt-in via strict mode.
//
// The package implements five scan layers:
//   - Adversarial keyword scanning (injection patterns)
//   - Content hash drift detection
//   - Markdown structure validation
//   - Content size anomaly detection
//
// All functions are pure (no I/O, no global state mutation) and designed for
// deterministic, parallel-safe execution. The Scan orchestrator aggregates
// findings from all layers into a single ScanResult.
package sanitize

import (
	"io"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

// logger is the package-level structured logger for the sanitize package.
// Follows the same pattern as source/source.go for consistency.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/sanitize",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// SetLogLevel sets the logging level for the sanitize package.
// Use log.DebugLevel for verbose output during diagnostics.
func SetLogLevel(level log.Level) {
	logger.SetLevel(level)
}

// SetLogOutput replaces the sanitize package logger with one that writes to
// the given writer at the given level. Used to enable file logging.
func SetLogOutput(w io.Writer, level log.Level) {
	newLogger := log.NewWithOptions(w, log.Options{
		Prefix:          "dewey/sanitize",
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	})
	*logger = *newLogger
}

// Mode constants define the sanitization behavior for a source.
const (
	// ModeWarn logs findings at WARN level and merges them into page
	// properties but does not reject content. Default for web and github
	// sources.
	ModeWarn = "warn"

	// ModeStrict rejects documents with critical or high severity findings.
	// Documents are logged at ERROR level and skipped during indexing.
	ModeStrict = "strict"

	// ModeOff disables sanitization entirely. Default for disk and code
	// sources where content is user-authored.
	ModeOff = "off"
)

// DriftLevel classifies the magnitude of content change between index cycles.
type DriftLevel string

const (
	// DriftNormal indicates no meaningful content change was detected.
	DriftNormal DriftLevel = "normal"

	// DriftSignificant indicates a notable content change was detected.
	DriftSignificant DriftLevel = "significant"

	// DriftSuspicious indicates a content change that warrants investigation.
	DriftSuspicious DriftLevel = "suspicious"
)

// ScanInput contains the document content and metadata needed by the scan
// orchestrator. Follows AP-001 Options/Result pattern — callers construct
// the input struct with all relevant context, and Scan returns a structured
// result.
type ScanInput struct {
	// Content is the raw document content to scan.
	Content string

	// SourceID identifies the source that produced this document.
	SourceID string

	// DocumentID identifies the specific document being scanned.
	DocumentID string

	// SourceType is the source type (e.g., "web", "github", "disk", "code").
	SourceType string

	// PreviousHash is the content hash from the previous index cycle.
	// Empty string on first-time indexing.
	PreviousHash string

	// CurrentHash is the content hash of the current document content.
	CurrentHash string

	// SourceStats provides per-source size statistics for anomaly detection.
	// When nil, the size anomaly layer is skipped.
	SourceStats *SourceStats
}

// ScanConfig controls which scan layers execute and how findings are handled.
type ScanConfig struct {
	// Mode controls sanitizer behavior: "warn" (default), "strict", or "off".
	// In warn mode, findings are logged and merged into page properties.
	// In strict mode, documents with critical/high findings are rejected.
	// In off mode, scanning is skipped entirely.
	Mode string

	// InvisibleCharThreshold is the maximum number of invisible Unicode
	// characters allowed before a finding is generated. Default: 5.
	InvisibleCharThreshold int

	// Patterns is the injection pattern database used for adversarial
	// keyword scanning. When nil, DefaultPatterns is used.
	Patterns []PatternRule
}

// ScanResult contains the aggregated findings from all scan layers, along
// with metadata about the scan execution.
type ScanResult struct {
	// Findings contains all findings from all scan layers.
	Findings []Finding

	// SourceID identifies the source that produced the scanned document.
	SourceID string

	// DocumentID identifies the document that was scanned.
	DocumentID string

	// ScannedAt is the UTC timestamp when the scan was performed.
	ScannedAt time.Time

	// Layers lists which scan layers were executed (e.g., "injection",
	// "drift", "structure", "anomaly").
	Layers []string

	// PatternVersion is the version of the pattern database used for
	// injection scanning. Enables staleness detection when patterns are
	// updated in future releases.
	PatternVersion int
}

// Finding represents a single security or quality issue detected during
// content scanning.
type Finding struct {
	// Pattern is the name of the matched pattern or check that produced
	// this finding.
	Pattern string `json:"pattern"`

	// Line is the 1-indexed line number in the content where the finding
	// was detected. Zero when not applicable (e.g., document-level checks).
	Line int `json:"line"`

	// Severity classifies the finding's impact: "critical", "high",
	// "medium", "low", or "info".
	Severity string `json:"severity"`

	// Category groups the finding by scan layer: "injection", "drift",
	// "structure", or "anomaly".
	Category string `json:"category"`

	// Context contains up to 200 characters of surrounding content for
	// human review.
	Context string `json:"context,omitempty"`

	// Message is a human-readable description of the finding.
	Message string `json:"message"`
}

// SourceStats holds per-source content size statistics used by the size
// anomaly detector. Computed from all documents in the current indexing
// batch via ComputeStats.
type SourceStats struct {
	// Mean is the average content length (in characters) across all
	// documents in the source.
	Mean float64

	// StdDev is the standard deviation of content lengths across all
	// documents in the source.
	StdDev float64

	// Count is the number of documents used to compute the statistics.
	Count float64
}

// PatternRule, DefaultPatternVersion, DefaultPatterns, and
// ScanInjectionPatterns are defined in patterns.go.

// Scan orchestrates all scan layers against the provided input and returns
// an aggregated ScanResult. Each layer runs independently and contributes
// findings to the result.
//
// When config.Mode is "off", Scan returns an empty ScanResult with populated
// metadata (SourceID, DocumentID, ScannedAt) and zero findings. This allows
// callers to unconditionally use the result without nil checks.
//
// Scan layers executed (in order):
//  1. Injection patterns — adversarial keyword scanning
//  2. Content drift — hash-based change detection
//  3. Structure validation — invisible chars, data URIs, HTML
//  4. Size anomaly — statistical outlier detection
//
// Design decision: Scan follows the AP-001 Options/Result pattern. The
// ScanInput struct carries all document context, and ScanConfig controls
// behavior. This future-proofs the API for additional scan parameters
// without breaking callers.
func Scan(input ScanInput, config ScanConfig) (*ScanResult, error) {
	result := &ScanResult{
		SourceID:       input.SourceID,
		DocumentID:     input.DocumentID,
		ScannedAt:      time.Now().UTC(),
		PatternVersion: DefaultPatternVersion,
		Findings:       []Finding{},
		Layers:         []string{},
	}

	// Off mode: return empty result with metadata, no scanning performed.
	if config.Mode == ModeOff {
		return result, nil
	}

	// Apply defaults for zero-value config fields.
	if config.InvisibleCharThreshold == 0 {
		config.InvisibleCharThreshold = 5
	}

	// Layer 1: Injection pattern scanning.
	patterns := config.Patterns
	if patterns == nil {
		patterns = DefaultPatterns
	}
	injectionFindings := ScanInjectionPatterns(input.Content, patterns)
	result.Findings = append(result.Findings, injectionFindings...)
	result.Layers = append(result.Layers, "injection")

	// Layer 2: Content hash drift detection (drift.go).
	if driftFinding := ContentDrift(input.PreviousHash, input.CurrentHash); driftFinding != nil {
		result.Findings = append(result.Findings, *driftFinding)
	}
	result.Layers = append(result.Layers, "drift")

	// Layer 3: Markdown structure validation.
	// Placeholder — implemented in task 4.1 (ValidateStructure).
	structureFindings := ValidateStructure(input.Content, config.InvisibleCharThreshold)
	result.Findings = append(result.Findings, structureFindings...)
	result.Layers = append(result.Layers, "structure")

	// Layer 4: Content size anomaly detection.
	// Only runs when SourceStats are provided by the caller.
	if input.SourceStats != nil {
		if _, finding := SizeAnomaly(len(input.Content), *input.SourceStats); finding != nil {
			result.Findings = append(result.Findings, *finding)
		}
	}
	result.Layers = append(result.Layers, "anomaly")

	// Log findings based on mode.
	logFindings(input, config, result)

	return result, nil
}

// logFindings emits structured log entries for each finding based on the
// scan mode. In warn mode, findings are logged at WARN level. In strict
// mode, critical and high findings are logged at ERROR level; others at
// WARN level.
func logFindings(input ScanInput, config ScanConfig, result *ScanResult) {
	for _, f := range result.Findings {
		fields := []interface{}{
			"source", input.SourceID,
			"page", input.DocumentID,
			"pattern", f.Pattern,
			"severity", f.Severity,
			"category", f.Category,
			"line", f.Line,
		}

		switch config.Mode {
		case ModeStrict:
			if f.Severity == "critical" || f.Severity == "high" {
				logger.Error(f.Message, fields...)
			} else {
				logger.Warn(f.Message, fields...)
			}
		case ModeWarn:
			logger.Warn(f.Message, fields...)
		}
	}
}

// HasBlockingFindings returns true if any finding has critical or high severity.
// Used by strict mode to determine whether a document should be rejected.
// Exported so both cli.go and tools/indexing.go can share the same logic
// without duplication (DRY principle).
func HasBlockingFindings(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == "critical" || f.Severity == "high" {
			return true
		}
	}
	return false
}

// DetermineSanitizeMode resolves the sanitize mode for a source based on
// its type and optional explicit configuration. The sourceType parameter
// is the source's type string (e.g., "web", "github", "disk", "code").
// The explicitMode parameter is the value from the source config's
// "sanitize_mode" field, or empty string if not set.
//
// Default depends on source type per D9:
//   - disk, code → off
//   - web, github → warn
//
// Exported so both cli.go and tools/indexing.go can share the same logic.
func DetermineSanitizeMode(sourceType, explicitMode string) string {
	if explicitMode != "" {
		return explicitMode
	}

	switch sourceType {
	case "web", "github":
		return ModeWarn
	default:
		return ModeOff
	}
}

// DetermineTrustTier resolves the trust tier for a source. The explicitTier
// parameter is the value from the source config's "trust_tier" field, or
// empty string if not set. Default is "authored".
//
// Exported so both cli.go and tools/indexing.go can share the same logic.
func DetermineTrustTier(explicitTier string) string {
	if explicitTier != "" {
		return explicitTier
	}
	return "authored"
}
