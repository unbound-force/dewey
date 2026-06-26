---
description: "Triage a GitHub issue using the Divisor review panel"
---
<!-- scaffolded by uf vdev -->


# Triage Issue

You are a token-efficient issue analyst. The user provides a GitHub issue number. Fetch the issue, fan out to the Divisor review panel for multi-agent assessment, consolidate verdicts into a classification, then execute triage actions (labels, comments, child issues) with user confirmation. Produce a JSON artifact for every invocation.

The command follows four sequential phases (Ingest → Assess → Classify → Act). Phases are not independently invocable — run all four in sequence every invocation.

## Arguments

- **Issue number** (required): The GitHub issue number to triage (e.g., `42`).

**Argument parsing** (before any tool calls): Check the user's message for an issue number argument. Validate that it matches `^[1-9][0-9]*$` (positive integer, no leading zeros). If the argument is missing, invalid, zero, negative, or contains non-numeric characters: **STOP** with error:
> "Invalid issue number. Must be a positive integer."

No `gh` CLI commands are permitted until the argument passes validation. Set `ISSUE_NUMBER` to the validated value. All subsequent steps use `<ISSUE_NUMBER>`.

---

## Phase 1: Ingest

Fetch the issue, repository context, and duplicate candidates.

### 1.0 Prerequisites

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

Record `{owner}/{repo}` for all subsequent API calls. Repository identifiers MUST NOT be hardcoded.

### 1.1 Fetch Issue

```bash
gh issue view <ISSUE_NUMBER> --json number,title,body,labels,author,comments,assignees,createdAt,state
```

**If the issue does not exist**: **STOP** with the `gh` error output.

**If the issue is closed**: **STOP** with error:
> "Issue #<ISSUE_NUMBER> is closed. Triage applies to open issues only."

Record the full issue data for agent consumption.

### 1.2 Re-Run Detection

Check for previous triage actions on this issue to support idempotent re-runs:

1. **Existing labels**: Extract the `labels` array from the issue data fetched in 1.1. Record which triage-relevant labels are already applied (`bug`, `enhancement`, `question`, `design-discussion`, `duplicate`, `needs-info`).

2. **Previous triage comment**: Check the `comments` array for any comment containing the footer `_This triage was performed by the Divisor review panel._`. If found, note that a previous triage comment exists.

3. **Existing artifact**: Check if `.uf/artifacts/issue-triage/issue-<ISSUE_NUMBER>.json` exists. If so, the new artifact will use a round number (e.g., `issue-<ISSUE_NUMBER>-2.json`).

### 1.3 Fetch Repository Context

Read project context files to provide agents with design philosophy:

1. Read `README.md` (if it exists) — project description and purpose
2. Read `AGENTS.md` — coding conventions, project structure, behavioral rules

These provide agents with the context needed to assess whether an issue aligns with project goals and conventions.

### 1.4 Duplicate Check

Extract keywords from the issue title and first paragraph of the body for duplicate search:

1. **Extract keywords**: Take the issue title and the first paragraph of the body. Select 5-10 meaningful keywords (nouns, verbs, technical terms). Exclude common stop words.

2. **Sanitize keywords**: Remove shell metacharacters (`;`, `|`, `` ` ``, `$`, `(`, `)`, `"`) and strip any strings starting with `--` to prevent CLI flag injection.

3. **Search for duplicates**:

```bash
gh issue list --search "<sanitized-keywords>" --state open --json number,title,url --limit 10
```

4. **Filter results**: Exclude the current issue from the results. Record any remaining candidates as `duplicate_candidates` for agent evaluation.

---

## Phase 2: Assess (Parallel Fan-Out)

Fan out to the Divisor review panel for multi-agent assessment.

### 2.1 Discover Available Agents

1. **Read the `.opencode/agents/` directory** using the Read tool to list all entries.

2. **Filter for triage panel agents**: From the directory listing, select only entries matching these five target agents:
   - `divisor-adversary.md`
   - `divisor-architect.md`
   - `divisor-guard.md`
   - `divisor-sre.md`
   - `divisor-testing.md`

   Note: `divisor-curator` is excluded from the triage panel. Its domain (documentation gap detection and content pipeline triage) is not relevant to issue classification.

3. **Extract agent names**: For each matching file, strip the `.md` extension to get the agent name (e.g., `divisor-adversary.md` → `divisor-adversary`).

4. **Guard clause**: If zero triage panel agents are discovered: **STOP** with error:
   > "No Divisor agents found. At least one agent is required for triage."

5. **Note absent agents**: Compare discovered agents against the five target agents. Any target agent not discovered is noted as absent. Absent agents are informational only — they do not block triage.

### 2.2 Fan Out to Agents

Delegate assessment to all discovered agents **in parallel** using the Task tool. For each agent, provide:

- Full issue text (title, body, comments, author, labels, creation date)
- Repository context (README.md and AGENTS.md summaries)
- Duplicate candidates from Phase 1

Each agent MUST return a **structured assessment** with these fields:

| Field | Values |
|---|---|
| **verdict** | `VALID`, `INVALID`, or `NEEDS-CLARIFICATION` |
| **category** | `bug`, `feature`, `enhancement`, `question`, `opinion`, `duplicate`, or `needs-clarification` |
| **objectivity** | `objective` (verifiable evidence exists) or `subjective` (preference-based) |
| **reasoning** | Evidence-based explanation for the verdict and category |
| **split_recommendation** | `null` or an array of `{title, description}` — each item is a proposed child issue |

Use the following focus areas when prompting each agent:

| Agent | Focus |
|---|---|
| `divisor-adversary` | Security implications, attack surface, error handling gaps, dependency risks, injection vectors |
| `divisor-architect` | Architectural alignment, design patterns, scope fit, technical feasibility, convention adherence |
| `divisor-guard` | Intent alignment with project goals, constitution compliance, scope discipline, user value |
| `divisor-sre` | Operational impact, performance implications, deployment concerns, monitoring gaps, reliability |
| `divisor-testing` | Testability, reproducibility, test coverage implications, regression risk, acceptance criteria clarity |

For any discovered agent not in this table, use a generic prompt: "Assess this GitHub issue for validity, category, and objectivity. Return your structured assessment."

### 2.3 Collect Assessments

Collect all agent responses. If an agent fails to respond or returns an unparseable response, record the failure and proceed with the remaining assessments. The command SHOULD proceed with as few as one successful assessment.

---

## Phase 3: Classify (Consolidation)

Consolidate individual agent assessments into a single classification.

### 3.1 Verdict Resolution

Apply the three-rule majority in order:

1. **NEEDS-CLARIFICATION majority**: If NEEDS-CLARIFICATION verdicts constitute a majority of all agents (>50%), the overall verdict is **NEEDS-CLARIFICATION**.

2. **Exclude NEEDS-CLARIFICATION**: Otherwise, exclude NEEDS-CLARIFICATION verdicts. If VALID or INVALID has a majority of the remaining votes, that verdict wins.

3. **Tie-breaking**: If the remaining votes tie (equal VALID and INVALID after excluding NEEDS-CLARIFICATION), the overall verdict defaults to **NEEDS-CLARIFICATION**.

When fewer than 5 agents are available, majority of available agents applies.

### 3.2 Category Resolution

Resolve category disagreements using the specificity hierarchy:

```
bug > feature > enhancement > needs-clarification > opinion > question
```

When agents disagree, the most specific category wins.

**Duplicate resolution** (independent of hierarchy): An issue is classified as `duplicate` only when BOTH conditions are met:
1. Phase 1 duplicate search found matching candidates
2. At least two agents independently classify the issue as `duplicate`

When both conditions are met, `duplicate` takes precedence over the specificity hierarchy.

### 3.3 Objectivity Classification

- **Objective**: At least ONE agent provides verifiable evidence (reproducible bug, measurable performance issue, documented behavior contradiction)
- **Subjective**: ALL agents agree the issue is preference-based

### 3.4 Record Dissent

Record all dissenting agents (those whose verdict differs from the consolidated verdict) with their agent name and reasoning. These are included in the artifact for provenance tracing.

### 3.5 Synthesize Split Recommendations

If two or more agents recommend splitting the issue, synthesize their recommendations into a unified set of proposed child issues. Deduplicate overlapping proposals and merge complementary ones.

---

## Phase 4: Act (Interactive)

Present the analysis and execute triage actions with user confirmation.

### 4.1 Present Analysis Summary

Display the consolidated classification to the user:

```
─── Issue Triage Summary ─────────────────
Issue:        #<ISSUE_NUMBER>: <title>
Verdict:      <VALID|INVALID|NEEDS-CLARIFICATION>
Category:     <category>
Objectivity:  <objective|subjective>
Agents:       <N> consulted, <N> available
Dissent:      <agent names and brief reasons, or "none">
Duplicates:   <candidate issue numbers, or "none found">
Split:        <"recommended" with count, or "not recommended">
──────────────────────────────────────────

── Agent Assessments ──
<For each agent: name, verdict, category, brief reasoning>

── Proposed Actions ──
• Label: <label> (auto-apply / requires confirmation)
• Comment: <tone tier> (requires confirmation)
• Split: <N child issues> (requires confirmation, if applicable)
──────────────────────────────────────────
```

### 4.2 Label Application

Labels are applied **automatically without user confirmation**, with one exception: the `duplicate` label requires user confirmation because it carries implicit "close" semantics.

**Label mapping**:

| Category | Label |
|---|---|
| `bug` | `bug` |
| `feature` | `enhancement` |
| `enhancement` | `enhancement` |
| `question` | `question` |
| `opinion` | `design-discussion` |
| `duplicate` | `duplicate` |
| `needs-clarification` | `needs-info` |

**Re-run check**: If the target label is already applied (detected in Phase 1.2), skip label application and note "label already present."

**Label existence check**: Before applying, verify the label exists in the repository. If it does not exist, create it:

```bash
gh label create "<label>" --description "<description>" --color "<color>"
```

If label creation fails due to insufficient permissions, report the specific label that could not be created, skip that label, and continue with remaining actions. Record the failure in `actions_taken`.

**Apply the label**:

```bash
gh issue edit <ISSUE_NUMBER> --add-label "<label>"
```

**For `duplicate` label only**: Inform the user that the
`duplicate` label signals the issue should be closed.
Use the **AskUserQuestion tool** with options
`["Yes -- apply duplicate label", "No -- skip"]`.

### 4.3 Comment Composition and Posting

Compose a triage comment based on the consolidated classification. The comment tone follows three tiers:

| Verdict | Tone |
|---|---|
| VALID | Factual analysis: classification, recommendations, next steps |
| INVALID / OPINION | Warm, non-dismissive: acknowledge reporter effort, explain reasoning with specific references, offer alternatives, invite continued engagement |
| NEEDS-CLARIFICATION | Specific questions: what information would help, what to reproduce, what context is missing |

**All comments MUST include the footer**:
```
_This triage was performed by the Divisor review panel._
```

**If duplicate candidates were found** (but not classified as duplicate): mention similar issues in the comment for the reporter's awareness.

**Re-run check**: If a previous triage comment was
detected in Phase 1.2, warn the user that posting
another comment may cause confusion. Use the
**AskUserQuestion tool** with options `["Yes -- post
another comment", "No -- skip comment"]`. If the user
selects "No -- skip comment", record
`comment_posted: false` in the artifact and skip to
Phase 4.4.

**Present the composed comment to the user for
confirmation**: Use the **AskUserQuestion tool** with
options `["Approve -- post as-is", "Modify -- adjust
comment text", "Abort -- do not post"]`.

| Selection | Action |
|---|---|
| **Approve -- post as-is** | Post the comment as-is |
| **Modify -- adjust comment text** | Use **AskUserQuestion tool** (open-ended, no preset options) to collect the adjusted comment text; post the adjusted version |
| **Abort -- do not post** | Do not post any comment; record `comment_posted: false` in artifact |

**Post the comment** (on Approve or Modify):

Write the comment body to a temporary file and post via `gh api --input`. NEVER interpolate comment text into shell arguments.

```bash
# Write comment body to temp file (never interpolate into shell args)
COMMENT_FILE=$(mktemp)
chmod 600 "$COMMENT_FILE"
cat > "$COMMENT_FILE" << 'COMMENT_EOF'
{"body": "<comment content as JSON-escaped string>"}
COMMENT_EOF

# Post the comment
gh api repos/{owner}/{repo}/issues/<ISSUE_NUMBER>/comments \
  --method POST --input "$COMMENT_FILE"

# Clean up temp file in ALL paths (success, failure, abort)
rm -f "$COMMENT_FILE"
```

**On API failure**: Report the error, do NOT retry. Record the failure in `actions_taken`. Clean up the temp file.

### 4.4 Child Issue Creation (If Splitting)

This section applies only when Phase 3.5 produced split recommendations.

For each proposed child issue:

1. **Present to user**: Show the proposed title and body.
   Use the **AskUserQuestion tool** with options
   `["Yes -- create this child issue", "No -- skip"]`
   for each child issue individually.

2. **Duplicate check**: Before creating, search for existing issues matching the proposed title:

```bash
gh issue list --search "<sanitized-child-title>" --state open --json number,title --limit 5
```

   If a close match is found, warn the user about the
   potential duplicate and use the **AskUserQuestion
   tool** with options `["Yes -- create anyway",
   "No -- skip this child issue"]`.

3. **Create the child issue** (if the user selected a confirming option):

Each child issue body MUST include a cross-reference: `Split from #<ISSUE_NUMBER>`.

Write the child issue payload to a temporary file and create via `gh api --input`:

```bash
CHILD_FILE=$(mktemp)
chmod 600 "$CHILD_FILE"
cat > "$CHILD_FILE" << 'CHILD_EOF'
{"title": "<child title>", "body": "<child body with Split from #N>"}
CHILD_EOF

gh api repos/{owner}/{repo}/issues \
  --method POST --input "$CHILD_FILE"

rm -f "$CHILD_FILE"
```

Record each created child issue number and title.

4. **Post parent comment**: After all confirmed child issues are created, post a comment on the parent issue listing the created children:

```
This issue has been split into the following child issues:
- #<child_number>: <child_title>
- #<child_number>: <child_title>

_This triage was performed by the Divisor review panel._
```

Post this comment using the same temp file + `--input` pattern from 4.3.

5. **Parent issue remains open**: The parent issue MUST NOT be auto-closed after splitting.

### 4.5 Produce Triage Artifact

Write the artifact for **every invocation**, regardless of outcome (success, user abort, partial failure).

**Artifact path**: `.uf/artifacts/issue-triage/issue-<ISSUE_NUMBER>.json`

**Round number**: If the file already exists, scan for the highest existing round number and increment (e.g., `issue-42.json` exists → write `issue-42-2.json`; `issue-42-2.json` exists → write `issue-42-3.json`).

**Atomic write**: Write to a temp file first, then rename to the final path.

**Envelope wrapper** (standard schema):

```json
{
  "hero": "the-divisor",
  "version": "1.0.0",
  "timestamp": "<ISO 8601>",
  "artifact_type": "issue-triage",
  "schema_version": "1.0.0",
  "context": {
    "repository": "<owner/repo>",
    "issue_number": "<ISSUE_NUMBER>"
  },
  "payload": {
    "issue_number": 42,
    "issue_url": "https://github.com/<owner>/<repo>/issues/<ISSUE_NUMBER>",
    "repo": "<owner>/<repo>",
    "title": "<issue title>",
    "author": "<issue author login>",
    "category": "bug",
    "validity": "valid",
    "objectivity": "objective",
    "duplicate_of": null,
    "split_issues": [],
    "assessments": [
      {
        "agent": "divisor-adversary",
        "verdict": "valid",
        "category": "bug",
        "objectivity": "objective",
        "reasoning": "...",
        "split_recommendation": null
      }
    ],
    "actions_taken": {
      "labels_applied": ["bug"],
      "comment_posted": true,
      "child_issues_created": [],
      "label_creation_failed": false
    },
    "summary": {
      "agents_consulted": 5,
      "agents_available": 5,
      "consensus": "4/5 valid",
      "dissenting_agents": [
        {"agent": "divisor-sre", "reasoning": "..."}
      ]
    }
  }
}
```

Fields may be `null` when not applicable (e.g., `duplicate_of` when the issue is not a duplicate). Use lowercase enum values in the payload (`valid`, `invalid`, `needs-clarification`, `bug`, `feature`, etc.) matching the schema definitions. Use UPPERCASE (`VALID`, `INVALID`, etc.) only in display/summary output to the user.

**On partial failure**: The `actions_taken` section MUST reflect the actual state — which actions completed and which failed. Set `label_creation_failed` to `true` if label creation failed due to permissions. Set `comment_posted` to `false` if the user aborted or the API call failed.

---

## Guardrails

1. **No auto-close**: MUST NOT close or lock any issue under any circumstances. The parent issue remains open even after splitting. The `duplicate` label is applied only with user confirmation.

2. **No comments without confirmation**: Every issue comment is shown to the user before posting. No autonomous public communication.

3. **No child issues without confirmation**: Every proposed child issue is presented to the user before creation. No autonomous issue creation.

4. **Single issue per invocation**: The command processes exactly one issue. If the user provides multiple issue numbers, use only the first and ignore the rest with a warning.

5. **gh CLI verification before API calls**: `which gh` and `gh auth status` MUST succeed before any GitHub API call. Specific error messages per prerequisite failure.

6. **API failure handling**: When any `gh` CLI or API call fails (network error, HTTP 403 rate limit, HTTP 5xx), report the specific error, indicate which phase failed, and list any actions already completed. MUST NOT proceed with subsequent GitHub mutations after a failure. MUST still produce the artifact with `actions_taken` reflecting partial state.

7. **Idempotent re-run**: On re-invocation for the same issue, detect previously applied labels (from issue data) to avoid duplication. Detect previously posted triage comments by checking for the Divisor review panel footer. Use round numbers for artifacts to preserve history.

8. **Shell injection prevention**: All untrusted text (issue content, agent output, synthesized comments, child issue content) MUST be written to temporary files and passed via `--input` for all `gh api` calls. Untrusted text MUST NOT be interpolated into shell arguments. Temp files MUST use restrictive permissions (`chmod 600`) and be cleaned up in all exit paths (success, failure, abort).

9. **Safe artifact paths**: The issue number is validated as a positive integer (matching `^[1-9][0-9]*$`) before use in any file path. This validation occurs in the Arguments section before any other processing.
