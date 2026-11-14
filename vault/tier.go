package vault

import "strings"

// Trust-tier derivation for the git-trust-source capability
// (openspec/changes/git-is-the-source-of-record-for-page-trust-tiers): the
// default branch's blob equality is the authority for the validated tier,
// and lowering trust is honored from any file, branch, or local edit.
//
// Design decision (task 1.1): DeriveEffectiveTier is a pure function over
// already-collected signals — frontmatter values and blob object IDs — so
// the git plumbing that produces them (the task 2.x resolver) stays
// injectable and independently testable behind this seam. No I/O, no
// globals, no clock.

// Canonical trust-tier vocabulary per GTS-FR-001. A frontmatter tier value
// matches canonically only when it is exactly equal — case-sensitively,
// with no trimming or other normalization — to one of these lowercase
// strings. Kept unexported so the vocabulary stays local to the ingestion
// package (CS-012); callers outside vault/ keep using string literals
// exactly as before.
const (
	canonicalTierAuthored  = "authored"
	canonicalTierCurated   = "curated"
	canonicalTierValidated = "validated"
	canonicalTierDraft     = "draft"
	canonicalTierUntrusted = "untrusted"
)

// Validity values that lower trust per GTS-FR-003's exhaustive downgrade
// predicate. The canonical validity "current" is deliberately absent: it is
// not a downgrade signal. This is the trust-lowering subset only — the full
// validity vocabulary for learning input validation lives in
// tools/learning_temporal.go (validValidities).
const (
	downgradeValidityStaleUnverified = "stale-unverified"
	downgradeValiditySuperseded      = "superseded"
	downgradeValidityHistorical      = "historical"
)

// EffectiveTierInput carries the signals DeriveEffectiveTier decides from.
// Every field is compared exactly — case-sensitively, with no trimming or
// other normalization — against the canonical lowercase vocabulary.
type EffectiveTierInput struct {
	// FileTier is the page's frontmatter tier value as parsed (for example
	// "validated", "curated", or "draft"). The YAML frontmatter layer
	// already trims plain scalars, so well-formed files parse to exact
	// canonical values; quoted padded values and case variants such as
	// "Validated" are non-canonical and read as unknown.
	FileTier string
	// Validity is the page's validity value, from frontmatter or derived
	// (for example by synthesis). Only the exact values
	// "stale-unverified", "superseded", and "historical" lower trust;
	// "current" and any non-canonical value do not.
	Validity string
	// FileBlobSHA is the blob object ID of the page file's current content
	// (working tree or branch tip), as reported by git plumbing.
	FileBlobSHA string
	// DefaultBranchBlobSHA is the blob object ID for the same path at the
	// locally fetched <remote>/<default-branch> ref. Empty when the ref is
	// unresolvable (non-git vault, missing fetch, git failure), which
	// withholds the validated grant but does not by itself demote
	// canonical non-validated tiers — GTS-FR-004's source-level fallback
	// is the caller's decision, not this predicate's.
	DefaultBranchBlobSHA string
}

// DeriveEffectiveTier returns the effective trust tier for a compiled or
// curated page per GTS-FR-001 through GTS-FR-003 of the git-trust-source
// capability. It is a pure function: no I/O, no globals, and a
// deterministic result for a given EffectiveTierInput.
//
// Rules, in application order:
//
//   - Downgrade ever honored (GTS-FR-003): an exact frontmatter "draft",
//     or an exact validity of "stale-unverified", "superseded", or
//     "historical", lowers trust regardless of git state. A validity
//     downgrade withholds the validated grant — a validated-claiming page
//     reads as "draft" — while the caller preserves the validity itself as
//     an orthogonal attribute (validity says whether knowledge is still
//     true; tier says who vouched for it). Canonical non-validated tiers
//     stay as declared.
//   - Validated only via blob equality (GTS-FR-001, GTS-FR-002): the
//     effective tier is "validated" if and only if FileTier is exactly
//     "validated" and both blob object IDs are non-empty and equal.
//     Working-tree edits, unmerged branches, regenerated articles, and an
//     absent default-branch ref therefore read as "draft".
//   - Unknown never raises trust (GTS-FR-001, GTS-FR-003): any tier or
//     validity value outside the canonical vocabulary — case variants
//     ("Validated", "VALIDATED"), whitespace-padded values, empty or
//     absent fields — degrades to "draft" and never grants "validated".
//
// Learning pages are out of scope: their ingestion keeps the permanent
// draft clamp (IP-FR-002) and never consults the validated grant.
func DeriveEffectiveTier(in EffectiveTierInput) string {
	switch in.FileTier {
	case canonicalTierDraft:
		// GTS-FR-003 signal (b): a canonical draft declaration lowers
		// trust from any file, branch, or local edit, regardless of blob
		// match.
		return canonicalTierDraft
	case canonicalTierAuthored, canonicalTierCurated, canonicalTierUntrusted:
		// Canonical non-validated tiers never seek the validated grant and
		// are preserved as declared — GTS-FR-001 forbids the gate from
		// degrading them to draft. Validity is orthogonal (the caller
		// preserves it separately), so the downgrade predicate has no tier
		// to lower here.
		return in.FileTier
	case canonicalTierValidated:
		if isDowngradeValidity(in.Validity) {
			// GTS-FR-003 signal (a): the downgrade wins even when the
			// blobs match — stale knowledge never appears validated.
			return canonicalTierDraft
		}
		if blobEqualityHolds(in.FileBlobSHA, in.DefaultBranchBlobSHA) {
			// GTS-FR-001: the only grant of validated — a canonical
			// request plus blob equality with the fetched default-branch
			// ref.
			return canonicalTierValidated
		}
		// GTS-FR-002: frontmatter alone cannot validate. No blob match —
		// or no resolvable default-branch ref — reads as draft.
		return canonicalTierDraft
	default:
		// Unknown or absent tier: never raises trust (GTS-FR-003).
		return canonicalTierDraft
	}
}

// isDowngradeValidity reports whether v is one of the exact validity
// values that lower trust per GTS-FR-003's exhaustive predicate. The
// predicate is closed: "current", empty, and non-canonical values (case
// variants, padded values) are neither downgrade signals nor upgrade
// signals — they leave the tier decision to the other rules.
func isDowngradeValidity(v string) bool {
	switch v {
	case downgradeValidityStaleUnverified, downgradeValiditySuperseded, downgradeValidityHistorical:
		return true
	default:
		return false
	}
}

// blobEqualityHolds reports the GTS-FR-001 grant condition: both blob
// object IDs are present and exactly equal. Empty inputs never match, so
// an unresolvable default-branch ref or a missing file blob withholds the
// grant (fail-safe per GTS-FR-004) instead of trivially equaling.
func blobEqualityHolds(fileBlobSHA, defaultBranchBlobSHA string) bool {
	return fileBlobSHA != "" && defaultBranchBlobSHA != "" && fileBlobSHA == defaultBranchBlobSHA
}

// Page-family namespace prefixes for the ingestion tier policy (IP-FR-001,
// IP-FR-002). Matched case-insensitively against the page name — both the
// vault-scan name (file path minus .md) and the source-document name
// (sourceID/docID) — so a hand-titled Compiled/ page cannot escape its
// family. The trailing slash is load-bearing: "learning-notes/" is not a
// learning page and "compiled-todos/" is not a compiled page.
const (
	learningPagePrefix  = "learning/"
	learningsPagePrefix = "learnings/"
	compiledPagePrefix  = "compiled/"
	knowledgePagePrefix = "knowledge/"
)

// isLearningPage reports whether path names a learning page per IP-FR-002:
// a page under the learning/ or learnings/ namespace. path is the page's
// location within its root — the vault-relative page name for the vault
// scan, or the source-relative document ID for source indexing (whose
// stored page names carry a source-ID prefix). Learning pages carry the
// permanent draft clamp on every ingestion route — nothing escalates a
// learning, regardless of what its frontmatter claims.
func isLearningPage(path string) bool {
	name := strings.ToLower(path)
	return strings.HasPrefix(name, learningPagePrefix) || strings.HasPrefix(name, learningsPagePrefix)
}

// isGitTrustPage reports whether path names a compiled or curated page —
// the compiled/ and knowledge/ namespaces whose trust tier ingestion
// derives from frontmatter (plus, once the task 2.x resolver is wired,
// default-branch blob equality) per IP-FR-001 and IP-FR-002. path follows
// the same convention as isLearningPage.
func isGitTrustPage(path string) bool {
	name := strings.ToLower(path)
	return strings.HasPrefix(name, compiledPagePrefix) || strings.HasPrefix(name, knowledgePagePrefix)
}

// DeriveTierFromProps derives the effective trust tier for a compiled or
// curated page from its parsed frontmatter properties, applying the
// GTS-FR-001 through GTS-FR-003 rules of DeriveEffectiveTier. It is the
// frontmatter seam for ingestion call sites outside the vault package (the
// knowledge-store indexer). fileBlobSHA and defaultBranchBlobSHA carry the
// git blob object IDs when a resolver is available and are empty otherwise
// — an unresolvable ref withholds the validated grant without demoting
// canonical curated/authored/untrusted claims (GTS-FR-004 leaves the
// no-ref source-level fallback to the caller).
//
// The tier and validity properties must be string-valued to count: absent,
// nil, or non-string values (numbers, booleans, nested structures) read as
// unknown and never raise trust.
func DeriveTierFromProps(props map[string]any, fileBlobSHA, defaultBranchBlobSHA string) string {
	return DeriveEffectiveTier(EffectiveTierInput{
		FileTier:             propString(props, "tier"),
		Validity:             propString(props, "validity"),
		FileBlobSHA:          fileBlobSHA,
		DefaultBranchBlobSHA: defaultBranchBlobSHA,
	})
}

// propString reads a string-valued property from parsed frontmatter. A nil
// map, an absent key, or a non-string value returns the empty string: per
// GTS-FR-001 an unknown value must never raise trust, so a mistyped field
// degrades to the draft fallback rather than erroring.
func propString(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	s, ok := props[key].(string)
	if !ok {
		return ""
	}
	return s
}

// ingestTier returns the trust tier ingestion assigns to the page at path
// and whether the page family carries an ingestion-specific tier policy
// at all. path is the page's location within its root — the vault-relative
// page name for the vault scan, or the source-relative document ID for
// source indexing. Learning pages always ingest as draft (the IP-FR-002
// clamp, honored on every ingestion route). Compiled and curated pages
// derive their tier from frontmatter and blob equality per IP-FR-001.
// Every other page family returns ok=false and keeps the caller's existing
// behavior — the source-config trust tier for index documents, or no tier
// assignment at all for the vault scan (whose InsertPage default is
// authored).
//
// The blob object IDs are empty until the task 2.x resolver wiring; an
// unresolvable ref withholds the validated grant without demoting
// canonical non-validated claims.
func ingestTier(path string, props map[string]any, fileBlobSHA, defaultBranchBlobSHA string) (string, bool) {
	switch {
	case isLearningPage(path):
		return canonicalTierDraft, true
	case isGitTrustPage(path):
		return DeriveTierFromProps(props, fileBlobSHA, defaultBranchBlobSHA), true
	default:
		return "", false
	}
}

// TierBlobResolver supplies the git blob object IDs that back trust-tier
// derivation for compiled and curated pages (IP-FR-001). Implementations hold
// the per-index-run state from design D2 — one resolved default-branch ref
// and one batch blob map — and answer per-page lookups without further git
// invocations. The production implementation lives in the gitref package and
// satisfies this interface structurally; neither package imports the other,
// and the orchestrator (CLI index commands, MCP indexing, the serve startup
// path) wires them together.
//
// The seam is deliberately optional and nil-safe: a nil resolver (non-git
// vault, unresolvable ref, or a caller that predates the wiring) yields empty
// blob SHAs on every lookup, which withholds the validated grant (GTS-FR-004)
// without demoting canonical non-validated tiers — exactly the pre-wiring
// behavior, keeping ingestion backward compatible.
type TierBlobResolver interface {
	// PageBlobs returns the working-tree and default-branch blob object IDs
	// for the page file at filePath (repository-relative, .md extension
	// included, for example "compiled/auth.md"). Empty returns withhold the
	// validated grant and never fail the caller.
	PageBlobs(filePath string) (fileBlobSHA, defaultBranchBlobSHA string)
}

// resolveTierBlobs looks up a page's blob object IDs through the optional
// resolver. A nil resolver is the "no resolver" default: empty blob SHAs,
// validated grant withheld (GTS-FR-004), behavior identical to the
// pre-wiring call sites.
func resolveTierBlobs(r TierBlobResolver, filePath string) (string, string) {
	if r == nil {
		return "", ""
	}
	return r.PageBlobs(filePath)
}
