package artifacts

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDeriveKindTag exercises every root-classification branch directly: the
// three known store suffixes, the custom-root fallback, the empty relative
// segment inside a knowledge root, and a path that matches no configured
// root.
func TestDeriveKindTag(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		identity string
		roots    []string
		wantKind string
		wantTag  string
	}{
		{
			name:     "learning root yields learning kind and tag stem",
			path:     ".uf/dewey/learnings/authentication-20260101T000000Z-alice.md",
			identity: "authentication-20260101T000000Z-alice",
			roots:    []string{".uf/dewey/learnings/"},
			wantKind: KindLearning,
			wantTag:  "authentication",
		},
		{
			name:     "compiled root yields compiled kind and full identity",
			path:     ".uf/dewey/compiled/authentication.md",
			identity: "authentication",
			roots:    []string{".uf/dewey/compiled/"},
			wantKind: KindCompiled,
			wantTag:  "authentication",
		},
		{
			name:     "knowledge root yields first path segment as tag",
			path:     ".uf/dewey/knowledge/team/decision.md",
			identity: "team-1234",
			roots:    []string{".uf/dewey/knowledge/"},
			wantKind: KindKnowledge,
			wantTag:  "team",
		},
		{
			name:     "knowledge root with empty relative segment falls back to identity",
			path:     ".uf/dewey/knowledge/",
			identity: "fallback-id",
			roots:    []string{".uf/dewey/knowledge/"},
			wantKind: KindKnowledge,
			wantTag:  "fallback-id",
		},
		{
			name:     "custom store root defaults to knowledge with dirname tag",
			path:     ".uf/dewey/knowledge/custom-store/foo.md",
			identity: "ignored",
			roots:    []string{".uf/dewey/knowledge/custom-store/"},
			wantKind: KindKnowledge,
			wantTag:  "custom-store",
		},
		{
			name:     "path matching no root defaults to knowledge with identity",
			path:     "somewhere/else.md",
			identity: "orphan",
			roots:    []string{".uf/dewey/learnings/"},
			wantKind: KindKnowledge,
			wantTag:  "orphan",
		},
		{
			name:     "prefix miss then hit across multiple roots",
			path:     ".uf/dewey/compiled/x.md",
			identity: "x-stem",
			roots:    []string{".uf/dewey/learnings/", ".uf/dewey/compiled/"},
			wantKind: KindCompiled,
			wantTag:  "x-stem",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, tag := deriveKindTag(tc.path, tc.identity, tc.roots)
			if kind != tc.wantKind {
				t.Errorf("deriveKindTag(%q, %q, %v) kind = %q, want %q",
					tc.path, tc.identity, tc.roots, kind, tc.wantKind)
			}
			if tag != tc.wantTag {
				t.Errorf("deriveKindTag(%q, %q, %v) tag = %q, want %q",
					tc.path, tc.identity, tc.roots, tag, tc.wantTag)
			}
		})
	}
}

// TestRenderBoundedDiff_Bounds drives every bound in the bounded-diff
// renderer: the whole-file fallback for oversized inputs, the 100-line and
// 16 KiB output bounds, per-line truncation, and the identical-content
// short circuit.
func TestRenderBoundedDiff_Bounds(t *testing.T) {
	t.Run("oversized input falls back to whole-file diff", func(t *testing.T) {
		contentA := strings.Repeat("a\n", diffMaxInputLines+1)
		got := renderBoundedDiff("left", "right", contentA, "b\n")
		if !strings.Contains(got, "--- left") || !strings.Contains(got, "+++ right") {
			t.Errorf("renderBoundedDiff oversized input missing headers:\n%s", got)
		}
		// The fallback emits whole-file op counts: 802 a-side lines (801
		// repeats plus the trailing empty line) against 2 b-side lines. The
		// additions themselves are elided by the 100-line output bound.
		if !strings.Contains(got, "-a") || !strings.Contains(got, "@@ -1,802 +1,2 @@") {
			t.Errorf("renderBoundedDiff oversized input missing whole-file ops:\n%s", got)
		}
	})

	t.Run("line-count bound elides the diff", func(t *testing.T) {
		var a, b strings.Builder
		for i := 0; i < diffMaxLines+50; i++ {
			a.WriteString("old\n")
			b.WriteString("new\n")
		}
		got := renderBoundedDiff("left", "right", a.String(), b.String())
		if !strings.Contains(got, "diff elided") {
			t.Errorf("renderBoundedDiff line bound not applied:\n%s", got)
		}
	})

	t.Run("byte bound elides the diff under the line bound", func(t *testing.T) {
		// 40 deletions plus 40 additions stays under the 100-line bound,
		// but the wide addition lines push output past 16 KiB.
		longLine := strings.Repeat("z", 500) + "\n"
		var a, b strings.Builder
		for i := 0; i < 40; i++ {
			a.WriteString("x\n")
			b.WriteString(longLine)
		}
		got := renderBoundedDiff("left", "right", a.String(), b.String())
		if !strings.Contains(got, "diff elided") {
			t.Errorf("renderBoundedDiff byte bound not applied:\n%s", got)
		}
	})

	t.Run("oversized line is truncated", func(t *testing.T) {
		contentB := strings.Repeat("x", diffMaxLineBytes+1024) + "\n"
		got := renderBoundedDiff("left", "right", "short\n", contentB)
		if !strings.Contains(got, "line truncated") {
			t.Errorf("renderBoundedDiff line truncation not applied:\n%s", got)
		}
	})

	t.Run("identical content renders headers only", func(t *testing.T) {
		got := renderBoundedDiff("left", "right", "same\n", "same\n")
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("renderBoundedDiff identical content produced %d lines, want 2:\n%s", len(lines), got)
		}
	})
}

// TestBuildHunk_OneSidedCounts pins the GNU @@ header convention for hunks
// where one side contributes no lines, plus the context-window clamps.
func TestBuildHunk_OneSidedCounts(t *testing.T) {
	t.Run("pure addition at file start", func(t *testing.T) {
		ops := []diffOp{{kind: '+', text: "a"}, {kind: '+', text: "b"}}
		h := buildHunk(ops, 0, 1, 3)
		if h.aCount != 0 || h.bCount != 2 {
			t.Errorf("aCount/bCount = %d/%d, want 0/2", h.aCount, h.bCount)
		}
		if h.aStart != 0 {
			t.Errorf("aStart = %d, want 0 (line before the hunk)", h.aStart)
		}
		if h.bStart != 1 {
			t.Errorf("bStart = %d, want 1", h.bStart)
		}
	})

	t.Run("pure deletion at file start", func(t *testing.T) {
		ops := []diffOp{{kind: '-', text: "a"}, {kind: '-', text: "b"}}
		h := buildHunk(ops, 0, 1, 3)
		if h.aCount != 2 || h.bCount != 0 {
			t.Errorf("aCount/bCount = %d/%d, want 2/0", h.aCount, h.bCount)
		}
		if h.aStart != 1 {
			t.Errorf("aStart = %d, want 1", h.aStart)
		}
		if h.bStart != 0 {
			t.Errorf("bStart = %d, want 0 (line before the hunk)", h.bStart)
		}
	})

	t.Run("context before the hunk advances both starts", func(t *testing.T) {
		ops := []diffOp{
			{kind: ' ', text: "c1"},
			{kind: ' ', text: "c2"},
			{kind: ' ', text: "c3"},
			{kind: ' ', text: "c4"},
			{kind: '-', text: "gone"},
			{kind: '+', text: "new"},
		}
		h := buildHunk(ops, 4, 5, 3)
		if len(h.ops) != 5 {
			t.Errorf("len(h.ops) = %d, want 5 (start clamped to 1)", len(h.ops))
		}
		if h.aCount != 4 || h.bCount != 4 {
			t.Errorf("aCount/bCount = %d/%d, want 4/4", h.aCount, h.bCount)
		}
		if h.aStart != 2 || h.bStart != 2 {
			t.Errorf("aStart/bStart = %d/%d, want 2/2", h.aStart, h.bStart)
		}
	})

	t.Run("context window clamps to the op slice", func(t *testing.T) {
		ops := []diffOp{{kind: '-', text: "x"}, {kind: '+', text: "y"}}
		h := buildHunk(ops, 0, 1, 3)
		if len(h.ops) != 2 {
			t.Errorf("len(h.ops) = %d, want 2 (end clamped)", len(h.ops))
		}
	})
}

// TestEnumerateStashes_ErrorPaths covers the guard rails that reject bad
// store paths and fail fast when the repository root is unusable.
func TestEnumerateStashes_ErrorPaths(t *testing.T) {
	t.Run("store path escaping the repository is rejected", func(t *testing.T) {
		_, _, _, err := enumerateStashes(t.TempDir(), []string{"../outside/learnings"})
		if err == nil || !strings.Contains(err.Error(), "is outside the git repository") {
			t.Fatalf("enumerateStashes escaping store path: err = %v", err)
		}
	})

	t.Run("non-git directory fails the stash listing", func(t *testing.T) {
		_, _, _, err := enumerateStashes(t.TempDir(), nil)
		if err == nil || !strings.Contains(err.Error(), "list stashes") {
			t.Fatalf("enumerateStashes non-git dir: err = %v", err)
		}
	})

	t.Run("nonexistent repository root fails symlink resolution", func(t *testing.T) {
		_, _, _, err := enumerateStashes(filepath.Join(t.TempDir(), "missing"), nil)
		if err == nil || !strings.Contains(err.Error(), "resolve repository root") {
			t.Fatalf("enumerateStashes missing root: err = %v", err)
		}
	})
}
