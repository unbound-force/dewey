## 1. Core Sanitize Package — Types and Orchestrator [depends: none]

- [x] 1.1 Create `sanitize/sanitize.go` with `ScanInput`, `ScanConfig`, `ScanResult`, `Finding`, `SourceStats` types and `Scan(input ScanInput, config ScanConfig) (*ScanResult, error)` orchestrator function following AP-001 Options/Result pattern. `Scan()` calls each scan layer and aggregates `[]Finding` into a `ScanResult` with `PatternVersion`, `Layers`, and `ScannedAt` fields. (FR-SAN-001, FR-SAN-007)
- [x] 1.2 Define `ScanConfig` struct with `Mode` (`warn`/`strict`/`off`), `InvisibleCharThreshold` (int, default 5), `Patterns` (`[]PatternRule`, defaults to `DefaultPatterns`). Define `ScanInput` struct with `Content`, `SourceID`, `DocumentID`, `SourceType`, `PreviousHash`, `CurrentHash` (strings), and `SourceStats *SourceStats` (optional). (FR-SAN-007, FR-SAN-008)
- [x] 1.3 Add package-level `charmbracelet/log` logger with `dewey/sanitize` prefix. Log findings with structured fields (`source`, `page`, `pattern`, `severity`, `category`, `line`) at WARN level (warn mode) or ERROR level (strict mode). (FR-SAN-001, FR-SAN-008)

## 2. Adversarial Keyword Scanning [depends: 1, parallel: 3,4,5]

- [x] 2.1 Create `sanitize/patterns.go` with `PatternRule` struct (compiled `*regexp.Regexp`, `Name string`, `Severity string`, `Description string`, `Version int`) and `DefaultPatterns` variable, plus `DefaultPatternVersion int` constant. (FR-SAN-002)
- [x] 2.2 Implement default pattern database with compiled regexes. Severity calibration: `critical` for high-specificity multi-word phrases (`(?i)ignore (?:all )?(?:previous|prior|above) (?:instructions|context|rules)`, `<\|im_start\|>`, `\[INST\]`, `<<SYS>>`), `high` for role reassignment (`(?i)you are now`, `(?i)pretend (?:to be|you are)`) and delimiter injection (`(?m)^(?:system|user|assistant):`), `medium` for context phrases (`(?i)act as if`, `(?i)system (?:prompt|message)`, `(?i)###\s*(?:system|instruction)`). (FR-SAN-002)
- [x] 2.3 Implement `ScanInjectionPatterns(content string, patterns []PatternRule) []Finding` — iterates over patterns, finds all matches with line numbers, produces `Finding` with category `"injection"` and up to 200 chars of surrounding context. (FR-SAN-002)
- [x] 2.4 Write tests `TestScanInjectionPatterns_KnownPayloads`, `TestScanInjectionPatterns_LegitimateSecurityDocs`, `TestScanInjectionPatterns_CleanContent`, `TestScanInjectionPatterns_LineNumbers`, `TestScanInjectionPatterns_Unicode`: known injection payloads (critical matches), legitimate security docs (matches but non-blocking), clean content (no matches), multi-line content with line number accuracy, Unicode content handling. (FR-SAN-002)

## 3. Content Hash Drift Detection [depends: 1, parallel: 2,4,5]

- [x] 3.1 Create `sanitize/drift.go` with `ContentDrift(previousHash, currentHash string) *Finding`. Returns nil when hashes match, nil when previousHash is empty (first index), or a finding with severity `info` and category `drift` when hashes differ. (FR-SAN-003)
- [x] 3.2 Write tests `TestContentDrift_HashChanged`, `TestContentDrift_HashUnchanged`, `TestContentDrift_FirstIndex`, `TestContentDrift_EmptyHashes`: hash changed (finding returned), unchanged (nil), first index with empty previous (nil), both empty (nil). (FR-SAN-003)

## 4. Markdown Structure Validation [depends: 1, parallel: 2,3,5]

- [x] 4.1 Create `sanitize/structure.go` with `ValidateStructure(content string, invisibleThreshold int) []Finding`. (FR-SAN-004)
- [x] 4.2 Implement invisible Unicode detection: scan for U+200B (zero-width space), U+200C (ZWNJ), U+200D (ZWJ), U+200E/U+200F (LTR/RTL marks), U+FEFF (BOM), U+2060 (word joiner), U+2061-U+2064 (invisible operators). Count occurrences and report finding if count exceeds threshold. (FR-SAN-004)
- [x] 4.3 Implement data URI detection: regex scan for `data:` URIs in Markdown image/link syntax (`![...](data:...)` or `[...](data:...)`). Report each as a finding with severity `high`. (FR-SAN-004)
- [x] 4.4 Implement heading depth validation: detect heading markers with more than 6 `#` characters. Report as finding with severity `low`. (FR-SAN-004)
- [x] 4.5 Implement suspicious HTML detection: scan for raw `<script>`, `<iframe>`, `<object>`, `<embed>`, `<form>` tags and HTML event handler attributes (`on\w+=`). Report each as a finding with severity `high`. (FR-SAN-004)
- [x] 4.6 Write tests `TestValidateStructure_InvisibleChars`, `TestValidateStructure_DataURI`, `TestValidateStructure_HeadingDepth`, `TestValidateStructure_SuspiciousHTML`, `TestValidateStructure_CleanContent`: content with zero-width characters (above/below threshold), data URIs (base64 encoded), invalid heading depth, embedded HTML tags, clean Markdown (no findings). (FR-SAN-004)

## 5. Content Size Anomaly Detection [depends: 1, parallel: 2,3,4]

- [x] 5.1 Create `sanitize/anomaly.go` with `SourceStats` struct (Mean, StdDev, Count float64) and `ComputeStats(lengths []int) SourceStats`. (FR-SAN-005)
- [x] 5.2 Implement `SizeAnomaly(contentLen int, stats SourceStats) (bool, *Finding)` — return true with finding (severity `medium`, category `anomaly`) when `contentLen > mean + 3*stddev` and `stats.Count >= 5`. Return false when `stats.Count < 5` or stats is zero-value. (FR-SAN-005)
- [x] 5.3 Write tests `TestSizeAnomaly_Detected`, `TestSizeAnomaly_Normal`, `TestSizeAnomaly_InsufficientSample`, `TestSizeAnomaly_UniformSizes`, `TestSizeAnomaly_ZeroStdDev`, `TestComputeStats`: content at 4 sigma (anomaly detected), content at 2 sigma (normal), source with 3 pages (skipped), source with uniform page sizes, edge case with zero standard deviation, stats computation accuracy. (FR-SAN-005)

## 6. Pipeline Integration [depends: 1,2,3,4,5]

- [x] 6.1 Add `sanitize.Scan()` call in `cli.go` `indexDocuments()` function. Compute `ScanConfig` from source's `sanitize_mode`. Skip scanning when mode is `off`. After `vault.ParseDocument()` returns frontmatter properties, merge `ScanResult.Findings` into the properties map as `sanitize_findings`. In `strict` mode, skip documents with `critical` or `high` findings. Add `// SYNC: identical sanitization logic in tools/indexing.go:indexDocuments()` comment. (FR-SAN-001, FR-SAN-008)
- [x] 6.2 Add identical `sanitize.Scan()` call in `tools/indexing.go` `indexDocuments()` function. Add `// SYNC: identical sanitization logic in cli.go:indexDocuments()` comment. (FR-SAN-001, FR-SAN-008)
- [x] 6.3 Compute per-source `SourceStats` from document content lengths before per-document scanning using `sanitize.ComputeStats()`. Pass stats via `ScanInput.SourceStats`. (FR-SAN-005)
- [x] 6.4 Wire content hash drift: pass the existing page's `ContentHash` (from `store.GetPage()`) as `ScanInput.PreviousHash` and the new document's `ContentHash` as `ScanInput.CurrentHash`. For first-time pages (no existing record), pass empty `PreviousHash`. (FR-SAN-003)
- [x] 6.5 Write integration tests for pipeline: `TestIndexDocuments_ScanCalledForWebSource`, `TestIndexDocuments_ScanSkippedForDiskSource`, `TestIndexDocuments_StrictModeSkipsDocument`, `TestIndexDocuments_FindingsSurvivePersistence`, `TestIndexDocuments_FindingsMergedWithFrontmatter`. Use in-memory SQLite and verify findings are queryable via `GetPagesWithProperty()`. (FR-SAN-001, FR-SAN-008)

## 7. Source Configuration Extension [depends: none, parallel: 2,3,4,5]

- [x] 7.1 Add `trust_tier` field validation to `validateSourceConfig()` in `source/config.go`: accept `authored`, `curated`, `validated`, `draft`, or `untrusted`. Reject other values with clear error message. (FR-SAN-006)
- [x] 7.2 Add `sanitize_mode` field validation to `validateSourceConfig()`: accept `warn`, `strict`, or `off`. Reject other values with clear error message. (FR-SAN-008)
- [x] 7.3 Propagate `trust_tier` from source config to page `tier` column during indexing in `cli.go` and `tools/indexing.go`. Default to `authored` when not specified. (FR-SAN-006)
- [x] 7.4 Write tests `TestValidateConfig_InvalidTrustTier`, `TestValidateConfig_InvalidSanitizeMode`, `TestValidateConfig_DefaultsApplied`: invalid trust_tier (error), invalid sanitize_mode (error), missing both fields (defaults applied). (FR-SAN-006, FR-SAN-008)

## 8. Trust Tier Extension [depends: 7]

- [x] 8.1 Update `types/tools.go` `SemanticSearchFilteredInput` tier schema description to include `untrusted`. (FR-SAN-006)
- [x] 8.2 Update `tools/semantic.go` tier ordering comment to `authored > curated > validated > draft > untrusted`. (FR-SAN-006)
- [x] 8.3 Add `store.GetPagesWithProperty(key string) ([]Page, error)` method using SQLite `json_extract()` to query pages by property key existence. (FR-SAN-009, FR-SAN-010)
- [x] 8.4 Write tests `TestGetPagesWithProperty_Exists`, `TestGetPagesWithProperty_NotExists`, `TestGetPagesWithProperty_EmptyIndex`: pages with property returned, pages without property excluded, empty index returns empty slice. (FR-SAN-009, FR-SAN-010)

## 9. Diagnostics and Linting [depends: 6,8]

- [x] 9.1 Add sanitization check category to `dewey doctor` following spec 005 emoji marker conventions — query store via `GetPagesWithProperty("sanitize_findings")`, aggregate finding counts by severity and source, display warning marker for any `critical` findings, display pass marker when no findings exist. (FR-SAN-009)
- [x] 9.2 Add sanitization lint rule to `dewey lint` — surface pages with active findings, report page name, finding count, highest severity, and pattern version per page. Flag stale findings (pattern version < `DefaultPatternVersion`) with re-scan recommendation. (FR-SAN-010)

## 10. Testing and Documentation [depends: all]

- [x] 10.1 Create `sanitize/sanitize_test.go` integration test for `Scan()` orchestrator: `TestScan_AllLayersExecute`, `TestScan_ConfigControlsLayers`, `TestScan_StrictModeRejectsHighSeverity`, `TestScan_WarnModeAllowsAllContent`, `TestScan_OffModeSkipsAll`. Verify all layers execute, findings aggregate correctly, scan config controls which layers run, strict mode produces rejection signal, warn mode produces findings without rejection. (FR-SAN-001, FR-SAN-007, FR-SAN-008)
- [x] 10.2 Create test fixtures in `sanitize/testdata/`: `injection-override.md` (critical injection), `injection-role.md` (high injection), `clean-api-docs.md` (no findings), `invisible-unicode.md` (structure findings), `data-uri-payload.md` (structure findings), `large-content.md` (anomaly trigger). (FR-SAN-002 through FR-SAN-005)
- [x] 10.3 Add `BenchmarkScan` benchmark test measuring per-document scanning latency. Target: < 1ms per typical documentation page (2000 chars). (Design coverage strategy)
- [x] 10.4 Update `AGENTS.md`: document `sanitize` package in Architecture section, add `untrusted` to Trust Tiers table, document `trust_tier` and `sanitize_mode` config fields, add `dewey doctor` sanitization check, add `dewey lint` sanitization rule
- [x] 10.5 Update `README.md`: add content sanitization section documenting the five defense layers, configuration options, and how to interpret findings
- [x] 10.6 Run `go build ./...` and `go test -race -count=1 ./...` to verify no regressions
- [x] 10.7 Run `go vet ./...` for static analysis
- [x] 10.8 Create GitHub issue in `unbound-force/website` documenting: new `trust_tier` and `sanitize_mode` source config fields, new `untrusted` trust tier, new `dewey doctor` sanitization check, new `dewey lint` sanitization rule, five defense layers
<!-- spec-review: passed -->
<!-- code-review: passed -->
