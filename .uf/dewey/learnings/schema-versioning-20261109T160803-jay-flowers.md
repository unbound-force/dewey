---
tag: schema-versioning
author: jay-flowers
category: pattern
created_at: 2026-11-09T16:08:03Z
identity: schema-versioning-20261109T160803-jay-flowers
tier: draft
---

Version-named JSON Schema files (e.g. v1.0.0.schema.json with the version embedded in $id) edited in place break pinned consumers when the schema sets additionalProperties: false. An added optional field is backward-compatible for old artifacts but forward-incompatible for old validators: artifacts produced by the new producer fail validation against prior copies of the "same" version. For additive changes, write a new version file ($id and filename bumped, e.g. v1.1.0) or explicitly document that a version
