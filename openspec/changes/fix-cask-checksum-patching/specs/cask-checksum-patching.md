## ADDED Requirements

### Requirement: Cask checksum post-patch verification

The `sign-macos` job MUST verify that the patched cask file contains the correct signed checksums before pushing to the Homebrew tap. If verification fails, the job MUST exit with a non-zero status and log which checksums are missing.

#### Scenario: Checksums match after patching
- **GIVEN** the `sign-macos` job has signed darwin binaries and patched the cask file
- **WHEN** the verification step runs
- **THEN** the cask file MUST contain the exact `ARM64_SHA` value within the `on_arm` section and the exact `AMD64_SHA` value within the `on_intel` section, and the step MUST exit 0

#### Scenario: Checksums do not match after patching
- **GIVEN** the cask patching logic has a bug or GoReleaser changed its template
- **WHEN** the verification step runs
- **THEN** the step MUST exit non-zero with an error message identifying all platforms with missing checksums (not just the first failure), and the job MUST NOT push to the Homebrew tap

### Requirement: GoReleaser version pinning

The release workflow MUST pin the GoReleaser binary to a specific version rather than using a floating range.

#### Scenario: Reproducible cask generation
- **GIVEN** a release is triggered by a `v*` tag push
- **WHEN** GoReleaser runs
- **THEN** it MUST use the exact pinned version (not a range like `~> v2`), ensuring consistent cask template output across releases

### Requirement: Defensive extraction of original checksums

The checksum extraction step MUST fail explicitly if the GoReleaser-generated cask does not match the expected structure. The extraction MUST NOT proceed with empty or incorrect values.

#### Scenario: Cask structure does not match expected format
- **GIVEN** GoReleaser generates a cask without recognizable `on_macos`/`on_intel`/`on_arm` sections
- **WHEN** the extraction step attempts to find original checksums
- **THEN** the step MUST exit non-zero with an error identifying that the expected cask structure was not found, and the job MUST NOT push to the Homebrew tap

#### Scenario: Extraction finds exactly one checksum per darwin platform
- **GIVEN** a GoReleaser-generated cask with the expected `on_macos > on_intel` and `on_macos > on_arm` sections
- **WHEN** the extraction step runs
- **THEN** it MUST extract exactly one SHA-256 hash per darwin platform, and if extraction yields zero or more than one hash for any platform, the step MUST exit non-zero

## MODIFIED Requirements

### Requirement: Cask checksum patching logic

The `sign-macos` job MUST patch darwin SHA-256 checksums in the GoReleaser-generated cask to match the signed binary checksums. The patching logic MUST be order-agnostic — it MUST produce correct results regardless of whether `sha256` appears before or after `url` in each platform section.

**Precondition**: The GoReleaser-generated cask contains unique SHA-256 values for each platform archive. This is guaranteed by the fact that different binaries produce different hashes.

Previously: The awk script assumed `url` (containing the platform identifier) appeared before `sha256`, setting a flag on the `url` line and replacing the next `sha256` line. This failed when GoReleaser placed `sha256` before `url`.

#### Scenario: sha256-before-url ordering (Dewey's current GoReleaser output)
- **GIVEN** GoReleaser generates a cask where each platform section has `sha256` before `url`
- **WHEN** the `sign-macos` job patches darwin checksums
- **THEN** the `on_intel` section within `on_macos` MUST contain the signed `darwin_amd64` checksum, the `on_arm` section within `on_macos` MUST contain the signed `darwin_arm64` checksum, and all `on_linux` sections MUST remain unchanged

#### Scenario: url-before-sha256 ordering (unbound-force/unbound-force's GoReleaser output)
- **GIVEN** GoReleaser generates a cask where each platform section has `url` before `sha256`
- **WHEN** the `sign-macos` job patches darwin checksums
- **THEN** the same correct results MUST be produced as in the sha256-before-url scenario

#### Scenario: Linux checksums are not modified
- **GIVEN** a GoReleaser-generated cask with both darwin and linux platform sections
- **WHEN** the `sign-macos` job patches darwin checksums
- **THEN** all checksum values within `on_linux` sections MUST remain identical to the GoReleaser-generated values

## REMOVED Requirements

None.
