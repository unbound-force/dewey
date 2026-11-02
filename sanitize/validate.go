package sanitize

// maxTagBytes bounds the length of a ValidTag tag. Tags become filename
// components; an unbounded tag would fail with ENAMETOOLONG far from the
// validation site (PR 145 review amendment).
const maxTagBytes = 128

// ValidTag reports whether tag is safe to use as a filename component and a
// plain YAML scalar. It accepts a deliberately restricted character set —
// ASCII letters, digits, hyphen, and underscore — and a bounded length, so
// a validated tag can be embedded in file paths and frontmatter without
// quoting or escaping.
func ValidTag(tag string) bool {
	if tag == "" {
		return false
	}
	if len(tag) > maxTagBytes {
		return false
	}
	for _, r := range tag {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isAlpha && !isDigit && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// SafeFrontmatterScalar reports whether s can be embedded as a single-line
// YAML scalar without injecting new keys. It rejects all control characters
// (including NUL and DEL), the Unicode line separators U+0085, U+2028, and
// U+2029, and every code point below 0x20 — any of which could terminate a
// scalar and introduce attacker-controlled YAML structure.
func SafeFrontmatterScalar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return false
		}
	}
	return true
}
