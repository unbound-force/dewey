---
tag: release-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-26T17:48:58Z
identity: release-pipeline-20261126T174858-jay-flowers
tier: draft
as_of: "dewey@1a15abf4114f3c4f2d1d8b1618fe6523181dd7ab"
---

When wiring a post-release smoke check that runs an installed binary's version command (e.g. ship's go_install_check / tap_install_check), a bare `<bin> version` only proves the binary installs and exits 0 — it does not prove the installed artifact is the released tag, so a stale module-proxy cache or wrong install still passes. To actually verify the version, assert the output against the tag, but normalize the leading 'v': dewey's GoReleaser ldflags build prints `3.9.0` while a `go install module@tag` build-info binary prints `v3.8.1` (resolveVersionFrom returns the value verbatim), so an equality check must strip the prefix (e.g. compare `sed 's/^v//'` output to the tag) or it will pass for one distribution path and fail for the other.
