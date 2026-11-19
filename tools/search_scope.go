package tools

import (
	"strings"

	"github.com/unbound-force/dewey/v3/store"
)

// pageScope is the optional source_id / has_tag scope accepted by the
// full-text search and find_by_tag tools. Its semantics mirror
// store.SearchFilters as applied by semantic_search_filtered: SourceID
// matches the page's source_id exactly; HasTag matches when the page's
// properties JSON contains the tag or the result's block content
// contains "#tag", both ASCII case-insensitive like SQLite LIKE.
type pageScope struct {
	SourceID string
	HasTag   string
}

// active reports whether any scope filter is set.
func (p pageScope) active() bool {
	return p.SourceID != "" || p.HasTag != ""
}

// scopedFetchLimit is how many full-text hits to request before scope
// filtering: the limit itself when unscoped, otherwise max(10×limit, 200)
// so a scope matching a small share of the corpus still fills the limit.
func scopedFetchLimit(limit int, scope pageScope) int {
	if !scope.active() {
		return limit
	}
	return max(limit*10, 200)
}

// scopeMatcher resolves page scope against the persistent store, caching
// one page lookup per page name.
type scopeMatcher struct {
	scope pageScope
	store *store.Store
	pages map[string]*store.Page
}

// newScopeMatcher returns a matcher for scope. The store must be non-nil
// when scope is active; callers check that first.
func newScopeMatcher(scope pageScope, st *store.Store) *scopeMatcher {
	return &scopeMatcher{scope: scope, store: st, pages: make(map[string]*store.Page)}
}

// matches reports whether a result on pageName with block content
// satisfies the scope. An inactive scope matches everything. A page that
// is not in the store (or fails to load) never matches an active scope,
// the same way the SQL join in SearchSimilarFiltered drops it.
func (m *scopeMatcher) matches(pageName, content string) bool {
	if !m.scope.active() {
		return true
	}
	page := m.page(pageName)
	if page == nil {
		return false
	}
	if m.scope.SourceID != "" && page.SourceID != m.scope.SourceID {
		return false
	}
	if m.scope.HasTag != "" {
		tag := strings.ToLower(m.scope.HasTag)
		if !strings.Contains(strings.ToLower(page.Properties), tag) &&
			!strings.Contains(strings.ToLower(content), "#"+tag) {
			return false
		}
	}
	return true
}

// page returns the store page for name (case-insensitive, matching the
// vault's page keying), or nil.
func (m *scopeMatcher) page(name string) *store.Page {
	key := strings.ToLower(name)
	if p, ok := m.pages[key]; ok {
		return p
	}
	p, err := m.store.GetPageFoldCase(name)
	if err != nil {
		p = nil
	}
	m.pages[key] = p
	return p
}

// scopeUnavailableMessage is returned when a scope filter is requested
// but no persistent store is configured to resolve page sources.
const scopeUnavailableMessage = "source_id/has_tag filters unavailable: no persistent store configured."
