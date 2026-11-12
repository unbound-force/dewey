---
description: >
  Synthesize stored learnings into compiled knowledge articles.
  Groups learnings by topic and produces current-state articles
  with per-claim citations and history. Supersession is explicit:
  synthesis presents unresolved contradictions with both positions
  and proposes supersessions for human confirmation.
---

# Command: /dewey-compile

## Description

Synthesize stored learnings into compiled knowledge articles.
Groups learnings by topic and produces current-state articles with
per-claim citations and history. Supersession is explicit: synthesis
presents unresolved contradictions with both positions and proposes
supersessions for human confirmation.

## Usage

```
/dewey-compile
/dewey-compile --incremental authentication-3
```

<protect>
## Instructions

Call the Dewey MCP tool `compile` to synthesize stored learnings.

If `--incremental` is specified with learning identities, pass them
to compile only the affected topic clusters.

If no arguments, run a full compilation of all learnings.

Report the per-article citation coverage and any proposed
supersessions the result carries. Proposed pairs are confirmed by a
human through the supersede_learning tool — never applied
automatically.

Display the returned summary showing topics compiled, articles
generated, and elapsed time.
</protect>
