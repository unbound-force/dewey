// Branch sanitizer tests (specs/018-weekly-distill-automation, T008;
// contracts/rolling-pr.md Branch Sanitization).
package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

func TestSanitizeThemeBranch_ContractExamples(t *testing.T) {
	cases := []struct {
		theme string
		want  string
	}{
		// "Auth/SSO..Flow" → "auth-sso-flow-a1b2c3d4" (hash elided in the
		// contract example; the shape is asserted separately).
		{"Auth/SSO..Flow", "auth-sso-flow"},
		// "/" sanitizes empty → untagged fallback.
		{"/", "untagged"},
		// Traversal collapses to dashes and trims.
		{"../../etc/passwd", "etc-passwd"},
		// Control characters and Unicode collapse.
		{"auth\x00sso\tflow", "auth-sso-flow"},
		{"AuthéÜFlow", "auth-flow"},
		// A theme that is only separators falls back to untagged.
		{"///", "untagged"},
		{"...", "untagged"},
	}
	for _, tc := range cases {
		got := SanitizeThemeBranch(tc.theme)
		if !strings.HasPrefix(got, tc.want+"-") {
			t.Errorf("SanitizeThemeBranch(%q) = %q, want prefix %q-", tc.theme, got, tc.want)
		}
		if !strings.HasSuffix(got, hashSuffixFor(t, tc.theme)) {
			t.Errorf("SanitizeThemeBranch(%q) = %q, want suffix %s", tc.theme, got, hashSuffixFor(t, tc.theme))
		}
	}
}

func TestSanitizeThemeBranch_Shape(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9-]+$`)
	cases := []string{
		"auth", "Auth/SSO..Flow", "../../etc/passwd", "/", "\x00\x01",
		strings.Repeat("x", 250), strings.Repeat("é", 100), "MIXED CASE 123",
	}
	for _, theme := range cases {
		got := SanitizeThemeBranch(theme)
		if !valid.MatchString(got) {
			t.Errorf("SanitizeThemeBranch(%q) = %q, want [a-z0-9-]+", theme, got)
		}
		if strings.ContainsAny(got, "/.") {
			t.Errorf("SanitizeThemeBranch(%q) = %q, contains traversal characters", theme, got)
		}
		if len(got) > maxSanitizedSegmentChars {
			t.Errorf("SanitizeThemeBranch(%q) = %q, exceeds %d chars", theme, got, maxSanitizedSegmentChars)
		}
	}
}

func TestSanitizeThemeBranch_LongThemeTruncatesBase(t *testing.T) {
	long := strings.Repeat("ab", 100) // 200 chars of [a-z0-9]
	got := SanitizeThemeBranch(long)
	// base (50) + "-" + hash (8) = 59.
	if len(got) != maxSanitizedSegmentChars {
		t.Fatalf("len = %d, want %d", len(got), maxSanitizedSegmentChars)
	}
	base := strings.TrimSuffix(strings.TrimSuffix(got, hashSuffixFor(t, long)), "-")
	if len(base) != maxSanitizedBaseChars {
		t.Errorf("base len = %d, want %d (base %q)", len(base), maxSanitizedBaseChars, base)
	}
	if !strings.HasPrefix(got, "ababab") {
		t.Errorf("truncated base lost its prefix: %q", got)
	}
}

func TestSanitizeThemeBranch_TruncationTrimsTrailingDash(t *testing.T) {
	// A 51-char theme whose 51st char is a separator: truncation at 50
	// leaves no trailing dash to trim, but a 50-char cut ending in "-" at
	// position 50 exercises the trim.
	theme := strings.Repeat("a", 49) + "-X"
	got := SanitizeThemeBranch(theme)
	if strings.HasSuffix(strings.TrimSuffix(got, hashSuffixFor(t, theme)), "-") {
		t.Errorf("base keeps a trailing dash: %q", got)
	}
}

func TestSanitizeThemeBranch_DistinctThemesDistinctBranches(t *testing.T) {
	themes := []string{
		"Auth", "auth", // same sanitized base, different bytes → different hash
		"authentication", "vault-walker", "///", "..",
	}
	seen := make(map[string]string)
	for _, theme := range themes {
		branch := SanitizeThemeBranch(theme)
		if other, dup := seen[branch]; dup {
			t.Errorf("themes %q and %q collided on %q", other, theme, branch)
		}
		seen[branch] = theme
	}
}

func TestAssignThemeBranches_DuplicateThemeStringsShareBranch(t *testing.T) {
	assigned := AssignThemeBranches([]string{"alpha", "alpha", "alpha"})
	if len(assigned) != 1 {
		t.Fatalf("assigned %d themes, want 1", len(assigned))
	}
	if err := ValidateThemeBranch(assigned["alpha"]); err != nil {
		t.Errorf("ValidateThemeBranch(%q): %v", assigned["alpha"], err)
	}
	if strings.HasSuffix(assigned["alpha"], "-2") {
		t.Errorf("duplicate theme string gained a collision suffix: %q", assigned["alpha"])
	}
}

func TestAssignThemeBranches_HashCollisionSuffixed(t *testing.T) {
	// A forced segment collision — distinct themes mapping to the same
	// sanitized segment, the hash-collision path — suffixes -2, -3 in
	// lexicographic theme order.
	assigned := assignThemeBranches(
		[]string{"zeta", "alpha", "beta"},
		func(string) string { return "same-segment" },
	)
	if assigned["alpha"] != DistillBranchPrefix+"same-segment" {
		t.Errorf("lexicographically first theme = %q, want unsuffixed", assigned["alpha"])
	}
	if assigned["beta"] != DistillBranchPrefix+"same-segment-2" {
		t.Errorf("second theme = %q, want -2 suffix", assigned["beta"])
	}
	if assigned["zeta"] != DistillBranchPrefix+"same-segment-3" {
		t.Errorf("third theme = %q, want -3 suffix", assigned["zeta"])
	}
}

func TestAssignThemeBranches_UniqueThemesUnsuffixed(t *testing.T) {
	assigned := AssignThemeBranches([]string{"zeta", "alpha", "beta"})
	if len(assigned) != 3 {
		t.Fatalf("assigned %d themes, want 3", len(assigned))
	}
	for theme, branch := range assigned {
		if !strings.HasPrefix(branch, DistillBranchPrefix) {
			t.Errorf("theme %q branch = %q, want prefix %q", theme, branch, DistillBranchPrefix)
		}
		if strings.HasSuffix(branch, "-2") || strings.HasSuffix(branch, "-3") {
			t.Errorf("unique theme %q gained a collision suffix: %q", theme, branch)
		}
		if err := ValidateThemeBranch(branch); err != nil {
			t.Errorf("ValidateThemeBranch(%q for %q): %v", branch, theme, err)
		}
	}
}

func TestValidateThemeBranch(t *testing.T) {
	if err := ValidateThemeBranch(DistillBranchPrefix + "auth-" + "a1b2c3d4"); err != nil {
		t.Errorf("valid branch rejected: %v", err)
	}
	if err := ValidateThemeBranch("main"); err == nil {
		t.Error("branch without prefix accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix); err == nil {
		t.Error("empty segment accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix + "Auth"); err == nil {
		t.Error("uppercase segment accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix + "auth/escape"); err == nil {
		t.Error("slash in segment accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix + ".."); err == nil {
		t.Error("traversal segment accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix + strings.Repeat("a", 70)); err == nil {
		t.Error("over-length segment accepted")
	}
	if err := ValidateThemeBranch(DistillBranchPrefix + strings.Repeat("a", 70)); err == nil {
		t.Error("over-length branch accepted")
	}
}

// hashSuffixFor computes the expected 8-hex sha256(theme) suffix so tests
// assert the contract's exact hash input (the ORIGINAL theme bytes).
func hashSuffixFor(t *testing.T, theme string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(theme))
	return "-" + hex.EncodeToString(sum[:])[:hashSuffixLen]
}
