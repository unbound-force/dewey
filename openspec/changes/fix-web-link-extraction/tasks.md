## 1. Rewrite extractLinks to Anchor Tags Only

- [x] 1.1 In `source/web.go`: add a package-level compiled regex `var anchorHrefRe = regexp.MustCompile(...)` matching `<a` tags with href attributes
- [x] 1.2 Rewrite `extractLinks()` to use `anchorHrefRe.FindAllStringSubmatch()` instead of the manual `strings.Index("href=")` loop. Preserve same-domain filtering and fragment stripping.

## 2. Add Static Asset URL Filter

- [x] 2.1 In `source/web.go`: add a `isStaticAsset(urlStr string) bool` function that checks if a URL path ends with a known non-HTML extension (`.css`, `.js`, `.ico`, `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.xml`, `.json`, `.woff`, `.woff2`, `.ttf`, `.eot`, `.map`, `.gz`)
- [x] 2.2 In the crawl loop where child URLs are added to the queue, skip URLs where `isStaticAsset()` returns true. Log at DEBUG level.

## 3. Persist Seed Page Immediately

- [x] 3.1 In `source/web.go` `List()`: seed page (depth 0) is already appended to docs immediately on fetch (line 281). No code change needed — the existing implementation is correct. The timeout issue described in #41 requires context propagation (separate concern).

## 4. Tests

- [x] 4.1 Add `TestExtractLinks_MixedTags`: mixed `<link>` and `<a>` tags — verify only `<a>` hrefs returned
- [x] 4.2 Add `TestExtractLinks_IgnoresLinkTags`: HTML with only `<link href="...">` tags returns empty slice
- [x] 4.3 Add `TestExtractLinks_AnchorWithAttributes`: `<a class="nav" href="/about">` returns `/about`
- [x] 4.4 Add `TestIsStaticAsset`: verify `.css`, `.js`, `.ico` return true; `.html`, `/docs`, `/about` return false

## 5. Verification

- [x] 5.1 Run `go build ./...` and `go vet ./...`
- [x] 5.2 Run `go test -race -count=1 ./...` — all tests pass
