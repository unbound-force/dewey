package gitref

import (
	"strings"
	"testing"
)

// TestValidateRefName_GrammarAndInjection exercises the <remote>/<branch>
// grammar per IP-FR-004: git check-ref-format rules plus the flag-injection
// guards that reject values such as "--upload-pack=evil" before any git
// invocation.
func TestValidateRefName_GrammarAndInjection(t *testing.T) {
	cases := []struct {
		name    string
		ref     string
		wantErr string // empty means the ref must validate
	}{
		// Valid values.
		{name: "default ref", ref: "origin/main"},
		{name: "remote with dash", ref: "up-stream/main"},
		{name: "branch with nested slash", ref: "origin/release/1.0"},
		{name: "short names", ref: "o/m"},
		{name: "branch with dots", ref: "origin/v1.2.3"},
		{name: "branch with underscore", ref: "origin/feature_branch"},

		// Injection guards (D7): rejected before git ever runs.
		{name: "upload-pack injection", ref: "--upload-pack=evil", wantErr: "begins with '-'"},
		{name: "short-flag injection", ref: "-h", wantErr: "begins with '-'"},
		{name: "component leading dash", ref: "origin/-main", wantErr: "begins with '-'"},

		// Grammar: <remote>/<branch> requires a separator.
		{name: "no slash", ref: "main", wantErr: "no '/' separator"},
		{name: "empty", ref: "", wantErr: "empty ref"},

		// Whitespace and control characters.
		{name: "internal space", ref: "origin main", wantErr: "space or control"},
		{name: "leading space", ref: " origin/main", wantErr: "space or control"},
		{name: "trailing space", ref: "origin/main ", wantErr: "space or control"},
		{name: "tab", ref: "origin/ma\tin", wantErr: "space or control"},
		{name: "control character", ref: "origin/ma\x01in", wantErr: "space or control"},
		{name: "delete character", ref: "origin/ma\x7fin", wantErr: "delete character"},

		// check-ref-format forbidden characters.
		{name: "tilde", ref: "origin/ma~in", wantErr: "forbidden character"},
		{name: "caret", ref: "origin/ma^in", wantErr: "forbidden character"},
		{name: "colon", ref: "origin/ma:in", wantErr: "forbidden character"},
		{name: "question mark", ref: "origin/ma?in", wantErr: "forbidden character"},
		{name: "asterisk", ref: "origin/ma*in", wantErr: "forbidden character"},
		{name: "open bracket", ref: "origin/ma[in", wantErr: "forbidden character"},
		{name: "backslash", ref: "origin/ma\\in", wantErr: "forbidden character"},

		// Structural check-ref-format rules.
		{name: "consecutive dots", ref: "origin/ma..in", wantErr: "'..'"},
		{name: "component leading dot", ref: "origin/.main", wantErr: "begins with '.'"},
		{name: "component lock suffix", ref: "origin/main.lock", wantErr: "'.lock'"},
		{name: "trailing dot", ref: "origin/main.", wantErr: "ends with '.'"},
		{name: "at-brace sequence", ref: "origin/ma@{in", wantErr: "'@{'"},
		{name: "single at", ref: "@", wantErr: "single character '@'"},
		{name: "leading slash", ref: "/origin/main", wantErr: "'/'"},
		{name: "trailing slash", ref: "origin/main/", wantErr: "'/'"},
		{name: "empty component", ref: "origin//main", wantErr: "empty component"},

		// Bounded length and encoding.
		{name: "too long", ref: strings.Repeat("a", maxRefLen+1), wantErr: "bound"},
		{name: "invalid utf-8", ref: "origin/ma\xffin", wantErr: "not valid UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRefName(tc.ref)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateRefName(%q) = %v, want nil", tc.ref, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateRefName(%q) = nil, want error containing %q", tc.ref, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateRefName(%q) = %v, want error containing %q", tc.ref, err, tc.wantErr)
			}
		})
	}
}

// TestValidateRefName_MaxLengthBoundary checks the length bound is inclusive:
// a 255-byte value passes the bound check (it may still fail another rule,
// so the components are chosen to be otherwise valid).
func TestValidateRefName_MaxLengthBoundary(t *testing.T) {
	ref := "o/" + strings.Repeat("b", maxRefLen-2)
	if len(ref) != maxRefLen {
		t.Fatalf("fixture length = %d, want %d", len(ref), maxRefLen)
	}
	if err := ValidateRefName(ref); err != nil {
		t.Fatalf("ValidateRefName(255-byte ref) = %v, want nil", err)
	}
}

// TestValidateRefComponent_PerComponentRules exercises the component helper
// directly, including the empty-component branch that ValidateRefName itself
// cannot reach — the whole-value shape rules reject empty components first.
// The branch is kept so the helper stays total for any caller.
func TestValidateRefComponent_PerComponentRules(t *testing.T) {
	cases := []struct {
		name      string
		component string
		wantErr   string // empty means the component must validate
	}{
		{name: "empty component", component: "", wantErr: "empty component"},
		{name: "leading dash", component: "-main", wantErr: "begins with '-'"},
		{name: "leading dot", component: ".main", wantErr: "begins with '.'"},
		{name: "lock suffix", component: "main.lock", wantErr: "'.lock'"},
		{name: "valid component", component: "main"},
		{name: "valid component with dots", component: "v1.2.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRefComponent(tc.component)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRefComponent(%q) = %v, want nil", tc.component, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateRefComponent(%q) = nil, want error containing %q", tc.component, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateRefComponent(%q) = %v, want error containing %q", tc.component, err, tc.wantErr)
			}
		})
	}
}
