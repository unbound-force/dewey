<!--
Requirement numbering: this change allocates DVE-FR-001 for the
doctor-vertex-embedding capability; DVE-FR-002 through DVE-FR-010
are reserved for future changes to the same capability. Sibling
diagnostics capabilities keep their requirements in their own
change deltas (for example, doctor-synthesis-section under the
doctor-synthesis-diagnostics change); capabilities with a main
spec live under openspec/specs/.
-->
## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Provider-Aware Vertex Embedding Diagnostics

Requirement DVE-FR-001: provider-aware Vertex embedding
diagnostics.

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

## REMOVED Requirements

_None._
<!-- scaffolded by uf vdev -->
