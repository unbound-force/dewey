package vault

import "testing"

// Tests for DeriveEffectiveTier (git-trust-source task 1.1, GTS-FR-001
// through GTS-FR-004). Inputs use literal vocabulary strings rather than
// the canonicalTier*/downgradeValidity* constants on purpose: the literals
// pin the frontmatter wire format, so an accidental vocabulary change
// fails here instead of silently re-defining canonical matching.

const (
	// blobMerged and blobRegenerated stand in for git blob object IDs:
	// blobMerged is the blob at the fetched default-branch ref, and
	// blobRegenerated is a locally regenerated blob that was never merged
	// (issue #204).
	blobMerged      = "e6a6686b3d19b8a1f0b2c4d5e6f708192a3b4c5d"
	blobRegenerated = "9f1e2d3c4b5a6978869a0b1c2d3e4f5061627384"
)

// TestDeriveEffectiveTier_ValidatedOnlyViaBlobEquality covers the
// GTS-FR-001 grant and the GTS-FR-002/GTS-FR-004 withhold paths.
func TestDeriveEffectiveTier_ValidatedOnlyViaBlobEquality(t *testing.T) {
	tests := []struct {
		name string
		in   EffectiveTierInput
		want string
	}{
		{
			name: "canonical validated with matching blobs is validated",
			in:   EffectiveTierInput{FileTier: "validated", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "validated",
		},
		{
			name: "canonical validated with regenerated blob is draft (#204)",
			in:   EffectiveTierInput{FileTier: "validated", FileBlobSHA: blobRegenerated, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "canonical validated with missing file blob is draft",
			in:   EffectiveTierInput{FileTier: "validated", FileBlobSHA: "", DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "canonical validated with unresolvable default-branch ref is draft",
			in:   EffectiveTierInput{FileTier: "validated", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: ""},
			want: "draft",
		},
		{
			name: "canonical validated with both blobs absent is draft, not a trivial match",
			in:   EffectiveTierInput{FileTier: "validated", FileBlobSHA: "", DefaultBranchBlobSHA: ""},
			want: "draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveEffectiveTier(tt.in)
			if got != tt.want {
				t.Errorf("DeriveEffectiveTier(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDeriveEffectiveTier_CanonicalMatchingIsExact covers GTS-FR-001's
// canonical-match definition: non-canonical tier values (case variants,
// whitespace padding, unknown vocabulary, absent) are unknown and never
// raise trust, even with blob equality.
func TestDeriveEffectiveTier_CanonicalMatchingIsExact(t *testing.T) {
	tests := []struct {
		name string
		in   EffectiveTierInput
		want string
	}{
		{
			name: "tier Validated with blob match is draft",
			in:   EffectiveTierInput{FileTier: "Validated", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "tier VALIDATED with blob match is draft",
			in:   EffectiveTierInput{FileTier: "VALIDATED", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "leading-whitespace-padded validated is unknown and draft",
			in:   EffectiveTierInput{FileTier: " validated", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "trailing-whitespace-padded validated is unknown and draft",
			in:   EffectiveTierInput{FileTier: "validated ", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "absent tier never raises trust despite blob match",
			in:   EffectiveTierInput{FileTier: "", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "tier value outside the vocabulary never raises trust",
			in:   EffectiveTierInput{FileTier: "trusted", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "case-variant Draft is unknown, not the draft downgrade signal",
			in:   EffectiveTierInput{FileTier: "Draft", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveEffectiveTier(tt.in)
			if got != tt.want {
				t.Errorf("DeriveEffectiveTier(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDeriveEffectiveTier_DowngradeWinsRegardlessOfMatch covers the
// GTS-FR-003 exhaustive downgrade predicate: canonical tier draft, the
// three canonical validity downgrades, and the closed-set behavior of
// unknown validity values.
func TestDeriveEffectiveTier_DowngradeWinsRegardlessOfMatch(t *testing.T) {
	tests := []struct {
		name string
		in   EffectiveTierInput
		want string
	}{
		{
			name: "canonical tier draft wins even with blob match",
			in:   EffectiveTierInput{FileTier: "draft", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "validity stale-unverified downgrades validated despite blob match",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "stale-unverified", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "validity superseded downgrades validated despite blob match",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "superseded", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "validity historical downgrades validated despite blob match",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "historical", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
		{
			name: "validity current is not a downgrade signal",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "current", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "validated",
		},
		{
			name: "case-variant validity Stale-Unverified is not a downgrade signal",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "Stale-Unverified", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "validated",
		},
		{
			name: "uppercase validity SUPERSEDED is not a downgrade signal",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "SUPERSEDED", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "validated",
		},
		{
			name: "padded validity superseded is not a downgrade signal",
			in:   EffectiveTierInput{FileTier: "validated", Validity: " superseded ", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "validated",
		},
		{
			name: "unknown validity cannot raise trust past the git rule",
			in:   EffectiveTierInput{FileTier: "validated", Validity: "super-validated", FileBlobSHA: blobRegenerated, DefaultBranchBlobSHA: blobMerged},
			want: "draft",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveEffectiveTier(tt.in)
			if got != tt.want {
				t.Errorf("DeriveEffectiveTier(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDeriveEffectiveTier_NonValidatedTiersPreserved covers GTS-FR-001's
// preservation clause: canonical non-validated tiers are kept as declared,
// independent of blob state, and validity stays orthogonal to them.
func TestDeriveEffectiveTier_NonValidatedTiersPreserved(t *testing.T) {
	tests := []struct {
		name string
		in   EffectiveTierInput
		want string
	}{
		{
			name: "curated preserved with blob match",
			in:   EffectiveTierInput{FileTier: "curated", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "curated",
		},
		{
			name: "authored preserved with blob match",
			in:   EffectiveTierInput{FileTier: "authored", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "authored",
		},
		{
			name: "untrusted preserved with blob match",
			in:   EffectiveTierInput{FileTier: "untrusted", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "untrusted",
		},
		{
			name: "curated preserved on blob mismatch; the gate must not degrade it",
			in:   EffectiveTierInput{FileTier: "curated", FileBlobSHA: blobRegenerated, DefaultBranchBlobSHA: blobMerged},
			want: "curated",
		},
		{
			name: "untrusted preserved with unresolvable default-branch ref",
			in:   EffectiveTierInput{FileTier: "untrusted", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: ""},
			want: "untrusted",
		},
		{
			name: "validity downgrade is orthogonal to the curated tier",
			in:   EffectiveTierInput{FileTier: "curated", Validity: "superseded", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "curated",
		},
		{
			name: "validity downgrade is orthogonal to the authored tier",
			in:   EffectiveTierInput{FileTier: "authored", Validity: "stale-unverified", FileBlobSHA: blobMerged, DefaultBranchBlobSHA: blobMerged},
			want: "authored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveEffectiveTier(tt.in)
			if got != tt.want {
				t.Errorf("DeriveEffectiveTier(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsLearningPage_Classification covers the IP-FR-002 page-family
// predicate: the learning/ and learnings/ namespaces, case-insensitively,
// with the trailing slash load-bearing. Source document IDs carry the
// family prefix the same way vault-relative page names do, while the
// source-namespaced page name (disk-local/...) does not — classification
// happens before namespacing.
func TestIsLearningPage_Classification(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "vault-relative learning page", path: "learning/auth-1", want: true},
		{name: "vault-relative learnings page", path: "learnings/auth-1", want: true},
		{name: "uppercase Learning namespace", path: "Learning/auth-1", want: true},
		{name: "uppercase LEARNINGS namespace", path: "LEARNINGS/auth-1", want: true},
		{name: "source document ID with .md suffix", path: "learning/auth-1.md", want: true},
		{name: "nested page under the namespace", path: "learning/2026/auth-1", want: true},
		{name: "bare learning page is not in the family", path: "learning", want: false},
		{name: "sibling namespace is not in the family", path: "learning-notes/auth-1", want: false},
		{name: "compiled page is not a learning page", path: "compiled/auth", want: false},
		{name: "knowledge page is not a learning page", path: "knowledge/store/auth-1", want: false},
		{name: "source-namespaced page name does not carry the family", path: "disk-local/learning/auth-1.md", want: false},
		{name: "ordinary vault page", path: "specs/008-workflow", want: false},
		{name: "empty path", path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLearningPage(tt.path); got != tt.want {
				t.Errorf("isLearningPage(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestIsGitTrustPage_Classification covers the compiled/curated page-family
// predicate: the compiled/ and knowledge/ namespaces, case-insensitively,
// with the trailing slash load-bearing.
func TestIsGitTrustPage_Classification(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "compiled page", path: "compiled/authentication", want: true},
		{name: "uppercase Compiled namespace", path: "Compiled/authentication", want: true},
		{name: "uppercase COMPILED namespace", path: "COMPILED/authentication", want: true},
		{name: "knowledge store page", path: "knowledge/team-knowledge/auth-1", want: true},
		{name: "uppercase Knowledge namespace", path: "Knowledge/team/auth-1", want: true},
		{name: "source document ID with .md suffix", path: "compiled/auth.md", want: true},
		{name: "bare compiled page is not in the family", path: "compiled", want: false},
		{name: "compile namespace is not in the family", path: "compile/auth", want: false},
		{name: "sibling namespace is not in the family", path: "compiled-todos/auth", want: false},
		{name: "learning page is not a git-trust page", path: "learning/auth-1", want: false},
		{name: "source-namespaced page name does not carry the family", path: "disk-local/compiled/auth.md", want: false},
		{name: "ordinary vault page", path: "projects/dewey", want: false},
		{name: "empty path", path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGitTrustPage(tt.path); got != tt.want {
				t.Errorf("isGitTrustPage(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestDeriveTierFromProps_PropsExtraction covers the frontmatter seam:
// string-valued tier/validity properties feed DeriveEffectiveTier's rules,
// and absent, nil, or non-string values read as unknown (never raising
// trust).
func TestDeriveTierFromProps_PropsExtraction(t *testing.T) {
	tests := []struct {
		name             string
		props            map[string]any
		fileBlobSHA      string
		defaultBranchSHA string
		want             string
	}{
		{
			name:  "curated claim preserved without blobs",
			props: map[string]any{"tier": "curated"},
			want:  "curated",
		},
		{
			name:  "authored claim preserved without blobs",
			props: map[string]any{"tier": "authored"},
			want:  "authored",
		},
		{
			name:  "untrusted claim preserved without blobs",
			props: map[string]any{"tier": "untrusted"},
			want:  "untrusted",
		},
		{
			name:             "validated claim with blob equality is validated",
			props:            map[string]any{"tier": "validated"},
			fileBlobSHA:      blobMerged,
			defaultBranchSHA: blobMerged,
			want:             "validated",
		},
		{
			name:  "validated claim without blob proof is draft",
			props: map[string]any{"tier": "validated"},
			want:  "draft",
		},
		{
			name:             "validity downgrade withholds validated despite blob match",
			props:            map[string]any{"tier": "validated", "validity": "superseded"},
			fileBlobSHA:      blobMerged,
			defaultBranchSHA: blobMerged,
			want:             "draft",
		},
		{
			name:  "non-string tier reads as unknown",
			props: map[string]any{"tier": 42},
			want:  "draft",
		},
		{
			name:  "case-variant tier reads as unknown",
			props: map[string]any{"tier": "Curated"},
			want:  "draft",
		},
		{
			name:  "nil props read as unknown",
			props: nil,
			want:  "draft",
		},
		{
			name:  "empty props read as unknown",
			props: map[string]any{},
			want:  "draft",
		},
		{
			name:             "non-string validity is not a downgrade signal",
			props:            map[string]any{"tier": "validated", "validity": true},
			fileBlobSHA:      blobMerged,
			defaultBranchSHA: blobMerged,
			want:             "validated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveTierFromProps(tt.props, tt.fileBlobSHA, tt.defaultBranchSHA)
			if got != tt.want {
				t.Errorf("DeriveTierFromProps(%v, %q, %q) = %q, want %q",
					tt.props, tt.fileBlobSHA, tt.defaultBranchSHA, got, tt.want)
			}
		})
	}
}

// TestIngestTier_FamilyPolicy covers the ingestion tier policy: learning
// pages clamp to draft, git-trust pages derive from frontmatter (the blob
// object IDs are forwarded, not hardcoded), and every other family keeps
// the caller's existing assignment (ok=false).
func TestIngestTier_FamilyPolicy(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		props            map[string]any
		fileBlobSHA      string
		defaultBranchSHA string
		wantTier         string
		wantKnown        bool
	}{
		{
			name:      "learning page clamps to draft despite validated claim",
			path:      "learning/auth-1",
			props:     map[string]any{"tier": "validated"},
			wantTier:  "draft",
			wantKnown: true,
		},
		{
			name:      "learnings page clamps to draft",
			path:      "learnings/auth-1.md",
			props:     nil,
			wantTier:  "draft",
			wantKnown: true,
		},
		{
			name:             "learning page clamps to draft even with blob equality",
			path:             "learning/auth-1",
			props:            map[string]any{"tier": "validated"},
			fileBlobSHA:      blobMerged,
			defaultBranchSHA: blobMerged,
			wantTier:         "draft",
			wantKnown:        true,
		},
		{
			name:      "compiled page derives curated from frontmatter",
			path:      "compiled/auth.md",
			props:     map[string]any{"tier": "curated"},
			wantTier:  "curated",
			wantKnown: true,
		},
		{
			name:      "knowledge page derives draft from absent tier",
			path:      "knowledge/team/auth-1",
			props:     nil,
			wantTier:  "draft",
			wantKnown: true,
		},
		{
			name:      "compiled page withholds validated without blob proof",
			path:      "compiled/auth.md",
			props:     map[string]any{"tier": "validated"},
			wantTier:  "draft",
			wantKnown: true,
		},
		{
			name:             "compiled page grants validated when blobs match",
			path:             "compiled/auth.md",
			props:            map[string]any{"tier": "validated"},
			fileBlobSHA:      blobMerged,
			defaultBranchSHA: blobMerged,
			wantTier:         "validated",
			wantKnown:        true,
		},
		{
			name:      "ordinary vault page keeps caller behavior",
			path:      "specs/008-workflow",
			props:     map[string]any{"tier": "validated"},
			wantTier:  "",
			wantKnown: false,
		},
		{
			name:      "source document ID outside the families keeps caller behavior",
			path:      "issues/123",
			props:     nil,
			wantTier:  "",
			wantKnown: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTier, gotKnown := ingestTier(tt.path, tt.props, tt.fileBlobSHA, tt.defaultBranchSHA)
			if gotTier != tt.wantTier || gotKnown != tt.wantKnown {
				t.Errorf("ingestTier(%q, ...) = (%q, %v), want (%q, %v)",
					tt.path, gotTier, gotKnown, tt.wantTier, tt.wantKnown)
			}
		})
	}
}
