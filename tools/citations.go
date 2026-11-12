package tools

import "strings"

// historyHeadingTitle and proposalsHeadingTitle name the level-2 headings
// that bound a compiled article's claim-bearing body (design D5): the
// trailing auto-generated History table, and the trailing Supersession
// Proposals section synthesis may append (design D4).
const (
	historyHeadingTitle   = "History"
	proposalsHeadingTitle = "Supersession Proposals"
)

// CitationSource is one declared source learning of a compiled article,
// carrying the supersession status the caller resolved from the learning's
// page properties.
type CitationSource struct {
	// Identity is the source learning's identity in bare form
	// ("auth-20261101T000000-alice") or page-name form
	// ("learning/auth-20261101T000000-alice"); the page-name form is
	// normalized to the bare identity before comparison, so compile's bare
	// identities and store_compiled's page names count identically.
	Identity string
	// Superseded reports whether the learning records a superseded_by link
	// or validity: superseded. A superseded source is accounted for when
	// cited in the body, named in the History table, or mentioned in the
	// body; a current source must be cited (ES-FR-010).
	Superseded bool
}

// CoverageOptions carries the inputs of CheckCitationCoverage.
type CoverageOptions struct {
	// Article is the full compiled-article markdown: leading frontmatter,
	// claim-bearing body, trailing History table, and any trailing
	// Supersession Proposals section.
	Article string
	// Sources lists the article's declared source learnings with their
	// supersession status.
	Sources []CitationSource
}

// CoverageResult reports a compiled article's citation coverage in both
// directions. Violations are named: UnresolvedCitations are the forward
// violations (a citation resolving to no declared source) and
// UncitedSources are the backward violations (a declared source neither
// cited nor accounted for).
type CoverageResult struct {
	// UnresolvedCitations lists citation identities extracted from the
	// claim-bearing body that resolve to no declared source, in order of
	// first occurrence, deduplicated.
	UnresolvedCitations []string
	// UncitedSources lists declared sources that are neither cited in the
	// claim-bearing body nor — when superseded — accounted for by the
	// History table or an in-body mention, in declared order.
	UncitedSources []string
	// CitedSources lists declared sources cited in the claim-bearing body,
	// in declared order.
	CitedSources []string
	// AccountedSupersededSources lists superseded sources accounted for by
	// the History table or an in-body mention rather than by a citation, in
	// declared order.
	AccountedSupersededSources []string
	// CitationCount is the number of distinct citation identities
	// extracted from the claim-bearing body.
	CitationCount int
	// ResolvedCitations is the number of distinct citations that resolved
	// to a declared source.
	ResolvedCitations int
	// SourceCount is the number of distinct declared sources after
	// normalization.
	SourceCount int
}

// Complete reports whether coverage holds in both directions: no
// unresolved citations and no uncited sources.
func (r CoverageResult) Complete() bool {
	return len(r.UnresolvedCitations) == 0 && len(r.UncitedSources) == 0
}

// CheckCitationCoverage verifies citation coverage in both directions
// against a compiled article's declared sources (design D5, ES-FR-010).
//
// Forward: every bracketed citation in the claim-bearing body resolves to
// a declared source. Backward: every declared source is cited at least
// once — except a superseded source, which is also accounted for when
// named in the article's History table or mentioned in the body (the bare
// identity outside any bracketed group); for a current source the History
// table does not satisfy coverage.
//
// The check is pure: the article text, the declared sources, and their
// supersession status are the only inputs, so callers resolve page
// properties themselves and the helper performs no store lookup.
func CheckCitationCoverage(opts CoverageOptions) CoverageResult {
	body, history := splitArticle(opts.Article)
	outside, groups := scanBrackets(body)
	citations := citationIdentities(groups)

	// Normalize and dedupe declared sources, preserving declared order.
	type declaredSource struct {
		identity   string
		superseded bool
	}
	var sources []declaredSource
	declared := make(map[string]bool, len(opts.Sources))
	for _, src := range opts.Sources {
		id := supersedesIdentity(src.Identity)
		if id == "" || declared[id] {
			continue
		}
		declared[id] = true
		sources = append(sources, declaredSource{identity: id, superseded: src.Superseded})
	}

	// Forward pass: resolve each citation against the declared set.
	res := CoverageResult{SourceCount: len(sources), CitationCount: len(citations)}
	cited := make(map[string]bool, len(sources))
	for _, id := range citations {
		if declared[id] {
			cited[id] = true
			res.ResolvedCitations++
			continue
		}
		res.UnresolvedCitations = append(res.UnresolvedCitations, id)
	}

	// Backward pass: account for each declared source. Superseded sources
	// may be satisfied by the History table or an in-body mention; current
	// sources only by a citation.
	for _, src := range sources {
		switch {
		case cited[src.identity]:
			res.CitedSources = append(res.CitedSources, src.identity)
		case src.superseded && (strings.Contains(history, src.identity) || strings.Contains(outside, src.identity)):
			res.AccountedSupersededSources = append(res.AccountedSupersededSources, src.identity)
		default:
			res.UncitedSources = append(res.UncitedSources, src.identity)
		}
	}
	return res
}

// splitArticle splits a compiled article into its claim-bearing body and
// its History section (design D5). The body is the text after the leading
// frontmatter block, ending at the earlier of the last "## History"
// heading — where buildCompiledArticle appends the auto-generated table —
// and the first "## Supersession Proposals" heading; an earlier "##
// History" heading does not end the body, so its content stays in scope
// and cannot fence citations out of the check. The History section runs
// from the last "## History" heading to the next heading of any level or
// the end of the article, and is "" when no History heading exists. The
// frontmatter sources list and any Supersession Proposals section are
// excluded from both returns, so they never count as citations or
// mentions.
func splitArticle(article string) (body, history string) {
	s := stripLeadingFrontmatter(article)
	lines := strings.SplitAfter(s, "\n")
	offsets := make([]int, len(lines))
	lastHistory, firstProposals := -1, -1
	offset := 0
	for i, line := range lines {
		offsets[i] = offset
		switch {
		case isLevel2Heading(line, historyHeadingTitle):
			lastHistory = i
		case firstProposals < 0 && isLevel2Heading(line, proposalsHeadingTitle):
			firstProposals = i
		}
		offset += len(line)
	}
	if lastHistory < 0 && firstProposals < 0 {
		return s, ""
	}
	cut := len(s)
	if lastHistory >= 0 {
		cut = offsets[lastHistory]
	}
	if firstProposals >= 0 && offsets[firstProposals] < cut {
		cut = offsets[firstProposals]
	}
	if lastHistory < 0 {
		return s[:cut], ""
	}
	end := len(s)
	for i := lastHistory + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "#") {
			end = offsets[i]
			break
		}
	}
	return s[:cut], s[offsets[lastHistory]:end]
}

// stripLeadingFrontmatter removes a leading YAML frontmatter block — a
// first line of "---" closed by a later line of "---" — and returns the
// text after the closing delimiter. Text without a closed leading block is
// returned unchanged, so an article that starts directly with a title
// works too.
func stripLeadingFrontmatter(article string) string {
	rest, ok := strings.CutPrefix(article, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(article, "---\r\n")
		if !ok {
			return article
		}
	}
	consumed := 0
	for consumed < len(rest) {
		line := rest[consumed:]
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i+1]
		}
		if strings.TrimRight(line, " \t\r\n") == "---" {
			return rest[consumed+len(line):]
		}
		consumed += len(line)
	}
	return article
}

// isLevel2Heading reports whether line is a markdown level-2 ATX heading
// with exactly the given title, for example "## History". The article
// generator and the synthesis prompt both write headings at column zero
// with a single space, so headings with leading indentation or a
// different title do not match.
func isLevel2Heading(line, title string) bool {
	rest, ok := strings.CutPrefix(line, "## ")
	if !ok {
		return false
	}
	return strings.TrimSpace(rest) == title
}

// scanBrackets walks s tracking square-bracket depth and returns the text
// outside every bracketed group together with the contents of the groups
// that open at depth zero, in order. A group is opened by "[" and closed
// by the matching "]" that returns depth to zero; brackets nested inside a
// group stay in its content, which is what keeps double-bracket wikilinks
// ("[[page]]") from reading as their inner text — the group content then
// carries the inner bracket and fails the identity grammar. Text after an
// unclosed "[" stays inside the group, and a stray "]" at depth zero is
// outside text.
func scanBrackets(s string) (outside string, groups []string) {
	var out strings.Builder
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			if depth == 0 {
				start = i
			}
			depth++
		case ']':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					groups = append(groups, s[start+1:i])
					start = -1
				}
			} else {
				out.WriteByte(s[i])
			}
		default:
			if depth == 0 {
				out.WriteByte(s[i])
			}
		}
	}
	return out.String(), groups
}

// citationIdentities returns the learning identities cited by the given
// bracketed groups: a group is a citation only when its entire
// comma-separated content matches the learning-identity grammar (design
// D3) — every element, trimmed, must pass isLearningIdentity, in either
// the timestamped or the legacy {tag}-{sequence} form. A group with any
// non-identity element is not a citation and is ignored entirely, so
// markdown link text, reference labels, numeric footnotes, checkbox
// brackets, double-bracket wikilinks, empty brackets, and prose in
// brackets never read as citations. Identities are returned in order of
// first occurrence, deduplicated.
func citationIdentities(groups []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, group := range groups {
		elements := strings.Split(group, ",")
		identities := make([]string, len(elements))
		valid := true
		for i, el := range elements {
			el = strings.TrimSpace(el)
			if !isLearningIdentity(el) {
				valid = false
				break
			}
			identities[i] = el
		}
		if !valid {
			continue
		}
		for _, id := range identities {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// extractCitations returns the learning identities cited in body, per the
// citation grammar of citationIdentities, in order of first occurrence,
// deduplicated.
func extractCitations(body string) []string {
	_, groups := scanBrackets(body)
	return citationIdentities(groups)
}
