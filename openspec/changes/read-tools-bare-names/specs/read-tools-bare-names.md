## ADDED Requirements

### Requirement: Bare-Name Store Resolution

When a page name without `/` misses the in-memory index and the exact
store lookup, the store fallback MUST try `compiled/<name>` and then
`learning/<name>`, case-insensitively, and MUST serve the first page
found. Every name-based read through the vault client (including
`get_links` forward direction, `traverse` start pages, `get_whiteboard`,
and `bulk_update_properties`) MUST share this resolution. Vault pages
MUST take precedence. With no store configured, behavior MUST NOT
change. The write guard MUST NOT use this resolution.

#### Scenario: Bare compiled topic
- **GIVEN** `compiled/gateway` is in the store and no vault page has
  that name
- **WHEN** `get_page("gateway")` is called
- **THEN** the compiled article MUST be returned under its canonical
  name

#### Scenario: Bare learning identity
- **GIVEN** `learning/auth-1` is in the store
- **WHEN** the block tree for `auth-1` is read via `get_page`
- **THEN** its blocks MUST be returned

#### Scenario: Compiled wins over learning
- **GIVEN** `compiled/x` and `learning/x` are both in the store
- **WHEN** `get_page("x")` is called
- **THEN** the compiled article MUST be returned

#### Scenario: Vault file precedence
- **GIVEN** a vault page `gateway` and a store page `compiled/gateway`
- **WHEN** `get_page("gateway")` is called
- **THEN** the vault page MUST be returned

#### Scenario: Slash names skip the shorthand
- **GIVEN** `compiled/a/b` is in the store
- **WHEN** `get_page("a/b")` is called
- **THEN** no page MUST be returned

#### Scenario: Write guard is exact
- **GIVEN** `compiled/gateway` is in the store
- **WHEN** a page named `gateway` is created
- **THEN** the write guard MUST NOT reject it

## MODIFIED Requirements

_None._

## REMOVED Requirements

_None._
