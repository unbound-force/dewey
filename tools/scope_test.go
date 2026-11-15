// Scope computation tests (specs/018-weekly-distill-automation, Phase 2:
// data model, policy loader, and the two enumeration legs — T006, T007,
// T010).
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/gitref"
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
			name: "sources list",
			content: "---\ntier: draft\ncompiled_at: 2026-11-15T00:00:00Z\nsources:\n  - auth-1\n  - auth-3\ntopic: auth\n---\n\n# Auth\n",
			want: []string{"auth-1", "auth-3"},
			ok:   true,
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
