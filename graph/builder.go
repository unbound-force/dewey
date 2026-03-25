package graph

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/parser"
	"github.com/unbound-force/dewey/types"
)

// Cache holds a recently built graph to avoid rebuilding on every analyze call.
type Cache struct {
	mu      sync.Mutex
	graph   *Graph
	built   time.Time
	ttl     time.Duration
	backend backend.Backend
}

// NewCache creates a graph cache with the given TTL backed by the provided
// [backend.Backend]. Returns a Cache that lazily builds the graph on the
// first [Cache.Get] call and re-uses it until the TTL expires.
func NewCache(b backend.Backend, ttl time.Duration) *Cache {
	return &Cache{
		backend: b,
		ttl:     ttl,
	}
}

// Get returns the cached graph if it was built within the TTL, or builds
// a fresh one by calling [Build]. Returns an error if the graph build
// fails. Safe for concurrent use (serialized via mutex).
func (c *Cache) Get(ctx context.Context) (*Graph, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.graph != nil && time.Since(c.built) < c.ttl {
		return c.graph, nil
	}

	g, err := Build(ctx, c.backend)
	if err != nil {
		return nil, err
	}

	c.graph = g
	c.built = time.Now()
	return g, nil
}

// Invalidate clears the cached graph, forcing the next [Cache.Get] call
// to rebuild it from the backend. Safe for concurrent use.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.graph = nil
}

// Graph is an in-memory representation of the knowledge graph's link structure.
type Graph struct {
	// Forward links: page name (lowercase) → set of linked page names (original case)
	Forward map[string]map[string]bool
	// Backward links: page name (lowercase) → set of pages that link to it
	Backward map[string]map[string]bool
	// Pages: lowercase name → PageEntity
	Pages map[string]types.PageEntity
	// BlockCounts: lowercase name → total block count
	BlockCounts map[string]int
}

// Build fetches all pages and their block trees from the backend and
// constructs an in-memory link graph. Returns the graph with forward links,
// backward links, page metadata, and block counts populated. Pages with
// empty names are skipped; block tree fetch errors are logged and skipped
// (partial graph). Returns an error if GetAllPages fails.
func Build(ctx context.Context, c backend.Backend) (*Graph, error) {
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		return nil, err
	}

	g := &Graph{
		Forward:     make(map[string]map[string]bool),
		Backward:    make(map[string]map[string]bool),
		Pages:       make(map[string]types.PageEntity),
		BlockCounts: make(map[string]int),
	}

	for _, page := range pages {
		if page.Name == "" {
			continue
		}
		key := strings.ToLower(page.Name)
		g.Pages[key] = page

		// Ensure entries exist even for pages with no links
		if g.Forward[key] == nil {
			g.Forward[key] = make(map[string]bool)
		}

		blocks, err := c.GetPageBlocksTree(ctx, page.Name)
		if err != nil {
			continue
		}

		g.BlockCounts[key] = countBlocksRecursive(blocks)
		extractLinksRecursive(blocks, key, g)
	}

	return g, nil
}

func countBlocksRecursive(blocks []types.BlockEntity) int {
	count := len(blocks)
	for _, b := range blocks {
		if len(b.Children) > 0 {
			count += countBlocksRecursive(b.Children)
		}
	}
	return count
}

func extractLinksRecursive(blocks []types.BlockEntity, sourceKey string, g *Graph) {
	for _, b := range blocks {
		parsed := parser.Parse(b.Content)
		for _, link := range parsed.Links {
			linkKey := strings.ToLower(link)
			g.Forward[sourceKey][link] = true

			if g.Backward[linkKey] == nil {
				g.Backward[linkKey] = make(map[string]bool)
			}
			g.Backward[linkKey][sourceKey] = true
		}
		if len(b.Children) > 0 {
			extractLinksRecursive(b.Children, sourceKey, g)
		}
	}
}

// OutDegree returns the number of outgoing links from the named page.
// Returns 0 if the page does not exist in the graph.
func (g *Graph) OutDegree(name string) int {
	return len(g.Forward[strings.ToLower(name)])
}

// InDegree returns the number of incoming links to the named page.
// Returns 0 if the page does not exist in the graph or has no incoming links.
func (g *Graph) InDegree(name string) int {
	return len(g.Backward[strings.ToLower(name)])
}

// TotalDegree returns the sum of outgoing and incoming link counts for
// the named page. Returns 0 if the page does not exist in the graph.
func (g *Graph) TotalDegree(name string) int {
	return g.OutDegree(name) + g.InDegree(name)
}

// OriginalName returns the original display name for a page identified by
// its lowercase key. Returns the key itself if the page is not found or
// has no OriginalName set.
func (g *Graph) OriginalName(key string) string {
	if p, ok := g.Pages[key]; ok && p.OriginalName != "" {
		return p.OriginalName
	}
	return key
}
