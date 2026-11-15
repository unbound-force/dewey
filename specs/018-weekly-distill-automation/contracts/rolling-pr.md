# Contract: Rolling PR

## Input

- `themes: Theme[]`
- Existing open PRs (via `gh pr list --label dewey-distill --state open`)

## Behavior

Per theme:

1. Branch `dewey-distill/<sanitized>`.
2. If open PR exists for branch → `git fetch origin/<branch>`, rebase preserving human commits, regenerate draft articles + hygiene, `gh pr edit` body, `git push --force-with-lease`.
3. Else → create branch, write draft articles (`tier: draft` + `validity`/`superseded_by` hygiene), `gh pr create --label human-gate --body ReviewQueue`.
4. Conflicts on rebase → leave conflict markers, label `needs-human-resolution`, body lists files.
5. Closed PR for same theme → fresh create, never reopen.
6. No new learnings → leave open PR untouched, no empty push.
7. Never write `tier: validated`; server guard rejects.

## Review Queue Body (per article)

```md
## <article title> (`<path>`)
**What changed**: <1-line theme delta>
**Newly cited**: `learning/<id>` (N)
**Supersessions**: `<successor> supersedes <target>`
**Lint**: `anchor_drift` / `embedding_gap` findings
**Confidence**: avg + per-decision distances
```

## Tests

- Two themes → two PRs, draft only.
- Second run updates same PR, preserves fixup commit.
- Empty update → SHA stable.
- Closed → new PR.
- Conflict → label + body notes.
- Attempted validated write → rejected.
