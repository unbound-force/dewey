# Delta Spec: artifact-publishing

## ADDED Requirements

### Requirement: Stranded-State Enumeration

The plan command MUST enumerate every Dewey knowledge artifact from
four stranded states: the working tree (including untracked,
git-ignored, and staged files), stash tracked parts, stash untracked
parts, and unpushed commits — commits reachable from any local
branch or a detached HEAD but not from any remote-tracking ref
(stash refs are enumerated by their own mechanism; tag-reachable
commits are out of scope). Each enumerated artifact MUST carry
provenance: repository-relative path, identity, content sha256, and
stranded state. Enumeration MUST consider only regular files in
every source: symbolic links and other non-regular file types MUST
be skipped and surfaced as plan warnings — never read, hashed,
planned, or published. Basenames containing control characters
(newline, tab, NUL), a colon (ambiguous in `<rev>:<path>` revision
syntax), or a leading hyphen MUST likewise be skipped in every
source with a surfaced warning. A directory or file that cannot be
read during enumeration (permission errors, I/O errors) MUST fail
closed with a wrapped, actionable error naming the path —
enumeration MUST NOT silently skip unreadable content. Enumeration
walk roots — the vault
root, the repository root, the `.uf` and `.uf/dewey` path components,
artifact directory components, and store-path prefixes — MUST be
verified free of symbolic links: every path component Lstat-checked,
or each walk root resolved via symlink resolution with the resolved
root required to remain inside the resolved repository root. A
symlinked walk-root component MUST fail closed with an actionable
error naming the walk root and the offending component — lexical
containment is not physical containment, and files reached through
symlinked intermediate directories Lstat as regular files, defeating
entry-level checks.

#### Scenario: Working-tree artifacts including ignored files

- **GIVEN** a git repository whose remote default branch is fetched, with
  learnings in `.uf/dewey/learnings/` that are untracked, git-ignored,
  and staged
- **WHEN** `dewey artifacts plan --json` runs
- **THEN** every artifact file under the artifact directories is
  enumerated with source provenance identifying the working tree,
  regardless of its tracked/ignored/staged state

#### Scenario: Stash tracked part

- **GIVEN** a stash created from tracked modifications containing
  `.uf/dewey/learnings/auth-20260401T101112-alice.md`
- **WHEN** the plan command runs
- **THEN** the stashed learning is enumerated with provenance
  identifying `stash@{N}`, and its content is read via
  `git show stash@{N}:<path>` without applying the stash

#### Scenario: Stash untracked part

- **GIVEN** a stash created with `git stash -u` whose untracked files
  include `.uf/dewey/compiled/auth.md`
- **WHEN** the plan command runs
- **THEN** the untracked-part commit `stash@{N}^3` is probed, the
  compiled article is enumerated with provenance identifying the stash
  untracked part, and stashes without a `^3` parent are skipped
  without error

#### Scenario: Unpushed branch commits

- **GIVEN** a local branch with unpushed commits that add
  `.uf/dewey/knowledge/team/meeting-notes.md`
- **WHEN** the plan command runs
- **THEN** the artifact is enumerated from the newest unpushed commit
  touching it, with provenance identifying the commit

#### Scenario: Symbolic links are skipped, not published

- **GIVEN** a symbolic link under `.uf/dewey/learnings/` pointing at
  a file outside the artifact directories
- **WHEN** `dewey artifacts plan --json` runs
- **THEN** the symlink is not enumerated, hashed, or published, and
  the plan surfaces a warning identifying the skipped link

#### Scenario: Symlinked walk-root component fails closed

- **GIVEN** a repository whose `.uf` component is a symbolic link
  pointing outside the repository
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an error naming the walk
  root and the offending component, and no plan is produced

#### Scenario: Unreadable directory fails closed

- **GIVEN** an artifact directory whose permissions deny the
  enumeration read
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an error naming the
  unreadable path, and no plan is produced

### Requirement: Artifact Directory Coverage

Enumeration MUST cover `.uf/dewey/learnings/`,
`.uf/dewey/compiled/`, and `.uf/dewey/knowledge/` — the default
trio resolved under the vault root (vault root from `--vault`, else
`OBSIDIAN_VAULT_PATH`, else the working directory; Dewey writes
these directories at `<vaultPath>/.uf/dewey/`) — as well as the
same trio at the git repository root when it differs from the
vault-anchored location (working-tree walks cover both, deduplicated
by repository-relative path), plus each configured knowledge-store
path resolved from `.uf/dewey/knowledge-stores.yaml`. Stash and
commit tree filters MUST match the sanctioned root set — the trio at
any depth (`**/.uf/dewey/{learnings,compiled,knowledge}/**`) plus
each configured knowledge-store path prefix relative to the
repository root — so vault-in-subdirectory layouts and custom store
paths are not silently missed. The vault
root MUST be inside the git repository — a vault outside the
repository MUST fail closed with an actionable error naming both the
vault root and the repository root. A configured knowledge-store
path that resolves outside the repository, or whose configured
value contains control characters, a colon, or a leading-hyphen
component, MUST fail closed with an actionable error naming the
store and its resolved path. Missing
artifact directories MUST be treated as empty, and hidden files
(dot-prefixed, e.g. `.DS_Store`) MUST be skipped in every
enumeration source — the working tree, both stash parts, and commit
trees.

#### Scenario: Missing artifact directories

- **GIVEN** a repository with no `.uf/dewey/learnings/`,
  `.uf/dewey/compiled/`, or `.uf/dewey/knowledge/` directories
- **WHEN** the plan command runs
- **THEN** the plan succeeds with zero enumerated artifacts

#### Scenario: Custom knowledge-store path

- **GIVEN** `.uf/dewey/knowledge-stores.yaml` declaring a store with a
  custom in-repository path
- **WHEN** the plan command runs
- **THEN** artifacts under the custom path are enumerated alongside
  the default directories

#### Scenario: Store path outside the repository fails closed

- **GIVEN** `.uf/dewey/knowledge-stores.yaml` declaring a store whose
  resolved path lies outside the repository root
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an error naming the store
  and its resolved path, and no plan is produced

#### Scenario: Vault in a repository subdirectory

- **GIVEN** a vault root at `docs/vault` inside the git repository,
  with learnings at `docs/vault/.uf/dewey/learnings/` and additional
  artifacts at the repository root's `.uf/dewey/learnings/`
- **WHEN** the plan command runs
- **THEN** artifacts from both the vault-anchored trio and the
  repository-root trio are enumerated, deduplicated by
  repository-relative path, and stash or commit tree filters match
  the trio at any depth

#### Scenario: Vault outside the repository fails closed

- **GIVEN** a vault root resolved outside the git repository root
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an actionable error naming
  the vault root and the repository root, and no plan is produced

### Requirement: Non-Destructive Execution

The sweep MUST NOT pop, apply, or drop any stash; MUST NOT move,
modify, or delete any working-tree file; and MUST NOT modify the
user's checkout. `git status` and `git stash list` output MUST be
identical before and after the full plan-and-sync flow.

#### Scenario: Zero mutation across the full flow

- **GIVEN** a fixture repository containing artifacts in all four
  stranded states, with `git status --porcelain` and `git stash list`
  captured before the run
- **WHEN** `dewey artifacts plan` and `dewey artifacts sync` both run
  to completion
- **THEN** `git status --porcelain` and `git stash list` are
  byte-identical to the captured before-state

### Requirement: Deterministic Plan Output

`dewey artifacts plan --json` MUST produce byte-identical JSON output
for two consecutive runs over an unchanged repository. Plan output
MUST NOT embed run-volatile data (no generation timestamps, no
temporary paths), and all collections MUST be serialized in a stable
canonical order.

#### Scenario: Twice-test

- **GIVEN** an unchanged repository with artifacts in multiple
  stranded states
- **WHEN** `dewey artifacts plan --json` runs twice
- **THEN** the two outputs are byte-identical

### Requirement: Path Dedup

The plan MUST deduplicate artifacts by repository-relative path
across all enumeration sources — path is the deduplication,
collision, conflict, and resolution key. Identity (the learning
filename, compiled-article name, or store path) MUST be retained on
each entry as display and PR-grouping metadata only. Artifacts with
the same path and identical content sha256 MUST collapse to a single
entry recording every source.

#### Scenario: Same content in two states deduplicates

- **GIVEN** the same artifact path with identical content in the
  working tree and in a stash
- **WHEN** the plan command runs
- **THEN** the plan lists the artifact once, with provenance recording
  both sources

### Requirement: Base-Tree Comparison

The publish remote MUST be resolved deterministically: `upstream`
when a git remote with that name is configured, else `origin`. When
`upstream` is the resolved publish remote, an `origin` remote MUST
also exist (it is the push remote in fork layouts); its absence MUST
fail closed with an actionable error naming both remotes. The
base tree is the fetched default branch of the resolved publish
remote. Artifacts whose content sha256 matches that tree entry at
the same path MUST be excluded from the publish set. The plan MUST
fail closed with an actionable error naming the resolved remote when
its base ref cannot be resolved.

#### Scenario: Artifact already on the remote base

- **GIVEN** a learning whose path and content sha256 already exist in
  the fetched remote default-branch tree
- **WHEN** the plan command runs
- **THEN** the artifact is excluded from the publish set and the plan
  reports nothing new for it

#### Scenario: Unresolvable base ref fails closed

- **GIVEN** a repository whose remote default branch cannot be
  resolved after fetching
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an error directing the user
  to fetch the remote

#### Scenario: Upstream without origin fails closed

- **GIVEN** a repository with an `upstream` remote configured but no
  `origin` remote
- **WHEN** the plan command runs
- **THEN** the command exits non-zero with an actionable error naming
  both remotes and produces no plan

### Requirement: Collision Detection

Collisions are defined across enumeration sources only: when the same
repository-relative path exists with different content (different
sha256) in two or more sources, the plan MUST surface it as a
conflict listing every variant identified by sha256 and
stranded-state source. A path whose content differs from the base
tree entry is NOT a collision — it is a plain `new` entry (updated
content republished, by design); base-tree comparison excludes only
identical content.

#### Scenario: Collision flagged in plan

- **GIVEN** `.uf/dewey/learnings/auth-20260401T101112-alice.md`
  exists with content A in the working tree and content B in a stash
- **WHEN** the plan command runs
- **THEN** the plan reports a conflict for that path listing both
  variants with their sha256 values and sources

### Requirement: Plan-Time Content Screening

The plan MUST scan every candidate artifact's content — including
every variant of every conflict — with two scanners before the
artifact enters the publish set: the existing `sanitize` package's
injection-pattern scan AND a credential-detection pattern set
defined in the artifacts package (standard-library regular
expressions matching common credential formats — API-key prefixes
such as `sk-` and `ghp_`, AWS access-key ids, private-key blocks,
and credential-bearing connection strings — introducing no new
runtime dependencies). Each credential-detection pattern MUST carry a
documented per-pattern severity mirroring the sanitize injection
patterns' severity discipline — `critical` for private-key blocks
and credential-bearing connection strings, `high` for key-prefix
patterns — and the blocking threshold is `critical` or `high`. The
pattern set is a pre-exposure floor, not a ceiling: it is the only
control that runs before PR creation (secret scanners such as gitleaks
inspect the repository only after the PR exists), and its
false-negative boundary MUST be documented in the README. Artifacts whose scan produces `critical` or
`high` severity findings MUST be excluded from the publish set and
listed in a `blocked` collection carrying the findings; each finding
MUST record which scanner produced it (`injection` or `credential`)
and MUST NOT carry the scanner's matched-content excerpt, so blocked
findings never re-deliver payload to the gate. Lower-severity
findings MUST be carried on the artifact entry (a `findings`
collection, non-blocking) and surfaced as warnings in the plan's
tool-rendered human summary. Blocked
artifacts MUST NOT be written by sync.

Screening precedence is defined per variant: a conflict entry lists
every variant each carrying its own findings (empty when clean);
variants with `critical` or `high` findings are blocked variants —
MUST NOT be offered as or accepted as resolution choices, and a
resolution naming one MUST fail closed. A path whose every variant
is blocked appears only in the `blocked` collection; the conflicts
collection holds only paths with at least one clean variant.

The plan pipeline order MUST be: enumeration → dedup → collision
detection → screening (all candidates including variants) →
gitignore detection.

#### Scenario: Credential-like content is blocked

- **GIVEN** a learning whose content embeds an API key matching a
  credential-detection pattern
- **WHEN** the plan command runs
- **THEN** the learning is excluded from the publish set, listed in
  the plan's blocked collection with a `credential`-scanner finding
  carrying no matched-content excerpt, and sync does not write it

#### Scenario: Blocked variant is not resolvable

- **GIVEN** a path with a clean working-tree variant and a stash
  variant whose scan produces a critical finding
- **WHEN** the plan command runs, and sync is invoked with a
  resolution selecting the flagged variant
- **THEN** the plan lists the conflict with per-variant findings, and
  sync fails closed on the resolution

#### Scenario: All variants blocked

- **GIVEN** a path whose every variant across sources carries a
  critical or high finding
- **WHEN** the plan command runs
- **THEN** the path appears only in the blocked collection with
  per-variant findings and is never offered as a conflict resolution

### Requirement: Fail-Closed Conflict Resolution

Sync MUST refuse to execute while the plan contains unresolved
conflicts. Collision resolutions MUST arrive as parameters to sync (a
JSON object mapping repository-relative path to chosen sha256) and
MUST be validated against the plan's conflict entries; a resolution
keying an unknown path, naming a sha256 that is not a variant of
that path's conflict, or naming a blocked variant MUST fail closed.

#### Scenario: Sync refuses unresolved conflicts

- **GIVEN** a plan containing an unresolved conflict
- **WHEN** `dewey artifacts sync --plan <file>` runs without a
  resolution for that path
- **THEN** sync exits non-zero without creating any branch, commit, or
  worktree

#### Scenario: Resolution parameter unblocks sync

- **GIVEN** a plan containing a conflict and a resolution file
  selecting one variant's sha256 for that conflict's path
- **WHEN** sync runs with `--resolutions <file>`
- **THEN** sync publishes the chosen variant's content and the commit
  contains only that content for the identity

### Requirement: Gitignore Bug Detection and Fix

The plan MUST detect, via `git check-ignore -v`, the bare-`dewey`
gitignore pattern (an unanchored `dewey` line that ignores
`.uf/dewey/` artifact paths) and MUST flag the bounded fix block —
anchoring the pattern to `/dewey` AND appending re-ignore lines for
non-publishable runtime state and private configuration
(`.uf/dewey/graph.db`, `.uf/dewey/graph.db-shm`,
`.uf/dewey/graph.db-wal`, `.uf/dewey/dewey.log`,
`.uf/dewey/dewey.lock`, `.uf/dewey/cache/`,
`.uf/dewey/config.yaml`, `.uf/dewey/sources.yaml`,
`.uf/dewey/knowledge-stores.yaml`) — so anchoring does not silently un-ignore
runtime state and private configuration after the merge. The fix
block MUST be golden-pinned, and the plan MUST surface the
post-merge staging consequence: after the block merges, artifact
directories become stageable while runtime state stays ignored.
Detection MUST be skipped when
the pattern is already fixed or artifact paths are not ignored.
`git check-ignore -v` reports only the last matching pattern, so a
wholesale `.uf/` ignore coexisting with the bare `dewey` line is
classified as `other-ignore-warning` (documented degradation). When
flagged and the matching pattern's source file is a tracked in-repo
`.gitignore`, sync MUST apply the fix in the chore branch; when the
source is outside the repository (a global excludesfile or
`.git/info/exclude`) or absent from the worktree, sync MUST skip the
fix and the plan MUST surface a warning naming the external source
and directing the user to fix it manually — the sweep MUST NOT
error. When artifact paths are ignored by other patterns, the plan
MUST surface a warning for the human gate.

#### Scenario: Bare-dewey pattern flagged and fixed

- **GIVEN** a repository whose `.gitignore` contains an unanchored
  `dewey` line that ignores `.uf/dewey/learnings/`
- **WHEN** the plan runs, and sync runs on the approved plan
- **THEN** the plan flags the bounded fix block, and the chore branch's
  commit contains the `.gitignore` change anchoring `dewey` to `/dewey`
  plus the runtime-state re-ignore lines, alongside the artifact files

#### Scenario: Already fixed upstream

- **GIVEN** a repository whose `.gitignore` anchors the pattern as
  `/dewey` (or does not ignore artifact paths at all)
- **WHEN** the plan runs
- **THEN** no gitignore fix is flagged and the chore commit contains
  only artifact files

#### Scenario: Ignore source outside the repository

- **GIVEN** artifact paths ignored by a bare `dewey` pattern whose
  source is the user's global git excludesfile
- **WHEN** the plan runs, and sync runs on the approved plan
- **THEN** no fix is applied in the chore branch, the plan surfaces a
  warning naming the external source file, and the sweep completes
  without error

### Requirement: Temp Worktree Materialization

Sync MUST materialize an approved plan in a temporary git worktree
created under the OS temp directory — outside the vault and outside
any configured disk source path — checked out from the fetched remote
base, on a branch named `chore/dewey-artifacts-YYYYMMDD` (date
resolved at sync time).

#### Scenario: Worktree lives outside the vault

- **GIVEN** sync running in a repository that is also a configured
  Dewey disk source
- **WHEN** the temp worktree is created
- **THEN** the worktree path is under the OS temp directory, outside
  the repository and outside every configured source path

### Requirement: Exact Publish Set

Sync MUST write exactly the planned files, MUST verify that the staged
set equals the planned set (failing closed on mismatch), and MUST
commit with a tool-rendered Conventional Commit message. The
resulting commit MUST contain only artifact files plus the optional
bounded gitignore fix block. Push and PR creation MUST be left to the
calling layer.

#### Scenario: Plan-to-worktree round trip

- **GIVEN** an approved plan enumerating artifacts from all four
  stranded states
- **WHEN** sync runs
- **THEN** every planned artifact is readable in the temp worktree via
  `git show` on the chore branch with content matching its planned
  sha256

#### Scenario: Commit contains exactly the planned set

- **GIVEN** a completed sync
- **WHEN** the chore branch commit is inspected
- **THEN** the committed file set equals the planned artifact paths
  plus, when flagged, the bounded `.gitignore` fix block — and nothing else

### Requirement: Validated Plan Consumption

Before creating any worktree, branch, or file, sync MUST structurally
validate the plan file: the schema version MUST be one sync supports,
`repo.base_sha` MUST be a 40-character hexadecimal sha, every
`repo.stash_refs` value MUST be a 40-character hexadecimal sha, every
`sources[]` label MUST match the documented vocabulary
(`working-tree`, `stash:stash@{N}`, `stash-untracked:stash@{N}`,
`commit:<40-hex>`), every sha256 field — artifact content, conflict
variant, resolution target — MUST be a 64-character hexadecimal
digest, and every
artifact, conflict, and blocked path MUST be a clean
repository-relative path — not absolute, free of `..` traversal
components, free of control characters, colons, and
leading-hyphen components, and confined to the sanctioned artifact
directories. The
sanctioned set is re-derived by sync at execution time (the three
default directories under the vault root and at the repository root,
plus store paths re-resolved from `.uf/dewey/knowledge-stores.yaml`)
and is the confinement authority; the roots recorded in the plan are
informational, cross-checked against the re-derived set, and any
mismatch MUST fail closed. A plan recording a `gitignore.fix` object
MUST carry the literal file value `.gitignore`; any other value MUST
fail closed. Any violation MUST fail closed before any filesystem
write or git operation.

#### Scenario: Tampered plan with traversal path fails closed

- **GIVEN** a plan file whose artifact path was edited to
  `../outside-repo.md`
- **WHEN** `dewey artifacts sync --plan <file>` runs
- **THEN** sync exits non-zero before creating any worktree, branch,
  or file, with an error identifying the invalid path

#### Scenario: Absolute path fails closed

- **GIVEN** a plan file carrying an artifact path that is absolute
- **WHEN** sync runs with that plan
- **THEN** sync exits non-zero before creating any worktree, branch,
  or file

#### Scenario: Tampered plan widening the sanctioned set fails closed

- **GIVEN** a plan file edited to record a store path of `.`
- **WHEN** `dewey artifacts sync --plan <file>` runs
- **THEN** sync exits non-zero before creating any worktree, branch,
  or file: the re-derived sanctioned set does not contain `.` and the
  cross-check against the plan-recorded roots fails

#### Scenario: Forged trailer via --approved-by fails closed

- **GIVEN** sync invoked with an `--approved-by` value containing a
  newline or control character
- **WHEN** `dewey artifacts sync --plan <file> --approve --approved-by <value>` runs
- **THEN** sync exits non-zero before creating any worktree, branch,
  or file — the approver value is charset-validated because a newline
  could forge additional commit trailers such as a second
  `Plan-sha256:`

### Requirement: Sync Approval Seam

Sync MUST refuse to execute without an explicit `--approve` flag —
its absence MUST exit non-zero before any worktree, branch, or file
is created, with an error directing the caller to the human gate.
When approved, sync MUST record the approver identity (from
`--approved-by`, falling back to `git config user.name`; the
`--approved-by` value MUST be charset-validated — printable
characters only, no newlines or control characters) as an
`Approved-by:` trailer, and MUST record the consumed plan file's
sha256 as a `Plan-sha256:` trailer, in the chore commit message and
in the sync JSON output — binding the audit trail to the exact plan
content executed.

#### Scenario: Sync without approval refuses

- **GIVEN** a valid plan file
- **WHEN** `dewey artifacts sync --plan <file>` runs without
  `--approve`
- **THEN** sync exits non-zero without creating any branch, commit,
  or worktree, directing the caller to the human gate

#### Scenario: Approval is auditable

- **GIVEN** sync run with `--approve --approved-by alice`
- **WHEN** the chore commit is inspected
- **THEN** the commit message carries an `Approved-by: alice` trailer
  and the sync JSON reports the approver

### Requirement: Stale Plan Re-verification

Sync MUST re-verify content integrity for every source, not only the
working tree. Before writing, working-tree-sourced files MUST be
re-checked as regular files whose sha256 matches the plan (a symlink
swap or edit fails closed). Stash-sourced content MUST be fetched by
the recorded stash commit sha — never the mutable `stash@{N}` label,
which renumbers when stashes are pushed or dropped — and a recorded
sha missing from the object store (stash dropped and pruned) MUST
fail closed directing the user to re-run the plan. After
materializing each planned file in the worktree, sync MUST re-compute
its sha256 and MUST fail closed on any mismatch with the plan, for
every source. Before writing any planned file, sync MUST scan each
planned destination path and its ancestor directories within the
checked-out worktree for symbolic links — the remote base tree can
carry them, and a write through a symlinked destination escapes the
worktree — and MUST fail closed naming the path.

#### Scenario: Stale plan fails closed

- **GIVEN** a plan whose working-tree artifact content was modified
  after the plan was generated
- **WHEN** sync runs with that plan
- **THEN** sync exits non-zero without committing, directing the user
  to re-run the plan

#### Scenario: Stash renumbered between plan and sync

- **GIVEN** a plan recording a stash artifact by resolved commit sha,
  and a new stash pushed after the plan was generated (renumbering
  `stash@{N}` labels)
- **WHEN** sync runs with that plan
- **THEN** sync publishes the planned content by fetching it via the
  recorded sha, and the result matches the plan's sha256

#### Scenario: Recorded stash sha missing fails closed

- **GIVEN** a plan recording a stash artifact whose commit sha is no
  longer in the object store
- **WHEN** sync runs with that plan
- **THEN** sync exits non-zero without committing, directing the user
  to re-run the plan

#### Scenario: Symlinked destination in the base tree fails closed

- **GIVEN** a remote base tree containing a symbolic link at an
  artifact destination path (or an ancestor of one)
- **WHEN** sync runs on an approved plan writing that path
- **THEN** sync exits non-zero before writing, naming the symlinked
  destination

### Requirement: DB-Free Operation

The sweep MUST NOT open the SQLite store, MUST NOT acquire
`.uf/dewey/dewey.lock`, and MUST operate exclusively through git
subprocesses and read-only filesystem access. The full flow MUST
succeed while another process holds the exclusive flock on
`.uf/dewey/dewey.lock`.

#### Scenario: Full flow with lock held

- **GIVEN** a fixture repository where a test process holds an
  exclusive flock on `.uf/dewey/dewey.lock`
- **WHEN** plan and sync run
- **THEN** both succeed without attempting to open the lock or the
  store

### Requirement: Idempotent Nothing-To-Publish

When no new artifacts exist beyond the remote base tree, the flow
MUST produce an explicit nothing-to-publish result and MUST NOT
create a branch, commit, worktree, or PR.

#### Scenario: Re-run over an unchanged repository

- **GIVEN** a repository whose artifacts were all previously published
  (present with identical content on the remote base branch)
- **WHEN** `/dewey-publish` runs again
- **THEN** the flow reports nothing to publish and creates no branch,
  commit, worktree, or PR

### Requirement: Command-Layer Orchestration and Human Gate

The `/dewey-publish` command MUST gate sync behind explicit human
approval — sync MUST NOT run without it, and the approval MUST flow
through sync's `--approve` and `--approved-by` parameters — and MUST
pass the gate's judgment products (approval, path-keyed collision
resolutions) as parameters. Before requesting approval, the gate MUST
present the resolved publish destination — the publish remote name,
its fetch URL, and the target repository and branch the PR will be
opened against — so the human approves a named destination, not an
abstract one. Before requesting approval, the gate MUST present the
plan's tool-rendered human summary — carried in the plan JSON as
`rendered_summary` and relayed verbatim — the itemized, tag-grouped
artifact listing (path, kind, sha256) plus the conflict, blocked, and
gitignore findings — not aggregates alone, so the human approves a
named set of content. Rendered paths, identities, tags, and store
names are control-character-stripped and markdown-escaped by the
core renderers (untrusted-data framing). For each conflict, the plan core MUST render a
bounded, deterministic unified diff between the clean variants —
capped at 100 lines, with each line truncated at 4 KiB and the
total diff at 16 KiB, each bound carrying an explicit elision or
truncation marker — as part of the plan's
tool-rendered summary, and the gate MUST relay it verbatim — the
diff is untrusted content rendered for the resolution choice, never
interpreted, and bounded so it cannot re-deliver unbounded payload.
The PR body MUST relay verbatim the
tool-rendered, tag-grouped artifact listing produced by the sync core
and pinned by golden test; judgment framing MAY surround the relayed
listing. The command MUST resolve the publish remote (`upstream` when
configured, else `origin`), push the chore branch to the push remote
(`origin` in fork layouts, else the publish remote), and target the
publish remote's default branch in the PR. It MUST prefer the uf
plugin tools (`git_fetch`, `git_push`, `uf_finale_create_pr`) with
raw `git` / `gh` fallbacks, and MUST report the merge → pull →
server-restart pickup path.

#### Scenario: Gate blocks unapproved sync

- **GIVEN** a presented plan awaiting gate approval
- **WHEN** the user declines or does not respond
- **THEN** sync is never invoked with `--approve`, push and PR
  creation do not run, and no repository state changes

#### Scenario: Gate presents the publish destination

- **GIVEN** a fork-layout repository with `upstream` configured
- **WHEN** the gate presents the plan for approval
- **THEN** the presentation names the publish remote (`upstream`), its
  fetch URL, and the target repository and default branch, before any
  approval is requested

#### Scenario: PR body groups artifacts by tag

- **GIVEN** an approved plan containing learnings with tags `auth`,
  `deployment`, and a compiled article for `auth`
- **WHEN** the sync result is produced and the PR is created
- **THEN** the sync core's tool-rendered PR-body fragment groups the
  artifact identities under their tags, and the PR body relays that
  fragment verbatim

#### Scenario: Pickup path reported

- **GIVEN** a successfully created chore PR
- **WHEN** the command completes
- **THEN** the user is told that merged artifacts are picked up after
  merge, pull, and Dewey server restart (learnings re-ingestion runs
  at serve startup)

### Requirement: Additive CLI Surface

The `dewey artifacts` command group MUST be additive: all existing
`dewey` commands, flags, and output formats MUST remain unchanged,
and the sweep MUST introduce no new runtime dependencies.

#### Scenario: Existing commands unchanged

- **GIVEN** the implemented command group
- **WHEN** the pre-existing CLI test suite runs
- **THEN** all existing command behavior tests pass unmodified

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
