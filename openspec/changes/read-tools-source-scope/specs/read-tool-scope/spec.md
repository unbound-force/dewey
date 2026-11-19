# Delta Spec: read-tool-scope

<!--
  Requirement numbering allocation
  ================================
  Capability: read-tool-scope (prefix RS)
  Allocated to read-tools-source-scope: RS-FR-001 .. RS-FR-004
  Reserved for future changes: RS-FR-005 .. RS-FR-099
-->

## ADDED Requirements

### Requirement: Semantic tools accept source and tag scope

**ID**: RS-FR-001

`semantic_search` and `similar` MUST accept optional `source_id` and
`has_tag` arguments. When either is set, results MUST be restricted
with the same filter `semantic_search_filtered` applies for those
arguments. When neither is set, results MUST be unchanged.

#### Scenario: Scoped semantic search matches the filtered tool

- **GIVEN** an index with pages from sources `disk-alpha` and
  `disk-beta`
- **WHEN** `semantic_search` is called with `source_id: disk-alpha`
- **THEN** every result's `source_id` is `disk-alpha`
- **AND** the result pages equal those of `semantic_search_filtered`
  called with the same query and `source_id`

#### Scenario: Similar scopes results, not the query

- **GIVEN** a block in `disk-alpha`
- **WHEN** `similar` is called with that block's UUID and
  `source_id: disk-beta`
- **THEN** only `disk-beta` pages are returned

### Requirement: Full-text search accepts source and tag scope

**ID**: RS-FR-002

`search` MUST accept optional `source_id` and `has_tag`. A hit MUST be
returned only when its page is in the persistent store, its page's
`source_id` equals `source_id` (when set), and — when `has_tag` is set
— the page's properties contain the tag or the hit's block content
contains `#<tag>`, both compared case-insensitively. Out-of-scope hits
MUST NOT count toward `limit`.

#### Scenario: Out-of-scope hits do not consume the limit

- **GIVEN** more than `limit` out-of-scope hits ranked above one
  in-scope hit
- **WHEN** `search` is called with `source_id` and `limit`
- **THEN** the in-scope hit is returned

### Requirement: Tag search accepts source scope

**ID**: RS-FR-003

`find_by_tag` MUST accept an optional `source_id` and return only
blocks on pages whose store `source_id` equals it.

#### Scenario: find_by_tag restricted to one source

- **GIVEN** pages tagged `research` in `disk-alpha` and `disk-beta`
- **WHEN** `find_by_tag` is called with `tag: research` and
  `source_id: disk-beta`
- **THEN** only `disk-beta` blocks are returned

### Requirement: A scope that cannot be honored is an error

**ID**: RS-FR-004

When `search` or `find_by_tag` is called with a scope argument and no
persistent store is configured, Dewey MUST return an error result and
MUST NOT return unscoped results.

#### Scenario: No persistent store

- **GIVEN** a server without a persistent store
- **WHEN** `search` is called with `source_id`
- **THEN** the result is an error naming the missing store
