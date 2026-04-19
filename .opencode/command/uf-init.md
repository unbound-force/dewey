---
description: >
  Apply Unbound Force project customizations to third-party tool
  files (OpenSpec skills and commands). Uses LLM reasoning to find
  correct insertion points. Run after uf init, uf setup, or
  updating the OpenSpec CLI.
---
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vdev -->
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

1. **Check** if the file exists in `.opencode/command/`
2. **If it exists**: Report `⊘ <filename>: already exists (skipped)`
3. **If it does not exist**: Create it with the content
   described below. Report `✅ <filename>: created`

#### speckit.analyze.md

Create `.opencode/command/speckit.analyze.md` — a
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

Create `.opencode/command/speckit.checklist.md` — a
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

Create `.opencode/command/speckit.clarify.md` — a spec
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

Create `.opencode/command/speckit.taskstoissues.md` — a
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
`.opencode/command/speckit.*.md` files. For each file:

1. **Read** the file content
2. **Check** if a `## Guardrails` section already exists
   (search for the heading text `## Guardrails`)
3. **If already present**: Report
   `⊘ <filename>: guardrails already present (skipped)`
4. **If not present**: Append the following block at the
   very end of the file. Report
   `✅ <filename>: guardrails injected`

The guardrails block to append:

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
```

The 9 target files are:
- `speckit.specify.md`
- `speckit.clarify.md`
- `speckit.plan.md`
- `speckit.tasks.md`
- `speckit.analyze.md`
- `speckit.checklist.md`
- `speckit.implement.md`
- `speckit.constitution.md`
- `speckit.taskstoissues.md`

**Note**: `speckit.implement.md` is an exception — it IS
allowed to modify source code. However, the guardrails
section is still injected for consistency. The implement
command's own instructions override the guardrails where
they conflict (implement's instructions explicitly say
to write source code).

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
     reference `/review-council` or the Divisor review
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

For `.opencode/command/opsx-propose.md`:

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
  /finale's responsibility
- **NEVER run /unleash, /opsx-apply, or /cobalt-crush**
  — the user decides when to implement
- After artifacts are complete, STOP and prompt the
  user to run /unleash, /opsx-apply, or /cobalt-crush
```

### Step 9: AGENTS.md Behavioral Guidance

For the repo's `AGENTS.md` file, inject standardized
behavioral guidance sections if not already present.

1. **Check** if `AGENTS.md` exists at the repo root.
   If not, report `⊘ AGENTS.md: not found (skipped)`
   and skip this entire step.

2. **Read** the full contents of `AGENTS.md`.

3. For each of the 8 guidance blocks below, in the order
   listed (Core Mission first, Knowledge Retrieval last):
   a. Check if the detection phrase (or semantic
      equivalent heading) exists in the file
   b. If present: report `⊘ <block>: already present
      (skipped)`
   c. If not present: find the appropriate insertion
      point per the placement guidance and append the
      block text. Report `✅ <block>: injected`

4. After processing all 8 blocks, save the file once.

**Injection order** (optimized for document flow):
1. Core Mission
2. Gatekeeping Value Protection
3. Workflow Phase Boundaries
4. CI Parity Gate
5. Review Council PR Prerequisite
6. Spec-First Development
7. Website Documentation Sync Gate
8. Knowledge Retrieval

#### Block 1: Core Mission

**Detection phrases**: `## Core Mission`, or both
`Strategic Architecture` AND `Outcome Orientation`
present in the same section.

**Placement**: After `## Project Overview`, before
`## Behavioral Constraints`. If neither heading exists,
append near the top of the file after any frontmatter
and title.

**Text to inject**:

```markdown
## Core Mission

- **Strategic Architecture**: Engineers shift from manual
  coding to directing an "infinite supply of junior
  developers" (AI agents).
- **Outcome Orientation**: Focus on conveying business
  value and user intent rather than low-level technical
  sub-tasks.
- **Intent-to-Context**: Treat specs and rules as the
  medium through which human intent is manifested into
  code.
```

#### Block 2: Gatekeeping Value Protection

**Detection phrases**: `Gatekeeping Value Protection`
heading, or `MUST NOT modify values that serve as
quality`.

**Placement**: Inside `## Behavioral Constraints`
section. If `## Behavioral Constraints` does not exist,
create it and place it after `## Core Mission` (or after
`## Project Overview` if Core Mission is absent).

**Text to inject**:

```markdown
### Gatekeeping Value Protection

Agents MUST NOT modify values that serve as quality or
governance gates to make an implementation pass. The
following categories are protected:

1. **Coverage thresholds and CRAP scores** — minimum
   coverage percentages, CRAP score limits, coverage
   ratchets
2. **Severity definitions and auto-fix policies** —
   CRITICAL/HIGH/MEDIUM/LOW boundaries, auto-fix
   eligibility rules
3. **Convention pack rule classifications** —
   MUST/SHOULD/MAY designations on convention pack rules
   (downgrading MUST to SHOULD is prohibited)
4. **CI flags and linter configuration** — `-race`,
   `-count=1`, `govulncheck`, `golangci-lint` rules,
   pinned action SHAs
5. **Agent temperature and tool-access settings** —
   frontmatter `temperature`, `tools.write`, `tools.edit`,
   `tools.bash` restrictions
6. **Constitution MUST rules** — any MUST rule in
   `.specify/memory/constitution.md` or hero constitutions
7. **Review iteration limits and worker concurrency** —
   max review iterations, max concurrent Swarm workers,
   retry limits
8. **Workflow gate markers** — `<!-- spec-review: passed
   -->`, task completion checkboxes used as gates, phase
   checkpoint requirements

**What to do instead**: When an implementation cannot
meet a gate, the agent MUST stop, report which gate is
blocking and why, and let the human decide whether to
adjust the gate or rework the implementation. Modifying
a gate without explicit human authorization is a
constitution violation (CRITICAL severity).
```

#### Block 3: Workflow Phase Boundaries

**Detection phrases**: `Workflow Phase Boundaries`
heading, or `MUST NOT cross workflow phase boundaries`.

**Placement**: Inside `## Behavioral Constraints`,
after Gatekeeping Value Protection (if present). If
`## Behavioral Constraints` does not exist, create it.

**Text to inject**:

```markdown
### Workflow Phase Boundaries

Agents MUST NOT cross workflow phase boundaries:

- **Specify/Clarify/Plan/Tasks/Analyze/Checklist** phases:
  spec artifacts ONLY (`specs/NNN-*/` directory). No
  source code, test, agent, command, or config changes.
- **Implement** phase: source code changes allowed,
  guided by spec artifacts.
- **Review** phase: findings and minor fixes only. No new
  features.

A phase boundary violation is treated as a process error.
The agent MUST stop and report the violation rather than
proceeding with out-of-phase changes.
```

#### Block 4: CI Parity Gate

**Detection phrases**: `CI Parity Gate` heading or bold
text, or `replicate the CI checks locally`.

**Placement**: Inside `## Behavioral Constraints` or
`## Technical Guardrails`, after Workflow Phase
Boundaries (if present). If neither section exists,
create `## Behavioral Constraints` and place it there.

**Text to inject**:

```markdown
### CI Parity Gate

Before marking any implementation task complete or
declaring a PR ready, agents MUST replicate the CI checks
locally. Read `.github/workflows/` to identify the exact
commands CI runs, then execute those same commands. Any
failure is a blocking error — a task is not complete
until all CI-equivalent checks pass locally. Do not rely
on a memorized list of commands; always derive them from
the workflow files, which are the source of truth.
```

#### Block 5: Review Council PR Prerequisite

**Detection phrases**: Both `Review Council` AND
`PR Prerequisite` present, or `/review-council` as a
command reference in a PR workflow context.

**Placement**: After behavioral constraints, before
build commands or testing conventions. If no clear
anchor exists, append after the last behavioral
constraint block.

**Text to inject**:

```markdown
### Review Council as PR Prerequisite

Before submitting a pull request, agents **must** run
`/review-council` and resolve all REQUEST CHANGES
findings until all reviewers return APPROVE. There must
be **minimal to no code changes** between the council's
APPROVE verdict and the PR submission — the council
reviews the final code, not a draft that changes
afterward.

Workflow:

1. Complete all implementation tasks
2. Run CI checks locally (build, test, vet)
3. Run `/review-council` — fix any findings, re-run
   until APPROVE
4. Commit, push, and submit PR immediately after council
   APPROVE
5. Do NOT make further code changes between APPROVE and
   PR submission

Exempt from council review:

- Constitution amendments (governance documents, not code)
- Documentation-only changes (README, AGENTS.md, spec
  artifacts)
- Emergency hotfixes (must be retroactively reviewed)
```

#### Block 6: Spec-First Development

**Detection phrases**: `Spec-First Development` heading,
or `preceded by a spec workflow`.

**Placement**: After behavioral constraints, before
build commands. If no clear anchor exists, append after
the last behavioral constraint or workflow rule block.

**Text to inject**:

```markdown
## Spec-First Development

All changes that modify production code, test code, agent
prompts, embedded assets, or CI configuration **must** be
preceded by a spec workflow. The constitution
(`.specify/memory/constitution.md`) is the highest-
authority document in this project — all work must align
with it.

Two spec workflows are available:

| Workflow | Location | Best For |
|----------|----------|----------|
| **Speckit** | `specs/NNN-name/` | Numbered feature specs with the full pipeline |
| **OpenSpec** | `openspec/changes/name/` | Targeted changes with lightweight artifacts |

**What requires a spec** (no exceptions without explicit
user override):

- New features or capabilities
- Refactoring that changes function signatures, extracts
  helpers, or moves code between packages
- Test additions or assertion strengthening across
  multiple functions
- Agent prompt changes
- CI workflow modifications
- Data model changes (new struct fields, schema updates)

**What is exempt** (may be done directly):

- Constitution amendments (governed by the constitution's
  own Governance section)
- Typo corrections, comment-only changes, single-line
  formatting fixes
- Emergency hotfixes for critical production bugs (must
  be retroactively documented)

When an agent is unsure whether a change is trivial, it
**must** ask the user rather than proceeding without a
spec. The cost of an unnecessary spec is minutes; the
cost of an unplanned change is rework, drift, and broken
CI.
```

#### Block 7: Website Documentation Sync Gate

**Detection phrases**: `Website Documentation` AND
`Gate` in a heading, or `gh issue create --repo` with
`unbound-force/website`.

**Placement**: Near documentation validation gate or
spec commit gate. If no clear anchor exists, append
after Spec-First Development.

**Text to inject**:

````markdown
### Website Documentation Gate

When a change affects user-facing behavior, hero
capabilities, CLI commands, or workflows, a GitHub issue
**MUST** be created in the `unbound-force/website`
repository to track required documentation or website
updates. The issue must be created before the
implementing PR is merged.

```bash
gh issue create --repo unbound-force/website \
  --title "docs: <brief description of what changed>" \
  --body "<what changed, why it matters, which pages
          need updating>"
```

**Exempt changes** (no website issue needed):
- Internal refactoring with no user-facing behavior
  change
- Test-only changes
- CI/CD pipeline changes
- Spec artifacts (specs are internal planning documents)

**Examples requiring a website issue**:
- New CLI command or flag added
- Hero capabilities changed (new agent, removed feature)
- Installation steps changed (`uf setup` flow)
- New convention pack added
- Breaking changes to any user-facing workflow
````

#### Block 8: Knowledge Retrieval

**Detection phrases**: `## Knowledge Retrieval` heading,
or `dewey_semantic_search` as a tool reference (in a
table or code block, not just prose), or
`Tool Selection Matrix`.

**Placement**: After coding conventions, before testing
conventions. If neither section exists, append near the
end of the file before any appendix or changelog.

**Text to inject**:

```markdown
## Knowledge Retrieval

Agents SHOULD prefer Dewey MCP tools over grep/glob/read
for cross-repo context, design decisions, and
architectural patterns. Dewey provides semantic search
across all indexed Markdown files, specs, and web
documentation — returning ranked results with provenance
metadata that grep cannot match.

### Tool Selection Matrix

| Query Intent | Dewey Tool | When to Use |
|-------------|-----------|-------------|
| Conceptual understanding | `dewey_semantic_search` | "How does X work?" |
| Keyword lookup | `dewey_search` | Known terms, FR numbers |
| Read specific page | `dewey_get_page` | Known document path |
| Relationship discovery | `dewey_find_connections` | "How are X and Y related?" |
| Similar documents | `dewey_similar` | "Find specs like this one" |
| Tag-based discovery | `dewey_find_by_tag` | "All pages tagged #decision" |
| Property queries | `dewey_query_properties` | "All specs with status: draft" |
| Filtered semantic | `dewey_semantic_search_filtered` | Semantic search within source type |
| Graph navigation | `dewey_traverse` | Dependency chain walking |

### When to Fall Back to grep/glob/read

Use direct file operations instead of Dewey when:
- **Dewey is unavailable** — MCP tools return errors or
  are not configured
- **Exact string matching is needed** — searching for a
  specific error message, variable name, or code pattern
- **Specific file path is known** — reading a file you
  already know the path to (use Read directly)
- **Binary/non-Markdown content** — Dewey indexes
  Markdown; use grep for Go source, JSON, YAML, etc.

### Graceful Degradation (3-Tier Pattern)

**Tier 3 (Full Dewey)** — semantic + structured search:
- `dewey_semantic_search` — natural language queries
- `dewey_search` — keyword queries
- `dewey_get_page`, `dewey_find_connections`,
  `dewey_traverse` — structured navigation
- `dewey_find_by_tag`, `dewey_query_properties` —
  metadata queries

**Tier 2 (Graph-only, no embedding model)** — structured
search only:
- `dewey_search` — keyword queries (no embeddings needed)
- `dewey_get_page`, `dewey_traverse`,
  `dewey_find_connections` — graph navigation
- `dewey_find_by_tag`, `dewey_query_properties` —
  metadata queries
- Semantic search unavailable — use exact keyword matches

**Tier 1 (No Dewey)** — direct file access:
- Use Read tool for direct file access
- Use Grep for keyword search across the codebase
- Use Glob for file pattern matching
```

### Step 10: Report Results

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

### AGENTS.md Guidance
  [status] Core Mission: [action]
  [status] Gatekeeping Value Protection: [action]
  [status] Workflow Phase Boundaries: [action]
  [status] CI Parity Gate: [action]
   [status] Review Council PR Prerequisite: [action]
   [status] Spec-First Development: [action]
   [status] Website Documentation Sync Gate: [action]
  [status] Knowledge Retrieval: [action]

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

### Next Steps

After customizations are applied:

- Run `/unleash` for autonomous pipeline execution
  (parallel swarm, recommended for multi-task changes)
- Run `/cobalt-crush` to start implementing — it
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

Re-run `/uf-init` after:
- Running `uf init` or `uf setup` (new tool versions
  may reset third-party files)
- Updating the OpenSpec CLI (`npm update`)
- Upgrading the `uf` binary (`brew upgrade unbound-force`
  — new versions may add scaffold files that need
  customization)
