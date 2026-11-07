## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Compound Terms Are Searchable by Component

The in-memory search index MUST index each term containing `-` or
`_` both whole and as its components of at least 2 characters.
`search` MUST return exact-term matches first, unchanged. When the
query contains a compound term and exact matches number fewer than
the limit, `search` MUST append blocks matching every component of
the query, without duplicates, up to the limit.

Previously: compound terms were indexed only whole, so only the full
spelling matched.

#### Scenario: Full ID
- **GIVEN** a block containing `KB-REVIEW-ADJUD-01`
- **WHEN** searching `KB-REVIEW-ADJUD-01`
- **THEN** the block MUST be returned

#### Scenario: Truncated ID
- **GIVEN** a block containing `KB-REVIEW-ADJUD-01`
- **WHEN** searching `KB-REVIEW-ADJUD`
- **THEN** the block MUST be returned

#### Scenario: Space-split ID
- **GIVEN** a block containing `KB-REVIEW-ADJUD-01`
- **WHEN** searching `KB REVIEW ADJUD 01`
- **THEN** the block MUST be returned

#### Scenario: Exact matches first
- **GIVEN** one block containing `foo-bar` and another containing
  `foo` and `bar` separately
- **WHEN** searching `foo-bar`
- **THEN** the `foo-bar` block MUST come first and each block MUST
  appear once

## REMOVED Requirements

_None._
