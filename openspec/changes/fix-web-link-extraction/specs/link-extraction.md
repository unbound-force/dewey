## MODIFIED Requirements

### Requirement: Link Extraction from Anchor Tags Only

`extractLinks()` MUST only extract href values from `<a>` tags. Hrefs from `<link>`, `<area>`, `<base>`, and other non-anchor tags MUST be ignored.

Previously: All `href=` attributes were extracted regardless of tag type.

#### Scenario: HTML with mixed tag types
- **GIVEN** HTML containing `<link href="style.css">` and `<a href="/docs">Docs</a>`
- **WHEN** `extractLinks` is called
- **THEN** only `/docs` is returned (not `style.css`)

#### Scenario: Anchor tag with attributes before href
- **GIVEN** HTML containing `<a class="nav" href="/about">About</a>`
- **WHEN** `extractLinks` is called
- **THEN** `/about` is returned

### Requirement: Static Asset URL Filtering

URLs ending with known non-HTML extensions MUST be skipped before fetching. A DEBUG-level log MUST be emitted for each skipped URL.

#### Scenario: CSS file URL
- **GIVEN** an extracted URL ending in `.css`
- **WHEN** the URL is evaluated for crawling
- **THEN** it is skipped and not fetched

### Requirement: Seed Page Persistence

The seed page (depth 0) MUST be persisted as a document immediately after successful fetch, independent of child page crawling completion.

Previously: The seed page was only included in results when List() completed all crawling.

#### Scenario: Child crawling timeout
- **GIVEN** the seed page is fetched successfully
- **WHEN** child page crawling times out
- **THEN** the seed page document is still included in the returned documents
