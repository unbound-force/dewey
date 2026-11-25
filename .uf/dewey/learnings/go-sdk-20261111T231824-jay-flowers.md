---
tag: go-sdk
author: jay-flowers
category: gotcha
created_at: 2026-11-11T23:18:24Z
identity: go-sdk-20261111T231824-jay-flowers
tier: draft
---

Bumping github.com/modelcontextprotocol/go-sdk 1.2.0→1.4.1 (PR #176, via jsonschema-go 0.4) changed generated input schemas: slice fields are now nullable, so a property's JSON Schema "type" renders as ["null","array"] instead of "array". PR #176 added the schemaTypeIncludes helper (server_test.go:1695) and migrated the promote schema test, but PR #197's TestNewServer_VerifyCandidatesSchema copied the older exact-equality pattern (prop["type"] != "array"), leaving main's build-and-test red after both landed; PR #199 fixed it by switching to the helper. Two takeaways: (1) new schema-assertion tests in dewey must use schemaTypeIncludes rather than exact string equality on "type"; (2) a dependency bump that changes generated schema shapes must sweep every schema-asserting test in the repo, not only the ones the bump PR happens to touch. Note the boundary: only slice-level types go nullable — element types (items.type "string") and non-schema config values stay exact-match.
