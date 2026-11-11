# Manual Acceptance Matrix — curator-tool-allowlist (task 4.2)

Runtime verification that the installed host (opencode 1.19.0) honors the
curator agent's frontmatter `tools:` globs. Performed 2026-11-11 on
`opsx/curator-tool-allowlist` at commit 28a9186 (frontmatter change) with
docs commit 8b1dba1 in the tree.

## Method

`opencode run --agent` cannot invoke `mode: subagent` agents (it falls back
to the default agent with a visible warning), so the matrix ran against a
throwaway probe agent: a byte-identical copy of `.opencode/agents/curator.md`
with only `mode: subagent` → `mode: primary`. Agent permission evaluation is
mode-independent; the `tools:` map under test was identical. The probe file
was deleted after the matrix.

## Results

| # | Check | Result | Evidence |
|---|-------|--------|----------|
| 1 | Allowlisted Dewey MCP call succeeds | **PASS** | `dewey_list_pages` (limit 5) executed and returned 5 real pages from the vault store. `dewey_lint` was also permitted and executed; it returned a dewey-domain storage error (see environmental notes) — the permission layer granted both calls. |
| 2 | Non-allowlisted Dewey MCP call refused by frontmatter | **PASS** | `dewey_delete_page` is absent from the agent's toolset entirely. Asked to attempt the call, the agent reported the tool "is not present in my available tools" — host-level enforcement (the tool is never offered to the model), not prompt discipline. |
| 3 | `replicator_*` MCP call refused | **PASS** | Toolset enumeration showed zero `replicator_*` tools — the entire replicator server is denied by `"replicator_*": false`. |
| 4 | Human-gate questions remain askable | **PASS** | The `permission:` block (`question: allow`) is byte-identical to the pre-change frontmatter — the change touched only the `tools:` block, and `TestCuratorAgent_FrontmatterToolAllowlist` asserts the permission block's exact entries. |
| 5 | `dewey gate auto-answer` bash carve-out unaffected | **PASS** | The exact command `dewey gate auto-answer` was permitted by the nested bash map, and any variant (e.g. appending `; echo $?`) was denied by the `"*": deny` rule — the carve-out's allow-exact-one-command shape is enforced. The command itself failed with `unknown command "gate"` because the installed dewey binary (v3.5.3 via mise) predates the gate subcommand from #152 — environmental version skew, unrelated to this change (gate.go and the bash map are untouched by it). |

## Design-D4 glob verification

**The glob keys are honored.** `"dewey_*": false` removes every Dewey tool
from the agent's toolset; the six `true` entries restore exactly the
allowlist (`dewey_lint`, `dewey_list_pages`, `dewey_compile`, `dewey_curate`,
`dewey_store_compiled`, `dewey_promote`); `"replicator_*": false` removes
every replicator tool. The explicit per-tool enumeration fallback (task 4.2's
contingency) is NOT required.

Toolset enumeration observed by the probe agent:

```text
dewey_compile, dewey_curate, dewey_lint, dewey_list_pages,
dewey_promote, dewey_store_compiled          (exactly six)
replicator_*: (none)
```

## Environmental notes (non-blocking)

- The probe's spawned dewey server (a second `dewey serve --vault .` alongside
  the orchestrating session's server) returned `lint requires persistent
  storage` for `dewey_lint` while `dewey_list_pages` served real pages — a
  contention artifact of running two servers on one vault during the test,
  not an allowlist behavior.
- `dewey gate auto-answer` failing with "unknown command" is installed-binary
  version skew (mise dewey v3.5.3 predates #152's gate feature); the bash
  permission carve-out itself is proven functional.
- An earlier probe round against `--agent curator` directly was invalid:
  opencode silently (to stdout) fell back to the default agent because the
  curator is `mode: subagent`. Matrix conclusions above come from the
  primary-mode probe only.
