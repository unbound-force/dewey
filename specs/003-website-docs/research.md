# Research: Website Documentation for Dewey

## Decision 1: Update Existing Pages vs. Create New

**Decision**: Update existing `knowledge.md` and `team/dewey.md` pages; create only `projects/dewey.md` as a new file.

**Rationale**: The website already has placeholder/partial content for Dewey in `knowledge.md` and `team/dewey.md` from earlier integration work (Phase 4.1, PR #52). Replacing these with fresh content would lose any links or references other pages have to them. Updating preserves URL stability and builds on existing structure.

**Alternatives considered**:
- Create all-new pages: Rejected because existing pages already have correct Hugo frontmatter, correct `weight` values for sidebar ordering, and are referenced by other pages' "Next Steps" sections.

## Decision 2: Content Scope — v0.2.0 Only

**Decision**: Document only features available in Dewey v0.2.0. No forward-looking content about autonomous define (Phase 5) or planned Gaze classifier improvements.

**Rationale**: FR-020 requires zero references to unimplemented features. The Phase 5 autonomous define workflow is not started. Documenting it would create expectations that can't be met. The website can be updated again when Phase 5 ships.

**Alternatives considered**:
- Include a "Roadmap" section mentioning Phase 5: Rejected because roadmap content goes stale and creates maintenance burden. Better to document what exists.

## Decision 3: Linux Installation Path

**Decision**: Document `go install github.com/unbound-force/dewey@latest` as the Linux alternative to Homebrew cask.

**Rationale**: Homebrew cask (`brew install --cask`) is macOS-only. Linux users need an alternative. `go install` is the standard Go distribution mechanism and works on any platform with Go installed. Pre-built binaries are also available from GitHub Releases but `go install` is the simplest single-command path.

**Alternatives considered**:
- Direct binary download from GitHub Releases: Valid but more steps (download, extract, move to PATH). Mention as a secondary option.
- Snap/Flatpak: Rejected — not set up, not worth the maintenance for a developer tool.

## Decision 4: Dewey Query Examples — Real vs. Hypothetical

**Decision**: Use realistic but hypothetical query examples that illustrate Dewey's capabilities without requiring specific repository content.

**Rationale**: Real queries against the Unbound Force repos would be accurate but would not generalize to other users' repositories. Hypothetical examples like "How does Cobra handle subcommand validation?" are understandable by any Go developer and demonstrate the semantic search concept clearly.

**Alternatives considered**:
- Screenshots of actual query results: Rejected because they go stale with every release and are harder to maintain than text examples.

## Decision 5: Projects Page Format

**Decision**: Model `projects/dewey.md` after the existing `projects/gaze.md` page structure for consistency.

**Rationale**: The projects section has an established format (project description, installation, key features, link to guide). Following the same structure ensures visual consistency in the sidebar and meets user expectations set by the Gaze project page.

**Alternatives considered**:
- Unique format for Dewey: Rejected — consistency in a documentation site is more important than per-page optimization.
