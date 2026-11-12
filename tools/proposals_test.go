package tools

import (
	"strings"
	"testing"
	"time"
)

// TestParseSupersessionProposals_EverySectionStripped pins that a
// synthesizer emitting the proposals section twice leaves neither in the
// stored article, and both sections' entries are parsed.
func TestParseSupersessionProposals_EverySectionStripped(t *testing.T) {
	const a, b, c = "auth-20261101T000000-alice", "auth-20261102T000000-bob", "auth-20261103T000000-carol"
	text := "## Current State\n\nBody [" + b + "].\n\n" +
		"## Supersession Proposals\n\n- successor: " + b + "\n  target: " + a + "\n  rationale: b replaces a\n\n" +
		"## Open Conflicts\n\nKept.\n\n" +
		"## Supersession Proposals\n\n- successor: " + c + "\n  target: " + b + "\n  rationale: c replaces b\n"
	stripped, proposals, dropped := parseSupersessionProposals(text, []string{a, b, c})
	if strings.Contains(stripped, "Supersession Proposals") {
		t.Errorf("stripped text still carries a proposals section:\n%s", stripped)
	}
	if !strings.Contains(stripped, "## Open Conflicts\n\nKept.") {
		t.Errorf("content between sections must survive:\n%s", stripped)
	}
	if len(proposals) != 2 || len(dropped) != 0 {
		t.Fatalf("want 2 proposals and no drops, got %v / %v", proposals, dropped)
	}
}

// TestParseSupersessionProposals_FenceAware pins that a heading quoted
// inside a fenced code block neither opens a section nor, inside a real
// section, closes it.
func TestParseSupersessionProposals_FenceAware(t *testing.T) {
	const a, b = "auth-20261101T000000-alice", "auth-20261102T000000-bob"
	quoted := "## Current State\n\nThe format is:\n\n```\n## Supersession Proposals\n- successor: x\n```\n\nBody [" + b + "].\n"
	stripped, proposals, _ := parseSupersessionProposals(quoted, []string{a, b})
	if stripped != quoted || len(proposals) != 0 {
		t.Errorf("a fenced example heading must not be stripped or parsed; got %q / %v", stripped, proposals)
	}

	section := "Body.\n\n## Supersession Proposals\n\n```\n# not a heading\n```\n- successor: " + b + "\n  target: " + a + "\n  rationale: b replaces a\n"
	stripped, proposals, _ = parseSupersessionProposals(section, []string{a, b})
	if stripped != "Body.\n\n" {
		t.Errorf("whole section must be stripped, got %q", stripped)
	}
	if len(proposals) != 1 {
		t.Errorf("entry after a fenced '#' line must parse, got %v", proposals)
	}
}

// TestFilterProposalsByLinkState pins the link-state drops: pairs that
// supersede_learning would refuse against declared links, and pairs that
// conflict with one already kept in the same run, never reach the review
// gate.
func TestFilterProposalsByLinkState(t *testing.T) {
	s := newTestStore(t)
	const (
		a = "auth-20260315T100001-test"
		b = "auth-20260316T100002-test"
		c = "auth-20260317T100003-test"
		d = "auth-20260318T100004-test"
		e = "auth-20260319T100005-test"
	)
	at := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	storeLearningWithProps(t, s, a, `{"tag":"auth"}`, "a", at)
	storeLearningWithProps(t, s, b, `{"tag":"auth"}`, "b", at)
	storeLearningWithProps(t, s, c, `{"tag":"auth"}`, "c", at)
	// d is already superseded by e.
	storeLearningWithProps(t, s, d, `{"tag":"auth","superseded_by":"`+e+`","validity":"superseded"}`, "d", at)
	storeLearningWithProps(t, s, e, `{"tag":"auth","supersedes":"`+d+`"}`, "e", at)

	run := newProposalRun()
	in := []SupersessionProposal{
		{Successor: b, Target: a, Rationale: "r"}, // kept
		{Successor: b, Target: a, Rationale: "r"}, // duplicate_proposal
		{Successor: c, Target: a, Rationale: "r"}, // duplicate_target
		{Successor: b, Target: c, Rationale: "r"}, // duplicate_successor
		{Successor: a, Target: b, Rationale: "r"}, // reverse_of_proposal
		{Successor: c, Target: d, Rationale: "r"}, // link_conflict: d already superseded
		{Successor: e, Target: c, Rationale: "r"}, // link_conflict: e already supersedes d
		{Successor: c, Target: b, Rationale: "r"}, // kept: chain extension
	}
	kept, dropped := filterProposalsByLinkState(s, run, in)
	if len(kept) != 2 || kept[0].Target != a || kept[1].Target != b {
		t.Fatalf("kept = %v, want (b→a) and (c→b)", kept)
	}
	want := []string{"duplicate_proposal", "duplicate_target", "duplicate_successor", "reverse_of_proposal", "link_conflict", "link_conflict"}
	if len(dropped) != len(want) {
		t.Fatalf("dropped = %v, want %d entries", dropped, len(want))
	}
	for i, reason := range want {
		if dropped[i].Reason != reason {
			t.Errorf("dropped[%d].Reason = %q, want %q", i, dropped[i].Reason, reason)
		}
	}

	// A later call in the same run (another cluster) sees the kept pairs.
	_, dropped = filterProposalsByLinkState(s, run, []SupersessionProposal{{Successor: c, Target: a, Rationale: "r"}})
	if len(dropped) != 1 || dropped[0].Reason != "duplicate_target" {
		t.Errorf("cross-cluster duplicate target must drop, got %v", dropped)
	}
}
