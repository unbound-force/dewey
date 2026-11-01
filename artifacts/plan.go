package artifacts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Diff rendering bounds (DE-001): the conflict diff renderer is deterministic,
// capped at 100 emitted lines, truncates individual lines at 4 KiB, and caps
// the total at 16 KiB — each bound with an explicit marker.
const (
	diffMaxInputLines = 800
	diffContextLines  = 3
	diffMaxLines      = 100
	diffMaxLineBytes  = 4096
	diffMaxTotalBytes = 16384
)

// BuildPlan assembles the full artifact publish plan for the repository at
// repoRoot: it enumerates stranded artifacts from the working tree, stash
// entries, and unpushed commits, resolves the repository's publish posture
// (remotes, base ref, base tree), dedupes candidates across sources, screens
// every candidate's content (injection + credential scanners), routes
// blocked paths and conflicts, probes gitignore posture, and renders both the
// canonical JSON payload and the tool-rendered human summary. The result is a
// pure function of repository state — running it twice yields byte-identical
// JSON. vaultRoot must resolve inside the git repository; storePaths may add
// custom knowledge-store paths (each must resolve inside the repository).
func BuildPlan(repoRoot, vaultRoot string, storePaths []string) (*Plan, error) {
	repoRoot = filepath.Clean(repoRoot)
	vaultRoot = filepath.Clean(vaultRoot)

	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root %s: %w", repoRoot, err)
	}
	resolvedVault, err := filepath.EvalSymlinks(vaultRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root %s: %w", vaultRoot, err)
	}
	if !withinDir(resolvedVault, resolvedRepo) {
		return nil, fmt.Errorf("vault root %s (resolved %s) is outside the git repository %s (resolved %s)",
			vaultRoot, resolvedVault, repoRoot, resolvedRepo)
	}
	vaultRel := relWithin(resolvedVault, resolvedRepo)
	roots, err := workingTreeWalkRoots(repoRoot, resolvedRepo, vaultRoot, vaultRel, storePaths)
	if err != nil {
		return nil, err
	}

	wt, wtWarnings, err := enumerateWorkingTree(repoRoot, vaultRoot, storePaths)
	if err != nil {
		return nil, err
	}
	st, stashRefs, stWarnings, err := enumerateStashes(repoRoot, storePaths)
	if err != nil {
		return nil, err
	}
	cm, cmWarnings, err := enumerateCommits(repoRoot, storePaths)
	if err != nil {
		return nil, err
	}

	repo, baseTree, err := resolveRepoForPlan(repoRoot, roots)
	if err != nil {
		return nil, err
	}
	repo.StashRefs = stashRefs
	if repo.StashRefs == nil {
		repo.StashRefs = map[string]string{}
	}

	gi, giWarnings, err := probeGitignore(repoRoot, roots)
	if err != nil {
		return nil, err
	}

	plan := &Plan{
		SchemaVersion: PlanSchemaVersion,
		Repo:          repo,
		Gitignore:     gi,
		Artifacts:     []Artifact{},
		Conflicts:     []Conflict{},
		Blocked:       []BlockedArtifact{},
	}

	cands := append(append(wt, st...), cm...)
	contents := make(map[string]string, len(cands))
	for _, c := range cands {
		contents[c.SHA256] = string(c.Content)
	}

	for _, g := range groupCandidatesByPath(cands) {
		findingsBySHA := make(map[string][]Finding)
		allBlocked := true
		for _, c := range g.cands {
			if _, ok := findingsBySHA[c.SHA256]; !ok {
				findingsBySHA[c.SHA256] = screenContent(string(c.Content))
			}
			if !shaBlocked(findingsBySHA[c.SHA256]) {
				allBlocked = false
			}
		}

		if allBlocked {
			plan.Blocked = append(plan.Blocked, BlockedArtifact{
				Path:     g.path,
				Identity: g.cands[0].Identity,
				Findings: unionFindings(findingsBySHA),
			})
			continue
		}

		art, isConflict := buildArtifactEntry(g, baseTree)
		if !isConflict {
			// allBlocked is false, so no finding on this sha blocks.
			art.Findings = findingsBySHA[g.cands[0].SHA256]
			plan.Artifacts = append(plan.Artifacts, art)
			continue
		}
		conf, ok := buildConflictEntry(g)
		if !ok {
			continue
		}
		for i := range conf.Variants {
			conf.Variants[i].Findings = findingsBySHA[conf.Variants[i].SHA256]
		}
		plan.Conflicts = append(plan.Conflicts, conf)
	}

	warnings := append(append(append(wtWarnings, stWarnings...), cmWarnings...), giWarnings...)
	sort.Strings(warnings)
	if warnings == nil {
		warnings = []string{}
	}
	plan.Warnings = warnings

	newCount, unchangedCount := 0, 0
	for _, a := range plan.Artifacts {
		switch a.Status {
		case StatusNew:
			newCount++
		case StatusUnchanged:
			unchangedCount++
		}
	}
	plan.Summary = Summary{
		Total:     len(plan.Artifacts),
		New:       newCount,
		Unchanged: unchangedCount,
		Conflicts: len(plan.Conflicts),
		Blocked:   len(plan.Blocked),
	}

	plan.RenderedSummary = renderSummary(plan, contents)
	return plan, nil
}

// shaBlocked reports whether any finding in the slice blocks publication.
func shaBlocked(findings []Finding) bool {
	for _, f := range findings {
		if blocksPublish(f) {
			return true
		}
	}
	return false
}

// unionFindings merges the findings of every variant of an all-blocked path,
// deduplicating identical findings (two variants may trip the same rule at
// the same line) and sorting by scanner, pattern, then line.
func unionFindings(bySHA map[string][]Finding) []Finding {
	seen := make(map[Finding]bool)
	var all []Finding
	for _, findings := range bySHA {
		for _, f := range findings {
			if !seen[f] {
				seen[f] = true
				all = append(all, f)
			}
		}
	}
	sortFindings(all)
	return all
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Scanner != findings[j].Scanner {
			return findings[i].Scanner < findings[j].Scanner
		}
		if findings[i].Pattern != findings[j].Pattern {
			return findings[i].Pattern < findings[j].Pattern
		}
		return findings[i].Line < findings[j].Line
	})
}

// renderSummary renders the plan's tool-rendered human summary (DE-003): the
// itemized tag-grouped artifact listing, the conflict list with per-variant
// findings and bounded unified diffs between clean variants, blocked entries
// with their findings, the gitignore finding, and warnings. Every untrusted
// string is control-character-stripped and rendered as a markdown code span;
// diff lines are control-stripped and indented.
func renderSummary(plan *Plan, contents map[string]string) string {
	var b strings.Builder
	b.WriteString("# Dewey artifact publish plan\n\n")

	fmt.Fprintf(&b, "Repository: %s @ %s → publish %s (push: %s)\n",
		renderString(plan.Repo.BaseBranch), plan.Repo.BaseSHA,
		renderString(plan.Repo.PublishRemote), renderString(plan.Repo.PushRemote))
	fmt.Fprintf(&b, "Artifacts: %d total (new: %d, unchanged: %d) — conflicts: %d, blocked: %d\n",
		plan.Summary.Total, plan.Summary.New, plan.Summary.Unchanged,
		plan.Summary.Conflicts, plan.Summary.Blocked)

	b.WriteString("\n## Artifacts by tag\n")
	byTag := make(map[string][]Artifact)
	var tags []string
	for _, a := range plan.Artifacts {
		if _, ok := byTag[a.Tag]; !ok {
			tags = append(tags, a.Tag)
		}
		byTag[a.Tag] = append(byTag[a.Tag], a)
	}
	sort.Strings(tags)
	if len(tags) == 0 {
		b.WriteString("\n(none)\n")
	}
	for _, tag := range tags {
		fmt.Fprintf(&b, "\n### tag: %s\n", renderString(tag))
		for _, a := range byTag[tag] {
			fmt.Fprintf(&b, "- %s (%s, sha256: %s)\n", renderString(a.Path), a.Kind, a.SHA256)
			for _, f := range a.Findings {
				writeFindingLine(&b, "  ", f)
			}
		}
	}

	if len(plan.Conflicts) > 0 {
		b.WriteString("\n## Conflicts\n")
		for _, c := range plan.Conflicts {
			fmt.Fprintf(&b, "\n### %s (identity: %s)\n", renderString(c.Path), renderString(c.Identity))
			var clean []Variant
			for _, v := range c.Variants {
				fmt.Fprintf(&b, "- variant %s — sources: %s\n", v.SHA256, renderString(strings.Join(v.Sources, ", ")))
				for _, f := range v.Findings {
					writeFindingLine(&b, "  ", f)
				}
				if !shaBlocked(v.Findings) {
					clean = append(clean, v)
				}
			}
			if len(clean) >= 2 {
				b.WriteString("Diff between clean variants:\n")
				for i := 1; i < len(clean); i++ {
					diff := renderBoundedDiff(
						"clean variant "+clean[0].SHA256,
						"clean variant "+clean[i].SHA256,
						contents[clean[0].SHA256], contents[clean[i].SHA256])
					for _, line := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
						fmt.Fprintf(&b, "  %s\n", stripControl(line))
					}
				}
			}
		}
	}

	if len(plan.Blocked) > 0 {
		b.WriteString("\n## Blocked\n")
		for _, bl := range plan.Blocked {
			fmt.Fprintf(&b, "\n### %s (identity: %s)\n", renderString(bl.Path), renderString(bl.Identity))
			for _, f := range bl.Findings {
				writeFindingLine(&b, "  ", f)
			}
		}
	}

	b.WriteString("\n## Gitignore\n\n")
	fmt.Fprintf(&b, "Status: %s\n", renderString(plan.Gitignore.Status))
	if fix := plan.Gitignore.Fix; fix != nil {
		fmt.Fprintf(&b, "Fix: replace the bare `dewey` pattern in %s with the anchored re-ignore block (%d lines); applied by sync on the chore branch.\n",
			renderString(fix.File), len(fix.Reignore))
	}

	if len(plan.Warnings) > 0 {
		b.WriteString("\n## Warnings\n")
		for _, w := range plan.Warnings {
			fmt.Fprintf(&b, "- %s\n", renderString(w))
		}
	}

	return b.String()
}

// writeFindingLine renders one screening finding line at the given indent.
func writeFindingLine(b *strings.Builder, indent string, f Finding) {
	fmt.Fprintf(b, "%s- finding: [%s] %s (%s) line %d — %s\n",
		indent, f.Scanner, f.Pattern, f.Severity, f.Line, f.Message)
}

// renderString renders an untrusted plan string (path, identity, tag, store
// name, warning) as a markdown code span with control characters stripped.
func renderString(s string) string {
	s = stripControl(s)
	s = strings.ReplaceAll(s, "`", "'")
	return "`" + s + "`"
}

// stripControl removes control characters (runes below 0x20 and DEL) so
// untrusted content cannot smuggle terminal or markdown control sequences
// into rendered output.
func stripControl(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// diffOp is one line of a line diff: kind is ' ' (context), '-' (removed
// from A), or '+' (added in B).
type diffOp struct {
	kind byte
	text string
}

// renderBoundedDiff renders a deterministic bounded unified diff between two
// contents (DE-001). Inputs larger than diffMaxInputLines per side skip the
// LCS pass (whole-file removal+addition fallback). Emission is capped at
// diffMaxLines lines and diffMaxTotalBytes bytes, individual lines at
// diffMaxLineBytes — each bound marked explicitly.
func renderBoundedDiff(headerA, headerB, contentA, contentB string) string {
	al := strings.Split(contentA, "\n")
	bl := strings.Split(contentB, "\n")

	var ops []diffOp
	if len(al) > diffMaxInputLines || len(bl) > diffMaxInputLines {
		for _, l := range al {
			ops = append(ops, diffOp{kind: '-', text: l})
		}
		for _, l := range bl {
			ops = append(ops, diffOp{kind: '+', text: l})
		}
	} else {
		ops = lcsDiffOps(al, bl)
	}
	hunks := unifiedHunks(ops, diffContextLines)

	var out strings.Builder
	out.WriteString("--- " + headerA + "\n")
	out.WriteString("+++ " + headerB + "\n")

	emitted := 0
	truncated := false
	for _, h := range hunks {
		if truncated {
			break
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", h.aStart, h.aCount, h.bStart, h.bCount)
		emitted++
		if emitted >= diffMaxLines || out.Len() >= diffMaxTotalBytes {
			truncated = true
			break
		}
		for _, op := range h.ops {
			line := op.text
			if len(line) > diffMaxLineBytes {
				line = line[:diffMaxLineBytes] + "… [line truncated]"
			}
			out.WriteByte(op.kind)
			out.WriteString(line)
			out.WriteByte('\n')
			emitted++
			if emitted >= diffMaxLines || out.Len() >= diffMaxTotalBytes {
				truncated = true
				break
			}
		}
	}
	if truncated {
		out.WriteString("… [diff elided — 100-line or 16 KiB bound reached]\n")
	}
	return out.String()
}

// lcsDiffOps computes a line-level diff of al and bl via longest-common
// subsequence backtracking. Deletions are preferred over additions when tied,
// making the output a pure function of the inputs.
func lcsDiffOps(al, bl []string) []diffOp {
	n, m := len(al), len(bl)
	// dp[i][j] is the LCS length of al[i:] and bl[j:].
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			ops = append(ops, diffOp{kind: ' ', text: al[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{kind: '-', text: al[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: '+', text: bl[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{kind: '-', text: al[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{kind: '+', text: bl[j]})
	}
	return ops
}

// diffHunk is one unified-diff hunk: the op slice plus the a/b-side start
// line and count for the @@ header.
type diffHunk struct {
	aStart, aCount int
	bStart, bCount int
	ops            []diffOp
}

// unifiedHunks clusters change ops into unified-diff hunks with the given
// number of context lines; changes separated by more than 2*context+1
// context ops start a new hunk.
func unifiedHunks(ops []diffOp, context int) []diffHunk {
	var changeIdx []int
	for i, op := range ops {
		if op.kind != ' ' {
			changeIdx = append(changeIdx, i)
		}
	}
	if len(changeIdx) == 0 {
		return nil
	}

	var hunks []diffHunk
	start := changeIdx[0]
	prev := changeIdx[0]
	for _, idx := range changeIdx[1:] {
		if idx-prev > 2*context+1 {
			hunks = append(hunks, buildHunk(ops, start, prev, context))
			start = idx
		}
		prev = idx
	}
	hunks = append(hunks, buildHunk(ops, start, prev, context))
	return hunks
}

// buildHunk slices ops from firstChange-context to lastChange+context+1 and
// computes the @@ header numbers (GNU convention: a 0-count side starts at
// the line before the hunk).
func buildHunk(ops []diffOp, firstChange, lastChange, context int) diffHunk {
	start := firstChange - context
	if start < 0 {
		start = 0
	}
	end := lastChange + context + 1
	if end > len(ops) {
		end = len(ops)
	}
	h := diffHunk{ops: ops[start:end]}

	aBefore, bBefore := countSides(ops[:start])
	h.aCount, h.bCount = countSides(h.ops)
	h.aStart = aBefore + 1
	h.bStart = bBefore + 1
	if h.aCount == 0 {
		h.aStart = aBefore
	}
	if h.bCount == 0 {
		h.bStart = bBefore
	}
	return h
}

// countSides counts the lines each diff side contributes to ops: context
// and deletions advance the a-side, context and additions advance the b-side.
func countSides(ops []diffOp) (a, b int) {
	for _, op := range ops {
		if op.kind == ' ' || op.kind == '-' {
			a++
		}
		if op.kind == ' ' || op.kind == '+' {
			b++
		}
	}
	return a, b
}
