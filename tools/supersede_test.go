package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// newSupersedeTool builds a Learning tool over a fresh test store and a
// temp vault directory whose learnings backups land under
// {vault}/.uf/dewey/learnings/.
func newSupersedeTool(t *testing.T) (*Learning, *store.Store, string) {
	t.Helper()
	s := newTestStore(t)
	vaultDir := t.TempDir()
	return NewLearning(nil, s, vaultDir, 0), s, vaultDir
}

// backupContent renders a minimal learning backup with the given
// frontmatter keys, for seeding via writeLearningBackup.
func backupContent(keys [][2]string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	for _, kv := range keys {
		sb.WriteString(kv[0] + ": " + kv[1] + "\n")
	}
	sb.WriteString("---\n\nbody\n")
	return sb.String()
}

// readStoredProps decodes a learning page's properties from the store.
func readStoredProps(t *testing.T, s *store.Store, identity string) map[string]any {
	t.Helper()
	page, err := s.GetPage("learning/" + identity)
	if err != nil {
		t.Fatalf("GetPage(%s): %v", identity, err)
	}
	if page == nil {
		return nil
	}
	props := map[string]any{}
	if page.Properties != "" {
		if err := json.Unmarshal([]byte(page.Properties), &props); err != nil {
			t.Fatalf("decode props on %s: %v", identity, err)
		}
	}
	return props
}

// overwriteLearningProps replaces a seeded learning's store properties,
// for tests that need pre-declared link state on an existing page.
func overwriteLearningProps(t *testing.T, s *store.Store, identity, properties string) {
	t.Helper()
	page, err := s.GetPage("learning/" + identity)
	if err != nil || page == nil {
		t.Fatalf("GetPage(%s): %v (page=%v)", identity, err, page)
	}
	page.Properties = properties
	if err := s.UpdatePage(page); err != nil {
		t.Fatalf("UpdatePage(%s): %v", identity, err)
	}
}

// readBackup reads a learning's markdown backup, or "" when absent.
func readBackup(t *testing.T, vaultDir, identity string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultDir, ".uf", "dewey", "learnings", identity+".md"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read backup %s: %v", identity, err)
	}
	return string(data)
}

// seedSupersedeLearnings stores clean learnings a (older) and b (newer)
// with backups, returning their identities.
func seedSupersedeLearnings(t *testing.T, l *Learning, s *store.Store, vaultDir string) (string, string) {
	t.Helper()
	const a = "auth-20260315T100001-test"
	const b = "auth-20260316T100002-test"
	storeLearningWithProps(t, s, a, `{"tag":"auth","created_at":"2026-03-15T10:00:01Z"}`, "older learning", time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, b, `{"tag":"auth","created_at":"2026-03-16T10:00:02Z"}`, "newer learning", time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC))
	writeLearningBackup(t, vaultDir, a, backupContent([][2]string{{"tag", "auth"}, {"created_at", "2026-03-15T10:00:01Z"}}))
	writeLearningBackup(t, vaultDir, b, backupContent([][2]string{{"tag", "auth"}, {"created_at", "2026-03-16T10:00:02Z"}}))
	return a, b
}

// callSupersede invokes SupersedeLearning and returns the result.
func callSupersede(t *testing.T, l *Learning, successor, target string) *mcp.CallToolResult {
	t.Helper()
	result, _, err := l.SupersedeLearning(context.Background(), nil, types.SupersedeLearningInput{Successor: successor, Target: target})
	if err != nil {
		t.Fatalf("SupersedeLearning: %v", err)
	}
	return result
}

// TestSupersedeLearning_Confirmation covers the ES-FR-007 confirm
// scenario: both sides record their links in the store and in their
// markdown backups, the target stays present with its tier unchanged.
func TestSupersedeLearning_Confirmation(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)

	result := callSupersede(t, l, b, a)
	if result.IsError {
		t.Fatalf("SupersedeLearning returned error: %s", resultText(result))
	}
	parsed := parseCompileResult(t, resultText(result))
	if parsed["status"] != "superseded" {
		t.Errorf("status = %v, want superseded", parsed["status"])
	}
	if parsed["successor"] != b || parsed["target"] != a {
		t.Errorf("result identities = %v/%v, want %s/%s", parsed["successor"], parsed["target"], b, a)
	}
	if _, has := parsed["supersedes_warning"]; has {
		t.Errorf("unexpected supersedes_warning: %v", parsed["supersedes_warning"])
	}

	// Store: successor gains supersedes; target gains superseded_by and
	// validity superseded; the target keeps its tier.
	succProps := readStoredProps(t, s, b)
	if succProps["supersedes"] != a {
		t.Errorf("successor supersedes = %v, want %s", succProps["supersedes"], a)
	}
	tgtProps := readStoredProps(t, s, a)
	if tgtProps["superseded_by"] != b {
		t.Errorf("target superseded_by = %v, want %s", tgtProps["superseded_by"], b)
	}
	if tgtProps["validity"] != "superseded" {
		t.Errorf("target validity = %v, want superseded", tgtProps["validity"])
	}
	page, err := s.GetPage("learning/" + a)
	if err != nil || page == nil {
		t.Fatalf("target must still exist: %v (page=%v)", err, page)
	}
	if page.Tier != "draft" {
		t.Errorf("target tier = %q, want unchanged draft", page.Tier)
	}

	// Backups: both sides carry the links in frontmatter.
	succBackup := readBackup(t, vaultDir, b)
	if value, ok := frontmatterScalar(succBackup, "supersedes"); !ok || value != a {
		t.Errorf("successor backup supersedes = %q (found=%v), want %s", value, ok, a)
	}
	tgtBackup := readBackup(t, vaultDir, a)
	if value, ok := frontmatterScalar(tgtBackup, "superseded_by"); !ok || value != b {
		t.Errorf("target backup superseded_by = %q (found=%v), want %s", value, ok, b)
	}
	if value, ok := frontmatterScalar(tgtBackup, "validity"); !ok || value != "superseded" {
		t.Errorf("target backup validity = %q (found=%v), want superseded", value, ok)
	}
}

// TestSupersedeLearning_UnknownIdentity covers the ES-FR-008 existence
// refusal on either side, with nothing written.
func TestSupersedeLearning_UnknownIdentity(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)

	tests := []struct {
		name      string
		successor string
		target    string
		missing   string
	}{
		{"unknown successor", "auth-20260909T000000-nobody", a, "auth-20260909T000000-nobody"},
		{"unknown target", b, "auth-20260909T000000-nobody", "auth-20260909T000000-nobody"},
		{"page-name form resolves", "learning/" + b, "learning/" + a, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callSupersede(t, l, tt.successor, tt.target)
			if tt.missing != "" {
				if !result.IsError {
					t.Fatalf("expected refusal, got: %s", resultText(result))
				}
				if !strings.Contains(resultText(result), tt.missing) {
					t.Errorf("result = %q, want it to name %s", resultText(result), tt.missing)
				}
				if props := readStoredProps(t, s, a); props["superseded_by"] != nil {
					t.Errorf("nothing must be written to the target, got %v", props["superseded_by"])
				}
			} else if result.IsError {
				t.Fatalf("page-name form must resolve: %s", resultText(result))
			}
		})
	}
}

// TestSupersedeLearning_SameLearning covers the ES-FR-008 distinctness
// refusal.
func TestSupersedeLearning_SameLearning(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, _ := seedSupersedeLearnings(t, l, s, vaultDir)

	result := callSupersede(t, l, a, a)
	if !result.IsError {
		t.Fatalf("expected refusal, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "same learning") {
		t.Errorf("result = %q, want same-learning refusal", resultText(result))
	}
	if props := readStoredProps(t, s, a); props["superseded_by"] != nil || props["supersedes"] != nil {
		t.Errorf("nothing must be written, got %v", props)
	}
}

// TestSupersedeLearning_TargetAlreadySuperseded covers the ES-FR-008
// scenario: the target already records a superseded_by, so the call is
// refused with an error naming the recorded successor and nothing is
// written.
func TestSupersedeLearning_TargetAlreadySuperseded(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	c := "auth-20260317T100003-test"
	storeLearningWithProps(t, s, c, `{"tag":"auth","created_at":"2026-03-17T10:00:03Z"}`, "third learning", time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC))
	// The target a is already superseded by c.
	overwriteLearningProps(t, s, a, `{"tag":"auth","created_at":"2026-03-15T10:00:01Z","superseded_by":"`+c+`","validity":"superseded"}`)

	result := callSupersede(t, l, b, a)
	if !result.IsError {
		t.Fatalf("expected refusal, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "already superseded") || !strings.Contains(resultText(result), c) {
		t.Errorf("result = %q, want already-superseded refusal naming %s", resultText(result), c)
	}
	// The recorded successor is not overwritten.
	if props := readStoredProps(t, s, a); props["superseded_by"] != c {
		t.Errorf("recorded successor overwritten: %v", props["superseded_by"])
	}
	if props := readStoredProps(t, s, b); props["supersedes"] != nil {
		t.Errorf("nothing must be written to the successor, got %v", props["supersedes"])
	}
}

// TestSupersedeLearning_SuccessorDeclaresOther covers the ES-FR-008
// refusal when the successor already declares a supersedes naming a
// different learning.
func TestSupersedeLearning_SuccessorDeclaresOther(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	c := "auth-20260317T100003-test"
	storeLearningWithProps(t, s, c, `{"tag":"auth","created_at":"2026-03-17T10:00:03Z"}`, "third learning", time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC))
	// The successor b already supersedes c, not the requested target a.
	overwriteLearningProps(t, s, b, `{"tag":"auth","created_at":"2026-03-16T10:00:02Z","supersedes":"`+c+`"}`)

	result := callSupersede(t, l, b, a)
	if !result.IsError {
		t.Fatalf("expected refusal, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "already declares supersedes") || !strings.Contains(resultText(result), c) {
		t.Errorf("result = %q, want refusal naming %s", resultText(result), c)
	}
	if props := readStoredProps(t, s, a); props["superseded_by"] != nil {
		t.Errorf("nothing must be written to the target, got %v", props["superseded_by"])
	}
	if props := readStoredProps(t, s, b); props["supersedes"] != c {
		t.Errorf("successor's existing declaration must stand, got %v", props["supersedes"])
	}
}

// TestSupersedeLearning_CycleRefused covers the ES-FR-008 cycle
// scenario: B already declares supersedes: A, so linking successor A to
// target B is refused with an error reporting the cycle and neither
// learning is written.
func TestSupersedeLearning_CycleRefused(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	// b already supersedes a (half-linked the wrong way for this call).
	overwriteLearningProps(t, s, b, `{"tag":"auth","created_at":"2026-03-16T10:00:02Z","supersedes":"`+a+`"}`)

	result := callSupersede(t, l, a, b)
	if !result.IsError {
		t.Fatalf("expected cycle refusal, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "cycle") {
		t.Errorf("result = %q, want cycle refusal", resultText(result))
	}
	if props := readStoredProps(t, s, a); props["supersedes"] != nil {
		t.Errorf("successor must not be written, got %v", props["supersedes"])
	}
	if props := readStoredProps(t, s, b); props["superseded_by"] != nil {
		t.Errorf("target must not be written, got %v", props["superseded_by"])
	}
}

// TestSupersedeLearning_PreExistingCycleTerminatesAndRefused covers the
// ES-FR-008 scenario: A, B, and C already form a cyclic chain of declared
// links that excludes target D. The walk must terminate via its visited
// set rather than spinning, the tool must return an error naming the
// pre-existing cycle, and nothing must be written.
func TestSupersedeLearning_PreExistingCycleTerminatesAndRefused(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, _ := seedSupersedeLearnings(t, l, s, vaultDir)
	b := "auth-20260316T100002-test"
	c := "auth-20260317T100003-test"
	d := "auth-20260318T100004-test"
	// The cyclic chain a → b → c → a, excluding d: a declares no
	// supersedes of its own — its edge into the chain is a superseded_by
	// link — while b and c declare supersedes closing the loop.
	overwriteLearningProps(t, s, a, `{"tag":"auth","created_at":"2026-03-15T10:00:01Z","superseded_by":"`+b+`"}`)
	overwriteLearningProps(t, s, b, `{"tag":"auth","created_at":"2026-03-16T10:00:02Z","supersedes":"`+c+`"}`)
	storeLearningWithProps(t, s, c, `{"tag":"auth","created_at":"2026-03-17T10:00:03Z","supersedes":"`+a+`"}`, "c", time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC))
	storeLearningWithProps(t, s, d, `{"tag":"auth","created_at":"2026-03-18T10:00:04Z"}`, "d", time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC))

	result := callSupersede(t, l, a, d)
	if !result.IsError {
		t.Fatalf("expected pre-existing-cycle refusal, got: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "pre-existing supersession cycle") {
		t.Errorf("result = %q, want pre-existing-cycle refusal", text)
	}
	// The cycle is named as a chain of its members.
	for _, member := range []string{a, b, c} {
		if !strings.Contains(text, member) {
			t.Errorf("result = %q, want it to name cycle member %s", text, member)
		}
	}
	// Nothing is written: a keeps its declared links, d stays clean.
	if props := readStoredProps(t, s, a); props["superseded_by"] != b {
		t.Errorf("a's declared superseded_by must stand, got %v", props["superseded_by"])
	}
	if props := readStoredProps(t, s, d); props["superseded_by"] != nil {
		t.Errorf("target must not be written, got %v", props["superseded_by"])
	}
}

// TestSupersedeLearning_IdempotentHalfLinkCompletion covers the designed
// idempotency: a successor that already declares supersedes naming the
// requested target (a half link) is re-confirmed, not refused — the call
// completes the target's side.
func TestSupersedeLearning_IdempotentHalfLinkCompletion(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	// b already half-links to a; a records nothing.
	overwriteLearningProps(t, s, b, `{"tag":"auth","created_at":"2026-03-16T10:00:02Z","supersedes":"`+a+`"}`)

	result := callSupersede(t, l, b, a)
	if result.IsError {
		t.Fatalf("half-link completion must succeed, got: %s", resultText(result))
	}
	if props := readStoredProps(t, s, a); props["superseded_by"] != b {
		t.Errorf("target superseded_by = %v, want %s", props["superseded_by"], b)
	}
	if props := readStoredProps(t, s, b); props["supersedes"] != a {
		t.Errorf("successor supersedes = %v, want %s", props["supersedes"], a)
	}
}

// TestSupersedeLearning_LostClaimWarning covers the ES-FR-009 outcome at
// the claim level: claimSuperseded's guarded re-read of an already-claimed
// target yields the lost claim with the recorded winner, and
// lostClaimWarning names that winner. (The handler's full in-hold ladder
// refuses same-process races before the claim; the claim guard remains
// the authority over writers that bypass the process mutex, matching
// store_learning semantics.)
func TestSupersedeLearning_LostClaimWarning(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	winner := "auth-20260317T100003-test"
	storeLearningWithProps(t, s, winner, `{"tag":"auth","created_at":"2026-03-17T10:00:03Z"}`, "winner", time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC))
	// A raw writer recorded the winner on the target outside the mutex.
	overwriteLearningProps(t, s, a, `{"tag":"auth","created_at":"2026-03-15T10:00:01Z","superseded_by":"`+winner+`","validity":"superseded"}`)

	target, err := l.store.GetPage("learning/" + a)
	if err != nil || target == nil {
		t.Fatalf("resolve target: %v (page=%v)", err, target)
	}
	claimed, recorded, claimErr := l.claimSuperseded(target, b)
	if claimErr != nil {
		t.Fatalf("claimSuperseded: %v", claimErr)
	}
	if claimed {
		t.Fatal("claim must not land on an already-claimed target")
	}
	if recorded != winner {
		t.Fatalf("winner = %q, want %s", recorded, winner)
	}
	warning := lostClaimWarning(target.Name, recorded)
	if !strings.Contains(warning, winner) || !strings.Contains(warning, "concurrently superseded") {
		t.Errorf("warning = %q, want it to name the winner %s", warning, winner)
	}
	// The recorded winner is not overwritten.
	if props := readStoredProps(t, s, a); props["superseded_by"] != winner {
		t.Errorf("recorded winner overwritten: %v", props["superseded_by"])
	}
}

// TestSupersedeLearning_PartialFailureBackupRefusal covers the ES-FR-009
// partial-failure warning: when the target's markdown backup already
// records a different successor (an operator hand-edit), the store legs
// land but the target's file leg refuses, and the result carries
// supersedes_warning naming the recorded value.
func TestSupersedeLearning_PartialFailureBackupRefusal(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	// The target's backup records a differing successor while its store
	// properties stay clean.
	writeLearningBackup(t, vaultDir, a, backupContent([][2]string{
		{"tag", "auth"}, {"created_at", "2026-03-15T10:00:01Z"},
		{"superseded_by", "auth-20260909T000000-nobody"},
	}))

	result := callSupersede(t, l, b, a)
	if result.IsError {
		t.Fatalf("partial failure must not be an error result: %s", resultText(result))
	}
	parsed := parseCompileResult(t, resultText(result))
	warning, _ := parsed["supersedes_warning"].(string)
	if warning == "" {
		t.Fatalf("result missing supersedes_warning: %v", parsed)
	}
	if !strings.Contains(warning, "markdown backup") || !strings.Contains(warning, "auth-20260909T000000-nobody") {
		t.Errorf("warning = %q, want it to name the backup refusal", warning)
	}
	// The store legs stand.
	if props := readStoredProps(t, s, a); props["superseded_by"] != b {
		t.Errorf("store target leg must stand, got %v", props["superseded_by"])
	}
	if props := readStoredProps(t, s, b); props["supersedes"] != a {
		t.Errorf("store successor leg must stand, got %v", props["supersedes"])
	}
	// The backup is left byte-for-byte unchanged.
	if value, ok := frontmatterScalar(readBackup(t, vaultDir, a), "superseded_by"); !ok || value != "auth-20260909T000000-nobody" {
		t.Errorf("backup must keep its recorded value, got %q (found=%v)", value, ok)
	}
}

// TestSupersedeLearning_ConcurrentSameTargetRace covers the ES-FR-007
// serialization: two calls superseding the same target started together
// serialize on the learning-properties lock; exactly one link lands, the
// refused call reports the recorded winner, and no fork is written. No
// sleeps — a start barrier plus the -race detector.
func TestSupersedeLearning_ConcurrentSameTargetRace(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)
	c := "auth-20260317T100003-test"
	storeLearningWithProps(t, s, c, `{"tag":"auth","created_at":"2026-03-17T10:00:03Z"}`, "third learning", time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC))

	inputs := []types.SupersedeLearningInput{
		{Successor: b, Target: a},
		{Successor: c, Target: a},
	}
	results := make([]*mcp.CallToolResult, len(inputs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res, _, err := l.SupersedeLearning(context.Background(), nil, inputs[i])
			if err != nil {
				t.Errorf("SupersedeLearning: %v", err)
			}
			results[i] = res
		}(i)
	}
	close(start)
	wg.Wait()

	// Exactly one direction landed.
	propsA := readStoredProps(t, s, a)
	winner, _ := propsA["superseded_by"].(string)
	if winner != b && winner != c {
		t.Fatalf("target superseded_by = %v, want exactly one of %s/%s", propsA["superseded_by"], b, c)
	}
	loser := c
	if winner == c {
		loser = b
	}
	// The winner's link stands; the loser's successor was never written.
	if props := readStoredProps(t, s, winner); props["supersedes"] != a {
		t.Errorf("winner supersedes = %v, want %s", props["supersedes"], a)
	}
	if props := readStoredProps(t, s, loser); props["supersedes"] != nil {
		t.Errorf("loser must not be written, got %v", props["supersedes"])
	}
	// The refused call reports the recorded winner.
	var refused *mcp.CallToolResult
	for _, r := range results {
		if r != nil && r.IsError {
			refused = r
		}
	}
	if refused == nil {
		t.Fatal("one call must be refused")
	}
	if !strings.Contains(resultText(refused), winner) {
		t.Errorf("refusal = %q, want it to name the winner %s", resultText(refused), winner)
	}
}

// TestSupersedeLearning_ConcurrentReversePairRace covers the ES-FR-008
// reverse-pair scenario: A→B and B→A started together serialize on the
// lock; exactly one direction lands, the refused call returns an error
// naming the outcome, and no cycle is written — neither learning records
// both a supersedes and a superseded_by naming the other.
func TestSupersedeLearning_ConcurrentReversePairRace(t *testing.T) {
	l, s, vaultDir := newSupersedeTool(t)
	a, b := seedSupersedeLearnings(t, l, s, vaultDir)

	inputs := []types.SupersedeLearningInput{
		{Successor: a, Target: b},
		{Successor: b, Target: a},
	}
	results := make([]*mcp.CallToolResult, len(inputs))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res, _, err := l.SupersedeLearning(context.Background(), nil, inputs[i])
			if err != nil {
				t.Errorf("SupersedeLearning: %v", err)
			}
			results[i] = res
		}(i)
	}
	close(start)
	wg.Wait()

	propsA := readStoredProps(t, s, a)
	propsB := readStoredProps(t, s, b)
	aSupersedes, _ := propsA["supersedes"].(string)
	bSupersedes, _ := propsB["supersedes"].(string)

	// At most one direction of the pair is written.
	if aSupersedes == b && bSupersedes == a {
		t.Fatalf("cycle written: a.supersedes=%q b.supersedes=%q", aSupersedes, bSupersedes)
	}
	// One call succeeded, one was refused.
	succeeded := 0
	for i, r := range results {
		if r == nil {
			t.Fatalf("call %d returned no result", i)
		}
		if r.IsError {
			if !strings.Contains(resultText(r), "cycle") && !strings.Contains(resultText(r), "already superseded") {
				t.Errorf("refusal %d = %q, want cycle or already-superseded refusal", i, resultText(r))
			}
		} else {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded = %d, want exactly 1 (results: %s / %s)", succeeded, resultText(results[0]), resultText(results[1]))
	}

	// No learning records both directions naming the other.
	for identity, props := range map[string]map[string]any{a: propsA, b: propsB} {
		sup, _ := props["supersedes"].(string)
		by, _ := props["superseded_by"].(string)
		other := b
		if identity == b {
			other = a
		}
		if sup == other && by == other {
			t.Fatalf("learning %s records both directions naming %s", identity, other)
		}
	}
}
