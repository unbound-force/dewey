## Context

The `sign-macos` job in `.github/workflows/release.yml` signs darwin binaries, replaces them in the GitHub Release, and then patches the GoReleaser-generated Homebrew cask with the new SHA-256 checksums before pushing to `unbound-force/homebrew-tap`.

The patching uses an awk script that sets a flag when it encounters `darwin_amd64` or `darwin_arm64` in a line, then substitutes the next `sha256` line. This works when `url` precedes `sha256` (as in `unbound-force/unbound-force` with GoReleaser v2.14.1) but fails when `sha256` precedes `url` (as Dewey's GoReleaser output does). The flag is set on the `url` line but the `sha256` line has already been printed — causing each platform's hash to land on the wrong line.

The `unbound-force/unbound-force` repo uses an identical awk script but avoids the bug because it pins GoReleaser to v2.14.1, which happens to produce the `url`-first ordering.

## Goals / Non-Goals

### Goals
- Fix the cask checksum patching to work regardless of `sha256`/`url` ordering in GoReleaser output
- Add a verification gate that catches mismatches before pushing to the tap
- Pin GoReleaser version to prevent future template drift
- Remediate the current v3.2.0 cask in `homebrew-tap`

### Non-Goals
- Fixing the same bug in `unbound-force/unbound-force` or `replicator` (separate repos, separate issues)
- Changing the GoReleaser cask template or switching from cask to formula
- Adding automated testing of the release workflow (would require a separate spec)

## Decisions

### D1: Use section-scoped extraction + `sed` replacement instead of flag-based awk

**Decision**: Replace the awk script with a two-pass approach: awk for section-scoped extraction of original checksums, then `sed` for exact string substitution.

**Rationale**: The awk flag approach is inherently fragile — it depends on line ordering within sections. A section-scoped `sed` that identifies `on_intel` vs `on_arm` blocks within `on_macos` and replaces the sha256 value within each block is order-agnostic.

**Approach**: Use awk to extract the original darwin checksums from the GoReleaser-generated cask by parsing `on_macos > on_intel` and `on_macos > on_arm` sections, then use `sed` to replace those exact hash strings with the signed values. Since each SHA-256 hash is unique within the file, direct string replacement is unambiguous.

### D2: Add post-patch verification step

**Decision**: After patching, grep the cask file for the expected signed checksums and fail the job if they are not found.

**Rationale**: Aligns with the Observable Quality constitution principle — the signing pipeline should fail loudly on mismatch rather than silently publishing wrong checksums. This catches both the original bug and any future GoReleaser template changes.

### D3: Pin GoReleaser to a specific version

**Decision**: Change `.goreleaser.yaml` action from `version: "~> v2"` to a pinned version (matching `unbound-force/unbound-force` at `v2.14.1`) and update the goreleaser-action SHA to v7.x (verify exact SHA at implementation time against the official releases page). The goreleaser-action v6→v7 bump is a Node runtime upgrade with no breaking changes to inputs/outputs.

**Rationale**: Floating version ranges are a reliability risk for reproducible builds. The `unbound-force/unbound-force` repo already pins its version, and Dewey should follow the same practice.

## Risks / Trade-offs

- **Risk**: The `sed` approach assumes each SHA-256 hash appears exactly once in the cask file. GoReleaser-generated casks do not repeat hashes (different binaries produce different hashes), so this is safe. The verification step catches any violation.
- **Risk**: Pinning GoReleaser version means manual updates are needed for new features. This is acceptable — reproducibility is more important than auto-updates for release tooling.
- **Risk**: The `sed` usage must be compatible with BSD `sed` (macOS) since the `sign-macos` job runs on `macos-latest`. The approach (exact string substitution writing to a new file) works identically on BSD and GNU `sed`.
- **Trade-off**: We could rewrite the awk to be order-agnostic instead of switching to `sed`. However, the `sed` approach is simpler to read and maintain, and the explicit extraction + replacement pattern makes the logic self-documenting.
- **Known limitation**: If the cask patching or verification step fails after the signed archives have been uploaded to the GitHub Release, the release will have correct signed binaries but the Homebrew tap will not be updated. Manual remediation (similar to task 4.1) would be required. This is acceptable because the verification step prevents *wrong* checksums from being pushed — the worst case is *stale* checksums, not *incorrect* ones.
