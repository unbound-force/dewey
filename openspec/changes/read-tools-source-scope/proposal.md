# Proposal: Scope Every Read Tool by Source

## Why

Only `semantic_search_filtered` accepts `source_id` and `has_tag`. The
tools agents actually reach for — `semantic_search`, `search`,
`similar` and `find_by_tag` — search the whole index, so a caller that
wants one source's pages has to remember to switch tools. In practice
it doesn't: across one machine's opencode logs (all projects),
`semantic_search` appears 1,414 times and `semantic_search_filtered` 89
times.

This matters most where one Dewey index holds several unrelated
corpora as separate sources — for example a research repository with
one `disk-<topic>` source per topic. A plugin that wants to keep a
session inside one topic (`opencode-research-harness`) can fill in a
missing argument on a tool call, but it cannot add an argument a tool
doesn't have. Today it has to reject the unfiltered tools outright and
steer the agent to `semantic_search_filtered`. With an optional
`source_id` on every read tool it can scope the call instead.

## What Changes

1. **`semantic_search` and `similar`** (`tools/semantic.go`) gain
   optional `source_id` and `has_tag`. When either is set the search
   runs through `store.SearchSimilarFiltered` — the same SQL filter
   `semantic_search_filtered` uses — so a scoped call returns exactly
   the filtered tool's results. With neither set, behavior is
   unchanged. For `similar`, the scope applies to the results; the
   query page or block may be in any source.
2. **`search`** (`tools/search.go`) gains optional `source_id` and
   `has_tag`; **`find_by_tag`** gains optional `source_id`. These tools
   read through the backend, whose hits carry only page names, so the
   handler resolves each hit's page in the persistent store
   (`GetPageFoldCase`, one lookup per page) and keeps hits whose page
   matches: `source_id` exactly, `has_tag` when the page properties
   mention the tag or the block content contains `#tag`
   (case-insensitive) — the semantics of `SearchSimilarFiltered`. A
   scoped indexed search over-fetches (`max(10×limit, 200)`) before
   filtering so in-scope hits ranked below out-of-scope ones still fill
   the limit; the brute-force path skips out-of-scope matches before
   they count.
3. **Wiring** (`server.go`): `tools.NewSearch(b).WithStore(cfg.store)`.
   Without a persistent store, a scoped `search` / `find_by_tag` call
   returns an error result rather than silently ignoring the scope.
4. **Docs**: tool descriptions, README tool tables, AGENTS.md tool
   matrix.

`find_by_tag` gets no `has_tag`: it already selects by tag.

## Capabilities

### New Capabilities
- `read-tool-scope`: optional `source_id` (and `has_tag`) on
  `semantic_search`, `search`, `similar` and `find_by_tag` with
  `semantic_search_filtered`'s semantics.

### Modified Capabilities
- None. Unscoped calls are unchanged.

### Removed Capabilities
- None.

## Impact

- **Files**: `types/tools.go`, `tools/semantic.go`, `tools/search.go`,
  new `tools/search_scope.go`, `server.go`, new
  `tools/search_scope_test.go`, `README.md`, `AGENTS.md`.
- **API**: additive, optional JSON fields on four tool inputs. No CLI,
  config, schema or file-format change.
- **Inherited tools**: `search` and `find_by_tag` are among the 37
  inherited graphthulhu tools. Without the new fields their behavior
  and output are byte-identical (Neighborhood Rule).
- **Website**: user-facing MCP tool parameters change, so a
  `unbound-force/website` docs sync issue is required.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

Callers (agents, plugins) scope reads through declared tool arguments
— an artifact-level contract — with no runtime coupling to Dewey's
internals.

### II. Composability First

**Assessment**: PASS

Purely additive and optional. No dependency or configuration change;
servers without a persistent store keep every unscoped behavior and
report a clear error only when a scope is requested.

### III. Observable Quality

**Assessment**: PASS

Scoped results carry the same provenance fields as before (`source_id`
on semantic results). A scope that cannot be honored is an error, never
a silent unscoped answer.

### IV. Testability

**Assessment**: PASS

Every path is covered with an in-memory store and the existing mock
backends (indexed, brute-force, tag-searcher, DataScript), including a
parity test against `semantic_search_filtered`.
