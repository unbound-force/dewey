## 1. Author Resolution

- [x] 1.1 Add `resolveAuthor(gitResolver func() (string, error))` function in `tools/learning.go` that implements the three-tier fallback chain: `DEWEY_AUTHOR` env var -> `gitResolver()` call -> `"anonymous"`. Empty or whitespace-only `DEWEY_AUTHOR` values MUST be treated as unset (fall through to git). The function MUST normalize the result using the existing `normalizeTag` function (no rename). If normalization produces an empty string (e.g., CJK-only names), fall back to `"anonymous"`. Truncate the normalized result to 64 characters. The production git resolver MUST use `exec.CommandContext` with a 2-second timeout and trim whitespace/newlines from the output.

- [x] 1.2 Add unit tests for `resolveAuthor()` in `tools/learning_test.go` as a table-driven test with at least 10 cases: (a) returns env var value when `DEWEY_AUTHOR` is set, (b) normalizes author names (spaces, special chars), (c) returns `"anonymous"` when env var is unset and git resolver returns error, (d) returns `"anonymous"` when env var is unset and git resolver returns empty string, (e) empty `DEWEY_AUTHOR` falls through to git, (f) whitespace-only `DEWEY_AUTHOR` falls through to git, (g) CJK/all-special-char names normalize to empty and fall back to `"anonymous"`, (h) leading/trailing hyphens are stripped, (i) very long author strings are truncated to 64 chars, (j) git resolver returning value with newline is trimmed. Use `t.Setenv` for environment variable tests. Inject mock git resolver functions — MUST NOT depend on test runner's actual git configuration.

## 2. Identity Generation

- [x] 2.1 Replace the `NextLearningSequence` call in `StoreLearning` (`tools/learning.go`) with timestamp-based identity generation. The identity format MUST be `{tag}-{YYYYMMDDTHHMMSS}-{author}` using UTC time. Update `pageName` and `docID` derivation accordingly. Call `resolveAuthor()` with a production git resolver that uses `exec.CommandContext`.

- [x] 2.2 Add sub-second collision avoidance: use `os.OpenFile` with `O_CREATE|O_EXCL` flags for atomic file creation. If the file already exists (`os.ErrExist`), append `-2`, then `-3`, etc. to the identity, capped at 99 attempts. If 99 attempts are exhausted, return an MCP error result. Update the identity, pageName, and docID to match the final filename. The collision suffix attaches after the author segment: `{tag}-{timestamp}-{author}-{N}`.

- [x] 2.3 Update the `writeLearningFile` function signature — remove `seq int` parameter, add `author string` parameter for frontmatter. Derive the filename from the existing `identity` parameter: `{identity}.md`. Add `author:` field to the YAML frontmatter. Use `O_CREATE|O_EXCL` for file creation.

- [x] 2.4 Add `"author"` to the properties JSON map and to the MCP response result map in `StoreLearning`.

## 3. Remove NextLearningSequence

- [x] 3.1 Delete the `NextLearningSequence` method from `store/store.go`.

- [x] 3.2 Delete the `NextLearningSequence` tests from `store/migrate_test.go`: `TestNextLearningSequence_Empty`, `TestNextLearningSequence_WithExisting`, `TestNextLearningSequence_DifferentTags`, `TestNextLearningSequence_IgnoresNonLearningPages`.

## 4. Re-ingestion Backward Compatibility

- [x] 4.1 Add `Author` field to `learningFrontmatter` struct in `main.go`: `Author string \`yaml:"author"\``.

- [x] 4.2 Update `reIngestLearnings` in `main.go` to include `author` in the `propsMap` when `fm.Author` is non-empty. No other changes needed — the function already uses `fm.Identity` from frontmatter for page naming, so both old-format and new-format identities are handled automatically.

- [x] 4.3 Update `TestParseLearningFrontmatter` in `main_test.go` to include `author:` in the test frontmatter and assert `fm.Author` is parsed correctly.

- [x] 4.4 Add a new test `TestReIngestLearnings_OldFormatCompatibility` that creates an old-format learning file (`tag-1.md` with no `author` field) and verifies it is re-ingested successfully with empty author in properties.

- [x] 4.5 Add a new test `TestReIngestLearnings_MixedFormats` that creates a learnings directory with 2 old-format files and 2 new-format files, verifies all 4 are re-ingested correctly with appropriate metadata (old files have empty author, new files have author populated).

## 5. Update Tag Extraction (Compilation)

- [x] 5.1 Update `extractTagFromIdentity` in `tools/compile.go` to accept a `properties` string parameter (the page's properties JSON). If properties contain a `"tag"` key, return that value directly. Fall back to the existing string-parsing logic for backward compatibility when properties are empty or missing the tag.

- [x] 5.2 Update all call sites of `extractTagFromIdentity` in `tools/compile.go` (`buildLearningEntries`, `compileIncremental`) to pass the page's `Properties` field.

- [x] 5.3 Update `storeLearningDirect` helper in `tools/compile_test.go` to use new-format identities and include `"tag"` in properties JSON.

- [x] 5.4 Update `TestExtractTagFromIdentity` in `tools/compile_test.go` to test both old-format and new-format identities, and the properties-based extraction path.

- [x] 5.5 Update `storeLearningForLint` helper in `tools/lint_test.go` to use new-format identities and include `"tag"` in properties JSON.

## 6. Update Existing Tests

- [x] 6.1 Update `TestStoreLearning_Basic` — assert identity matches `{tag}-{YYYYMMDDTHHMMSS}-{author}` pattern (use regex) instead of exact `"general-1"` match. Assert `"author"` key exists in response.

- [x] 6.2 Update `TestStoreLearning_WithTag` — assert identity matches `authentication-{timestamp}-{author}` pattern instead of exact `"authentication-1"`.

- [x] 6.3 Update `TestStoreLearning_EmptyTag` — assert identity matches `general-{timestamp}-{author}` pattern instead of exact `"general-1"`.

- [x] 6.4 Update `TestStoreLearning_BackwardCompat` — assert identity matches `gotcha-{timestamp}-{author}` pattern instead of exact `"gotcha-1"`.

- [x] 6.5 Update `TestStoreLearning_SequenceIncrement` — rewrite to verify that three learnings with the same tag produce three distinct identities with the same tag prefix. Since timestamps may differ by at least 1 second or use collision suffixes, assert all three identities are unique and start with `deployment-`.

- [x] 6.6 Update `TestStoreLearning_DifferentTagSequences` — rewrite to verify that learnings with different tags produce identities with the correct tag prefix (`auth-` vs `deploy-`). Assert all identities contain the same author.

- [x] 6.7 Update `TestStoreLearning_DualWritesMarkdown` — assert filename matches new format pattern instead of exact `"authentication-1.md"`.

- [x] 6.8 Update `TestStoreLearning_MarkdownFormat` — assert frontmatter contains `author:` field. Update identity assertion to match new format.

- [x] 6.9 Update `TestStoreLearning_MarkdownFormatNoCategory` — update identity assertion to match new format.

- [x] 6.10 Update `TestStoreLearning_FileWriteFailure` — update `s.GetPage` call to use regex-matched page name instead of exact `"learning/resilience-1"`.

- [x] 6.11 Update `TestStoreLearning_NoVaultPath` — update `s.GetPage` call to use regex-matched page name instead of exact `"learning/test-1"`.

- [x] 6.12 Update `TestReIngestLearnings_RecoversMissing` — create test files with new-format names and frontmatter including `author:`. Verify author is preserved in properties.

- [x] 6.13 Update `TestReIngestLearnings_PreservesCreatedAt` — update test file to use new-format name and frontmatter.

## 7. Documentation and Verification

- [x] 7.1 Update GoDoc comments on `StoreLearning`, `writeLearningFile`, `Learning` struct, and the new `resolveAuthor` function to reflect the new identity format and author resolution.

- [x] 7.2 Update `types/tools.go` — modify the `CompileInput.Incremental` field's jsonschema description to use the new identity format example (e.g., `['authentication-20260502T143022-alice']` instead of `['authentication-3']`).

- [x] 7.3 Update `AGENTS.md` — modify three sections: (a) "Store Learning API" — identity format change to `{tag}-{timestamp}-{author}`, author resolution chain, `DEWEY_AUTHOR` env var documentation; (b) "File-Backed Learnings" — filename format change from `{tag}-{seq}.md` to `{tag}-{timestamp}-{author}.md`; (c) "Recent Changes" — add entry for this change. Add `DEWEY_AUTHOR` to a new "Environment Variables" subsection or to the existing documentation where environment variables are listed.

- [x] 7.4 Assess `README.md` for staleness — verify the description of file-backed learnings does not reference the old `{tag}-{seq}` format. Update if needed.

- [x] 7.5 File a GitHub issue in `unbound-force/website` documenting the identity format change from `{tag}-{seq}` to `{tag}-{timestamp}-{author}`, the new `author` field in the `store_learning` MCP response, and the new `DEWEY_AUTHOR` environment variable. Affected pages: `content/docs/getting-started/knowledge.md` (Storing and Retrieving Learnings section). Use format: `docs: sync dewey store_learning identity format change`.

- [x] 7.6 Run `go build ./...`, `go vet ./...`, and `go test -race -count=1 ./...` to verify all changes compile and pass. Run `go test -race -count=1 -coverprofile=coverage.out ./...` and verify no coverage regression. All new functions (`resolveAuthor`, collision avoidance) MUST have test coverage.

- [x] 7.7 Verify constitution alignment: Composability (Dewey works without git — anonymous fallback), Observable Quality (author provenance in filename and frontmatter), Testability (author resolution testable without external services via injectable git resolver and env var injection).

<!-- spec-review: passed -->
<!-- code-review: passed -->
