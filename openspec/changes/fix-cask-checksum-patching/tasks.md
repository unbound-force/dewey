<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Pin GoReleaser Version

- [x] 1.1 In `.github/workflows/release.yml`, update the `goreleaser/goreleaser-action` step: pin to the latest v7.x SHA (verify against https://github.com/goreleaser/goreleaser-action/releases before committing), and change `version: "~> v2"` to `version: "v2.14.1"`. Confirm `.goreleaser.yaml` has `version: 2` as a precondition check.

## 2. Rewrite Cask Checksum Patching

- [x] 2.1 In `.github/workflows/release.yml`, replace the awk-based checksum patching block (the `awk -v amd64=... -v arm64=...` block in the "Update Homebrew cask" step) with order-agnostic logic: (a) Extract the original unsigned darwin checksums from the GoReleaser-generated cask by parsing `on_macos > on_intel` and `on_macos > on_arm` section context using awk. (b) If extraction fails to find exactly one checksum per darwin platform, exit non-zero with a descriptive error before attempting any substitution. (c) Use `sed` to replace each original hash with the corresponding signed hash (BSD-compatible syntax since the job runs on `macos-latest`). (d) Ensure linux sections are untouched.

## 3. Add Post-Patch Verification

- [x] 3.1 In `.github/workflows/release.yml`, immediately after the patching logic (task 2.1), add a verification block that: (a) Uses section-aware checking (awk or grep with context) to confirm `$AMD64_SHA` appears within the `on_intel` section and `$ARM64_SHA` appears within the `on_arm` section. (b) Confirms the original unsigned darwin checksums are no longer present in the patched file (ensuring replacement, not duplication). (c) Exits non-zero with a descriptive error identifying all failing platforms if any check fails, preventing the push to `homebrew-tap`.

## 4. Remediate Existing Release

- [x] 4.1 Remediate the v3.2.0 cask in `unbound-force/homebrew-tap`: (a) Download the v3.2.0 `checksums.txt` from the GitHub Release and extract the darwin checksums. (b) Push a corrected `Casks/dewey.rb` with the signed darwin checksums. (c) Verify via task 6.2 that `brew install` succeeds. Rollback path: `git revert` on the homebrew-tap commit if the fix is wrong. v3.1.0 is not remediated — it is superseded by v3.2.0.

## 5. Cross-Repo Follow-Up

- [x] 5.1 [P] File a GitHub issue on `unbound-force/unbound-force` noting that the same awk patching logic is latently vulnerable to GoReleaser template ordering changes, and recommending the same order-agnostic fix.
- [x] 5.2 [P] File a GitHub issue on `unbound-force/dewey` (or close #67) linking to this fix.
- [x] 5.3 [P] File a GitHub issue on `unbound-force/replicator` if it uses the same awk-based cask patching pattern, recommending the same order-agnostic fix.

## 6. Verify Constitution Alignment

- [x] 6.1 Confirm Observable Quality: the verification step (task 3.1) fails loudly on checksum mismatch before pushing to the tap. Verified: the implementation checks for signed hash presence and original hash absence, exits non-zero with descriptive error on any failure.
- [x] 6.2 Confirm Composability First: `brew install unbound-force/tap/dewey` succeeds after remediation (task 4.1). Verified: v3.2.0 cask in homebrew-tap corrected with signed checksums from checksums.txt (commit 7677217).

<!-- spec-review: passed -->
<!-- code-review: passed -->
