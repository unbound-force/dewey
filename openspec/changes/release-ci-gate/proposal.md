## Why

The current `release.yml` workflow triggers on any `v*` tag push and immediately runs GoReleaser with no verification that CI checks passed on the tagged commit. A tag pushed from a commit that never passed tests will produce and distribute broken release binaries.

The `unbound-force/unbound-force` repository already solved this with a `preflight` job that verifies CI status via the GitHub Checks API before building artifacts. Dewey should adopt the same pattern.

Reference: [unbound-force/dewey#65](https://github.com/unbound-force/dewey/issues/65)

## What Changes

Replace the `push.tags` trigger with `workflow_dispatch` (manual trigger with a `tag` input) and add a `preflight` job that gates artifact publishing on CI verification. Add branch validation to ensure releases only come from `main`.

## Capabilities

### New Capabilities
- `preflight job`: Validates branch (main-only), tag format, tag uniqueness, semver ordering, CI status on HEAD via Checks API, and unreleased commits before creating the tag and proceeding to GoReleaser
- `workflow_dispatch trigger`: Manual release trigger with explicit version input, replacing automatic tag-push trigger
- `branch validation`: Rejects releases triggered from non-main branches

### Modified Capabilities
- `release job`: Now depends on `preflight` completing successfully; checks out the tag created by preflight; uses `RELEASE_TAG` env var instead of `GITHUB_REF_NAME`
- `sign-macos job`: Updated to reference `RELEASE_TAG` instead of `GITHUB_REF_NAME`; gains explicit per-job `permissions: { contents: write }`

### Removed Capabilities
- `push.tags trigger`: Removed in favor of `workflow_dispatch` to prevent unverified releases

## Impact

- **Single file changed**: `.github/workflows/release.yml`
- **Documentation update**: `AGENTS.md` CI/CD section updated to reflect new trigger and preflight gate
- **Release process**: Operators must use the GitHub Actions "Run workflow" UI (or `gh workflow run`) instead of pushing a tag manually. The tag is created by the workflow after preflight passes.
- **No Go code changes**: This is a CI-only change with no impact on the binary, tests, or production behavior.
- **Existing macOS signing**: The `sign-macos` job structure is preserved; tag reference variable and permissions are updated.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Autonomous Collaboration

**Assessment**: N/A

This change modifies a GitHub Actions workflow file only. No artifact-based communication between heroes is affected. No runtime coupling is introduced or changed.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable. The release workflow is an operational concern that does not affect the binary's standalone functionality. No new dependencies are introduced to the Go module.

### III. Observable Quality

**Assessment**: PASS

This change directly improves observable quality by ensuring that release artifacts are only published from commits that have passed CI verification. The preflight job produces clear, auditable output for each validation step (branch, tag format, uniqueness, semver ordering, CI status, unreleased commits).

### IV. Testability

**Assessment**: PASS

This change modifies a GitHub Actions workflow. Testability is addressed through YAML validation (task 2.1) and the workflow's own execution as the integration test. No new Go packages are introduced. Workflow changes have inherently limited unit-testability -- the verification strategy (YAML lint + first execution) is the best available approach for CI-only changes.
