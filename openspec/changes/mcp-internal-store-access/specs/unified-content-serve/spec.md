# Delta Spec: unified-content-serve

## ADDED Requirements

### Requirement: Live Retrieval of Internal Store Pages

MCP read tools MUST retrieve pages that live in the internal SQLite
store without requiring a server restart. When a page name is absent
from the vault file index and a persistent store is configured, the
vault backend MUST fall back to a live store lookup and return the
page's metadata (name, original name, properties, timestamps), tier,
and full reconstructed block tree, in the same result shape used for
vault file pages.

#### Scenario: Compiled article readable in the same session

- **GIVEN** a running `dewey serve` with a persistent store whose
  background startup indexing has completed, and no page named
  `compiled/gateway` in the vault file index
- **WHEN** `store_compiled(tag="gateway", content="# Gateway\n\nBody
  text.", sources=["s1"])` is called, followed by
  `get_page(name="compiled/gateway")`
- **THEN** `get_page` returns the compiled page with its block tree,
  `tier: draft`, and the stored properties (`topic`, `compiled_at`,
  `sources`), without a server restart

#### Scenario: Learning readable in the same session

- **GIVEN** a running `dewey serve` with a persistent store
- **WHEN** `store_learning(tag="auth", information="Use parameterized
  SQL.")` returns identity `auth-YYYYMMDDTHHMMSS-author`, followed by
  `get_page(name="learning/auth-YYYYMMDDTHHMMSS-author")`
- **THEN** `get_page` returns the learning page with its parsed block
  tree and frontmatter-derived properties (`tag`, `author`,
  `created_at`, `category`)

### Requirement: Freshness of Store Reads

Store fallback reads MUST query the store at read time rather than
serving a mid-session cached copy, so that pages stored or re-stored
during the current server session are returned with their latest
committed content.

#### Scenario: Re-stored page is never stale

- **GIVEN** a running `dewey serve` where `compiled/gateway` was
  stored earlier in the same session and has since been read at least
  once via `get_page`
- **WHEN** `store_compiled(tag="gateway", content="# Gateway
  v2\n\nUpdated body.")` is called, followed by
  `get_page(name="compiled/gateway")`
- **THEN** the returned block content reflects the v2 content, not
  the previously read version

### Requirement: Vault Precedence on Name Collision

On a name collision between a vault file page and a store page, the
vault file page MUST take precedence in every read path.

#### Scenario: Vault file page precedence on name collision

- **GIVEN** a vault containing a file whose page name is `notes/topic`
  and a persistent store that also contains a page named
  `notes/topic` from a non-vault source
- **WHEN** `get_page(name="notes/topic")` is called
- **THEN** the vault file page is returned

### Requirement: Block Retrieval for Store Pages

`get_block` MUST resolve block UUIDs that belong to internal store
pages by falling back to a live store lookup when the UUID is absent
from the vault file index. The returned block MUST carry its content,
UUID, and page reference. Because `store_learning` returns the first
block's UUID, an agent MUST be able to pass that UUID directly to
`get_block` in the same session.

#### Scenario: Learning block by UUID

- **GIVEN** a running `dewey serve` with a persistent store
- **WHEN** `store_learning(...)` returns uuid `U`, followed by
  `get_block(uuid=U)`
- **THEN** `get_block` returns the block with content matching the
  stored learning and a page reference to the learning page

### Requirement: Unified Page Enumeration

Page enumeration MUST include pages that exist only in the persistent
store, merged with vault file pages. The merged result MUST contain
each page name at most once, store-backed entries MUST retain their
stored metadata including tier and properties, and enumeration
filters (namespace, property, tag) MUST apply to the merged set.

#### Scenario: list_pages includes stored learnings and compiled articles

- **GIVEN** a running `dewey serve` with a persistent store containing
  `learning/auth-...` and `compiled/gateway` pages, and a vault file
  index that does not contain those names
- **WHEN** `list_pages()` is called
- **THEN** the result contains both store-backed pages alongside the
  vault file pages, with no duplicate names

#### Scenario: Namespace filter over the merged set

- **GIVEN** the merged page set contains store-backed pages under the
  `compiled/` namespace
- **WHEN** `list_pages(namespace="compiled")` is called
- **THEN** the store-backed compiled pages are returned

#### Scenario: No duplication of vault pages persisted to the store

- **GIVEN** a vault file `readme.md` that background indexing has
  persisted into the store under the same page name
- **WHEN** `list_pages()` is called
- **THEN** `readme` appears exactly once

### Requirement: Unified Full-Text Search

`search` MUST return matching blocks that live in the persistent
store for pages not present in the vault file index, merged with
matches from the vault's in-memory search index. Store-side matching
MUST use parameterized queries only.

#### Scenario: Search finds a same-session compiled article

- **GIVEN** a running `dewey serve` with a persistent store where
  `store_compiled(tag="gateway", content="...unique-phrase...")` was
  called earlier in the session
- **WHEN** `search(query="unique-phrase")` is called
- **THEN** the matching block from `compiled/gateway` is returned with
  its page name and UUID

### Requirement: Unified Tag Search

`find_by_tag` MUST return tag matches from pages that exist only in
the persistent store — matching frontmatter property tags
(`tag`/`tags`) and block-content `#tags` — not only `learning/`
pages as previously implemented. Store-backed results MUST be merged
without duplicating pages already matched from the vault file index.

#### Scenario: find_by_tag matches a compiled article by frontmatter tag

Resolved decision: the `topic` property IS tag-equivalent for
`find_by_tag`, source-scoped to pages from the compiled source — a
stray `topic` property on a learning or external page does not match.
Store pages otherwise match on frontmatter `tag`/`tags` and
block-content `#tags` only.

- **GIVEN** a persistent store containing `compiled/gateway` whose
  stored properties include `topic: gateway`
- **WHEN** `find_by_tag(tag="gateway")` is called
- **THEN** the compiled page's blocks are returned among the results

#### Scenario: find_by_tag matches block-content tags in store pages

- **GIVEN** a persistent store containing an external-source page
  whose block content includes `#deploy`
- **WHEN** `find_by_tag(tag="deploy")` is called
- **THEN** that block is returned among the results

### Requirement: Reload Preserves Store-Backed Pages

The `reload` MCP tool MUST NOT drop pages loaded from the persistent
store. After re-reading vault files, reload MUST re-load store-backed
pages so that the post-reload read view equals the pre-reload view
for pages that were not changed on disk.

#### Scenario: reload keeps same-session learnings visible

- **GIVEN** a running `dewey serve` where `store_learning` created
  `learning/auth-...` earlier in the session and the page is visible
  via `get_page`
- **WHEN** the `reload` tool is called
- **THEN** `get_page(name="learning/auth-...")` still returns the
  page without a server restart

### Requirement: Promote Locates Internal Store Pages

The `promote` MCP tool MUST locate and promote draft pages in the
internal store — including `learning/*` and `compiled/*` pages —
using the persistent store directly rather than the vault file index.
Existing promote invariants are unchanged: only `draft` pages are
promotable, the transition target is `validated`, and `updated_at`
is refreshed.

#### Scenario: Promote a compiled article

- **GIVEN** a running `dewey serve` with a persistent store where
  `store_compiled(tag="gateway", ...)` created `compiled/gateway`
  with tier `draft`
- **WHEN** `promote(page="compiled/gateway")` is called
- **THEN** the page's tier becomes `validated`, and the result
  reports the previous and new tier

#### Scenario: Promote a learning

- **GIVEN** a persistent store where `store_learning` created
  `learning/auth-...` with tier `draft`
- **WHEN** `promote(page="learning/auth-...")` is called
- **THEN** the learning's tier becomes `validated`

#### Scenario: Non-draft page is rejected

- **GIVEN** a persistent store containing `compiled/gateway` with tier
  `validated`
- **WHEN** `promote(page="compiled/gateway")` is called
- **THEN** promote fails with the existing tier-invariant error
  message

### Requirement: Promote Name Resolution

When the input name does not match a store page exactly, `promote`
MUST resolve it by trying the `compiled/{name}` and `learning/{name}`
prefixes before reporting failure.

Resolved decision: shorthand resolution is INCLUDED — when the exact
name misses and the input contains no `/`, promote tries
`compiled/{name}` then `learning/{name}`, first existing wins.

#### Scenario: Shorthand resolution

- **GIVEN** a persistent store containing `compiled/gateway` with tier
  `draft`
- **WHEN** `promote(page="gateway")` is called
- **THEN** `compiled/gateway` is resolved and promoted

### Requirement: Promote Error Distinction

`promote` MUST distinguish a page that does not exist from an
underlying store failure: absence MUST report `Page '<name>' not
found`, while a store error MUST surface the underlying error text so
the failure is diagnosable.

#### Scenario: Store failure is not masked as not-found

- **GIVEN** a promote invocation during which the store lookup returns
  a query error
- **WHEN** `promote(page="compiled/gateway")` is called
- **THEN** the error result includes the underlying store error text,
  distinguishable from the not-found message

### Requirement: CLI Promote Lock Diagnostics

When `dewey promote` cannot open the persistent store because another
process holds the database lock, the CLI error MUST identify the lock
holder using the PID and command recorded in `.uf/dewey/dewey.lock`
and MUST point the user to the MCP `promote` tool as the supported
write path while a Dewey server is running. The locking protocol
itself (exclusive, non-blocking flock) MUST NOT change.

#### Scenario: Lock held by a running server

- **GIVEN** a running `dewey serve` holding the store lock for a
  vault, and the lock file recording the server's PID and command
- **WHEN** `dewey promote compiled/gateway --vault <vault>` is run
- **THEN** the command fails (the lock is still honored) and the error
  names the holding process's PID/command and suggests the MCP
  `promote` tool or stopping the server

#### Scenario: Lock file unreadable or stale

- **GIVEN** a locked store whose lock file cannot be read or contains
  no PID data
- **WHEN** `dewey promote` fails to open the store
- **THEN** the error still reports the lock contention cause, degrades
  gracefully without the holder details, and exits non-zero

### Requirement: Degraded-Mode Safety

When a Dewey server runs without a persistent store (for example, a
second instance that could not acquire the database lock), write
tools that require persistence MUST return clear MCP error results —
they MUST NOT panic — and read tools MUST operate against the
in-memory file index with behavior identical to the pre-change
implementation. Specifically, `store_compiled` MUST return the shared
"requires persistent storage" error when its store is nil.

#### Scenario: store_compiled without a store

- **GIVEN** a `dewey serve` running without a persistent store
- **WHEN** `store_compiled(tag="x", content="...")` is called
- **THEN** the tool returns an MCP error result stating that
  persistent storage is required (hinting that another Dewey process
  may hold the database lock), and the server process does not crash

#### Scenario: Inherited tools unchanged without a store

- **GIVEN** a `dewey serve` running without a persistent store
- **WHEN** any of the inherited graphthulhu read tools is called
  (`get_page`, `list_pages`, `search`, `get_block`, `find_by_tag`,
  `graph_overview`)
- **THEN** results are identical to the pre-change implementation
