## MODIFIED Requirements

### Requirement: default-backend

The default backend MUST be `obsidian` instead of
`logseq`.

#### Scenario: no backend specified

- **GIVEN** no `--backend` flag and no `DEWEY_BACKEND`
  env var
- **WHEN** `dewey serve` or `dewey search` runs
- **THEN** the obsidian backend is used

#### Scenario: logseq explicitly requested

- **GIVEN** `--backend logseq` is specified
- **WHEN** `dewey serve` runs
- **THEN** the logseq backend is used (backward compat)
