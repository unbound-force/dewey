// Scope computation tests (specs/018-weekly-distill-automation, Phase 2:
// data model, policy loader, and the two enumeration legs — T006, T007,
// T010).
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/store"
)

// --- Data model (T006) ---

func TestNewScopeSet_SortsAndDerivesEmpty(t *testing.T) {
	set := map[string]struct{}{
		"c-20260101T000000-a": {}, "a-20260101T000000-a": {}, "b-20260101T000000-a": {},
	}
	all := map[string]struct{}{}
	for id := range set {
		all[id] = struct{}{}
	}
	cited := map[string]struct{}{"a-20260101T000000-a": {}}
	inScope := map[string]struct{}{"c-20260101T000000-a": {}, "b-20260101T000000-a": {}}

	got := NewScopeSet(all, cited, inScope, "git:origin/main:abc")

	want := ScopeSet{
		AllLearnings: []string{"a-20260101T000000-a", "b-20260101T000000-a", "c-20260101T000000-a"},
		Cited:        []string{"a-20260101T000000-a"},
		InScope:      []string{"b-20260101T000000-a", "c-20260101T000000-a"},
		Empty:        false,
		Provenance:   "git:origin/main:abc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewScopeSet = %+v, want %+v", got, want)
	}

	empty := NewScopeSet(all, cited, map[string]struct{}{}, "filesystem:degraded")
	if !empty.Empty {
		t.Error("Empty = false for empty in_scope, want true")
	}
	if empty.Provenance != ScopeProvenanceDegraded {
		t.Errorf("Provenance = %q, want %q", empty.Provenance, ScopeProvenanceDegraded)
	}
}

func TestIdentityTag(t *testing.T) {
	cases := []struct {
		identity string
		want     string
	}{
		// Current format: {tag}-{YYYYMMDDTHHMMSS}-{author}.
		{"authentication-20260502T143022-alice", "authentication"},
		// Multi-segment tags keep their hyphens.
		{"vault-walker-20260502T143022-bob", "vault-walker"},
		{"api-auth-20260502T143022-carol", "api-auth"},
		// Collision suffixes ride after the author; the tag still ends at
		// the timestamp.
		{"auth-20260502T143022-alice-2", "auth"},
		{"auth-20260502T143022-alice-99", "auth"},
		// Legacy format: {tag}-{sequence}.
		{"auth-1", "auth"},
		{"vault-walker-2", "vault-walker"},
		// Bare or unusual values degrade to the whole identity, mirroring
		// extractTagFromIdentity's no-properties fallback.
		{"auth", "auth"},
		{"20260101T000000-leading-timestamp", "20260101T000000-leading-timestamp"},
	}
	for _, tc := range cases {
		if got := IdentityTag(tc.identity); got != tc.want {
			t.Errorf("IdentityTag(%q) = %q, want %q", tc.identity, got, tc.want)
		}
	}
}

func TestDistillPolicy_ExcludesIdentity(t *testing.T) {
	policy := DistillPolicy{ExcludeTags: []string{"scratch-*", "wip"}}
	cases := []struct {
		identity string
		want     bool
	}{
		// Tag "scratch-notes" matches the scratch-* glob.
		{"scratch-notes-20260101T000000-a", true},
		// Tag "scratch" needs its own exact glob entry — path.Match's *
		// requires at least the hyphen.
		{"scratch-20260101T000000-a", false},
		{"wip-20260101T000000-a", true},
		{"authentication-20260101T000000-a", false},
		{"scratch", false}, // bare legacy tag without the -timestamp- shape
	}
	for _, tc := range cases {
		if got := policy.ExcludesIdentity(tc.identity); got != tc.want {
			t.Errorf("ExcludesIdentity(%q) = %v, want %v", tc.identity, got, tc.want)
		}
	}
}

// --- Policy loader (T007; contracts/scope.md Policy Exclusions) ---

func writePolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "distill-policy.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

func TestLoadDistillPolicy_MissingFileIsDefault(t *testing.T) {
	policy, warnings := LoadDistillPolicy(filepath.Join(t.TempDir(), "absent.yaml"))
	if len(policy.ExcludeTags) != 0 || len(policy.ExcludeStores) != 0 {
		t.Errorf("missing policy yielded exclusions: %+v", policy)
	}
	if len(warnings) != 0 {
		t.Errorf("missing policy yielded warnings: %v", warnings)
	}
}

func TestLoadDistillPolicy_ValidEntries(t *testing.T) {
	path := writePolicy(t, "exclude_tags:\n  - scratch-*\n  - wip\nexclude_stores:\n  - team-draft\n")
	policy, warnings := LoadDistillPolicy(path)
	if len(warnings) != 0 {
		t.Fatalf("valid policy yielded warnings: %v", warnings)
	}
	if !reflect.DeepEqual(policy.ExcludeTags, []string{"scratch-*", "wip"}) {
		t.Errorf("ExcludeTags = %v", policy.ExcludeTags)
	}
	if !reflect.DeepEqual(policy.ExcludeStores, []string{"team-draft"}) {
		t.Errorf("ExcludeStores = %v", policy.ExcludeStores)
	}
}

func TestLoadDistillPolicy_MalformedEntriesSkipped(t *testing.T) {
	path := writePolicy(t, "exclude_tags:\n  - scratch-*\n  - bad**recursive\n  - slash/glob\n  - \"[unclosed\"\nexclude_stores:\n  - Team_Draft\n  - ok-store\n")
	policy, warnings := LoadDistillPolicy(path)

	if !reflect.DeepEqual(policy.ExcludeTags, []string{"scratch-*"}) {
		t.Errorf("ExcludeTags = %v, want only scratch-*", policy.ExcludeTags)
	}
	if !reflect.DeepEqual(policy.ExcludeStores, []string{"ok-store"}) {
		t.Errorf("ExcludeStores = %v, want only ok-store", policy.ExcludeStores)
	}
	// One warning per skipped entry: three bad tags + one bad store.
	if len(warnings) != 4 {
		t.Fatalf("warnings = %d (%v), want 4", len(warnings), warnings)
	}
}

func TestLoadDistillPolicy_OverlyBroadRejected(t *testing.T) {
	for _, entry := range []string{"*", "*/*", "?*", "[]"} {
		path := writePolicy(t, fmt.Sprintf("exclude_tags:\n  - %q\n", entry))
		policy, warnings := LoadDistillPolicy(path)
		if len(policy.ExcludeTags) != 0 {
			t.Errorf("overly broad entry %q was accepted: %v", entry, policy.ExcludeTags)
		}
		if len(warnings) != 1 {
			t.Errorf("entry %q: warnings = %v, want exactly one", entry, warnings)
		}
	}
}

func TestLoadDistillPolicy_UnknownKeyWarned(t *testing.T) {
	path := writePolicy(t, "exclude_tags:\n  - scratch-*\nunknown_key: value\n")
	policy, warnings := LoadDistillPolicy(path)
	if !reflect.DeepEqual(policy.ExcludeTags, []string{"scratch-*"}) {
		t.Errorf("ExcludeTags = %v", policy.ExcludeTags)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unknown key") {
		t.Errorf("warnings = %v, want one unknown-key warning", warnings)
	}
}

func TestLoadDistillPolicy_OverSizeLimitIgnored(t *testing.T) {
	// A file just over 16 KiB: content is valid policy, but the bound
	// ignores it in full (contracts/scope.md: "over limit, ignored").
	var b strings.Builder
	b.WriteString("exclude_tags:\n")
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&b, "  - tag-%03d-some-padding-padding-padding\n", i)
	}
	body := b.String()
	for len(body) <= maxPolicyFileBytes {
		body += body
	}
	path := writePolicy(t, body)
	policy, warnings := LoadDistillPolicy(path)
	if len(policy.ExcludeTags) != 0 {
		t.Errorf("over-size policy yielded exclusions: %d entries", len(policy.ExcludeTags))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "over limit") {
		t.Errorf("warnings = %v, want one over-limit warning", warnings)
	}
}

func TestLoadDistillPolicy_OverArrayLimitIgnored(t *testing.T) {
	var b strings.Builder
	b.WriteString("exclude_tags:\n")
	for i := 0; i <= maxPolicyArrayEntries; i++ {
		fmt.Fprintf(&b, "  - tag-%03d\n", i)
	}
	path := writePolicy(t, b.String())
	policy, warnings := LoadDistillPolicy(path)
	if len(policy.ExcludeTags) != 0 {
		t.Errorf("over-limit array yielded exclusions: %d entries", len(policy.ExcludeTags))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "over limit") {
		t.Errorf("warnings = %v, want one over-limit warning", warnings)
	}
}

func TestLoadDistillPolicy_UnparseableIgnored(t *testing.T) {
	path := writePolicy(t, "exclude_tags: [unclosed\n  bad: : :\n")
	policy, warnings := LoadDistillPolicy(path)
	if len(policy.ExcludeTags) != 0 {
		t.Errorf("unparseable policy yielded exclusions: %v", policy.ExcludeTags)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want exactly one", warnings)
	}
}

func TestLoadDistillPolicy_DepthOverLimitIgnored(t *testing.T) {
	path := writePolicy(t, "exclude_tags:\n  - scratch-*\nnested:\n  deeper:\n    deepest:\n      - x\n")
	policy, warnings := LoadDistillPolicy(path)
	if len(policy.ExcludeTags) != 0 {
		t.Errorf("over-depth policy yielded exclusions: %v", policy.ExcludeTags)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "depth") {
		t.Errorf("warnings = %v, want one depth warning", warnings)
	}
}

func TestLoadDistillPolicy_EntryOverCharLimitSkipped(t *testing.T) {
	long := strings.Repeat("a", maxPolicyEntryChars+1)
	path := writePolicy(t, fmt.Sprintf("exclude_tags:\n  - %s\n  - ok\n", long))
	policy, warnings := LoadDistillPolicy(path)
	if !reflect.DeepEqual(policy.ExcludeTags, []string{"ok"}) {
		t.Errorf("ExcludeTags = %v, want only ok", policy.ExcludeTags)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want exactly one", warnings)
	}
}

// --- Compiled-article sources parsing (T010) ---

func TestParseCompiledSources(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
		ok      bool
	}{
		{
			name:    "sources list",
			content: "---\ntier: draft\ncompiled_at: 2026-11-15T00:00:00Z\nsources:\n  - auth-1\n  - auth-3\ntopic: auth\n---\n\n# Auth\n",
			want:    []string{"auth-1", "auth-3"},
			ok:      true,
		},
		{
			name:    "no sources key cites nothing",
			content: "---\ntier: draft\ntopic: auth\n---\n\n# Auth\n",
			want:    nil,
			ok:      true,
		},
		{
			name:    "no frontmatter",
			content: "# Auth\n\nNo frontmatter at all.\n",
			want:    nil,
			ok:      false,
		},
		{
			name:    "unterminated frontmatter",
			content: "---\ntier: draft\nsources:\n  - auth-1\n",
			want:    nil,
			ok:      false,
		},
		{
			name:    "non-list sources",
			content: "---\nsources: auth-1\n---\n",
			want:    nil,
			ok:      false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseCompiledSources(tc.content)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sources = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- Filesystem enumeration leg (T010; degraded mode) ---

func TestScanDistillTreeFS(t *testing.T) {
	vault := t.TempDir()
	learnings := filepath.Join(vault, distillLearningsPrefix)
	compiled := filepath.Join(vault, distillCompiledPrefix)
	if err := os.MkdirAll(learnings, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(compiled, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"auth-20260101T000000-a", "scratch-x-20260101T000000-b"} {
		if err := os.WriteFile(filepath.Join(learnings, id+".md"), []byte("info"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	article := "---\ntier: draft\nsources:\n  - auth-20260101T000000-a\ntopic: auth\n---\n\n# Auth\n"
	if err := os.WriteFile(filepath.Join(compiled, "auth.md"), []byte(article), 0o644); err != nil {
		t.Fatal(err)
	}

	tree := scanDistillTreeFS(vault)
	if want := []string{"auth-20260101T000000-a", "scratch-x-20260101T000000-b"}; !reflect.DeepEqual(tree.LearningIdentities, want) {
		t.Errorf("LearningIdentities = %v, want %v", tree.LearningIdentities, want)
	}
	cited, ok := tree.CompiledCitations[distillCompiledPrefix+"auth.md"]
	if !ok {
		t.Fatalf("compiled citations missing auth.md: %v", tree.CompiledCitations)
	}
	if !reflect.DeepEqual(cited, []string{"auth-20260101T000000-a"}) {
		t.Errorf("cited = %v", cited)
	}
	if len(tree.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", tree.Warnings)
	}
}

func TestScanDistillTreeFS_MissingDirsAreEmpty(t *testing.T) {
	tree := scanDistillTreeFS(t.TempDir())
	if len(tree.LearningIdentities) != 0 || len(tree.CompiledCitations) != 0 {
		t.Errorf("empty vault yielded content: %+v", tree)
	}
	if len(tree.Warnings) != 0 {
		t.Errorf("empty vault yielded warnings: %v", tree.Warnings)
	}
}

func TestScanDistillTreeFS_UnparseableCompiledWarns(t *testing.T) {
	vault := t.TempDir()
	compiled := filepath.Join(vault, distillCompiledPrefix)
	if err := os.MkdirAll(compiled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compiled, "broken.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := scanDistillTreeFS(vault)
	if len(tree.CompiledCitations) != 0 {
		t.Errorf("unparseable article yielded citations: %v", tree.CompiledCitations)
	}
	if len(tree.Warnings) != 1 || !strings.Contains(tree.Warnings[0], "broken.md") {
		t.Errorf("warnings = %v, want one naming broken.md", tree.Warnings)
	}
}

// --- Git enumeration leg (T010) ---

// scriptedRunner replays canned responses keyed by the first argv word,
// so one runner can answer both the ls-tree and cat-file invocations.
type scriptedRunner struct {
	lsTreeOut string
	lsTreeOk  bool
	blobs     map[string]string // sha → content
	calls     [][]string
}

func (r *scriptedRunner) Run(_ context.Context, dir string, args []string) (string, string, int, error) {
	r.calls = append(r.calls, args)
	switch args[0] {
	case "ls-tree":
		if !r.lsTreeOk {
			return "", "fatal: not a git repository", 128, fmt.Errorf("exit 128")
		}
		return r.lsTreeOut, "", 0, nil
	case "cat-file":
		content, ok := r.blobs[args[2]]
		if !ok {
			return "", "fatal: bad object", 128, fmt.Errorf("exit 128")
		}
		return content, "", 0, nil
	}
	return "", "", -1, fmt.Errorf("unexpected call %v", args)
}

func lsTreeRecord(mode, typ, sha, path string) string {
	return fmt.Sprintf("%s %s %s\t%s\x00", mode, typ, sha, path)
}

func TestEnumerateDistillTreeGit(t *testing.T) {
	res := gitref.RefResolution{Ref: "origin/main", FullRef: "refs/remotes/origin/main", SHA: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", Status: gitref.StatusResolved}
	run := &scriptedRunner{
		lsTreeOk: true,
		lsTreeOut: strings.Join([]string{
			lsTreeRecord("100644", "blob", strings.Repeat("aa", 20), ".uf/dewey/learnings/auth-20260101T000000-a.md"),
			lsTreeRecord("100644", "blob", strings.Repeat("bb", 20), ".uf/dewey/learnings/scratch-20260101T000000-b.md"),
			lsTreeRecord("100644", "blob", strings.Repeat("cc", 20), ".uf/dewey/compiled/auth.md"),
			lsTreeRecord("040000", "tree", strings.Repeat("dd", 20), ".uf/dewey/learnings/subdir"),
		}, ""),
	}

	tree, ok := enumerateDistillTreeGit(context.Background(), run, "/vault", res)
	if !ok {
		t.Fatal("enumeration reported failure for a healthy ls-tree")
	}
	wantIds := []string{"auth-20260101T000000-a", "scratch-20260101T000000-b"}
	if got := tree.learningIdentities(); !reflect.DeepEqual(got, wantIds) {
		t.Errorf("learningIdentities = %v, want %v", got, wantIds)
	}
	wantPaths := []string{".uf/dewey/compiled/auth.md"}
	if got := tree.compiledPaths(); !reflect.DeepEqual(got, wantPaths) {
		t.Errorf("compiledPaths = %v, want %v", got, wantPaths)
	}

	// The batch call covers both prefixes in one invocation.
	if len(run.calls) != 1 {
		t.Fatalf("git invocations = %d, want 1 (batch)", len(run.calls))
	}
	args := run.calls[0]
	if args[0] != "ls-tree" || args[1] != "-r" || args[2] != "-z" {
		t.Errorf("ls-tree args = %v", args)
	}
	foundLearnings, foundCompiled := false, false
	for _, a := range args {
		if a == distillLearningsPrefix {
			foundLearnings = true
		}
		if a == distillCompiledPrefix {
			foundCompiled = true
		}
	}
	if !foundLearnings || !foundCompiled {
		t.Errorf("ls-tree pathspecs missing distill prefixes: %v", args)
	}
	if args[len(args)-len([]string{"--", distillLearningsPrefix, distillCompiledPrefix})] != "--" {
		t.Errorf("ls-tree args missing -- separator before pathspecs: %v", args)
	}
}

func TestEnumerateDistillTreeGit_FailureSignalsFallback(t *testing.T) {
	res := gitref.RefResolution{Ref: "origin/main", FullRef: "refs/remotes/origin/main", Status: gitref.StatusResolved}
	run := &scriptedRunner{lsTreeOk: false}
	if _, ok := enumerateDistillTreeGit(context.Background(), run, "/vault", res); ok {
		t.Fatal("failed ls-tree reported success; caller would skip the degraded fallback")
	}

	// An unresolved ref never invokes git at all.
	run2 := &scriptedRunner{lsTreeOk: true}
	if _, ok := enumerateDistillTreeGit(context.Background(), run2, "/vault", gitref.RefResolution{Ref: "origin/main"}); ok {
		t.Fatal("unresolved ref reported success")
	}
	if len(run2.calls) != 0 {
		t.Errorf("unresolved ref invoked git: %v", run2.calls)
	}
}

func TestBlobContent(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	run := &scriptedRunner{blobs: map[string]string{sha: "---\nsources:\n  - auth-1\n---\n"}}
	content, ok := blobContent(context.Background(), run, "/vault", sha)
	if !ok {
		t.Fatal("blobContent failed for a known sha")
	}
	if !strings.Contains(content, "auth-1") {
		t.Errorf("content = %q", content)
	}

	if _, ok := blobContent(context.Background(), run, "/vault", strings.Repeat("zz", 20)); ok {
		t.Error("blobContent succeeded for an unknown sha")
	}
	if _, ok := blobContent(context.Background(), run, "/vault", ""); ok {
		t.Error("blobContent succeeded for an empty sha")
	}
}

// --- ComputeScope (T016/T017/T020; contracts/scope.md Behavior + Tests) ---

// distillFixtureIdentities is the ten-learning fixture (SC-001): the first
// two are cited by the compiled article's frontmatter sources, one carries
// the scratch-notes tag for policy exclusion, and the remaining seven stay in
// scope unconditionally.
var distillFixtureIdentities = []string{
	"auth-20260101T000001-alice", // cited
	"auth-20260101T000002-bob",   // cited
	"deploy-20260101T000003-carol",
	"vault-20260101T000004-dave",
	"scope-20260101T000005-eve",
	"gate-20260101T000006-frank",
	"doctor-20260101T000007-grace",
	"embed-20260101T000008-heidi",
	"scratch-notes-20260101T000009-ivan", // policy-excluded
	"tmp-notes-20260101T000010-judy",
}

// distillFixtureCompiled cites the first two fixture learnings.
const distillFixtureCompiled = `---
tier: draft
sources:
  - auth-20260101T000001-alice
  - auth-20260101T000002-bob
---
# Authentication
`

// expectedFixtureInScope lists the uncited fixture identities, sorted, with
// the scratch-tagged one included — the default (no policy) expectation.
func expectedFixtureInScope() []string {
	var ids []string
	for _, id := range distillFixtureIdentities {
		if id == "auth-20260101T000001-alice" || id == "auth-20260101T000002-bob" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// initDistillFixtureRepo creates a real git repository whose committed tree
// carries the distill fixture: the ten learnings and the citing compiled
// article. refs/remotes/origin/main points at the commit so the gitref
// precedence chain resolves without a remote, and the vault config leg
// pins origin/main so ambient env or global config cannot redirect the
// fixture. Returns the vault dir and the HEAD sha.
func initDistillFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	git("init", "-q")
	// Pin the resolver's highest-precedence leg (vault config) so the
	// fixture resolves origin/main regardless of ambient environment.
	write(".uf/dewey/config.yaml", "default_branch_ref: origin/main\n")
	for _, id := range distillFixtureIdentities {
		write(".uf/dewey/learnings/"+id+".md", "---\ntier: draft\n---\n# "+id+"\n")
	}
	write(".uf/dewey/compiled/auth.md", distillFixtureCompiled)
	git("add", ".")
	git("commit", "-q", "-m", "distill fixture")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, out)
	}
	return dir, strings.TrimSpace(string(out))
}

// TestComputeScope_GitFixture verifies SC-001's exact-equality case against
// a real git repository: ten learnings, two cited by the compiled article's
// frontmatter, no policy — eight in scope, deterministic from git alone
// with git:<ref>:<sha> provenance.
func TestComputeScope_GitFixture(t *testing.T) {
	dir, sha := initDistillFixtureRepo(t)

	scope, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}

	wantAll := append([]string(nil), distillFixtureIdentities...)
	sort.Strings(wantAll)
	if !reflect.DeepEqual(scope.AllLearnings, wantAll) {
		t.Errorf("AllLearnings = %v, want %v", scope.AllLearnings, wantAll)
	}
	wantCited := []string{"auth-20260101T000001-alice", "auth-20260101T000002-bob"}
	if !reflect.DeepEqual(scope.Cited, wantCited) {
		t.Errorf("Cited = %v, want %v", scope.Cited, wantCited)
	}
	if want := expectedFixtureInScope(); !reflect.DeepEqual(scope.InScope, want) {
		t.Errorf("InScope = %v, want %v (SC-001 exact equality)", scope.InScope, want)
	}
	if scope.Empty {
		t.Error("Empty = true, want false")
	}
	if want := "git:origin/main:" + sha; scope.Provenance != want {
		t.Errorf("Provenance = %q, want %q", scope.Provenance, want)
	}

	// FR-005 determinism: the same repo and policy compute the same set.
	again, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope (second pass): %v", err)
	}
	if !reflect.DeepEqual(scope, again) {
		t.Errorf("second computation differs: %+v vs %+v (FR-005 determinism)", scope, again)
	}
}

// TestComputeScope_PolicyExcludesScratch applies an exclude_tags policy to
// the fixture: scratch-* removes the scratch-tagged learning, everything
// else stays.
func TestComputeScope_PolicyExcludesScratch(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)

	policy := DistillPolicy{ExcludeTags: []string{"scratch-*"}}
	scope, err := ComputeScope(context.Background(), dir, nil, policy)
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}

	want := expectedFixtureInScope()
	filtered := want[:0]
	for _, id := range want {
		if id != "scratch-notes-20260101T000009-ivan" {
			filtered = append(filtered, id)
		}
	}
	if !reflect.DeepEqual(scope.InScope, filtered) {
		t.Errorf("InScope = %v, want %v (scratch-* excluded)", scope.InScope, filtered)
	}
}

// TestComputeScope_OverlyBroadPolicyNotStarved: a ["*"] exclusion is
// rejected by the loader (warn + skip), so the scope it computes is the
// unfiltered default — never starved to empty.
func TestComputeScope_OverlyBroadPolicyNotStarved(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)

	policy, warnings := LoadDistillPolicy(writePolicy(t, "exclude_tags: [\"*\"]\n"))
	if len(warnings) == 0 {
		t.Fatal("overly broad policy produced no warnings")
	}
	scope, err := ComputeScope(context.Background(), dir, nil, policy)
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	if want := expectedFixtureInScope(); !reflect.DeepEqual(scope.InScope, want) {
		t.Errorf("InScope = %v, want the unfiltered default %v", scope.InScope, want)
	}
	if scope.Empty {
		t.Error("Empty = true; an overly broad policy must not starve scope")
	}
}

// TestComputeScope_EmptyScopeIsNoOpSignal verifies the FR-021 signal: a
// repo whose every learning is cited computes an empty in-scope set —
// no-op territory, no PR, no comment.
func TestComputeScope_EmptyScopeIsNoOpSignal(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)

	// Cite every learning: one compiled article listing all ten sources.
	citing := "---\ntier: draft\nsources:\n"
	for _, id := range distillFixtureIdentities {
		citing += "  - " + id + "\n"
	}
	citing += "---\n# Everything\n"
	if err := os.WriteFile(filepath.Join(dir, ".uf/dewey/compiled/everything.md"), []byte(citing), 0o644); err != nil {
		t.Fatalf("write everything.md: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("add", ".")
	git("commit", "-q", "-m", "cite everything")
	// Scope enumerates at the ref, not HEAD: re-point origin/main at the
	// citing commit so the new article is visible to the ls-tree pass.
	git("update-ref", "refs/remotes/origin/main", "HEAD")

	scope, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	if !scope.Empty || len(scope.InScope) != 0 {
		t.Errorf("InScope = %v, want empty (every learning cited)", scope.InScope)
	}
}

// TestComputeScope_NonGitFallsBackDegraded verifies the fallback (edge
// case: non-git vault): the same tree scanned from the filesystem, unioned
// with the SQLite seam's learning names, provenance filesystem:degraded,
// no panic.
func TestComputeScope_NonGitFallsBackDegraded(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	for _, id := range distillFixtureIdentities {
		write(".uf/dewey/learnings/"+id+".md", "---\ntier: draft\n---\n")
	}
	write(".uf/dewey/compiled/auth.md", distillFixtureCompiled)
	// A learning present only in the store seam: the union must pick it
	// up even though no file backs it.
	storeOnly := "orphan-20260101T000011-karl"
	pages := LearningPageNames(func() ([]string, error) {
		return []string{"auth-20260101T000001-alice", storeOnly}, nil
	})

	scope, err := ComputeScope(context.Background(), dir, pages, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	if scope.Provenance != ScopeProvenanceDegraded {
		t.Errorf("Provenance = %q, want %q", scope.Provenance, ScopeProvenanceDegraded)
	}
	want := expectedFixtureInScope()
	want = append(want, storeOnly)
	sort.Strings(want)
	if !reflect.DeepEqual(scope.InScope, want) {
		t.Errorf("InScope = %v, want %v (FS scan unioned with store names)", scope.InScope, want)
	}
}

// TestComputeScope_BadVaultDirErrors: a vault dir that does not exist is a
// hard error — it must not masquerade as an empty no-op scope.
func TestComputeScope_BadVaultDirErrors(t *testing.T) {
	if _, err := ComputeScope(context.Background(), filepath.Join(t.TempDir(), "missing"), nil, DistillPolicy{}); err == nil {
		t.Error("ComputeScope accepted a nonexistent vault dir; want an error")
	}
	if _, err := ComputeScope(context.Background(), "", nil, DistillPolicy{}); err == nil {
		t.Error("ComputeScope accepted an empty vault dir; want an error")
	}
}

// TestStoreLearningPageNames verifies the SQLite fallback seam adapter:
// learning/ draft pages yield their identities, other namespaces are
// dropped, a closed store surfaces a wrapped error, and a nil store yields
// a nil seam (no store, no union).
func TestStoreLearningPageNames(t *testing.T) {
	if got := StoreLearningPageNames(nil); got != nil {
		t.Error("StoreLearningPageNames(nil) returned a non-nil seam, want nil")
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	defer func() { _ = s.Close() }()

	seed := []struct {
		name string
		tier string
	}{
		{"learning/auth-20260101T000001-alice", "draft"},
		{"learning/vault-20260101T000004-dave", "draft"},
		{"learning/legacy-1", "draft"},
		{"compiled/auth", "draft"},        // draft, but not the learning/ namespace
		{"knowledge/team", "draft"},       // draft, but not the learning/ namespace
		{"Learning/case-folded", "draft"}, // NOCASE range still prefixes learning/
	}
	for _, p := range seed {
		if err := s.InsertPage(&store.Page{
			Name:         p.name,
			OriginalName: p.name,
			SourceID:     store.SourceLearning,
			SourceDocID:  p.name, // unique per row for the (source_id, source_doc_id) key
			Tier:         p.tier,
		}); err != nil {
			t.Fatalf("insert %s: %v", p.name, err)
		}
	}

	pages := StoreLearningPageNames(s)
	ids, err := pages()
	if err != nil {
		t.Fatalf("seam: %v", err)
	}
	sort.Strings(ids)
	want := []string{"auth-20260101T000001-alice", "case-folded", "legacy-1", "vault-20260101T000004-dave"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("seam identities = %v, want %v", ids, want)
	}

	// A closed store surfaces a wrapped error, never a panic — the
	// fallback path logs and continues with the filesystem scan.
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := pages(); err == nil || !strings.Contains(err.Error(), "list learning pages") {
		t.Errorf("seam error = %v, want a wrapped list learning pages error", err)
	}
}
