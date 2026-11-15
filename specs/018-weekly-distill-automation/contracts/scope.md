# Contract: Scope Computation

## Input

- `default_ref`: remote default branch (resolved via `gitref` precedence chain)
- `compiled_prefix`: `.uf/dewey/compiled/` (configurable via distill-policy if needed)
- `learnings_prefix`: `.uf/dewey/learnings/`
- `backlog_from` (optional, `YYYY-MM-DD`): bound the scope to learnings created on or before this UTC date — the `--backlog-from` backlog bootstrap (FR-020). Identities embed `{tag}-{YYYYMMDDTHHMMSS}-{author}` timestamps; a learning passes when its timestamp is strictly before the end of the cutoff day. Legacy identities without an embedded timestamp always pass. An unparseable date fails closed with an error — never a silently unbounded or silently empty scope.

## Behavior

```text
scope(ref):
  learnings = ls_tree(ref, learnings_prefix) -> { identities }
  compiled_blobs = ls_tree(ref, compiled_prefix) -> { paths }
  cited = union( parse_frontmatter(blob).sources for blob in compiled_blobs )
  raw = learnings - cited
  filtered = raw - policy.exclude_tags.matches(raw)
  return { all: learnings, cited, in_scope: filtered, provenance: git:<ref>:<sha> }
```

Fallback when not git / no ref: scan `.uf/dewey/learnings/*.md` + SQLite `pages` with `namespace=learning`; provenance `filesystem:degraded` + warning.

## Policy Exclusions (`.uf/dewey/distill-policy.yaml`)

Optional file; when present it is bounded before use (FR-007):

- **Base path**: repository root (vault root if different, resolved via `git rev-parse --show-toplevel` or `config/vault` path; CWD never used). Non-existent → no exclusions.
- **Bounds**: file ≤ 16 KiB, depth ≤ 3, arrays ≤ 100 entries, entries ≤ 200 chars. Over-limit → warn `distill-policy: over limit, ignored` and apply no exclusions; run proceeds.
- **Schema**:

| Key | Type | Valid values | On invalid |
|-----|------|--------------|------------|
| `exclude_tags` | `[]string` | glob per `path.Match` subset `*` `?` `[seq]` on tag namespace; no recursive `**` | warn + skip entry |
| `exclude_stores` | `[]string` | knowledge store names `[a-z0-9-]{1,50}` existing in `knowledge-stores.yaml` | warn + skip entry |
| unknown key | - | - | warn + ignore key |

- **Deny overly broad**: `exclude_tags: ["*"]` or `exclude_tags: ["*/*"]` that would starve scope to empty is rejected with warn `overly broad exclusion ignored`; require at least one non-wildcard char.
- **Precedence**: repo-local `distill-policy.yaml` only; no global policy exists; malformed entry → warn + skip that entry, do not fail run.
- Writes to this file are not produced by distiller; humans own it.

## Durable No-Op Record

Empty scope exits no-op with no PR and no comment, but MUST still record a durable run artifact (FR-021):

- Path `.uf/dewey/distill-runs/<YYYYMMDD-HHMMSS>.json` (also logged to pane) `{ schema, run_at, scope_size, themes, prs_touched[], no_op_reason }`. `schema` is the record's JSON schema version (`1` today) so future field changes remain distinguishable across the append-only history.
- If `in_scope == ∅`, artifact says `no_op_reason: "empty scope"` and is the sole audit surface (PR/issue bodies do not exist on that path).

## Invariants

- FR-005: deterministic from git alone.
- FR-006: never calls `question` tool. Asserted by log grep `question` == 0 in distiller run.
- Empty scope → exit no-op, no PR, no comment, but durable artifact written.
- Provenance logging per grouping decision (FR-010) persisted to durable artifact.

## Tests

- Fixture: 10 learnings, 2 cited → 8 in-scope exact equality.
- Policy excludes `scratch-*` → removed.
- Missing policy → default, no error.
- Empty → no-op with artifact.
- `backlog_from` on/after every fixture timestamp → full set; before every timestamp → empty no-op; invalid date → error result.
- Non-git → degraded, no panic.
- Over-broad `["*"]` → warn, not starved.
- 5 MB / 500-entry policy → bounded, warn.
