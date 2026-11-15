# Contract: Distiller Allowlist (global-only)

## Agent Files

- `.opencode/agents/distiller.md` — unattended variant, `command: dewey-distill-auto`
- `.opencode/agents/curator.md` — interactive unchanged (both HUMAN_GATE questions)

## Allowed (distiller)

- `read`, `list_pages`, `get_page`, `lint`, `compile`, `curate`, `verify_candidates`, `store_compiled` (draft only), `supersede_learning` (append), filesystem git reads.

## Denied (distiller, always)

- `question` (any) — rejected + logged, fallback to deterministic scope.
- Any `tier: validated` write — server guard in `tools/promote.go` / `store/` rejects, file stays `draft`.
- `promote` to validated — denied.

## Widening Rule

Only global `opencode-supervisor` `command: dewey-distill-auto` profile may narrow/widen; repo-local `.opencode/supervisor.yaml` additions for distiller ignored. Verified by config-precedence test (repo-local `question` add still denied).

## Interactive `/dewey-distill` unchanged

Retains both `HUMAN_GATE:` prompts + full curator tool set.

## Tests

- Distiller `question` → reject.
- Distiller validated write → reject, remains draft.
- Repo-local widen → ignored.
- Interactive → both gates present.
