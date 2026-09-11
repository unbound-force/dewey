---
name: always-on-guidance
description: Global coding rules and tool usage discipline
tags: [always-on, coding, quality]
---
<!-- scaffolded by uf v0.17.0 -->

# Always-On Guidance

Rules that apply to every coding session.

## Tool Usage Discipline

- Read files before editing — never guess at content
- Use `replicator_org_*` tools for work item management
- Use `replicator_comms_*` tools for agent messaging and file reservations
- Use `replicator_forge_*` tools for multi-agent coordination
- Use `dewey_store_learning` / `dewey_semantic_search` for learning storage and retrieval
- Check `dewey_semantic_search` before solving problems from scratch

## Code Quality

- Functions do one thing well
- Names reveal intent — no abbreviations
- Comments explain *why*, not *what*
- No dead code or unused imports
- Error messages include context

## Testing

- Write tests for all new code
- Use `db.OpenMemory()` for database tests
- Use `t.TempDir()` for filesystem tests
- Standard library `testing` package only — no testify
- Test names: `TestXxx_Description`

## Error Handling

- Return errors, don't panic
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Handle all error paths — no ignored returns
- Use `errors.Is` for sentinel error checks

## Command Template Fidelity

- When a command template prescribes a fixed exit or
  output format, re-read that section from the template
  file before emitting — especially after context
  compression
- After compression, the template file is the sole
  authority for prescribed output format — never
  reconstruct it from memory or compressed summaries

## Git Discipline

- Conventional commits: `type: description`
- Never force push to main
- Commit early, commit often
