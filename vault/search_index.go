package vault

import (
	"sort"
	"strings"
	"sync"

	"github.com/unbound-force/dewey/v3/parser"
	"github.com/unbound-force/dewey/v3/types"
)

// SearchIndex is a simple inverted index for full-text search.
// Maps lowercase terms to the blocks containing them.
type SearchIndex struct {
	mu sync.RWMutex
	// term → list of block references
	index map[string][]blockRef
	// page lowercase → set of terms (for efficient removal on reindex)
	pageTerms map[string]map[string]bool
}

// blockRef identifies a block within a page.
type blockRef struct {
	pageName string // original case page name
	uuid     string
	content  string
}

// NewSearchIndex creates an empty search index.
func NewSearchIndex() *SearchIndex {
	return &SearchIndex{
		index:     make(map[string][]blockRef),
		pageTerms: make(map[string]map[string]bool),
	}
}

// BuildFrom indexes all pages in the vault.
func (si *SearchIndex) BuildFrom(pages map[string]*cachedPage) {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.index = make(map[string][]blockRef)
	si.pageTerms = make(map[string]map[string]bool)

	seen := make(map[string]bool)
	for _, page := range pages {
		if seen[page.lowerName] {
			continue
		}
		seen[page.lowerName] = true
		si.indexPageLocked(page)
	}
}

// ReindexPage removes and re-adds a single page to the index.
func (si *SearchIndex) ReindexPage(page *cachedPage) {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.removePageLocked(page.lowerName)
	si.indexPageLocked(page)
}

// RemovePage removes a page from the index.
func (si *SearchIndex) RemovePage(lowerName string) {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.removePageLocked(lowerName)
}

// Search finds blocks matching all terms in the query (AND semantics).
// Exact-term matches come first. When the query has compound terms
// (containing `-` or `_`) and exact matches do not fill the limit, the
// rest is filled with blocks matching every component of those terms,
// so truncated and space-split forms of hyphenated IDs still find them
// (issue #149). The second return value is the number of pass-one
// exact-term matches, letting callers distinguish them from pass-two
// component fills when running higher-precision fallbacks.
func (si *SearchIndex) Search(query string, limit int) ([]SearchResult, int) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, 0
	}

	if limit <= 0 {
		limit = 20
	}

	results := si.matchAllLocked(terms, limit, nil)
	exact := len(results)
	expanded, compound := expandCompoundTerms(terms)
	if !compound || len(results) >= limit {
		return results, exact
	}

	seen := make(map[string]bool, len(results))
	for _, r := range results {
		seen[r.UUID] = true
	}
	return append(results, si.matchAllLocked(expanded, limit-len(results), seen)...), exact
}

// matchAllLocked returns up to limit blocks whose terms include every
// term, skipping blocks whose UUID is in skip. Caller holds si.mu.
func (si *SearchIndex) matchAllLocked(terms []string, limit int, skip map[string]bool) []SearchResult {
	// Find blocks that contain ALL terms.
	// Start with the rarest term for efficiency.
	var rarest string
	rarestCount := int(^uint(0) >> 1) // max int
	for _, t := range terms {
		if count := len(si.index[t]); count < rarestCount {
			rarestCount = count
			rarest = t
		}
	}

	// Candidates from rarest term.
	candidates := si.index[rarest]
	if len(candidates) == 0 {
		return nil
	}

	// Build a set of UUIDs for each other term.
	otherTermSets := make([]map[string]bool, 0, len(terms)-1)
	for _, t := range terms {
		if t == rarest {
			continue
		}
		set := make(map[string]bool)
		for _, ref := range si.index[t] {
			set[ref.uuid] = true
		}
		otherTermSets = append(otherTermSets, set)
	}

	// Filter candidates that appear in ALL other term sets.
	var results []SearchResult
	for _, ref := range candidates {
		inAll := true
		for _, set := range otherTermSets {
			if !set[ref.uuid] {
				inAll = false
				break
			}
		}
		if !inAll || skip[ref.uuid] {
			continue
		}

		results = append(results, SearchResult{
			PageName: ref.pageName,
			UUID:     ref.uuid,
			Content:  ref.content,
		})

		if len(results) >= limit {
			break
		}
	}

	return results
}

// SearchResult is a block that matched a search query.
type SearchResult struct {
	PageName string `json:"page"`
	UUID     string `json:"uuid"`
	Content  string `json:"content"`
}

// --- Internal ---

func (si *SearchIndex) indexPageLocked(page *cachedPage) {
	terms := make(map[string]bool)
	si.indexBlocksLocked(page.blocks, page.entity.OriginalName, terms)
	si.pageTerms[page.lowerName] = terms
}

func (si *SearchIndex) indexBlocksLocked(blocks []types.BlockEntity, pageName string, terms map[string]bool) {
	for _, b := range blocks {
		blockTerms := withCompoundComponents(tokenize(b.Content))

		// Also index parsed links and tags as terms.
		parsed := parser.Parse(b.Content)
		for _, link := range parsed.Links {
			blockTerms = append(blockTerms, tokenize(link)...)
		}
		for _, tag := range parsed.Tags {
			blockTerms = append(blockTerms, tokenize(tag)...)
		}

		ref := blockRef{
			pageName: pageName,
			uuid:     b.UUID,
			content:  b.Content,
		}

		seen := make(map[string]bool)
		for _, term := range blockTerms {
			if seen[term] {
				continue
			}
			seen[term] = true
			terms[term] = true
			si.index[term] = append(si.index[term], ref)
		}

		if len(b.Children) > 0 {
			si.indexBlocksLocked(b.Children, pageName, terms)
		}
	}
}

func (si *SearchIndex) removePageLocked(lowerName string) {
	terms, ok := si.pageTerms[lowerName]
	if !ok {
		return
	}

	for term := range terms {
		refs := si.index[term]
		filtered := refs[:0]
		for _, ref := range refs {
			if strings.ToLower(ref.pageName) != lowerName {
				filtered = append(filtered, ref)
			}
		}
		if len(filtered) == 0 {
			delete(si.index, term)
		} else {
			si.index[term] = filtered
		}
	}

	delete(si.pageTerms, lowerName)
}

// tokenize splits text into lowercase terms for indexing.
// Strips common markdown syntax and splits on whitespace + punctuation.
func tokenize(text string) []string {
	// Remove markdown syntax.
	text = strings.ReplaceAll(text, "[[", " ")
	text = strings.ReplaceAll(text, "]]", " ")
	text = strings.ReplaceAll(text, "((", " ")
	text = strings.ReplaceAll(text, "))", " ")
	text = strings.ReplaceAll(text, "#", " ")
	text = strings.ReplaceAll(text, "::", " ")
	text = strings.ReplaceAll(text, "**", " ")
	text = strings.ReplaceAll(text, "__", " ")
	text = strings.ReplaceAll(text, "`", " ")

	text = strings.ToLower(text)

	// Split on non-alphanumeric (Unicode-aware).
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !isWordChar(r)
	})

	// Filter very short terms and deduplicate.
	var terms []string
	for _, w := range words {
		if len(w) >= 2 { // skip single chars
			terms = append(terms, w)
		}
	}

	return terms
}

// splitCompound returns the components (at least 2 bytes each, matching
// tokenize's term-length convention) of a term containing `-` or `_`, or
// nil when the term is not compound or has no component long enough to
// index (issue #149).
func splitCompound(term string) []string {
	if !strings.ContainsAny(term, "-_") {
		return nil
	}
	var parts []string
	for _, p := range strings.FieldsFunc(term, func(r rune) bool { return r == '-' || r == '_' }) {
		if len(p) >= 2 {
			parts = append(parts, p)
		}
	}
	return parts
}

// withCompoundComponents returns terms followed by the components of each
// compound term, so a block is indexed under both `kb-review-adjud-01`
// and `kb`, `review`, `adjud`, `01`. When no term is compound, terms is
// returned unchanged.
func withCompoundComponents(terms []string) []string {
	out := terms
	copied := false
	for _, t := range terms {
		parts := splitCompound(t)
		if len(parts) == 0 {
			continue
		}
		if !copied {
			out = append([]string(nil), terms...)
			copied = true
		}
		out = append(out, parts...)
	}
	return out
}

// expandCompoundTerms replaces each compound query term with its
// components. Reports whether any term was replaced; a compound with no
// indexable component is kept whole.
func expandCompoundTerms(terms []string) ([]string, bool) {
	expanded := make([]string, 0, len(terms))
	changed := false
	for _, t := range terms {
		if parts := splitCompound(t); len(parts) > 0 {
			expanded = append(expanded, parts...)
			changed = true
			continue
		}
		expanded = append(expanded, t)
	}
	return expanded, changed
}

// isWordChar returns true for letters and digits (Unicode-aware).
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r > 127
}

// SortByRelevance sorts search results by term frequency.
// Not currently used but available for future ranking improvements.
func SortByRelevance(results []SearchResult, query string) {
	terms := tokenize(query)
	sort.Slice(results, func(i, j int) bool {
		iScore := countTermHits(results[i].Content, terms)
		jScore := countTermHits(results[j].Content, terms)
		return iScore > jScore
	})
}

func countTermHits(content string, terms []string) int {
	lower := strings.ToLower(content)
	count := 0
	for _, t := range terms {
		count += strings.Count(lower, t)
	}
	return count
}
