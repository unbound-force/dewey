/**
 * Pure branch-name classification shared by uf-workflow and uf-finale.
 *
 * Extracts the classification previously inlined in uf_finale_startup
 * (plugins/uf-finale/index.ts) so the branch-gate tool and the finale
 * startup tool share one implementation (OpenSpec change
 * unleash-deterministic-tools, design decision D3).
 *
 * This module is deliberately free of filesystem and git access:
 * classification is a pure string function, testable in isolation
 * (constitution Principle IV).
 */

/** Workflow tier inferred from the branch naming convention. */
export type BranchType = "opsx" | "speckit" | "quick" | "other"

/** Classification of a git branch name. */
export interface BranchClassification {
  /** Conventional workflow the branch belongs to. */
  readonly branchType: BranchType
  /**
   * Change name derived from the branch: `opsx/<name>` and `quick/<name>`
   * yield `<name>`; `NNN-<name>` yields `<name>` (numeric prefix stripped).
   * For every other branch this is the branch name verbatim.
   */
  readonly changeName: string
  /** True only for the exact branch names "main" and "master". */
  readonly isMain: boolean
}

/**
 * Classify a branch name into its workflow tier and change name.
 *
 * Reproduces the legacy uf_finale_startup classification and extends
 * it with the quick tier (OpenSpec change unleash-tier-routing):
 * - `opsx/<name>` → branchType "opsx", changeName `<name>`
 * - `NNN-<name>` → branchType "speckit", changeName `<name>` with the
 *   leading numeric prefix stripped
 * - `quick/<name>` → branchType "quick", changeName `<name>`
 * - `main` / `master` → branchType "other" with isMain true
 * - anything else → branchType "other", branch verbatim as changeName
 *
 * Design notes:
 * - For speckit branches the numeric prefix is intentionally stripped
 *   from changeName (legacy behavior). Downstream consumers such as the
 *   branch gate derive `specs/<branch>/` from the raw branch string,
 *   never from changeName.
 * - isMain is computed independently of the tier chain, so main/master
 *   keep the non-tier "other" classification with isMain true; the
 *   gate derives its distinct main class from isMain.
 *
 * @param branch Raw branch name (e.g. from `git rev-parse --abbrev-ref HEAD`).
 * @returns The classification. Never throws.
 */
export function classifyBranch(branch: string): BranchClassification {
  const isMain = branch === "main" || branch === "master"

  if (branch.startsWith("opsx/")) {
    return { branchType: "opsx", changeName: branch.slice("opsx/".length), isMain }
  }
  if (branch.startsWith("quick/")) {
    return { branchType: "quick", changeName: branch.slice("quick/".length), isMain }
  }
  if (/^\d+-/.test(branch)) {
    return { branchType: "speckit", changeName: branch.replace(/^\d+-/, ""), isMain }
  }
  return { branchType: "other", changeName: branch, isMain }
}
