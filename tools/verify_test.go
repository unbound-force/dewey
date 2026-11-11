package tools

// Tests for the verify_candidates tool (issue #151): enumeration and scope
// (tasks.md 3.1), anchor outcomes and compiled-source aggregation (tasks.md
// 3.2), degraded and failure modes with determinism and non-mutation
// (tasks.md 3.3), the golden checked output shape (tasks.md 3.5), and the
// full-store round-trip non-mutation audit (tasks.md 3.6).
//
// The enumeration tests pin the selector contract of KV-FR-001 and the
// skipped-entry contract of KV-FR-002: pages resolves exact stored names
// only, tags match exactly through the page-name delimiter or the tag
// property (never for knowledge/ names), supplying both selectors is
// rejected before any verification runs, the default queue is every tier
// draft page under learning/, compiled/, and knowledge/, duplicate requests
// are deduplicated, and a requested page that is missing or not draft is
// reported as skipped with a reason rather than silently dropped. There the
// fixed-ok fakeAnchors checker (reused from the lint anchor tests) only
// drives each pass to completion, so the enumerated queue is observable as
// a checked result.
//
// The anchor-outcome tests below drive a real temporary git repository
// through the same anchorRepo plumbing the lint anchor tests use, pinning
// KV-FR-003's outcome matrix — anchors_ok, drifted, and missing (a path
// absent at the pin, or a pin that is not a commit in the repository) —
// the compiled-article aggregation with per-source results, and the
// failed-pass routing of KV-FR-004 when the checker cannot answer.
//
// The degraded-mode tests pin the rest of KV-FR-004 and KV-FR-005: a vault
// that is not a git repository skips the pass — every candidate skipped
// with the ineligibility reason, and not an error result — while a git
// failure, a cancelled request, or an exhausted budget fails the pass as an
// error result whose every entry, candidates checked before the failure
// included, is skipped with a reason naming the failed pass; two runs over
// an unchanged store and HEAD return identical output; and a completed
// pass leaves every candidate's tier, validity, content, and files
// byte-identical.
//
// The golden test pins the observable shape of one representative checked
// pass (DE-007 layer 2): the exact JSON document a caller parses — pass
// status and value, every per-candidate field, the fixed result enum spelled
// as literal wire values, and the candidate-counting summary — asserted
// against a golden fixture, plus the stability of that field set across
// runs, complementing TestVerify_DeterministicRepeat's full-value comparison
// with a field-set comparison.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// newTestVerify builds a Verify over s with the given anchor checker
// injected through the unexported seam, mirroring the lint anchor tests:
// NewVerify with an empty vault path leaves anchors nil, and the field is
// assigned directly so no git binary or repository is needed.
func newTestVerify(s *store.Store, anchors anchorChecker) *Verify {
	v := NewVerify(s, "")
	v.anchors = anchors
	return v
}

// insertVerifyPage stores a page with the given name, tier, and properties
// for verify fixtures. Unlike insertPinnedLearning it covers every
// namespace and tier. Enumeration filters on name, tier, and properties
// only, so the source pair is derived from the name to keep call sites
// short; a nil props marshals to JSON null, which reads as no properties.
func insertVerifyPage(t *testing.T, s *store.Store, name, tier string, props map[string]any) {
	t.Helper()
	data, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal properties for %q: %v", name, err)
	}
	source := name
	if i := strings.Index(name, "/"); i >= 0 {
		source = name[:i]
	}
	if err := s.InsertPage(&store.Page{
		Name:         name,
		OriginalName: name,
		SourceID:     source,
		SourceDocID:  name,
		Properties:   string(data),
		Tier:         tier,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

// runVerifyChecked runs the verify tool and returns its parsed JSON result,
// failing the test unless the pass completed as checked — the observable
// proof that the enumerated queue, not a degraded pass, is under test.
func runVerifyChecked(t *testing.T, v *Verify, input types.VerifyCandidatesInput) map[string]any {
	t.Helper()
	result, _, err := v.VerifyCandidates(context.Background(), nil, input)
	if err != nil || result.IsError {
		t.Fatalf("VerifyCandidates = %v, %v", resultText(result), err)
	}
	parsed := parseLearningResult(t, resultText(result))
	if status, _ := parsed["status"].(string); status != verifyPassChecked {
		t.Fatalf("status = %q, want %q: %v", status, verifyPassChecked, parsed)
	}
	return parsed
}

// verifyCandidateEntries returns the result's candidate entries in order.
func verifyCandidateEntries(t *testing.T, parsed map[string]any) []map[string]any {
	t.Helper()
	list, ok := parsed["candidates"].([]any)
	if !ok {
		t.Fatalf("candidates missing from result: %v", parsed)
	}
	entries := make([]map[string]any, 0, len(list))
	for _, c := range list {
		e, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("candidate entry is not an object: %v", c)
		}
		entries = append(entries, e)
	}
	return entries
}

// verifyCandidateNames returns the candidate page names in result order.
func verifyCandidateNames(t *testing.T, parsed map[string]any) []string {
	t.Helper()
	entries := verifyCandidateEntries(t, parsed)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name, _ := e["page"].(string)
		names = append(names, name)
	}
	return names
}

// verifyEntryOf returns the candidate entry carrying page, failing the
// test when no candidate does.
func verifyEntryOf(t *testing.T, parsed map[string]any, page string) map[string]any {
	t.Helper()
	for _, e := range verifyCandidateEntries(t, parsed) {
		if name, _ := e["page"].(string); name == page {
			return e
		}
	}
	t.Fatalf("no candidate entry for page %q in %v", page, verifyCandidateNames(t, parsed))
	return nil
}

// requireVerifyPages asserts that the result's candidate page names equal
// want exactly, in order and with no duplicates or extras.
func requireVerifyPages(t *testing.T, parsed map[string]any, want []string) {
	t.Helper()
	got := verifyCandidateNames(t, parsed)
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidates[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

// TestVerify_PagesSelector pins the pages selector's exact-only resolution
// (KV-FR-001, KV-FR-002): names match stored page names exactly — no
// case-insensitive ladder and no bare-name shorthand, unlike promote — and
// a requested name that matches no stored page, or a page whose tier is
// not draft, becomes a skipped candidate carrying a reason, never a silent
// drop. The selector names explicit candidates only: unrequested pages
// never join the queue.
func TestVerify_PagesSelector(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "auth-1", map[string]any{"tag": "auth"})
	insertVerifyPage(t, s, "compiled/gateway", "draft", nil)
	insertVerifyPage(t, s, "learning/validated-1", "validated", nil)

	parsed := runVerifyChecked(t, newTestVerify(s, fakeAnchors{}), types.VerifyCandidatesInput{Pages: []string{
		"learning/auth-1",      // exact draft page → verified candidate
		"learning/AUTH-1",      // case variant → no exact match
		"learning/auth",        // partial string → no exact match
		"auth-1",               // bare name → no shorthand ladder
		"learning/missing-1",   // no stored page
		"learning/validated-1", // stored, but tier is not draft
	}})

	// One entry per requested name, ordered by name; compiled/gateway was
	// never requested, so it must not appear.
	requireVerifyPages(t, parsed, []string{
		"auth-1",
		"learning/AUTH-1",
		"learning/auth",
		"learning/auth-1",
		"learning/missing-1",
		"learning/validated-1",
	})

	for _, name := range []string{"auth-1", "learning/AUTH-1", "learning/auth", "learning/missing-1"} {
		e := verifyEntryOf(t, parsed, name)
		if result, _ := e["result"].(string); result != verifySkipped {
			t.Errorf("%s: result = %q, want %q", name, result, verifySkipped)
		}
		reason, _ := e["reason"].(string)
		if !strings.Contains(reason, "no stored page named") || !strings.Contains(reason, name) {
			t.Errorf("%s: reason = %q, want a missing-page reason naming it", name, reason)
		}
		// An unresolved page has no facts to present, so no tier may be
		// fabricated for it.
		if tier, ok := e["tier"].(string); ok {
			t.Errorf("%s: tier = %q on an unresolved page, want omitted", name, tier)
		}
	}

	// A requested page that exists but is not draft is skipped with a
	// reason naming its tier and the draft-only rule.
	nonDraft := verifyEntryOf(t, parsed, "learning/validated-1")
	if result, _ := nonDraft["result"].(string); result != verifySkipped {
		t.Errorf("non-draft result = %q, want %q", result, verifySkipped)
	}
	reason, _ := nonDraft["reason"].(string)
	if !strings.Contains(reason, "validated") || !strings.Contains(reason, "only draft pages are candidates") {
		t.Errorf("non-draft reason = %q, want the tier and the draft-only rule", reason)
	}

	// The one exactly resolved draft page is enumerated as a candidate,
	// not skipped, and keeps its stored tier.
	resolved := verifyEntryOf(t, parsed, "learning/auth-1")
	if result, _ := resolved["result"].(string); result == verifySkipped {
		t.Errorf("resolved candidate result = %q, want a checked outcome", result)
	}
	if tier, _ := resolved["tier"].(string); tier != "draft" {
		t.Errorf("resolved candidate tier = %q, want draft", tier)
	}
}

// TestVerify_TagsExactMatching pins KV-FR-001's tag contract: membership is
// read from the page-name prefixes and the tag property, and both forms
// match exactly. The name form requires the delimiter after the tag —
// learning/auth-flow-… is a member of auth, learning/authentication-… is
// not — or the exact compiled name; the property form requires equality;
// and knowledge/ page names carry no tag prefix, so those pages match only
// through the property.
func TestVerify_TagsExactMatching(t *testing.T) {
	s := newTestStore(t)
	// Name-form members of auth: the delimiter after the tag, and the
	// exact compiled name.
	insertPinnedLearning(t, s, "auth-1", map[string]any{"tag": "auth"})
	insertPinnedLearning(t, s, "auth-flow-1", map[string]any{"tag": "auth-flow"})
	insertVerifyPage(t, s, "compiled/auth", "draft", nil)
	// Name-form non-members: authentication is a different tag, and
	// compiled matching is the exact name, not a prefix.
	insertPinnedLearning(t, s, "authentication-1", map[string]any{"tag": "authentication"})
	insertVerifyPage(t, s, "compiled/authentication", "draft", nil)
	insertVerifyPage(t, s, "compiled/auth-extra", "draft", nil)
	// Property-form members and non-members: exact equality.
	insertPinnedLearning(t, s, "other-1", map[string]any{"tag": "auth"})
	insertPinnedLearning(t, s, "other-2", map[string]any{"tag": "authentication"})
	// knowledge/ page names carry no tag prefix, so only the property can
	// match — a knowledge/ name that happens to start with auth- must not.
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "auth"})
	insertVerifyPage(t, s, "knowledge/team/other", "draft", map[string]any{"tag": "gateway"})
	insertVerifyPage(t, s, "knowledge/auth-named", "draft", map[string]any{"tag": "gateway"})

	parsed := runVerifyChecked(t, newTestVerify(s, fakeAnchors{}), types.VerifyCandidatesInput{Tags: []string{"auth"}})
	requireVerifyPages(t, parsed, []string{
		"compiled/auth",
		"knowledge/team/file",
		"learning/auth-1",
		"learning/auth-flow-1",
		"learning/other-1",
	})
}

// TestVerify_BothSelectorsRejected pins KV-FR-001's both-selectors rule:
// supplying pages and tags together is a validation error returned before
// any verification runs.
func TestVerify_BothSelectorsRejected(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "auth-1", map[string]any{"tag": "auth"})
	// If the input were not rejected before the pass, Head would run and
	// this sentinel would surface inside a failed-pass result; it must
	// appear nowhere.
	sentinel := errors.New("verification must not start")

	v := newTestVerify(s, fakeAnchors{headErr: sentinel})
	result, _, err := v.VerifyCandidates(context.Background(), nil, types.VerifyCandidatesInput{
		Pages: []string{"learning/auth-1"},
		Tags:  []string{"auth"},
	})
	if err != nil {
		t.Fatalf("VerifyCandidates: %v", err)
	}
	if !result.IsError {
		t.Fatalf("both selectors = %q, want an error result", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "provide either pages or tags, not both") {
		t.Errorf("error = %q, want the both-selectors validation message", text)
	}
	if strings.Contains(text, sentinel.Error()) {
		t.Errorf("error = %q: the verification pass ran despite the rejected input", text)
	}
}

// TestVerify_DefaultQueue pins the no-selector queue (KV-FR-001): every
// tier draft page under learning/, compiled/, and knowledge/ — including a
// draft knowledge/ page — while non-draft pages and pages outside the
// three namespaces never qualify.
func TestVerify_DefaultQueue(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "draft-1", map[string]any{"tag": "queue"})
	insertVerifyPage(t, s, "learning/validated-1", "validated", nil)
	insertVerifyPage(t, s, "compiled/draft-topic", "draft", nil)
	insertVerifyPage(t, s, "compiled/validated-topic", "validated", nil)
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "team"})
	insertVerifyPage(t, s, "knowledge/team/curated-thing", "curated", map[string]any{"tag": "team"})
	insertVerifyPage(t, s, "notes/elsewhere", "draft", nil)

	parsed := runVerifyChecked(t, newTestVerify(s, fakeAnchors{}), types.VerifyCandidatesInput{})
	requireVerifyPages(t, parsed, []string{
		"compiled/draft-topic",
		"knowledge/team/file",
		"learning/draft-1",
	})
	// Every enumerated candidate is a draft page; the validated, curated,
	// and out-of-namespace pages are excluded by the exact list above.
	for _, e := range verifyCandidateEntries(t, parsed) {
		if tier, _ := e["tier"].(string); tier != "draft" {
			t.Errorf("%v: tier = %q, want draft", e["page"], tier)
		}
	}
}

// TestVerify_DeduplicatesCandidates pins KV-FR-001's deduplication: a page
// named twice by the pages selector, or matching two requested tags at
// once, is enumerated exactly once.
func TestVerify_DeduplicatesCandidates(t *testing.T) {
	s := newTestStore(t)
	// Member of auth by name form and of flow by the tag property.
	insertPinnedLearning(t, s, "auth-1", map[string]any{"tag": "flow"})
	v := newTestVerify(s, fakeAnchors{})

	parsed := runVerifyChecked(t, v, types.VerifyCandidatesInput{
		Pages: []string{"learning/auth-1", "learning/auth-1"},
	})
	requireVerifyPages(t, parsed, []string{"learning/auth-1"})

	parsed = runVerifyChecked(t, v, types.VerifyCandidatesInput{Tags: []string{"auth", "flow"}})
	requireVerifyPages(t, parsed, []string{"learning/auth-1"})
}

// --- Anchor outcomes and compiled aggregation (tasks.md 3.2) ---
//
// The tests below pin KV-FR-003 and KV-FR-004's failure routing. Real-git
// tests reuse anchorRepo from the lint anchor tests — the same t.TempDir +
// git init + commits plumbing — so the production gitAnchorChecker answers
// every question; only the checker-error test substitutes the fakeAnchors
// seam, never shelling out beyond what the lint tests already do.

// verifyStringList decodes a JSON list field into strings, failing the
// test when the field is missing or an element is not a string.
func verifyStringList(t *testing.T, field string, got any) []string {
	t.Helper()
	list, ok := got.([]any)
	if !ok {
		t.Fatalf("%s = %v, want a list of strings", field, got)
	}
	out := make([]string, 0, len(list))
	for i, e := range list {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("%s[%d] = %v, want a string", field, i, e)
		}
		out = append(out, s)
	}
	return out
}

// requireStrings asserts that got equals want exactly, in order.
func requireStrings(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("%s[%d] = %q, want %q (all: %v)", field, i, got[i], w, got)
		}
	}
}

// verifySourceEntry returns the per-source result a compiled entry's
// sources list carries for identity, failing the test when the entry
// carries no such source.
func verifySourceEntry(t *testing.T, entry map[string]any, identity string) map[string]any {
	t.Helper()
	list, ok := entry["sources"].([]any)
	if !ok {
		t.Fatalf("entry for %v carries no sources list: %v", entry["page"], entry)
	}
	for _, s := range list {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("source result is not an object: %v", s)
		}
		if id, _ := m["identity"].(string); id == identity {
			return m
		}
	}
	t.Fatalf("no source result for identity %q in %v", identity, list)
	return nil
}

// verifySummaryCount returns the summary count for one result value. Every
// enum value has a fixed key (KV-FR-002), so a zero count must be present
// as zero, never absent.
func verifySummaryCount(t *testing.T, parsed map[string]any, result string) int {
	t.Helper()
	summary, ok := parsed["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing from result: %v", parsed)
	}
	count, ok := summary[result].(float64)
	if !ok {
		t.Fatalf("summary[%q] = %v, want a number", result, summary[result])
	}
	return int(count)
}

// TestVerify_AnchorOutcomes pins KV-FR-003's learning outcome matrix
// against a real temporary git repository: a cited path unchanged between
// the pin and HEAD is anchors_ok; a path changed — or removed — since the
// pin is drifted; a path absent at the pin, or a pin that names no commit
// in the repository, is missing. A learning with evidence but no readable
// pin is no_anchors with the ineligibility as context, and the pass stays
// checked (KV-FR-002: absence of anchors is a fact about the candidate,
// not a failure).
func TestVerify_AnchorOutcomes(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	// pkg/b.go exists at the pin and is unchanged at HEAD.
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	// pkg/a.go exists at the pin but its content changed after it.
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	// pkg/c.go exists at the pin and was removed after it: drift, not
	// missing — removal between the pin and HEAD is a change.
	insertPinnedLearning(t, s, "removed-1", map[string]any{
		"tag": "removed", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/c.go"},
	})
	// nope.go never existed: absent at the pin.
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	// A pin that names no commit in the repository is missing too.
	insertPinnedLearning(t, s, "unknownpin-1", map[string]any{
		"tag": "unknownpin", "as_of": "kb@" + strings.Repeat("0", 40), "evidence": []string{"kb:pkg/b.go"},
	})
	// Evidence but no pin: nothing is checkable, so no_anchors with the
	// ineligibility as context, never a fabricated positive.
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{
		"tag": "unpinned", "evidence": []string{"kb:pkg/b.go"},
	})

	// NewVerify over the real repository path is the production wiring:
	// the git-backed anchorChecker answers Head, Repo, and every anchor
	// question with local git.
	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{})
	requireVerifyPages(t, parsed, []string{
		"learning/absent-1",
		"learning/changed-1",
		"learning/removed-1",
		"learning/steady-1",
		"learning/unknownpin-1",
		"learning/unpinned-1",
	})

	for page, want := range map[string]string{
		"learning/steady-1":     verifyAnchorsOK,
		"learning/changed-1":    verifyDrifted,
		"learning/removed-1":    verifyDrifted,
		"learning/absent-1":     verifyMissing,
		"learning/unknownpin-1": verifyMissing,
		"learning/unpinned-1":   verifyNoAnchors,
	} {
		if got, _ := verifyEntryOf(t, parsed, page)["result"].(string); got != want {
			t.Errorf("%s: result = %q, want %q", page, got, want)
		}
	}

	// The anchors_ok entry carries the candidate facts KV-FR-002 requires:
	// the learning identity, the tier, and the bounded evidence locators.
	ok := verifyEntryOf(t, parsed, "learning/steady-1")
	if identity, _ := ok["identity"].(string); identity != "steady-1" {
		t.Errorf("steady-1 identity = %q, want steady-1", identity)
	}
	if tier, _ := ok["tier"].(string); tier != "draft" {
		t.Errorf("steady-1 tier = %q, want draft", tier)
	}
	requireStrings(t, "evidence", verifyStringList(t, "evidence", ok["evidence"]), []string{"kb:pkg/b.go"})

	// The no_anchors learning carries the ineligibility as its context.
	unpinned := verifyEntryOf(t, parsed, "learning/unpinned-1")
	if reason, _ := unpinned["reason"].(string); !strings.Contains(reason, "as_of") {
		t.Errorf("unpinned-1 reason = %q, want the missing-pin ineligibility context", reason)
	}

	for result, want := range map[string]int{
		verifyAnchorsOK: 1,
		verifyDrifted:   2,
		verifyMissing:   2,
		verifyNoAnchors: 1,
		verifySkipped:   0,
	} {
		if got := verifySummaryCount(t, parsed, result); got != want {
			t.Errorf("summary[%s] = %d, want %d", result, got, want)
		}
	}
}

// TestVerify_AnchorCheckerErrorsFailThePass pins KV-FR-004's failure
// routing when the anchor checker cannot answer a candidate's check: the
// whole pass fails — the result is an error result (IsError) with status
// failed — and every candidate entry carries result skipped with a reason
// naming the failed pass, so nothing checked before the failure can be
// presented as verified. The candidate's stored facts still ride along as
// context; only the verification outcome is withheld.
func TestVerify_AnchorCheckerErrorsFailThePass(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{
		"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	boom := errors.New("boom")

	for name, checker := range map[string]fakeAnchors{
		"commit": {commitErr: boom},
		"path":   {pathErr: boom},
		"diff":   {diffErr: boom},
	} {
		result, _, err := newTestVerify(s, checker).VerifyCandidates(context.Background(), nil, types.VerifyCandidatesInput{})
		if err != nil {
			t.Fatalf("%s: VerifyCandidates: %v", name, err)
		}
		if !result.IsError {
			t.Fatalf("%s: result = %q, want an error result", name, resultText(result))
		}
		parsed := parseLearningResult(t, resultText(result))
		if status, _ := parsed["status"].(string); status != verifyPassFailed {
			t.Errorf("%s: status = %q, want %q", name, status, verifyPassFailed)
		}
		if reason, _ := parsed["reason"].(string); !strings.Contains(reason, "boom") {
			t.Errorf("%s: reason = %q, want the checker failure", name, reason)
		}

		entries := verifyCandidateEntries(t, parsed)
		if len(entries) != 1 {
			t.Fatalf("%s: candidates = %v, want exactly gate-1", name, verifyCandidateNames(t, parsed))
		}
		e := entries[0]
		if got, _ := e["result"].(string); got != verifySkipped {
			t.Errorf("%s: gate-1 result = %q, want %q", name, got, verifySkipped)
		}
		if reason, _ := e["reason"].(string); !strings.Contains(reason, "verification pass failed") || !strings.Contains(reason, "boom") {
			t.Errorf("%s: gate-1 reason = %q, want the failed pass naming boom", name, reason)
		}
		// Stored facts are context, not outcomes: identity, tier, and
		// evidence are presented, but no source-level detail is.
		if identity, _ := e["identity"].(string); identity != "gate-1" {
			t.Errorf("%s: gate-1 identity = %q, want gate-1", name, identity)
		}
		if tier, _ := e["tier"].(string); tier != "draft" {
			t.Errorf("%s: gate-1 tier = %q, want draft", name, tier)
		}
		requireStrings(t, "evidence", verifyStringList(t, "evidence", e["evidence"]), []string{"kb:pkg/a.go"})

		for result, want := range map[string]int{
			verifyAnchorsOK: 0,
			verifyDrifted:   0,
			verifyMissing:   0,
			verifyNoAnchors: 0,
			verifySkipped:   1,
		} {
			if got := verifySummaryCount(t, parsed, result); got != want {
				t.Errorf("%s: summary[%s] = %d, want %d", name, result, got, want)
			}
		}
	}
}

// TestVerify_CompiledAggregatesSources pins KV-FR-003's compiled-article
// aggregation: an article with one anchors_ok and one drifted source is
// drifted, its presented evidence is the deduplicated union of its
// sources' locators, and its entry lists each source identity with that
// source's individual result so the reviewer can see which source drifted
// without opening every candidate by hand.
func TestVerify_CompiledAggregatesSources(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "changed-1"},
	})

	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{Pages: []string{"compiled/mixed"}})
	entry := verifyEntryOf(t, parsed, "compiled/mixed")
	if got, _ := entry["result"].(string); got != verifyDrifted {
		t.Errorf("compiled/mixed result = %q, want %q", got, verifyDrifted)
	}
	// Identity is not defined for a compiled candidate — the page name is
	// its identifier — so no identity may be fabricated.
	if _, ok := entry["identity"]; ok {
		t.Errorf("compiled entry carries identity %q; identity is not defined for compiled candidates", entry["identity"])
	}
	if tier, _ := entry["tier"].(string); tier != "draft" {
		t.Errorf("compiled/mixed tier = %q, want draft", tier)
	}
	// The union of the sources' locators, deduplicated, in first-seen
	// order across the declared sources.
	requireStrings(t, "evidence", verifyStringList(t, "evidence", entry["evidence"]), []string{"kb:pkg/b.go", "kb:pkg/a.go"})

	// Each source identity with its individual result, in declared order.
	sources, ok := entry["sources"].([]any)
	if !ok || len(sources) != 2 {
		t.Fatalf("sources = %v, want steady-1 and changed-1", entry["sources"])
	}
	identities := make([]string, 0, len(sources))
	results := make([]string, 0, len(sources))
	for _, src := range sources {
		m, _ := src.(map[string]any)
		id, _ := m["identity"].(string)
		res, _ := m["result"].(string)
		identities = append(identities, id)
		results = append(results, res)
	}
	requireStrings(t, "source identities", identities, []string{"steady-1", "changed-1"})
	requireStrings(t, "source results", results, []string{verifyAnchorsOK, verifyDrifted})

	// Both sources fit the reporting bound, so nothing was omitted.
	if _, ok := entry["sources_omitted"]; ok {
		t.Errorf("compiled entry reports sources_omitted = %v; both sources fit the reporting bound", entry["sources_omitted"])
	}
	// The summary counts candidates, not sources: one drifted candidate
	// and no anchors_ok — the aggregate never counts as a positive.
	if got := verifySummaryCount(t, parsed, verifyDrifted); got != 1 {
		t.Errorf("summary[drifted] = %d, want 1", got)
	}
	if got := verifySummaryCount(t, parsed, verifyAnchorsOK); got != 0 {
		t.Errorf("summary[anchors_ok] = %d, want 0: the aggregate is drifted and source results are not candidates", got)
	}
}

// TestVerify_CompiledUnresolvableSource pins the aggregation when a
// compiled article's sources include one that resolves to no stored
// learning: the unresolvable source contributes nothing — no per-source
// result, no evidence, no say in the aggregate — and never hides or
// poisons the sources that do resolve (KV-FR-003, design D3). A
// stored-but-ineligible source is reported as no_anchors with the
// ineligibility as context, never a fabricated positive (spec-review
// pinned advisory).
func TestVerify_CompiledUnresolvableSource(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	// Stored but not anchor-checkable: evidence, but no pin.
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{
		"tag": "unpinned", "evidence": []string{"kb:pkg/b.go"},
	})
	// nowhere-1 names no stored learning.
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "absent-1", "nowhere-1", "unpinned-1"},
	})

	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{Pages: []string{"compiled/mixed"}})
	entry := verifyEntryOf(t, parsed, "compiled/mixed")
	// missing dominates the aggregate; the unresolvable source has no say.
	if got, _ := entry["result"].(string); got != verifyMissing {
		t.Errorf("compiled/mixed result = %q, want %q", got, verifyMissing)
	}

	// Every resolved source is reported with its own result, in declared
	// order; the unresolvable source contributes no entry at all, so it
	// neither hides the others nor fabricates a result for itself.
	sources, ok := entry["sources"].([]any)
	if !ok || len(sources) != 3 {
		t.Fatalf("sources = %v, want steady-1, absent-1, and unpinned-1 only", entry["sources"])
	}
	identities := make([]string, 0, len(sources))
	for _, src := range sources {
		m, _ := src.(map[string]any)
		id, _ := m["identity"].(string)
		identities = append(identities, id)
	}
	requireStrings(t, "source identities", identities, []string{"steady-1", "absent-1", "unpinned-1"})

	if got, _ := verifySourceEntry(t, entry, "steady-1")["result"].(string); got != verifyAnchorsOK {
		t.Errorf("steady-1 source result = %q, want %q", got, verifyAnchorsOK)
	}
	if got, _ := verifySourceEntry(t, entry, "absent-1")["result"].(string); got != verifyMissing {
		t.Errorf("absent-1 source result = %q, want %q", got, verifyMissing)
	}
	// The stored-but-ineligible source: no_anchors with the ineligibility
	// as its context — never a fabricated positive.
	ineligible := verifySourceEntry(t, entry, "unpinned-1")
	if got, _ := ineligible["result"].(string); got != verifyNoAnchors {
		t.Errorf("unpinned-1 source result = %q, want %q", got, verifyNoAnchors)
	}
	if detail, _ := ineligible["detail"].(string); !strings.Contains(detail, "as_of") {
		t.Errorf("unpinned-1 source detail = %q, want the missing-pin ineligibility context", detail)
	}

	// The union of the resolved sources' locators: nowhere-1 contributed
	// nothing, and the duplicate kb:pkg/b.go appears once.
	requireStrings(t, "evidence", verifyStringList(t, "evidence", entry["evidence"]), []string{"kb:pkg/b.go", "kb:nope.go"})
	if got := verifySummaryCount(t, parsed, verifyMissing); got != 1 {
		t.Errorf("summary[missing] = %d, want 1", got)
	}
}

// TestVerify_CompiledNoResolvableSource pins the spec scenario for an
// article whose sources name no stored learning at all: nothing on it is
// checkable, so its result is no_anchors — not an error, and not a
// fabricated aggregate — with no per-source results and no evidence
// presented.
func TestVerify_CompiledNoResolvableSource(t *testing.T) {
	repo, _ := anchorRepo(t)
	s := newTestStore(t)
	insertVerifyPage(t, s, "compiled/orphan", "draft", map[string]any{
		"sources": []string{"nowhere-1", "nowhere-2"},
	})

	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{Pages: []string{"compiled/orphan"}})
	entry := verifyEntryOf(t, parsed, "compiled/orphan")
	if got, _ := entry["result"].(string); got != verifyNoAnchors {
		t.Errorf("compiled/orphan result = %q, want %q", got, verifyNoAnchors)
	}
	// No source resolved, so no per-source results, no evidence, and no
	// reporting-bound omission may appear.
	for _, key := range []string{"sources", "evidence", "sources_omitted"} {
		if _, ok := entry[key]; ok {
			t.Errorf("compiled/orphan carries %s = %v; no source resolved", key, entry[key])
		}
	}
	if got := verifySummaryCount(t, parsed, verifyNoAnchors); got != 1 {
		t.Errorf("summary[no_anchors] = %d, want 1", got)
	}
}

// --- Degraded and failure modes, determinism, non-mutation (tasks.md 3.3) ---
//
// The tests below pin KV-FR-004's degraded and failure routing and
// KV-FR-005's determinism and non-mutation guarantees. A vault that is not
// a git repository skips the pass — every candidate skipped with the
// ineligibility reason, and NOT an error result, so the promotion gate
// still runs. A git failure, a cancelled request, or an exhausted budget
// fails the pass: an error result (IsError) with status failed where every
// candidate — including ones checked before the failure — carries result
// skipped with a reason naming the failed pass, so a half-run pass can
// never be read as clean. Two runs over an unchanged store, pin, and HEAD
// return identical output, and a completed pass leaves every candidate's
// tier, validity, content, and files byte-identical.

// requireVerifyFailedPass asserts KV-FR-004's failure contract on one
// verify result: an error result (IsError) with pass status failed, a
// reason carrying want, every candidate entry skipped with a reason naming
// the failed pass, and summary counts reporting no anchors_ok, drifted, or
// missing — no candidate from a failed pass may be presented as verified.
func requireVerifyFailedPass(t *testing.T, result *mcp.CallToolResult, want string) map[string]any {
	t.Helper()
	if !result.IsError {
		t.Fatalf("result = %q, want an error result", resultText(result))
	}
	parsed := parseLearningResult(t, resultText(result))
	if status, _ := parsed["status"].(string); status != verifyPassFailed {
		t.Fatalf("status = %q, want %q", status, verifyPassFailed)
	}
	if reason, _ := parsed["reason"].(string); !strings.Contains(reason, want) {
		t.Errorf("reason = %q, want the failure %q", reason, want)
	}
	entries := verifyCandidateEntries(t, parsed)
	for _, e := range entries {
		page, _ := e["page"].(string)
		if got, _ := e["result"].(string); got != verifySkipped {
			t.Errorf("%s: result = %q, want %q", page, got, verifySkipped)
		}
		reason, _ := e["reason"].(string)
		if !strings.Contains(reason, "verification pass failed") || !strings.Contains(reason, want) {
			t.Errorf("%s: reason = %q, want the failed pass naming %q", page, reason, want)
		}
	}
	for kind, wantCount := range map[string]int{
		verifyAnchorsOK: 0,
		verifyDrifted:   0,
		verifyMissing:   0,
		verifyNoAnchors: 0,
		verifySkipped:   len(entries),
	} {
		if got := verifySummaryCount(t, parsed, kind); got != wantCount {
			t.Errorf("summary[%s] = %d, want %d", kind, got, wantCount)
		}
	}
	return parsed
}

// flakyAnchors is an anchorChecker that answers the first okCalls
// CommitExists questions and then fails every later one with err, so a
// verification pass fails mid-queue with earlier candidates genuinely
// checked. The anchorChecker seam is the only deterministic way to fail
// the git layer (KV-FR-004): a real repository answers every question.
type flakyAnchors struct {
	fakeAnchors
	okCalls int
	calls   int
	err     error
}

// CommitExists answers the first okCalls calls, then fails with err.
func (f *flakyAnchors) CommitExists(context.Context, string) (bool, error) {
	if f.calls >= f.okCalls {
		return false, f.err
	}
	f.calls++
	return true, nil
}

// stallingAnchors is an anchorChecker that answers the first okCalls
// CommitExists questions and then stalls until its context ends, so a pass
// whose budget is too small to finish the queue expires mid-queue with
// earlier candidates genuinely checked — the budget, not a scripted error,
// is what stops the pass.
type stallingAnchors struct {
	fakeAnchors
	okCalls int
	calls   int
}

// CommitExists answers the first okCalls calls, then waits for ctx.
func (f *stallingAnchors) CommitExists(ctx context.Context, _ string) (bool, error) {
	if f.calls >= f.okCalls {
		<-ctx.Done()
		return false, ctx.Err()
	}
	f.calls++
	return true, nil
}

// TestVerify_NonRepositoryVaultSkipsPass pins KV-FR-004's degraded mode: a
// vault path that is not a git repository skips the whole pass — status
// skipped with the ineligibility reason, every candidate skipped carrying
// that reason, and NOT an error result, so the promotion gate still runs
// and the reviewer sees an unverified queue rather than a clean one.
func TestVerify_NonRepositoryVaultSkipsPass(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{
		"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "team"})

	for _, tc := range []struct {
		name    string
		v       *Verify
		reason  string
		needGit bool
	}{
		{
			// NewVerify over a plain directory is the production wiring:
			// the git-backed checker runs rev-parse, git exits non-zero,
			// and the pass is skipped, not failed.
			name:    "not a git repository",
			v:       NewVerify(s, t.TempDir()),
			reason:  errAnchorNoRepo.Error(),
			needGit: true,
		},
		{
			// An empty vault path constructs no checker at all.
			name:   "no vault path",
			v:      NewVerify(s, ""),
			reason: "no vault path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needGit {
				if _, err := exec.LookPath("git"); err != nil {
					t.Skip("git not installed")
				}
			}
			result, _, err := tc.v.VerifyCandidates(context.Background(), nil, types.VerifyCandidatesInput{})
			if err != nil {
				t.Fatalf("VerifyCandidates: %v", err)
			}
			if result.IsError {
				t.Fatalf("result = %q, want a skipped pass, not an error result", resultText(result))
			}
			parsed := parseLearningResult(t, resultText(result))
			if status, _ := parsed["status"].(string); status != verifyPassSkipped {
				t.Errorf("status = %q, want %q", status, verifyPassSkipped)
			}
			if reason, _ := parsed["reason"].(string); reason != tc.reason {
				t.Errorf("reason = %q, want %q", reason, tc.reason)
			}

			// Every candidate is skipped carrying the pass reason, and no
			// positive outcome may appear anywhere (no false clean).
			entries := verifyCandidateEntries(t, parsed)
			if len(entries) != 2 {
				t.Fatalf("candidates = %v, want the two stored candidates", verifyCandidateNames(t, parsed))
			}
			for _, e := range entries {
				page, _ := e["page"].(string)
				if got, _ := e["result"].(string); got != verifySkipped {
					t.Errorf("%s: result = %q, want %q", page, got, verifySkipped)
				}
				if reason, _ := e["reason"].(string); reason != tc.reason {
					t.Errorf("%s: reason = %q, want the pass reason %q", page, reason, tc.reason)
				}
			}
			for kind, want := range map[string]int{
				verifyAnchorsOK: 0,
				verifyDrifted:   0,
				verifyMissing:   0,
				verifyNoAnchors: 0,
				verifySkipped:   2,
			} {
				if got := verifySummaryCount(t, parsed, kind); got != want {
					t.Errorf("summary[%s] = %d, want %d", kind, got, want)
				}
			}

			// Stored facts still ride along as context: the learning keeps
			// its identity and tier even though nothing was checked.
			gate := verifyEntryOf(t, parsed, "learning/gate-1")
			if identity, _ := gate["identity"].(string); identity != "gate-1" {
				t.Errorf("gate-1 identity = %q, want gate-1", identity)
			}
			if tier, _ := gate["tier"].(string); tier != "draft" {
				t.Errorf("gate-1 tier = %q, want draft", tier)
			}
		})
	}
}

// TestVerify_GitFailureMidPassWithholdsOutcomes pins KV-FR-004's
// no-false-clean contract when git fails mid-pass: the result is failed and
// an error result, and every candidate — including one checked, and
// would-be anchors_ok, before the failure — carries result skipped with a
// reason naming the failed pass. The pre-failure outcome is never reported
// as the candidate's result; the candidate's stored facts ride along as
// context only. An enumeration-skipped candidate keeps its own reason
// ahead of the pass failure.
func TestVerify_GitFailureMidPassWithholdsOutcomes(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "early-1", map[string]any{
		"tag": "early", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	insertPinnedLearning(t, s, "late-1", map[string]any{
		"tag": "late", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	input := types.VerifyCandidatesInput{Pages: []string{
		"learning/early-1", "learning/late-1", "learning/vanished-1",
	}}

	// Control: an answering checker verifies both learnings — early-1
	// first in queue order — and reports vanished-1 as an enumeration
	// skip. This proves early-1 is genuinely checkable ahead of late-1,
	// so the failing pass below really does check it before failing.
	control := runVerifyChecked(t, newTestVerify(s, fakeAnchors{}), input)
	for page, want := range map[string]string{
		"learning/early-1": verifyAnchorsOK,
		"learning/late-1":  verifyAnchorsOK,
	} {
		if got, _ := verifyEntryOf(t, control, page)["result"].(string); got != want {
			t.Errorf("control: %s result = %q, want %q", page, got, want)
		}
	}

	// The failing pass: early-1 is checked — and would be anchors_ok —
	// before late-1's CommitExists fails.
	boom := errors.New("git unreachable")
	result, _, err := newTestVerify(s, &flakyAnchors{okCalls: 1, err: boom}).VerifyCandidates(
		context.Background(), nil, input)
	if err != nil {
		t.Fatalf("VerifyCandidates: %v", err)
	}
	parsed := requireVerifyFailedPass(t, result, "git unreachable")

	// early-1's pre-failure anchors_ok is withheld: it is skipped with the
	// pass-failure reason, never presented as verified...
	early := verifyEntryOf(t, parsed, "learning/early-1")
	if got, _ := early["result"].(string); got != verifySkipped {
		t.Errorf("early-1 result = %q, want %q: a candidate checked before the failure must not be presented as verified", got, verifySkipped)
	}
	// ...but its stored facts ride along as context, not outcomes.
	if identity, _ := early["identity"].(string); identity != "early-1" {
		t.Errorf("early-1 identity = %q, want early-1", identity)
	}
	if tier, _ := early["tier"].(string); tier != "draft" {
		t.Errorf("early-1 tier = %q, want draft", tier)
	}
	requireStrings(t, "evidence", verifyStringList(t, "evidence", early["evidence"]), []string{"kb:pkg/a.go"})

	// The enumeration-skipped candidate keeps its own reason ahead of the
	// pass failure, so a vanished page stays distinguishable (KV-FR-002).
	vanished := verifyEntryOf(t, parsed, "learning/vanished-1")
	reason, _ := vanished["reason"].(string)
	if !strings.Contains(reason, "no stored page named") || !strings.Contains(reason, "verification pass failed") {
		t.Errorf("vanished-1 reason = %q, want its enumeration reason ahead of the failed pass", reason)
	}
}

// TestVerify_CancelledContextFailsPass pins KV-FR-004's cancellation
// routing: a request context already cancelled when the tool runs stops
// the pass, which fails — an error result with status failed — so the
// cancelled run can never be read as a clean one. The production git
// checker honors the context in its invocation, and the pass loop honors
// it between candidates even under a checker that ignores its context.
func TestVerify_CancelledContextFailsPass(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "gate-1", map[string]any{
		"tag": "gate", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("git invocation", func(t *testing.T) {
		repo, _ := anchorRepo(t)
		result, _, err := NewVerify(s, repo).VerifyCandidates(ctx, nil, types.VerifyCandidatesInput{})
		if err != nil {
			t.Fatalf("VerifyCandidates: %v", err)
		}
		requireVerifyFailedPass(t, result, "context canceled")
	})

	t.Run("pass loop", func(t *testing.T) {
		result, _, err := newTestVerify(s, fakeAnchors{}).VerifyCandidates(ctx, nil, types.VerifyCandidatesInput{})
		if err != nil {
			t.Fatalf("VerifyCandidates: %v", err)
		}
		requireVerifyFailedPass(t, result, "context canceled")
	})
}

// TestVerify_ExhaustedBudgetFailsPass pins KV-FR-004's budget routing: a
// pass budget (the anchorBudget seam) too small to complete the queue
// stops the pass, which fails — an error result with status failed and
// every candidate skipped — and the call is bounded, so an unfinishable
// pass can neither hang the gate nor read as clean.
func TestVerify_ExhaustedBudgetFailsPass(t *testing.T) {
	s := newTestStore(t)
	insertPinnedLearning(t, s, "early-1", map[string]any{
		"tag": "early", "as_of": "kb@abc", "evidence": []string{"kb:pkg/a.go"},
	})
	insertPinnedLearning(t, s, "late-1", map[string]any{
		"tag": "late", "as_of": "kb@abc", "evidence": []string{"kb:pkg/b.go"},
	})

	// early-1 is checked before late-1's question stalls, so the queue has
	// genuinely started when the budget runs out; the stalling call parks
	// until the pass context ends, making the budget the cause of the stop.
	v := newTestVerify(s, &stallingAnchors{okCalls: 1})
	v.anchorBudget = 10 * time.Millisecond

	type verifyRun struct {
		result *mcp.CallToolResult
		err    error
	}
	done := make(chan verifyRun, 1)
	go func() {
		result, _, err := v.VerifyCandidates(context.Background(), nil, types.VerifyCandidatesInput{})
		done <- verifyRun{result: result, err: err}
	}()

	var run verifyRun
	select {
	case run = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("verification pass was not bounded by its budget")
	}
	if run.err != nil {
		t.Fatalf("VerifyCandidates: %v", run.err)
	}
	requireVerifyFailedPass(t, run.result, "context deadline exceeded")
}

// TestVerify_DeterministicRepeat pins KV-FR-005's determinism guarantee:
// two runs over the same store content, vault pin, and HEAD that each
// complete a checked pass return the same candidates in the same order
// with the same results — here compared as the marshaled result text, so
// ordering, fields, and counts must all be byte-identical.
func TestVerify_DeterministicRepeat(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	// A queue exercising every learning outcome plus compiled aggregation
	// and a knowledge page, so the compared payload covers the full shape.
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{
		"tag": "unpinned", "evidence": []string{"kb:pkg/b.go"},
	})
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "changed-1"},
	})
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "team"})

	v := NewVerify(s, repo)
	run := func() string {
		result, _, err := v.VerifyCandidates(context.Background(), nil, types.VerifyCandidatesInput{})
		if err != nil || result.IsError {
			t.Fatalf("VerifyCandidates = %v, %v", resultText(result), err)
		}
		text := resultText(result)
		if status, _ := parseLearningResult(t, text)["status"].(string); status != verifyPassChecked {
			t.Fatalf("status = %q, want %q: %s", status, verifyPassChecked, text)
		}
		return text
	}
	if firstRun, secondRun := run(), run(); firstRun != secondRun {
		t.Errorf("two runs over the same store, pin, and HEAD differ:\nfirst:\n%s\nsecond:\n%s", firstRun, secondRun)
	}
}

// verifyStoreSnapshot renders every named page's stored fields and block
// contents as one byte-comparable string: the dimensions KV-FR-005
// protects are tier, validity (inside the properties JSON), content, and
// identity.
func verifyStoreSnapshot(t *testing.T, s *store.Store, names []string) string {
	t.Helper()
	var b strings.Builder
	for _, name := range names {
		page, err := s.GetPage(name)
		if err != nil || page == nil {
			t.Fatalf("GetPage(%q) = %v, %v", name, page, err)
		}
		fmt.Fprintf(&b, "page %s\n  tier %s\n  original %s\n  source %s/%s\n  properties %s\n  hash %s\n",
			page.Name, page.Tier, page.OriginalName, page.SourceID, page.SourceDocID, page.Properties, page.ContentHash)
		blocks, err := s.GetBlocksByPage(name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", name, err)
		}
		for _, blk := range blocks {
			fmt.Fprintf(&b, "  block %s pos %d level %d %q\n", blk.UUID, blk.Position, blk.HeadingLevel, blk.Content)
		}
	}
	return b.String()
}

// verifyVaultSnapshot captures every file under dir as path → content,
// skipping .git (git's own state, not vault content the tool could write).
// Verification writes nothing (KV-FR-005), so the snapshot must be stable
// across a pass.
func verifyVaultSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

// requireSameVault asserts two vault snapshots are identical, reporting
// created, removed, and changed files.
func requireSameVault(t *testing.T, before, after map[string]string) {
	t.Helper()
	for path, content := range before {
		got, ok := after[path]
		if !ok {
			t.Errorf("verification removed vault file %s", path)
			continue
		}
		if got != content {
			t.Errorf("verification changed vault file %s: %q -> %q", path, content, got)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("verification created vault file %s", path)
		}
	}
}

// TestVerify_NonMutation pins KV-FR-005's read-only guarantee at the
// candidate level: a completed pass — including candidates whose outcomes
// a mutating flow would act on (drifted and missing learnings, which
// lint --fix would mark stale-unverified) — leaves every candidate's tier,
// validity, content, and properties byte-identical and writes no file into
// the vault, so verification can neither advance nor pre-select a
// promotion.
func TestVerify_NonMutation(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "changed-1"},
	})
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "team"})
	// One block, so stored content is part of the compared snapshot.
	if err := s.InsertBlock(&store.Block{
		UUID:     "steady-block",
		PageName: store.LearningPrefix + "steady-1",
		Content:  "steady body",
		Position: 0,
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	names := []string{
		"learning/steady-1", "learning/changed-1", "learning/absent-1",
		"compiled/mixed", "knowledge/team/file",
	}
	beforeStore := verifyStoreSnapshot(t, s, names)
	beforeVault := verifyVaultSnapshot(t, repo)

	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{})
	// The pass really produced the acting-worthy outcomes — drifted and
	// missing alongside anchors_ok — so the non-mutation claim below is
	// not vacuous: these are exactly the outcomes a mutating flow acts on.
	for page, want := range map[string]string{
		"learning/steady-1":  verifyAnchorsOK,
		"learning/changed-1": verifyDrifted,
		"learning/absent-1":  verifyMissing,
	} {
		if got, _ := verifyEntryOf(t, parsed, page)["result"].(string); got != want {
			t.Errorf("%s: result = %q, want %q", page, got, want)
		}
	}

	if after := verifyStoreSnapshot(t, s, names); after != beforeStore {
		t.Errorf("verification changed the store:\nbefore:\n%s\nafter:\n%s", beforeStore, after)
	}
	requireSameVault(t, beforeVault, verifyVaultSnapshot(t, repo))

	// Explicit field checks for readable failures: tier and validity are
	// exactly what a promotion or a staleness fix would change.
	page, props := pageProps(t, s, "learning/changed-1")
	if page.Tier != "draft" {
		t.Errorf("changed-1 tier = %q, want draft", page.Tier)
	}
	if props["validity"] != "current" {
		t.Errorf("changed-1 validity = %v, want current", props["validity"])
	}
}

// --- Golden checked output shape (tasks.md 3.5) ---
//
// DE-007 layer 2: the documented output shape of a completed pass, pinned as
// one golden JSON document over a representative mixed queue — an anchors_ok
// learning, a drifted one, a missing one, a compiled article aggregating
// three sources, and an enumeration skip. The golden is the JSON a caller
// parses, never the internal Go types, and it spells the wire values
// literally — the enum strings, the key spellings, the message — so renaming
// a constant's value or reshaping a field cannot pass silently. The summary
// counts candidates and never sources (the semantics tasks.md 3.2's tests
// established): the compiled aggregate counts once as its own result, the
// per-source results count not at all, and no_anchors stays zero even though
// one source carries it.

// verifyCheckedGolden is the exact observable payload of the golden checked
// pass in TestVerify_CheckedOutputShapeGolden. A checked pass carries no
// top-level reason — reason is the not-checked field — and identity appears
// on learning candidates only: the compiled article and the unresolved skip
// have none, because presenting one there would invent facts the store does
// not hold.
const verifyCheckedGolden = `{
  "status": "checked",
  "candidates": [
    {
      "page": "compiled/mixed",
      "tier": "draft",
      "evidence": ["kb:pkg/b.go", "kb:pkg/a.go"],
      "result": "drifted",
      "sources": [
        {"identity": "steady-1", "result": "anchors_ok"},
        {"identity": "changed-1", "result": "drifted"},
        {"identity": "unpinned-1", "result": "no_anchors", "detail": "no readable as_of pin"}
      ]
    },
    {
      "page": "learning/absent-1",
      "identity": "absent-1",
      "tier": "draft",
      "evidence": ["kb:nope.go"],
      "result": "missing"
    },
    {
      "page": "learning/changed-1",
      "identity": "changed-1",
      "tier": "draft",
      "evidence": ["kb:pkg/a.go"],
      "result": "drifted"
    },
    {
      "page": "learning/steady-1",
      "identity": "steady-1",
      "tier": "draft",
      "evidence": ["kb:pkg/b.go"],
      "result": "anchors_ok"
    },
    {
      "page": "learning/vanished-1",
      "result": "skipped",
      "reason": "no stored page named \"learning/vanished-1\""
    }
  ],
  "summary": {
    "anchors_ok": 1,
    "drifted": 2,
    "missing": 1,
    "no_anchors": 0,
    "skipped": 1
  },
  "message": "Verification pass completed; 5 candidate(s) reported. anchors_ok states only that cited paths exist at the pin and are unchanged at HEAD — never that a claim is correct, worth acting on, or safe to trust. Nothing was promoted or modified."
}`

// sortedKeys returns m's keys in sorted order, the canonical order for a
// field-set fingerprint.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// verifyResultShape renders the field-set fingerprint of a parsed verify
// result: the sorted key set at every level — the top level, each candidate
// entry, each per-source result, and the summary — anchored by queue
// position, page name, and source identity so the sets compare positionally.
// Values are deliberately excluded: this renders only the shape a caller's
// parser sees, asserted stable across runs by TestVerify_CheckedOutputShapeGolden
// while TestVerify_DeterministicRepeat compares the full values.
func verifyResultShape(t *testing.T, parsed map[string]any) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "top-level: %s\n", strings.Join(sortedKeys(parsed), ","))
	for i, e := range verifyCandidateEntries(t, parsed) {
		page, _ := e["page"].(string)
		fmt.Fprintf(&b, "candidate[%d] %s: %s\n", i, page, strings.Join(sortedKeys(e), ","))
		if sources, ok := e["sources"].([]any); ok {
			for j, src := range sources {
				m, ok := src.(map[string]any)
				if !ok {
					t.Fatalf("candidate[%d] source[%d] is not an object: %v", i, j, src)
				}
				identity, _ := m["identity"].(string)
				fmt.Fprintf(&b, "  source[%d] %s: %s\n", j, identity, strings.Join(sortedKeys(m), ","))
			}
		}
	}
	if summary, ok := parsed["summary"].(map[string]any); ok {
		fmt.Fprintf(&b, "summary: %s\n", strings.Join(sortedKeys(summary), ","))
	} else {
		b.WriteString("summary: absent\n")
	}
	return b.String()
}

// TestVerify_CheckedOutputShapeGolden pins DE-007 layer 2: the documented
// output shape of a completed verification pass, as the JSON a caller
// parses. One representative checked pass over a mixed queue — anchors_ok,
// drifted, and missing learnings, a compiled article aggregating three
// sources (one of them stored but ineligible, so the entry's detail field
// appears), and an enumeration skip for a requested page that matches
// nothing — must equal the golden document exactly: the pass status and its
// value, every per-candidate field (page, identity on learnings only, tier,
// presented evidence, the fixed result enum, the skip reason, the per-source
// results), and the summary counts, which tally candidates and never
// sources. A second run over the same store, pin, and HEAD must reproduce
// the same field set at every level — the shape-level determinism claim that
// complements TestVerify_DeterministicRepeat's full-value comparison.
func TestVerify_CheckedOutputShapeGolden(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	// pkg/b.go exists at the pin and is unchanged at HEAD.
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	// pkg/a.go exists at the pin but its content changed after it.
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	// nope.go never existed: absent at the pin.
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	// Stored but never requested: unpinned-1 rides the golden queue only as
	// compiled/mixed's third source — the stored-but-ineligible one whose
	// no_anchors result carries its detail, the shape's only source detail.
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{
		"tag": "unpinned", "evidence": []string{"kb:pkg/b.go"},
	})
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "changed-1", "unpinned-1"},
	})

	// The pages selector in queue order (enumeration sorts by name anyway);
	// learning/vanished-1 matches no stored page, so it is the
	// enumeration-skip entry only the pages selector can produce.
	input := types.VerifyCandidatesInput{Pages: []string{
		"compiled/mixed", "learning/absent-1", "learning/changed-1",
		"learning/steady-1", "learning/vanished-1",
	}}
	v := NewVerify(s, repo)
	parsed := runVerifyChecked(t, v, input)

	// The golden comparison: the parsed payload must equal the golden
	// document structurally and value-for-value. Both sides are decoded
	// JSON — what a caller parses — never the internal Go types, so a
	// missing field, an extra fabrication, or a changed value all fail here.
	var want any
	if err := json.Unmarshal([]byte(verifyCheckedGolden), &want); err != nil {
		t.Fatalf("golden fixture is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(parsed, want) {
		got, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			t.Fatalf("marshal parsed result: %v", err)
		}
		t.Errorf("checked result drifted from the golden shape:\ngot:\n%s\nwant:\n%s", got, verifyCheckedGolden)
	}

	// Shape stability: the same input must produce the same field set at
	// every level on every completed pass. The values are 3.3's determinism
	// claim; the field set is this test's.
	second := runVerifyChecked(t, v, input)
	if firstShape, secondShape := verifyResultShape(t, parsed), verifyResultShape(t, second); firstShape != secondShape {
		t.Errorf("field set is not stable across runs:\nfirst:\n%s\nsecond:\n%s", firstShape, secondShape)
	}
}

// --- Full-store round-trip non-mutation audit (tasks.md 3.6) ---
//
// DE-007 layer 3: the store-level round trip. TestVerify_NonMutation pins
// the candidate-level contract — the enumerated candidates' tiers,
// validity, content, and files — while this audit pins the broader one: a
// verification pass over the whole default queue leaves the complete store
// state byte-identical, pages the queue never enumerated included. The
// audited population carries the queue's skips on purpose — a validated
// learning and a curated knowledge page are not draft, and a draft page
// outside learning/, compiled/, and knowledge/ is out of namespace — so a
// mutation confined to a non-candidate would still fail the comparison.
// That is exactly what "verification is not endorsement" needs: no tier
// advances, no property is touched, no content is rewritten, anywhere in
// the store. Both pass outcomes are audited — a checked pass through the
// real git-backed wiring, and a failed pass under a checker that cannot
// answer, so the failure path's reads and error routing are covered too.

// verifyFullStoreSnapshot renders the complete store state as one
// byte-comparable JSON document: every page the store holds with all its
// fields — tier and properties JSON included — every block under it with
// content, UUID, parent, heading level, and position, and the store's own
// page and block counts. Unlike verifyStoreSnapshot, which renders the
// named candidates only (tasks.md 3.3), this walks the store's full read
// surface — ListPages plus GetBlocksByPage per page — so the audit in
// TestVerify_StoreRoundTripNonMutation compares the whole store, not just
// the enumerated candidates. Blocks are ordered by position then UUID so
// equal positions cannot make the rendering depend on SQLite's tie order:
// the audit compares block content, not physical row order.
func verifyFullStoreSnapshot(t *testing.T, s *store.Store) string {
	t.Helper()
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	type snapshotBlock struct {
		UUID         string
		PageName     string
		ParentValid  bool
		ParentUUID   string
		Content      string
		HeadingLevel int
		Position     int
	}
	type snapshotPage struct {
		Name         string
		OriginalName string
		SourceID     string
		SourceDocID  string
		Properties   string
		ContentHash  string
		IsJournal    bool
		CreatedAt    int64
		UpdatedAt    int64
		Tier         string
		Category     string
		Blocks       []snapshotBlock
	}
	doc := struct {
		Pages      []snapshotPage
		PageCount  int
		BlockCount int
	}{Pages: make([]snapshotPage, 0, len(pages))}
	for _, p := range pages {
		blocks, err := s.GetBlocksByPage(p.Name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", p.Name, err)
		}
		sp := snapshotPage{
			Name:         p.Name,
			OriginalName: p.OriginalName,
			SourceID:     p.SourceID,
			SourceDocID:  p.SourceDocID,
			Properties:   p.Properties,
			ContentHash:  p.ContentHash,
			IsJournal:    p.IsJournal,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
			Tier:         p.Tier,
			Category:     p.Category,
			Blocks:       make([]snapshotBlock, 0, len(blocks)),
		}
		for _, b := range blocks {
			sp.Blocks = append(sp.Blocks, snapshotBlock{
				UUID:         b.UUID,
				PageName:     b.PageName,
				ParentValid:  b.ParentUUID.Valid,
				ParentUUID:   b.ParentUUID.String,
				Content:      b.Content,
				HeadingLevel: b.HeadingLevel,
				Position:     b.Position,
			})
		}
		sort.Slice(sp.Blocks, func(i, j int) bool {
			if sp.Blocks[i].Position != sp.Blocks[j].Position {
				return sp.Blocks[i].Position < sp.Blocks[j].Position
			}
			return sp.Blocks[i].UUID < sp.Blocks[j].UUID
		})
		doc.Pages = append(doc.Pages, sp)
	}
	if doc.PageCount, err = s.CountPages(); err != nil {
		t.Fatalf("CountPages: %v", err)
	}
	if doc.BlockCount, err = s.CountBlocks(); err != nil {
		t.Fatalf("CountBlocks: %v", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal store snapshot: %v", err)
	}
	return string(data)
}

// TestVerify_StoreRoundTripNonMutation pins DE-007 layer 3: verification
// leaves the store byte-identical, at full-store scope and on both pass
// outcomes. The population is a representative mixed one — draft learnings
// across every outcome class, a compiled article aggregating two sources,
// and a draft knowledge page forming the default queue, plus a validated
// learning, a curated knowledge page, and an out-of-namespace draft page
// that the queue must skip, all carrying stored blocks — and the audit
// marshals the complete store (pages, blocks, properties, tiers, counts)
// before and after a checked pass over the whole default queue, and again
// after a failed pass under a checker that cannot answer, comparing each
// rendering byte for byte. The queue and outcome assertions ahead of each
// comparison keep the audit non-vacuous: the passes really did enumerate
// every candidate and produce the acting-worthy outcomes — drifted and
// missing alongside anchors_ok — a mutating flow would act on.
func TestVerify_StoreRoundTripNonMutation(t *testing.T) {
	repo, first := anchorRepo(t)
	s := newTestStore(t)
	// The default queue: draft learnings across the outcome classes —
	// steady-1 anchors_ok, changed-1 drifted, absent-1 missing, unpinned-1
	// no_anchors — a compiled article aggregating two of them, and a draft
	// knowledge page, which carries neither a pin nor sources and
	// verifies as no_anchors.
	insertPinnedLearning(t, s, "steady-1", map[string]any{
		"tag": "steady", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/b.go"},
	})
	insertPinnedLearning(t, s, "changed-1", map[string]any{
		"tag": "changed", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:pkg/a.go"},
	})
	insertPinnedLearning(t, s, "absent-1", map[string]any{
		"tag": "absent", "validity": "current", "as_of": "kb@" + first, "evidence": []string{"kb:nope.go"},
	})
	insertPinnedLearning(t, s, "unpinned-1", map[string]any{
		"tag": "unpinned", "evidence": []string{"kb:pkg/b.go"},
	})
	insertVerifyPage(t, s, "compiled/mixed", "draft", map[string]any{
		"sources": []string{"steady-1", "changed-1"},
	})
	insertVerifyPage(t, s, "knowledge/team/file", "draft", map[string]any{"tag": "team"})
	// Pages the default queue must skip: a validated learning and a
	// curated knowledge page are not draft, and notes/elsewhere sits
	// outside the three namespaces. The audit still covers them — the
	// contract is the whole store, not the enumerated candidates.
	insertVerifyPage(t, s, "learning/validated-1", "validated", nil)
	insertVerifyPage(t, s, "knowledge/team/curated-thing", "curated", map[string]any{"tag": "team"})
	insertVerifyPage(t, s, "notes/elsewhere", "draft", nil)

	// Stored blocks across the population — candidates and skipped pages
	// both, one nested under a parent so parent UUIDs are audited too.
	// parent_uuid references blocks(uuid), so the parent precedes the
	// child in the insertion order.
	for _, b := range []store.Block{
		{UUID: "steady-block", PageName: store.LearningPrefix + "steady-1", Content: "steady body", Position: 0},
		{UUID: "changed-block", PageName: store.LearningPrefix + "changed-1", Content: "changed body", Position: 0},
		{UUID: "compiled-block", PageName: store.CompiledPrefix + "mixed", Content: "compiled body", Position: 0},
		{UUID: "knowledge-parent", PageName: "knowledge/team/file", Content: "knowledge body", HeadingLevel: 1, Position: 0},
		{UUID: "knowledge-child", PageName: "knowledge/team/file", Content: "nested body", ParentUUID: sql.NullString{String: "knowledge-parent", Valid: true}, Position: 1},
		{UUID: "validated-block", PageName: store.LearningPrefix + "validated-1", Content: "validated body", Position: 0},
		{UUID: "curated-block", PageName: "knowledge/team/curated-thing", Content: "curated body", Position: 0},
		{UUID: "notes-block", PageName: "notes/elsewhere", Content: "notes body", Position: 0},
	} {
		if err := s.InsertBlock(&b); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.UUID, err)
		}
	}

	queue := []string{
		"compiled/mixed", "knowledge/team/file", "learning/absent-1",
		"learning/changed-1", "learning/steady-1", "learning/unpinned-1",
	}
	before := verifyFullStoreSnapshot(t, s)

	// The checked pass through the production git wiring over the whole
	// default queue: the enumerated candidates are exactly the six draft
	// pages in scope — the validated, curated, and out-of-namespace pages
	// never join — and every outcome class is present.
	parsed := runVerifyChecked(t, NewVerify(s, repo), types.VerifyCandidatesInput{})
	requireVerifyPages(t, parsed, queue)
	for page, want := range map[string]string{
		"learning/steady-1":   verifyAnchorsOK,
		"learning/changed-1":  verifyDrifted,
		"learning/absent-1":   verifyMissing,
		"learning/unpinned-1": verifyNoAnchors,
		"compiled/mixed":      verifyDrifted,
		"knowledge/team/file": verifyNoAnchors,
	} {
		if got, _ := verifyEntryOf(t, parsed, page)["result"].(string); got != want {
			t.Errorf("%s: result = %q, want %q", page, got, want)
		}
	}
	if after := verifyFullStoreSnapshot(t, s); after != before {
		t.Errorf("checked pass changed the store:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// The failed pass: the same default queue under a checker that cannot
	// answer, so the failure path is audited too. The pass presents the
	// same six candidates — every one skipped, none presented as verified
	// — and the store is still byte-identical to before either pass ran.
	boom := errors.New("anchor checker unreachable")
	result, _, err := newTestVerify(s, fakeAnchors{commitErr: boom}).VerifyCandidates(
		context.Background(), nil, types.VerifyCandidatesInput{})
	if err != nil {
		t.Fatalf("VerifyCandidates: %v", err)
	}
	failed := requireVerifyFailedPass(t, result, boom.Error())
	requireVerifyPages(t, failed, queue)
	if after := verifyFullStoreSnapshot(t, s); after != before {
		t.Errorf("failed pass changed the store:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
