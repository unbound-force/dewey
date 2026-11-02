package sanitize

import (
	"strings"
	"testing"
)

func TestValidTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want bool
	}{
		{"simple", "auth", true},
		{"hyphenated", "vault-walker", true},
		{"underscored", "my_tag_123", true},
		{"uppercase", "ABC", true},
		{"empty", "", false},
		{"dot", "..", false},
		{"path traversal", "../evil", false},
		{"slash", "auth/sub", false},
		{"dot in name", "tag.dot", false},
		{"space", "has space", false},
		{"null byte", "null\x00byte", false},
		{"backslash", "back\\slash", false},
		{"newline", "a\nb", false},
		{"colon", "tag: yes", false},
		{"non-ascii", "t\u00e9g", false},
		{"129 bytes", strings.Repeat("a", 129), false},
		{"128 bytes", strings.Repeat("a", 128), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidTag(tt.tag); got != tt.want {
				t.Errorf("ValidTag(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestSafeFrontmatterScalar(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"plain ascii", "compiled/auth", true},
		{"enum value", "missing_rationale", true},
		{"spaces", "a b c", true},
		{"empty", "", true},
		{"newline", "a\nb", false},
		{"carriage return", "a\rb", false},
		{"null byte", "null\x00byte", false},
		{"tab", "a\tb", false},
		{"del", "a\x7fb", false},
		{"nel", "a\u0085b", false},
		{"line separator", "a\u2028b", false},
		{"paragraph separator", "a\u2029b", false},
		{"other control", "a\x01b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeFrontmatterScalar(tt.s); got != tt.want {
				t.Errorf("SafeFrontmatterScalar(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
