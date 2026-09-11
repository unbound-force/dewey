---
description: >
  Apply Unbound Force project customizations to third-party tool
  files (OpenSpec skills and commands). Uses LLM reasoning to find
  correct insertion points. Run after uf init, uf setup, or
  updating the OpenSpec CLI.
---
<!-- scaffolded by uf v0.17.0 -->

# Command: /uf.init

## Description

Apply project-specific customizations to third-party tool files
that cannot be modified by the `uf init` Go binary. This command
uses LLM reasoning to read target files, understand their
structure, and intelligently insert customizations at the correct
locations.

**When to run**: After `uf init` (terminal), after `uf setup`,
or after updating the OpenSpec CLI (`npm update`). Safe to re-run
-- idempotent.

## Instructions

### Step 0: Command Directory Migration

Check whether the legacy `.opencode/command/` directory needs
to be migrated to `.opencode/commands/`:

1. Check if `.opencode/command/` exists (old directory):
   ```bash
   ls -d .opencode/command/ 2>/dev/null
   ```

2. Check if `.opencode/commands/` exists (new directory):
   ```bash
   ls -d .opencode/commands/ 2>/dev/null
   ```

3. **If old exists and new does NOT exist**: rename the
   directory:
   ```bash
   mv .opencode/command .opencode/commands
   ```
   Report: `✅ command/ → commands/: migrated`

4. **If both exist**: move unique files from old to new,
   remove duplicates from old, and clean up:
   ```bash
   # For each file in old dir, move if not in new, else remove
   for f in .opencode/command/*; do
     name="$(basename "$f")"
     if [ -f ".opencode/commands/$name" ]; then
       rm "$f"
     else
       mv "$f" .opencode/commands/
     fi
   done
   rmdir .opencode/command 2>/dev/null
   ```
   Report: `✅ command/ → commands/: merged (old dir cleaned up)`

5. **If old does NOT exist**: no migration needed.
   Report: `⊘ command/ migration: not needed`

### Step 1: Check Prerequisites

Verify the project has been initialized:

1. Check that `.opencode/` directory exists. If not, **STOP** with:
   > `.opencode/` not found. Run `uf init` from your terminal first.

2. Check that these 4 OpenSpec skill files exist:
   - `.opencode/skills/openspec-propose/SKILL.md`
   - `.opencode/skills/openspec-apply-change/SKILL.md`
   - `.opencode/skills/openspec-archive-change/SKILL.md`
   - `.opencode/skills/openspec-explore/SKILL.md`

3. Check that these 3 OpenSpec command files exist:
   - `.opencode/commands/opsx-propose.md`
   - `.opencode/commands/opsx-apply.md`
   - `.opencode/commands/opsx-archive.md`

For each missing file, report an error:
> `❌ <path>: file not found`
> This file should have been created by `openspec init` which
> runs as part of `uf init`. Run `uf setup` to install OpenSpec,
> then `uf init` to scaffold files, then re-run `/uf.init`.

Continue checking remaining files even if some are missing.
**Track which files are missing** -- in Steps 2-4, skip any
file that was reported missing here. Report
`❌ <filename>: skipped (file not found in prerequisites)`.

**Recovery note**: All target files are git-tracked. If any
insertion looks wrong after running this command, restore with
`git checkout -- <path>`. Run `git diff` after `/uf.init`
completes to review all changes before committing.

### Step 2: Apply Branch Enforcement

For each target file listed below, apply the branch enforcement
customization. Each enforcement category has **independent**
idempotency checks -- presence of one variant MUST NOT cause
other variants to be skipped. For each file:

1. **Read** the file content
2. **Check** each applicable variant independently:
   - **Basic branch check**: Look for `opsx/<name>`,
     `opsx/<change-name>`, or `git checkout -b opsx/`
   - **Dirty tree check** (propose only): Look for
     `git status --short` in a pre-branch-creation
     context
   - **Commit-before-archive** (archive-change only):
     Look for `git add` and `git commit` appearing
     before the archive move step
   - **Branch-switch confirmation** (explore only): Look
     for `uncommitted changes` or `switch branches` in
     the guardrails section
3. **For each variant**: If present, report
   `⊘ <filename>: [variant] already present (skipped)`.
   If not present, insert it and report
   `✅ <filename>: [variant] inserted`

#### Branch Enforcement: Propose (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-propose/SKILL.md`
- `.opencode/commands/opsx-propose.md`

**Two independent checks**:

**A. Basic branch check** -- idempotency marker:
`opsx/<name>` or `git checkout -b opsx/`

If not present, insert after the step that creates the
change directory (`openspec new change "<name>"`):

> **Create and checkout a branch**
>
> ```bash
> git checkout -b opsx/<name>
> ```
>
> **Guard**: Before creating the branch, check the current branch:
> - If already on `opsx/<name>` (exact match): skip branch creation, proceed.
> - If on a different `opsx/*` branch: **STOP** with error: "Already on branch `opsx/<other>` -- finish or archive that change first."
> - If on `main` or any non-opsx branch: create and checkout `opsx/<name>`.

**Where**: After the change directory creation step, before the
artifact creation steps. Insert as a new numbered step; do NOT
renumber existing steps (to avoid accidental content loss).

**B. Dirty tree check** -- idempotency marker:
`git status --short` in a pre-branch-creation context

If not present, insert before the branch creation guard
(part A above), or immediately before the existing branch
check if part A is already present:

> **Check for uncommitted changes**
>
> Before creating or switching branches, run
> `git status --short`. If there are uncommitted changes
> (staged, unstaged, or untracked files that appear
> related to work):
> - **STOP** and ask the user for confirmation before
>   switching branches. Show what uncommitted changes
>   exist and warn that switching branches with a dirty
>   working tree may cause changes to be applied to the
>   wrong branch.
> - If the user confirms, proceed. If not, abort.
> - Exception: if the user explicitly requested a new
>   change, this still requires confirmation -- never
>   silently switch branches with uncommitted work.

**Where**: Before the branch creation guard. Insert as a
sub-step or preceding step.

#### Branch Enforcement: Apply (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-apply-change/SKILL.md`
- `.opencode/commands/opsx-apply.md`

**What to insert**: Before the implementation begins, insert a
branch validation step:

> **Validate branch**
>
> Run `git rev-parse --abbrev-ref HEAD` to get the current branch.
>
> - If the current branch is `opsx/<change-name>`: proceed.
> - If the current branch is NOT `opsx/<change-name>`: **STOP** with error:
>   > "Must be on branch `opsx/<change-name>` to implement this change.
>   > Run: `git checkout opsx/<change-name>`"

**Where**: After the change selection step, before the status
check step. Insert as a new numbered step; do NOT renumber
existing steps.

#### Branch Enforcement: Archive (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-archive-change/SKILL.md`
- `.opencode/commands/opsx-archive.md`

**Two independent checks**:

**A. Return to main branch** -- idempotency marker:
`git checkout main` after the archive move

If not present, insert after the archive move completes:

> **Return to main branch**
>
> After the archive move completes:
> ```bash
> git checkout main
> ```
>
> The `opsx/<name>` branch still exists locally. Note in the
> summary that the developer can delete it manually with
> `git branch -d opsx/<name>` if desired.

**Where**: After the step that moves the change directory to
the archive, before the display summary step. Insert as a new
numbered step; do NOT renumber existing steps.

**B. Commit-before-archive** -- idempotency markers:
`git add` and `git commit` appearing before the archive
move step

If not present, insert before the archive move step
(the step that moves the change directory to the archive):

> **Commit and push all changes**
>
> Before archiving, ensure all work is committed:
>
> 1. Run `git status --short` to check for uncommitted
>    changes.
> 2. If uncommitted changes exist:
>    - Stage the change directory and implementation
>      files explicitly:
>      `git add openspec/changes/<name>/ .opencode/`
>      and any other modified files shown by
>      `git status --short`
>    - Commit with a descriptive message:
>      `git commit -m "feat(<name>): complete implementation"`
>    - Push to remote: `git push`
> 3. Verify the working tree is clean after push.
>
> **CRITICAL**: Do NOT move to the archive step with
> uncommitted changes. All work must be committed and
> pushed before the change directory is moved to the
> archive.

**Where**: Before the step that moves the change directory
to the archive. Insert as a new numbered step; do NOT
renumber existing steps.

#### Branch Enforcement: Explore (Guardrail Only)

**Target file**:
- `.opencode/skills/openspec-explore/SKILL.md`

**Note**: Explore mode does not create or modify changes, so
full branch management does not apply. However, explore may
lead to creating a proposal, which requires a branch switch.

**Single check** -- idempotency marker: `uncommitted changes`
or `switch branches` in the guardrails section

If not present, append this bullet to the explore SKILL.md
guardrails section:

> - Don't switch branches without confirmation -- If
>   exploration leads to creating a proposal (which
>   requires a new `opsx/` branch), check for uncommitted
>   changes first and ask the user before switching.
>   Never silently leave uncommitted work behind.

**Where**: Append to the existing guardrails bullet list
at the end of the file.

Report: `✅ openspec-explore/SKILL.md: branch-switch
confirmation inserted` or `⊘ openspec-explore/SKILL.md:
branch-switch confirmation already present (skipped)`

### Step 3: Apply Dewey Context

For each target file listed below, apply Dewey context query
instructions. For each file:

1. **Read** the file content
2. **Check** if Dewey context is already present. Look for
   `dewey_semantic_search` or `dewey_search` as **tool
   invocation references** (not in prose descriptions or
   comments). The word "Dewey" alone in a sentence is NOT
   sufficient -- it must appear as part of actual tool usage
   instructions (e.g., "use `dewey_semantic_search` to find...").
3. **If already present**: Report `⊘ <filename>: already present (skipped)`
4. **If not present**: Find the correct insertion point and
   insert. Report `✅ <filename>: inserted`

#### Dewey Context: Propose (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-propose/SKILL.md`
- `.opencode/commands/opsx-propose.md`

**What to insert**: Before drafting the proposal artifacts, add
a context retrieval step:

> **Retrieve context from Dewey**
>
> Before drafting the proposal, query Dewey for relevant context:
>
> - `dewey_semantic_search` with the change description to find
>   related specs, past proposals, and similar changes
> - `dewey_semantic_search_filtered` with `source_type: "github"`
>   to find related issues across the organization
> - `dewey_traverse` on any discovered related specs to understand
>   dependencies
>
> Use the retrieved context to inform the proposal's scope,
> identify potential conflicts with existing work, and reference
> relevant prior decisions.
>
> If Dewey is unavailable, proceed without cross-repo context --
> use direct file reads of local specs and backlog items instead.

**Where**: After change creation (and branch setup if present),
before artifact creation begins. This location is independent
of whether branch enforcement was applied.

#### Dewey Context: Apply (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-apply-change/SKILL.md`
- `.opencode/commands/opsx-apply.md`

**What to insert**: Before implementing tasks, add a context
retrieval step:

> **Retrieve implementation context from Dewey**
>
> Before implementing, query Dewey for relevant patterns:
>
> - `dewey_semantic_search` with the task description to find
>   similar implementations in other repos
> - `dewey_semantic_search_filtered` with `source_type: "web"`
>   to find relevant toolstack documentation
> - `dewey_search` for convention pack references related to the
>   task's domain
>
> Use the retrieved context to follow established patterns and
> avoid reinventing solutions that already exist in the ecosystem.
>
> If Dewey is unavailable, proceed with direct file reads of
> convention packs and local code examples.

**Where**: After the change selection step (and branch
validation if present), before the implementation loop begins.
This location is independent of whether branch enforcement was
applied.

#### Dewey Context: Explore (Skills only)

**Target file**:
- `.opencode/skills/openspec-explore/SKILL.md`

**What to insert**: As part of the exploration workflow, add
Dewey as the primary investigation tool:

> **Use Dewey for investigation**
>
> When exploring ideas or investigating problems, use Dewey as
> the primary context source:
>
> - `dewey_semantic_search` to find conceptually related content
>   across all indexed sources (specs, issues, docs)
> - `dewey_similar` to find documents similar to the one being
>   explored
> - `dewey_traverse` to follow relationships between related
>   documents
> - `dewey_semantic_search_filtered` to narrow searches by source
>   type (e.g., only GitHub issues, only web docs)
>
> Dewey provides cross-repo context that direct file reads cannot
> -- it finds related content even when different terminology is
> used.
>
> If Dewey is unavailable, fall back to direct file reads using
> the Read and Grep tools, and reference convention packs for
> standards.

**Where**: Near the beginning of the exploration workflow, as
an instruction for how to gather context.

### Step 4: Apply 3-Tier Dewey Degradation (Skills Only)

For each target skill file listed below, apply the 3-tier
degradation pattern. For each file:

1. **Read** the file content
2. **Check** if the degradation pattern is already present. Look
   for mentions of "Tier 1", "Tier 2", "Tier 3", "graceful
   degradation", "graph-only", or a structured fallback pattern
   involving Dewey availability.
3. **If already present**: Report `⊘ <filename>: already present (skipped)`
4. **If not present**: Find the appropriate location and insert.
   Report `✅ <filename>: inserted`

**Target files**:
- `.opencode/skills/openspec-propose/SKILL.md`
- `.opencode/skills/openspec-apply-change/SKILL.md`
- `.opencode/skills/openspec-explore/SKILL.md`

**What to insert**: A degradation section that describes behavior
at each tier:

> **Dewey Availability Tiers**
>
> Adjust context retrieval based on Dewey availability:
>
> **Tier 3 (Full Dewey)**: Use `dewey_semantic_search`,
> `dewey_search`, `dewey_traverse`, and
> `dewey_semantic_search_filtered` for comprehensive cross-repo
> and toolstack context.
>
> **Tier 2 (Graph-only, no embedding model)**: Use
> `dewey_search` and `dewey_traverse` for keyword-based and
> structural queries. Semantic search is unavailable.
>
> **Tier 1 (No Dewey)**: Fall back to direct file operations:
> - Use the Read tool to read local specs, backlog items, and
>   convention packs
> - Use the Grep tool for keyword search across the codebase
> - Reference `.opencode/uf/packs/` for coding standards
>
> All tiers produce valid results. Higher tiers provide richer
> cross-repo context but are never required.

**Where**: After the Dewey context retrieval section (Step 3
customization), or at the end of the file if no natural
insertion point exists. Do NOT insert this in command files --
skills only (commands delegate to skills for behavior).

### Step 5: Speckit Custom Commands

Create the 4 UF-custom speckit commands that upstream
`specify init` does not provide. For each file below:

1. **Check** if the file exists in `.opencode/commands/`
2. **If it exists**: Report `⊘ <filename>: already exists (skipped)`
3. **If it does not exist**: Create it with the content
   described below. Report `✅ <filename>: created`

#### speckit.analyze.md

Create `.opencode/commands/speckit.analyze.md` — a
read-only cross-artifact consistency and quality analysis
command. The command:
- Runs after `/speckit.tasks` produces `tasks.md`
- Loads spec.md, plan.md, tasks.md, and constitution
- Performs 6 detection passes: duplication, ambiguity,
  underspecification, constitution alignment, coverage
  gaps, and inconsistency
- Assigns severity (CRITICAL/HIGH/MEDIUM/LOW)
- Produces a Markdown analysis report (no file writes)
- Offers optional remediation suggestions

Use this frontmatter:
```yaml
---
description: Perform a non-destructive cross-artifact consistency and quality analysis across spec.md, plan.md, and tasks.md after task generation.
---
```

#### speckit.checklist.md

Create `.opencode/commands/speckit.checklist.md` — a
requirements quality validation command ("unit tests
for English"). The command:
- Generates checklists that test REQUIREMENTS quality,
  not implementation behavior
- Creates files in `FEATURE_DIR/checklists/[domain].md`
- Items use question format: "Are [X] defined for [Y]?"
- Items include quality dimension tags: [Completeness],
  [Clarity], [Consistency], [Measurability], [Coverage]
- Asks up to 3 clarifying questions before generating
- Each run creates a NEW checklist file (never overwrites)

Use this frontmatter:
```yaml
---
description: Generate a custom checklist for the current feature based on user requirements.
---
```

#### speckit.clarify.md

Create `.opencode/commands/speckit.clarify.md` — a spec
ambiguity detection and resolution command. The command:
- Scans spec.md for ambiguities across 10 taxonomy
  categories (functional scope, data model, UX flow,
  non-functional, integration, edge cases, constraints,
  terminology, completion signals, placeholders)
- Asks up to 5 targeted questions, one at a time
- Provides recommended answers with reasoning
- Integrates answers directly into spec.md sections
- Records Q&A in a `## Clarifications` section

Use this frontmatter:
```yaml
---
description: Identify underspecified areas in the current feature spec by asking up to 5 highly targeted clarification questions and encoding answers back into the spec.
---
```

#### speckit.taskstoissues.md

Create `.opencode/commands/speckit.taskstoissues.md` — a
GitHub issue creation command. The command:
- Reads tasks.md and creates GitHub issues for each task
- Requires a GitHub remote URL (validates before creating)
- Uses the GitHub MCP server for issue creation
- NEVER creates issues in repos that don't match the
  remote URL

Use this frontmatter:
```yaml
---
description: Convert existing tasks into actionable, dependency-ordered GitHub issues for the feature based on available design artifacts.
tools: ['github/github-mcp-server/issue_write']
---
```

All 4 commands MUST include the standard initialization
step: run `.specify/scripts/bash/check-prerequisites.sh
--json` from repo root and parse JSON for FEATURE_DIR.

### Step 6: Speckit Command Guardrails

Inject a `## Guardrails` section into ALL 9
`.opencode/commands/speckit.*.md` files. Use four
variants depending on the command type.

**Spec-phase commands** (get Guardrails WITH
review-rationale sentence):
- `speckit.specify.md`
- `speckit.clarify.md`
- `speckit.plan.md`
- `speckit.tasks.md`
- `speckit.analyze.md`
- `speckit.checklist.md`

**`speckit.implement.md`** (command-specific: implementation
guardrails — this command writes source code)

**`speckit.constitution.md`** (command-specific: constitution
guardrails — writes to `.specify/memory/` and templates)

**`speckit.taskstoissues.md`** (command-specific: issue creation
guardrails — creates GitHub issues via MCP API)

For each file:

1. **Read** the file content
2. **Check** if a `## Guardrails` section already exists
   (search for the heading text `## Guardrails` as a
   markdown heading outside of fenced code blocks)
3. **If NOT present**: Append the appropriate guardrails
   variant at the very end of the file. Report
   `✅ <filename>: guardrails injected`
4. **If already present — spec-phase commands**: Perform
   a secondary check — search for BOTH the phrase "review
   defeats the purpose" AND the UF-reference markers
   `dewey_semantic_search` and
   `.specify/memory/constitution.md`. If the Guardrails
   heading exists but ANY of these markers is missing,
   the block is stale (pre-reconciliation): replace the
   entire `## Guardrails` section (from the heading to the
   next `##` heading or end of file) with the current
   Spec-phase guardrails block below.
   Report `✅ <filename>: guardrails corrected`.
   If ALL markers are already present, report
   `⊘ <filename>: guardrails already present (skipped)`
5. **If already present — command-specific commands**
   (`implement`, `constitution`, `taskstoissues`):
   Check for ALL of the command's correctness markers.
   A block is correct only when EVERY marker is present:

   | Command | Correctness markers |
   |---------|--------------------|
   | `speckit.implement.md` | "writes source code" AND `dewey_semantic_search` AND `.specify/memory/constitution.md` AND `/uf.review-council` |
   | `speckit.constitution.md` | ".specify/memory/" AND `dewey_semantic_search` AND `.specify/memory/constitution.md` |
   | `speckit.taskstoissues.md` | "GitHub issues via" |

   If EVERY correctness marker IS present in the existing
   `## Guardrails` section, the guardrails are correct.
   Report
   `⊘ <filename>: guardrails already present (skipped)`

   If ANY correctness marker is ABSENT, the guardrails
   are incorrect (likely the old shared template or a
   pre-reconciliation block missing the UF references).
   Replace the entire `## Guardrails` section (from the
   heading to the next `##` heading or end of file) with
   the command-specific guardrail block. Report
   `✅ <filename>: guardrails corrected`

**Spec-phase guardrails block** (with review-rationale):

```markdown

## Guardrails

- **NEVER modify source code** — this command updates
  spec artifacts ONLY. Implementation changes belong in
  `/speckit.implement`, `/unleash`, or `/cobalt-crush`.
- **NEVER modify test files, Go source, Markdown agents,
  convention packs, or config files** outside the
  `specs/NNN-*/` feature directory.
- The ONLY files this command may write are:
  - `FEATURE_SPEC` (the spec.md file)
  - Files within `FEATURE_DIR` (spec artifacts:
    plan.md, tasks.md, research.md, data-model.md,
    quickstart.md, contracts/, checklists/)
- The user needs to review the plan before
  implementation begins. Implementing without review
  defeats the purpose of the spec-first workflow.
- Consult the project constitution
  (`.specify/memory/constitution.md`) and retrieve prior
  context via Dewey (`dewey_semantic_search`) before
  writing spec artifacts.
```

**Implement guardrails block** (`speckit.implement.md`):

```markdown

## Guardrails

- This command **writes source code** — implementation
  is its primary purpose. It executes the tasks defined
  in the active feature's `tasks.md`.
- Scope modifications to the active feature's
  implementation plan. Do not make changes unrelated to
  the current task group.
- Mark task checkboxes `[x]` as each task is completed.
- Consult the project constitution
  (`.specify/memory/constitution.md`) and retrieve prior
  context via Dewey (`dewey_semantic_search`) before
  writing code.
- Run `/uf.review-council` before completing
  implementation.
```

**Constitution guardrails block** (`speckit.constitution.md`):

```markdown

## Guardrails

- This command updates the project constitution and
  propagates changes to dependent templates.
- The ONLY files this command may write are:
  - `.specify/memory/constitution.md`
  - `.specify/templates/*-template.md` (consistency
    propagation)
- Do NOT modify source code, test files, or any files
  outside the `.specify/` directory.
- Consult the project constitution
  (`.specify/memory/constitution.md`) and retrieve prior
  context via Dewey (`dewey_semantic_search`) before
  amending governance documents.
```

**Taskstoissues guardrails block** (`speckit.taskstoissues.md`):

```markdown

## Guardrails

- This command creates **GitHub issues via** the MCP API.
  It does NOT write local files.
- Issues MUST only be created in the repository matching
  the current Git remote. NEVER create issues in
  unrelated repositories.
- Do NOT modify source code, spec artifacts, or any
  local files.
```

### Step 7: Speckit UF Customizations

Verify that UF-specific content is present in the
upstream speckit commands. For each of the 5 upstream
commands (`speckit.specify.md`, `speckit.plan.md`,
`speckit.tasks.md`, `speckit.implement.md`,
`speckit.constitution.md`):

1. **Read** the file content
2. **Check** for these UF-specific references:
   - Dewey integration: does the file mention
     `dewey_semantic_search` or `dewey_search` as tool
     invocations? (Not just prose mentions of "Dewey")
   - Constitution check: does the file reference
     `.specify/memory/constitution.md` or the
     Constitution Check gate?
   - Review council: does `speckit.implement.md`
     reference `/uf.review-council` or the Divisor review
     system?
3. **If all references present**: Report
   `⊘ <filename>: UF customizations present (skipped)`
4. **If any reference missing**: Report which references
   are missing but do NOT modify the file. These are
   informational — the upstream commands may not include
   UF-specific content, and that's acceptable.
   Report `ℹ <filename>: missing [list] (informational)`

This step is read-only — it verifies but does not modify.

### Step 8: OpenSpec Command Guardrails

For `.opencode/commands/opsx-propose.md`:

1. **Read** the file content
2. **Check** if a `## Guardrails` section exists at the
   end of the file (search for the heading text
   `## Guardrails`)
3. **If already present**: Report
   `⊘ opsx-propose.md: guardrails already present (skipped)`
4. **If not present**: Append the following block at the
   very end of the file. Report
   `✅ opsx-propose.md: guardrails injected`

The guardrails block to append:

```markdown

## Guardrails

- **NEVER implement code changes** — this command
  creates artifacts ONLY (proposal, design, specs,
  tasks)
- **NEVER commit, push, or create PRs** — those are
  /uf.finale's responsibility
- **NEVER run /uf.unleash, /opsx-apply, or /uf.cobalt-crush**
  — the user decides when to implement
- After artifacts are complete, STOP and prompt the
  user to run /uf.unleash, /opsx-apply, or /uf.cobalt-crush
```

### Step 9: Report Results

After processing all customizations, display a summary:

```
## /uf.init: Project Customizations

### Prerequisites
  ✅ .opencode/ exists
  ✅ OpenSpec skills found (N/4 files)
  ✅ OpenSpec commands found (N/3 files)

### Branch Enforcement
  [status] [filename]: [action]
  ...

### Dewey Context
  [status] [filename]: [action]
  ...

### 3-Tier Degradation (Skills only)
  [status] [filename]: [action]
  ...

### Speckit Custom Commands
  [status] [filename]: [action]
  ...

### Speckit Command Guardrails
  [status] [filename]: [action]
  ...

### Speckit UF Customizations
  [status] [filename]: [action]
  ...

### OpenSpec Command Guardrails
  [status] [filename]: [action]
  ...

### STOP HERE Blocks
  [status] [filename]: [action]
  ...

### Scaffold Comment Deduplication
  [status] [filename]: [action]
  ...

### Legacy Directory Cleanup
  [status] [item]: [action]
  ...

### Summary
Applied: N | Already present: N | Errors: N
```

Use these status indicators:
- `✅` -- customization was inserted
- `⊘` -- customization already present (skipped)
- `❌` -- file not found or error (with fix suggestion)

### Step 10: STOP HERE Blocks

Inject a STOP HERE block into each spec-phase speckit
command file. The block prevents premature advancement
to implementation by instructing the LLM to stop and
prompt the user.

**Target files** (spec-phase commands only):
- `speckit.specify.md`
- `speckit.plan.md`
- `speckit.tasks.md`
- `speckit.analyze.md`
- `speckit.checklist.md`
- `speckit.clarify.md`

**Excluded** (execution/utility commands -- no STOP HERE):
- `speckit.implement.md`
- `speckit.constitution.md`
- `speckit.taskstoissues.md`

For each target file:

1. **Read** the file content
2. **Check** if "STOP HERE" (case-sensitive) is already
   present in the file
3. **If already present**: Report
   `⊘ <filename>: STOP HERE already present (skipped)`
4. **If not present**: Insert the STOP HERE block. Report
   `✅ <filename>: STOP HERE inserted`

**What to insert**:

```markdown

**STOP HERE. Do NOT proceed to implementation.**

Your job is done. Report the results and prompt the
user. The user will invoke a separate command
(`/speckit.implement`, `/unleash`, or `/cobalt-crush`)
when they are ready to implement.
```

**Where**: Immediately after the `## Outline` heading
(or equivalent section heading), before the first
numbered workflow step. If no `## Outline` heading
exists, insert before the first numbered step in the
file.

### Step 11: Scaffold Comment Deduplication

Deduplicate scaffold comments in all files processed by
`/uf-init`. Repeated runs of `uf init` across versions
can accumulate multiple `<!-- scaffolded by uf ... -->`
comments in the same file.

**Target scope**: All files processed by `/uf-init`:
- The 4 OpenSpec skill files (Step 2-4 targets)
- The 3 OpenSpec command files (Step 2-3 targets)
- The 9 speckit command files (Step 5-6 targets)

For each file:

1. **Read** the file content
2. **Count** lines matching the pattern
   `<!-- scaffolded by uf` (any version string after "uf")
3. **If 0 or 1 matches**: No action needed. Report
   `⊘ <filename>: scaffold comments clean`
4. **If 2+ matches**: Keep only the LAST occurrence,
   remove all earlier occurrences. Report
   `✅ <filename>: deduplicated scaffold comments
   (N removed)`

**Important**: This step runs AFTER all other insertion
steps to catch any duplicates introduced by earlier steps.

### Step 12: Legacy Directory Cleanup

Clean up legacy directory artifacts from older versions
of `uf init`.

Set `LEGACY_PACKS = .opencode/` + `unbound/packs/`
(the pre-Spec-025 convention pack location).

#### Sub-task A: Legacy Packs Removal

1. Check if `LEGACY_PACKS` exists
2. **If it does NOT exist**: Report
   `⊘ legacy packs: not present`
3. **If it exists**: Verify pre-conditions:
   - `.opencode/uf/packs/default.md` MUST exist
   - `.opencode/uf/packs/severity.md` MUST exist
4. **If pre-conditions met**: Remove `LEGACY_PACKS`
   recursively (NOT its parent directory). Then check
   if the parent directory is empty -- if so, remove it
   too. If the parent still contains other files or
   directories, leave it and report a warning:
   `⚠️ legacy parent dir: packs/ removed but directory
   contains other content -- leaving in place`.
   Report `✅ legacy packs: removed (migrated to
   uf/packs/)`
5. **If pre-conditions NOT met**: Report
   `❌ legacy packs: uf/packs/ missing core files,
   skipped`

#### Sub-task B: `command/` Migration Hardening

After Step 0 runs (command directory migration), verify
the migration was effective:

1. Check if any `speckit.*.md` files remain in
   `.opencode/command/` (singular)
2. **If found**: Report
   `⚠️ command/: speckit commands still in singular
   directory after migration -- check Step 0 output`
3. **If not found**: Report
   `⊘ command/: migration verified (or not needed)`

### Post-Write Verification

After all customizations are applied, for each file that was
modified (had at least one `✅` insertion):

1. **Re-read** the file
2. **Verify** the inserted content is present (search for the
   key phrases from the insertion)
3. **Verify** no existing content was accidentally removed
   (the file should be longer than before, not shorter)
4. If verification fails, report: `⚠️ <filename>: verification
   failed -- review with git diff`

Finally, remind the user:
> Run `git diff` to review all changes before committing.

### Next Steps

After customizations are applied:

- Run `/uf.unleash` for autonomous pipeline execution
  (parallel swarm, recommended for multi-task changes)
- Run `/uf.cobalt-crush` to start implementing — it
  auto-detects your active workflow (Speckit or OpenSpec)
  and delegates to the correct implementation command.
  Preferred over calling `/opsx-apply` directly.

### Tool Delegation (Spec 027)

As of Spec 027, `uf init` delegates workspace initialization
to external CLIs when they are available:

- **Speckit**: `.specify/` is created by `specify init` (not
  embedded). If `specify` is in PATH and `.specify/` does not
  exist, `uf init` calls `specify init` automatically.
  Post-init customization of Speckit scripts/templates is
  handled by the `specify` CLI itself.

- **OpenSpec**: `openspec/config.yaml` and base structure are
  created by `openspec init --tools opencode` (not embedded).
  The custom OpenSpec schema (`openspec/schemas/unbound-force/`)
  is still deployed from embedded assets. If `openspec` is in
  PATH and `openspec/config.yaml` does not exist, `uf init`
  calls `openspec init --tools opencode` automatically.

- **Gaze**: Gaze agent files (e.g., `gaze-reporter.md`) are
  created by `gaze init` (not embedded). If `gaze` is in PATH
  and `.opencode/agents/gaze-reporter.md` does not exist,
  `uf init` calls `gaze init` automatically.

All delegations are optional — if a tool is not installed,
`uf init` skips its delegation silently (Constitution
Principle II — Composability First).

### When to Re-run

Re-run `/uf.init` after:
- Running `uf init` or `uf setup` (new tool versions
  may reset third-party files)
- Updating the OpenSpec CLI (`npm update`)
- Upgrading the `uf` binary (`brew upgrade unbound-force`,
   or on Fedora/RHEL: `sudo dnf upgrade unbound-force`
   — new versions may add scaffold files that need
   customization)
