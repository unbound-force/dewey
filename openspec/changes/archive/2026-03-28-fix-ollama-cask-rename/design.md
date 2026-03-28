## Context

Homebrew has renamed the `ollama` cask to `ollama-app`. Dewey's GoReleaser configuration (`.goreleaser.yaml`) declares `cask: ollama` as a dependency in the `homebrew_casks` section. This causes a deprecation warning during installation:

```
Warning: Cask ollama was renamed to ollama-app.
```

The GoReleaser config generates a `Casks/dewey.rb` file that is pushed to the `unbound-force/homebrew-tap` repository by the release workflow. The dependency name flows through the entire pipeline: GoReleaser config -> generated cask -> tap repo -> end-user install.

## Goals / Non-Goals

### Goals
- Eliminate the Homebrew deprecation warning during `brew install --cask unbound-force/tap/dewey`
- Update all documentation referencing the old `ollama` cask name
- Ensure the next release automatically propagates the fix to `unbound-force/homebrew-tap`

### Non-Goals
- Changing the Ollama runtime integration (the `embed/embed.go` HTTP client is unaffected)
- Modifying the release workflow pipeline (signing, notarization, checksum patching all remain unchanged)
- Manually updating `unbound-force/homebrew-tap` (the release pipeline handles this automatically)

## Decisions

**D1: Update `.goreleaser.yaml` dependency from `ollama` to `ollama-app`**

This is the single source of truth for the cask dependency. Line 37 changes from `- cask: ollama` to `- cask: ollama-app`. The release workflow will propagate this to the tap repo on the next tagged release.

**D2: Update README.md install instructions**

The README references Ollama installation. Any references to `brew install ollama` or `brew install --cask ollama` should be updated to `ollama-app` to match the canonical Homebrew name.

**D3: No changes to the release workflow**

The `sign-macos` job in `.github/workflows/release.yml` downloads, patches, and pushes the generated cask file. It does not hardcode the `ollama` dependency name — it works with whatever GoReleaser generates. No workflow changes are needed.

## Risks / Trade-offs

**Low risk**: This is a configuration-only change. The GoReleaser `homebrew_casks` section generates Ruby code for the cask file. Homebrew's dependency resolution already understands `ollama-app` (it currently resolves the old name via a rename redirect). Switching to the new name eliminates the redirect and the warning.

**Propagation delay**: The fix reaches end users only after a new tagged release is cut. Until then, existing `Casks/dewey.rb` in the tap repo still references `ollama`. This is acceptable — there's no urgency since the old name still resolves (with a warning).
