package tools

// Note: The health MCP tool is defined inline in server.go (main package),
// not in tools/navigate.go. The Dewey-specific health fields are tested
// via the server integration in server_test.go. This file exists per the
// task specification (T042B) but the actual health tool test is in the
// main package where the tool is defined.
//
// This file tests the Navigate tool helpers that are used by the health tool.

import "testing"

// TestNavigateNewNavigate verifies Navigate constructor.
func TestNavigateNewNavigate(t *testing.T) {
	nav := NewNavigate(nil)
	if nav == nil {
		t.Fatal("NewNavigate(nil) returned nil")
	}
}
