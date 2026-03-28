## Why

Homebrew renamed the `ollama` cask to `ollama-app`. When users install Dewey via `brew install unbound-force --cask`, the installation succeeds but emits a warning on every operation that touches the dependency:

```
Warning: Cask ollama was renamed to ollama-app.
```

This warning appears multiple times during install (once per dependency resolution pass) and erodes confidence in the installation — users may wonder if the dependency was correctly installed or if something is misconfigured. The fix is straightforward: update the dependency declaration to use the new canonical name.

## What Changes

- Update the Homebrew cask dependency in `.goreleaser.yaml` from `ollama` to `ollama-app`
- Update any documentation that references `brew install ollama` or the old cask name

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `homebrew-cask-install`: Dewey's Homebrew cask declares the correct dependency name (`ollama-app` instead of deprecated `ollama`), eliminating the rename warning during installation

### Removed Capabilities
- None

## Impact

- **`.goreleaser.yaml`**: Change `cask: ollama` to `cask: ollama-app` in the `dependencies` section of `homebrew_casks`
- **`README.md`**: Update any install instructions that reference the old `ollama` cask name
- **`unbound-force/homebrew-tap`** (downstream): The next tagged release will automatically push an updated `Casks/dewey.rb` to the tap with the corrected dependency name — no manual changes to the tap repo are needed
- **Existing users**: Users who already have Ollama installed will see no change. Users doing fresh installs will no longer see the deprecation warning.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change modifies build/distribution configuration only. No MCP tools, tool contracts, or inter-agent communication are affected.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. The Ollama dependency is still optional at runtime (semantic search degrades gracefully without it). This change only corrects the package name used by Homebrew's dependency resolver.

### III. Observable Quality

**Assessment**: N/A

No changes to query results, provenance metadata, health reporting, or index state.

### IV. Testability

**Assessment**: N/A

No production code or test code changes. This is a build configuration fix.
