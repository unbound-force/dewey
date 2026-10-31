---
pack_id: review-voice
language: Any
version: 1.0.0
---
<!-- scaffolded by uf vdev -->

# Convention Pack: Review Voice

This is the review voice convention pack for the Unbound Force
agent ecosystem. It defines the default voice — sentence style,
framing, and tone — for GitHub-posted review output: finding
descriptions, root causes, summaries, verdict justifications,
inline comments, review bodies, triage comments, and feedback
replies.

This is a non-coding pack. It is always deployed regardless of
detected project language and loads alongside `default.md`, the
severity and content packs, and any language-specific pack. The
review commands (`uf.review-pr`, `uf.review-council`,
`uf.triage-issue`, `uf.address-feedback`) and dispatched child
reviewers apply it when composing GitHub-posted prose. Voice
rules are style-only: they never change what a review
concludes, only how its prose reads.

---

## Applicability

- **RV-001** [MUST] This pack governs the prose style of
  GitHub-posted review output: finding descriptions, root
  causes, summaries, verdict justifications, inline comments,
  review bodies, triage comments, and feedback replies.

- **RV-002** [MUST] This pack MUST NOT govern CI tables,
  walkthrough tables, tool output, severity labels, verdict
  values, or structural headers. These elements keep their
  tool-defined formats unchanged.

- **RV-003** [MUST] Terminal reports and code inside
  suggestion blocks are out of scope. Voice applies to the
  prose surrounding code, never to the code itself or to
  fixed-format terminal output.

---

## Style-Only Precedence

- **RV-004** [MUST] Voice rules are style-only. They MUST NOT
  alter severities, verdicts, finding counts, required
  footers, tone-tier content requirements, reply-template
  required elements, top-15 inline comment selection,
  suggestion-block format, or body size limits.

- **RV-005** [MUST] Where a voice rule conflicts with a
  structural or content requirement, the structural or
  content requirement prevails. The voice rule MUST yield for
  that element and remain in effect for the surrounding
  prose.

- **RV-006** [MUST] Custom voice rules in
  `review-voice-custom.md` override these defaults and
  inherit these precedence constraints. Custom rules MAY
  change voice — word choice, sentence shape, framing
  habits — but MUST NOT weaken, bypass, or disable any
  structural, content, or gate requirement.

---

## Neutral Voice Defaults

- **RV-007** [MUST] Review prose MUST be evidence-first:
  cite the observable basis — file, line, command output,
  or a spec/convention reference — before interpretation or
  recommendation.

- **RV-008** [MUST] Finding prose MUST be problem-first:
  state the defect or risk in the opening sentence, then
  add context. Background MUST NOT precede the point.

- **RV-009** [MUST] Sentences MUST use present tense and
  active voice: "the handler skips the error check," not
  "the error check was skipped" or "the handler will skip
  the check."

- **RV-010** [MUST] Framing MUST describe the code and its
  impact, not the author. Reference the artifact ("this
  function", "the test"); second-person blame ("you
  forgot", "you failed to") MUST NOT appear.

- **RV-011** [MUST] Review prose MUST NOT editorialize.
  Value judgments ("terrible", "sloppy", "hacky") MUST
  NOT appear; every qualitative claim MUST be tied to a
  concrete impact or a cited rule.

- **RV-012** [MUST] Tone MUST be direct, respectful, and
  non-sarcastic. Sarcasm, mockery, and condescension MUST
  NOT appear in any review prose.

- **RV-013** [MUST] Hedging boilerplate ("I might be
  wrong, but...", "it seems like maybe...") MUST NOT pad
  review prose. State observations directly; express
  genuine uncertainty as a specific question that names
  the evidence that would resolve it.

---

## Custom Rules

<!-- This section is intentionally empty in the canonical
     pack. Project-specific custom rules belong in
     review-voice-custom.md alongside this file. Custom
     rules use the CR-NNN identifier prefix. -->
