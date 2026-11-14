package gitref

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BlobMapResult is the outcome of one batch default-branch blob mapping: the
// parsed path→blob map plus the signal that distinguishes IP-FR-001's two
// empty-map causes. A healthy invocation whose tree simply lacks the
// requested paths returns an authoritative (possibly empty) Blobs with
// BatchFailed false — withholding is correct because no default-branch blob
// exists to equal. A failed invocation returns BatchFailed true with an empty
// Blobs, so a caller holding a resolved ref MAY recover per-path SHAs through
// per-page lookups instead of withholding every page in the run.
type BlobMapResult struct {
	// Blobs maps repository path → default-branch blob object ID. It is
	// empty whenever BatchFailed is true.
	Blobs map[string]string
	// BatchFailed reports that the batch ls-tree invocation itself failed —
	// as opposed to a successful invocation over a tree that lacks the
	// paths, or a guard that skipped git entirely because no runner or
	// resolvable ref exists for a fallback to use.
	BatchFailed bool
}

// MapDefaultBranchBlobsResult maps repository paths to their blob object IDs
// at the resolved default-branch ref using one batch `git ls-tree -r -z`
// invocation for all requested pathspecs — design D2's batch fast path and
// IP-FR-001's "Batch ref resolution prefers ls-tree" scenario: N
// compiled/curated pages compare against one in-memory map instead of N
// per-page `cat-file`/`hash-object` lookups — and reports whether that
// invocation itself failed, the distinction IP-FR-001's per-page fallback
// needs.
//
// The invocation follows the package contract: an argv array (never a shell
// string, per IP-FR-004), the fully qualified res.FullRef, a `--` separator
// before the pathspecs, bounded by the per-call timeout, with context
// cancellation propagated to the git child process. Records are
// NUL-separated `<mode> <type> <sha>\t<path>` entries (mirroring the
// artifacts/dedupe.go parsing pattern); only blob entries are kept.
//
// It never returns an error: a failed batch yields BatchFailed true with an
// empty Blobs, which withholds the validated grant (GTS-FR-004) while
// ingestion continues — unless the caller arms the per-page fallback.
func MapDefaultBranchBlobsResult(ctx context.Context, run Runner, repoDir string, res RefResolution, paths []string) BlobMapResult {
	blobs := make(map[string]string)
	if run == nil || len(paths) == 0 || !res.Resolved() || res.FullRef == "" {
		// Not attempted, so not failed: with no runner, no pathspecs, or no
		// resolvable ref there is nothing a per-page fallback could use, and
		// the empty map is the authoritative (withholding) answer.
		return BlobMapResult{Blobs: blobs}
	}

	ctx, cancel := context.WithTimeout(ctx, invokeTimeout)
	defer cancel()

	args := make([]string, 0, len(paths)+5)
	args = append(args, "ls-tree", "-r", "-z", res.FullRef, "--")
	args = append(args, paths...)
	out, _, code, err := run.Run(ctx, repoDir, args)
	if err != nil || code != 0 {
		return BlobMapResult{Blobs: blobs, BatchFailed: true}
	}

	parsed, ok := parseLsTreeBlobs(out)
	if !ok {
		return BlobMapResult{Blobs: blobs, BatchFailed: true}
	}
	return BlobMapResult{Blobs: parsed}
}

// MapDefaultBranchBlobs maps repository paths to their blob object IDs at the
// resolved default-branch ref — the empty-map-semantics wrapper around
// MapDefaultBranchBlobsResult. Any failure — an unresolvable resolution, a
// nil runner, no paths, a non-zero exit, a failure to run git at all
// (timeout, cancellation), or malformed/truncated output — returns an empty
// map and never an error: an empty map withholds the validated grant
// (GTS-FR-004) while ingestion continues. Callers that need to distinguish a
// failed batch invocation from a healthy map over a tree lacking the paths —
// to arm IP-FR-001's per-page fallback — MUST use MapDefaultBranchBlobsResult.
func MapDefaultBranchBlobs(ctx context.Context, run Runner, repoDir string, res RefResolution, paths []string) map[string]string {
	return MapDefaultBranchBlobsResult(ctx, run, repoDir, res, paths).Blobs
}

// parseLsTreeBlobs parses NUL-separated `<mode> <type> <sha>\t<path>` ls-tree
// records into a path → blob object ID map, shared by the batch and
// single-path invocations. Non-blob records (trees, gitlinks) are valid
// entries, just not pages, and are skipped. Any malformed or truncated record
// fails the whole parse — a partially trusted map could grant validated on a
// path whose real entry was the corrupted one — so ok is false and callers
// must discard the map.
func parseLsTreeBlobs(out string) (map[string]string, bool) {
	blobs := make(map[string]string)
	for _, record := range strings.Split(out, "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			return nil, false
		}
		meta := strings.Fields(record[:tab])
		if len(meta) != 3 {
			return nil, false
		}
		if meta[1] != "blob" {
			// Trees and gitlinks are valid records, just not pages.
			continue
		}
		if !isHex40(meta[2]) {
			return nil, false
		}
		blobs[record[tab+1:]] = meta[2]
	}
	return blobs, true
}

// FileBlobSHA computes the git blob object ID for the file at path — the
// in-process equivalent of `git hash-object <path>`: SHA-1 over the canonical
// loose-object payload `blob <size>\x00` followed by the raw file content. It
// answers the working-tree side of the IP-FR-001 comparison in process, so
// trust derivation never depends on git plumbing for that side — the ref side
// comes from the batch ls-tree map, or from the per-page ls-tree fallback
// when the batch invocation failed.
//
// A read error returns the error; callers treat it as an absent file blob SHA,
// which withholds the validated grant (GTS-FR-004) without failing ingestion.
func FileBlobSHA(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %s for blob hash: %w", path, err)
	}
	return BlobSHA(data), nil
}

// BlobSHA computes the git blob object ID for raw content — SHA-1 over
// `blob <size>\x00` plus the content bytes. Extracted from FileBlobSHA so
// callers holding content in memory hash it without a re-read.
func BlobSHA(content []byte) string {
	h := sha1.New()
	// QF1012 fix: Fprintf replaces Write([]byte(Sprintf(...))); the explicit
	// discard keeps errcheck satisfied — SHA-1 hashing cannot fail.
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// BlobResolver answers per-page blob object IDs for trust-tier derivation
// (IP-FR-001) from one resolved default-branch ref and its batch blob map.
// It satisfies the vault package's TierBlobResolver seam structurally: vault
// defines the interface it consumes, gitref provides the implementation, and
// the orchestrator wires them together — Dependency Inversion with neither
// package importing the other.
//
// A BlobResolver is safe for concurrent use. Unarmed (healthy batch map, or
// the ref never resolved) it is immutable after construction: PageBlobs only
// reads the snapshot map and the working-tree file. Armed with the per-page
// fallback — batch ls-tree failed while the ref resolved — PageBlobs may
// additionally run one bounded single-path ls-tree per distinct path,
// memoized under fallbackMu so each path invokes git at most once per
// resolver and concurrent lookups stay safe.
type BlobResolver struct {
	repoDir string
	blobs   map[string]string

	// Per-page fallback state (IP-FR-001), zero when unarmed. fallbackRunner
	// and fallbackFullRef are fixed at construction and read-only afterwards;
	// fallbackMu guards fallbackMemo and serializes the bounded git
	// invocation it accompanies — the lock-held subprocess run is the rare
	// batch-failure recovery path, and it is what upholds the
	// one-invocation-per-path contract under concurrency.
	fallbackRunner  Runner
	fallbackFullRef string
	fallbackMu      sync.Mutex
	fallbackMemo    map[string]string
}

// NewBlobResolver returns a BlobResolver that resolves repo-relative file
// paths against repoDir on disk and the given default-branch blob map
// (path → blob object ID, as produced by MapDefaultBranchBlobs). The
// resolver carries no per-page fallback: every map miss withholds the
// validated grant. Callers that hold a resolved ref recovered from a failed
// batch invocation arm the fallback through NewPageBlobResolver.
func NewBlobResolver(repoDir string, blobs map[string]string) *BlobResolver {
	return &BlobResolver{repoDir: repoDir, blobs: blobs}
}

// newFallbackBlobResolver arms the IP-FR-001 per-page fallback: map misses
// run one bounded single-path `git ls-tree -z <fullRef> -- <path>` through
// run before withholding. blobs stays ahead of the fallback in lookup order,
// so a partially populated map still serves its hits directly.
func newFallbackBlobResolver(repoDir string, blobs map[string]string, run Runner, fullRef string) *BlobResolver {
	return &BlobResolver{
		repoDir:         repoDir,
		blobs:           blobs,
		fallbackRunner:  run,
		fallbackFullRef: fullRef,
		fallbackMemo:    make(map[string]string),
	}
}

// PageBlobs returns the working-tree and default-branch blob object IDs for
// the page file at filePath (repo-relative, .md extension included, for
// example "compiled/auth.md").
//
// A path absent from the default-branch side returns two empty strings
// without touching the working tree: with no default-branch blob there is no
// possible equality, so the validated grant is withheld (GTS-FR-004) and the
// file hash need not be computed. When construction armed the per-page
// fallback (batch ls-tree failed, ref resolved), a map miss first attempts
// one bounded single-path ls-tree for the path — IP-FR-001's recovery — and
// withholds only when that also fails or the path is absent from the
// default-branch tree. A file read error degrades the file side to empty the
// same way — lookups never fail.
func (b *BlobResolver) PageBlobs(filePath string) (string, string) {
	refSHA, ok := b.blobs[filePath]
	if !ok {
		refSHA = b.fallbackRefSHA(filePath)
		if refSHA == "" {
			return "", ""
		}
	}
	fileSHA, err := FileBlobSHA(filepath.Join(b.repoDir, filePath))
	if err != nil {
		return "", refSHA
	}
	return fileSHA, refSHA
}

// fallbackRefSHA answers the default-branch blob object ID for one path via
// the per-page fallback, or "" when the resolver is unarmed. Results are
// memoized — including negative results — so each distinct path runs at most
// one git invocation per resolver.
func (b *BlobResolver) fallbackRefSHA(filePath string) string {
	if b.fallbackRunner == nil || b.fallbackFullRef == "" {
		return ""
	}
	b.fallbackMu.Lock()
	defer b.fallbackMu.Unlock()
	if sha, seen := b.fallbackMemo[filePath]; seen {
		return sha
	}
	sha := b.singlePathBlobSHA(filePath)
	b.fallbackMemo[filePath] = sha
	return sha
}

// singlePathBlobSHA resolves the default-branch blob object ID for exactly
// one path via `git ls-tree -z <fullRef> -- <path>` — the per-page recovery
// IP-FR-001 reserves for a failed batch invocation. The invocation follows
// the package contract: an argv array (never a shell string, per IP-FR-004),
// the fully qualified fallbackFullRef, a `--` separator before the path, and
// a bounded per-call timeout — the vault seam's TierBlobResolver carries no
// context, so the fallback derives its own background context rather than
// retaining the construction context. The record is parsed by the same shared
// helper as the batch output, and only the exact requested path's SHA is
// answered. An empty string means withheld: the invocation failed, its output
// was malformed, or the path is absent from the default-branch tree.
func (b *BlobResolver) singlePathBlobSHA(filePath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), invokeTimeout)
	defer cancel()

	args := []string{"ls-tree", "-z", b.fallbackFullRef, "--", filePath}
	out, _, code, err := b.fallbackRunner.Run(ctx, b.repoDir, args)
	if err != nil || code != 0 {
		return ""
	}
	blobs, ok := parseLsTreeBlobs(out)
	if !ok {
		return ""
	}
	return blobs[filePath]
}

// TrustPathspecs returns the git pathspecs covering the page families whose
// trust tier ingestion derives from default-branch blob equality (IP-FR-001):
// the compiled/ and knowledge/ namespaces. A fresh slice per call keeps the
// shared vocabulary unmutated (CS-007).
func TrustPathspecs() []string {
	return []string{"compiled", "knowledge"}
}

// NewPageBlobResolver runs the whole per-index-run derivation for the vault
// at opts.VaultDir: one Resolve through the IP-FR-004 precedence chain plus
// one batch ls-tree over paths (design D2 — one ls-tree per index run, not
// per page), returning a resolver that answers per-page lookups from the
// in-memory map.
//
// When the batch ls-tree invocation itself fails while the ref resolved,
// construction arms IP-FR-001's per-page fallback — "falling back to per-page
// `git cat-file`/`hash-object` only when batch is unavailable": each map miss
// then runs one bounded single-path ls-tree (memoized per path) instead of
// withholding the grant for every page in the run. A healthy map, a path the
// default-branch tree simply lacks, or an unresolved ref never triggers a
// per-page invocation.
//
// The result is usable even when the ref is unresolvable — every lookup then
// withholds the validated grant (GTS-FR-004), behaving exactly like a nil
// resolver at the vault seam — so callers never branch on resolution status.
func NewPageBlobResolver(ctx context.Context, opts Options, paths []string) *BlobResolver {
	r := New(opts)
	res := r.Resolve(ctx)
	mapped := MapDefaultBranchBlobsResult(ctx, r.runner, r.vaultDir, res, paths)
	if mapped.BatchFailed && res.Resolved() && res.FullRef != "" {
		return newFallbackBlobResolver(r.vaultDir, mapped.Blobs, r.runner, res.FullRef)
	}
	return NewBlobResolver(r.vaultDir, mapped.Blobs)
}
