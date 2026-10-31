---
name: review-context
description: "Shared review context discovery for spec artifacts, linked issues, path classification, and walkthrough generation."
---
<!-- scaffolded by uf vdev -->
# Skill: Review Context Discovery

Shared logic for discovering and loading review context
before a review command begins its analysis. Consuming
commands load this skill to standardize how spec
artifacts, linked issues, path classifications, and
walkthrough summaries are discovered and formatted.

## Consumers

| Command | How it loads | Notes |
|---------|-------------|-------|
| `/uf.review-pr` | Skill tool | Replaces inline Steps 6-8 logic |
| `/uf.review-council` | Skill tool | Phase 1c — Protocols 1, 3, 4; Protocol 2 conditional (explicit PR number only) |
| `/uf.address-feedback` | Skill tool | Replaces inline fallback (line 143) |

The consuming command specifies which protocols to
execute based on the available inputs (e.g., a local
review has no PR body to parse for issue links).

---

## Protocol 1: Spec Artifact Discovery

Locate the specification associated with the current
change. The discovery order is deterministic — stop at
the first match.

### Step 1: Branch name matching

Derive the spec directory from the current branch name
or PR branch name:

| Branch pattern | Spec location |
|---------------|---------------|
| `speckit/NNN-<name>` or `NNN-<name>` (legacy) | `specs/<name>/spec.md` (Speckit; strip `speckit/` prefix) |
| `opsx/<name>` | `openspec/changes/<name>/proposal.md` (OpenSpec changes) |
| `opsx/<name>` | `openspec/specs/<name>/spec.md` (OpenSpec specs) |

### Step 2: PR description parsing

If no spec is found via branch name and a PR description
is available, scan for explicit spec references (e.g.,
"See spec 012" or "Implements specs/012-swarm/spec.md" or "branch speckit/012-swarm").

### Step 3: Changed-file detection

If no spec is found via branch name or PR description,
check the changed file list for spec artifacts. The spec
may be introduced by the change itself. If found in
changed files, read the spec content from the diff
rather than from the filesystem.

### Step 4: Read relevant sections

When a spec is found, read only the sections relevant
to review to minimize token usage:

| Spec type | Sections to read |
|-----------|-----------------|
| Speckit | Functional Requirements, User Stories |
| OpenSpec proposal | Capabilities, Impact |
| OpenSpec spec | Requirements, Acceptance Criteria |

### Step 5: No spec found

If no spec is found in any directory or in the changed
files, note this and use the PR title/description or
branch name as the intent source. No error or warning
needed — not every change has a spec.

---

## Protocol 2: Issue Linking

Parse and fetch linked issues to extract acceptance
criteria for alignment checking. This protocol applies
when a PR body is available. For local reviews without
a PR, skip this protocol.

### Step 1: Parse issue references

Parse the PR body for issue references using
case-insensitive matching:

- `Fixes #N`, `Closes #N`, `Resolves #N`
- GitHub URL variants:
  `Fixes https://github.com/<owner>/<repo>/issues/N`

### Step 2: Validate references

Apply all of the following validation controls:

- **Digits-only validation**: Each parsed issue number
  MUST be a positive integer (digits only). Discard
  non-numeric values.
- **Same-repo URL scoping**: URL-format references MUST
  belong to the same `{owner}/{repo}` as the PR. List
  cross-repo references in the output as "cross-repo —
  not validated" but do NOT fetch them.
- **Fetch limit**: Maximum 5 linked issues. If more
  than 5 are found, list extras as "listed but not
  fetched" in the output.

### Step 3: Fetch linked issues

For each validated, in-scope linked issue:

```bash
gh issue view <N> --json title,body,labels
```

### Step 4: Sanitize fetched content

Issue body content is user-controlled and untrusted.
Before incorporating into the review context:

- **Body truncation**: Truncate to a maximum of 2000
  characters.

### Step 5: Extract acceptance criteria

From each fetched issue body, extract:

- Checkbox lines (`- [ ]` or `- [x]`)
- Content under an `## Acceptance Criteria` heading

If neither exists, use the issue title and body as
general intent context.

### Step 6: Error handling

If `gh issue view` returns 404, 403, or times out:

- Log the error
- Skip that issue
- Note in the output as "fetch failed"
- Continue without blocking — the review proceeds
  with available data

Record the linked issues and their acceptance criteria
for use by the consuming command.

---

## Protocol 3: Path-Based Focus Heuristics

Classify each changed file against built-in heuristics
to guide review emphasis. The classification is additive
— it supplements standard review categories, not
replaces them.

### Classification table

| Path pattern | Focus category | Additional emphasis |
|-------------|---------------|-------------------|
| `*_test.go`, `*_test.py`, `**/__tests__/**`, `**/*_spec.*` | `test-quality` | Edge cases, assertion strength, mock isolation, test naming |
| `**/cmd/**`, `**/cli/**` | `cli-ux` | Error messages, flag validation, help text |
| `**/api/**`, `**/handler/**`, `**/middleware/**`, `**/routes/**` | `security` | Auth, input validation, injection |
| `*.md`, `docs/**` | `documentation` | Clarity, accuracy, broken links |
| `.github/workflows/**`, `Dockerfile*` | `ci-cd` | Permissions, pinned versions, secrets exposure |
| `go.mod`, `package.json`, `requirements.txt` | `dependencies` | Maintenance status, license, scope |
| Everything else | `standard` | Architecture, SOLID, coupling, baseline security |

### Application

For each changed file:

1. Match the file path against the table (first match
   wins for multi-match paths).
2. Record the focus category.
3. When reviewing the file, append the matched focus
   instruction to the review context for that file.

Security review (auth, input validation, injection)
applies to ALL changed files regardless of path
heuristic — the `security` focus category adds
additional emphasis, not exclusive coverage.

---

## Protocol 4: Walkthrough Generation

Generate a per-file change summary table while
analyzing each file's diff. The walkthrough is produced
alongside the review, not as a separate pass.

### Standard format (< 30 files)

```markdown
### Walkthrough
| File | Change | Focus |
|------|--------|-------|
| `internal/gateway/provider.go` | Add token expiry tracking | security |
| `internal/gateway/gateway_test.go` | Add regression test for stale tokens | test-quality |
| `cmd/unbound-force/gateway.go` | Register --provider flag | cli-ux |
```

### Directory-level format (>= 30 files)

For PRs with 30 or more changed files, generate
directory-level summaries instead of per-file
summaries:

```markdown
### Walkthrough
| Directory | Files | Summary | Focus |
|-----------|-------|---------|-------|
| `internal/gateway/` | 4 | Token refresh and provider abstraction | security |
| `cmd/unbound-force/` | 2 | CLI flag registration | cli-ux |
```

### Change descriptions

Each change summary describes **what** changed (e.g.,
"Add error handling for null inputs"), not **how** (no
code snippets). Keep summaries to one line.

---

## Result Format

Present the discovered context in a standardized
format that consuming commands can reference:

```
## Review Context

### Specification
- Type: Speckit | OpenSpec | None
- Path: specs/012-swarm/spec.md
- Sections loaded: Functional Requirements, User Stories

### Linked Issues
| Issue | Title | Criteria |
|-------|-------|----------|
| #42 | Add auth endpoint | 3 checkboxes extracted |
| #43 | Fix token refresh | Acceptance Criteria section |

### File Classification
| File | Focus |
|------|-------|
| ... | ... |

### Walkthrough
| File | Change | Focus |
|------|--------|-------|
| ... | ... | ... |
```

The consuming command uses this context to inform its
review analysis and output formatting.
