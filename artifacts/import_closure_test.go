package artifacts

import (
	"os/exec"
	"strings"
	"testing"
)

// TestImportClosure_ExcludesRuntimePackages folds the spec-review advisory
// from the dewey-publish review (iteration 5): the artifacts package must
// stay seam-isolated from Dewey's runtime packages. A dependency on any of
// the banned packages would couple the publish chore (plan/sync) to the
// SQLite store, the curation pipeline, the LLM providers, the embedding
// pipeline, or the content-source manager — none of which it needs. The
// sanitize package is intentionally allowed: screening converts sanitize
// findings into contract Finding values without runtime coupling.
//
// The constraint applies to the artifacts package only. The CLI layer
// (cli.go) may import curate to resolve knowledge-store paths; that seam
// lives outside this package.
func TestImportClosure_ExcludesRuntimePackages(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v", err)
	}

	banned := []string{
		"github.com/unbound-force/dewey/v3/store",
		"github.com/unbound-force/dewey/v3/curate",
		"github.com/unbound-force/dewey/v3/llm",
		"github.com/unbound-force/dewey/v3/embed",
		"github.com/unbound-force/dewey/v3/source",
	}

	for _, line := range strings.Split(string(out), "\n") {
		dep := strings.TrimSpace(line)
		if dep == "" {
			continue
		}
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Errorf("artifacts package depends on banned package %s (seam isolation violated)", dep)
			}
		}
	}
}
