package tools

import (
	"strings"
	"testing"
)

// TestPersistentStorageError_Message verifies the shared constructor's
// message contains every element required by the Degraded-Mode Safety spec:
// the tool name, the "requires persistent storage" statement, the --vault
// configuration hint, and the database-lock hint.
func TestPersistentStorageError_Message(t *testing.T) {
	err := persistentStorageError("promote")
	if err == nil {
		t.Fatal("persistentStorageError returned nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"promote requires persistent storage",
		"--vault",
		".uf/dewey/",
		"another Dewey process",
		"database lock",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message = %q, should contain %q", msg, want)
		}
	}
}

// TestPersistentStorageError_PerTool verifies the tool name is interpolated
// so each store-dependent tool's error remains distinguishable while sharing
// one message shape (CS-004).
func TestPersistentStorageError_PerTool(t *testing.T) {
	storeDependentTools := []string{
		"compile",
		"store_compiled",
		"store_learning",
		"promote",
		"lint",
		"curate",
		"index",
		"reindex",
	}
	for _, tool := range storeDependentTools {
		msg := persistentStorageError(tool).Error()
		if want := tool + " requires persistent storage"; !strings.HasPrefix(msg, want) {
			t.Errorf("persistentStorageError(%q) = %q, should start with %q", tool, msg, want)
		}
	}
}
