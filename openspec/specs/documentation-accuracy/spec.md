# Documentation Accuracy Spec

## ADDED Requirements

_None_

## MODIFIED Requirements

### Requirement: README Environment Variables Accuracy

The README.md environment variables table MUST reflect the actual code defaults. `DEWEY_BACKEND` MUST show `obsidian` as the default (not `logseq`).

#### Scenario: Developer reads environment variables table
- **GIVEN** a developer reads the README.md environment variables section
- **WHEN** they check the `DEWEY_BACKEND` default value
- **THEN** it MUST say `obsidian`, matching the actual code in `resolveBackendType()`

### Requirement: Ollama Installation Command Accuracy

All references to Ollama Homebrew installation MUST use `ollama-app` (the current cask name), not the deprecated `ollama`.

#### Scenario: Developer follows install instructions
- **GIVEN** a developer reads the README.md Ollama install section
- **WHEN** they copy the Homebrew command
- **THEN** the command MUST be `brew install --cask ollama-app` (no deprecation warning)

### Requirement: AGENTS.md CLI Command Completeness

AGENTS.md MUST list all available CLI commands and their flags, reflecting the current v1.4.1 feature set.

#### Scenario: AI agent reads AGENTS.md for context
- **GIVEN** an AI agent reads AGENTS.md
- **WHEN** it looks for available Dewey commands
- **THEN** it MUST find documentation for `dewey doctor`, `dewey reindex`, `dewey search`, and the global `--verbose`, `--log-file`, `--no-embeddings` flags

### Requirement: Spec Status Accuracy

Completed spec artifacts MUST have their status field updated to "Complete" when all tasks are done and merged.

#### Scenario: Agent checks spec status
- **GIVEN** spec 004 has all tasks completed and changes merged
- **WHEN** the spec.md status field is read
- **THEN** it MUST say "Complete" (not "Draft")

## REMOVED Requirements

_None_
