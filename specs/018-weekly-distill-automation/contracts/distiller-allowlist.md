# Contract: Distiller Allowlist (global-only)

## Agent Files

- `.opencode/agents/distiller.md` — unattended variant, `command: dewey-distill-auto`
- `.opencode/agents/curator.md` — interactive unchanged (both HUMAN_GATE questions)

## Allowed (distiller)

- `read`, `list_pages`, `get_page`, `lint`, `compile`, `curate`, `verify_candidates`, `store_compiled` (draft only), `supersede_learning` (append), filesystem git reads (`git ls-tree`, `git show`, `git rev-parse` read-only).
- **Execution surface for rolling PRs** (required for deployability — does not widen beyond PRs):
  - Filesystem: write/regenerate only under `.uf/dewey/compiled/**` and `.uf/dewey/distill-runs/**` (draft `tier: draft` only; validated write rejected by server guard).
  - Git: `git fetch`, `git rebase --preserve-merges`, `git commit`, `git push --force-with-lease` for `dewey-distill/*` branches only; no `push --force`, no rebase/force on other refs.
  - GitHub: `gh pr list --label dewey-distill`, `gh pr create --label human-gate`, `gh pr edit`, `gh issue list/comment` via MCP `gh` tool — no `gh api` generic write, no CODEOWNERS edits.
  - Registration: implements `tools/scope.go`, `tools/group.go`, `tools/rolling_pr.go` exposed as MCP tools `scope`, `group_learners`, `rolling_pr` in `server.go`; added to distiller allowlist explicitly.

## Denied (distiller, always)

- `question` (any) — rejected + logged, fallback to deterministic scope.
- Any `tier: validated` write — server guard in `tools/promote.go` / `store/` rejects, file stays `draft`.
- `promote` to validated — denied.
- `git push --force` (without `--force-with-lease`), branch deletion, workflow file writes, `CODEOWNERS`/`.github/**` edits, `replicator_*` MCP servers.

## Widening Rule

Only global `opencode-supervisor` `command: dewey-distill-auto` profile may narrow/widen; repo-local `.opencode/supervisor.yaml` additions for distiller ignored. Verified by config-precedence test (repo-local `question` add still denied). Until `jflowers/opencode-supervisor#51` ships, the mechanism is not verifiable — see Assumptions fail-closed handling: unattended mode is not enabled and the workflow creates only a no-op artifact + notice comment.

## Interactive `/dewey-distill` unchanged

Retains both `HUMAN_GATE:` prompts + full curator tool set.

## Tests

- Distiller `question` → reject.
- Distiller validated write → reject, remains draft.
- Repo-local widen → ignored.
- Interactive → both gates present.
- Distiller `push --force` → reject; `push --force-with-lease` on `dewey-distill/*` → allow.
- `gh pr create` outside `dewey-distill/*` → reject.
