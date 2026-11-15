package tools

// Distill automation MCP tools (018-weekly-distill-automation, T036):
// the unattended distill pipeline's three server-side stages — scope
// computation, theme grouping, and rolling per-theme PRs — exposed as
// the `scope`, `group_learners`, and `rolling_pr` MCP tools named by
// the distiller agent's frontmatter allowlist
// (contracts/distiller-allowlist.md Registration).
//
// The tools wrap the pipeline functions (tools/scope.go, tools/group.go,
// tools/rolling_pr.go) so the distiller agent — whose tool boundary
// denies direct file writes and arbitrary shell — drives the pipeline
// entirely through this surface: the agent supplies identities and
// grouping output, and every filesystem read, git operation, and PR
// mutation happens server-side behind the pipeline's own guards.
// GuardDistillTier rejects `tier: validated` at every write path the
// tools can reach, the no-op and run artifacts are recorded here where
// the vault is reachable (FR-021), and scope never consults a model or
// the question tool — ambiguity degrades deterministically (FR-006).
//
// Input structs live in this package rather than types/ because they
// carry tools.Theme and tools.ScopeSet values, and types/ cannot import
// tools (tools already imports types; the reverse would be circular).

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/store"
)

// Distill implements the scope, group_learners, and rolling_pr MCP
// tools: the unattended distill pipeline's server-side stages. The
// store backs the scope computation's SQLite learning union (nil is
// tolerated — the filesystem leg remains authoritative for the
// file-backed learnings), the embedder backs theme grouping's
// embedding-cosine assignments (nil selects the documented tag-fallback
// degraded mode), and the vault path anchors every git, filesystem, and
// artifact operation.
type Distill struct {
	store     *store.Store
	embedder  embed.Embedder
	vaultPath string
	// gitRunner and ghRunner are optional injection seams forwarded to
	// the rolling-PR stage's RollingPROptions (nil selects the
	// production exec runners — the same default ManageRollingPRs
	// applies). NewDistill leaves both nil; in-package tests inject
	// scripted runners so the handler's happy path runs without git
	// remotes or gh binaries (CS-007 dependency injection).
	gitRunner gitref.Runner
	ghRunner  GhRunner
}

// NewDistill creates the distill pipeline handler over the given store,
// embedder, and vault path. All three fields are optional at
// construction: a nil store degrades scope's filesystem fallback, a nil
// embedder degrades grouping to tag centroids, and an empty vault path
// fails every handler at call time with a clear error (the server
// always configures the vault; the guard documents the invariant).
func NewDistill(s *store.Store, e embed.Embedder, vaultPath string) *Distill {
	return &Distill{store: s, embedder: e, vaultPath: vaultPath}
}

// --- scope (contracts/scope.md) ---

// DistillScopeInput is the scope tool's input: no fields. The vault,
// default-branch ref, and exclusion policy are resolved server-side
// from the server's configuration — the tool takes nothing from the
// caller, so the same server state always yields the same scope
// (FR-005 determinism).
type DistillScopeInput struct{}

// distillScopeResult is the scope tool's result: the computed set, the
// FR-021 no-op signal, and the policy warnings the computation logged.
type distillScopeResult struct {
	// Scope is the deterministic scope set (sorted identities,
	// provenance).
	Scope ScopeSet `json:"scope"`
	// NoOp reports InScope == ∅: the run exits here — no PR, no
	// comment, one durable artifact recorded server-side.
	NoOp bool `json:"no_op"`
	// NoOpReason carries the artifact's no_op_reason on a no-op.
	NoOpReason string `json:"no_op_reason,omitempty"`
	// Artifact is the recorded no-op artifact's path (joined from the
	// vault root; no-op runs only).
	Artifact string `json:"artifact,omitempty"`
	// Warnings lists the distill-policy warnings (over-limit,
	// unparseable, unknown keys) the load surfaced.
	Warnings []string `json:"warnings,omitempty"`
}

// Scope implements the `scope` MCP tool: compute the unattended distill
// scope — every learning identity at the vault's default-branch ref,
// minus the identities cited by compiled articles' frontmatter sources,
// minus distill-policy tag exclusions. Deterministic from git alone
// (FR-005), never interactive (FR-006), and a degraded filesystem
// fallback when the ref is unavailable. An empty scope exits no-op: the
// FR-021 artifact is recorded server-side (the agent cannot write
// files) and the result carries no_op with the artifact path so the
// caller knows to stop without opening or commenting on anything.
func (d *Distill) Scope(ctx context.Context, _ *mcp.CallToolRequest, _ DistillScopeInput) (*mcp.CallToolResult, any, error) {
	if d.vaultPath == "" {
		return errorResult("scope: vault path is not configured"), nil, nil
	}
	// The policy file is vault-rooted, never CWD-rooted
	// (contracts/scope.md Policy Exclusions: repository root resolved
	// via the configured vault path; CWD never used). A missing file
	// means no exclusions — LoadDistillPolicy's documented default.
	policyPath := filepath.Join(d.vaultPath, ".uf", "dewey", "distill-policy.yaml")
	policy, warnings := LoadDistillPolicy(policyPath)
	scope, err := ComputeScope(ctx, d.vaultPath, StoreLearningPageNames(d.store), policy)
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	result := distillScopeResult{Scope: scope, Warnings: warnings}
	if scope.Empty {
		// The no-op artifact is the sole audit surface for an empty
		// scope (FR-021): no PR or issue body exists on this path, so
		// the tool records it server-side rather than leaving the
		// audit to an agent that cannot write files.
		artifact, err := RecordNoOpDistillRun(d.vaultPath, scope)
		if err != nil {
			return errorResult(fmt.Sprintf("scope: record no-op artifact: %v", err)), nil, nil
		}
		result.NoOp = true
		result.NoOpReason = NoOpReasonEmptyScope
		result.Artifact = artifact
	}
	res, err := jsonTextResult(result)
	if err != nil {
		return nil, nil, fmt.Errorf("scope: %w", err)
	}
	return res, nil, nil
}

// --- group_learners (contracts/grouping.md) ---

// DistillGroupInput is the group_learners tool's input: the scope
// result's in_scope identities. Learning texts, supersession edges, and
// existing compiled articles are re-derived server-side from the vault
// (CollectGroupInput) — the agent never carries file contents, keeping
// the tool boundary write-free and the payloads small.
type DistillGroupInput struct {
	// InScope lists the identities to group, as returned by the scope
	// tool's in_scope field.
	InScope []string `json:"in_scope"`
}

// distillGroupResult is the group_learners tool's result: the themes
// and the grouping mode.
type distillGroupResult struct {
	// Themes is the grouping output — one rolling PR per theme, sorted
	// by name.
	Themes []Theme `json:"themes"`
	// EmbeddingsAvailable reports whether the embedding assignments ran
	// (false means the documented tag-centroid degraded mode; each
	// AssignmentDecision.Source records the mode per member).
	EmbeddingsAvailable bool `json:"embeddings_available"`
}

// GroupLearners implements the `group_learners` MCP tool: merge the
// in-scope identities into must-colocate supersession components, then
// assign each to its closest compiled article by embedding cosine
// (tag-centroid fallback when embeddings are unavailable), with a
// deterministic tie-breaker — never a question (FR-006). Every
// assignment carries its source and confidence (FR-010). An empty
// in_scope is a caller error: the scope tool exits no-op before
// grouping, so reaching this tool with nothing to group means the
// pipeline was driven out of order.
func (d *Distill) GroupLearners(ctx context.Context, _ *mcp.CallToolRequest, input DistillGroupInput) (*mcp.CallToolResult, any, error) {
	if d.vaultPath == "" {
		return errorResult("group_learners: vault path is not configured"), nil, nil
	}
	if len(input.InScope) == 0 {
		return errorResult("group_learners: in_scope is empty — the scope tool exits no-op before grouping; an empty scope has no themes to group"), nil, nil
	}
	// CollectGroupInput re-derives texts, edges, and articles from the
	// vault at the resolved ref. Only the in-scope identities feed the
	// ScopeSet here — the grouping contract consumes nothing else from
	// the scope result.
	groupInput, err := CollectGroupInput(ctx, d.vaultPath, ScopeSet{InScope: sortedIdentities(setOf(input.InScope))})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	themes := GroupLearners(ctx, groupInput, GroupOptions{Embedder: d.embedder})
	payload := distillGroupResult{
		Themes:              themes,
		EmbeddingsAvailable: d.embedder != nil,
	}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("group_learners: %w", err)
	}
	return res, nil, nil
}

// --- rolling_pr (contracts/rolling-pr.md) ---

// DistillRollingPRInput is the rolling_pr tool's input: the grouping
// output plus the lint findings for the review queue body. The
// in-scope identities come from the themes' members — grouping
// partitions the scope, so the union of members is exactly the scope —
// and the learning texts are re-derived server-side from the vault,
// keeping the agent's payload free of file contents.
type DistillRollingPRInput struct {
	// Themes is the group_learners output — one rolling PR per theme.
	Themes []Theme `json:"themes"`
	// Lint optionally maps theme name → lint finding lines for the
	// review queue body (#147 anchor_drift / embedding_gap).
	Lint map[string][]string `json:"lint,omitempty"`
}

// distillRollingPRResult is the rolling_pr tool's result: the per-theme
// outcomes and the durable run artifact's path.
type distillRollingPRResult struct {
	// Results reports one outcome per theme: branch, PR number, state,
	// labels, head SHA, human-commit preservation, and the guards the
	// run enforced.
	Results []RollingPRResult `json:"results"`
	// Artifact is the recorded run artifact's path (joined from the
	// vault root; FR-021 audit surface).
	Artifact string `json:"artifact"`
}

// RollingPR implements the `rolling_pr` MCP tool: create or update one
// rolling dewey-distill/<theme> PR per theme — fresh branch, draft
// articles, and human-gate PR for new themes; rebase-preserving-human-
// commits updates with review-queue bodies and force-with-lease pushes
// for existing ones. Never writes `tier: validated` (GuardDistillTier
// rejects it at the render path), never overwrites a human edit
// (regeneration guard), and records the durable run artifact server-side
// (the agent cannot write files). Git and gh runners default to the
// production exec runners; the base pins the vault's HEAD at run start.
func (d *Distill) RollingPR(ctx context.Context, _ *mcp.CallToolRequest, input DistillRollingPRInput) (*mcp.CallToolResult, any, error) {
	if d.vaultPath == "" {
		return errorResult("rolling_pr: vault path is not configured"), nil, nil
	}
	if len(input.Themes) == 0 {
		return errorResult("rolling_pr: no themes — the group_learners tool produces the themes this pipeline stage consumes; an empty grouping has no PRs to roll"), nil, nil
	}
	// Grouping partitions the scope, so the themes' members are exactly
	// the in-scope identities. Texts are re-derived at the resolved ref
	// — the same source scope and grouping read — rather than carried
	// through the agent payload (the working tree is clean on every
	// path that reaches here, so ref and working tree agree).
	identities := themeMemberIdentities(input.Themes)
	groupInput, err := CollectGroupInput(ctx, d.vaultPath, ScopeSet{InScope: identities})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	// The run options default to the production runners; injected
	// seams (tests) forward through unchanged.
	opts := RollingPROptions{VaultDir: d.vaultPath, GitRunner: d.gitRunner, GhRunner: d.ghRunner}
	results, err := ManageRollingPRs(ctx, opts, RollingPRInput{
		Themes: input.Themes,
		Texts:  groupInput.Texts,
		Lint:   input.Lint,
	})
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}
	// The run artifact is the audit surface for what the run touched
	// (FR-021): every theme grouped, every PR created or updated, and
	// the scope size the themes partition. Conflict and untouched
	// themes appear in Themes but never in PRsTouched — they pushed
	// nothing.
	artifact, err := WriteDistillRunArtifact(d.vaultPath, distillRunRecord(input.Themes, results))
	if err != nil {
		return errorResult(fmt.Sprintf("rolling_pr: record run artifact: %v", err)), nil, nil
	}
	payload := distillRollingPRResult{Results: results, Artifact: artifact}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("rolling_pr: %w", err)
	}
	return res, nil, nil
}

// themeMemberIdentities returns the sorted union of the themes'
// members — the in-scope set the grouping partitioned.
func themeMemberIdentities(themes []Theme) []string {
	members := make(map[string]struct{})
	for _, t := range themes {
		for _, id := range t.Members {
			members[id] = struct{}{}
		}
	}
	return sortedIdentities(members)
}

// distillRunRecord builds the FR-021 run artifact from the pipeline's
// inputs and outcomes: theme names sorted, touched branches (created or
// updated states only) sorted, and the scope size the themes partition.
func distillRunRecord(themes []Theme, results []RollingPRResult) DistillRunRecord {
	themeNames := make([]string, 0, len(themes))
	scopeSize := 0
	for _, t := range themes {
		themeNames = append(themeNames, t.Name)
		scopeSize += len(t.Members)
	}
	sort.Strings(themeNames)
	touched := make([]string, 0, len(results))
	for _, r := range results {
		if r.State == RollingPRCreated || r.State == RollingPRUpdated {
			touched = append(touched, r.Branch)
		}
	}
	sort.Strings(touched)
	return DistillRunRecord{
		RunAt:      time.Now().UTC(),
		ScopeSize:  scopeSize,
		Themes:     themeNames,
		PRsTouched: touched,
	}
}

// setOf converts a slice into its set form.
func setOf(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
