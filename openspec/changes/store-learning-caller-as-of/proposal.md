# Proposal: Caller-Supplied `as_of` on `store_learning`

## Why

`store_learning` always pins `as_of` to the vault's git HEAD
(`<repo>@<sha>`, issue #146). That is right for a learning about the
vault's own code. It is wrong for a learning about something else: an
agent researching `owner/project` at commit `abc1234` stores a
learning whose `as_of` names the research vault's HEAD, which says
nothing about the code the learning describes. Today the only way to
pin the observed source is to write the learning file directly
(the `kb_to_dewey.py` pattern) and wait for re-ingestion.

## What Changes

1. **`StoreLearningInput.AsOf`** (`types/tools.go`): optional
   `as_of` string.
2. **Validation** (`tools/learning_temporal.go`): the value is trimmed
   and must match `owner/repo@<sha>` or `repo@<sha>` with a 7-64
   character hex SHA (`callerAsOfPattern`). Anything else (a branch
   name, a URL, an option-looking string) is rejected before anything
   is written, like the other temporal inputs.
3. **Precedence** (`prepareTemporal`): a valid caller `as_of` is
   stored as given; the git-HEAD resolver runs only when none is
   supplied, so the default is unchanged and a caller pin never shells
   out to git.
4. **Docs**: tool description, README Temporal Validity table and tool
   table, AGENTS.md Store Learning API.

`repo@<sha>` is accepted alongside `owner/repo@<sha>` because it is the
shape Dewey itself stamps: a caller can pin a learning to an earlier
commit of the vault and keep anchor checks working.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `learning-temporal-validity` — As-Of Pinning: `as_of` MAY be supplied
  by the caller; the git-HEAD stamp is the default.

### Removed Capabilities
- None.

## Impact

- **Files**: `types/tools.go`, `tools/learning_temporal.go`,
  `server.go` (description), `tools/learning_temporal_test.go`,
  `README.md`, `AGENTS.md`.
- **API**: one additive optional field. No schema, file-format,
  frontmatter or CLI change; the stored `as_of` has the same key and
  string type.
- **Lint / verify**: `splitAsOf` already splits on the last `@`. A pin
  whose repository part is not the vault's base name (every
  `owner/repo@sha` pin) is treated as pinned to another repository:
  `anchor_drift` / `anchor_missing` skip it and `verify_candidates`
  reports why. No change needed.
- **Website**: MCP tool parameter added — docs sync issue required.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The pin travels in the learning's own properties and frontmatter, so
any consumer can tell which source and commit a learning describes
without asking the agent that stored it.

### II. Composability First

**Assessment**: PASS

Optional field; omitted, behavior is byte-identical to today. No new
dependency or configuration.

### III. Observable Quality

**Assessment**: PASS

Provenance gets more accurate: the pin names the code the learning was
observed in. Malformed pins are rejected with a message naming the
expected shape rather than stored.

### IV. Testability

**Assessment**: PASS

Validation is a pure function with a table test; precedence is tested
with an injected resolver that counts calls, against an in-memory store
and a temp vault.
