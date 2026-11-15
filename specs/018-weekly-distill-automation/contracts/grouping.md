# Contract: Theme Grouping

## Input

- `ScopeSet.in_scope`
- Existing compiled articles (headers + embeddings)
- Explicit supersession links (`supersedes` frontmatter)
- `Embedder` (optional; degraded when unavailable)

## Behavior

1. Build must-colocate components from supersession edges (FR-009).
2. For each component, compute closest compiled article via embedding cosine (or tag centroid).
3. If distance < threshold or tag match, assign to that article's theme; else new-theme bucket.
4. Tie within margin → deterministic centroid tie-breaker, log ambiguity (FR-010).
5. LLM judge (Jev) behind flag: only if configured; server-side, every decision logged.

## Output

- `themes: Theme[]` (one per rolling PR)
- Each `AssignmentDecision` with `{source, confidence, details}`

## Invariants

- Supersession-linked pair never split.
- Every decision logged with provenance.
- No judge call when not configured.

## Tests

- 20 learnings / 4 clusters / 3 articles → 1 new theme, 3 assignments correct.
- Supersession pair → same theme.
- Embeddings unavailable → tag fallback.
- Tight margin → tie-breaker path.
