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

## Branch Sanitization (FR-011)

Theme names derive from attacker-influenced tags/embeddings (394 near-duplicate tags) and optional LLM judge. Branch mapping `dewey-distill/<sanitized>` MUST define charset, length, and collision handling:

- Lowercase.
- Replace any rune not in `[a-z0-9]` with `-`.
- Collapse consecutive `-` to single `-`; trim leading/trailing `-`.
- If empty after sanitization → `untagged`.
- Truncate base to 50 chars (trim trailing `-` after truncation).
- Append `-` + 8-hex short-hash `sha256(theme)[:8]`.
- Example: `Auth/SSO..Flow` → `auth-sso-flow-a1b2c3d4`; `/` → `untagged-<hash>`; `../../etc/passwd` → `etc-passwd-<hash>`; 250-char tag → `a…(50)-<hash>`; control chars / Unicode → `-` collapsed.
- Collision: hash suffix makes collisions negligible; if two themes still collide on `dewey-distill/<sanitized>` (hash collision), append `-2`, `-3` by lexicographic theme order.
- Validation: `[a-z0-9-]{1,59}` after prefix; max total `dewey-distill/<sanitized>` 80 chars; traversal rejected if `..` or `/` in sanitized.

Tests: `/`, `..`, `//`, control chars, 250-char, Unicode, duplicate theme names → distinct branches; `[a-z0-9-]` only.

## Sanitization Gate

Learnings are scanned by `sanitize` per-source; grouping→compile→PR-body MUST consume findings:

- Critical/high findings on a learning (prompt injection, data URIs, exfiltration links): drop from synthesis prompt; note `sanitization: dropped <id> (critical)` in PR body.
- PR body fields (`What changed`, `Newly cited`, `Supersessions`) interpolate only escaped Markdown: escape `[`, `]`, `(`, `)`, `!`, `#`, `*`, `_`, `` ` ``, `<`, `>`; truncate identity lists at 100, link to full scope file.
- Synthesis prompt omits HTML comments, `<!--` blocks, and `data:` URIs from learning content.

## Human-Commit Preservation & Regeneration Guard (FR-015)

- Preservation is not only rebase semantics: before regenerating draft articles, compare working-tree file content to `origin/<branch>` last-committed content for that path.
- If human edited a file the run also regenerates (content differs from run's last committed output for that path), refuse to overwrite: label `needs-human-resolution`, list files in body, skip that file; human work stays in history and is surfaced, not silently replaced.
- Rebase preserves fixup commits by message (`fixup!` or non-distiller author) — verified by commit-message preservation.

## Concurrency for PR Creation

`concurrency.group` serializes workflow runs only. Two distiller runs racing on `gh pr list → gh pr create` for same theme MUST:

- Search for existing branch/PR immediately before create; if conflict (branch exists), treat as update path.
- Push uses `git push --force-with-lease=<branch>:<expected-sha>` CAS; on `stale info` retry as update once.

## Review Queue Body (per article)

```md
## <article title> (`<path>`)
**What changed**: <1-line theme delta>
**Newly cited**: `learning/<id>` (N)
**Supersessions**: `<successor> supersedes <target>`
**Lint**: `anchor_drift` / `embedding_gap` findings
**Confidence**: avg + per-decision distances
```

Escaped per Sanitization Gate; capped at 65k chars total body.

## Tests

- Two themes → two PRs, draft only.
- Second run updates same PR, preserves fixup commit.
- Empty update → SHA stable.
- Closed → new PR.
- Conflict → label + body notes.
- Attempted validated write → rejected.
- Poison tag `../../x` → sanitized branch, no traversal.
- Human edit + regeneration → not overwritten, flagged.
- Racing create → single PR, second becomes update.
