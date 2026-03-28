# Homebrew Dependency Spec

> Synced from: `openspec/changes/fix-ollama-cask-rename/specs/homebrew-dependency.md`
> Date: 2026-03-28

## Requirements

### Requirement: Homebrew Cask Ollama Dependency

The GoReleaser `homebrew_casks` configuration MUST declare the Ollama dependency using the canonical cask name `ollama-app`. The generated `Casks/dewey.rb` MUST NOT reference the deprecated `ollama` cask name.

#### Scenario: Clean install of Dewey cask
- **GIVEN** a user has tapped `unbound-force/tap` and Ollama is not installed
- **WHEN** the user runs `brew install --cask unbound-force/tap/dewey`
- **THEN** Homebrew MUST install `ollama-app` as a dependency without emitting a rename warning

#### Scenario: Install when Ollama is already present
- **GIVEN** a user already has `ollama-app` installed via Homebrew
- **WHEN** the user runs `brew install --cask unbound-force/tap/dewey`
- **THEN** Homebrew MUST recognize the dependency as satisfied and MUST NOT emit a rename warning

### Requirement: Documentation Accuracy for Ollama Installation

All user-facing documentation MUST reference `ollama-app` as the Homebrew cask name when providing install instructions. References to `brew install --cask ollama` MUST be updated to `brew install --cask ollama-app`.

#### Scenario: User follows README install instructions
- **GIVEN** a user is reading the Dewey README for setup instructions
- **WHEN** they reach the Ollama installation section
- **THEN** the documented command MUST use the canonical cask name `ollama-app`
