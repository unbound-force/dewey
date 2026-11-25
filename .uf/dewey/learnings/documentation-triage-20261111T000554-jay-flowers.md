---
tag: documentation-triage
author: jay-flowers
category: pattern
created_at: 2026-11-11T00:05:54Z
identity: documentation-triage-20261111T000554-jay-flowers
tier: draft
---

For documentation-triage reviews of a spec that adds user-facing surfaces, verify the implementation plan names every doc surface the change makes stale — not just the obvious release-notes and CLI-reference targets. Two surfaces are routinely missed: (1) parallel enumerated lists (this repo keeps a cli.go subcommand list in both AGENTS.md and README.md, so an update planned for one is easily omitted for the other), and (2) existing open website issues for the same command family (a docs issue, a tutorial issue, and a blog issue can all exist separately — extending the docs issue does not keep the tutorial accurate when the workflow's opening steps change). Also check whether an architecture tree enumerates root files at all before demanding new root-file entries: Dewey's trees list main.go/cli.go/server.go plus package directories and have never listed slash_commands.go, so root-file omissions are not the established convention while package-directory additions are.
