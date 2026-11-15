// Package tools — the distiller tier guard (FR-012, FR-017;
// specs/018-weekly-distill-automation).
//
// The weekly unattended distill automates demotion, never promotion: a
// run may flag drift, supersede, and regenerate draft articles, but only
// a human promotes — the vouch is the merge of a CODEOWNERS-approved
// pull request (#203). This file is the server-side enforcement hook for
// that boundary: every write path a distiller can reach asserts its tier
// through GuardDistillTier before writing, so a `tier: validated` write
// is rejected and logged no matter how the tier value arrived — a
// refactored input, a theme carrying a validated existing article, or a
// prompt-influenced parameter.
//
// Deliberate non-implementation: the store layer (store.UpdatePage /
// InsertPage) MUST NOT enforce this rule. Trust derivation writes
// `validated` into the pages cache on every index when a file's blob
// equals the fetched default-branch ref (GTS-FR-005: the caller-supplied
// tier always lands), so a store-level rejection would break #203's
// git-is-the-source-of-record contract. The guard belongs at the tool
// layer, on the distiller-reachable write paths — store_compiled here,
// rolling_pr's article writer — while promote stays the single
// human-facing `tier: validated` file-write path (the distiller
// allowlist denies it; contracts/distiller-allowlist.md).
package tools

import (
	"errors"
)

// DistillTierDraft is the only tier a distiller-mode write may carry
// (data-model.md RollingPR.tier_written: always `draft` in distiller
// mode). Write paths use it as their constant and pass it through
// GuardDistillTier so the invariant is asserted, not assumed.
const DistillTierDraft = "draft"

// ErrValidatedTierRejected is the sentinel error GuardDistillTier returns
// on a rejected write. The file on disk stays draft — nothing was
// written.
var ErrValidatedTierRejected = errors.New("distiller write rejected: tier 'validated' is human-only (FR-012/FR-017); the file stays 'draft'")

// GuardDistillTier enforces the FR-012/FR-017 invariant at a
// distiller-mode write path: the run MUST never write `tier: validated`
// into any file. A validated tier — tierValidated, the canonical value
// promote writes (promote_file.go), i.e. the human-only tier — is
// rejected with the sentinel error and a logged warning (FR-017:
// attempts MUST be rejected and logged); every other tier — draft in
// normal operation — passes. Callers translate the error into an error
// result and leave the target file untouched.
func GuardDistillTier(tier string) error {
	if tier == tierValidated {
		scopeLogger.Warn("distiller attempted a tier:validated write; rejected, file stays draft",
			"tier", tier)
		return ErrValidatedTierRejected
	}
	return nil
}
