package artifacts

import "sort"

// buildConflictEntry turns a path group whose candidates carry more than one
// distinct content sha256 into a conflict entry: one variant per distinct
// sha256, each variant carrying every enumeration source that produced that
// content (sorted), with an empty findings slice that the screening stage
// fills per variant. Variants are sorted by sha256.
//
// Collisions are across stranded-state sources only: a path whose content
// differs from the remote base tree is a plain new entry, never a conflict —
// the caller routes single-sha groups to buildArtifactEntry, and this
// function reports ok=false for groups without at least two distinct
// sha256 values (including empty and single-candidate groups).
func buildConflictEntry(group pathGroup) (Conflict, bool) {
	if len(group.cands) < 2 {
		return Conflict{}, false
	}

	// Sources are unique within a group (groupCandidatesByPath keeps one
	// candidate per source), so plain appends cannot duplicate.
	sourcesBySHA := make(map[string][]string)
	for _, c := range group.cands {
		sourcesBySHA[c.SHA256] = append(sourcesBySHA[c.SHA256], c.Source)
	}
	if len(sourcesBySHA) < 2 {
		return Conflict{}, false
	}

	first := group.cands[0]
	variants := make([]Variant, 0, len(sourcesBySHA))
	for sha, sources := range sourcesBySHA {
		sort.Strings(sources)
		variants = append(variants, Variant{
			SHA256:   sha,
			Sources:  sources,
			Findings: []Finding{},
		})
	}
	sort.Slice(variants, func(i, j int) bool { return variants[i].SHA256 < variants[j].SHA256 })

	return Conflict{
		Path:     group.path,
		Identity: first.Identity,
		Variants: variants,
	}, true
}
