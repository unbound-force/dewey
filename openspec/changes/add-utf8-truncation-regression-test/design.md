## Context

`buildCompiledArticle` writes each learning's content into the compiled article history table and limits the summary to 80 Unicode code points. PR #75 changed the implementation from byte slicing to `[]rune` slicing because cutting a multibyte character at byte offset 80 produced invalid UTF-8. `TestCompile_CompiledArticleFormat` verifies the surrounding Markdown structure but does not exercise truncation or encoding boundaries.

The proposal's constitution assessment identifies this as an isolated Testability and Observable Quality improvement. The test will exercise the observable article output without external services or runtime coupling.

## Goals / Non-Goals

### Goals

- Reproduce the former byte-slicing failure with deterministic multibyte boundary inputs.
- Verify valid UTF-8, the exact retained prefix, and ellipsis behavior.
- Cover em dash, CJK, emoji, and exactly 80-rune cases in one table-driven test.
- Keep the test package-local and independent of external services.

### Non-Goals

- Change `buildCompiledArticle` or extract new production helpers.
- Change the 80-code-point summary limit or Markdown history table format.
- Add end-to-end compilation, persistence, MCP, or CLI coverage.
- Modify user-facing documentation or dependencies.

## Decisions

### Test `buildCompiledArticle` directly

Add the regression case beside `TestCompile_CompiledArticleFormat` in `tools/compile_test.go`. A package-local test can construct a minimal `Cluster`, call `buildCompiledArticle`, and inspect the generated history row without involving storage, an LLM provider, the filesystem, or an MCP session. This preserves Composability First and isolates the behavior under test.

### Use boundary inputs that fail under byte slicing

Each multibyte case will place 79 ASCII characters before one multibyte rune and include trailing content so truncation is required. Replacing the production code with `summary[:80]` would then cut the multibyte encoding and cause `utf8.ValidString` and the exact expected-summary assertion to fail. An exactly 80-rune case will verify that the boundary does not add `...`.

### Assert observable output rather than implementation details

The table will define input content and an explicit expected history summary. Each subtest will assert that the complete article is valid UTF-8 and contains the expected formatted history row. The test will not assert that production code uses `[]rune`, allowing any future implementation that preserves the specified output contract.

## Risks / Trade-offs

- Directly matching a history row couples the regression test to the existing Markdown table contract, but that contract is the observable output being protected.
- Unicode code points are not grapheme clusters; the existing behavior can split a combined grapheme. This change intentionally preserves the current 80-rune contract rather than expanding scope to grapheme-aware truncation.
- A whole-article UTF-8 assertion does not identify the exact corrupt field by itself, so the explicit expected history summary provides the field-level diagnostic.

<!-- scaffolded by uf v0.17.0 -->
