package tools

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unbound-force/dewey/v3/store"
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
	// successor_equals_target, or empty_rationale from the entry ladder;
	// link_conflict, unresolved_learning, duplicate_proposal,
	// duplicate_target, duplicate_successor, or reverse_of_proposal from
	// the link-state check.
	Reason string
}

// parseSupersessionProposals parses, validates, and strips every "##
// Supersession Proposals" section from synthesized text (design D4,
// ES-FR-005/006). A section runs from its heading to the next heading of
// any level or the end of the text; headings and "#" lines inside fenced
// code blocks neither open nor close a section. The returned stripped
// text is everything outside the sections, concatenated. Text with no
// section is returned unchanged. Every section is parsed and stripped, so
// a synthesizer that emits the section twice never leaves one behind in
// the stored article.
//
// An entry is valid when its successor and target both resolve to one of
// sources (each normalized like a declared citation source), are distinct,
// and its rationale is non-empty after sanitization — control and
// invisible Unicode stripped, then bounded to maxProposalRationaleRunes
// runes. Valid entries are returned as proposals; invalid entries are
// dropped with a reason. The sections are stripped either way, so stored
// articles never contain them.
func parseSupersessionProposals(text string, sources []string) (stripped string, proposals []SupersessionProposal, dropped []DroppedProposal) {
	sourceSet := buildSourceSet(sources)

	lines := strings.SplitAfter(text, "\n")
	spans, offsets := locateProposalsSections(lines)
	if len(spans) == 0 {
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
	var out strings.Builder
	prev := 0
	for _, sp := range spans {
		out.WriteString(text[prev:offsets[sp.start]])
		prev = offsets[sp.end]
		for i := sp.start + 1; i < sp.end; i++ {
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
	}
	out.WriteString(text[prev:])
	return out.String(), proposals, dropped
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

// lineSpan is a half-open range of line indexes [start, end).
type lineSpan struct{ start, end int }

// locateProposalsSections finds every "## Supersession Proposals" section
// in lines (produced by strings.SplitAfter with trailing newlines
// retained). Each span runs from the heading line to the next heading
// line of any level, or to len(lines). Lines inside a fenced code block
// (``` or ~~~) are never headings, so a quoted example heading cannot
// open a section and a "#" comment inside a fence cannot close one. It
// also returns each line's byte offset, with offsets[len(lines)] equal to
// the text length, for slicing.
func locateProposalsSections(lines []string) (spans []lineSpan, offsets []int) {
	offsets = make([]int, len(lines)+1)
	headings := make([]bool, len(lines))
	offset := 0
	inFence := false
	for i, line := range lines {
		offsets[i] = offset
		offset += len(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		headings[i] = !inFence && strings.HasPrefix(line, "#")
	}
	offsets[len(lines)] = offset
	for i := 0; i < len(lines); i++ {
		if !headings[i] || !isLevel2Heading(lines[i], proposalsHeadingTitle) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if headings[j] {
				end = j
				break
			}
		}
		spans = append(spans, lineSpan{start: i, end: end})
		i = end - 1
	}
	return spans, offsets
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

// proposalRun tracks the proposals already kept during one compile or
// store_compiled call, so proposals that could not all be confirmed
// together are dropped before they reach the review gate.
type proposalRun struct {
	successorOf map[string]string // target → kept successor
	targetOf    map[string]string // successor → kept target
}

func newProposalRun() *proposalRun {
	return &proposalRun{successorOf: map[string]string{}, targetOf: map[string]string{}}
}

// filterProposalsByLinkState drops proposals that supersede_learning would
// refuse against the store's current declared links (the same link-state
// ladder it runs: target already superseded, successor already declaring
// a different target, target already superseding the successor), and
// proposals that conflict with one already kept in this run: the same
// pair twice, a second successor for one target, a second target for one
// successor, or the reverse of a kept pair. Without this, the review gate
// could present pairs that cannot all be applied, and a confirmed pair
// the tool then refuses would stall the run.
func filterProposalsByLinkState(s *store.Store, run *proposalRun, proposals []SupersessionProposal) (kept []SupersessionProposal, dropped []DroppedProposal) {
	for _, p := range proposals {
		if reason := proposalLinkStateReason(s, run, p); reason != "" {
			dropped = append(dropped, DroppedProposal{Successor: p.Successor, Target: p.Target, Reason: reason})
			continue
		}
		run.successorOf[p.Target] = p.Successor
		run.targetOf[p.Successor] = p.Target
		kept = append(kept, p)
	}
	return kept, dropped
}

// proposalLinkStateReason returns the drop reason for one proposal, or ""
// when it can be presented for confirmation.
func proposalLinkStateReason(s *store.Store, run *proposalRun, p SupersessionProposal) string {
	if succ, ok := run.successorOf[p.Target]; ok {
		if succ == p.Successor {
			return "duplicate_proposal"
		}
		return "duplicate_target"
	}
	if _, ok := run.targetOf[p.Successor]; ok {
		return "duplicate_successor"
	}
	if run.successorOf[p.Successor] == p.Target {
		return "reverse_of_proposal"
	}
	if s == nil {
		return ""
	}
	succProps, ok := learningProps(s, p.Successor)
	if !ok {
		return "unresolved_learning"
	}
	tgtProps, ok := learningProps(s, p.Target)
	if !ok {
		return "unresolved_learning"
	}
	if err := checkLinkStateLadder(succProps, tgtProps, p.Successor, p.Target); err != nil {
		return "link_conflict"
	}
	return ""
}

// learningProps loads and decodes a learning's stored properties.
func learningProps(s *store.Store, identity string) (map[string]any, bool) {
	page, err := s.GetPage(store.LearningPrefix + identity)
	if err != nil || page == nil || page.SourceID != store.SourceLearning {
		return nil, false
	}
	props, err := decodeLearningProperties(page.Properties)
	if err != nil {
		return nil, false
	}
	return props, true
}
