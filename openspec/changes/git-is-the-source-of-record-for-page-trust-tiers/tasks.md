<!--
  Task markers: `[P]` marks a task safe to run in parallel with the
  other `[P]` tasks in its group — it touches different files, depends
  on no prior task in the group, and needs no ordering. Unmarked tasks
  run sequentially first; `[P]` tasks run after them.
-->

## 1. Trust derivation core (ingest pipeline)

- [x] 1.1 `vault/` or `main.go` ingestion: introduce `deriveEffectiveTier` helper that takes file frontmatter tier + file blob SHA + default-branch blob SHA + downgrade signals (enumerated predicate per GTS-FR-003: `validity` ∈ {`stale-unverified`, `superseded`, `historical`} canonical trimmed case-sensitive lowercase OR frontmatter `tier: draft` canonical; unknown/non-canonical values like `tier: Validated` never raise trust) and returns effective tier per GTS-FR-001 + GTS-FR-003 (validated only when canonical `tier: validated` AND blobs match; downgrade wins regardless of match); GoDoc, pure function, no I/O
- [x] 1.2 Wire helper into ingestion for `compiled/` and `knowledge/` (curated) pages; keep `learning/` clamp in `main.go:1087` (learnings stay `draft` per IP-FR-002) and remove/disable the compiled-page draft clamp so IP-FR-002 is satisfied; ensure `tier` from DB is recomputed on next index (GTS-FR-005)
- [x] 1.3 Tests: table-driven `deriveEffectiveTier` cases — canonical `validated`+match→validated, `Validated`/`VALIDATED`/whitespace-padded variants→unknown→draft even with blob match, `validated`+mismatch→draft, `validated`+match+stale→downgrade, `draft` downgrade wins even with match, unknown `validity`/`tier` values never raise trust (GTS-FR-003 canonical), learning-path always draft; no git needed
- [x] 1.4 Phase checkpoint — `go test -race -count=1 ./...` passes before phase 2

## 2. Git ref resolution (offline, batch fast path)

- [x] 2.1 Git resolver: resolve `<remote>/<default-branch>` from local git state only (default `origin/main`; config precedence per-vault → global → env `DEWEY_DEFAULT_BRANCH_REF` → git config → default per IP-FR-004 with `<remote>/<branch>` grammar validated per `git check-ref-format` before any git invocation); record precedence leg (or fallback) in diagnostics observable in logs and via `dewey doctor`; invoke git plumbing via argv arrays with `--` separators (IP-FR-004); return ref name + SHA or resolvable-missing signal for GTS-FR-004 fallback; any git derivation failure (missing binary, permission denied, corrupt object DB, malformed/truncated output, non-zero exit not meaning "ref not found", malformed/injection config like `--upload-pack=evil` rejected before git) and hung process past bounded context timeout (5s per invocation, 60s cumulative per source per `tools/lint_anchors.go` precedent, cancellation propagated to git child) degrade affected pages to `draft` with diagnostic and do not fail ingestion; no network fetch; GoDoc; injectable git runner for tests
- [x] 2.2 Batch fast path: `git ls-tree -r <ref> -- <compiled|knowledge paths>` once per source per index to map path→blob for all N pages; per-page `hash-object`/`cat-file` fallback when batch unavailable; wire into ingestion so IP-FR-001 batch scenario passes
- [x] 2.3 Tests with `t.TempDir()` fixture repos: blob-match→validated, worktree edit→draft, branch-only→draft, regenerated blob B vs default-branch blob A → draft (#204), missing ref → draft fallback; verify `ls-tree` batch + fallback both exercised; `-race`
- [x] 2.4 Phase checkpoint — `go test -race -count=1 ./store/... ./vault/... ./tools/...` passes before phase 3

## 3. Promote becomes file edit + migration fix

- [x] 3.1 `tools/promote.go` + MCP `promote` registration: change from `UPDATE pages SET tier='validated'` to frontmatter mutation (`tier: validated`) on current branch file(s); resolve the page argument to a canonical vault-relative path starting from vault root — clean joined path, resolve symlinks in every component to a fully resolved absolute path, verify fully resolved path stays inside vault root and under `compiled/` or `knowledge/` prefixes (PW-FR-004); reject absolute paths, `..` traversal, and paths outside allowed prefixes; perform the edit atomically — temp file in same directory + rename preserving mode, failure leaves original unchanged and returns an error (PW-FR-001); final mutation MUST use no-follow on final component (`O_NOFOLLOW` or dirfd-relative handle after validation) so TOCTOU symlink swap is treated as validation failure and no file outside allowed prefixes is modified; file diff/PR is the artifact; DB write if any is derived-only and does not outlive next index (PW-FR-002); update `cli.go` promote help text if present
- [x] 3.2 `store/migrate.go`: deterministic migration (IP-FR-003) — v1→v2→v3 migration MUST NOT modify `pages.tier` for any existing rows; keep stored tier unchanged, effective tier re-derived only on next index per IP-FR-001 (GTS-FR-001 blob equality + GTS-FR-003 downgrade); bump/test `schemaVersion = 3` migration path; learnings path unchanged; fresh DB built from vault whose compiled pages are `validated` on default branch ingests as `validated` via IP-FR-001; upgrade from v2 with row at `validated` leaves `pages.tier` unchanged, next index reflects IP-FR-001
- [x] 3.3 `tools/promote_test.go` + `vault/`/`store` tests: promote edits file, branch-only indexes as draft, post-merge+fetch indexes as validated (PW-FR-003); path-traversal/absolute/outside-prefix rejection and atomic-failure-leaves-original-unchanged cases (PW-FR-004, PW-FR-001); pre/post migration ingest cases; two servers different DB same fetched ref → same tiers (GTS-FR-005)
- [x] 3.4 Phase checkpoint — `go test -race -count=1 ./...` passes before phase 4

## 4. Diagnostics and acceptance wiring

- [x] 4.1 `dewey doctor` / `dewey status` diagnostics: surface missing/unresolvable default-branch ref note (GTS-FR-004 fallback) and effective-tier derivation observability (file tier, default-branch blob vs current blob, fallback reason); `charmbracelet/log` compliant, no `fmt.Fprintf`
- [x] 4.2 Acceptance tests: end-to-end fixture covering six listed acceptances — identical→validated, modified working-tree/branch-only→draft, validated but stale→stale, learning claiming validated on default branch→draft, two servers same ref→same tiers, promote changes file not DB and branch-only stays draft
- [x] 4.3 Non-goals guard: verify no CODEOWNERS/required-review files added (#194 out of scope), no network fetch introduced, no new deps/CGO, store-less mode untouched
- [x] 4.4 Phase checkpoint — `go test -race -count=1 ./...` passes before phase 5

## 5. Docs and verification

- [x] 5.1 [P] Documentation (unconditional): update `README.md` + `AGENTS.md` trust semantics (trust-from-git, the enumerated downgrade predicate, promote-as-file-edit, migration note); update `promote` CLI help text and the `promote` MCP tool description (frontmatter edit for a PR, not a DB tier write); document the new `dewey doctor`/`dewey status` tier-derivation diagnostics (missing ref, blob mismatch, git failure fallback); file the Website Documentation Sync issue (`gh issue create --repo unbound-force/website --label docs`) covering promote semantics, tier display, doctor diagnostics, and the migration note; keep `isPlanningComplete: false` until applied
- [x] 5.2 Constitution-alignment verification: re-read four principles (`.specify/memory/constitution.md`) against final diff — Composability First (no new service, offline git), Autonomous Collaboration (PR merge is vouch), Observable Quality (blob equality + doctor), Testability (pure helper + fixture repos); record any drift
- [x] 5.3 CI parity: derive exact commands from `.github/workflows/ci.yml` at run time (`go build -v ./...`, `go vet ./...`, `go test -race -count=1 -coverprofile=coverage.out ./...`, Gaze `crap --max-crapload=48 --max-gaze-crapload=45`); any failure blocks completion

<!-- scaffolded by uf vdev -->
