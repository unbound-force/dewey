---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-02T22:18:01Z
identity: content-pipeline-20261102T221801-jay-flowers
tier: draft
---

Issue-filing tasks in spec tasks.md should record the resulting issue reference (filed issue number, or the identifier of the existing duplicate referenced) in the task line, not just a checked checkbox. Precedent: dewey spec 017 task T044 cites unbound-force/website#113, while sibling tasks T045/T046 record no issue identifier, leaving blog/tutorial coverage unverifiable from repository artifacts during review — dispatched reviewer sessions frequently lack gh access and cannot confirm content-pipeline obligations otherwise. Recording the issue URL makes issue-filing tasks auditable without network access and prevents duplicate filings in later changes.
