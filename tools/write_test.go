package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/types"
)

func TestCreatePage_Success(t *testing.T) {
	mb := newMockBackend()
	mb.createPageResult = &types.PageEntity{UUID: "page-uuid-1", Name: "new-page"}
	w := NewWrite(mb)

	result, _, err := w.CreatePage(context.Background(), nil, types.CreatePageInput{
		Name:       "new-page",
		Properties: map[string]any{"type": "test"},
		Blocks:     []string{"block one", "block two"},
	})
	if err != nil {
		t.Fatalf("CreatePage() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CreatePage() returned error result")
	}

	// Verify page was created.
	if len(mb.createdPages) != 1 || mb.createdPages[0] != "new-page" {
		t.Errorf("expected page 'new-page' to be created, got %v", mb.createdPages)
	}

	// Verify blocks were appended.
	if len(mb.appendedBlocks) != 2 {
		t.Fatalf("expected 2 appended blocks, got %d", len(mb.appendedBlocks))
	}
	if mb.appendedBlocks[0].content != "block one" {
		t.Errorf("first block content = %q, want %q", mb.appendedBlocks[0].content, "block one")
	}

	// Verify result JSON.
	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["created"] != true {
		t.Errorf("created = %v, want true", parsed["created"])
	}
	if parsed["name"] != "new-page" {
		t.Errorf("name = %v, want %q", parsed["name"], "new-page")
	}
	if parsed["uuid"] != "page-uuid-1" {
		t.Errorf("uuid = %v, want %q", parsed["uuid"], "page-uuid-1")
	}
	if parsed["blocksAdded"] != float64(2) {
		t.Errorf("blocksAdded = %v, want 2", parsed["blocksAdded"])
	}
}

func TestCreatePage_Error(t *testing.T) {
	mb := newMockBackend()
	mb.createPageErr = fmt.Errorf("backend unavailable")
	w := NewWrite(mb)

	result, _, err := w.CreatePage(context.Background(), nil, types.CreatePageInput{
		Name: "fail-page",
	})
	if err != nil {
		t.Fatalf("CreatePage() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestCreatePage_BlockAppendError(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockErr = fmt.Errorf("block write failed")
	w := NewWrite(mb)

	result, _, err := w.CreatePage(context.Background(), nil, types.CreatePageInput{
		Name:   "page-with-blocks",
		Blocks: []string{"will fail"},
	})
	if err != nil {
		t.Fatalf("CreatePage() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when block append fails")
	}
}

func TestAppendBlocks_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.AppendBlocks(context.Background(), nil, types.AppendBlocksInput{
		Page:   "my-page",
		Blocks: []string{"first block", "second block"},
	})
	if err != nil {
		t.Fatalf("AppendBlocks() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("AppendBlocks() returned error result")
	}

	if len(mb.appendedBlocks) != 2 {
		t.Fatalf("expected 2 appended blocks, got %d", len(mb.appendedBlocks))
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["blocksCreated"] != float64(2) {
		t.Errorf("blocksCreated = %v, want 2", parsed["blocksCreated"])
	}
}

func TestAppendBlocks_EmptyBlocks(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.AppendBlocks(context.Background(), nil, types.AppendBlocksInput{
		Page:   "my-page",
		Blocks: []string{},
	})
	if err != nil {
		t.Fatalf("AppendBlocks() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when no blocks provided")
	}
}

func TestAppendBlocks_Error(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockErr = fmt.Errorf("write failed")
	w := NewWrite(mb)

	result, _, err := w.AppendBlocks(context.Background(), nil, types.AppendBlocksInput{
		Page:   "my-page",
		Blocks: []string{"will fail"},
	})
	if err != nil {
		t.Fatalf("AppendBlocks() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestUpsertBlocks_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.upsertBlocks(context.Background(), types.UpsertBlocksInput{
		Page: "target-page",
		Blocks: []types.BlockInput{
			{Content: "block A", Properties: map[string]string{"key": "val"}},
			{Content: "block B"},
		},
	})
	if err != nil {
		t.Fatalf("upsertBlocks() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("upsertBlocks() returned error result")
	}

	// Default position is append.
	if len(mb.appendedBlocks) != 2 {
		t.Fatalf("expected 2 appended blocks, got %d", len(mb.appendedBlocks))
	}

	// First block should have property appended to content.
	if mb.appendedBlocks[0].content != "block A\nkey:: val" {
		t.Errorf("first block content = %q, want %q", mb.appendedBlocks[0].content, "block A\nkey:: val")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["blocksCreated"] != float64(2) {
		t.Errorf("blocksCreated = %v, want 2", parsed["blocksCreated"])
	}
}

func TestUpsertBlocks_Prepend(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.upsertBlocks(context.Background(), types.UpsertBlocksInput{
		Page:     "target-page",
		Position: "prepend",
		Blocks: []types.BlockInput{
			{Content: "prepended block"},
		},
	})
	if err != nil {
		t.Fatalf("upsertBlocks() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("upsertBlocks() returned error result")
	}

	if len(mb.prependedBlocks) != 1 {
		t.Fatalf("expected 1 prepended block, got %d", len(mb.prependedBlocks))
	}
}

func TestUpdateBlock_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.UpdateBlock(context.Background(), nil, types.UpdateBlockInput{
		UUID:    "block-uuid-1",
		Content: "updated content",
	})
	if err != nil {
		t.Fatalf("UpdateBlock() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("UpdateBlock() returned error result")
	}

	if len(mb.updatedBlocks) != 1 {
		t.Fatalf("expected 1 updated block, got %d", len(mb.updatedBlocks))
	}
	if mb.updatedBlocks[0].uuid != "block-uuid-1" {
		t.Errorf("updated UUID = %q, want %q", mb.updatedBlocks[0].uuid, "block-uuid-1")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["updated"] != true {
		t.Errorf("updated = %v, want true", parsed["updated"])
	}
}

func TestUpdateBlock_Error(t *testing.T) {
	mb := newMockBackend()
	mb.updateBlockErr = fmt.Errorf("update failed")
	w := NewWrite(mb)

	result, _, err := w.UpdateBlock(context.Background(), nil, types.UpdateBlockInput{
		UUID:    "block-uuid-1",
		Content: "new content",
	})
	if err != nil {
		t.Fatalf("UpdateBlock() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestDeleteBlock_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.DeleteBlock(context.Background(), nil, types.DeleteBlockInput{
		UUID: "block-to-delete",
	})
	if err != nil {
		t.Fatalf("DeleteBlock() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DeleteBlock() returned error result")
	}

	if len(mb.removedBlocks) != 1 || mb.removedBlocks[0] != "block-to-delete" {
		t.Errorf("expected block 'block-to-delete' to be removed, got %v", mb.removedBlocks)
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["deleted"] != true {
		t.Errorf("deleted = %v, want true", parsed["deleted"])
	}
}

func TestDeleteBlock_Error(t *testing.T) {
	mb := newMockBackend()
	mb.removeBlockErr = fmt.Errorf("remove failed")
	w := NewWrite(mb)

	result, _, err := w.DeleteBlock(context.Background(), nil, types.DeleteBlockInput{
		UUID: "block-to-delete",
	})
	if err != nil {
		t.Fatalf("DeleteBlock() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestDeletePage_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.DeletePage(context.Background(), nil, types.DeletePageInput{
		Name: "page-to-delete",
	})
	if err != nil {
		t.Fatalf("DeletePage() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DeletePage() returned error result")
	}

	if len(mb.deletedPages) != 1 || mb.deletedPages[0] != "page-to-delete" {
		t.Errorf("expected page 'page-to-delete' to be deleted, got %v", mb.deletedPages)
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["deleted"] != true {
		t.Errorf("deleted = %v, want true", parsed["deleted"])
	}
	if parsed["name"] != "page-to-delete" {
		t.Errorf("name = %v, want %q", parsed["name"], "page-to-delete")
	}
}

func TestDeletePage_Error(t *testing.T) {
	mb := newMockBackend()
	mb.deletePageErr = fmt.Errorf("delete failed")
	w := NewWrite(mb)

	result, _, err := w.DeletePage(context.Background(), nil, types.DeletePageInput{
		Name: "page-to-delete",
	})
	if err != nil {
		t.Fatalf("DeletePage() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestRenamePage_Success(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.RenamePage(context.Background(), nil, types.RenamePageInput{
		OldName: "old-name",
		NewName: "new-name",
	})
	if err != nil {
		t.Fatalf("RenamePage() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("RenamePage() returned error result")
	}

	if len(mb.renamedPages) != 1 {
		t.Fatalf("expected 1 rename, got %d", len(mb.renamedPages))
	}
	if mb.renamedPages[0].old != "old-name" || mb.renamedPages[0].new != "new-name" {
		t.Errorf("renamed %q to %q, want %q to %q",
			mb.renamedPages[0].old, mb.renamedPages[0].new, "old-name", "new-name")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["renamed"] != true {
		t.Errorf("renamed = %v, want true", parsed["renamed"])
	}
}

func TestRenamePage_Error(t *testing.T) {
	mb := newMockBackend()
	mb.renamePageErr = fmt.Errorf("rename failed")
	w := NewWrite(mb)

	result, _, err := w.RenamePage(context.Background(), nil, types.RenamePageInput{
		OldName: "old-name",
		NewName: "new-name",
	})
	if err != nil {
		t.Fatalf("RenamePage() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestLinkPages_Success(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockResult = &types.BlockEntity{UUID: "link-block-uuid"}
	w := NewWrite(mb)

	result, _, err := w.LinkPages(context.Background(), nil, types.LinkPagesInput{
		From:    "page-a",
		To:      "page-b",
		Context: "related topic",
	})
	if err != nil {
		t.Fatalf("LinkPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("LinkPages() returned error result")
	}

	// Should create 2 blocks: one in each page.
	if len(mb.appendedBlocks) != 2 {
		t.Fatalf("expected 2 appended blocks, got %d", len(mb.appendedBlocks))
	}

	// First block: in page-a, linking to page-b with context.
	if mb.appendedBlocks[0].page != "page-a" {
		t.Errorf("first block page = %q, want %q", mb.appendedBlocks[0].page, "page-a")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["linked"] != true {
		t.Errorf("linked = %v, want true", parsed["linked"])
	}
}

func TestLinkPages_Error(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockErr = fmt.Errorf("append failed")
	w := NewWrite(mb)

	result, _, err := w.LinkPages(context.Background(), nil, types.LinkPagesInput{
		From: "page-a",
		To:   "page-b",
	})
	if err != nil {
		t.Fatalf("LinkPages() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestBulkUpdateProperties_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page1"}, types.BlockEntity{
		UUID:    "block-1",
		Content: "existing content",
	})
	mb.addPage(types.PageEntity{Name: "page2"}, types.BlockEntity{
		UUID:    "block-2",
		Content: "other content\nstatus:: draft",
	})
	w := NewWrite(mb)

	result, _, err := w.BulkUpdateProperties(context.Background(), nil, types.BulkUpdatePropertiesInput{
		Pages:    []string{"page1", "page2"},
		Property: "status",
		Value:    "reviewed",
	})
	if err != nil {
		t.Fatalf("BulkUpdateProperties() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkUpdateProperties() returned error result")
	}

	// Both pages should have been updated.
	if len(mb.updatedBlocks) != 2 {
		t.Fatalf("expected 2 updated blocks, got %d", len(mb.updatedBlocks))
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["updatedCount"] != float64(2) {
		t.Errorf("updatedCount = %v, want 2", parsed["updatedCount"])
	}
}

func TestBulkUpdateProperties_EmptyPages(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	result, _, err := w.BulkUpdateProperties(context.Background(), nil, types.BulkUpdatePropertiesInput{
		Pages:    []string{},
		Property: "status",
		Value:    "done",
	})
	if err != nil {
		t.Fatalf("BulkUpdateProperties() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when no pages specified")
	}
}

func TestBulkUpdateProperties_PartialFailure(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "good-page"}, types.BlockEntity{
		UUID:    "block-good",
		Content: "content",
	})
	// "bad-page" has no blocks, so the update will fail.
	mb.addPage(types.PageEntity{Name: "bad-page"})
	w := NewWrite(mb)

	result, _, err := w.BulkUpdateProperties(context.Background(), nil, types.BulkUpdatePropertiesInput{
		Pages:    []string{"good-page", "bad-page"},
		Property: "tag",
		Value:    "project",
	})
	if err != nil {
		t.Fatalf("BulkUpdateProperties() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkUpdateProperties() returned error result (should report partial)")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["updatedCount"] != float64(1) {
		t.Errorf("updatedCount = %v, want 1", parsed["updatedCount"])
	}
	if parsed["failedCount"] != float64(1) {
		t.Errorf("failedCount = %v, want 1", parsed["failedCount"])
	}
}

func TestInsertChildren_SingleChild(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	children := []types.BlockInput{
		{Content: "child one"},
	}

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", children)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	if len(uuids) != 1 {
		t.Fatalf("insertChildren() returned %d UUIDs, want 1", len(uuids))
	}

	// Verify InsertBlock was called with correct parent and content.
	if len(mb.insertedBlocks) != 1 {
		t.Fatalf("expected 1 InsertBlock call, got %d", len(mb.insertedBlocks))
	}
	if mb.insertedBlocks[0].parent != "parent-uuid" {
		t.Errorf("InsertBlock parent = %q, want %q", mb.insertedBlocks[0].parent, "parent-uuid")
	}
	if mb.insertedBlocks[0].content != "child one" {
		t.Errorf("InsertBlock content = %q, want %q", mb.insertedBlocks[0].content, "child one")
	}
}

func TestInsertChildren_MultipleChildren(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	children := []types.BlockInput{
		{Content: "first child"},
		{Content: "second child"},
		{Content: "third child"},
	}

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", children)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	if len(uuids) != 3 {
		t.Fatalf("insertChildren() returned %d UUIDs, want 3", len(uuids))
	}

	// Verify all three InsertBlock calls used the same parent.
	if len(mb.insertedBlocks) != 3 {
		t.Fatalf("expected 3 InsertBlock calls, got %d", len(mb.insertedBlocks))
	}
	for i, ib := range mb.insertedBlocks {
		if ib.parent != "parent-uuid" {
			t.Errorf("InsertBlock[%d] parent = %q, want %q", i, ib.parent, "parent-uuid")
		}
	}
	if mb.insertedBlocks[0].content != "first child" {
		t.Errorf("InsertBlock[0] content = %q, want %q", mb.insertedBlocks[0].content, "first child")
	}
	if mb.insertedBlocks[1].content != "second child" {
		t.Errorf("InsertBlock[1] content = %q, want %q", mb.insertedBlocks[1].content, "second child")
	}
	if mb.insertedBlocks[2].content != "third child" {
		t.Errorf("InsertBlock[2] content = %q, want %q", mb.insertedBlocks[2].content, "third child")
	}
}

func TestInsertChildren_EmptyChildren(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", nil)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	if len(uuids) != 0 {
		t.Errorf("insertChildren(nil) returned %d UUIDs, want 0", len(uuids))
	}
	if len(mb.insertedBlocks) != 0 {
		t.Errorf("expected 0 InsertBlock calls, got %d", len(mb.insertedBlocks))
	}
}

func TestInsertChildren_WithProperties(t *testing.T) {
	mb := newMockBackend()
	w := NewWrite(mb)

	children := []types.BlockInput{
		{
			Content:    "block with props",
			Properties: map[string]string{"type": "task"},
		},
	}

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", children)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	if len(uuids) != 1 {
		t.Fatalf("insertChildren() returned %d UUIDs, want 1", len(uuids))
	}

	// Verify properties were appended to content.
	if len(mb.insertedBlocks) != 1 {
		t.Fatalf("expected 1 InsertBlock call, got %d", len(mb.insertedBlocks))
	}
	content := mb.insertedBlocks[0].content
	if content != "block with props\ntype:: task" {
		t.Errorf("InsertBlock content = %q, want content with property appended", content)
	}
}

func TestInsertChildren_NestedChildren(t *testing.T) {
	// Use a mock that returns unique UUIDs per InsertBlock call
	// so we can verify parent-child relationships.
	mb := &mockBackendWithInsertCounter{
		mockBackend: newMockBackend(),
	}
	w := NewWrite(mb)

	children := []types.BlockInput{
		{
			Content: "parent block",
			Children: []types.BlockInput{
				{Content: "nested child"},
			},
		},
	}

	uuids, err := w.insertChildren(context.Background(), "root-uuid", children)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	// Should return 2 UUIDs: the parent block + its nested child.
	if len(uuids) != 2 {
		t.Fatalf("insertChildren() returned %d UUIDs, want 2", len(uuids))
	}

	// Verify two InsertBlock calls were made.
	if mb.callCount != 2 {
		t.Fatalf("expected 2 InsertBlock calls, got %d", mb.callCount)
	}

	// First call: parent block inserted under root-uuid.
	if mb.calls[0].parent != "root-uuid" {
		t.Errorf("InsertBlock[0] parent = %q, want %q", mb.calls[0].parent, "root-uuid")
	}
	if mb.calls[0].content != "parent block" {
		t.Errorf("InsertBlock[0] content = %q, want %q", mb.calls[0].content, "parent block")
	}

	// Second call: nested child inserted under the first block's UUID.
	if mb.calls[1].parent != "uuid-1" {
		t.Errorf("InsertBlock[1] parent = %q, want %q (UUID of first block)", mb.calls[1].parent, "uuid-1")
	}
	if mb.calls[1].content != "nested child" {
		t.Errorf("InsertBlock[1] content = %q, want %q", mb.calls[1].content, "nested child")
	}
}

func TestInsertChildren_InsertBlockError(t *testing.T) {
	mb := newMockBackend()
	mb.insertBlockErr = fmt.Errorf("backend insert failed")
	w := NewWrite(mb)

	children := []types.BlockInput{
		{Content: "will fail"},
	}

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", children)
	if err == nil {
		t.Fatal("insertChildren() expected error, got nil")
	}
	if err.Error() != "backend insert failed" {
		t.Errorf("error = %q, want %q", err.Error(), "backend insert failed")
	}
	if len(uuids) != 0 {
		t.Errorf("expected 0 UUIDs on error, got %d", len(uuids))
	}
}

func TestInsertChildren_NilInsertBlockResult(t *testing.T) {
	mb := newMockBackend()
	// Set insertBlockResult to simulate nil return (no block created).
	mb.insertBlockResult = nil
	// Override the default InsertBlock to return nil without error.
	w := NewWrite(&mockBackendNilInsert{mockBackend: mb})

	children := []types.BlockInput{
		{Content: "ghost block"},
	}

	uuids, err := w.insertChildren(context.Background(), "parent-uuid", children)
	if err != nil {
		t.Fatalf("insertChildren() error: %v", err)
	}

	// When InsertBlock returns nil, no UUID should be collected.
	if len(uuids) != 0 {
		t.Errorf("insertChildren() returned %d UUIDs, want 0 (nil insert result)", len(uuids))
	}
}

// mockBackendWithInsertCounter wraps mockBackend to return unique UUIDs
// for each InsertBlock call, enabling parent-child relationship verification.
type mockBackendWithInsertCounter struct {
	*mockBackend
	callCount int
	calls     []struct{ parent, content string }
}

func (m *mockBackendWithInsertCounter) InsertBlock(_ context.Context, srcBlock any, content string, opts map[string]any) (*types.BlockEntity, error) {
	m.callCount++
	m.calls = append(m.calls, struct{ parent, content string }{fmt.Sprint(srcBlock), content})
	uuid := fmt.Sprintf("uuid-%d", m.callCount)
	return &types.BlockEntity{UUID: uuid, Content: content}, nil
}

// mockBackendNilInsert wraps mockBackend but returns nil from InsertBlock
// without an error, simulating a backend that silently skips block creation.
type mockBackendNilInsert struct {
	*mockBackend
}

func (m *mockBackendNilInsert) InsertBlock(_ context.Context, _ any, _ string, _ map[string]any) (*types.BlockEntity, error) {
	return nil, nil
}

// extractText extracts the text string from a CallToolResult.
func extractText(t *testing.T, result interface{}) string {
	t.Helper()
	// Use reflection-free approach: marshal and re-parse.
	type mcpResult struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var r mcpResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal result wrapper: %v", err)
	}
	if len(r.Content) == 0 {
		t.Fatal("no content in result")
	}
	return r.Content[0].Text
}
