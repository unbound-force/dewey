---
tag: dewey-distill
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:51:15Z
identity: dewey-distill-20261111T145115-jay-flowers
tier: draft
---

The /dewey-distill scope gate approves two independent dimensions, not one: which tags to compile AND which knowledge store to curate (a named store, all stores, or none). Curated knowledge/ pages are store-scoped and their page names carry no tag prefix, so they match a tag only through the `tag` property — often absent. Any downstream step or tool scoped by "the tags the human approved at the scope gate" therefore silently misses curate-leg candidates. Verified while reviewing dewey-distill-verify-step (issue #151): KD-FR-104's dual scope formulation ("approved tags OR produced pages") diverges exactly there, and only the pages selector can cover all three namespaces the promotable queue defines. Changes that touch the distill pipeline should scope by the pages the review gate will present, never by tags alone.
