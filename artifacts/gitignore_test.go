package artifacts

import (
	"os"
	"strings"
	"testing"
)

func TestProbeGitignore_NoIgnoresIsOK(t *testing.T) {
	f := newFixtureRepo(t)

	finding, warnings, err := probeGitignore(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusOK {
		t.Fatalf("status = %q, want %q", finding.Status, GitignoreStatusOK)
	}
	if finding.Fix != nil {
		t.Fatalf("fix = %+v, want nil", finding.Fix)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestProbeGitignore_AnchoredDeweyIsOK(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(".gitignore", "/dewey\n")

	finding, warnings, err := probeGitignore(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusOK {
		t.Fatalf("status = %q, want %q (anchored /dewey does not ignore .uf/dewey paths)", finding.Status, GitignoreStatusOK)
	}
	if finding.Fix != nil {
		t.Fatalf("fix = %+v, want nil", finding.Fix)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestProbeGitignore_BareDeweyFix(t *testing.T) {
	f := newFixtureRepo(t)
	f.enableBareDeweyIgnore()

	finding, _, err := probeGitignore(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusBareDeweyFix {
		t.Fatalf("status = %q, want %q", finding.Status, GitignoreStatusBareDeweyFix)
	}
	fix := finding.Fix
	if fix == nil {
		t.Fatalf("fix = nil, want the bounded fix block")
	}
	if fix.File != ".gitignore" {
		t.Fatalf("fix.File = %q, want .gitignore", fix.File)
	}
	if fix.Anchor.From != "dewey" || fix.Anchor.To != "/dewey" {
		t.Fatalf("fix.Anchor = %+v, want {dewey /dewey}", fix.Anchor)
	}
	want := []string{
		".uf/dewey/graph.db",
		".uf/dewey/graph.db-shm",
		".uf/dewey/graph.db-wal",
		".uf/dewey/dewey.log",
		".uf/dewey/dewey.lock",
		".uf/dewey/cache/",
		".uf/dewey/config.yaml",
		".uf/dewey/sources.yaml",
		".uf/dewey/knowledge-stores.yaml",
	}
	if len(fix.Reignore) != len(want) {
		t.Fatalf("fix.Reignore has %d entries, want %d", len(fix.Reignore), len(want))
	}
	for i, w := range want {
		if fix.Reignore[i] != w {
			t.Fatalf("fix.Reignore[%d] = %q, want %q", i, fix.Reignore[i], w)
		}
	}
}

func TestProbeGitignore_OtherIgnoreWarning(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(".gitignore", ".uf/dewey/\n")

	finding, warnings, err := probeGitignore(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusOtherIgnoreWarning {
		t.Fatalf("status = %q, want %q", finding.Status, GitignoreStatusOtherIgnoreWarning)
	}
	if finding.Fix != nil {
		t.Fatalf("fix = %+v, want nil", finding.Fix)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %v (%d), want 3 (one per default trio root)", warnings, len(warnings))
	}
	if !strings.Contains(warnings[0], ".uf/dewey/") || !strings.Contains(warnings[0], ".gitignore:1") {
		t.Fatalf("warnings[0] = %q, want the offending pattern and source location", warnings[0])
	}
}

func TestProbeGitignore_ExternalExclude(t *testing.T) {
	f := newFixtureRepo(t)
	excl := f.path(".git/info/exclude")
	fh, err := os.OpenFile(excl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open .git/info/exclude: %v", err)
	}
	if _, err := fh.WriteString("\ndewey\n"); err != nil {
		t.Fatalf("append to .git/info/exclude: %v", err)
	}
	_ = fh.Close()

	finding, warnings, err := probeGitignore(physicalPath(t, f.root), nil)
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusIgnoreSourceExternal {
		t.Fatalf("status = %q, want %q", finding.Status, GitignoreStatusIgnoreSourceExternal)
	}
	if finding.Fix != nil {
		t.Fatalf("fix = %+v, want nil (external source — fix skipped)", finding.Fix)
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings empty, want the external-source warning")
	}
	if !strings.Contains(warnings[0], ".git/info/exclude") {
		t.Fatalf("warnings[0] = %q, want the external source named", warnings[0])
	}
}

func TestProbeGitignore_StoreProbeIncluded(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(".gitignore", ".uf/dewey/team-store/\n")
	resolved := physicalPath(t, f.root)
	wr, err := validateStorePath(f.root, resolved, ".uf/dewey/team-store")
	if err != nil {
		t.Fatalf("validateStorePath: %v", err)
	}

	finding, warnings, err := probeGitignore(resolved, []walkRoot{wr})
	if err != nil {
		t.Fatalf("probeGitignore: %v", err)
	}
	if finding.Status != GitignoreStatusOtherIgnoreWarning {
		t.Fatalf("status = %q, want %q (only the store probe is ignored)", finding.Status, GitignoreStatusOtherIgnoreWarning)
	}
	if finding.Fix != nil {
		t.Fatalf("fix = %+v, want nil", finding.Fix)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v (%d), want exactly 1 (the store probe)", warnings, len(warnings))
	}
	if !strings.Contains(warnings[0], ".uf/dewey/team-store/probe.md") {
		t.Fatalf("warnings[0] = %q, want the store probe path", warnings[0])
	}
}
