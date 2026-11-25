---
tag: opsx-demote-validated-on-regeneration
author: jay-flowers
category: gotcha
created_at: 2026-11-14T17:48:51Z
identity: opsx-demote-validated-on-regeneration-20261114T174851-jay-flowers
tier: draft
---

SQLite's UPDATE evaluates every SET expression against the pre-update row, which makes a content-bound tier rule expressible as a single statement in store.UpdatePage: `tier = CASE WHEN tier = 'validated' AND ? = content_hash THEN 'validated' ELSE ? END` compares the incoming hash against the STORED hash even though content_hash is reassigned earlier in the same statement. Binding the incoming hash for the equality check before the caller-supplied tier keeps demotion atomic with no read-then-write window. Two empty hashes compare equal because database/sql binds empty strings as '' (never NULL), so hash-less writers keep the pre-change never-demote behavior. The PR 145 promote-race property survives unchanged: a metadata write racing a promotion carries an unchanged hash, so validated is preserved — TestUpdatePage_PreservesValidatedTier passes unmodified.
