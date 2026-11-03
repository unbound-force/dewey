## MODIFIED Requirements

### Requirement: Provider-Aware Vertex Embedding Diagnostics

When `embedding.provider` is `vertex`, `dewey doctor`'s Embedding
Layer section MUST report configuration completeness without network
calls and MUST NOT run Ollama checks or suggest `ollama pull`.

Previously: Vertex embedding configuration was diagnosed through the
Ollama reachability and model-pull paths.

#### Scenario: Complete Vertex configuration
- **GIVEN** `embedding.provider: vertex` with model, project, and
  region
- **WHEN** `dewey doctor` runs
- **THEN** the Embedding Layer MUST report PASS for config naming the
  project, region, and model
- **AND** the section MUST NOT mention Ollama

#### Scenario: Incomplete Vertex configuration
- **GIVEN** `embedding.provider: vertex` without a project
- **WHEN** `dewey doctor` runs
- **THEN** the Embedding Layer MUST report FAIL naming the missing
  field
- **AND** the section MUST NOT suggest `ollama pull`
