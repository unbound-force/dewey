---
description: >
  Apply Unbound Force project customizations to third-party tool
  files (OpenSpec skills and commands). Uses LLM reasoning to find
  correct insertion points. Run after uf init, uf setup, or
  updating the OpenSpec CLI.
---
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vv0.6.1 -->

# Command: /uf-init

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
   - `.opencode/command/opsx-propose.md`
   - `.opencode/command/opsx-apply.md`
   - `.opencode/command/opsx-archive.md`

For each missing file, report an error:
> `❌ <path>: file not found`
> This file should have been created by `openspec init` which
> runs as part of `uf init`. Run `uf setup` to install OpenSpec,
> then `uf init` to scaffold files, then re-run `/uf-init`.

Continue checking remaining files even if some are missing.
**Track which files are missing** -- in Steps 2-4, skip any
file that was reported missing here. Report
`❌ <filename>: skipped (file not found in prerequisites)`.

**Recovery note**: All target files are git-tracked. If any
insertion looks wrong after running this command, restore with
`git checkout -- <path>`. Run `git diff` after `/uf-init`
completes to review all changes before committing.

### Step 2: Apply Branch Enforcement

For each target file listed below, apply the branch enforcement
customization. For each file:

1. **Read** the file content
2. **Check** if branch enforcement is already present. Look for
   the concept semantically -- does the file already describe
   creating, validating, or cleaning up an `opsx/<name>` branch?
   Check for phrases like `git checkout -b opsx/`, `opsx/<name>`,
   `opsx/<change-name>`, or equivalent branch management
   instructions.
3. **If already present**: Report `⊘ <filename>: already present (skipped)`
4. **If not present**: Read the file structure, find the correct
   insertion point, and insert the customization. Report
   `✅ <filename>: inserted`

#### Branch Enforcement: Propose (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-propose/SKILL.md`
- `.opencode/command/opsx-propose.md`

**What to insert**: After the step that creates the change
directory (`openspec new change "<name>"`), insert a new step:

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

#### Branch Enforcement: Apply (Skills + Commands)

**Target files**:
- `.opencode/skills/openspec-apply-change/SKILL.md`
- `.opencode/command/opsx-apply.md`

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
- `.opencode/command/opsx-archive.md`

**What to insert**: After the archive move completes, insert a
branch cleanup step:

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

**Note**: `openspec-explore` and `opsx-explore.md` are
intentionally excluded from branch enforcement -- explore mode
does not create or modify changes, so branch management does
not apply.

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
- `.opencode/command/opsx-propose.md`

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
- `.opencode/command/opsx-apply.md`

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
> - Reference `.opencode/unbound/packs/` for coding standards
>
> All tiers produce valid results. Higher tiers provide richer
> cross-repo context but are never required.

**Where**: After the Dewey context retrieval section (Step 3
customization), or at the end of the file if no natural
insertion point exists. Do NOT insert this in command files --
skills only (commands delegate to skills for behavior).

### Step 5: Report Results

After processing all customizations, display a summary:

```
## /uf-init: Project Customizations

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

### Summary
Applied: N | Already present: N | Errors: N
```

Use these status indicators:
- `✅` -- customization was inserted
- `⊘` -- customization already present (skipped)
- `❌` -- file not found or error (with fix suggestion)

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
