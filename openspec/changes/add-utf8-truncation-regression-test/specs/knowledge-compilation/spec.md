## ADDED Requirements

### Requirement: Rune-safe compiled history summaries

When a learning's content exceeds 80 Unicode code points, the compiled article history summary MUST contain the first 80 code points followed by `...`. The compiled article MUST remain valid UTF-8 when the retained prefix ends with a multibyte character. Content containing no more than 80 code points MUST remain untruncated and MUST NOT receive an ellipsis.

#### Scenario: Multibyte character crosses the former byte boundary

- **GIVEN** learning content with 79 ASCII characters followed by an em dash, CJK character, or emoji and additional content
- **WHEN** Dewey builds the compiled article history table
- **THEN** the summary MUST contain the first 80 Unicode code points followed by `...`
- **AND** the retained multibyte character MUST remain intact
- **AND** the complete compiled article MUST be valid UTF-8

#### Scenario: Content is exactly 80 Unicode code points

- **GIVEN** learning content containing exactly 80 Unicode code points
- **WHEN** Dewey builds the compiled article history table
- **THEN** the summary MUST equal the original content
- **AND** the summary MUST NOT end with an added ellipsis
- **AND** the complete compiled article MUST be valid UTF-8

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

<!-- scaffolded by uf v0.17.0 -->
