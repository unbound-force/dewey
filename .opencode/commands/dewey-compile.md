---
description: >
  Synthesize stored learnings into compiled knowledge articles.
  Groups learnings by topic, resolves contradictions temporally,
  and produces current-state articles with history.
---

# Command: /dewey-compile

## Description

Synthesize stored learnings into compiled knowledge articles.
Groups learnings by topic, resolves contradictions temporally,
and produces current-state articles with history.

## Usage

```
/dewey-compile
/dewey-compile --incremental authentication-3
```

## Instructions

Call the Dewey MCP tool `compile` to synthesize stored learnings.

If `--incremental` is specified with learning identities, pass them
to compile only the affected topic clusters.

If no arguments, run a full compilation of all learnings.

Display the returned summary showing topics compiled, articles
generated, and elapsed time.
