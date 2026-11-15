# Contract: Distiller Allowlist (global-only)

## Agent Files

- `.opencode/agents/distiller.md` — unattended variant, `command: dewey-distill-auto`
- `.opencode/agents/curator.md` — interactive unchanged (both HUMAN_GATE questions)

## Allowed (distiller)

MCP tools: `read`, `list_pages`, `get_page`, `lint`, `compile`, `curate`, `verify_candidates`, `store_compiled` (draft only), `supersede_learning` (append), plus the three pipeline tools `scope`, `group_learners`, `rolling_pr` (registered in `server.go`, implemented by `tools/scope.go`, `tools/group.go`, `tools/rolling_pr.go`).

Shell (bash allowlist, exhaustive):

- `dewey gate auto-answer`, `dewey gate distill-unattended` — the two mandatory startup gates.
- `gh issue comment` — the no-op notice posted on the dispatch issue when a gate refuses.

Nothing else. In particular, **no raw git or gh PR command is on the bash allowlist**.

### Why no raw git/gh writes

Allowlist entries are prefix globs — they match command names (and prefixes), but cannot restrict a command's arguments. An entry like `git push --force-with-lease` allows force-pushing *any* ref; "`dewey-distill/*` branches only" is not expressible at the shell layer. Rather than grant a raw write capability and scope it in prose (which the shell cannot enforce), every git and gh mutation — fetch, rebase, commit, `push --force-with-lease`, `gh pr list`/`create`/`edit` — happens **server-side inside the `rolling_pr` MCP tool**, where the `dewey-distill/*` branch confinement, human-commit preservation, and validated-tier guards are enforced in code. The distiller's shell surface is therefore read-free and write-free: it holds no capability the server has not already bounded.

### Server-side execution surface (rolling_pr tool)

The `rolling_pr` tool performs, inside the server process:

- Filesystem: write/regenerate only under `.uf/dewey/compiled/**` and `.uf/dewey/distill-runs/**` (`tier: draft` only; validated write rejected by server guard).
- Git: `fetch`, rebase, `commit`, `push --force-with-lease` on `dewey-distill/*` branches only; no `push --force`, no rebase/force on other refs.
- GitHub: `gh pr list --label dewey-distill`, `gh pr create --label human-gate`, `gh pr edit` — no `gh api` generic write, no CODEOWNERS edits.

## Denied (distiller, always)

- `question` (any) — rejected + logged, fallback to deterministic scope.
- Any `tier: validated` write — server guard in `tools/promote.go` / `store/` rejects, file stays `draft`.
- `promote` to validated — denied.
- Every raw git and gh command except the three bash allowlist entries above — including all of `git fetch`/`rebase`/`commit`/`push` (any form) and all `gh pr`/`gh issue list` forms. `git push --force` and `git push --force-with-lease` are equally rejected: neither is on the allowlist.
- Workflow file writes, `CODEOWNERS`/`.github/**` edits, `replicator_*` MCP servers.

## Widening Rule

Only global `opencode-supervisor` `command: dewey-distill-auto` profile may narrow/widen; repo-local `.opencode/supervisor.yaml` additions for distiller ignored. Verified by config-precedence test (repo-local `question` add still denied). Until `jflowers/opencode-supervisor#51` ships, the mechanism is not verifiable — see Assumptions fail-closed handling: unattended mode is not enabled and the workflow creates only a no-op artifact + notice comment.

## Interactive `/dewey-distill` unchanged

Retains both `HUMAN_GATE:` prompts + full curator tool set.

## Tests

- Distiller `question` → reject.
- Distiller validated write → reject, remains draft.
- Repo-local widen → ignored.
- Interactive → both gates present.
- Distiller `git push --force` **and** `git push --force-with-lease` → both rejected (no raw git on the bash allowlist); the rolling_pr tool alone pushes, confined to `dewey-distill/*`.
- Distiller `gh pr create` (any form, any branch) → rejected.
- Bash allowlist shape: deny-all `"*"` precedes exactly three allows — the two gates and `gh issue comment` (frontmatter shape test).
