---
description: >
  Finalize a branch: commit, push, create PR, watch CI
  checks, and return to main. The PR stays open for
  review. One command to wrap up any feature or OpenSpec
  branch.
---
<!-- scaffolded by uf v0.17.0 -->

# Command: /uf.finale

## User Input

```text
$ARGUMENTS
```

## Description

Automate the end-of-branch workflow. Stages all changes,
generates a conventional commit message, pushes, creates
a PR, watches CI checks, and returns to `main`. The PR
stays open for human review. Works with both Speckit
(`NNN-*` or `NNN-*` legacy) and OpenSpec
(`opsx/*`) branches.

## Usage

```
/uf.finale                    # auto-detect everything
/uf.finale fix the typo       # use as commit message hint
```

## Instructions

<protect>

**Session-resume guard**: If this session has been
   resumed from compressed context, or if you cannot
   verify that the human explicitly confirmed a gate
   in the current uncompressed conversation history, you
   MUST re-read this entire template, recover state from
   the execution checklist below, and re-confirm any
   pending gates via the **question tool** before
   proceeding. Do NOT rely on gate confirmations recorded
   in compressed context. Do NOT infer step completion
   from compressed summaries. When in doubt, re-confirm
   — false re-confirmation is harmless; executing without
   consent is a violation.

**Execution checklist**: The agent MUST maintain this
checklist using the **Edit tool** after completing each
step. This checklist survives context compression and
serves as the source of truth for workflow state.

```
BRANCH=<set when known>
COMMIT=<set when known>
PR_NUMBER=<set when known>
PR_URL=<set when known>
CONFLICT_OPTION=<set when known>

[ ] Step 1 — Branch Safety Gate
[ ] Step 2 — Check for Changes to Commit
[ ] Step 3 — Generate and Confirm Commit Message
[ ] Step 4 — Push to Remote
[ ] Step 5 — Create or Find PR
[ ] Step 6 — Watch CI Checks
[ ] Step 7 — Return to Main
[ ] Step 8 — Summary
```

After completing each step, use the Edit tool to:
1. Mark the step `[x]` in this checklist
2. Set any state variables that became known
   (e.g., `BRANCH=opsx/my-feature` after Step 1)

If resuming from compressed context, re-read this
checklist to determine which steps are complete and
what state is available.

### 1. Branch Safety Gate

Get the current branch:

```bash
git rev-parse --abbrev-ref HEAD
```

- If on `main`: **STOP** with error:
  > "Cannot run /uf.finale on main. Switch to a feature
   > branch (e.g., `opsx/*` or `NNN-*`) first."
- Otherwise: proceed. Note the branch name for the
  summary.

### 2. Check for Changes to Commit

Run `git status --short` to inspect the working tree.

**If no changes exist** (clean working tree):
- Check if there are unpushed commits:
  `git log origin/<branch>..HEAD --oneline 2>/dev/null`
- If unpushed commits exist: skip to step 4 (push).
- If no unpushed commits: check if a PR exists (step 5).
  If a PR exists, skip to step 6 (watch checks). If no
  PR and no changes, report "Nothing to finalize" and
  stop.

**If changes exist**:
- **Secrets check**: Scan unstaged/untracked files for
  names that likely contain secrets:
  - `.env`, `.env.*`
  - `credentials.json`, `secrets.json`, `*.key`, `*.pem`
  - Any file matching common secret patterns

  If potential secret files are found:

  >>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

  > "Warning: the following files may contain secrets
  > and should not be committed:
  >
  > - .env.local
  > - credentials.json
  >
  > Proceed with staging all files? These files will be
  > included in the commit."

  Use the **question tool** with options
  `["Yes -- stage all files and continue", "No -- stop here"]`.

  - If the user selects **"Yes -- stage all files and
    continue"**: proceed to `git add .`.
  - If the user selects **"No -- stop here"**: **STOP**.
    Do not run `git add .`. Do not proceed to Step 3 or
    any subsequent steps. Report that the user declined
    and let them handle the secret files manually.

  >>> END MANDATORY GATE <<<

- **Stage all changes**: `git add .`

**Checkpoint**: Update the execution checklist (mark
Step 2 `[x]`) before proceeding.

### 3. Generate and Confirm Commit Message

a. Analyze the staged changes:

```bash
git diff --cached --stat
git diff --cached
git log --oneline -5
```

b. Generate a conventional commit message:
- Determine the type: `feat:`, `fix:`, `docs:`,
  `chore:`, `refactor:`, `test:`
- Write a concise summary (1 line) focusing on the
  "why" not the "what"
- Add a body with bullet points if multiple logical
  changes are staged
- If `$ARGUMENTS` is not empty, use it as a hint or
  directly as the summary if it's already well-formed
- Append AI attribution after the commit body,
  separated by a blank line:
  1. A git trailer: `Assisted-by: <model>`
  2. A human-readable footer:
     `Generated with AI assistance (<model>)`

  Where `<model>` is the model family name you are
  currently running as. To resolve the model name:
  (1) read your model identifier from the system
  prompt (e.g., "You are powered by the model named
  X") or runtime environment; (2) remove everything
  before and including the last `/` character;
  (3) remove everything after and including the first
  `@` character;   (4) remove any trailing date suffix
  matching `-YYYYMMDD` (a hyphen followed by exactly
  8 digits); (5) repeatedly remove any trailing
  version segment matching `-N` (a hyphen followed by
  a single digit at the end) until no more remain;
  (6) validate the result contains only
  `[a-zA-Z0-9._-]` characters. If the result is empty,
  contains invalid characters, or cannot be determined,
  use the literal string `unknown-model` and warn the
  user (e.g., "Could not determine AI model name —
  using 'unknown-model' in attribution").

  Examples:
  - `google-vertex-anthropic/claude-sonnet-4-20250514@default` → `claude-sonnet`
  - `claude-opus-4-20250514` → `claude-opus`
  - `gpt-4o` → `gpt-4o`
  - `gemini-2.5-pro` → `gemini-2.5-pro`

>>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

**Session-resume guard**: If this session was resumed
from compressed context, or if you cannot verify that
the human explicitly confirmed the commit message in
the current uncompressed conversation history, you
MUST re-present the proposed commit message below and
obtain fresh confirmation via the **question tool**
before committing. Do NOT
rely on confirmation recorded in compressed context.
When in doubt, re-confirm — false re-confirmation is
harmless; committing without consent is a violation.

c. Show the proposed message to the user:

> **Proposed commit message:**
>
> ```
> feat: add /uf.finale slash command for branch finalization
>
> - Create finale.md command definition
> - Add scaffold asset and update file count test
>
> Assisted-by: claude-sonnet
> Generated with AI assistance (claude-sonnet)
> ```
>
> Approve, edit, or provide your own?

The user MAY edit or remove the attribution during
the approval step. If the user removes it, use their
edited message without re-adding attribution.

Use the **question tool** with options
`["Approve and commit", "Edit commit message",
"Provide my own message"]`.

- **"Approve and commit"**: Proceed with the displayed
  commit message.
- **"Edit commit message"**: Let the user modify the
  message, then re-confirm.
- **"Provide my own message"**: Accept a full
  replacement message from the user.

**CRITICAL RULE**: NEVER commit without explicit human
confirmation via the **question tool**.

>>> END MANDATORY GATE <<<

d. Commit with the approved message.

**Checkpoint**: Update the execution checklist — set
`COMMIT=<hash>` and mark Step 3 `[x]` before proceeding.

### 4. Push to Remote

```bash
# Check if upstream is set
git rev-parse --abbrev-ref @{upstream} 2>/dev/null
```

Fetch the remote branch state to detect divergence:

```bash
git fetch origin <branch>
git status
```

>>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

**Session-resume guard**: If this session was resumed
from compressed context, or if you cannot verify that
the human explicitly confirmed the push in the current
uncompressed conversation history, you MUST re-present
the push target and branch status below and obtain
fresh confirmation via the **question tool** before
pushing. Do NOT rely on confirmation recorded in
compressed context. When in doubt, re-confirm — false
re-confirmation is harmless; pushing without consent
is a violation.

**If branch has diverged** (the remote has commits not
in the local branch): warn the user about the divergence
before presenting the confirmation gate.

Use the **question tool** with options
`["Push to remote", "Abort -- keep commits local"]`.

- If the user selects **"Push to remote"**:
  - If no upstream: `git push -u origin <branch>`
  - If upstream exists: `git push`
  - If push fails: report error and **STOP**. Do not
    proceed to Step 5 or any subsequent steps.
- If the user selects **"Abort -- keep commits local"**:
  report that local commits are preserved and **STOP**.
  Do not proceed to Step 5 or any subsequent steps.

**CRITICAL RULE**: NEVER push to the remote without
explicit human confirmation via the **question tool**.

>>> END MANDATORY GATE <<<

**Checkpoint**: Update the execution checklist (mark
Step 4 `[x]`) before proceeding.

### 5. Create or Find PR

Check if a PR already exists:

```bash
gh pr view --json number,url 2>/dev/null
```

- **If PR exists**: use its number and URL. Skip
  creation.
- **If no PR**: create one:

  a. **Fork detection**: Check if this repo is a fork:
  ```bash
  gh repo view --json isFork,parent
  ```
  If `isFork` is `true`, ask the user:

  > "This repo is a fork of `<parent.owner>/<parent.name>`.
  > Where should the PR target?
  >
  > 1. Upstream (`<parent.owner>/<parent.name>` main)
  > 2. Fork (`<origin.owner>/<origin.name>` main)"

  Use the answer to set `--repo` on `gh pr create`.
  If not a fork, proceed without asking.

  b. Generate PR title from commit history:
  ```bash
  git log main..HEAD --oneline
  ```
  Use the most descriptive commit message as the title,
  or synthesize from multiple commits.

  c. **PR template detection**: Before generating the
  PR body, check for a PR template:

  ```bash
  ls .github/PULL_REQUEST_TEMPLATE.md \
     .github/pull_request_template.md 2>/dev/null
  ```

  - **If a template is found**: read the template and
    use its `##` heading structure as the skeleton for
    the PR body. Map generated content to template
    sections using case-insensitive substring matching:

    | Generated Section | Matches Template Headings |
    |---|---|
    | Summary | Description, Summary, Overview, What |
    | How to Test | Testing, Test, Test Plan, Verification |
    | How to Demo | Demo, How to Demo |
    | Key Files Changed | Files Changed, Changes |

    Template sections that do not match generated
    content are preserved as-is for the user to fill
    in during the approval step. Generated content for
    sections without a template match is appended at
    the end.

    If the template contains no `##` headings (empty,
    malformed, or non-Markdown), fall back to the
    default structured format and warn the user that
    the template could not be parsed.

  - **If no template is found**: use the default
    structured format below.

  d. Generate PR body with structured sections:

  Analyze the branch commits, diff, and available spec
  artifacts to produce a PR body with these sections:

  - `## Summary` — what was done. Summarize the logical
    changes from commit history. Focus on the "why" and
    user-visible impact, not implementation details.
  - `## How to Test` — verification steps for reviewers.
    If on an `opsx/*` branch, read
    `openspec/changes/*/specs/*.md` for acceptance
    scenarios and translate them into concrete
    verification commands. If on a `NNN-*` or
    `NNN-*` (legacy) branch,
    check for `quickstart.md` in the feature directory.
    Otherwise, synthesize test steps from the diff.
  - `## How to Demo` — walkthrough for demonstrating
    the change. Describe what to do and what to observe.
    For trivial changes (e.g., typo fixes), use a brief
    note like "Trivial fix — no demo required."
  - `## Key Files Changed` — file listing from
    `git diff --stat main..HEAD` with brief descriptions
    of what changed in each file.

  Each section MUST contain substantive content. For
  trivial changes with insufficient source material,
  sections SHOULD contain a brief explanatory note
  rather than fabricated content.

  Append the attribution footer as the last line of
  the PR body:

  ```
  _This PR was generated by /uf.finale (AI-assisted)._
  ```

  e. **Review-council findings**: Check the conversation
  context for prior `/uf.review-council` output. If
  unresolved findings exist (findings that were
  acknowledged but not fixed during the session), add a
  `## Known Issues` section to the PR body between
  `## Key Files Changed` and the attribution footer.

  Each finding MUST include its severity and a brief
  description. Example:

  ```
  ## Known Issues

  The following findings from the review council were
  acknowledged but not resolved:

  - **LOW**: Unused variable in config parser
  - **MEDIUM**: Missing error context in HTTP handler
  ```

  If no `/uf.review-council` was run in the session, or no
  unresolved findings exist, omit this section entirely.

  Note: findings come from session context and may be
  stale if code changes were made after the review. The
  user SHOULD verify accuracy during the approval step.

  f. Show the proposed PR content to the user:

  **Session-resume guard**: If resuming from compressed
  context, re-present the PR title and body and obtain
  fresh confirmation before calling `gh pr create`.

  >>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

  > **Proposed PR:**
  >
  > **Title:** `<title>`
  >
  > **Body:**
  > ```
  > <body>
  > ```

  Use the **question tool** with options
  `["Approve — create PR", "Edit title or body",
  "Provide my own title and body", "Abort — do not
  create PR"]`.

  >>> END MANDATORY GATE <<<

  Use the approved (or edited/replaced) title and body
  for creation.

  g. Create:

  Write the approved PR body to a temporary file to
  avoid shell injection from AI-generated content:

  ```bash
  BODY_FILE=$(mktemp)
  chmod 600 "$BODY_FILE"
  ```

  Write the approved PR body content to `$BODY_FILE`.

  ```bash
  # If targeting upstream fork parent:
  gh pr create --repo <parent> --title "<title>" \
    --body-file "$BODY_FILE"
  # Otherwise (not a fork, or user chose fork target):
  gh pr create --title "<title>" --body-file "$BODY_FILE"
  ```

  Clean up the temp file in ALL exit paths (success,
  failure, user abort):

  ```bash
  rm -f "$BODY_FILE"
  ```

  If `mktemp` fails, report the error and **STOP**.
  Do NOT fall back to inline `--body` interpolation.

  h. Report the PR URL.

  **Checkpoint**: Update the execution checklist — set
  `PR_NUMBER=<number>`, `PR_URL=<url>`, and mark
  Step 5 `[x]` before proceeding.

### 6. Watch CI Checks

```bash
gh pr checks <number> --watch
```

- **If checks pass**: proceed to step 7.
- **If checks fail**: report the failure details and
  **STOP**:

  >>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

  > "CI checks failed on PR #<number>:
  >
  > - Build & Test: FAIL (45s)
  >   https://github.com/.../runs/...
  >
  > Options:
  > 1. Investigate the failure
  > 2. Re-run the checks
  > 3. Stop here and fix manually"

  Use the **question tool** to ask the user how
  to proceed.

  >>> END MANDATORY GATE <<<

- **If no checks are reported** ("no checks reported"
  or equivalent empty result): do NOT conclude that no
  CI is configured. Investigate using the mergeability
  gate below.

#### 6a. Mergeability Gate

When `gh pr checks` returns "no checks reported,"
query the PR mergeability status:

```bash
gh pr view <number> --json mergeable,mergeStateStatus
```

If the `gh pr view` command itself fails (non-zero
exit code), report the error and **STOP**:

> "Could not query PR mergeability. Check network
> connectivity and `gh` CLI authentication."

Otherwise, interpret the `mergeable` field:

- **`CONFLICTING`**: The PR has a merge conflict that
  is blocking CI checks from running. Proceed to
  step 6b (Conflict Recovery).

- **`UNKNOWN`**: GitHub is still computing the merge
  state. Warn the user:

  > "GitHub is computing mergeability — retrying in
  > 10 seconds..."

  Wait approximately 10 seconds, then re-run:

  ```bash
  gh pr view <number> --json mergeable,mergeStateStatus
  ```

  If still `UNKNOWN` after one retry, warn and
  proceed to step 6c (Workflow Cross-Reference) to
  gather more information before concluding.

- **`MERGEABLE`**: No merge conflict. Proceed to
  step 6c (Workflow Cross-Reference).

#### 6b. Conflict Recovery

Determine the target remote and branch for conflict
resolution. Use the PR's base ref (`baseRefName`
from step 5) and the target remote:

- If the PR targets an upstream fork parent (step 5a):
  `<target-remote>` is the upstream remote name
  (e.g., `upstream`)
- Otherwise: `<target-remote>` is `origin`

When `mergeable` is `CONFLICTING`, report the
conflict to the user and present recovery options:

>>> MANDATORY GATE: HUMAN CONFIRMATION REQUIRED <<<

> "PR #<number> has a merge conflict with
> `<target-remote>/<base-branch>`. CI checks cannot
> run until the conflict is resolved.
>
> Options:
> 1. Merge target branch (no force push needed)
> 2. Rebase onto target branch (requires force push)
> 3. Stop and resolve manually
> 4. Continue anyway (CI will not run)
> 5. Spawn sub-agent to resolve conflicts
>    (AI-assisted)"

Use the **question tool** to ask the user
which option to take. After the user selects an option,
update the execution checklist: set
`CONFLICT_OPTION=<N>` (where N is the selected option
number).

>>> END MANDATORY GATE <<<

**Option 1 — Merge target branch**:

This option does not require force push and works in
restricted environments with branch protection rules.

Show the commands and ask for explicit confirmation
before executing:

> "I will run the following commands:
>
> ```
> git fetch <target-remote> <base-branch>
> git merge <target-remote>/<base-branch>
> git push
> ```
>
> This creates a merge commit and pushes to the
> remote. Proceed?"

If the user confirms, execute:

```bash
git fetch <target-remote> <base-branch>
git merge <target-remote>/<base-branch>
```

- If the merge succeeds without conflicts:

  ```bash
  git push
  ```

  Then poll for CI checks using a bash loop to avoid
  consuming LLM tokens:

  ```bash
  while true; do
    STATUS=$(gh pr checks <number> 2>&1)
    if echo "$STATUS" | grep -qE 'pass|fail'; then
      echo "$STATUS"
      break
    fi
    sleep 10
  done
  ```

  Read the poll output and continue with normal
  step 6 check-watching behavior (checks pass →
  step 7, checks fail → report and stop).

- If the merge encounters conflicts:

  ```bash
  git merge --abort
  ```

  Report which files have conflicts and **STOP**:

  > "Merge failed — conflicts in the following
  > files:
  >
  > - <file1>
  > - <file2>
  >
  > Resolve the conflicts manually, then re-run
  > /finale."

**Option 2 — Rebase onto target branch**:

**Warning**: This option requires `--force-with-lease`
to push. Force push may be blocked in restricted work
environments with branch protection rules. If force
push is not available, use Option 1 (merge) instead.

Show the commands and ask for explicit confirmation
before executing:

> "I will run the following commands:
>
> ```
> git fetch <target-remote> <base-branch>
> git rebase <target-remote>/<base-branch>
> git push --force-with-lease
> ```
>
> **Note**: Force push is required after rebase. This
> may be blocked in restricted environments.
> Proceed?"

If the user confirms, execute:

```bash
git fetch <target-remote> <base-branch>
git rebase <target-remote>/<base-branch>
```

- If the rebase succeeds without conflicts:

  ```bash
  git push --force-with-lease
  ```

  If push fails (e.g., force push blocked by branch
  protection): report the error and suggest using
  Option 1 (merge) instead. **STOP**.

  If push succeeds, poll for CI checks using a bash
  loop (same as Option 1).

- If the rebase encounters conflicts:

  ```bash
  git rebase --abort
  ```

  Report which files have conflicts and **STOP**:

  > "Rebase failed — conflicts in the following
  > files:
  >
  > - <file1>
  > - <file2>
  >
  > Resolve the conflicts manually, then re-run
  > /finale."

**Option 3 — Stop for manual resolution**:

Report the conflict and **STOP**:

> "Merge conflict detected. Resolve it manually,
> then re-run /finale."

**Option 4 — Continue anyway**:

Proceed to step 7 (Return to Main). Set a flag so
that step 8 (Summary) includes a warning that CI
checks did not run. See step 8 for the warning
format.

**Option 5 — Spawn sub-agent to resolve conflicts**:

This option uses the merge strategy to create
conflict markers, then spawns a `cobalt-crush-dev`
sub-agent to attempt automated conflict resolution.

a. **Execute the merge to create conflict markers**:

   Show the commands and ask for explicit confirmation
   before executing:

   > "I will merge `<target-remote>/<base-branch>` to
   > create conflict markers, then spawn a sub-agent
   > to resolve them. Proceed?"

   If the user confirms, execute:

   ```bash
   git fetch <target-remote> <base-branch>
   git merge <target-remote>/<base-branch>
   ```

b. **Identify conflicting files**:

   ```bash
   git diff --name-only --diff-filter=U
   ```

   Capture the list of files with unresolved conflicts.

c. **Spawn the sub-agent**:

   Call the Task tool with `subagent_type:
   cobalt-crush-dev` and a prompt containing:

   - The list of conflicting files from step (b)
   - The target branch name
     (`<target-remote>/<base-branch>`)
   - Instructions: "Resolve the merge conflict markers
     (`<<<<<<<`, `=======`, `>>>>>>>`) in each of the
     following files. For each file: read the file,
     understand the intent of both sides of the
     conflict (the HEAD changes and the incoming
     changes), write the resolved content that
     preserves both intents, and stage the resolved
     file with `git add <file>`. Report per-file
     success or failure."
   - A directive: "Do NOT resolve the conflicts by
     simply choosing one side. Integrate both changes
     where possible. If the conflict is too complex to
     resolve confidently, report that file as
     unresolved."

   The sub-agent MUST NOT receive the full `/uf.finale`
   flow context. It receives only the information
   needed for conflict resolution.

d. **Evaluate the sub-agent result**:

   After the sub-agent returns, check for remaining
   conflict markers in all files:

   ```bash
   git diff --name-only --diff-filter=U
   ```

   - **If unresolved files remain** (sub-agent
     partially failed): report which files were
     resolved and which remain:

     > "Sub-agent resolved N of M files.
     > Unresolved: <file1>, <file2>
     >
     > Aborting merge to restore clean state."

     ```bash
     git merge --abort
     ```

     Return to the conflict recovery options menu
     (options 1-5).

   - **If no unresolved files remain** (sub-agent
     succeeded): proceed to step (e).

e. **User approval gate**:

   Show the staged diff to the user:

   ```bash
   git diff --cached
   ```

   > "The sub-agent resolved all conflicts. Review
   > the resolution diff above.
   >
   > Options:
   > 1. Approve, commit, and push
   > 2. Request edits (modify resolution manually)
   > 3. Abort (discard resolution)"

   Ask the user which option to take.

   - **If the user approves** (option 1): complete the
     merge commit (git will auto-create the merge
     commit message) and push:

     ```bash
     git commit --no-edit
     git push
     ```

     Then poll for CI checks using a bash loop (same
     as Option 1):

     ```bash
     while true; do
       STATUS=$(gh pr checks <number> 2>&1)
       if echo "$STATUS" | grep -qE 'pass|fail'; then
         echo "$STATUS"
         break
       fi
       sleep 10
     done
     ```

     Read the poll output and continue with normal
     step 6 check-watching behavior.

   - **If the user requests edits** (option 2):

     Inform the user that the conflicting files are
     staged with the sub-agent's resolution. The user
     can now edit the files manually, then stage the
     changes:

     > "Edit the resolved files as needed, then stage
     > your changes with `git add <file>`. When done,
     > tell me to continue."

     When the user signals they are done, re-show the
     staged diff (`git diff --cached`) and return to
     the approval gate (present the 3-option menu
     again).

   - **If the user aborts** (option 3):

     ```bash
     git merge --abort
     ```

     Return to the conflict recovery options menu
     (options 1-5).

#### 6c. Workflow Cross-Reference

When `mergeable` is `MERGEABLE` (or `UNKNOWN` after
retry) and no checks were reported, check whether CI
workflows are configured for the repository:

```bash
ls .github/workflows/*.yml .github/workflows/*.yaml \
  2>/dev/null
```

- **If workflow files exist**: warn the user:

  > "CI workflow files exist in `.github/workflows/`
  > but no checks were reported for PR #<number>.
  > This may indicate disabled workflows, a workflow
  > syntax error, or another configuration issue.
  >
  > Options:
  > 1. Proceed without CI checks
  > 2. Stop and investigate"

  Ask the user which option to take. If proceed,
  continue to step 7. If stop, **STOP** with the
  warning.

- **If no workflow files exist**: no CI is configured
  for this repository. Report:

  > "No CI workflows configured — proceeding without
  > checks."

  Proceed to step 7.

**Checkpoint**: Update the execution checklist (mark
Step 6 `[x]`) before proceeding.

### 7. Return to Main

Return to main so the developer can start other work:

```bash
git checkout main 2>/dev/null  # may already be on main
git pull
```

Verify:
```bash
git rev-parse --abbrev-ref HEAD
```

Should be `main`.

**Checkpoint**: Update the execution checklist (mark
Step 7 `[x]`) before proceeding.

### 8. Summary

Display a completion report:

```
## Finale Complete

**Branch:** opsx/finale-command (pushed)
**Commit:** feat: add /uf.finale slash command
**PR:** #65 — CI passed, ready for review
**Checks:** passed
**Status:** on main, up to date

Next: Request reviewers on the PR, then merge after
approval with: gh pr merge --rebase --delete-branch
```

If CI checks were skipped because the user chose
"Continue anyway" during a merge conflict (step 6b,
option 4), include a warning in the summary:

```
**Checks:** CI checks did not run due to merge conflict
```

Replace the `**Checks:** passed` line with the warning
above. This ensures the user is aware that CI
verification is incomplete.

**Checkpoint**: Update the execution checklist (mark
Step 8 `[x]`). All steps complete.

## Guardrails

- **NEVER run on `main`** — the command is for feature
  branches only
- **NEVER merge the PR** — /uf.finale creates PRs for
  review, not for immediate merge. Users merge manually
  after reviewer approval.
- **NEVER stage secret files without warning** — always
  prompt
- **NEVER commit without user approval** of the message
- **NEVER create a PR without user approval** of the
  title and body
- **ALWAYS report the PR URL** so the user can review it
- **If any step fails**, stop immediately with context
  and options — do not attempt to continue or recover
  silently
- **NEVER use `git push --force`** — always use
  `--force-with-lease` for safety when force push is
  required (e.g., after rebase)
- **NEVER rebase or force push without explicit user
  confirmation** — show the exact commands and wait
  for approval before executing any destructive
  git operation
- **NEVER commit sub-agent conflict resolutions without
  user approval** — always show the resolution diff
  and wait for explicit approval before committing

## Branch Safety

This command inherits the branch safety guardrails from
the OpenSpec and Speckit workflows:

- Checks `git status` before any destructive operation
- All changes are committed before any branch switch
- The remote branch is NOT deleted — it stays open with
  the PR until a reviewer merges

</protect>
