package tools

import "fmt"

// persistentStorageError returns the shared error reported by store-dependent
// MCP tools when they are invoked without a persistent store. A nil store
// means the server is running in degraded, in-memory-only mode for one of two
// reasons: either no --vault with a .uf/dewey/ directory was configured, or
// the store could not be opened because another Dewey process (such as a
// running dewey serve) holds the database lock.
//
// The tool parameter is the MCP tool name shown to the calling agent (e.g.
// "promote", "store_compiled"). One message shape is shared across every
// store-dependent tool (compile, store_compiled, store_learning, promote,
// lint, curate, index, reindex) so agents receive a consistent, actionable
// diagnosis regardless of which write tool they called (CS-004: identical
// message text in more than two locations must be a shared helper).
//
// Callers surface it via errorResult(persistentStorageError("tool").Error()).
func persistentStorageError(tool string) error {
	return fmt.Errorf("%s requires persistent storage. Configure --vault with a .uf/dewey/ directory. "+
		"If --vault is already set, another Dewey process (such as a running dewey serve) may hold the database lock", tool)
}
