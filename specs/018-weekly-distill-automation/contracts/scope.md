# Contract: Scope Computation

## Input

- `default_ref`: remote default branch (resolved via `gitref` precedence chain)
- `compiled_prefix`: `.uf/dewey/compiled/` (configurable via distill-policy if needed)
- `learnings_prefix`: `.uf/dewey/learnings/`

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

## Invariants

- FR-005: deterministic from git alone.
- FR-006: never calls `question` tool. Asserted by log grep `question` == 0 in distiller run.
- Empty scope → exit no-op, no PR, no comment.

## Tests

- Fixture: 10 learnings, 2 cited → 8 in-scope exact equality.
- Policy excludes `scratch-*` → removed.
- Missing policy → default, no error.
- Empty → no-op.
- Non-git → degraded, no panic.
