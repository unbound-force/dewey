package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/parser"
	"github.com/unbound-force/dewey/v3/sanitize"
	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// maxConcurrentIndexing is the maximum number of sources indexed concurrently.
// Balances I/O parallelism against SQLite write contention (D4, FR-102).
// Within each source, documents are processed sequentially to avoid write
// conflicts — the store uses SetMaxOpenConns(1) (D5).
const maxConcurrentIndexing = 4

// IndexResult captures the results of an IndexDocuments call.
type IndexResult struct {
	TotalIndexed    int
	TotalEmbeddings int
}

// IndexDocuments upserts fetched documents into the persistent store with full
// content persistence: blocks, links, and embeddings are parsed and stored
// alongside page metadata.
//
// Sources are processed concurrently using bounded workers (FR-102, D4).
// On the first persistence error, the context is cancelled and remaining
// goroutines stop. Within each source, documents are processed sequentially
// to avoid SQLite write contention (D5).
//
// This is the shared implementation used by both the CLI (dewey index/reindex)
// and MCP reindex tool, eliminating duplication (D6).
//
// An optional trailing TierBlobResolver supplies the git blob object IDs for
// trust-tier derivation on compiled/curated pages (IP-FR-001); it is resolved
// once per index run by the caller (design D2 — one ls-tree, not one per
// page). When absent or nil, blob SHAs stay empty and the validated grant is
// withheld (GTS-FR-004), exactly the pre-wiring behavior. At most the first
// resolver is used.
func IndexDocuments(s *store.Store, allDocs map[string][]source.Document, configs []source.SourceConfig, embedder embed.Embedder, maxChunkChars int, tierBlobs ...TierBlobResolver) (*IndexResult, error) {
	if maxChunkChars <= 0 {
		maxChunkChars = embed.DefaultMaxChunkChars
	}
	if len(allDocs) == 0 {
		return &IndexResult{}, nil
	}

	var resolver TierBlobResolver
	if len(tierBlobs) > 0 {
		resolver = tierBlobs[0]
	}

	var totalIndexed atomic.Int64
	var totalEmbeddings atomic.Int64

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrentIndexing)

	for sourceID, docs := range allDocs {
		g.Go(func() error {
			// Check for cancellation from a sibling goroutine's error.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			indexed, embeds, err := indexSourceDocuments(s, sourceID, docs, configs, embedder, maxChunkChars, resolver)
			if err != nil {
				return fmt.Errorf("index source %s: %w", sourceID, err)
			}

			totalIndexed.Add(int64(indexed))
			totalEmbeddings.Add(int64(embeds))

			// Update source record in the store.
			updateSourceRecord(s, sourceID, configs)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return &IndexResult{
			TotalIndexed:    int(totalIndexed.Load()),
			TotalEmbeddings: int(totalEmbeddings.Load()),
		}, err
	}

	return &IndexResult{
		TotalIndexed:    int(totalIndexed.Load()),
		TotalEmbeddings: int(totalEmbeddings.Load()),
	}, nil
}

// sourceIndexContext carries the per-source indexing decisions that every
// document in the source shares: the resolved source config, sanitize mode,
// trust tier, per-source sanitize statistics, and the per-run tier blob
// resolver (IP-FR-001). Bundling them keeps the per-document helper's
// signature stable as source-level decisions evolve.
type sourceIndexContext struct {
	sourceID     string
	srcCfg       *source.SourceConfig
	sanitizeMode string
	trustTier    string
	sourceStats  *sanitize.SourceStats
	tierBlobs    TierBlobResolver
}

// documentIndexOutcome reports what one document contributed to the index:
// whether it was skipped — rejected by strict sanitization, or dropped by
// the soft page-upsert failure policy — and its persisted block, link, and
// embedding counts.
type documentIndexOutcome struct {
	skipped bool
	blocks  int
	links   int
	embeds  int
}

// indexSourceDocuments processes all documents from a single source sequentially.
// Returns the number of documents indexed and embeddings generated.
// tierBlobs is the optional per-index-run blob resolver (IP-FR-001); nil
// withholds the validated grant (GTS-FR-004).
func indexSourceDocuments(s *store.Store, sourceID string, docs []source.Document, configs []source.SourceConfig, embedder embed.Embedder, maxChunkChars int, tierBlobs TierBlobResolver) (int, int, error) {
	start := time.Now()
	var blockCount, linkCount, embedCount, indexed int

	logger.Info("indexing source", "source", sourceID, "documents", len(docs))

	// Look up source config for sanitize_mode, trust_tier, and source type,
	// then resolve the per-source decisions every document shares.
	srcCfg := findSourceConfig(sourceID, configs)
	sanitizeMode := DetermineSanitizeMode(srcCfg)
	ctx := sourceIndexContext{
		sourceID:     sourceID,
		srcCfg:       srcCfg,
		sanitizeMode: sanitizeMode,
		trustTier:    DetermineTrustTier(srcCfg),
		sourceStats:  computeSourceStats(docs, sanitizeMode),
		tierBlobs:    tierBlobs,
	}

	for _, doc := range docs {
		// Namespace external page names: sourceID/docID.
		pageName := strings.ToLower(sourceID + "/" + doc.ID)
		logger.Debug("indexing document", "source", sourceID, "docID", doc.ID,
			"pageName", pageName, "contentHash", doc.ContentHash)

		outcome, err := indexSourceDocument(s, ctx, doc, pageName, embedder, maxChunkChars)
		if err != nil {
			return 0, 0, err
		}
		if outcome.skipped {
			continue
		}

		blockCount += outcome.blocks
		linkCount += outcome.links
		embedCount += outcome.embeds
		indexed++
	}

	elapsed := time.Since(start)
	logger.Info("source indexing complete",
		"source", sourceID,
		"documents", len(docs),
		"blocks", blockCount,
		"links", linkCount,
		"embeddings", embedCount,
		"elapsed", elapsed.Round(time.Millisecond),
	)

	return indexed, embedCount, nil
}

// computeSourceStats computes the per-source content-length statistics that
// size-anomaly detection consumes (D5 in 001-core-implementation), or nil
// when sanitization is off — no scanning layer runs, so no stats are needed.
func computeSourceStats(docs []source.Document, sanitizeMode string) *sanitize.SourceStats {
	if sanitizeMode == sanitize.ModeOff {
		return nil
	}
	lengths := make([]int, len(docs))
	for i, doc := range docs {
		lengths[i] = len(doc.Content)
	}
	stats := sanitize.ComputeStats(lengths)
	return &stats
}

// indexSourceDocument runs the full per-document indexing pipeline: content
// sanitization (with strict-mode rejection), parsing, sanitize-finding
// merge, trust-tier derivation, page upsert, and block, link, and embedding
// persistence. A skipped outcome means the document was rejected by strict
// sanitization or dropped by the soft page-upsert failure policy — the
// source keeps going either way. Block and link persistence failures are
// hard errors that abort the whole source.
func indexSourceDocument(s *store.Store, ctx sourceIndexContext, doc source.Document, pageName string, embedder embed.Embedder, maxChunkChars int) (documentIndexOutcome, error) {
	// Content sanitization: scan raw content BEFORE parsing.
	scanResult := scanSourceDocument(s, ctx, doc, pageName)
	if rejectedByStrictMode(ctx.sanitizeMode, scanResult) {
		logger.Error("document rejected by strict sanitization",
			"page", pageName,
			"source", ctx.sourceID,
			"findings", len(scanResult.Findings),
		)
		return documentIndexOutcome{skipped: true}, nil
	}

	// Parse document content into frontmatter and blocks.
	props, blocks := ParseDocument(pageName, doc.Content)
	logger.Debug("parsed document", "page", pageName, "blocks", len(blocks),
		"uuidSeed", pageName)

	// Merge sanitize findings into properties.
	props = mergeSanitizeFindings(props, scanResult)

	// Trust-tier policy: learning pages clamp to draft and compiled/
	// curated pages derive from frontmatter plus default-branch blob
	// equality through the optional resolver (IP-FR-001, IP-FR-002);
	// every other page family keeps the source-config trust tier.
	// Classification uses the document ID — the file's path within its
	// source root — because the stored page name carries a source-ID
	// prefix that would hide the family namespace. The same ID (with
	// its .md extension) keys the blob map for disk sources rooted at
	// the vault; other roots and non-disk sources miss the map, which
	// withholds the grant fail-safe.
	fileSHA, refSHA := resolveTierBlobs(ctx.tierBlobs, doc.ID)
	pageTier := ctx.trustTier
	if tier, known := ingestTier(doc.ID, props, fileSHA, refSHA); known {
		pageTier = tier
	}

	// Build properties JSON, then upsert the page record; a soft upsert
	// failure skips the document's remaining persistence steps.
	propsJSON := buildSourcePropsJSON(props, doc)
	if !upsertSourcePage(s, pageName, ctx.sourceID, doc, propsJSON, pageTier) {
		return documentIndexOutcome{skipped: true}, nil
	}

	// Persist blocks — block persistence failures are hard errors.
	if err := PersistBlocks(s, pageName, blocks, sql.NullString{}, 0); err != nil {
		return documentIndexOutcome{}, fmt.Errorf("persist blocks for %s: %w", pageName, err)
	}

	// Extract and persist links from blocks.
	if err := PersistLinks(s, pageName, blocks); err != nil {
		return documentIndexOutcome{}, fmt.Errorf("persist links for %s: %w", pageName, err)
	}

	outcome := documentIndexOutcome{
		blocks: countBlocks(blocks),
		links:  countLinks(blocks),
	}

	// Generate and persist embeddings if embedder is available.
	if embedder != nil && embedder.Available() {
		outcome.embeds = GenerateEmbeddings(s, embedder, pageName, blocks, nil, maxChunkChars)
	}
	return outcome, nil
}

// scanSourceDocument scans one document's raw content before parsing when
// the source enables sanitization, returning the scan result (nil when
// sanitization is off). Scan failures are soft: the error is logged and the
// returned result is still consulted by strict-mode rejection and finding
// merge, matching the pre-existing inline policy.
func scanSourceDocument(s *store.Store, ctx sourceIndexContext, doc source.Document, pageName string) *sanitize.ScanResult {
	if ctx.sanitizeMode == sanitize.ModeOff {
		return nil
	}

	var previousHash string
	existingForDrift, _ := s.GetPage(pageName)
	if existingForDrift != nil {
		previousHash = existingForDrift.ContentHash
	}

	scanInput := sanitize.ScanInput{
		Content:      doc.Content,
		SourceID:     ctx.sourceID,
		DocumentID:   doc.ID,
		SourceType:   ctx.srcCfg.Type,
		PreviousHash: previousHash,
		CurrentHash:  doc.ContentHash,
		SourceStats:  ctx.sourceStats,
	}
	scanCfg := sanitize.ScanConfig{
		Mode: ctx.sanitizeMode,
	}
	scanResult, scanErr := sanitize.Scan(scanInput, scanCfg)
	if scanErr != nil {
		logger.Warn("sanitize scan failed", "page", pageName, "err", scanErr)
	}
	return scanResult
}

// rejectedByStrictMode reports whether strict sanitization blocks the
// document: strict mode plus critical-or-high findings on the scan result.
func rejectedByStrictMode(sanitizeMode string, scanResult *sanitize.ScanResult) bool {
	return sanitizeMode == sanitize.ModeStrict && scanResult != nil &&
		sanitize.HasBlockingFindings(scanResult.Findings)
}

// mergeSanitizeFindings merges scan findings into the document's parsed
// properties under "sanitize_findings" so doctor and lint can surface them.
// The map is returned unchanged when there is nothing to merge; a nil map
// is initialized on demand.
func mergeSanitizeFindings(props map[string]any, scanResult *sanitize.ScanResult) map[string]any {
	if scanResult == nil || len(scanResult.Findings) == 0 {
		return props
	}
	if props == nil {
		props = make(map[string]any)
	}
	props["sanitize_findings"] = scanResult.Findings
	return props
}

// buildSourcePropsJSON serializes the properties stored with the page:
// parsed frontmatter properties win, falling back to the source document's
// own properties, and empty string when neither exists.
func buildSourcePropsJSON(props map[string]any, doc source.Document) string {
	if props != nil {
		data, _ := json.Marshal(props)
		return string(data)
	}
	if doc.Properties != nil {
		data, _ := json.Marshal(doc.Properties)
		return string(data)
	}
	return ""
}

// upsertSourcePage inserts or updates the page record for one document and
// reports whether the document should continue to block persistence.
// Re-indexing deletes the page's existing blocks and links first so
// re-parsed content fully replaces the previous version. Upsert failures
// are soft (logged, document skipped) — the pre-existing policy.
func upsertSourcePage(s *store.Store, pageName, sourceID string, doc source.Document, propsJSON, pageTier string) bool {
	existing, _ := s.GetPage(pageName)
	if existing != nil {
		// Re-index: delete existing blocks and links first.
		if err := s.DeleteBlocksByPage(pageName); err != nil {
			logger.Warn("failed to delete existing blocks for re-index",
				"page", pageName, "err", err)
		}
		if err := s.DeleteLinksByPage(pageName); err != nil {
			logger.Warn("failed to delete existing links for re-index",
				"page", pageName, "err", err)
		}

		existing.ContentHash = doc.ContentHash
		existing.SourceID = sourceID
		existing.SourceDocID = doc.ID
		existing.OriginalName = doc.Title
		existing.Properties = propsJSON
		existing.Tier = pageTier
		if err := s.UpdatePage(existing); err != nil {
			logger.Warn("failed to update page", "page", pageName, "err", err)
			return false
		}
		return true
	}

	page := &store.Page{
		Name:         pageName,
		OriginalName: doc.Title,
		SourceID:     sourceID,
		SourceDocID:  doc.ID,
		Properties:   propsJSON,
		ContentHash:  doc.ContentHash,
		CreatedAt:    doc.FetchedAt.UnixMilli(),
		UpdatedAt:    doc.FetchedAt.UnixMilli(),
		Tier:         pageTier,
	}
	if err := s.InsertPage(page); err != nil {
		logger.Warn("failed to insert page", "page", pageName, "err", err)
		return false
	}
	return true
}

// findSourceConfig returns the SourceConfig for a given source ID, or nil.
func findSourceConfig(sourceID string, configs []source.SourceConfig) *source.SourceConfig {
	for i := range configs {
		if configs[i].ID == sourceID {
			return &configs[i]
		}
	}
	return nil
}

// DetermineSanitizeMode extracts sanitize_mode from the source config map
// and delegates to sanitize.DetermineSanitizeMode for the core logic.
func DetermineSanitizeMode(cfg *source.SourceConfig) string {
	if cfg == nil {
		return sanitize.ModeOff
	}
	explicitMode := extractConfigString(cfg.Config, "sanitize_mode")
	return sanitize.DetermineSanitizeMode(cfg.Type, explicitMode)
}

// DetermineTrustTier extracts trust_tier from the source config map
// and delegates to sanitize.DetermineTrustTier for the core logic.
func DetermineTrustTier(cfg *source.SourceConfig) string {
	if cfg == nil {
		return "authored"
	}
	explicitTier := extractConfigString(cfg.Config, "trust_tier")
	return sanitize.DetermineTrustTier(explicitTier)
}

// extractConfigString safely extracts a string value from a config map.
// Returns empty string if the key is missing, nil, or not a string.
func extractConfigString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	v, ok := config[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// countBlocks returns the total number of blocks in a tree.
func countBlocks(blocks []types.BlockEntity) int {
	count := len(blocks)
	for _, b := range blocks {
		count += countBlocks(b.Children)
	}
	return count
}

// countLinks returns the total number of wikilinks in a block tree.
func countLinks(blocks []types.BlockEntity) int {
	count := 0
	for _, b := range blocks {
		parsed := parser.Parse(b.Content)
		count += len(parsed.Links)
		count += countLinks(b.Children)
	}
	return count
}

// updateSourceRecord creates or updates the source record in the store
// after indexing completes for a source.
func updateSourceRecord(s *store.Store, sourceID string, configs []source.SourceConfig) {
	existingSrc, _ := s.GetSource(sourceID)
	if existingSrc == nil {
		var srcType, srcName string
		for _, cfg := range configs {
			if cfg.ID == sourceID {
				srcType = cfg.Type
				srcName = cfg.Name
				break
			}
		}
		if err := s.InsertSource(&store.SourceRecord{
			ID:            sourceID,
			Type:          srcType,
			Name:          srcName,
			Status:        "active",
			LastFetchedAt: time.Now().UnixMilli(),
		}); err != nil {
			logger.Warn("failed to insert source record", "source", sourceID, "err", err)
		}
	} else {
		if err := s.UpdateLastFetched(sourceID, time.Now().UnixMilli()); err != nil {
			logger.Warn("failed to update source last fetched", "source", sourceID, "err", err)
		}
		if err := s.UpdateSourceStatus(sourceID, "active", ""); err != nil {
			logger.Warn("failed to update source status", "source", sourceID, "err", err)
		}
	}
}
