---
description: "Resolve a merge conflict on a PR (/uf.rebase-pr <PR_NUMBER>): rebase the PR's head branch onto its base and force-push with lease so it can be merged with a rebase once approved"
---
<!-- scaffolded by uf vdev -->

# Resolve Branch Conflict

You are a token-efficient developer. The user provides a GitHub pull
request number. You resolve the merge conflict on the PR's head
branch by rebasing it onto its base branch, so that once the PR is
approved it can be merged with a rebase.

<protect>

> **Session-resume guard**: If this session was resumed
> from compressed context, re-read this entire template
> before continuing. Do NOT infer step completion from
> compressed summaries. When in doubt, re-read — false
> re-reads are harmless; skipping steps due to stale
> context causes skipped validation, wrong-remote pushes,
> or corrupted conflict resolutions.

## Arguments

- **PR number** (required): The GitHub pull request number to fix
  (e.g., `42`).

**Argument parsing** (before any tool calls): Check the user's
message for a PR number argument. Validate that it matches
`^[1-9][0-9]*$` (positive integer, no leading zeros). If the
argument is missing, invalid, zero, negative, or contains
non-numeric characters: **STOP** with error:

> "Invalid PR number. Must be a positive integer."

No `gh` CLI commands are permitted until the argument passes
validation. Set `PR_NUMBER` to the validated value. All subsequent
steps use `<PR_NUMBER>`.

---

## Phase 1: Prerequisites

Verify the `gh` CLI is available and authenticated:

```bash
which gh
```

If not found: **STOP** with error:
> "GitHub CLI (gh) is not installed. Install from https://cli.github.com/"

```bash
gh auth status
```

If not authenticated: **STOP** with error:
> "GitHub CLI not authenticated. Run `gh auth login` to authenticate."

Detect the current repository:

```bash
gh repo view --json nameWithOwner --jq '.nameWithOwner'
```

Record `{owner}/{repo}` for all subsequent API calls. Repository
identifiers MUST NOT be hardcoded.

## Phase 2: Fetch the PR

```bash
gh pr view <PR_NUMBER> --json number,title,state,headRefName,baseRefName,mergeable,mergeStateStatus,baseRepository,headRepository
```

**If the PR does not exist**: **STOP** with the `gh` error output.

**If the PR is CLOSED or MERGED**: **STOP** with error:
> "PR #<PR_NUMBER> is <state>. Nothing to resolve."

Record:
- `HEAD_BRANCH` = `headRefName` (the branch that needs the conflict resolved)
- `BASE_BRANCH` = `baseRefName` (the branch to rebase onto)
- `BASE_REPO` = `<baseRepository.owner.login>/<baseRepository.name>` (the repository the PR targets)
- `HEAD_REPO` = `<headRepository.owner.login>/<headRepository.name>` (the repository holding the head branch; may differ from `BASE_REPO` for fork PRs)

**If the PR has no conflicts** (`mergeable` is `MERGEABLE` and `mergeStateStatus` is `CLEAN`): report "PR #<PR_NUMBER> has no conflicts to resolve." and **STOP**.

## Phase 3: Checkout and Rebase

Check out the PR's head branch (this also configures the correct
remotes):

```bash
gh pr checkout <PR_NUMBER>
```

If `gh pr checkout` fails (for example, the head branch was deleted),
fetch the head ref directly from the PR's head repository and check
it out:

```bash
git fetch https://github.com/<HEAD_REPO>.git <HEAD_BRANCH>
git checkout -B <HEAD_BRANCH> FETCH_HEAD
```

Resolve which local remote points to the base repository. `origin` is
NOT guaranteed to be `BASE_REPO` — for a fork PR, `origin` often
points to the fork (`HEAD_REPO`) instead. Inspect the configured
remotes:

```bash
git remote -v
```

Match `BASE_REPO` (recorded in Phase 2) to a remote by its fetch/push
URL. `gh pr checkout` typically names the base repository remote
`upstream`. If no existing remote matches `BASE_REPO`, add one (using
a name that does not collide with existing remotes):

```bash
git remote add <BASE_REMOTE> https://github.com/<BASE_REPO>.git
```

Likewise resolve `HEAD_REMOTE`: the remote whose URL matches
`HEAD_REPO`. For a fork PR this is usually `origin`; for a same-repo
PR it is the same remote as `<BASE_REMOTE>`. If none matches
`HEAD_REPO`, add one:

```bash
git remote add <HEAD_REMOTE> https://github.com/<HEAD_REPO>.git
```

Fetch the latest base branch from the base remote and rebase the head
branch onto it — `<BASE_REMOTE>` is the remote matched to (or added
for) `BASE_REPO`, and `<BASE_BRANCH>` is the PR's `baseRefName`.
Delegate both operations to the git plugin tools:

1. Call `git_fetch` with remote `<BASE_REMOTE>`.
2. Call `git_rebase` with ref `<BASE_REMOTE>/<BASE_BRANCH>`.

`git_rebase` returns conflict information instead of failing when the
rebase stops on conflicts — proceed to Phase 4 in that case.

## Phase 4: Resolve Conflicts

If the rebase stops with conflicts:

1. Run `git status` to list unmerged paths.
2. For each conflicted file, open it and resolve the conflict markers
   (`<<<<<<<`, `=======`, `>>>>>>>`). Preserve the PR author's
   intended code; do not make unrelated changes.
3. Stage the resolved file with `git add <file>`.
4. Continue the rebase with `git rebase --continue`.
5. Repeat until the rebase completes.

If a conflict cannot be resolved with confidence, do not guess.
**STOP**, explain what is blocking, and offer the user the choice to
abort with `git rebase --abort`.

If you get stuck mid-rebase (or the user chooses to abort), back out
cleanly with the `git_rebase_abort` plugin tool — it aborts the
in-progress rebase and restores the pre-rebase branch state. Then
report the situation rather than forcing a bad resolution.

## Phase 5: Push the Rebased Branch

The rebase rewrites history, so the head branch must be force-pushed
to the PR's head repository — under lease protection, with the push
target verified first. The `git_push_force_with_lease` plugin tool
takes no refspec: it pushes the current branch to its configured
upstream, which is not guaranteed to be the head remote (the Phase 3
checkout fallback leaves no upstream configured, and a pre-existing
local branch may carry stale tracking).

Verify the checked-out branch's upstream:

```bash
git rev-parse --abbrev-ref @{u}
```

- If the upstream is exactly `<HEAD_REMOTE>/<HEAD_BRANCH>`: call the
  `git_push_force_with_lease` plugin tool — it pushes the current
  branch to the verified upstream under lease protection.
- If the command reports no upstream, or any other remote or branch:
  fall back to the explicit refspec command, which pins the push
  target regardless of tracking state:

```bash
git push --force-with-lease <HEAD_REMOTE> <HEAD_BRANCH>
```

MUST use `--force-with-lease`, never `--force`. Push only to
`HEAD_REMOTE` (the remote resolved for `HEAD_REPO`) and the PR's head
branch (`HEAD_BRANCH`). MUST NOT push to `main`, `master`, or any
protected branch, and MUST NOT push to `BASE_REMOTE`.

## Phase 6: Verify

Confirm the conflict is resolved and the PR is mergeable:

```bash
gh pr view <PR_NUMBER> --json mergeable,mergeStateStatus
```

Verify `mergeable` is `MERGEABLE` and the PR is no longer reporting
a conflict. Report the result to the user.

Do NOT merge, approve, or close the PR. This command only makes the
PR ready to merge with a rebase once it is approved.

## Guardrails

1. **No merge, no approve**: MUST NOT merge, approve, or close the
   PR. Only resolve the conflict.
2. **Never push to the base branch**: Operate on the PR's head
   branch only. Never push to `main`, `master`, or protected
   branches.
3. **Resolve remotes, never assume `origin`**: Determine
   `BASE_REMOTE` and `HEAD_REMOTE` from `git remote -v` matched
   against the PR's `baseRepository`/`headRepository`. `origin` may
   point to a fork, not the base repo.
4. **`--force-with-lease`, never `--force`**: History is rewritten
   by the rebase; force-with-lease protects against overwriting
   others' pushes.
5. **Conflicts only**: Only touch files the rebase flagged as
   conflicted. Do not make unrelated edits.
6. **No guessing**: When a conflict resolution is ambiguous, stop
   and ask rather than fabricate a resolution.
7. **Clean backout**: Prefer `git_rebase_abort` (the plugin tool for
   `git rebase --abort`) over forcing through a broken rebase.
8. **gh CLI verification before API calls**: `which gh` and
   `gh auth status` MUST succeed before any GitHub API call.
9. **Deterministic**: The same PR rebased onto the same base MUST
   produce the same result.

</protect>
