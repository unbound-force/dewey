---
pack_id: ci
language: Any
version: 1.0.0
---
<!-- scaffolded by uf vdev -->

# Convention Pack: CI (GitHub Actions Workflows)

This is the CI convention pack for the Unbound Force
agent ecosystem. It contains rules for GitHub Actions
workflow files: action pinning, supply chain integrity,
workflow structure, permissions, and reusable workflow
design. It complements the language-agnostic `default.md`
pack with CI-specific standards.

When this pack is active, agents load it alongside
`default.md` and any language-specific pack. The CI pack
is always deployed regardless of detected project
language, because every project may have CI workflows.

---

## Action Pinning & Supply Chain

- **CI-001** [MUST] Every `uses:` reference for actions
  and reusable workflows in GitHub Actions workflow files
  MUST be pinned to a full 40-character commit SHA.
  Docker container references (`docker://image:tag`) are
  excluded; they SHOULD be pinned to image digests
  (`@sha256:...`) when available but are not subject to
  this rule.

- **CI-002** [MUST] SHAs MUST be verified against the
  source repository at authoring time. The agent MUST
  resolve the desired tag first, then derive the SHA
  from the API response. When the API response object
  type is `tag` (annotated tag), the agent MUST
  dereference to get the commit SHA by following the
  tag object's target. SHAs MUST NOT be sourced from
  memory or training data. When verification cannot be
  completed (network unavailable, API rate limit,
  missing authentication, tag not found), the agent
  MUST NOT write the unverified `uses:` reference and
  MUST report the verification failure.

- **CI-003** [SHOULD] A version comment (e.g.,
  `# v6.0.2`) SHOULD follow the SHA for human
  readability. Missing comments are a warning, not a
  failure. When a version comment IS included, it MUST
  accurately reflect the tag used to derive the SHA.

---

## Workflow Structure

- **CI-010** [MUST] Reusable workflows MUST be prefixed
  with `reusable_` and have a clear, descriptive name
  reflecting their function (e.g.,
  `reusable_vuln_scan.yml`). Consumer workflows MUST
  be prefixed with `ci_` (e.g., `ci_security.yml`).

- **CI-011** [MUST] Workflow files MUST include a header
  comment block describing the workflow's purpose. The
  comment SHOULD appear before or immediately after the
  `name:` key.

- **CI-012** [SHOULD] Workflows SHOULD use concurrency
  groups to prevent redundant runs on the same branch
  or pull request. Cancel-in-progress SHOULD be enabled
  for non-default branches.

---

## Permissions & Secrets

- **CI-020** [MUST] Workflows MUST follow the principle
  of least privilege. Write permissions MUST be avoided;
  when necessary, they MUST be defined in the minimal
  possible scope. Prefer explicit `permissions` blocks
  over relying on the default token permissions.

- **CI-021** [SHOULD] Permissions SHOULD be defined at
  the job level rather than the workflow level, so that
  each job receives only the permissions it requires.

- **CI-022** [MUST] Secrets MUST NOT be hardcoded in
  workflow files. Use GitHub Secrets or environment-based
  injection. Secrets MUST be scoped to the narrowest
  context needed.

---

## Reusable Workflow Design

- **CI-030** [MUST] Workflow inputs MUST have descriptive
  `description` fields. Required inputs MUST be marked
  with `required: true`.

- **CI-031** [SHOULD] Optional inputs SHOULD provide
  sensible `default` values aligned with the most common
  use case (Convention Over Configuration).

- **CI-032** [MUST] Reusable workflows MUST be generic
  enough to be consumed by any repository. They MUST
  NOT contain hardcoded org-specific or repo-specific
  values that would prevent reuse.

---

## Custom Rules

<!-- This section is intentionally empty in the canonical
     pack. Project-specific custom rules belong in
     ci-custom.md alongside this file. Custom rules
     use the CR-NNN identifier prefix. -->
