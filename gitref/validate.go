package gitref

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxRefLen bounds the length in bytes of a configured <remote>/<branch>
// value. Real remote and branch names are far shorter; the bound exists so a
// pathological configuration cannot flood git arguments or diagnostics.
const maxRefLen = 255

// refForbiddenRunes lists the characters git check-ref-format forbids in
// refnames, beyond control characters and space. The trailing "\\" is an
// escaped backslash.
const refForbiddenRunes = "~^:?*[\\"

// ValidateRefName reports whether ref is a well-formed <remote>/<branch>
// comparison-ref value — the grammar IP-FR-004 requires — applying
// git-check-ref-format rules plus the flag-injection guards design D7
// mandates. It is a pure function so every precedence leg validates
// identically, in-process, BEFORE the value reaches any git invocation.
//
// Rules (a value failing any rule is invalid and MUST degrade to the
// GTS-FR-004 draft fallback with a diagnostic naming it):
//
//   - non-empty, at most 255 bytes, and valid UTF-8
//   - at least one "/" separating a non-empty remote from a non-empty branch
//     (the branch itself may contain slashes, e.g. "origin/release/1.0")
//   - no leading "-" on the value or on any slash-separated component —
//     flag-like values such as "--upload-pack=evil" or "-h" are rejected
//   - no ASCII control characters (below 0x20), space, or DEL (0x7f), and
//     none of the check-ref-format forbidden characters ~ ^ : ? * [ \
//   - no two consecutive dots ".."; no component beginning with "." or
//     ending with ".lock"; no trailing "."; no "@{" sequence; not exactly "@"
//   - no leading or trailing "/" and no empty component ("//")
//
// The check is deliberately stricter than git itself where the two disagree
// (component-leading "-" and invalid UTF-8): both rejections fail safe into
// the draft fallback with a diagnostic, never into a git invocation.
// The rules are evaluated by small pure helpers — validateRefValue for the
// whole-value rules, validateRefComponent for each slash-separated
// component — in a fixed precedence order, so a value violating several
// rules always reports the same first failure. The decomposition applies
// Gaze's "decompose" fix strategy; it changes no rule and no diagnostic.
func ValidateRefName(ref string) error {
	if err := validateRefValue(ref); err != nil {
		return err
	}
	comps := strings.Split(ref, "/")
	if len(comps) < 2 {
		return errors.New("ref is not <remote>/<branch>: no '/' separator")
	}
	for _, c := range comps {
		if err := validateRefComponent(c); err != nil {
			return err
		}
	}
	return nil
}

// validateRefValue applies every whole-value rule in precedence order: the
// empty/length/UTF-8 bound, the value-level flag-injection guard, the
// forbidden-rune scan, the banned sequences, and the outer slash/dot shape.
// It runs before the value is split so byte- and rune-level rules always
// fire ahead of the component rules, exactly as a single monolithic check
// would.
func validateRefValue(ref string) error {
	if err := validateRefBound(ref); err != nil {
		return err
	}
	if ref[0] == '-' {
		return errors.New("ref begins with '-' (flag-like values are rejected)")
	}
	if err := validateRefRunes(ref); err != nil {
		return err
	}
	if err := validateRefSequences(ref); err != nil {
		return err
	}
	return validateRefShape(ref)
}

// validateRefBound enforces the preconditions that must hold before any
// byte- or rune-level inspection: the value is non-empty, within the length
// bound, and valid UTF-8. validateRefValue may index ref[0] afterwards
// because the empty case is rejected here.
func validateRefBound(ref string) error {
	if ref == "" {
		return errors.New("empty ref")
	}
	if len(ref) > maxRefLen {
		return fmt.Errorf("ref is %d bytes, exceeding the %d-byte bound", len(ref), maxRefLen)
	}
	if !utf8.ValidString(ref) {
		return errors.New("ref is not valid UTF-8")
	}
	return nil
}

// validateRefRunes scans every rune for the characters git forbids anywhere
// in a refname: ASCII control characters and space, DEL, and the
// check-ref-format forbidden set. The first offending rune in value order
// wins, so the diagnostic names the same character a whole-value scan would.
func validateRefRunes(ref string) error {
	for _, r := range ref {
		switch {
		case r <= 0x20:
			return fmt.Errorf("space or control character U+%04X", r)
		case r == 0x7f:
			return errors.New("delete character U+007F")
		case strings.ContainsRune(refForbiddenRunes, r):
			return fmt.Errorf("forbidden character %q", r)
		}
	}
	return nil
}

// validateRefSequences bans the multi-character forms git refuses in
// refnames: the ".." range syntax, the "@{" reflog syntax, and the bare "@"
// single-head form.
func validateRefSequences(ref string) error {
	if strings.Contains(ref, "..") {
		return errors.New("ref contains '..'")
	}
	if strings.Contains(ref, "@{") {
		return errors.New("ref contains '@{'")
	}
	if ref == "@" {
		return errors.New("ref is the single character '@'")
	}
	return nil
}

// validateRefShape checks the value's outer shape: no leading or trailing
// "/" and no "//" (each would leave an empty remote or branch component),
// and no trailing ".".
func validateRefShape(ref string) error {
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") || strings.Contains(ref, "//") {
		return errors.New("ref begins or ends with '/' or contains an empty component")
	}
	if strings.HasSuffix(ref, ".") {
		return errors.New("ref ends with '.'")
	}
	return nil
}

// validateRefComponent applies the per-component rules to one
// slash-separated component: the component-level flag-injection guard
// (leading "-") and git's leading-dot and ".lock"-suffix bans. The
// empty-component case is unreachable through ValidateRefName — the shape
// rules reject leading, trailing, and doubled slashes first — but is kept so
// the helper stays total for any caller.
func validateRefComponent(component string) error {
	switch {
	case component == "":
		return errors.New("ref contains an empty component")
	case component[0] == '-':
		return fmt.Errorf("component %q begins with '-' (flag-like values are rejected)", component)
	case component[0] == '.':
		return fmt.Errorf("component %q begins with '.'", component)
	case strings.HasSuffix(component, ".lock"):
		return fmt.Errorf("component %q ends with '.lock'", component)
	}
	return nil
}
