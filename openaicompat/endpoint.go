package openaicompat

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Operation paths appended to a configured base URL. Exactly one is
// appended per request and any trailing operation path already present
// on the base URL is removed first, so the final URL never contains a
// doubled path (FR-010).
const (
	// PathEmbeddings is the embeddings operation path.
	PathEmbeddings = "/embeddings"
	// PathChatCompletions is the chat completions operation path.
	PathChatCompletions = "/chat/completions"
	// PathModels is the model-list operation path used by the probe.
	PathModels = "/models"
)

// operationPaths lists every operation path in the order used for
// suffix detection.
var operationPaths = []string{PathEmbeddings, PathChatCompletions, PathModels}

// NormalizeBaseURL validates a configured gateway endpoint and returns a
// normalized API base URL: a missing scheme becomes "http://", trailing
// slashes are removed, and the result never carries a query string,
// fragment, userinfo, or non-HTTP(S) scheme (research.md R4, wire
// contract Base URL).
//
// The field parameter names the configuration key (for example
// "embedding.endpoint") so validation errors stay actionable. An empty
// or whitespace-only endpoint is rejected here; providers normally catch
// it earlier and return the FR-009 field-naming factory error.
func NormalizeBaseURL(raw, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%s is empty", field)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", field, err)
	}
	if err := validateEndpointURL(parsed); err != nil {
		return "", fmt.Errorf("invalid %s %q: %w", field, raw, err)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

// validateEndpointURL enforces the scheme, host, query, fragment, and
// userinfo rules for a configured base URL.
func validateEndpointURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (must be http or https)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("query strings and fragments are not supported")
	}
	if u.User != nil {
		return fmt.Errorf("embedded credentials are not supported")
	}
	return nil
}

// OperationURL joins a normalized base URL with exactly one operation
// path. A trailing operation path already present on base is removed
// first, so passing "https://host/v1/embeddings" with PathEmbeddings
// yields "https://host/v1/embeddings" rather than a doubled path.
func OperationURL(base, operation string) string {
	trimmed := strings.TrimRight(base, "/")
	for _, op := range operationPaths {
		if strings.HasSuffix(trimmed, op) {
			trimmed = strings.TrimSuffix(trimmed, op)
			break
		}
	}
	return trimmed + operation
}

// EndpointHost returns the lowercased hostname of a configured endpoint
// with any port stripped. It is used for the exact-host OpenRouter key
// gate (FR-007). An unparseable endpoint returns "".
func EndpointHost(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// IsLoopbackHost reports whether a hostname resolves to the local
// machine for cleartext-credential warning purposes. "localhost" and any
// IP in 127.0.0.0/8 or ::1 are treated as loopback.
func IsLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// InsecureEndpointForCredential reports whether a resolved API key would
// transit cleartext HTTP to a non-loopback host. An empty key is never
// insecure, and an unparseable endpoint is left to the factory error
// path. FR-024 uses this to warn at startup and in dewey doctor.
func InsecureEndpointForCredential(endpoint, key string) bool {
	if key == "" {
		return false
	}
	base, err := NormalizeBaseURL(endpoint, "endpoint")
	if err != nil {
		return false
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" && !IsLoopbackHost(parsed.Hostname())
}
