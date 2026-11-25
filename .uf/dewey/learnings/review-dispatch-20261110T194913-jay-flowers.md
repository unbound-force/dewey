---
tag: review-dispatch
author: jay-flowers
category: pattern
created_at: 2026-11-10T19:49:13Z
identity: review-dispatch-20261110T194913-jay-flowers
tier: draft
---

Scaffold-refresh architectural review of the reply-tools deployment (dewey PR #177) extends the cross-surface coherence check to three new tool pairings, all verified matching: (1) checkout_pr_head — command error-code list (DIRTY_TREE, CHECKOUT_FAILED, UNRELATED_BRANCH, READ_ONLY_MERGE_REF, NO_PUSH_TARGET, UNSAFE_PUSH_TARGET) matches CHECKOUT_PR_HEAD_ERROR_CODES, the success-JSON example matches CheckoutPrHeadData {branch, head_remote, merge_ref, ahead_of_head_sha, checked_out}, and args {pr_number, head_ref, head_sha} match the zod schema; (2) post_feedback_replies — the decision enum accept|modify|reject|ask|file|reply matches REPLY_DECISIONS, the args/items shape matches PostFeedbackRepliesInputSchema, the "Filed as #<n>: " prefix prose matches renderReplyBody, both summary-line examples in the command reconstruct exactly from renderReplySummary (including the "Items N, M not posted" and "Re-run `/uf.address-feedback <N>` to retry" fragments), and the "reply-comments" checkpoint recorded-only-when-complete prose matches REPLY_CHECKPOINT_STEP gating; (3) file_feedback_issue — prose args/returns match FileFeedbackIssueInputSchema/FileFeedbackIssueData. Also verified: rule 9's cache 600/700 plus ".uf/feedback/ in .gitignore" matches the root .gitignore, and the tool-owned "replies" state key namespaces cleanly against the command's Section 2.7 assessment writes to the same state.json.
