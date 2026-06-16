## ADDED Requirements

### Requirement: Release Preflight Gate

The release workflow MUST execute a preflight validation job before building or publishing any release artifacts. The preflight job MUST pass all validation steps before the release job is allowed to run.

#### Scenario: Successful release with all checks passing
- **GIVEN** the `release.yml` workflow is triggered via `workflow_dispatch` with `tag: "v1.2.3"` from the `main` branch
- **WHEN** the dispatched commit (`github.sha`) has passed both `build-and-test` and `MegaLinter` CI checks, the tag format is valid, the tag does not already exist, the version is greater than the latest existing tag, and there are unreleased commits since the last tag
- **THEN** the preflight job SHALL create and push the annotated tag `v1.2.3` (message: `v1.2.3`), and the release job SHALL proceed to build and publish artifacts

#### Scenario: Release blocked by failed CI
- **GIVEN** the `release.yml` workflow is triggered via `workflow_dispatch` with `tag: "v1.2.3"`
- **WHEN** the `build-and-test` check on the dispatched commit (`github.sha`) has concluded with a status other than `success` (e.g., `failure`, `cancelled`)
- **THEN** the preflight job MUST fail with an error message naming the failing check and its current status, and the release job MUST NOT run

#### Scenario: Release blocked by incomplete MegaLinter check
- **GIVEN** the `release.yml` workflow is triggered via `workflow_dispatch` with `tag: "v1.2.3"`
- **WHEN** the `MegaLinter` check on the dispatched commit (`github.sha`) has not concluded with `success` (absent, in-progress, failed, or cancelled)
- **THEN** the preflight job MUST fail with an error message advising the operator to wait for CI to complete before releasing

#### Scenario: Required check not found on dispatched commit
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.2.3"`
- **WHEN** the Checks API returns no check run named `build-and-test` for the dispatched commit
- **THEN** the preflight job MUST fail with an error indicating the check was not found and advising the operator to verify CI ran on this commit

#### Scenario: Both required checks fail
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.2.3"`
- **WHEN** both `build-and-test` and `MegaLinter` checks have not concluded with `success`
- **THEN** the preflight job MUST report ALL failing checks and their statuses (not fail-fast on the first), then fail

#### Scenario: Release attempted from non-main branch
- **GIVEN** the workflow is triggered from branch `feature-x` (not `main`)
- **WHEN** the preflight job starts
- **THEN** the preflight job MUST fail with an error indicating releases MUST be triggered from the `main` branch

Note: The Checks API is queried at `GET /repos/{owner}/{repo}/commits/{sha}/check-runs`. The check names correspond to: `build-and-test` (the job key in `ci.yml`, which has no explicit `name:` override) and `MegaLinter` (the `name:` field on the `megalinter` job in `mega-linter.yml`). When a check name returns multiple check runs (e.g., from re-runs), the most recent run's conclusion MUST be used.

### Requirement: Tag Format Validation

The preflight job MUST validate that the tag input matches the pattern `vMAJOR.MINOR.PATCH` (e.g., `v1.2.3`). Tags with pre-release suffixes, build metadata, non-numeric segments, or empty/whitespace content MUST be rejected.

#### Scenario: Invalid tag format rejected (too few segments)
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.2"`
- **WHEN** the preflight job validates the tag format
- **THEN** the job MUST fail with an error indicating the expected format `vMAJOR.MINOR.PATCH`

#### Scenario: Pre-release suffix rejected
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.2.3-rc1"`
- **WHEN** the preflight job validates the tag format
- **THEN** the job MUST fail with an error indicating the expected format

#### Scenario: Build metadata rejected
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.2.3+build.1"`
- **WHEN** the preflight job validates the tag format
- **THEN** the job MUST fail with an error indicating the expected format

#### Scenario: Non-numeric segments rejected
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v1.two.3"`
- **WHEN** the preflight job validates the tag format
- **THEN** the job MUST fail with an error indicating the expected format

#### Scenario: Empty tag input rejected
- **GIVEN** the `release.yml` workflow is triggered with `tag: ""`
- **WHEN** the preflight job validates the tag format
- **THEN** the job MUST fail with an error indicating the expected format

#### Scenario: Valid tag format accepted
- **GIVEN** the `release.yml` workflow is triggered with `tag: "v0.15.0"`
- **WHEN** the preflight job validates the tag format
- **THEN** validation SHALL pass and the job SHALL proceed to the next step

### Requirement: Tag Uniqueness

The preflight job MUST verify that the specified tag does not already exist on the remote pointing to a different commit. If the tag exists and points to a commit other than the dispatched commit (`github.sha`), the job MUST fail. If the tag exists and points to the dispatched commit (indicating a previous preflight run), the uniqueness check MUST pass to support idempotent re-runs.

#### Scenario: Duplicate tag rejected (different commit)
- **GIVEN** tag `v1.0.0` already exists on the remote pointing to commit `abc123`
- **WHEN** the workflow is triggered with `tag: "v1.0.0"` on a different commit `def456`
- **THEN** the preflight job MUST fail with an error indicating the tag already exists and points to a different commit

#### Scenario: Existing tag accepted (same commit, re-run)
- **GIVEN** tag `v1.0.0` already exists on the remote pointing to the dispatched commit
- **WHEN** the workflow is re-run with `tag: "v1.0.0"`
- **THEN** the uniqueness check MUST pass (idempotent re-run)

### Requirement: Semver Ordering

The preflight job MUST verify that the new tag version is strictly greater than the latest existing tag. If the new version is less than or equal to the latest tag, the job MUST fail. "Latest" means the highest version by semver ordering (not the most recently created tag).

#### Scenario: Version regression rejected
- **GIVEN** the latest tag on the remote is `v1.5.0`
- **WHEN** the workflow is triggered with `tag: "v1.4.0"`
- **THEN** the preflight job MUST fail with an error indicating the version is not greater than the latest release

#### Scenario: First release accepted
- **GIVEN** no version tags exist on the remote
- **WHEN** the workflow is triggered with `tag: "v0.1.0"`
- **THEN** the semver ordering check SHALL pass (first release)

#### Scenario: Major version bump accepted
- **GIVEN** the latest tag on the remote is `v1.5.0`
- **WHEN** the workflow is triggered with `tag: "v2.0.0"`
- **THEN** the semver ordering check SHALL pass

#### Scenario: Multi-digit version comparison
- **GIVEN** the latest tag on the remote is `v0.9.0`
- **WHEN** the workflow is triggered with `tag: "v0.10.0"`
- **THEN** the semver ordering check SHALL pass (numeric comparison, not lexicographic)

### Requirement: Unreleased Commits

The preflight job MUST verify that at least one commit exists between the latest tag and the dispatched commit. If there are no unreleased commits, the job MUST fail.

#### Scenario: No unreleased commits rejected
- **GIVEN** the latest tag `v1.0.0` points to the current dispatched commit
- **WHEN** the workflow is triggered with `tag: "v1.0.1"`
- **THEN** the preflight job MUST fail with an error indicating there are no unreleased commits

#### Scenario: First release (no existing tags)
- **GIVEN** no version tags exist on the remote
- **WHEN** the workflow checks for unreleased commits
- **THEN** the check SHALL pass (all commits are unreleased)

### Requirement: Tag Creation by Workflow

The preflight job MUST create an annotated tag and push it to the remote after all validation steps pass. The tag MUST NOT be created before validation succeeds. The tag message MUST be the version string (e.g., `git tag -a v1.2.3 -m "v1.2.3"`).

#### Scenario: Tag created after successful preflight
- **GIVEN** all preflight validation steps have passed
- **WHEN** the preflight job reaches the tag creation step
- **THEN** the job SHALL create an annotated tag with the version as the message and push it to the origin remote

#### Scenario: Tag already exists on re-run (same commit)
- **GIVEN** the preflight job previously created tag `v1.2.3` pointing to the dispatched commit, but the release job failed
- **WHEN** the workflow is re-run with the same tag input
- **THEN** the tag creation step MUST verify the existing tag points to the dispatched commit and skip tag creation (idempotent re-run)

### Requirement: Concurrent Release Prevention

The release workflow MUST use a concurrency group (`release-${{ github.ref }}`) with `cancel-in-progress: false` to prevent parallel release runs.

#### Scenario: Concurrent release queued
- **GIVEN** a release workflow is already running for tag `v1.2.0`
- **WHEN** a second release is triggered for tag `v1.3.0`
- **THEN** the second run SHALL queue until the first completes (not cancel the first)

### Requirement: Branch Validation

The preflight job MUST verify that the workflow was triggered from the `main` branch (`github.ref == 'refs/heads/main'`). Releases from non-main branches MUST be rejected.

#### Scenario: Feature branch release rejected
- **GIVEN** the workflow is triggered from the GitHub Actions UI with the branch dropdown set to `feature-x`
- **WHEN** the preflight job validates the branch
- **THEN** the job MUST fail with an error indicating releases must be triggered from `main`

## MODIFIED Requirements

### Requirement: Release Trigger Mechanism

The release workflow MUST use `workflow_dispatch` with a required `tag` string input instead of `push.tags: v*`.

Previously: The workflow triggered automatically on any `v*` tag push.

#### Scenario: Manual release trigger
- **GIVEN** an operator wants to release version `v1.2.3`
- **WHEN** they use the GitHub Actions UI "Run workflow" button or run `gh workflow run release.yml -f tag=v1.2.3`
- **THEN** the workflow SHALL start with the provided tag value

### Requirement: Release Job Dependencies

The `release` job MUST declare `needs: preflight` so it only runs after successful preflight validation. The `release` job MUST check out the tag created by the preflight job using `ref: ${{ inputs.tag }}`. All references to `GITHUB_REF_NAME` in the `release` job MUST be replaced with `${{ inputs.tag }}` or the `RELEASE_TAG` env var, because `GITHUB_REF_NAME` under `workflow_dispatch` resolves to the branch name (e.g., `main`), not the tag.

Previously: The `release` job ran unconditionally with no dependencies.

### Requirement: Sign-macOS Tag Reference and Permissions

The `sign-macos` job MUST reference the tag via `inputs.tag` (from `workflow_dispatch`) or the `RELEASE_TAG` env var instead of `GITHUB_REF_NAME` in ALL locations. Under `workflow_dispatch`, `GITHUB_REF_NAME` resolves to the branch name, not the tag -- using it would break asset downloads and uploads. The `sign-macos` job MUST also declare per-job `permissions: { contents: write }` because the workflow-level permissions change to `permissions: {}`.

Previously: The job used `GITHUB_REF_NAME` which resolved to the pushed tag. The job inherited workflow-level `permissions: contents: write`.

#### Scenario: macOS signing uses workflow input tag
- **GIVEN** the release workflow is triggered with `tag: "v1.2.3"`
- **WHEN** the `sign-macos` job downloads darwin archives, signs them, uploads replacements, and updates the Homebrew cask
- **THEN** all tag references in the job SHALL resolve to `v1.2.3`, never `GITHUB_REF_NAME`

### Requirement: Signing Secrets Check Location

The signing secrets check (`has_signing_secrets` output) MUST be moved from the `release` job to the `preflight` job. The `release` job MUST forward this output so the `sign-macos` job can access it via `needs.release.outputs.has_signing_secrets`.

Previously: The signing secrets check was in the `release` job.

## REMOVED Requirements

None.
