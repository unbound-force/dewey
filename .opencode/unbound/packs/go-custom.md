---
pack_id: go-custom
language: Go
version: 1.0.0
---
<!-- scaffolded by unbound vdev -->

# Custom Rules: Go

Project-specific Go conventions that extend the canonical
Go convention pack. Rules in this file are loaded alongside
`go.md` by Cobalt-Crush (during implementation) and
all Divisor persona agents (during review).

Use the `CR-NNN` prefix for all custom rules. Use `[MUST]`,
`[SHOULD]`, or `[MAY]` severity indicators per RFC 2119.

## Custom Rules

<!-- Add project-specific rules below this line -->

- **CR-001** [MUST] **Override AP-007**: Dewey retains the flat package layout inherited from the graphthulhu fork. Packages live at the repository root level (`store/`, `embed/`, `source/`, `vault/`, `tools/`, etc.) rather than under `internal/` and `cmd/`. The main package files (`main.go`, `cli.go`, `server.go`) remain in the repository root. This deviation from AP-007 is intentional — restructuring the entire codebase to `internal/`/`cmd/` would be a separate concern from the core implementation and would break backward compatibility with the upstream fork. See `plan.md` Complexity Tracking for rationale.
