package types

import (
	"encoding/json"
	"fmt"
)

// PageEntity represents a Logseq page.
type PageEntity struct {
	ID              int            `json:"id"`
	UUID            string         `json:"uuid"`
	Name            string         `json:"name"`
	OriginalName    string         `json:"originalName"`
	Journal         bool           `json:"journal?"`
	JournalDay      int            `json:"journalDay,omitempty"`
	Namespace       *NamespaceInfo `json:"namespace,omitempty"`
	Properties      map[string]any `json:"properties,omitempty"`
	PropertiesOrder []string       `json:"propertiesOrder,omitempty"`
	CreatedAt       int64          `json:"createdAt,omitempty"`
	UpdatedAt       int64          `json:"updatedAt,omitempty"`
	File            *FileInfo      `json:"file,omitempty"`
}

// BlockEntity represents a Logseq block (the atomic unit of knowledge).
type BlockEntity struct {
	ID              int            `json:"id"`
	UUID            string         `json:"uuid"`
	Content         string         `json:"content"`
	Format          string         `json:"format,omitempty"`
	Marker          string         `json:"marker,omitempty"`   // TODO, DOING, DONE, etc.
	Priority        string         `json:"priority,omitempty"` // A, B, C
	Page            *PageRef       `json:"page,omitempty"`
	Left            *BlockRef      `json:"left,omitempty"`
	Parent          *BlockRef      `json:"parent,omitempty"`
	Children        []BlockEntity  `json:"children,omitempty"`
	Properties      map[string]any `json:"properties,omitempty"`
	PropertiesOrder []string       `json:"propertiesOrder,omitempty"`
	PathRefs        []PageRef      `json:"pathRefs,omitempty"`
	Refs            []PageRef      `json:"refs,omitempty"`
	PreBlock        bool           `json:"preBlock,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler for BlockEntity.
// It handles two Logseq JSON formats for the Children field:
//   - Full objects from getPageBlocksTree: [{"uuid":"...", "content":"...", ...}]
//   - Compact refs from getBlock: [["uuid", "value"]]
//
// When children are in compact ref format, they are silently discarded and the
// Children field is left empty. The receiver's fields are populated from the
// unmarshaled data; an error is returned if the JSON cannot be parsed.
func (b *BlockEntity) UnmarshalJSON(data []byte) error {
	type blockAlias BlockEntity
	type blockRaw struct {
		blockAlias
		RawChildren json.RawMessage `json:"children,omitempty"`
	}
	var raw blockRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*b = BlockEntity(raw.blockAlias)

	if len(raw.RawChildren) == 0 {
		return nil
	}

	// Try full BlockEntity array first.
	var children []BlockEntity
	if err := json.Unmarshal(raw.RawChildren, &children); err == nil {
		b.Children = children
		return nil
	}

	// Compact format [["uuid","value"], ...] — skip, leave children empty.
	return nil
}

// PageRef is a lightweight page reference (used in block refs and path refs).
type PageRef struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// BlockRef is a lightweight block reference.
type BlockRef struct {
	ID int `json:"id"`
}

// NamespaceInfo contains namespace hierarchy data.
type NamespaceInfo struct {
	ID int `json:"id"`
}

// FileInfo contains file path data for a page.
type FileInfo struct {
	ID   int    `json:"id"`
	Path string `json:"path,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler for PageRef.
// It handles both the number form (compact, from write operations) and the
// object form ({"id": N, "name": "..."}). The receiver's ID and Name fields
// are populated from the unmarshaled data; an error is returned if the JSON
// matches neither format.
func (p *PageRef) UnmarshalJSON(data []byte) error {
	// Try number first (compact form from write operations)
	var id int
	if err := json.Unmarshal(data, &id); err == nil {
		p.ID = id
		return nil
	}
	// Fall back to object form
	type pageRefAlias PageRef
	var alias pageRefAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("PageRef: expected number or object, got %s", string(data))
	}
	*p = PageRef(alias)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for BlockRef.
// It handles both the number form (compact) and the object form ({"id": N}).
// The receiver's ID field is populated from the unmarshaled data; an error
// is returned if the JSON matches neither format.
func (b *BlockRef) UnmarshalJSON(data []byte) error {
	var id int
	if err := json.Unmarshal(data, &id); err == nil {
		b.ID = id
		return nil
	}
	type blockRefAlias BlockRef
	var alias blockRefAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("BlockRef: expected number or object, got %s", string(data))
	}
	*b = BlockRef(alias)
	return nil
}

// ParsedContent holds structured data extracted from block content.
type ParsedContent struct {
	Raw             string            `json:"raw"`
	Links           []string          `json:"links"`                // [[page name]]
	BlockReferences []string          `json:"blockReferences"`      // ((uuid))
	Tags            []string          `json:"tags"`                 // #tag
	Properties      map[string]string `json:"properties,omitempty"` // key:: value
	Marker          string            `json:"marker,omitempty"`     // TODO, DOING, DONE
	Priority        string            `json:"priority,omitempty"`   // [#A], [#B], [#C]
}

// EnrichedBlock extends BlockEntity with parsed content and ancestor chain.
type EnrichedBlock struct {
	BlockEntity
	Parsed    ParsedContent  `json:"parsed"`
	Ancestors []BlockSummary `json:"ancestors,omitempty"` // path from root to this block
	Siblings  []BlockSummary `json:"siblings,omitempty"`
}

// BlockSummary is a lightweight block representation for context.
type BlockSummary struct {
	UUID    string `json:"uuid"`
	Content string `json:"content"`
}

// EnrichedPage extends PageEntity with its full block tree and link data.
type EnrichedPage struct {
	PageEntity
	Blocks        []EnrichedBlock `json:"blocks"`
	OutgoingLinks []string        `json:"outgoingLinks"` // pages this page links to
	BackLinks     []BackLink      `json:"backlinks"`     // pages that link to this page
	BlockCount    int             `json:"blockCount"`
	LinkCount     int             `json:"linkCount"`
}

// BackLink represents an incoming link from another page.
type BackLink struct {
	PageName string         `json:"pageName"`
	Blocks   []BlockSummary `json:"blocks"` // the specific blocks containing the link
}

// LogseqAPIRequest is the JSON body sent to the Logseq HTTP API.
type LogseqAPIRequest struct {
	Method string `json:"method"`
	Args   []any  `json:"args,omitempty"`
}
