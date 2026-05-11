package sanitize

// ContentDrift detects when a page's content hash has changed between index
// cycles. It compares the previous content hash (from the last index) with
// the current content hash (from the current document).
//
// Returns nil in three cases:
//   - previousHash is empty (first-time indexing — no prior state to compare)
//   - previousHash equals currentHash (content unchanged)
//   - both hashes are empty (degenerate first-index case)
//
// Returns a Finding with severity "info" and category "drift" when hashes
// differ, indicating the page content has changed since the last index cycle.
// This is informational — content change is normal and expected. The finding
// enables downstream consumers (doctor, lint) to track change frequency.
//
// Design decision: Hash-only comparison keeps this function pure and O(1).
// Semantic diff analysis (e.g., what changed) is deferred to a future layer
// per FR-SAN-003's scope boundary.
func ContentDrift(previousHash, currentHash string) *Finding {
	// First-time index: no previous hash means nothing to compare against.
	if previousHash == "" {
		return nil
	}

	// Content unchanged: hashes match, no drift detected.
	if previousHash == currentHash {
		return nil
	}

	// Content changed: hashes differ, emit an informational finding.
	return &Finding{
		Pattern:  "content-hash-changed",
		Severity: "info",
		Category: "drift",
		Message:  "content hash changed since last index cycle",
	}
}
