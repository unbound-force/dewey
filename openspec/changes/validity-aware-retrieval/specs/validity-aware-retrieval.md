## ADDED Requirements

### Requirement: Validity in Semantic Results

`semantic_search` and `semantic_search_filtered` results MUST include
`validity` and `as_of` when the result's page properties carry them.

#### Scenario: Learning with validity
- **GIVEN** a learning with `validity: current` and `as_of: r@abc`
- **WHEN** a semantic search returns one of its blocks
- **THEN** the result MUST include both values

### Requirement: Validity Filter

`semantic_search_filtered` MUST accept `validity`; when set, only
results whose page validity equals it MUST be returned.

#### Scenario: Filter to current
- **GIVEN** matching pages with validity `current` and `superseded`
- **WHEN** searching with `validity: current`
- **THEN** only the `current` page's results MUST be returned

### Requirement: Validity-Weighted Ranking

By default both tools MUST order results by similarity multiplied by
a validity weight (`superseded` 0.6, `stale-unverified` 0.85, others
1.0), MUST report the raw similarity, and MUST over-fetch before
trimming to the limit. With `ignore_validity: true`, results MUST be
in raw similarity order.

#### Scenario: Superseded ranks below its replacement
- **GIVEN** a superseded learning with similarity 0.9 and its
  replacement with similarity 0.8
- **WHEN** searching
- **THEN** the replacement MUST come first and both MUST report their
  raw similarity

#### Scenario: Raw order on request
- **GIVEN** the same two learnings
- **WHEN** searching with `ignore_validity: true`
- **THEN** the superseded learning MUST come first
