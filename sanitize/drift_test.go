package sanitize

import "testing"

// TestContentDrift_HashChanged verifies that differing hashes produce an
// informational drift finding with the expected severity, category, pattern,
// and a non-empty message.
func TestContentDrift_HashChanged(t *testing.T) {
	finding := ContentDrift("abc123", "def456")
	if finding == nil {
		t.Fatal("expected a finding when hashes differ, got nil")
	}
	if finding.Severity != "info" {
		t.Errorf("severity = %q, want %q", finding.Severity, "info")
	}
	if finding.Category != "drift" {
		t.Errorf("category = %q, want %q", finding.Category, "drift")
	}
	if finding.Pattern != "content-hash-changed" {
		t.Errorf("pattern = %q, want %q", finding.Pattern, "content-hash-changed")
	}
	if finding.Message == "" {
		t.Error("message should not be empty")
	}
}

// TestContentDrift_HashUnchanged verifies that identical hashes produce no
// finding — content has not changed between index cycles.
func TestContentDrift_HashUnchanged(t *testing.T) {
	finding := ContentDrift("abc123", "abc123")
	if finding != nil {
		t.Errorf("expected nil when hashes match, got %+v", finding)
	}
}

// TestContentDrift_FirstIndex verifies that an empty previousHash (first-time
// indexing) produces no finding — there is no prior state to compare against.
func TestContentDrift_FirstIndex(t *testing.T) {
	finding := ContentDrift("", "abc123")
	if finding != nil {
		t.Errorf("expected nil on first index (empty previousHash), got %+v", finding)
	}
}

// TestContentDrift_EmptyHashes verifies that both hashes being empty produces
// no finding — this is a degenerate first-index case where no content hash
// was computed.
func TestContentDrift_EmptyHashes(t *testing.T) {
	finding := ContentDrift("", "")
	if finding != nil {
		t.Errorf("expected nil when both hashes are empty, got %+v", finding)
	}
}
