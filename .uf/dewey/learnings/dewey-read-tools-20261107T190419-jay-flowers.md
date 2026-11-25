---
tag: dewey-read-tools
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:04:19Z
identity: dewey-read-tools-20261107T190419-jay-flowers
tier: draft
---

dewey bare-name read resolution (read-tools-bare-names, dewey PR #163 closing #140): every name-based read through the vault client — get_page, get_links forward direction, traverse start pages, get_whiteboard, bulk_update_properties reads — resolves a slash-free name as exact (case-folded) → compiled/<name> → learning/<name>, first match wins, with vault files taking precedence. Consumption gotchas: get_links and traverse echo the REQUESTED bare name in their results (page: "gateway"; traverse paths seeded
