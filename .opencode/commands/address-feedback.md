---
description: "Triage and address PR review feedback with structured assessment"
---
<!-- scaffolded by uf vdev -->


# Address Feedback

You are a token-efficient feedback analyst. The user will provide a PR number or you will auto-detect it from the current branch. Fetch all unresolved review feedback from GitHub, classify each item with evidence from project standards, present to the author for triage, then execute decisions as a batch: group related fixes into logical commits, review-council gate, push, reply comments, and artifact production.

The command follows four sequential phases (Ingest → Assess → Triage → Execute). Phases are not independently invocable — run all four in sequence every invocation.

## Arguments

- **PR number** (optional): The pull request number to address feedback for (e.g., `42`). If omitted, auto-detect the open PR for the current branch.

**Argument parsing** (before any tool calls): Check the user's message for a PR number argument. If present, set `PR_NUMBER` to that value immediately. All subsequent steps use `<PR_NUMBER>` — no auto-detection commands are needed or permitted.

---

## Phase 1: Ingest

Fetch all review feedback from GitHub and build the item list.

### 1.0 Prerequisites

Verify the `gh` CLI is available and authenticated:

```bash
which gh
```

If not found: **STOP** with error:
> "`gh` CLI is not installed. Install with
> `dnf install gh` (Fedora/RHEL),
> `brew install gh` (macOS), or see
> https://cli.github.com/ for other platforms."

```bash
gh auth status
```

If not authenticated: **STOP** with error:
> "GitHub CLI not authenticated. Run `gh auth login` to authenticate."

### 1.1 Resolve PR Number

**If `PR_NUMBER` was already set from the argument**: skip this step entirely. Do NOT run `gh pr view` or any branch detection.

**Only if no PR number was provided**:

```bash
gh pr view --json number --jq '.number'
```

If no open PR: **STOP** with error:
> "No open PR found for the current branch. Specify a PR number: `/address-feedback 42`"

### 1.2 Fetch PR Metadata

```bash
gh pr view <PR_NUMBER> --json number,url,title,body,headRefName,baseRefName,author
```

Record PR number, URL, branch name, description (for linked issue parsing), and author login (to filter self-comments).

### 1.3 Fetch Reviews and Comments

Determine `{owner}/{repo}` from `gh repo view --json owner,name --jq '.owner.login + "/" + .name'`.

Fetch all three data sources. Handle pagination — append `--paginate` or follow `Link` headers to ensure complete data:

```bash
# Reviews (approval state + inline comments)
gh api repos/{owner}/{repo}/pulls/<PR_NUMBER>/reviews --paginate

# Review comments (inline, threaded)
gh api repos/{owner}/{repo}/pulls/<PR_NUMBER>/comments --paginate

# Issue comments (general PR-level)
gh api repos/{owner}/{repo}/issues/<PR_NUMBER>/comments --paginate
```

**On API failure** (network error, HTTP 5xx, 403 rate limit): report the specific error and **STOP**. Do NOT proceed with partial data. If 403, suggest:
> "GitHub API error (403): rate limit exceeded. Wait and retry."

### 1.4 Determine Reviewer Authority

For each reviewer, map `author_association` to authority tier:

| `author_association` | Authority |
|---|---|
| `OWNER`, `MEMBER` | maintainer |
| `COLLABORATOR` | collaborator |
| `CONTRIBUTOR`, `FIRST_TIMER`, `FIRST_TIME_CONTRIBUTOR` | contributor |
| `NONE` (no bot indicators) | external |
| `NONE` (bot indicators) | bot |

**Bot detection**: login ending in `[bot]` OR account `type` is `Bot`.

### 1.5 Filter and Group

**Filter out**:
- Already-resolved review threads (check `isResolved` or thread state)
- The PR author's own top-level comments
- Pure approval reviews with no inline comments

**Group**: Threaded conversations (a review comment and its replies) into a single feedback item. The assessment considers the latest state of the full thread, not just the opening comment.

**Detect**: GitHub suggestion blocks (` ```suggestion `) — preserve as structured data for the triage phase.

### 1.6 Cache Check

Check local cache at `.uf/feedback/pr-<PR_NUMBER>/state.json`:

- **Cache exists, thread unchanged** (same comment count, same last comment ID): reuse cached assessment — skip Assess for this item
- **Cache exists, thread has new comments**: mark cached assessment stale — re-assess
- **Cache exists, code at referenced lines changed** (compare current file content at line range against cached content): mark stale — re-assess
- **Thread resolved on GitHub since last run**: skip entirely
- **Cache missing**: assess from scratch (correct but slower)

On crash-recovery re-entry: items marked as fully executed (`comment-posted`) in the cache MUST be skipped to prevent duplicate comments.

If no items remain after filtering: report "No unresolved feedback to address" and **STOP**.

---

## Phase 2: Assess

Classify each feedback item with evidence and produce a recommendation.

### 2.1 Load Project Context

Load context for evidence-based classification (D10, FR-010):

1. Convention packs from `.opencode/uf/packs/*.md`
2. Constitution from `.specify/memory/constitution.md`
3. `AGENTS.md` coding and testing conventions
4. Spec artifacts for the PR branch:
   - Speckit: `specs/NNN-*/` matching branch pattern
   - OpenSpec: `openspec/changes/*/` matching branch
5. Linked issues from PR description (`Fixes #N`, `Closes #N`, `Resolves #N`) — load acceptance criteria
6. If `review-context` convention pack exists, use it for standardized discovery. Otherwise inline the discovery logic above.

### 2.2 Tiered Assessment

For each feedback item, determine the assessment tier:

**Tier 1 (direct)** — assess using loaded context when ALL conditions are met:
- Single file affected
- Clear match to a convention pack rule (or purely subjective, no rule applies)
- No security implications
- No architectural implications
- Reviewer feedback and project standards do not conflict

**Tier 2 (Divisor escalation)** — delegate to the relevant Divisor agent via Task tool when ANY condition is met:
- Security concern raised → `divisor-adversary`
- Architectural change suggested → `divisor-architect`
- Multi-file impact → `divisor-architect`
- Feedback contradicts a convention pack rule → `divisor-guard`
- Ambiguous classification → route by primary domain
- Test strategy or coverage concern → `divisor-testing`
- Performance or operational concern → `divisor-sre`
- Multiple domains → invoke multiple agents in parallel

**Fallback**: If no Divisor agents are available (not deployed), all items fall back to Tier 1. Set `tier2_unavailable: true` in the assessment output.

### 2.3 Classify Each Item

For each item, produce:

| Field | Value |
|---|---|
| **Classification** | `DATA-DRIVEN` or `SUBJECTIVE` |
| **Evidence** | Specific convention pack section, constitution principle, or coding standard reference |
| **Reviewer authority** | maintainer / collaborator / contributor / external / bot |
| **Recommendation** | `ACCEPT` or `AUTHOR-DECIDES` (from authority matrix below) |
| **Suggested approach** | Concrete implementation description (if recommendation is ACCEPT) |
| **Conflict flag** | True if another item provides contradictory guidance on overlapping file/line range |

**Classification rules**:
- `DATA-DRIVEN`: grounded in a verifiable project rule (convention pack, constitution, coding standard, lint rule) or identifies a demonstrable defect (logic error, missing error handling, security vulnerability)
- `SUBJECTIVE`: personal preference, stylistic choice, or alternative approach not mandated by project rules

### 2.4 Apply Authority Matrix

| Authority | Data-Driven | Subjective |
|---|---|---|
| Maintainer | ACCEPT (MUST fix) | AUTHOR-DECIDES (SHOULD consider) |
| Collaborator | ACCEPT (MUST fix) | AUTHOR-DECIDES |
| Contributor | ACCEPT (MUST fix) | AUTHOR-DECIDES |
| External | ACCEPT if validated | AUTHOR-DECIDES |
| Bot | ACCEPT if validated | AUTHOR-DECIDES (informational) |

**Bot/external validation**: cross-reference the finding against project convention packs. If the pack confirms the rule → ACCEPT. If no matching rule → AUTHOR-DECIDES with note that the rule is not backed by project standards.

### 2.5 Conflict Detection

Compare items referencing overlapping file and line ranges. If two or more items provide contradictory guidance for the same code section, flag both with `CONFLICT`. Present conflicting items together in Phase 3 so the author can choose one approach.

### 2.6 Cache Assessment Results

Write assessment results to `.uf/feedback/pr-<PR_NUMBER>/state.json`:
- Per-thread: classification, tier, evidence, recommendation, comment count, last comment ID, content snapshot at referenced lines
- Timestamp: ISO 8601 last-fetched time

**Permissions**: files `600`, directories `700`.

---

## Phase 3: Triage

Present each item to the author one-by-one for a decision.

### 3.1 Present Items

For each feedback item, display:

```
─── Item N of M ──────────────────────────
Reviewer: @<login> (<authority>)
File: <file>:<line> (or "General PR comment")
Classification: <DATA-DRIVEN|SUBJECTIVE>
Evidence: <pack/rule references or "none">
Recommendation: <ACCEPT|AUTHOR-DECIDES>
Conflict: <yes — conflicts with item X|no>
Tier: <1|2> <(Divisor agents: ...)>

── Thread ──
<full thread content: all comments in conversation order>

── Suggested Approach ──
<concrete implementation description, if applicable>
─────────────────────────────────────────
```

If the item has a GitHub suggestion block, display it clearly as an applicable code change.

### 3.2 Author Decision

For each item, use the **AskUserQuestion tool** with
options `["Accept", "Modify", "Reject", "Ask"]`. The
author chooses exactly one:

| Decision | Follow-up | Queued action |
|---|---|---|
| **Accept** | (none) | Code change using suggested approach |
| **Modify** | Use **AskUserQuestion tool** (open-ended, no preset options) to collect the alternative approach | Code change using author's approach |
| **Reject** | Use **AskUserQuestion tool** (open-ended, no preset options) to collect evidence-based reasoning | Reply comment with reasoning |
| **Ask** | Use **AskUserQuestion tool** (open-ended, no preset options) to collect the clarification question | Reply comment with question |

**No item may be skipped or deferred.** Every item MUST receive a decision before the triage phase completes.

### 3.3 Conflicting Items

When presenting items flagged with CONFLICT, present both conflicting items together. The author chooses one approach. The non-chosen reviewer receives a reply comment explaining the decision.

### 3.4 Triage Summary

After all items are decided, display a summary:

```
─── Triage Summary ──────────────────────
ACCEPT:  N items (code changes queued)
MODIFY:  N items (code changes queued)
REJECT:  N items (reply comments queued)
ASK:     N items (reply comments queued)
Total:   N items
─────────────────────────────────────────
```

List each item with its decision. Use the
**AskUserQuestion tool** with options `["Confirm --
proceed with execution", "Revise -- change decisions"]`
before execution proceeds.

---

## Phase 4: Execute

Implement all queued actions as a batch.

### 4.1 Implement Code Changes

For each ACCEPT and MODIFY item, implement the code change:
- ACCEPT: apply the suggested approach
- MODIFY: apply the author's alternative approach
- GitHub suggestion blocks: apply the exact suggestion diff

**If a code change cannot be applied cleanly** (e.g., referenced code has changed since the review): skip that item with a clear report, note the failure for the reply comment, and continue with remaining items.

### 4.2 Commit Changes

Group related fixes into logical commits. For example, multiple naming changes in the same file or related error-handling fixes across a package belong together. Unrelated fixes get separate commits. Use conventional commit format:

```
fix(<scope>): <description>

Addresses PR #<PR_NUMBER> review feedback from @<reviewer>.

Signed-off-by: <author>
Assisted-by: <model>
```

Where `<model>` is the model family name you are
currently running as. To resolve the model name:
(1) read your model identifier from the system prompt
or runtime environment; (2) remove everything before
and including the last `/`; (3) remove everything
after and including the first `@`; (4) remove any
trailing date suffix matching `-YYYYMMDD` (a hyphen
followed by exactly 8 digits); (5) repeatedly remove
any trailing version segment matching `-N` (a hyphen
followed by a single digit at the end) until no more
remain; (6) validate the result
contains only `[a-zA-Z0-9._-]` characters. If the
result is empty, contains invalid characters, or
cannot be determined, use the literal string
`unknown-model` and warn the user (e.g., "Could not
determine AI model name — using 'unknown-model' in
attribution").

The `<scope>` is the package or directory of the changed files. The description summarizes the logical group of fixes.

### 4.3 Review-Council Gate

After all code changes are committed locally, run `/review-council` on the cumulative changes.

- **If passes**: continue to push
- **If fails**: enter fix loop (same behavior as `/unleash`). Fix findings and re-run council.
- **If fix loop exhausts iterations**: **STOP** and report persistent findings. Do NOT push until council passes.

### 4.4 Push Changes

Before pushing, fetch the remote branch state:

```bash
git fetch origin <branch>
git status
```

**If branch has diverged** (another contributor pushed
commits): warn the author and use the
**AskUserQuestion tool** with options `["Rebase onto
remote and push", "Abort -- preserve local commits"]`.

Push all commits:

```bash
git push origin <branch>
```

**If push fails** (network error, branch protection rejection): preserve local commits and report which commits are stranded. Provide guidance:
> "Push failed. Local commits preserved. Retry with `git push origin <branch>` after resolving the issue."

### 4.5 Post Reply Comments

After push succeeds (or if there are no code changes),
post reply comments to the PR. Before posting, use the
**AskUserQuestion tool** with options `["Yes -- post
reply comments", "No -- skip posting"]`.

For each item, compose the reply:

| Decision | Reply content |
|---|---|
| ACCEPT | "Addressed in \`<commit_sha>\`: <brief description of the change>" |
| MODIFY | "Addressed in \`<commit_sha>\` (modified approach): <brief description>" |
| REJECT | Evidence-based reasoning referencing convention pack rules or constitution principles |
| ASK | Author's clarification question |

Post replies to the correct review thread. Always write the comment body to a temporary file and use `--input` to prevent shell injection from AI-generated or reviewer-authored text:

```bash
# Write reply body to temp file (never interpolate into shell args)
REPLY_FILE=$(mktemp)
cat > "$REPLY_FILE" << 'REPLY_EOF'
<reply content here>
REPLY_EOF

# For review comments (inline threads)
gh api repos/{owner}/{repo}/pulls/<PR_NUMBER>/comments/<comment_id>/replies \
  --method POST --input "$REPLY_FILE"

# For issue comments (general)
gh api repos/{owner}/{repo}/issues/<PR_NUMBER>/comments \
  --method POST --input "$REPLY_FILE"

# Clean up
rm -f "$REPLY_FILE"
```

**Crash recovery**: Track each comment's posting status in the cache (`comment-posted` flag). If posting fails partway (e.g., API rate limit after 3 of 6 comments), report partial progress:
> "Posted 3 of 6 reply comments. Items 4-6 pending. Re-run `/address-feedback <PR_NUMBER>` to retry."

Record progress in `.uf/feedback/pr-<PR_NUMBER>/state.json` for idempotent retry.

### 4.6 Resolve Threads

After posting reply comments for accepted items, offer to resolve those threads:

```bash
# GraphQL mutation to resolve a thread
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "<thread_id>"}) { thread { isResolved } } }'
```

Use the **AskUserQuestion tool** with options
`["Yes -- resolve accepted threads", "No -- leave
threads open"]` before resolving.

### 4.7 Produce Feedback-Triage Artifact

Write the artifact to `.uf/artifacts/feedback-triage/pr-<PR_NUMBER>-round-<M>.json`.

**Round number**: scan existing files for the highest round number and add 1 (not a file count — handles gaps from deleted files).

**Atomic write**: write to a temp file first, then rename to the final path.

**Envelope wrapper** (standard schema):

```json
{
  "hero": "cobalt-crush",
  "version": "1.0.0",
  "timestamp": "<ISO 8601>",
  "artifact_type": "feedback-triage",
  "schema_version": "1.0.0",
  "context": {
    "branch": "<PR source branch>",
    "commit": "<HEAD SHA after fixes, or pre-fix HEAD>",
    "backlog_item_id": "<linked issue or PR-N>"
  },
  "payload": {
    "pr_number": 42,
    "pr_url": "https://github.com/...",
    "branch": "<PR source branch>",
    "round": 1,
    "items": [
      {
        "thread_id": "...",
        "reviewer": "<login>",
        "reviewer_role": "maintainer|collaborator|contributor|external|bot",
        "file": "internal/foo/bar.go",
        "line": 42,
        "classification": "data-driven|subjective",
        "tier": 1,
        "evidence": ["go.md CS-001: ..."],
        "recommendation": "accept|author-decides",
        "decision": "accept|modify|reject|ask",
        "decision_reasoning": "...",
        "commit_sha": "abc1234",
        "divisor_agents_used": [],
        "tier2_unavailable": false,
        "conflict_flag": false
      }
    ],
    "summary": {
      "total_items": 6,
      "accepted": 3,
      "modified": 1,
      "rejected": 1,
      "asked": 1,
      "tier1_count": 4,
      "tier2_count": 2,
      "divisor_agents_invoked": ["adversary", "architect"]
    }
  }
}
```

Fields `file`, `line`, `decision_reasoning`, and `commit_sha` may be `null` (general PR comments have null file/line; REJECT/ASK items have null commit_sha).

---

## Guardrails

1. **No auto-merge**: This command addresses feedback. It NEVER merges the PR, approves the PR, or dismisses reviews.

2. **No code changes without triage**: Code is only modified after the author explicitly decides ACCEPT or MODIFY for each item. No autonomous fixes.

3. **No comments without confirmation**: Every PR comment is shown to the author before posting. No autonomous PR communication.

4. **No push without review-council**: Code changes MUST pass `/review-council` before pushing. No bypass.

5. **No partial data processing**: If GitHub API fails during ingestion, STOP. Do not assess or triage based on incomplete feedback.

6. **Cache is disposable**: The command MUST produce correct results even if `.uf/feedback/` is deleted. Never treat cache as authoritative — GitHub is the source of truth.

7. **Gatekeeping integrity**: MUST NOT modify quality gates, coverage thresholds, CI flags, or convention pack rules while addressing feedback. If a feedback item requests weakening a gate, classify as SUBJECTIVE with AUTHOR-DECIDES and note the gatekeeping constraint.

8. **Shell injection prevention**: Always write AI-generated or reviewer-authored text to temporary files and use `--input` for `gh api` calls. Never interpolate untrusted text into shell arguments.

9. **File permissions**: Cache files `600`, cache directories `700`. The `.uf/feedback/` directory MUST be in `.gitignore`.

10. **Commit scope**: Only commit files directly related to addressing the specific feedback item. Do not bundle unrelated changes into feedback fix commits.
