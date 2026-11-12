package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// maxSupersessionHops bounds the cycle walk's node expansions. Each
// learning declares at most one supersedes and one superseded_by link, so
// each expansion enqueues at most two identities; the bound plus the
// visited set guarantees the walk terminates on any chain, including a
// pre-existing cycle (ES-FR-008).
const maxSupersessionHops = 64

// SupersedeLearning links two existing learnings, successor and target,
// through the same guarded write paths as store_learning supersession
// (ES-FR-007): the compare-and-swap store claim, the guarded
// markdown-backup leg, and one withLearningPropsLock hold across both
// legs. The successor gains supersedes naming the target; the target
// gains superseded_by naming the successor and validity: superseded. The
// target is never deleted and its tier never changes.
//
// The full validation ladder (ES-FR-008) runs once before the lock as a
// fast-fail, then again inside the hold on freshly read state before the
// first write: a concurrent reverse-pair call (A→B vs B→A) serializes
// behind the hold, and the second call's in-hold ladder observes the
// first call's writes and refuses, so both directions of a pair can
// never be written. A writer that changed either side between validation
// and write is refused or routed to the lost-claim outcome rather than
// overwritten. Partial failures after the first write — a store-claim
// error, a lost claim, or a file-leg failure — are reported through
// supersedes_warning, matching store_learning semantics (ES-FR-009).
func (l *Learning) SupersedeLearning(_ context.Context, _ *mcp.CallToolRequest, input types.SupersedeLearningInput) (*mcp.CallToolResult, any, error) {
	successorID := supersedesIdentity(input.Successor)
	targetID := supersedesIdentity(input.Target)
	if successorID == "" {
		return errorResult("successor is required"), nil, nil
	}
	if targetID == "" {
		return errorResult("target is required"), nil, nil
	}

	// Fast-fail validation before the lock; the full ladder re-runs
	// inside the hold on fresh state before any write.
	if _, _, err := l.validateSupersession(successorID, targetID); err != nil {
		return errorResult(err.Error()), nil, nil
	}

	var warning string
	err := withLearningPropsLock(func() error {
		// Full ladder re-run inside the hold (ES-FR-007): existence,
		// both sides' link-state fast-fails, the reverse-pair check, and
		// the cycle walk, all on state a concurrent serialized call may
		// have just written.
		successor, target, err := l.validateSupersession(successorID, targetID)
		if err != nil {
			return err
		}
		// Successor leg, then the guarded target claim (ES-FR-007).
		// Warnings accumulate across the legs so a later leg's failure never
		// hides an earlier leg's (ES-FR-009 reports every failed leg).
		warning = joinWarnings(warning, l.recordSupersedes(successor, targetID))
		claimed, winner, claimErr := l.claimSuperseded(target, successorID)
		if claimErr != nil {
			warning = joinWarnings(warning, fmt.Sprintf("failed to mark %s superseded in the store: %v", target.Name, claimErr))
			return nil
		}
		if !claimed {
			warning = joinWarnings(warning, lostClaimWarning(target.Name, winner))
			return nil
		}
		warning = joinWarnings(warning, l.markSupersededFile(targetID, successorID))
		return nil
	})
	if err != nil {
		return errorResult(fmt.Sprintf("supersede_learning: %v", err)), nil, nil
	}

	result := map[string]any{
		"status":    "superseded",
		"successor": successorID,
		"target":    targetID,
	}
	if warning != "" {
		result["supersedes_warning"] = warning
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// joinWarnings accumulates partial-failure warnings from consecutive
// supersession write legs, so each leg's report stays visible when more
// than one leg degrades.
func joinWarnings(current, next string) string {
	switch {
	case current == "":
		return next
	case next == "":
		return current
	default:
		return current + "; " + next
	}
}

// validateSupersession runs the full ES-FR-008 ladder against current
// store state and returns the resolved successor and target pages: both
// identities must name learnings, be distinct, the target must record no
// superseded_by, the successor must declare no supersedes naming a
// different learning, the target must not already supersede the
// successor, and the bounded cycle walk from either learning over
// declared links in both directions must neither reach the other
// learning nor terminate on a pre-existing cycle. It is designed to run
// twice — once before the lock as a fast-fail, and again inside the
// withLearningPropsLock hold on freshly read state before the first
// write, so a concurrent call serialized behind the hold has its writes
// observed and refused rather than overlaid.
func (l *Learning) validateSupersession(successorID, targetID string) (*store.Page, *store.Page, error) {
	if successorID == targetID {
		return nil, nil, fmt.Errorf("successor and target are the same learning: %q", successorID)
	}
	successor, err := l.resolveLearningPage(successorID, "successor")
	if err != nil {
		return nil, nil, err
	}
	target, err := l.resolveLearningPage(targetID, "target")
	if err != nil {
		return nil, nil, err
	}
	succProps, err := decodeLearningProperties(successor.Properties)
	if err != nil {
		return nil, nil, fmt.Errorf("learning %q has unreadable properties: %w", successorID, err)
	}
	tgtProps, err := decodeLearningProperties(target.Properties)
	if err != nil {
		return nil, nil, fmt.Errorf("learning %q has unreadable properties: %w", targetID, err)
	}
	if err := checkLinkStateLadder(succProps, tgtProps, successorID, targetID); err != nil {
		return nil, nil, err
	}

	// Bounded cycle walk from either learning (ES-FR-008). The walk
	// ignores the requested pair's own forward link — the successor's
	// supersedes naming the target, whether already declared (the
	// idempotent half-link this call completes) or about to be written —
	// so completing a half-linked pair is not read as a cycle.
	for _, walk := range []struct{ start, other string }{
		{successorID, targetID},
		{targetID, successorID},
	} {
		reaches, cycle := l.walkSupersessionLinks(walk.start, walk.other, successorID, targetID)
		if reaches {
			return nil, nil, fmt.Errorf("following declared supersession links from %q reaches %q: the link would close a cycle", walk.start, walk.other)
		}
		if cycle != nil {
			return nil, nil, fmt.Errorf("pre-existing supersession cycle %s: refusing to extend a corrupt chain", strings.Join(cycle, " → "))
		}
	}
	return successor, target, nil
}

// checkLinkStateLadder applies the three ES-FR-008 link-state refusals to
// the pair's decoded properties: the target must record no superseded_by,
// the successor must declare no supersedes naming a different learning,
// and the target must not already supersede the successor (the direct
// reverse edge; the cycle walk covers every longer path).
func checkLinkStateLadder(succProps, tgtProps map[string]any, successorID, targetID string) error {
	targetSupersededBy, err := declaredLink(tgtProps, "superseded_by", targetID)
	if err != nil {
		return err
	}
	if targetSupersededBy != "" {
		return fmt.Errorf("learning %q is already superseded by %q", targetID, targetSupersededBy)
	}
	successorSupersedes, err := declaredLink(succProps, "supersedes", successorID)
	if err != nil {
		return err
	}
	if successorSupersedes != "" && successorSupersedes != targetID {
		return fmt.Errorf("learning %q already declares supersedes %q", successorID, successorSupersedes)
	}
	targetSupersedes, err := declaredLink(tgtProps, "supersedes", targetID)
	if err != nil {
		return err
	}
	if targetSupersedes == successorID {
		return fmt.Errorf("learning %q already supersedes %q: linking %q → %q would close a cycle", targetID, successorID, successorID, targetID)
	}
	return nil
}

// resolveLearningPage returns the learning page that identity names. role
// labels the error messages ("successor" or "target") so refusions name
// which side failed to resolve.
func (l *Learning) resolveLearningPage(identity, role string) (*store.Page, error) {
	if !ValidLearningIdentity(identity) {
		return nil, fmt.Errorf("%s: learning %q not found", role, identity)
	}
	page, err := l.store.GetPage(store.LearningPrefix + identity)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to look up learning %q: %v", role, identity, err)
	}
	if page == nil || page.SourceID != store.SourceLearning {
		return nil, fmt.Errorf("%s: learning %q not found", role, identity)
	}
	return page, nil
}

// declaredLink reads one declared link (supersedes or superseded_by) from
// a learning's decoded properties, normalized to a bare identity. Empty
// and absent values return "". A present value that is not a string, or
// does not normalize to a valid identity, is an error — the ladder and
// the store claim must agree on link state, and claimSuperseded already
// treats a non-string superseded_by as a hard error.
func declaredLink(props map[string]any, key, identity string) (string, error) {
	raw, present := props[key]
	if !present || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("learning %q records a non-string %s value", identity, key)
	}
	if s == "" {
		return "", nil
	}
	id := supersedesIdentity(s)
	if id == "" {
		return "", fmt.Errorf("learning %q records an invalid %s value %q", identity, key, s)
	}
	return id, nil
}

// walkSupersessionLinks follows declared supersession links — supersedes
// and superseded_by, both directions — breadth-first from start, with a
// visited set and a maxSupersessionHops bound on expansions, keeping
// parent pointers for cycle reconstruction. The edge ignoreFrom
// supersedes ignoreTo (the supersedes representation only) is skipped —
// the requested pair's own forward link, which is either the idempotent
// half-link this call completes or the link it is about to write, never
// a cycle-closing back-edge. A completed pair stores one logical edge as
// two representations (successor.supersedes = target plus
// target.superseded_by = successor); walking back along the reverse
// representation of the arrival edge is skipped so healthy completed
// pairs are not read as 2-cycles and chains remain extendable — a
// distinct second edge back to the parent (both directions declaring
// supersedes, or superseded_by disagreements) is still followed.
//
// The walk reports whether it reaches other (linking the pair would
// close a cycle) and the identity chain of a pre-existing cycle when one
// is found: a revisit of a node that is an ancestor of its re-visitor is
// a true back-edge cycle, while a revisit across branches is a diamond
// (two learnings converging on one successor), which is not a cycle.
// The visited set is what terminates a pre-existing cyclic chain — the
// walk stops at the first revisit rather than spinning (ES-FR-008). An
// unreadable chain node ends that branch; the ladder surfaces unreadable
// properties on the pair itself before walking.
func (l *Learning) walkSupersessionLinks(start, other, ignoreFrom, ignoreTo string) (reachesOther bool, cycle []string) {
	visited := map[string]bool{start: true}
	parent := map[string]string{}
	// arrivalViaSupersedes records how each visited node was reached:
	// true along its parent's supersedes (parent is the successor),
	// false along its parent's superseded_by (parent is the target).
	arrivalViaSupersedes := map[string]bool{}
	queue := []string{start}
	for hops := 0; hops < maxSupersessionHops && len(queue) > 0; hops++ {
		current := queue[0]
		queue = queue[1:]
		links, err := l.declaredLinks(current)
		if err != nil {
			continue
		}
		for _, link := range links {
			// The requested pair's own forward link — the successor's
			// supersedes naming the target, whether already declared
			// (the idempotent half-link this call completes) or about
			// to be written — is ignored wherever it appears. Only the
			// supersedes representation is ignored: a successor that
			// already records the target as ITS successor
			// (superseded_by) is a genuine reverse declaration and is
			// followed, refusing the call as cycle-closing.
			if current == ignoreFrom && link.fromSupersedes && link.neighbor == ignoreTo {
				continue
			}
			// A completed pair is stored as two representations of ONE
			// logical edge: successor.supersedes = target plus
			// target.superseded_by = successor. Walking back along the
			// reverse representation of the arrival edge would read
			// every healthy completed pair as a 2-cycle and block chain
			// extension, so only that mutual reverse is skipped — a
			// disagreement (a second, distinct edge back to the parent,
			// e.g. both directions declaring supersedes) is still
			// followed and reported as a cycle.
			if current != start && link.neighbor == parent[current] &&
				link.fromSupersedes != arrivalViaSupersedes[current] {
				continue
			}
			next := link.neighbor
			if next == other {
				return true, nil
			}
			if visited[next] {
				if chain := ancestorPath(parent, current, next); chain != nil {
					return false, append(chain, next)
				}
				continue
			}
			visited[next] = true
			parent[next] = current
			arrivalViaSupersedes[next] = link.fromSupersedes
			queue = append(queue, next)
		}
	}
	return false, nil
}

// declaredLinkEdge is one declared supersession adjacency of a learning:
// the neighbor's bare identity and the property the link was read from.
// fromSupersedes distinguishes the two stored representations of a
// logical edge (supersedes names the target; superseded_by names the
// successor), which the walk needs to collapse mutual pairs.
type declaredLinkEdge struct {
	neighbor       string
	fromSupersedes bool
}

// declaredLinks reads a learning's declared supersedes and superseded_by
// links, normalized to bare identities, skipping empty values.
func (l *Learning) declaredLinks(identity string) ([]declaredLinkEdge, error) {
	page, err := l.store.GetPage(store.LearningPrefix + identity)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return nil, nil
	}
	props, err := decodeLearningProperties(page.Properties)
	if err != nil {
		return nil, err
	}
	var links []declaredLinkEdge
	for _, key := range []string{"supersedes", "superseded_by"} {
		if id, err := declaredLink(props, key, identity); err == nil && id != "" {
			links = append(links, declaredLinkEdge{
				neighbor:       id,
				fromSupersedes: key == "supersedes",
			})
		}
	}
	return links, nil
}

// ancestorPath follows parent pointers from node back to candidate and
// returns the path [candidate, ..., node] when candidate is an ancestor
// of node in the walk's tree, else nil.
func ancestorPath(parent map[string]string, node, candidate string) []string {
	var path []string
	for current := node; ; {
		path = append(path, current)
		if current == candidate {
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		p, ok := parent[current]
		if !ok {
			return nil
		}
		current = p
	}
}
