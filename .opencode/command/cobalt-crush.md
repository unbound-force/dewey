---
description: >
  Invoke the Cobalt-Crush developer persona for implementation tasks.
  With arguments: delegates to the cobalt-crush-dev agent. Without
  arguments: detects active workflow and runs /speckit.implement or
  /opsx:apply.
---

# Command: /cobalt-crush

## Description

Invoke the Cobalt-Crush developer persona to implement code with
clean code principles, convention pack adherence, and feedback loop
integration.

## User Input

```text
$ARGUMENTS
```

## Usage

```
/cobalt-crush [task description or instructions]
/cobalt-crush                    # auto-detect workflow and implement
```

### Examples

```
/cobalt-crush implement T015 from tasks.md
/cobalt-crush fix the failing test in internal/metrics/store_test.go
/cobalt-crush refactor runMetrics to reduce complexity
/cobalt-crush                    # detects speckit or openspec context
```

## Instructions

### When arguments are provided

Delegate to the `cobalt-crush-dev` agent using the Task tool with
`subagent_type: "cobalt-crush-dev"`. Pass the user's arguments as
the task prompt. The agent will:

1. Read AGENTS.md, constitution, convention packs, and active specs
2. Apply its engineering philosophy (clean code, SOLID, TDD awareness)
3. Load convention packs from `.opencode/unbound/packs/`
4. Check `.unbound-force/artifacts/` for Gaze/Divisor feedback
5. Execute the requested task following project conventions
6. Document design decisions in code comments

### When no arguments are provided

Detect the active workflow and delegate the corresponding
implementation command to the `cobalt-crush-dev` agent:

1. **Check for a Speckit feature branch**: Run
   `.specify/scripts/bash/check-prerequisites.sh --json --paths-only`
   from the repo root. If it succeeds and returns a `FEATURE_DIR`
   with a `tasks.md`, this is a Speckit strategic workflow.
   Read the full contents of `.opencode/command/speckit.implement.md`
   and delegate it to the `cobalt-crush-dev` agent via the Task
   tool — pass the command file's instructions as the agent's
   prompt so the agent executes the implementation workflow.

2. **Check for an OpenSpec active change**: Look for directories
   under `openspec/changes/` that contain a `tasks.md` file. If
   one exists, this is an OpenSpec tactical workflow.
   Read the full contents of `.opencode/command/opsx-apply.md`
   and delegate it to the `cobalt-crush-dev` agent via the Task
   tool — pass the command file's instructions as the agent's
   prompt so the agent executes the apply workflow.

3. **If neither is detected**: Ask the user which workflow to run:

   > No active implementation context detected. Which workflow
   > should I execute?
   >
   > - `/speckit.implement` — Strategic spec implementation
   >   (requires a feature branch with `specs/NNN-*/tasks.md`)
   > - `/opsx:apply` — Tactical change implementation
   >   (requires an active change in `openspec/changes/`)
