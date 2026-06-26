---
description: >
  Finalize a branch: commit, push, create PR, watch CI
  checks, and return to main. The PR stays open for
  review. One command to wrap up any feature or OpenSpec
  branch.
---
<!-- scaffolded by uf vdev -->

# Command: /finale

## User Input

```text
$ARGUMENTS
```

## Description

Automate the end-of-branch workflow. Stages all changes,
generates a conventional commit message, pushes, creates
a PR, watches CI checks, and returns to `main`. The PR
stays open for human review. Works with both Speckit
(`NNN-*`) and OpenSpec (`opsx/*`) branches.

## Usage

```
/finale                    # auto-detect everything
/finale fix the typo       # use as commit message hint
```

## Instructions

### 1. Branch Safety Gate

Get the current branch:

```bash
git rev-parse --abbrev-ref HEAD
```

- If on `main`: **STOP** with error:
  > "Cannot run /finale on main. Switch to a feature
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
  > "Warning: the following files may contain secrets
  > and should not be committed:
  >
  > - .env.local
  > - credentials.json
  >
  > Proceed with staging all files? These files will be
  > included in the commit."

  Ask for confirmation. If the user declines, stop and
  let them handle it manually.

- **Stage all changes**: `git add .`

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

c. Show the proposed message to the user:

> **Proposed commit message:**
>
> ```
> feat: add /finale slash command for branch finalization
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

d. Commit with the approved message.

### 4. Push to Remote

```bash
# Check if upstream is set
git rev-parse --abbrev-ref @{upstream} 2>/dev/null
```

- If no upstream: `git push -u origin <branch>`
- If upstream exists: `git push`
- If push fails: report error and **STOP**.

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
    verification commands. If on an `NNN-*` branch,
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
  _This PR was generated by /finale (AI-assisted)._
  ```

  e. **Review-council findings**: Check the conversation
  context for prior `/review-council` output. If
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

  If no `/review-council` was run in the session, or no
  unresolved findings exist, omit this section entirely.

  Note: findings come from session context and may be
  stale if code changes were made after the review. The
  user SHOULD verify accuracy during the approval step.

  f. Show the proposed PR content to the user:

  > **Proposed PR:**
  >
  > **Title:** `<title>`
  >
  > **Body:**
  > ```
  > <body>
  > ```
  >
  > Approve, edit, or provide your own?

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

### 6. Watch CI Checks

```bash
gh pr checks <number> --watch
```

- **If checks pass**: proceed to step 7.
- **If checks fail**: report the failure details and
  **STOP**:

  > "CI checks failed on PR #<number>:
  >
  > - Build & Test: FAIL (45s)
  >   https://github.com/.../runs/...
  >
  > Options:
  > 1. Investigate the failure
  > 2. Re-run the checks
  > 3. Stop here and fix manually"

  Ask the user how to proceed.

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

### 8. Summary

Display a completion report:

```
## Finale Complete

**Branch:** opsx/finale-command (pushed)
**Commit:** feat: add /finale slash command
**PR:** #65 — CI passed, ready for review
**Checks:** passed
**Status:** on main, up to date

Next: Request reviewers on the PR, then merge after
approval with: gh pr merge --rebase --delete-branch
```

## Guardrails

- **NEVER run on `main`** — the command is for feature
  branches only
- **NEVER merge the PR** — /finale creates PRs for
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

## Branch Safety

This command inherits the branch safety guardrails from
the OpenSpec and Speckit workflows:

- Checks `git status` before any destructive operation
- All changes are committed before any branch switch
- The remote branch is NOT deleted — it stays open with
  the PR until a reviewer merges
