package tools

// Candidate verification (issue #151): the verify_candidates tool's
// read-only enumeration of the promotable queue.
//
// The queue is every tier `draft` page in the learning/, compiled/, and
// knowledge/ namespaces — the same namespaces promote resolves against,
// with the same tier-based eligibility (KV-FR-001). Enumeration resolves
// the pages/tags selectors into an ordered candidate list and never
// mutates tiers, validity, content, or files (KV-FR-005). Anchor checking,
// aggregation, and result shaping are the pass that consumes these
// candidates, reusing the lint anchor machinery (design D2: the
// anchorChecker, learningAnchors, and parseAnchor seams are shared; the
// pass runner is deliberately not extracted).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Verify implements the verify_candidates MCP tool (issue #151): a
// deterministic, read-only verification of the promotable candidate queue
// ahead of the human promotion gate.
//
// Design decision (D2): Verify carries the same anchorChecker seam and
// pass-budget shape as Lint (tools/lint_anchors.go), constructed with a
// local-git checker over the vault path, so tests can substitute the git
// layer or expire the budget without a repository. As in lint, the
// per-call git timeout is injectable through the checker itself
// (gitAnchorChecker.timeout, anchorGitTimeout when zero).
type Verify struct {
	store        *store.Store
	vaultPath    string
	anchors      anchorChecker // git-backed anchor checks; nil skips them
	anchorBudget time.Duration // bound on one verification pass; 0 uses anchorCheckBudget
}

// NewVerify creates a Verify handler over the given store and vault path.
// The store must be non-nil for the tool to function; a clear error is
// returned at call time if it is nil, as in NewLint. The vaultPath anchors
// local-git anchor checks; when it is empty they are skipped.
func NewVerify(s *store.Store, vaultPath string) *Verify {
	v := &Verify{store: s, vaultPath: vaultPath}
	if vaultPath != "" {
		v.anchors = gitAnchorChecker{dir: vaultPath}
	}
	return v
}

// --- Tool handler and result surface (KV-FR-002, KV-FR-004) ---
//
// The public surface consumes the enumerated queue and the pass outcomes
// and renders the MCP result: a pass status of checked, skipped, or failed
// with a reason when not checked, one entry per candidate, and summary
// counts per result. Nothing in the payload expresses or implies
// endorsement — anchors_ok states code correspondence only (KV-FR-002).

// Verification pass statuses (KV-FR-004): checked when the pass ran to
// completion, skipped when it could not run — no vault path, or a vault
// that is not a git repository, which is a degraded mode, not an error —
// and failed when git could not answer, the request was cancelled, or the
// pass budget was exhausted, which routes as an error result. These mirror
// lint's anchor_checks outcome (design D4).
const (
	verifyPassChecked = "checked"
	verifyPassSkipped = "skipped"
	verifyPassFailed  = "failed"
)

// verifyCandidateResult is one candidate's entry in the tool result
// (KV-FR-002): the resolved page name, the learning identity when the
// candidate is a learning — identity is not defined for compiled or
// knowledge candidates; the page name is their identifier — the tier, the
// bounded evidence locators, one result from the fixed enum, and the
// reason a skipped entry must carry. Sources and their omission count
// appear only for a compiled article whose sources were actually checked.
type verifyCandidateResult struct {
	Page           string               `json:"page"`
	Identity       string               `json:"identity,omitempty"`
	Tier           string               `json:"tier,omitempty"`
	Evidence       []string             `json:"evidence,omitempty"`
	Result         string               `json:"result"`
	Reason         string               `json:"reason,omitempty"`
	Sources        []verifySourceResult `json:"sources,omitempty"`
	SourcesOmitted int                  `json:"sources_omitted,omitempty"`
}

// entry renders one outcome as its result entry (KV-FR-002). An
// enumeration-skipped outcome has no page, so its entry carries the
// requested name and the enumeration reason only; tier, identity, and
// evidence are omitted rather than fabricated.
func (o verifyOutcome) entry() verifyCandidateResult {
	e := verifyCandidateResult{
		Page:           o.candidate.Name,
		Evidence:       o.evidence,
		Result:         o.result,
		Reason:         o.reason,
		Sources:        o.sources,
		SourcesOmitted: o.sourcesOmitted,
	}
	if o.candidate.Page != nil {
		e.Tier = o.candidate.Page.Tier
		e.Identity = learningIdentityOf(o.candidate.Page.Name)
	}
	return e
}

// verifySummary counts candidates per result value (KV-FR-002). Every enum
// value has a fixed key so a consumer can index any result without
// distinguishing absent from zero, and the marshaled shape is stable for
// the golden output test (design D9).
type verifySummary struct {
	AnchorsOK int `json:"anchors_ok"`
	Drifted   int `json:"drifted"`
	Missing   int `json:"missing"`
	NoAnchors int `json:"no_anchors"`
	Skipped   int `json:"skipped"`
}

// count tallies one result into the summary. Results outside the fixed
// enum are not counted; every result the pass emits comes from the enum.
func (s *verifySummary) count(result string) {
	switch result {
	case verifyAnchorsOK:
		s.AnchorsOK++
	case verifyDrifted:
		s.Drifted++
	case verifyMissing:
		s.Missing++
	case verifyNoAnchors:
		s.NoAnchors++
	case verifySkipped:
		s.Skipped++
	}
}

// VerifyCandidates handles the verify_candidates MCP tool (issue #151).
// It resolves the pages/tags selectors into the candidate queue, runs one
// bounded verification pass, and reports each candidate with its result
// from the fixed enum plus summary counts (KV-FR-002).
//
// Read-only guarantee (KV-FR-005, design D5): this handler performs no
// store writes, no file writes, no promote calls, and no tier, validity,
// content, or property changes — verification is not endorsement, and a
// passing check cannot advance a tier. Its only side effects are local git
// read queries through the anchorChecker seam, each bounded by the
// per-call timeout with the whole pass bounded by the pass budget, and the
// request context is honored between calls.
//
// Determinism (KV-FR-005): the queue is ordered lexicographically by name
// and the payload carries no timestamps or environmental data, so two runs
// over identical store content, pin, and HEAD that each complete a checked
// pass return identical candidates and results in the same order. A run
// ending skipped or failed is a failure mode, excluded from that
// guarantee.
//
// Degraded and failure behavior (KV-FR-004): a vault that is not a git
// repository skips the pass — status skipped, every candidate skipped with
// that reason, and NOT an error result, so the gate still runs. When git
// cannot answer, the request is cancelled, or the budget is exhausted, the
// result is an error result (IsError) with status failed, and every
// candidate entry carries result skipped with a reason naming the failed
// pass, so nothing checked before the failure is presented as verified.
func (v *Verify) VerifyCandidates(ctx context.Context, _ *mcp.CallToolRequest, input types.VerifyCandidatesInput) (*mcp.CallToolResult, any, error) {
	if v.store == nil {
		return errorResult(persistentStorageError("verify_candidates").Error()), nil, nil
	}
	candidates, err := v.enumerateCandidates(input)
	if err != nil {
		// Selector validation and enumeration errors precede the pass:
		// there is no queue to report, so the failure routes as a plain
		// error result rather than a failed-pass payload.
		return errorResult(err.Error()), nil, nil
	}
	outcomes, err := v.runPass(ctx, candidates)
	var skip *verifySkipError
	switch {
	case err == nil:
		return verifyCheckedResult(outcomes)
	case errors.As(err, &skip):
		return verifySkippedResult(candidates, skip.reason)
	default:
		return verifyFailedResult(candidates, err)
	}
}

// verifyCheckedResult renders a completed pass: status checked, one entry
// per outcome in queue order, and summary counts per result (KV-FR-002).
// The message states code correspondence only — never endorsement.
func verifyCheckedResult(outcomes []verifyOutcome) (*mcp.CallToolResult, any, error) {
	entries := make([]verifyCandidateResult, 0, len(outcomes))
	var summary verifySummary
	for _, o := range outcomes {
		entries = append(entries, o.entry())
		summary.count(o.result)
	}
	payload := map[string]any{
		"status":     verifyPassChecked,
		"candidates": entries,
		"summary":    summary,
		"message":    fmt.Sprintf("Verification pass completed; %d candidate(s) reported. anchors_ok states only that cited paths exist at the pin and are unchanged at HEAD — never that a claim is correct, worth acting on, or safe to trust. Nothing was promoted or modified.", len(entries)),
	}
	res, err := jsonTextResult(payload)
	return res, nil, err
}

// verifySkippedResult renders a pass that could not run — no vault path,
// or a vault that is not a git repository (KV-FR-004). The pass status is
// skipped with the reason and every candidate carries result skipped with
// that reason; this is NOT an error result, so the promotion gate still
// runs and the reviewer sees an unverified queue rather than a clean one.
// A candidate already skipped at enumeration keeps that reason ahead of
// the pass reason, because a vanished or non-draft requested page must
// stay distinguishable from a pass-level skip (KV-FR-002).
func verifySkippedResult(candidates []verifyCandidate, reason string) (*mcp.CallToolResult, any, error) {
	entries := make([]verifyCandidateResult, 0, len(candidates))
	var summary verifySummary
	for _, c := range candidates {
		e := unverifiedEntry(c, reason)
		summary.count(e.Result)
		entries = append(entries, e)
	}
	payload := map[string]any{
		"status":     verifyPassSkipped,
		"reason":     reason,
		"candidates": entries,
		"summary":    summary,
		"message":    fmt.Sprintf("Verification pass skipped: %s. Every candidate is reported skipped; none was verified. Nothing was promoted or modified.", reason),
	}
	res, err := jsonTextResult(payload)
	return res, nil, err
}

// verifyFailedResult renders a failed pass as an MCP error result
// (IsError) so a caller cannot read an unrun pass as clean (KV-FR-004):
// git could not answer, the request was cancelled, or the budget was
// exhausted. Every candidate entry — including those checked before the
// failure — carries result skipped with a reason naming the failed pass;
// pre-failure outcomes are never reported, so no anchors_ok, drifted, or
// missing can appear in the payload.
func verifyFailedResult(candidates []verifyCandidate, failure error) (*mcp.CallToolResult, any, error) {
	passReason := "verification pass failed: " + failure.Error()
	entries := make([]verifyCandidateResult, 0, len(candidates))
	var summary verifySummary
	for _, c := range candidates {
		e := unverifiedEntry(c, passReason)
		summary.count(e.Result)
		entries = append(entries, e)
	}
	payload := map[string]any{
		"status":     verifyPassFailed,
		"reason":     failure.Error(),
		"candidates": entries,
		"summary":    summary,
		"message":    fmt.Sprintf("Verification pass failed: %s. No candidate is reported as verified; candidates checked before the failure appear only as skipped context. Nothing was promoted or modified.", failure),
	}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, nil, err
	}
	res.IsError = true
	return res, nil, nil
}

// unverifiedEntry renders a candidate that was never checked: result
// skipped, no outcome data. Tier, identity, and the page's own stored
// evidence locators are still presented because they are facts read from
// the store, not verification outcomes — but the compiled per-source
// results are omitted, since no source was checked. passReason is the
// pass-level skip or failure reason the entry must carry.
func unverifiedEntry(c verifyCandidate, passReason string) verifyCandidateResult {
	e := verifyCandidateResult{
		Page:   c.Name,
		Result: verifySkipped,
		Reason: unverifiedReason(c.Reason, passReason),
	}
	if c.Page != nil {
		e.Tier = c.Page.Tier
		e.Identity = learningIdentityOf(c.Page.Name)
		e.Evidence = presentEvidence(pageEvidence(c.Page))
	}
	return e
}

// unverifiedReason joins a candidate's enumeration skip reason, when it
// has one, with the pass-level reason: an entry already skipped at
// enumeration keeps that context, and every entry's reason carries the
// pass-level reason (KV-FR-004).
func unverifiedReason(enumReason, passReason string) string {
	if enumReason == "" {
		return passReason
	}
	return enumReason + "; " + passReason
}

// learningIdentityOf returns the learning identity of a learning/ page —
// the page name without its learning/ prefix — and "" for every other
// candidate: identity is not defined for compiled or knowledge candidates,
// whose page name is their identifier (KV-FR-002). The strip mirrors
// lint's identity handling.
func learningIdentityOf(name string) string {
	if hasNamespacePrefix(name, store.LearningPrefix) {
		return strings.TrimPrefix(name, store.LearningPrefix)
	}
	return ""
}

// verifyCandidate is one enumerated member of a verification pass. Page is
// non-nil exactly when Reason is empty: either a resolved tier `draft` page
// ready to be checked, or a skipped pages request that matched no stored
// page or a page whose tier is not draft (KV-FR-002 — skipped entries carry
// a reason and are never silently dropped). Name is the resolved page name,
// or the requested name for a skipped entry, and is the queue's
// deterministic ordering key.
type verifyCandidate struct {
	Name   string
	Page   *store.Page
	Reason string
}

// enumerateCandidates resolves the selector into the deterministic
// candidate queue. Supplying both pages and tags is a validation error
// returned before any enumeration (KV-FR-001). With pages, each name is
// resolved exactly — no case-insensitive ladder and no bare-name
// shorthand, unlike promote — and a name that matches no stored page or a
// non-draft page becomes a skipped entry. With tags, or with neither
// selector, the queue is every tier `draft` page under learning/,
// compiled/, and knowledge/, restricted to the named tags. The result is
// ordered lexicographically by name (KV-FR-005).
func (v *Verify) enumerateCandidates(input types.VerifyCandidatesInput) ([]verifyCandidate, error) {
	if len(input.Pages) > 0 && len(input.Tags) > 0 {
		return nil, errors.New("verify_candidates: provide either pages or tags, not both")
	}
	if len(input.Pages) > 0 {
		return v.enumerateRequestedPages(input.Pages)
	}
	return v.enumerateQueue(input.Tags)
}

// enumerateRequestedPages resolves explicit page names into candidates.
// Names match stored page names exactly: an entry that matches nothing, or
// matches a page whose tier is not draft, is reported as a skipped
// candidate with a reason rather than dropped (KV-FR-002), because the
// reviewer needs to know a candidate vanished. Duplicate names are
// deduplicated, first occurrence winning.
func (v *Verify) enumerateRequestedPages(requested []string) ([]verifyCandidate, error) {
	seen := make(map[string]bool, len(requested))
	candidates := make([]verifyCandidate, 0, len(requested))
	for _, name := range requested {
		if seen[name] {
			continue
		}
		seen[name] = true
		page, err := v.store.GetPage(name)
		if err != nil {
			return nil, fmt.Errorf("resolve page %q: %w", name, err)
		}
		switch {
		case page == nil:
			candidates = append(candidates, verifyCandidate{
				Name:   name,
				Reason: fmt.Sprintf("no stored page named %q", name),
			})
		case page.Tier != "draft":
			candidates = append(candidates, verifyCandidate{
				Name:   name,
				Reason: fmt.Sprintf("page %q has tier %q; only draft pages are candidates", name, page.Tier),
			})
		default:
			candidates = append(candidates, verifyCandidate{Name: name, Page: page})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	return candidates, nil
}

// enumerateQueue enumerates the default candidate queue — every tier
// `draft` page under learning/, compiled/, and knowledge/ — restricted to
// the named tags when tags is non-empty. Pages matching no requested tag
// are out of scope, not skipped entries: a tag narrows the queue, it does
// not name candidates that could have vanished. The store returns the
// queue ordered by name.
func (v *Verify) enumerateQueue(tags []string) ([]verifyCandidate, error) {
	pages, err := v.store.ListDraftCandidatePages()
	if err != nil {
		return nil, fmt.Errorf("list draft candidate pages: %w", err)
	}
	candidates := make([]verifyCandidate, 0, len(pages))
	for _, p := range pages {
		if len(tags) > 0 && !matchesAnyTag(p, tags) {
			continue
		}
		candidates = append(candidates, verifyCandidate{Name: p.Name, Page: p})
	}
	return candidates, nil
}

// matchesAnyTag reports whether page p is a member of any requested tag.
func matchesAnyTag(p *store.Page, tags []string) bool {
	for _, tag := range tags {
		if pageMatchesTag(p, tag) {
			return true
		}
	}
	return false
}

// pageMatchesTag reports whether page p is a member of tag. Membership is
// read from the page-name prefixes and the `tag` property, never from
// prose (KV-FR-001), and both forms match exactly: the name form requires
// the delimiter after the tag — learning/{tag}-…, so tag `auth` never
// matches learning/authentication-… — or the exact name compiled/{tag};
// the property form requires the page's `tag` property to equal the
// requested tag. knowledge/ page names carry no tag prefix, so they match
// only through the property. A longer hyphenated tag can over-match by
// prefix (tag `auth` matches learning/auth-flow-…), mirroring the
// command's inventory step; that over-match is accepted.
func pageMatchesTag(p *store.Page, tag string) bool {
	if strings.HasPrefix(p.Name, store.LearningPrefix+tag+"-") || p.Name == store.CompiledPrefix+tag {
		return true
	}
	pageTag, ok := pageTagProperty(p)
	return ok && pageTag == tag
}

// pageTagProperty returns the page's `tag` property when it is present as
// a string. Unreadable properties carry no tag: one malformed page never
// fails the whole enumeration.
func pageTagProperty(p *store.Page) (string, bool) {
	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return "", false
	}
	tag, ok := props["tag"].(string)
	return tag, ok
}

// --- Verification pass (KV-FR-003) ---
//
// The pass consumes the enumerated queue and produces one outcome per
// candidate. Learning outcomes reuse the lint anchor machinery —
// learningAnchors for eligibility, the anchorChecker seam for every git
// question — and compiled articles aggregate their source learnings
// (design D3). The pass runner is deliberately not extracted from lint's
// checkAnchors (design D2's DRY decision): the two loops emit different
// shapes over different page sets, and two consumers do not meet the
// repo's rule-of-three extraction bar.

// Verification results: the fixed enum every candidate carries exactly
// one of (KV-FR-002). anchors_ok states code correspondence only — cited
// paths exist at the pin and are unchanged at HEAD — never that the claim
// is correct, worth acting on, or safe to trust.
const (
	verifyAnchorsOK = "anchors_ok"
	verifyDrifted   = "drifted"
	verifyMissing   = "missing"
	verifyNoAnchors = "no_anchors"
	verifySkipped   = "skipped"
)

// maxReportedSources bounds the per-source results reported for one
// compiled article (KV-FR-003 pinned advisory): sources is untrusted
// frontmatter that can name arbitrarily many learnings, so the reported
// list is capped and the remainder is surfaced as a count. The aggregate
// still folds every resolved source, reported or not.
const maxReportedSources = 50

// verifySourceResult is one source learning's individual result inside a
// compiled article's entry, reported alongside the aggregate so a reviewer
// can see which source drifted without opening every candidate by hand
// (KV-FR-003, design D3). Detail carries the ineligibility context when
// Result is no_anchors: a stored-but-ineligible source is never conflated
// with an unresolvable one.
type verifySourceResult struct {
	Identity string `json:"identity"`
	Result   string `json:"result"`
	Detail   string `json:"detail,omitempty"`
}

// verifyLearningCheck is a learning page's anchor outcome within one pass:
// the result, plus the ineligibility context when the result is
// no_anchors.
type verifyLearningCheck struct {
	result string
	detail string
}

// verifyOutcome is one candidate's verification outcome: the enumerated
// candidate, its result from the fixed enum, its presented evidence
// locators, and — for a compiled article — the per-source results behind
// the aggregate plus the count of sources beyond the reporting bound.
// Reason carries the enumeration skip reason for a skipped candidate, and
// for a no_anchors candidate the advisory context about why nothing was
// checkable; only the skipped reason is required by KV-FR-002.
type verifyOutcome struct {
	candidate      verifyCandidate
	result         string
	reason         string
	evidence       []string
	sources        []verifySourceResult
	sourcesOmitted int
}

// verifySkipError reports a condition that skips the whole verification
// pass rather than failing it — no vault path, or a vault that is not a
// git repository (KV-FR-004). Marking the pass skipped and every
// candidate skipped with this reason is the result layer's job; the pass
// runner only surfaces the condition.
type verifySkipError struct{ reason string }

// Error returns the skip reason.
func (e *verifySkipError) Error() string { return e.reason }

// runPass verifies every candidate within v.anchorBudget
// (anchorCheckBudget when the field is zero), consuming the budget and
// the per-call timeout exactly as Lint.checkAnchors does: the budget
// becomes a deadline on the pass context, each git call keeps the
// checker's own timeout, and the request context is honored between
// candidates. The runner is deliberately not shared with lint (design
// D2's DRY decision).
//
// It returns one outcome per candidate, in the candidate order. A
// *verifySkipError means the pass could not run and must be skipped, not
// failed. Any other error — git cannot answer, request cancelled, budget
// exhausted — fails the pass; outcomes verified before the failure are
// returned alongside it as context only, and MUST NOT be presented as
// verified (KV-FR-004).
func (v *Verify) runPass(ctx context.Context, candidates []verifyCandidate) ([]verifyOutcome, error) {
	if v.anchors == nil {
		return nil, &verifySkipError{reason: "no vault path"}
	}
	budget := v.anchorBudget
	if budget <= 0 {
		budget = anchorCheckBudget
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	head, err := v.anchors.Head(ctx)
	var repo string
	if err == nil {
		repo, err = v.anchors.Repo(ctx)
	}
	if err != nil {
		if errors.Is(err, errAnchorNoRepo) {
			return nil, &verifySkipError{reason: err.Error()}
		}
		return nil, fmt.Errorf("resolve vault repository: %w", err)
	}
	memo := make(map[string]verifyLearningCheck)
	outcomes := make([]verifyOutcome, 0, len(candidates))
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return outcomes, fmt.Errorf("verification pass stopped at %s: %w", c.Name, err)
		}
		outcome, err := v.verifyOne(ctx, memo, c, repo, head)
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// verifyOne computes one candidate's outcome. Skipped entries from the
// enumeration pass through with their reason (KV-FR-002: a vanished or
// non-draft requested page is never silently dropped). Checkable
// candidates dispatch by namespace: learning/ pages are anchor-checked
// through the shared seam, compiled/ articles aggregate their source
// learnings, and knowledge/ pages carry neither an as_of pin nor
// sources, so nothing on them is checkable and they verify as
// no_anchors.
func (v *Verify) verifyOne(ctx context.Context, memo map[string]verifyLearningCheck, c verifyCandidate, repo, head string) (verifyOutcome, error) {
	if c.Page == nil {
		return verifyOutcome{candidate: c, result: verifySkipped, reason: c.Reason}, nil
	}
	switch {
	case hasNamespacePrefix(c.Page.Name, store.LearningPrefix):
		return v.verifyLearning(ctx, memo, c, repo, head)
	case hasNamespacePrefix(c.Page.Name, store.CompiledPrefix):
		return v.verifyCompiled(ctx, memo, c, repo, head)
	default:
		return verifyOutcome{candidate: c, result: verifyNoAnchors, reason: "no as_of pin or sources to check"}, nil
	}
}

// hasNamespacePrefix reports whether name sits under the given page-name
// namespace prefix, case-insensitively. The candidate query admits
// namespace prefixes under NOCASE (ListDraftCandidatePages), so a page
// the queue admitted as a learning or compiled article is verified as
// one; everything else in the queue is a knowledge/ page.
func hasNamespacePrefix(name, prefix string) bool {
	return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

// verifyLearning computes a learning candidate's outcome. Eligibility and
// anchors come from the shared lint seam — learningAnchors plus the
// anchorChecker, reused rather than reimplemented (KV-FR-003) — and the
// presented evidence is the learning's own locator list, bounded at
// presentation because re-ingested frontmatter bypasses store_learning's
// validation.
func (v *Verify) verifyLearning(ctx context.Context, memo map[string]verifyLearningCheck, c verifyCandidate, repo, head string) (verifyOutcome, error) {
	check, err := v.checkLearning(ctx, memo, c.Page, repo, head)
	if err != nil {
		return verifyOutcome{}, err
	}
	return verifyOutcome{
		candidate: c,
		result:    check.result,
		reason:    check.detail,
		evidence:  presentEvidence(pageEvidence(c.Page)),
	}, nil
}

// verifyCompiled computes a compiled article's outcome by aggregating its
// source learnings (design D3). Each declared source identity resolves to
// its stored learning page — learning/<identity>, the bare identities
// compile writes — and is checked like a direct candidate; a source that
// does not resolve contributes nothing, so an article whose sources name
// no stored learning is no_anchors. Presented evidence is the
// deduplicated union of the resolved sources' locators, and the entry
// lists every resolved source identity with its individual result so the
// aggregate is auditable.
func (v *Verify) verifyCompiled(ctx context.Context, memo map[string]verifyLearningCheck, c verifyCandidate, repo, head string) (verifyOutcome, error) {
	declared := extractSourcesFromProperties(c.Page)
	var (
		sources  []verifySourceResult
		evidence []string
		seen     = make(map[string]bool)
	)
	for _, identity := range declared {
		if err := ctx.Err(); err != nil {
			return verifyOutcome{}, fmt.Errorf("verifying sources of %s: %w", c.Page.Name, err)
		}
		source, err := v.store.GetPage(store.LearningPrefix + identity)
		if err != nil {
			return verifyOutcome{}, fmt.Errorf("resolve source %q of %s: %w", identity, c.Page.Name, err)
		}
		if source == nil {
			continue
		}
		check, err := v.checkLearning(ctx, memo, source, repo, head)
		if err != nil {
			return verifyOutcome{}, err
		}
		sources = append(sources, verifySourceResult{
			Identity: identity,
			Result:   check.result,
			Detail:   check.detail,
		})
		for _, locator := range pageEvidence(source) {
			if !seen[locator] {
				seen[locator] = true
				evidence = append(evidence, locator)
			}
		}
	}
	reported, omitted := boundSources(sources)
	return verifyOutcome{
		candidate:      c,
		result:         aggregateSources(sources),
		evidence:       presentEvidence(evidence),
		sources:        reported,
		sourcesOmitted: omitted,
	}, nil
}

// checkLearning returns a learning page's anchor outcome, memoized by
// page name for the pass: a source learning checked for one compiled
// article and met again — as another article's source, or as a direct
// candidate — keeps one outcome instead of re-running the git questions.
// The memo is pass-local (git state is read afresh each pass) and never
// caches errors: a failed check fails the pass before anything is cached.
// A stored-but-ineligible learning reports no_anchors with the
// ineligibility as context (KV-FR-003 pinned advisory), never conflated
// with an unresolvable source.
func (v *Verify) checkLearning(ctx context.Context, memo map[string]verifyLearningCheck, p *store.Page, repo, head string) (verifyLearningCheck, error) {
	if check, ok := memo[p.Name]; ok {
		return check, nil
	}
	var check verifyLearningCheck
	sha, paths, ok := learningAnchors(p, repo)
	if !ok {
		check = verifyLearningCheck{result: verifyNoAnchors, detail: learningIneligibilityReason(p, repo)}
	} else {
		result, err := verifyLearningResult(ctx, v.anchors, sha, head, paths)
		if err != nil {
			return verifyLearningCheck{}, err
		}
		check = verifyLearningCheck{result: result}
	}
	memo[p.Name] = check
	return check, nil
}

// verifyLearningResult answers the git questions for one eligible
// learning and folds them into a single result: missing when the pin is
// not in the repository or a cited path is absent at the pin, drifted
// when a cited path changed or was removed between the pin and HEAD, else
// anchors_ok (KV-FR-003). The decision order mirrors lint's
// checkLearningAnchors — unknown pin first, then per path existence and
// drift — but emits the verify enum instead of findings; the first path
// absent at the pin decides the result, so remaining paths are not
// queried. When one learning cites several paths, missing dominates
// drifted — the same precedence the compiled aggregate uses — so a
// citation broken at the pin is never reported as mere drift.
func verifyLearningResult(ctx context.Context, checker anchorChecker, sha, head string, paths []string) (string, error) {
	known, err := checker.CommitExists(ctx, sha)
	if err != nil {
		return "", err
	}
	if !known {
		return verifyMissing, nil
	}
	drifted := false
	for _, path := range paths {
		present, err := checker.PathExists(ctx, sha, path)
		if err != nil {
			return "", err
		}
		if !present {
			return verifyMissing, nil
		}
		changed, err := checker.PathChanged(ctx, sha, head, path)
		if err != nil {
			return "", err
		}
		if changed {
			drifted = true
		}
	}
	if drifted {
		return verifyDrifted, nil
	}
	return verifyAnchorsOK, nil
}

// aggregateSources folds per-source results into a compiled article's
// aggregate: missing if any source is missing, else drifted if any is
// drifted, else anchors_ok if any is anchors_ok, else no_anchors
// (KV-FR-003, design D3). An empty list aggregates to no_anchors — an
// article with no resolvable source has nothing checkable.
func aggregateSources(sources []verifySourceResult) string {
	anyDrifted, anyOK := false, false
	for _, s := range sources {
		switch s.Result {
		case verifyMissing:
			return verifyMissing
		case verifyDrifted:
			anyDrifted = true
		case verifyAnchorsOK:
			anyOK = true
		}
	}
	switch {
	case anyDrifted:
		return verifyDrifted
	case anyOK:
		return verifyAnchorsOK
	default:
		return verifyNoAnchors
	}
}

// boundSources caps the per-source results reported for one compiled
// article at maxReportedSources and returns the count left unreported
// (KV-FR-003 pinned advisory: sources is untrusted frontmatter). The
// aggregate folds every source regardless of the cap.
func boundSources(sources []verifySourceResult) (reported []verifySourceResult, omitted int) {
	if len(sources) <= maxReportedSources {
		return sources, 0
	}
	return sources[:maxReportedSources], len(sources) - maxReportedSources
}

// pageEvidence returns a page's evidence locators as stored. The values
// are untrusted frontmatter data — store_learning normalizes them, but
// re-ingested files bypass that validation — so they are only ever
// presented through presentEvidence's bounds.
func pageEvidence(p *store.Page) []string {
	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return nil
	}
	raw, _ := props["evidence"].([]any)
	locators := make([]string, 0, len(raw))
	for _, e := range raw {
		if locator, ok := e.(string); ok {
			locators = append(locators, locator)
		}
	}
	return locators
}

// presentEvidence bounds an evidence list for presentation (KV-FR-003
// pinned advisory): empty locators are dropped, the rest are
// deduplicated preserving first-seen order, each is cut to maxEvidenceLen
// runes with an ellipsis, and the list is cut to maxEvidenceEntries with
// a final "…and N more" marker. These are the same limits learningAnchors
// re-checks for eligibility, applied again at presentation, because
// evidence locators are untrusted frontmatter data.
func presentEvidence(locators []string) []string {
	seen := make(map[string]bool, len(locators))
	deduped := make([]string, 0, len(locators))
	for _, locator := range locators {
		if locator == "" || seen[locator] {
			continue
		}
		seen[locator] = true
		deduped = append(deduped, locator)
	}
	bounded := deduped
	omitted := 0
	if len(deduped) > maxEvidenceEntries {
		bounded = deduped[:maxEvidenceEntries]
		omitted = len(deduped) - maxEvidenceEntries
	}
	presented := make([]string, 0, len(bounded)+1)
	for _, locator := range bounded {
		presented = append(presented, truncateLocator(locator))
	}
	if omitted > 0 {
		presented = append(presented, fmt.Sprintf("…and %d more", omitted))
	}
	return presented
}

// truncateLocator cuts a locator over maxEvidenceLen runes to that length
// plus an ellipsis, so an over-long locator stays identifiable without
// ballooning the presented evidence. The bound is in runes, matching
// store_learning's limit.
func truncateLocator(locator string) string {
	if utf8.RuneCountInString(locator) <= maxEvidenceLen {
		return locator
	}
	return string([]rune(locator)[:maxEvidenceLen]) + "…"
}

// learningIneligibilityReason explains why learningAnchors rejected p,
// mirroring its checks in order. learningAnchors stays the eligibility
// authority — this companion only diagnoses a page learningAnchors
// already rejected, so the two cannot disagree about eligibility — and
// its wording feeds the no_anchors context the pinned advisory requires
// for stored-but-ineligible sources. It returns a reason naming the
// missing path-form anchors when p looks otherwise eligible.
func learningIneligibilityReason(p *store.Page, repo string) string {
	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return "properties are unreadable"
	}
	if validity, _ := props["validity"].(string); validity == "superseded" || validity == "historical" {
		return fmt.Sprintf("validity is %s", validity)
	}
	asOf, _ := props["as_of"].(string)
	pinRepo, sha, ok := splitAsOf(asOf)
	switch {
	case !ok:
		return "no readable as_of pin"
	case pinRepo != repo:
		return fmt.Sprintf("as_of pins repository %q, not the vault repository %q", pinRepo, repo)
	case !anchorCommitSHA.MatchString(sha):
		return "as_of pin is not a commit SHA"
	}
	evidence, _ := props["evidence"].([]any)
	if len(evidence) > maxEvidenceEntries {
		return fmt.Sprintf("evidence has %d entries; at most %d are allowed", len(evidence), maxEvidenceEntries)
	}
	for _, e := range evidence {
		locator, _ := e.(string)
		if utf8.RuneCountInString(locator) > maxEvidenceLen {
			return "an evidence locator exceeds the re-checked length limit"
		}
	}
	return fmt.Sprintf("no %s:<path> evidence locators", repo)
}
