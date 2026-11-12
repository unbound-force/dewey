package tools

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxProposalRationaleRunes bounds a supersession-proposal rationale after
// sanitization (ES-FR-006): longer rationales are truncated to the bound.
const maxProposalRationaleRunes = 512

// SupersessionProposal is one validated supersession proposal parsed from
// a synthesized article's trailing "## Supersession Proposals" section
// (design D4, ES-FR-006).
type SupersessionProposal struct {
	// Successor is the identity of the learning that replaces Target.
	Successor string
	// Target is the identity of the learning being replaced.
	Target string
	// Rationale is the sanitized, rune-bounded justification. It is
	// LLM-authored data derived from learning content — every consumer
	// must present it, never execute it.
	Rationale string
}

// DroppedProposal is one invalid proposal entry and the reason it was
// dropped. Dropping never fails the article write (ES-FR-006).
type DroppedProposal struct {
	// Successor is the entry's normalized successor identity, possibly
	// empty when the entry never named one.
	Successor string
	// Target is the entry's normalized target identity, possibly empty.
	Target string
	// Reason is a machine-parseable drop reason: missing_successor,
	// missing_target, successor_not_source, target_not_source,
	// successor_equals_target, or empty_rationale.
	Reason string
}

// parseSupersessionProposals parses, validates, and strips the "##
// Supersession Proposals" section from synthesized text (design D4,
// ES-FR-005/006). The section runs from the first "## Supersession
// Proposals" heading to the next heading of any level or the end of the
// text; the returned stripped text is everything outside the section,
// concatenated. Text with no section is returned unchanged.
//
// An entry is valid when its successor and target both resolve to one of
// sources (each normalized like a declared citation source), are distinct,
// and its rationale is non-empty after sanitization — control and
// invisible Unicode stripped, then bounded to maxProposalRationaleRunes
// runes. Valid entries are returned as proposals; invalid entries are
// dropped with a reason. The section is stripped either way, so stored
// articles never contain it.
func parseSupersessionProposals(text string, sources []string) (stripped string, proposals []SupersessionProposal, dropped []DroppedProposal) {
	sourceSet := buildSourceSet(sources)

	lines := strings.SplitAfter(text, "\n")
	headingIdx, sectionEnd, offsets := locateProposalsSection(lines, len(text))
	if headingIdx < 0 {
		return text, nil, nil
	}

	// Entry parsing: a "- successor:" line opens an entry; indented
	// "target:" and "rationale:" lines complete it. Lines that belong to
	// no entry are ignored — the section is machine-parseable by
	// contract, and prose the synthesizer adds around entries has no
	// successor or target to report.
	var successor, target, rationale string
	inEntry := false
	flush := func() {
		if !inEntry {
			return
		}
		inEntry = false
		if p, reason := validateProposalEntry(successor, target, rationale, sourceSet); reason == "" {
			proposals = append(proposals, p)
		} else {
			dropped = append(dropped, DroppedProposal{
				Successor: supersedesIdentity(successor),
				Target:    supersedesIdentity(target),
				Reason:    reason,
			})
		}
		successor, target, rationale = "", "", ""
	}
	for i := headingIdx + 1; i < len(lines) && offsets[i] < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, "- successor:"):
			flush()
			inEntry = true
			successor = proposalFieldValue(trimmed, "- successor:")
		case !inEntry:
			// Prose or blank line between entries.
		case strings.HasPrefix(trimmed, "target:"):
			target = proposalFieldValue(trimmed, "target:")
		case strings.HasPrefix(trimmed, "rationale:"):
			rationale = proposalFieldValue(trimmed, "rationale:")
		}
	}
	flush()
	return text[:offsets[headingIdx]] + text[sectionEnd:], proposals, dropped
}

// buildSourceSet normalizes the declared source identities into a set the
// validation ladder resolves proposals against (ES-FR-006): each source is
// stripped of its learning/ prefix like a declared citation source, and
// empty results are skipped.
func buildSourceSet(sources []string) map[string]bool {
	sourceSet := make(map[string]bool, len(sources))
	for _, s := range sources {
		if id := supersedesIdentity(s); id != "" {
			sourceSet[id] = true
		}
	}
	return sourceSet
}

// locateProposalsSection finds the "## Supersession Proposals" section in
// lines (produced by strings.SplitAfter with trailing newlines retained).
// It returns the index of the section's heading line, the byte offset
// where the section ends (the next heading of any level, or textLen at
// end of text), and each line's byte offset for slicing. headingIdx is -1
// when the section is absent.
func locateProposalsSection(lines []string, textLen int) (headingIdx, sectionEnd int, offsets []int) {
	offsets = make([]int, len(lines))
	headingIdx = -1
	offset := 0
	for i, line := range lines {
		offsets[i] = offset
		if headingIdx < 0 && isLevel2Heading(line, proposalsHeadingTitle) {
			headingIdx = i
		}
		offset += len(line)
	}
	if headingIdx < 0 {
		return headingIdx, 0, offsets
	}
	sectionEnd = textLen
	for i := headingIdx + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "#") {
			sectionEnd = offsets[i]
			break
		}
	}
	return headingIdx, sectionEnd, offsets
}

// proposalResultFields flattens the per-cluster proposals and dropped
// entries into the compile result's top-level fields (ES-FR-006, design
// D8). Both slices are non-nil so serialized results carry empty arrays,
// never null.
func proposalResultFields(produced []synthesizedCluster) (proposed, dropped []map[string]any) {
	proposed = make([]map[string]any, 0)
	dropped = make([]map[string]any, 0)
	for _, sc := range produced {
		p, d := proposalMaps(sc.proposals, sc.dropped, sc.cluster.Topic)
		proposed = append(proposed, p...)
		dropped = append(dropped, d...)
	}
	return proposed, dropped
}

// proposalMaps renders validated proposals and dropped entries as the
// result fields shared by the compile and store_compiled paths
// (ES-FR-013). On the store_compiled path topic names the stored page —
// store_compiled has no cluster — keeping the review-gate presentation
// identical in shape across both paths. Both slices are non-nil so
// serialized results carry empty arrays, never null.
func proposalMaps(proposals []SupersessionProposal, dropped []DroppedProposal, topic string) (proposed, droppedMaps []map[string]any) {
	proposed = make([]map[string]any, 0)
	droppedMaps = make([]map[string]any, 0)
	for _, p := range proposals {
		proposed = append(proposed, map[string]any{
			"successor": p.Successor,
			"target":    p.Target,
			"rationale": p.Rationale,
			"topic":     topic,
		})
	}
	for _, d := range dropped {
		droppedMaps = append(droppedMaps, map[string]any{
			"successor": d.Successor,
			"target":    d.Target,
			"reason":    d.Reason,
		})
	}
	return proposed, droppedMaps
}

// proposalFieldValue returns the value after the field prefix, trimmed.
func proposalFieldValue(trimmedLine, prefix string) string {
	return strings.TrimSpace(strings.TrimPrefix(trimmedLine, prefix))
}

// validateProposalEntry applies the ES-FR-006 validity ladder to one
// parsed entry. It returns the valid proposal with an empty reason, or a
// zero proposal and the machine-parseable drop reason naming the first
// failing rule.
func validateProposalEntry(successor, target, rationale string, sourceSet map[string]bool) (SupersessionProposal, string) {
	succ := supersedesIdentity(successor)
	tgt := supersedesIdentity(target)
	switch {
	case succ == "":
		return SupersessionProposal{}, "missing_successor"
	case tgt == "":
		return SupersessionProposal{}, "missing_target"
	case !sourceSet[succ]:
		return SupersessionProposal{}, "successor_not_source"
	case !sourceSet[tgt]:
		return SupersessionProposal{}, "target_not_source"
	case succ == tgt:
		return SupersessionProposal{}, "successor_equals_target"
	}
	sanitized := sanitizeProposalRationale(rationale)
	if sanitized == "" {
		return SupersessionProposal{}, "empty_rationale"
	}
	return SupersessionProposal{Successor: succ, Target: tgt, Rationale: sanitized}, ""
}

// sanitizeProposalRationale strips control and invisible Unicode runes —
// Cc control characters, Cf format characters (zero-width spaces, bidi
// marks, the invisible operators, the BOM), and line/paragraph separators
// — then bounds the result to maxProposalRationaleRunes runes (ES-FR-006).
// The rationale is data presented at the review gate, never instructions,
// so the same trust boundary the identities get applies here.
func sanitizeProposalRationale(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isControlOrInvisibleRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if utf8.RuneCountInString(out) > maxProposalRationaleRunes {
		out = string([]rune(out)[:maxProposalRationaleRunes])
	}
	return out
}

// isControlOrInvisibleRune reports whether r is a control or invisible
// Unicode rune stripped by proposal-rationale sanitization: Cc control
// characters, Cf format characters (the zero-width and invisible-operator
// family the sanitize package enumerates, plus every other format rune),
// and the Zl/Zp separators.
func isControlOrInvisibleRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}
