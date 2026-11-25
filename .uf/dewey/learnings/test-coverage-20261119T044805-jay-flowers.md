---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-19T04:48:05Z
identity: test-coverage-20261119T044805-jay-flowers
tier: draft
as_of: "dewey@377610524098d68d9f685178d7f173f0a5558585"
---

Optional-setter DI seams on tool handlers (e.g. tools.Search.WithStore, defaulting to nil) are invisible to handler-level tests: every test constructs the handler with the dependency manually, so dropping the single production wiring call in newServer (server.go) passes the entire package suite and CI while disabling the feature in production — scoped calls fail with the no-store error, or worse, degrade to unscoped results if the nil-store arm is ever made lenient. When adding an optional-setter seam, extend the existing server-level wiring pattern instead: build newServer(vc, false, WithPersistentStore(s)) and invoke the registered tool through the server (the TestRegisterHealthTool_WithStore precedent in server_test.go), asserting the seam-dependent behavior (here: a scoped search call does not return the no-store error).
