package tools

import (
	"encoding/json"
	"sort"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Validity-aware retrieval (issue #148).
//
// Design decision: ranking is a stable sort by similarity multiplied by a
// fixed validity weight, applied after the store's similarity query; the
// reported similarity stays raw so scores remain comparable across calls.
// Historical knowledge is not penalized — it is immune to code evolution.

// validityWeights maps validity statuses to ranking weights. Statuses not
// listed (including absent, current, historical) weigh 1.0.
var validityWeights = map[string]float64{
	"superseded":       0.6,
	"stale-unverified": 0.85,
}

// validityWeight returns the ranking weight for a validity status.
func validityWeight(validity string) float64 {
	if w, ok := validityWeights[validity]; ok {
		return w
	}
	return 1.0
}

// rankByValidity stably orders results by similarity × validity weight.
func rankByValidity(results []types.SemanticSearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Similarity*validityWeight(results[i].Validity) >
			results[j].Similarity*validityWeight(results[j].Validity)
	})
}

// filterByValidity keeps results whose page validity equals validity. An
// empty validity keeps everything.
func filterByValidity(results []types.SemanticSearchResult, validity string) []types.SemanticSearchResult {
	if validity == "" {
		return results
	}
	kept := make([]types.SemanticSearchResult, 0, len(results))
	for _, r := range results {
		if r.Validity == validity {
			kept = append(kept, r)
		}
	}
	return kept
}

// semanticFetchLimit returns how many results to request from the store:
// the limit itself when results are used as-is, otherwise max(3×limit, 30)
// so post-filtering and re-ranking can draw on results below the limit.
func semanticFetchLimit(limit int, overFetch bool) int {
	if !overFetch {
		return limit
	}
	return max(limit*3, 30)
}

// finishSemanticResults applies the validity filter and ranking, then trims
// to limit.
func finishSemanticResults(results []types.SemanticSearchResult, validity string, ignoreValidity bool, limit int) []types.SemanticSearchResult {
	results = filterByValidity(results, validity)
	if !ignoreValidity {
		rankByValidity(results)
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// pageValidity returns the string validity and as_of properties of a page.
func pageValidity(page *store.Page) (validity, asOf string) {
	var props map[string]any
	if err := json.Unmarshal([]byte(page.Properties), &props); err != nil {
		return "", ""
	}
	validity, _ = props["validity"].(string)
	asOf, _ = props["as_of"].(string)
	return validity, asOf
}
