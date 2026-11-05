---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T18:03:44Z
identity: test-quality-20261105T180344-jay-flowers
tier: draft
---

`git rev-parse --show-toplevel` walks parent directories for .git, so a test asserting "not a git repository" against t.TempDir() really asserts that the runner's TMPDIR is outside every repository. On standard macOS/Linux and CI runners this holds, but a developer with TMPDIR inside a workspace gets a false failure that looks like a code defect. Prefer skip-with-diagnosis over failure for negative git assertions that depend on ambient directory placement.
