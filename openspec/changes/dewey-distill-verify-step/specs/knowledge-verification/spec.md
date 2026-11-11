# Delta Spec: knowledge-verification

<!--
  Requirement numbering allocation
  ================================
  Capability: knowledge-verification (prefix KV)
  Allocated to this change: KV-FR-001 .. KV-FR-005
  Reserved for future knowledge-verification changes:
    KV-FR-006 .. KV-FR-099
  Sibling capabilities:
    - knowledge-distillation (prefix KD): the command-side verify
      step and review-gate presentation are KD-FR-104 .. KD-FR-107
      in openspec/changes/dewey-distill-verify-step/specs/
      knowledge-distillation/spec.md. KD-FR-001 .. KD-FR-099 are
      reserved for the pending dewey-distill change's archive-time
      numbering; KD-FR-101 .. KD-FR-103 belong to the
      curator-agent-human-gates change.
    - curator-agent (prefix CA): the curator allowlist extension is
      CA-FR-013 .. CA-FR-014 in this change. CA-FR-001 .. CA-FR-008
      belong to the curator-agent-human-gates change, and
      CA-FR-009 .. CA-FR-012 to the curator-tool-allowlist change.
  This capability reuses the anchor-check mechanism specified by the
  lint-anchor-drift change (openspec/changes/lint-anchor-drift:
  `anchorChecker`, `learningAnchors`, `anchor_missing`,
  `anchor_drift`, `anchor_checks`); it allocates no ids there.
  The capability has no archived main spec, so this delta uses
  ADDED requirements. Requirement ids MUST stay inside the
  allocated range.
-->

## ADDED Requirements

### Requirement: Candidate Verification Tool

**ID**: KV-FR-001

Dewey MUST provide a `verify_candidates` MCP tool that
deterministically verifies the promotable candidate queue without
mutating anything. The candidate queue is every page with tier
`draft` in the `learning/`, `compiled/`, and `knowledge/`
namespaces — the same namespaces `promote` resolves against, with
the same tier-based eligibility, so a draft page in any of the
three is a candidate. The tool
MUST accept optional `pages` and optional `tags`, and MUST accept
at most one of them: supplying both MUST be a validation error.
`pages` names explicit candidates; `tags` restricts the queue to
the named tags; with neither, the tool verifies every candidate.
Tag membership MUST be read from the page-name prefixes
(`learning/{tag}-*`, `compiled/{tag}`) and the `tag` property, and
MUST NOT be inferred from prose. Tag matching MUST be exact: the
`tag` property MUST equal the requested tag, a page-name match
MUST carry the delimiter after the tag (`learning/{tag}-…`), and
`knowledge/` page names carry no tag prefix, so they MUST match
only through the `tag` property. A tag MUST NOT match by substring
or by prefix without its delimiter. Duplicate page names MUST be
deduplicated.

#### Scenario: Scope by tag
- **GIVEN** draft learning pages and draft compiled articles for
  the tags `auth` and `gateway`
- **WHEN** `verify_candidates` is called with `tags: ["auth"]`
- **THEN** only candidates matching `auth` MUST appear in the
  result

#### Scenario: Explicit pages
- **GIVEN** three draft candidates
- **WHEN** `verify_candidates` is called with `pages` naming two
  of them
- **THEN** the result MUST contain exactly those two candidates

#### Scenario: Both selectors supplied
- **WHEN** `verify_candidates` is called with non-empty `pages`
  and non-empty `tags`
- **THEN** it MUST return a validation error without verifying
  anything

#### Scenario: Default queue
- **GIVEN** draft and `validated` pages under `learning/`,
  `compiled/`, and `knowledge/`
- **WHEN** `verify_candidates` is called with neither selector
- **THEN** it MUST include every draft candidate from all three
  namespaces
- **AND** MUST NOT include a `validated` page
- **AND** MUST NOT include a `curated`-tier `knowledge/` page

#### Scenario: Draft knowledge page is a candidate
- **GIVEN** a `knowledge/team/file` page with tier `draft` and a
  `knowledge/team/other` page with tier `curated`
- **WHEN** `verify_candidates` is called with neither selector
- **THEN** the draft page MUST appear as a candidate
- **AND** the `curated` page MUST NOT appear

#### Scenario: Tag matching is exact
- **GIVEN** draft learnings for the tags `auth` and
  `authentication`
- **WHEN** `verify_candidates` is called with `tags: ["auth"]`
- **THEN** only the `auth` candidates MUST appear
- **AND** the `authentication` candidates MUST NOT match

#### Scenario: Knowledge page matches via the tag property
- **GIVEN** a draft `knowledge/team/file` page whose `tag`
  property is `auth`
- **WHEN** `verify_candidates` is called with `tags: ["auth"]`
- **THEN** that page MUST appear as a candidate
- **AND** a draft `knowledge/` page without a matching `tag`
  property MUST NOT appear

#### Scenario: Duplicate pages
- **WHEN** `pages` names the same page twice
- **THEN** the candidate MUST appear once

### Requirement: Per-Candidate Output Contract

**ID**: KV-FR-002

The result MUST report a pass status of `checked`, `skipped`, or
`failed` and, when not `checked`, a reason. For every candidate the
result MUST include the resolved page name, the learning identity
when the candidate is a learning, the tier, the evidence locators,
and one verification result from the fixed enum: `anchors_ok`,
`drifted`, `missing`, `no_anchors`, or `skipped`. A `skipped`
candidate MUST carry a reason. The result MUST include summary
counts per result. A requested page that cannot be resolved or is
not `draft` MUST appear as a `skipped` candidate with a reason and
MUST NOT be silently dropped. A candidate with evidence but no
locator checkable against the vault's repository MUST be
`no_anchors`, not an error. The result MUST NOT express or imply
endorsement: `anchors_ok` states only that the cited paths exist
at the pin and are unchanged at `HEAD`, never that the claim is
correct, worth acting on, or safe to trust.

#### Scenario: Candidate shape
- **GIVEN** a draft learning with `as_of` and evidence locators
- **WHEN** the tool verifies it
- **THEN** its entry MUST carry the page name, identity, tier,
  evidence locators, and a result from the enum
- **AND** the result MUST carry summary counts

#### Scenario: Requested page missing
- **GIVEN** a `pages` entry that matches no stored page
- **WHEN** the tool runs
- **THEN** that entry MUST appear as `skipped` with a reason
- **AND** it MUST NOT be dropped from the result

#### Scenario: Requested page not draft
- **GIVEN** a requested page whose tier is `validated`
- **WHEN** the tool runs
- **THEN** that entry MUST appear as `skipped` with a reason

#### Scenario: No checkable anchor
- **GIVEN** a draft learning with no `as_of` pin
- **WHEN** the tool verifies it
- **THEN** its result MUST be `no_anchors`
- **AND** the pass MUST remain `checked`

### Requirement: Anchor Reuse and Provenance Aggregation

**ID**: KV-FR-003

The tool MUST reuse the anchor-check mechanism from the
lint-anchor-drift change rather than reimplementing git access:
learning eligibility MUST match `learningAnchors`, and every check
MUST go through the `anchorChecker` interface. A learning whose
cited path exists at its pin and is unchanged at `HEAD` is
`anchors_ok`; a path absent at the pin, or a pin not in the
repository, is `missing`; a path changed or removed between the pin
and `HEAD` is `drifted`. A compiled article carries `sources`
(learning identities), not its own pin; its result MUST aggregate
its source learnings' results: `missing` if any source is
`missing`, else `drifted` if any source is `drifted`, else
`anchors_ok` if any source is `anchors_ok`, else `no_anchors`. Its
presented evidence MUST be the union of its source learnings'
evidence locators, deduplicated. Its entry MUST list each source
learning identity with that source's individual result, so the
aggregate is auditable and a reviewer can see which source
drifted without opening every candidate by hand.

#### Scenario: Cited path changed
- **GIVEN** a draft learning pinned at commit A citing
  `repo:pkg/a.go`, and `pkg/a.go` changed after A
- **WHEN** the tool verifies it
- **THEN** its result MUST be `drifted`

#### Scenario: Cited path absent
- **GIVEN** a draft learning citing `repo:nope.go`, absent at its
  pin
- **WHEN** the tool verifies it
- **THEN** its result MUST be `missing`

#### Scenario: Cited path unchanged
- **GIVEN** a draft learning whose cited path is unchanged since
  its pin
- **WHEN** the tool verifies it
- **THEN** its result MUST be `anchors_ok`

#### Scenario: Compiled article aggregates sources
- **GIVEN** a draft compiled article with two source learnings,
  one `anchors_ok` and one `drifted`
- **WHEN** the tool verifies it
- **THEN** its result MUST be `drifted`
- **AND** its evidence MUST be the union of the sources' evidence
- **AND** its entry MUST list both source identities with their
  individual results

#### Scenario: Compiled article with no resolvable source
- **GIVEN** a draft compiled article whose `sources` name no
  stored learning
- **WHEN** the tool verifies it
- **THEN** its result MUST be `no_anchors`

### Requirement: Degraded and Failure Behavior

**ID**: KV-FR-004

When the vault is not a git repository, the pass MUST be `skipped`
with a reason and every candidate MUST carry result `skipped` with
that reason; this is not an error. When git cannot answer, the
request is cancelled, or the pass budget is exhausted, the result
MUST be `failed` with a reason and MUST be an error result
(`IsError`), so a caller cannot read an unrun pass as clean; the
failure routes as an error, not as a shorter success. Candidates
checked before the failure MUST NOT be presented as verified —
they may ride in the error result as context only. A
skipped or failed pass MUST NOT report any candidate as
`anchors_ok`, `drifted`, or `missing`.

#### Scenario: Vault is not a git repository
- **GIVEN** a vault that is not a git repository
- **WHEN** the tool runs
- **THEN** the pass status MUST be `skipped` with a reason
- **AND** every candidate MUST be `skipped`
- **AND** the result MUST NOT be an error result

#### Scenario: Git cannot answer
- **GIVEN** a git failure during the pass
- **WHEN** the tool runs
- **THEN** the pass status MUST be `failed` with a reason
- **AND** the result MUST be an error result

#### Scenario: Request cancelled or budget exhausted
- **GIVEN** a cancelled request or an exhausted pass budget
- **WHEN** the tool runs
- **THEN** the pass MUST stop
- **AND** the pass status MUST be `failed`

#### Scenario: No false clean
- **GIVEN** a skipped or failed pass
- **WHEN** the result is inspected
- **THEN** no candidate MUST be reported as `anchors_ok`,
  `drifted`, or `missing`

### Requirement: Non-Mutation and Determinism

**ID**: KV-FR-005

The tool MUST NOT change any page's tier, validity, content, or
properties, MUST NOT write any file, and MUST NOT call `promote` or
any other mutating tool. It MUST NOT preview or pre-select a
promotion. For the same store content, vault pin, and `HEAD`, two
runs that each complete a `checked` pass MUST return the same
candidates and results, and candidate order MUST be deterministic.
A run that ends `skipped` or `failed` — budget exhaustion, request
cancellation, or git failure — is a failure mode, not a differing
deterministic result, and is excluded from the determinism
guarantee. The tool MUST use local git only, MUST
make no network call, and MUST honor the request context and a
bounded per-pass budget.

#### Scenario: Verification changes nothing
- **GIVEN** a draft candidate in the store
- **WHEN** the tool verifies it
- **THEN** its tier, validity, content, and properties MUST be
  unchanged
- **AND** no file MUST be written
- **AND** `promote` MUST NOT be called

#### Scenario: Deterministic repeat
- **GIVEN** unchanged store content, pin, and `HEAD`
- **WHEN** the tool runs twice
- **THEN** both results MUST contain the same candidates in the
  same order with the same results

#### Scenario: No network
- **WHEN** the tool verifies candidates
- **THEN** it MUST make no network call

#### Scenario: Context cancelled
- **GIVEN** a cancelled request context
- **WHEN** the tool runs
- **THEN** it MUST stop the pass

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
