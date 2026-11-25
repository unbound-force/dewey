---
tag: go-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:08:11Z
identity: go-testing-20261111T020811-jay-flowers
tier: draft
---

When extracting a frontmatter splitter, adding a leading-delimiter HasPrefix check silently makes a preceding BOM-strip load-bearing: the old SplitN-based parse accepted BOM-prefixed files by accident, but the new prefix check rejects them unless TrimPrefix("\\ufeff") runs first. The coupled pair must ship with a coupled test (BOM + frontmatter parses; BOM + no frontmatter errors), or a later 'redundant TrimPrefix' cleanup regresses BOM files with no failing test. Seen in dewey PR #187's splitFrontmatter extraction (main.go), consumed by both the curator gate's definition check and learning re-ingestion.
