package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// Promote implements the dewey_promote MCP tool and CLI command
// (openspec/changes/git-is-the-source-of-record-for-page-trust-tiers).
// Promotion is a frontmatter file edit on the current branch (PW-FR-001):
// the tool writes `tier: validated` into the page file's YAML frontmatter
// so a pull request can carry the promotion for human review. It never
// writes the database tier — `pages.tier` is a cache of the git rule
// (GTS-FR-005) recomputed on the next index from file state plus the
// fetched default-branch ref (PW-FR-002). A promotion on an unmerged
// branch therefore keeps indexing as draft on every server, including the
// promoting one, until the pull request merges and the ref is fetched
// (PW-FR-003): the merge is the vouch.
//
// Design decision: Only the draft → validated transition is supported
// (invariant 1). Validated pages cannot be re-promoted because they are
// already at the highest agent-achievable trust level, and authored or
// curated pages cannot be promoted because promotion reviews agent
// material. Learnings are never promotable (IP-FR-002: learnings stay
// draft forever); only files under the compiled/ and knowledge/ prefixes,
// in either the vault-root or the .uf/dewey workspace layout, can be
// edited (PW-FR-004).
//
// Promotion accepts either a single page name (Page) or a batch (Pages).
// Batch promotion validates every page and prepares every edit before any
// is applied: if one page is missing, ambiguous, not draft, or has no
// promotable file, no file is changed. The apply step is atomic per file
// (temp file + rename, PW-FR-001) and rolls back already-applied edits on
// mid-batch failure, keeping a human review decision all-or-nothing.
type Promote struct {
	store     *store.Store
	vaultPath string
}

// NewPromote creates a new Promote tool handler with the given store and
// vault root. The store resolves page names through the resolution ladder;
// the vault root anchors the PW-FR-004 path validation every edit target
// must pass (canonical vault-contained path under compiled/ or knowledge/,
// symlinks resolved, no-follow on the final component). The store must be
// non-nil for the tool to function; a clear error is returned at call time
// if it is nil.
func NewPromote(s *store.Store, vaultPath string) *Promote {
	return &Promote{store: s, vaultPath: vaultPath}
}

// Promote handles the dewey_promote MCP tool. Promotes a compiled or
// curated page by writing `tier: validated` into its file frontmatter on
// the current branch (PW-FR-001): the file diff, carried by a pull
// request, is the promotion artifact, and the database tier stays a cache
// recomputed on the next index (PW-FR-002, GTS-FR-005). Page names are
// resolved through resolvePromotePage, a tools-side ladder over narrow
// index-backed store queries: exact name, then case-insensitive name, then
// the learning/compiled/knowledge namespaces for bare tags, then source
// doc IDs. Raw arguments are path-validated before any lookup or file
// access (PW-FR-004). Returns an error result if any requested page
// cannot be promoted.
func (p *Promote) Promote(_ context.Context, _ *mcp.CallToolRequest, input types.PromoteInput) (*mcp.CallToolResult, any, error) {
	if p.store == nil {
		return errorResult(persistentStorageError("promote").Error()), nil, nil
	}

	batch := input.Pages != nil
	if batch && input.Page != "" {
		return errorResult("provide either page or pages, not both"), nil, nil
	}
	if !batch && strings.TrimSpace(input.Page) == "" {
		return errorResult("page or pages parameter is required"), nil, nil
	}

	requested := []string{input.Page}
	if batch {
		if len(input.Pages) == 0 {
			return errorResult("pages must contain at least one page name"), nil, nil
		}
		requested = dedupePageNames(input.Pages)
	}

	targets, failures, singleErr := p.resolvePromoteTargets(requested, batch)
	if singleErr != nil {
		return singleErr, nil, nil
	}

	if len(failures) > 0 {
		res, err := promoteFailureResult(failures)
		if err != nil {
			return nil, nil, err
		}
		return res, nil, nil
	}

	// Every requested page resolved and is draft. Resolve the vault root
	// the file edits are validated against (PW-FR-004).
	resolvedRoot, err := resolveVaultRoot(p.vaultPath)
	if err != nil {
		return errorResult(fmt.Sprintf("cannot promote: %v", err)), nil, nil
	}

	// Resolve each target to its promotable file and prepare the edit.
	// All edits are prepared before any is applied, keeping the batch
	// all-or-nothing (a failure during preparation touches nothing).
	edits := make([]promoteEdit, 0, len(targets))
	for _, t := range targets {
		rel, abs, fileErr := resolvePromotableFile(resolvedRoot, t.page)
		if fileErr != nil {
			if !batch {
				return errorResult(fmt.Sprintf("failed to promote page '%s': %v", t.requested, fileErr)), nil, nil
			}
			failures = append(failures, map[string]string{"page": t.page.Name, "error": fileErr.Error()})
			continue
		}
		edit, buildErr := buildPromoteEdit(t.page.Name, rel, abs)
		if buildErr != nil {
			if !batch {
				return errorResult(fmt.Sprintf("failed to promote page '%s': %v", t.requested, buildErr)), nil, nil
			}
			failures = append(failures, map[string]string{"page": t.page.Name, "error": buildErr.Error()})
			continue
		}
		edits = append(edits, edit)
	}
	if len(failures) > 0 {
		res, renderErr := promoteFailureResult(failures)
		if renderErr != nil {
			return nil, nil, renderErr
		}
		return res, nil, nil
	}

	// Apply every prepared edit atomically: temp file + rename per file,
	// with rollback of already-applied edits on mid-batch failure.
	if applyErr := applyPromoteEdits(edits); applyErr != nil {
		if !batch {
			return errorResult(fmt.Sprintf("failed to promote page: %v", applyErr)), nil, nil
		}
		res, renderErr := rollbackFailureResult(targets, applyErr)
		if renderErr != nil {
			return nil, nil, renderErr
		}
		return res, nil, nil
	}

	if !batch {
		edit := edits[0]
		result := map[string]any{
			"page":          edit.page,
			"previous_tier": "draft",
			"new_tier":      tierValidated,
			"file":          edit.relPath,
			"message": fmt.Sprintf(
				"Page '%s' promoted to validated tier: tier: validated written to %s on the current branch. Commit the change and open a pull request — the promotion takes effect for every server once the pull request merges to the default branch and the server re-indexes (promote edits the file, not the database).",
				edit.page, edit.relPath),
		}
		res, err := jsonTextResult(result)
		return res, nil, err
	}

	result := map[string]any{
		"status":         "promoted",
		"promoted":       promotedEntries(edits),
		"total_promoted": len(edits),
		"message": fmt.Sprintf(
			"%d page(s) promoted to validated tier via frontmatter file edits on the current branch; the promotions take effect once the changes merge to the default branch and are re-indexed.",
			len(edits)),
	}
	res, err := jsonTextResult(result)
	return res, nil, err
}

// promoteTarget pairs a requested input name with its resolved store page.
type promoteTarget struct {
	requested string
	page      *store.Page
}

// resolvePromoteTargets resolves every requested name through the
// resolution ladder and keeps only pages that are safe to promote. In
// single-page mode the first problem aborts with a rendered error result;
// in batch mode problems accumulate as per-page failure entries. The
// lookup and tier error strings are the handler's contract and must not
// change (PR 145 review amendment); the raw-argument path check is the
// PW-FR-004 addition and carries its own error strings.
func (p *Promote) resolvePromoteTargets(requested []string, batch bool) ([]promoteTarget, []map[string]string, *mcp.CallToolResult) {
	targets := make([]promoteTarget, 0, len(requested))
	resolvedNames := make(map[string]struct{}, len(requested))
	failures := make([]map[string]string, 0)

	for _, name := range requested {
		if strings.TrimSpace(name) == "" {
			failures = append(failures, map[string]string{"page": name, "error": "page name is required"})
			continue
		}

		// PW-FR-004 raw-argument guard: absolute paths and `..` segments
		// are rejected before any store lookup or filesystem access, so
		// no traversal-shaped argument can direct a later file edit.
		if argErr := checkPromoteArgument(name); argErr != nil {
			if !batch {
				return nil, nil, errorResult(argErr.Error())
			}
			failures = append(failures, map[string]string{"page": name, "error": argErr.Error()})
			continue
		}

		page, err := resolvePromotePage(p.store, name)
		if err != nil {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf("failed to look up page '%s': %s", name, err))
			}
			failures = append(failures, map[string]string{"page": name, "error": err.Error()})
			continue
		}
		if page == nil {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf("Page '%s' not found.", name))
			}
			failures = append(failures, map[string]string{"page": name, "error": "page not found"})
			continue
		}

		// Two different inputs (e.g. "one" and "compiled/one") can resolve
		// to the same page. Validate and report it once.
		if _, seen := resolvedNames[page.Name]; seen {
			continue
		}
		resolvedNames[page.Name] = struct{}{}

		if page.Tier != "draft" {
			if !batch {
				return nil, nil, errorResult(fmt.Sprintf(
					"Page '%s' has tier '%s' and cannot be promoted. Only 'draft' pages can be promoted to 'validated'.",
					page.Name, page.Tier,
				))
			}
			failures = append(failures, map[string]string{
				"page":  page.Name,
				"error": fmt.Sprintf("tier is '%s'; only 'draft' pages can be promoted to 'validated'", page.Tier),
			})
			continue
		}

		targets = append(targets, promoteTarget{requested: name, page: page})
	}
	return targets, failures, nil
}

// promoteFailureResult renders the pre-flight failure result: some pages
// could not be resolved or prepared, so no files were changed.
func promoteFailureResult(failures []map[string]string) (*mcp.CallToolResult, error) {
	payload := map[string]any{
		"status":       "failed",
		"failed":       failures,
		"total_failed": len(failures),
		"message":      fmt.Sprintf("%d page(s) could not be promoted; no files were changed.", len(failures)),
	}
	res, err := jsonTextResult(payload)
	if err != nil {
		return nil, err
	}
	res.IsError = true
	return res, nil
}

// rollbackFailureResult renders the mid-apply failure result: the batch
// rolled back and no files remain changed.
func rollbackFailureResult(targets []promoteTarget, promoteErr error) (*mcp.CallToolResult, error) {
	failed := make([]map[string]string, 0, len(targets))
	for _, target := range targets {
		failed = append(failed, map[string]string{
			"page":  target.page.Name,
			"error": fmt.Sprintf("file edits rolled back: %v", promoteErr),
		})
	}
	payload := map[string]any{
		"status":         "failed",
		"promoted":       []any{},
		"failed":         failed,
		"total_promoted": 0,
		"total_failed":   len(failed),
		"message":        "promotion edit failed; all file changes were rolled back.",
	}
	res, jsonErr := jsonTextResult(payload)
	if jsonErr != nil {
		return nil, jsonErr
	}
	res.IsError = true
	return res, nil
}

// promotedEntries renders the per-page success entries for the batch result.
func promotedEntries(edits []promoteEdit) []map[string]string {
	promoted := make([]map[string]string, 0, len(edits))
	for _, e := range edits {
		promoted = append(promoted, map[string]string{
			"page":          e.page,
			"previous_tier": "draft",
			"new_tier":      tierValidated,
			"file":          e.relPath,
		})
	}
	return promoted
}

// dedupePageNames removes duplicate entries while preserving request order.
// Blank entries are retained so batch validation can report them instead of
// silently ignoring part of the request.
func dedupePageNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// resolvePromotePage resolves a promote page reference to a stored page
// (dewey-distill feedback item 10). The ladder runs narrow, index-backed
// store queries and keeps the resolution policy in the tools layer:
//
//  1. exact page name (GetPage)
//  2. case-insensitive page name (ListPagesByNameFoldCase)
//  3. bare tags only: compiled/{name}, learning/{name}, and knowledge
//     pages whose name ends with "/{name}"
//  4. source doc IDs: "source/doc" pairs, then the bare tag against the
//     compiled and learning sources
//
// A unique match returns the page; multiple matches return an ambiguity
// error naming the candidates; no match returns (nil, nil) so callers can
// distinguish not-found from a store failure.
func resolvePromotePage(s *store.Store, name string) (*store.Page, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("page name is required")
	}

	// Tier 1: exact name.
	page, err := s.GetPage(trimmed)
	if err != nil {
		return nil, err
	}
	if page != nil {
		return page, nil
	}

	// Tier 2: case-insensitive exact name.
	matches, err := s.ListPagesByNameFoldCase(trimmed)
	if err != nil {
		return nil, err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, ambiguousPageError(trimmed, matches)
	}

	// Tier 3: bare-tag namespaces. Slash-containing references skip this
	// tier; they address pages by name or source doc directly.
	if !strings.Contains(trimmed, "/") {
		var candidates []*store.Page
		for _, prefix := range []string{store.CompiledPrefix, store.LearningPrefix} {
			ns, err := s.ListPagesByNameFoldCase(prefix + trimmed)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, ns...)
		}
		knowledge, err := s.ListKnowledgePagesFoldCase()
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, filterPagesBySuffix(knowledge, trimmed)...)
		candidates = dedupePages(candidates)
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		if len(candidates) > 1 {
			return nil, ambiguousPageError(trimmed, candidates)
		}
		// No namespace candidate: fall through to tier 4.
	}

	// Tier 4: source doc IDs. "source/doc" pairs address the doc directly;
	// bare tags search the compiled and learning sources.
	if source, doc, found := strings.Cut(trimmed, "/"); found {
		docMatches, err := s.GetPageBySourceDoc(source, doc)
		if err != nil {
			return nil, err
		}
		if len(docMatches) == 1 {
			return docMatches[0], nil
		}
		if len(docMatches) > 1 {
			return nil, ambiguousPageError(trimmed, docMatches)
		}
		return nil, nil
	}
	var candidates []*store.Page
	for _, source := range []string{store.SourceCompiled, store.SourceLearning} {
		docMatches, err := s.GetPageBySourceDoc(source, trimmed)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, docMatches...)
	}
	candidates = dedupePages(candidates)
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return nil, ambiguousPageError(trimmed, candidates)
	}
	return nil, nil
}

// filterPagesBySuffix returns pages whose name ends with "/suffix",
// case-insensitively (e.g. knowledge store pages for a bare tag).
func filterPagesBySuffix(pages []*store.Page, suffix string) []*store.Page {
	needle := "/" + strings.ToLower(suffix)
	out := make([]*store.Page, 0, len(pages))
	for _, p := range pages {
		if strings.HasSuffix(strings.ToLower(p.Name), needle) {
			out = append(out, p)
		}
	}
	return out
}

// dedupePages removes pages with duplicate names, preserving order.
func dedupePages(pages []*store.Page) []*store.Page {
	seen := make(map[string]struct{}, len(pages))
	out := make([]*store.Page, 0, len(pages))
	for _, p := range pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		out = append(out, p)
	}
	return out
}

// ambiguousPageError reports that a page reference matched multiple pages.
// The candidate list is sorted and capped so pathological matches cannot
// flood the error message.
func ambiguousPageError(name string, pages []*store.Page) error {
	names := make([]string, 0, len(pages))
	for _, p := range pages {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	const maxMatches = 10
	if len(names) > maxMatches {
		return fmt.Errorf("page name %q is ambiguous; matches: %s (and %d more)",
			name, strings.Join(names[:maxMatches], ", "), len(names)-maxMatches)
	}
	return fmt.Errorf("page name %q is ambiguous; matches: %s", name, strings.Join(names, ", "))
}
