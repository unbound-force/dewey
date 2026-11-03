<!-- originating_issue: 148 -->

## Why

With #146, learnings carry `validity` and `as_of`, but retrieval
ignores them: a `superseded` learning ranks exactly like its
replacement, and a consumer cannot see the pin without a second read.
Consumers have compensated with prose — a proposal to add Dewey reads
to spec-planning and review commands pasted the same "weigh prior
knowledge by tier and validity before citing it" paragraph into 20+
command and agent files. That is retrieval policy, and it belongs in
the server.

Depends on #146. Part of #153 (umbrella).

## What Changes

- `semantic_search` and `semantic_search_filtered` results include
  `validity` and `as_of` when the page carries them.
- `semantic_search_filtered` accepts a `validity` filter.
- Both rank by validity-weighted similarity by default:
  `superseded` × 0.6, `stale-unverified` × 0.85, everything else
  × 1.0. The reported `similarity` stays the raw score. Both
  over-fetch so down-weighted results can be displaced by unweighted
  ones just below the limit.
- `ignore_validity: true` restores pure similarity order.

## Capabilities

### New Capabilities
- `validity-aware retrieval`: validity in results, validity filter,
  validity-weighted ranking.

### Modified Capabilities
- `semantic_search`, `semantic_search_filtered`: result fields,
  input fields, default ordering.

### Removed Capabilities
- None.

## Impact

- `types/tools.go` — result and input fields.
- `tools/semantic.go` — enrichment, filter, ranking, fetch limit.
- `tools/semantic_validity.go` (new) — pure helpers.
- Tests: `tools/semantic_validity_test.go`.
- README semantic search rows.

**Not covered:** keyword `search` and `similar` results (block hits
without page metadata today), and validity written inside a page
body rather than its properties (for example per-claim validity in a
multi-claim markdown file).

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No new dependencies; pages without validity rank exactly as before.

### II. Autonomous Collaboration

**Assessment**: PASS

Results are more self-describing; consumers no longer need prose
policy to weigh them.

### III. Observable Quality

**Assessment**: PASS

Raw similarity is still reported; ranking is documented and can be
switched off.

### IV. Testability

**Assessment**: PASS

Ranking and filtering are pure functions over result slices.
