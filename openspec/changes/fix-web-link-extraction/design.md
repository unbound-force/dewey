## Context

`extractLinks()` in `source/web.go` uses `strings.Index(lower, "href=\"")` to find all hrefs in HTML. This matches `<link>`, `<a>`, `<area>`, and any other tag with href attributes. On `pkg.go.dev`, `<link>` tags for CSS/favicons dominate, producing zero useful navigation links.

## Goals / Non-Goals

### Goals
- Extract hrefs only from `<a>` tags
- Skip known static asset URLs before fetching
- Persist the seed page immediately, independent of child crawling

### Non-Goals
- Adding a full HTML parser dependency (avoid `golang.org/x/net/html`)
- Changing the crawl depth or rate limiting behavior
- Supporting non-`<a>` navigation patterns (JavaScript-rendered links, etc.)

## Decisions

**D1: Use `regexp` to match `<a` tags.** Replace the `strings.Index("href=")` approach with a compiled regex: `<a\s[^>]*href="([^"]*)"`. This matches only `<a>` tags, handles attributes before href (e.g., `<a class="foo" href="...">`), and is stdlib-only (no new dependency). The regex is compiled once as a package-level `var`.

**D2: Static asset extension filter.** Before adding a URL to the crawl queue, check if it ends with a known non-HTML extension: `.css`, `.js`, `.ico`, `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.xml`, `.json`, `.woff`, `.woff2`, `.ttf`, `.eot`, `.map`, `.gz`. Skip with a DEBUG log. This is a deny-list, not an allow-list — new page types are crawled by default.

**D3: Persist seed page on fetch.** After the seed page (depth 0) is successfully fetched and converted to text, add it to the documents list immediately. If child crawling times out or fails, the seed page is already in the result. This is a behavioral change: previously, the seed was only included when `List()` returned all pages.

## Risks / Trade-offs

**Risk: Regex edge cases.** The regex `<a\s[^>]*href="([^"]*)"` won't match single-quoted hrefs (`href='...'`) or unquoted hrefs. This is acceptable — well-formed HTML uses double quotes, and the existing code already assumes double quotes.

**Trade-off: Deny-list maintenance.** New static asset extensions could bypass the filter. The content-type check downstream catches them, so the filter is an optimization (avoid wasting rate-limited requests), not a correctness requirement.
