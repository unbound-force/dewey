---
description: End a session cleanly - release reservations, sync state, generate handoff note
---

<protect>
# /handoff

Wrap up a session cleanly.

## Workflow

Steps MUST execute in this exact order -- do not reorder
or parallelize.

0. **Forge precondition check** -- SHOULD check for
   active forge workers before proceeding. If workers
   are active, warn the user and request confirmation
   before releasing reservations. If forge tools are
   unavailable or you have no active forge context
   (no known epic_id/project_key), skip this check.

1. **Summarize completed work and open blockers** --
   do this first so you have full awareness of session
   state before releasing anything.

2. `comms_release_all()` -- free all file reservations.
   Depends on step 1: you need the summary to know what
   was reserved and whether any reservations are still
   needed.

3. `org_update()` / `org_close()` -- update cell
   statuses. Depends on step 2: reservations must be
   released before updating cell state to avoid stale
   lock references.

4. `org_sync()` -- persist state to git.
   Depends on step 3: cell statuses must be final
   before syncing to avoid persisting intermediate
   state.

5. `org_session_end(handoff_notes="...")` -- save
   handoff for next session. Depends on step 4: sync
   must complete before ending the session so the
   handoff reflects the final persisted state. Structure
   your handoff notes as follows:

   ```
   - Completed: What tasks were finished
   - In Progress: What was started but not finished
   - Blocked: What is waiting on external input
   - Next Steps: What the next agent should do first
   - Gotchas: Any surprises or edge cases discovered
   ```
</protect>
