---
tag: agent-orchestration
author: jay-flowers
category: pattern
created_at: 2026-11-10T16:27:55Z
identity: agent-orchestration-20261110T162755-jay-flowers
tier: draft
---

The uf scaffold's delegation-timeout design (dewey PR #167 refresh) treats a delegated child waiting at a human question gate as a stall to terminate, not a wait to honor: the executor's idle watchdog aborts a child whose session-subtree fingerprint is unchanged for the idle window even when a question tool call is pending, tags the result with timeout.pending_question: true, and the parent command recovers by re-presenting the gate itself (uf.run-pipeline-plan "Delegation timeout recovery"; Delegation 2 deliberately uses idle_timeout: 300000 to end the wait at the commit-approval gate). The abort is made recoverable rather than lossy by pairing it with durable resume state — readDelegateResumeState reads the delegated command's progress checkpoints (state key, last_checkpoint_step, completed_steps) so a re-delegation resumes instead of restarting. Net pattern: never let a child session burn wall-clock budget while blocked on a human; kill it, flag the pending question, and recover the gate plus checkpoint state at the parent.
