## Why

The web crawler's `extractLinks()` searches for `href="` anywhere in the HTML, matching `<link>` tags (stylesheets, favicons, XML) alongside `<a>` tags (navigation links). On sites like `pkg.go.dev`, static asset `<link>` tags appear first in `<head>`, consuming the crawler's rate-limited budget on CSS/favicon fetches that fail with non-HTML content type. The result: 0 pages indexed for Go stdlib documentation.

Additionally, when child crawling times out, the successfully-fetched seed page is discarded along with the incomplete children — losing the most valuable page.

Fixes GitHub issue #41.

## What Changes

1. Replace the raw string search in `extractLinks()` with Go's `html.Tokenizer` to only extract hrefs from `<a>` tags
2. Pre-filter extracted URLs to skip known static asset extensions (`.css`, `.js`, `.ico`, `.png`, `.svg`, `.xml`, etc.) before fetching
3. Persist the seed page (depth 0) immediately after successful fetch, independent of child crawling completion

## Capabilities

### Modified Capabilities
- `extractLinks()`: Now uses `html.Tokenizer` to only extract `<a>` tag hrefs (was: matched any tag with `href=`)
- `WebSource.List()`: Pre-filters URLs by extension before adding to crawl queue
- `WebSource.List()`: Persists seed page document immediately after fetch, not batched with children

## Impact

- **source/web.go**: `extractLinks()` rewritten, URL extension filter added, seed persistence change
- **source/web_test.go**: New tests for `<a>`-only extraction, extension filtering, seed persistence
- No API changes, no schema changes, no new dependencies (`html.Tokenizer` is stdlib `golang.org/x/net/html`)

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: PASS — MCP tool interface unchanged.

### II. Composability First
**Assessment**: PASS — `html.Tokenizer` is from `golang.org/x/net/html` which is quasi-stdlib. Check if already in go.mod.

### III. Observable Quality
**Assessment**: PASS — Previously invisible failure (0 pages) becomes correct indexing. Static asset skips logged at DEBUG level.

### IV. Testability
**Assessment**: PASS — `extractLinks` is a pure function, easily testable with HTML string fixtures.
