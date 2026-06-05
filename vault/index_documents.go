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

	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/parser"
	"github.com/unbound-force/dewey/sanitize"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
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
func IndexDocuments(s *store.Store, allDocs map[string][]source.Document, configs []source.SourceConfig, embedder embed.Embedder) (*IndexResult, error) {
	if len(allDocs) == 0 {
		return &IndexResult{}, nil
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

			indexed, embeds, err := indexSourceDocuments(s, sourceID, docs, configs, embedder)
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

// indexSourceDocuments processes all documents from a single source sequentially.
// Returns the number of documents indexed and embeddings generated.
func indexSourceDocuments(s *store.Store, sourceID string, docs []source.Document, configs []source.SourceConfig, embedder embed.Embedder) (int, int, error) {
	start := time.Now()
	var blockCount, linkCount, embedCount, indexed int

	logger.Info("indexing source", "source", sourceID, "documents", len(docs))

	// Look up source config for sanitize_mode, trust_tier, and source type.
	srcCfg := findSourceConfig(sourceID, configs)

	// Determine sanitize_mode and trust_tier from source config.
	sanitizeMode := DetermineSanitizeMode(srcCfg)
	trustTier := DetermineTrustTier(srcCfg)

	// Compute per-source SourceStats from document content lengths before
	// per-document scanning (D5 in 001-core-implementation).
	var sourceStats *sanitize.SourceStats
	if sanitizeMode != sanitize.ModeOff {
		lengths := make([]int, len(docs))
		for i, doc := range docs {
			lengths[i] = len(doc.Content)
		}
		stats := sanitize.ComputeStats(lengths)
		sourceStats = &stats
	}

	for _, doc := range docs {
		// Namespace external page names: sourceID/docID.
		pageName := strings.ToLower(sourceID + "/" + doc.ID)
		logger.Debug("indexing document", "source", sourceID, "docID", doc.ID,
			"pageName", pageName, "contentHash", doc.ContentHash)

		// Content sanitization: scan raw content BEFORE parsing.
		var scanResult *sanitize.ScanResult
		if sanitizeMode != sanitize.ModeOff {
			var previousHash string
			existingForDrift, _ := s.GetPage(pageName)
			if existingForDrift != nil {
				previousHash = existingForDrift.ContentHash
			}

			scanInput := sanitize.ScanInput{
				Content:      doc.Content,
				SourceID:     sourceID,
				DocumentID:   doc.ID,
				SourceType:   srcCfg.Type,
				PreviousHash: previousHash,
				CurrentHash:  doc.ContentHash,
				SourceStats:  sourceStats,
			}
			scanCfg := sanitize.ScanConfig{
				Mode: sanitizeMode,
			}
			var scanErr error
			scanResult, scanErr = sanitize.Scan(scanInput, scanCfg)
			if scanErr != nil {
				logger.Warn("sanitize scan failed", "page", pageName, "err", scanErr)
			}

			// Strict mode: skip documents with critical or high severity findings.
			if sanitizeMode == sanitize.ModeStrict && scanResult != nil {
				if sanitize.HasBlockingFindings(scanResult.Findings) {
					logger.Error("document rejected by strict sanitization",
						"page", pageName,
						"source", sourceID,
						"findings", len(scanResult.Findings),
					)
					continue
				}
			}
		}

		// Parse document content into frontmatter and blocks.
		props, blocks := ParseDocument(pageName, doc.Content)
		logger.Debug("parsed document", "page", pageName, "blocks", len(blocks),
			"uuidSeed", pageName)

		// Merge sanitize findings into properties.
		if scanResult != nil && len(scanResult.Findings) > 0 {
			if props == nil {
				props = make(map[string]any)
			}
			props["sanitize_findings"] = scanResult.Findings
		}

		// Build properties JSON.
		propsJSON := ""
		if props != nil {
			data, _ := json.Marshal(props)
			propsJSON = string(data)
		} else if doc.Properties != nil {
			data, _ := json.Marshal(doc.Properties)
			propsJSON = string(data)
		}

		// Upsert page record.
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
			existing.Tier = trustTier
			if err := s.UpdatePage(existing); err != nil {
				logger.Warn("failed to update page", "page", pageName, "err", err)
				continue
			}
		} else {
			page := &store.Page{
				Name:         pageName,
				OriginalName: doc.Title,
				SourceID:     sourceID,
				SourceDocID:  doc.ID,
				Properties:   propsJSON,
				ContentHash:  doc.ContentHash,
				CreatedAt:    doc.FetchedAt.UnixMilli(),
				UpdatedAt:    doc.FetchedAt.UnixMilli(),
				Tier:         trustTier,
			}
			if err := s.InsertPage(page); err != nil {
				logger.Warn("failed to insert page", "page", pageName, "err", err)
				continue
			}
		}

		// Persist blocks — block persistence failures are hard errors.
		if err := PersistBlocks(s, pageName, blocks, sql.NullString{}, 0); err != nil {
			return 0, 0, fmt.Errorf("persist blocks for %s: %w", pageName, err)
		}
		blockCount += countBlocks(blocks)

		// Extract and persist links from blocks.
		if err := PersistLinks(s, pageName, blocks); err != nil {
			return 0, 0, fmt.Errorf("persist links for %s: %w", pageName, err)
		}
		linkCount += countLinks(blocks)

		// Generate and persist embeddings if embedder is available.
		if embedder != nil && embedder.Available() {
			ec := GenerateEmbeddings(s, embedder, pageName, blocks, nil)
			embedCount += ec
		}

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
