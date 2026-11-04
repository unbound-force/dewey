package openaicompat

import (
	"fmt"
	"os"
	"time"
)

// Capability identifies which Dewey subsystem a Client serves. It scopes
// the API-key variable and the endpoint configuration field, keeping the
// embedding credential chain explicit (FR-007).
type Capability string

const (
	// CapabilityEmbedding serves dewey index and semantic search.
	CapabilityEmbedding Capability = "embedding"
)

// String returns the capability name used in error text.
func (c Capability) String() string { return string(c) }

// valid reports whether the capability is the supported value.
func (c Capability) valid() bool {
	return c == CapabilityEmbedding
}

// endpointField returns the configuration key named by factory and
// validation errors (FR-009).
func (c Capability) endpointField() string {
	return "embedding.endpoint"
}

// scopedKeyVar returns the capability-scoped API-key environment variable
// name.
func (c Capability) scopedKeyVar() string {
	return "DEWEY_EMBEDDING_API_KEY"
}

// defaultTimeout returns the FR-025 client timeout for the capability.
func (c Capability) defaultTimeout() time.Duration {
	return embeddingTimeout
}

// ResolveAPIKey resolves a gateway API key for the capability and
// endpoint using the FR-007 chain: the capability-scoped variable, then
// DEWEY_API_KEY, then OPENROUTER_API_KEY only when the endpoint host is
// exactly openrouter.ai (port stripped, case-insensitive, no subdomain
// match). Empty values count as unset, and no configuration-file key is
// ever read.
//
// The resolved key is transient: it is never logged, persisted, or
// written to an artifact.
func ResolveAPIKey(capability Capability, endpoint string) string {
	if key := os.Getenv(capability.scopedKeyVar()); key != "" {
		return key
	}
	if key := os.Getenv("DEWEY_API_KEY"); key != "" {
		return key
	}
	if EndpointHost(endpoint) == "openrouter.ai" {
		if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
			return key
		}
	}
	return ""
}

// AuthRequiredReason returns the FR-008 message for an endpoint that
// rejected an unauthenticated request. It names the capability-scoped
// variable and the shared fallback and never suggests storing the key in
// configuration.
func AuthRequiredReason(capability Capability) string {
	return fmt.Sprintf(
		"openai-compatible %s endpoint requires authentication and no API key is available; set %s or DEWEY_API_KEY",
		capability, capability.scopedKeyVar(),
	)
}

// VerifyKeyReason returns the message for an endpoint that rejected a
// request carrying a resolved key. It never includes credential material.
func VerifyKeyReason(capability Capability, status int) string {
	return fmt.Sprintf(
		"openai-compatible %s endpoint rejected the API key (HTTP %d); verify the key",
		capability, status,
	)
}
