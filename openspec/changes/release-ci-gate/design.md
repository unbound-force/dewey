## Context

Dewey's `release.yml` triggers on `push.tags: v*` and immediately runs GoReleaser. There is no verification that the tagged commit passed CI. The `unbound-force/unbound-force` repository has a battle-tested `preflight` job pattern that validates CI status, tag format, semver ordering, and unreleased commits before creating the tag and proceeding to artifact publishing.

This design adapts that proven pattern for dewey's simpler CI setup (two workflows: CI and MegaLinter, no security scanners).

## Goals / Non-Goals

### Goals
- Prevent release of binaries from commits that haven't passed CI
- Validate tag format, uniqueness, and semver ordering before creating the tag
- Verify unreleased commits exist to prevent empty releases
- Enforce releases only from the `main` branch
- Adopt `workflow_dispatch` trigger so the tag is created by the workflow, not pushed manually
- Match the reference implementation from `unbound-force/unbound-force` for consistency across the org

### Non-Goals
- Adding SBOM generation (Syft) or binary signing (Cosign) -- separate concern, separate issue
- Adding security scanners (OSV-Scanner, Trivy) -- dewey doesn't have these configured yet
- Changing the GoReleaser configuration or build matrix
- Modifying the macOS signing/notarization flow beyond updating tag references and permissions

## Decisions

### D1: Use `workflow_dispatch` instead of `push.tags`

**Decision**: Replace the `push.tags: v*` trigger with `workflow_dispatch` with a `tag` string input.

**Rationale**: With `push.tags`, the tag must exist before the workflow runs, so there's no opportunity to validate it first. With `workflow_dispatch`, the workflow validates everything and then creates the tag itself -- ensuring only verified releases get tagged. This matches the org-wide pattern.

**Trade-off**: Releases now require using the GitHub Actions UI ("Run workflow") or `gh workflow run release.yml -f tag=v1.2.3` instead of simply `git tag v1.2.3 && git push --tags`. This is a minor ceremony increase for a significant safety gain.

### D2: Verify specific CI check names via Checks API

**Decision**: Query the GitHub Checks API (`GET /repos/{owner}/{repo}/commits/{sha}/check-runs`) for check runs on the dispatched commit (`github.sha`). Require `build-and-test` and `MegaLinter` to have `conclusion: "success"`. Check all required checks and report all failures (not fail-fast).

**Check name derivation**:
- `build-and-test`: This is the job key in `ci.yml` (line 19). The job has no explicit `name:` field, so the Checks API uses the job key as the check name.
- `MegaLinter`: This is the `name:` field on the `megalinter` job in `mega-linter.yml` (line 23). Note the job key is `megalinter` (lowercase) but the display name is `MegaLinter` (PascalCase). The Checks API uses the `name:` field when present.

**When a check name returns multiple check runs** (e.g., from workflow re-runs), the most recent run's conclusion is used.

**Rationale**: These are the two CI workflows that run on push to main. The check names are stable identifiers. Using the API provides granular per-check verification.

**Note on MegaLinter scope**: MegaLinter only runs on pushes to `main` and PRs to `main` (per `mega-linter.yml` triggers). Combined with the branch validation (D6), this ensures releases come from commits that both CI pipelines have verified.

### D3: Skip security scan verification

**Decision**: Do not require security scan checks in the preflight. The reference implementation checks for OSV-Scanner/Trivy, but dewey doesn't have these configured.

**Rationale**: Adding a requirement for checks that don't exist would block all releases. Security scanning can be added later as a separate enhancement, and the preflight can be updated to include it at that time.

### D4: Use per-job permissions

**Decision**: Set `permissions: {}` at the workflow level and declare specific permissions per job.

**Per-job permissions**:
- `preflight`: `contents: write` (tag creation), `checks: read` (Checks API query)
- `release`: `contents: write` (GoReleaser uploads, cask upload)
- `sign-macos`: `contents: write` (release asset download/upload, Homebrew tap push)

**Rationale**: Follows the principle of least privilege and matches the reference implementation. Each job gets only the permissions it needs.

### D5: Add concurrency control

**Decision**: Add a `concurrency` group (`release-${{ github.ref }}`) with `cancel-in-progress: false` to prevent parallel release runs.

**Rationale**: Two concurrent releases could create conflicting tags or overwrite each other's artifacts. Using `cancel-in-progress: false` lets the first release finish rather than canceling it. The second run queues.

### D6: Enforce main-branch-only releases

**Decision**: Add a branch validation step in preflight that verifies `github.ref == 'refs/heads/main'`.

**Rationale**: `workflow_dispatch` can be triggered from any branch via the GitHub Actions UI or CLI. Without this check, an operator could accidentally release from a feature branch, creating a tag pointing to unmerged code. The branch validation provides an explicit guard rather than relying on the implicit protection of MegaLinter only running on main.

### D7: Use `RELEASE_TAG` env var for tag references

**Decision**: Add `RELEASE_TAG: ${{ inputs.tag }}` as a job-level env var in all three jobs. Shell steps use `$RELEASE_TAG` instead of `${{ inputs.tag }}` or `${GITHUB_REF_NAME}`.

**Rationale**: `GITHUB_REF_NAME` under `workflow_dispatch` resolves to the branch name (e.g., `main`), not the tag. Using it would cause asset download/upload failures. A dedicated env var avoids this gotcha and provides a single point of change if the input name ever changes. All existing `${GITHUB_REF_NAME}` references in the `release` and `sign-macos` jobs must be replaced.

## Risks / Trade-offs

### Risk: CI check names change

If the CI workflow job names are renamed (e.g., `build-and-test` becomes `ci`), the preflight will fail to find the check and block releases. **Mitigation**: The error message explicitly names the check and its status, making diagnosis straightforward. The fix is a one-line change to the `REQUIRED_CHECKS` array. Implementation should include a comment documenting the coupling: `# These check names must match the job IDs/names in ci.yml and mega-linter.yml`.

### Risk: MegaLinter timing

MegaLinter applies auto-fixes and commits them, which can cause transient states. If a release is attempted while MegaLinter is still running, the check won't show as `success` yet. **Mitigation**: The error message advises waiting for CI to complete before releasing.

### Risk: GitHub API availability

The Checks API query could fail due to rate limiting, network timeouts, or GitHub outages. **Mitigation**: API failures cause the step to fail, and the operator re-runs the workflow. The `timeout-minutes: 10` on the preflight job prevents indefinite hangs. No retry logic is added for simplicity -- transient failures are rare and re-running is trivial.

### Risk: Platform dependency on `sort -V`

The semver comparison uses GNU `sort -V` (version sort), which is a GNU coreutils extension. Ubuntu-based GitHub Actions runners include GNU sort. **Mitigation**: This is a platform assumption documented here. If the runner image changes, the comparison step would need to be rewritten with a pure-bash numeric comparison. The reference implementation uses the same approach.

### Trade-off: Manual trigger vs. tag-push automation

Moving to `workflow_dispatch` removes the ability to trigger a release by pushing a tag. Some CI/CD tools and scripts may expect the tag-push pattern. **Accepted**: The safety benefit of pre-validation outweighs the convenience of tag-push. The `gh workflow run` CLI command provides scriptability for automation needs.

### Recovery: Partial failure (preflight passes, release fails)

If the preflight creates the tag but GoReleaser fails, a tag exists on the remote with no release artifacts. **Recovery path**: Re-run the workflow with the same tag input. The tag uniqueness check recognizes the existing tag points to the same commit (idempotent re-run) and skips tag creation. GoReleaser then proceeds normally. Alternative: manually delete the tag via `git push origin :refs/tags/v1.2.3` and re-trigger.
