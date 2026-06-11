## Why

The awk script in `.github/workflows/release.yml` that patches Homebrew cask SHA-256 checksums after macOS code signing produces incorrect results. Every signed Dewey release (v3.1.0, v3.2.0) ships with wrong checksums in the `unbound-force/homebrew-tap` cask, blocking `brew install unbound-force/tap/dewey` on Apple Silicon and Intel Mac.

The root cause: the awk script assumes `url` appears before `sha256` in the GoReleaser-generated cask (as it does in `unbound-force/unbound-force`), but Dewey's GoReleaser output places `sha256` before `url`. This causes a cascading off-by-one — each platform's flag triggers replacement of the next platform's checksum instead of its own.

Related issues: [dewey#67](https://github.com/unbound-force/dewey/issues/67), [homebrew-tap#4](https://github.com/unbound-force/homebrew-tap/issues/4). The same class of bug previously affected Gaze ([gaze#25](https://github.com/unbound-force/gaze/pull/25)).

## What Changes

### New Capabilities
- None

### Modified Capabilities
- `release.yml sign-macos`: Rewrite the cask checksum patching logic to be order-agnostic, handling both `sha256-before-url` and `url-before-sha256` cask layouts
- `release.yml sign-macos`: Add a verification step that confirms patched checksums match `checksums.txt` before pushing to the tap
- `.goreleaser.yaml`: Pin GoReleaser action version and GoReleaser binary version to prevent future template drift

### Removed Capabilities
- None

## Impact

- **Files changed**: `.github/workflows/release.yml`, `.goreleaser.yaml`
- **Affected systems**: macOS signing job, Homebrew tap publishing
- **No production code changes**: This is purely a CI workflow fix
- **Cross-repo note**: The same latent bug exists in `unbound-force/unbound-force` if its GoReleaser version ever changes template ordering. A follow-up issue should be filed there. The `replicator` repo (which copied the same pipeline pattern) may also be affected.
- **Remediation**: After the fix is merged, v3.2.0 cask in `homebrew-tap` should be manually corrected using the signed checksums from `checksums.txt`. v3.1.0 is superseded by v3.2.0; users should upgrade rather than install an older version, so v3.1.0 remediation is not needed.
- **Website docs**: No website documentation sync needed — this restores existing `brew install` behavior, not a new installation method. The installation command is unchanged; only the underlying checksums are corrected.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Autonomous Collaboration

**Assessment**: PASS

This change restores the artifact-based communication channel (cask file) between the release pipeline and the Homebrew tap. No MCP tool interfaces are affected.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. This fix restores the Homebrew installation path that was broken — improving standalone usability rather than introducing dependencies.

### III. Observable Quality

**Assessment**: PASS

The fix adds a verification step that confirms patched checksums match `checksums.txt` before pushing to the tap. This makes the signing pipeline's output auditable and fails loudly on mismatch rather than silently publishing wrong checksums.

### IV. Testability

**Assessment**: PASS

Testability is addressed through the post-patch verification step (task 3.1), which serves as an automated regression gate within the CI workflow. No Go unit tests are needed since no Go code is changed. TC-006 (regression test for bug fixes) is not directly applicable to CI workflow shell scripts, which cannot be unit-tested without a full GitHub Actions environment — the verification gate serves as the runtime equivalent.
