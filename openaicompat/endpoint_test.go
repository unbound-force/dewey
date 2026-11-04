package openaicompat

import (
	"strings"
	"testing"
)

func TestNormalizeBaseURL_Validation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "https base", raw: "https://gateway.example/v1", want: "https://gateway.example/v1"},
		{name: "missing scheme", raw: "gateway.example/v1", want: "http://gateway.example/v1"},
		{name: "trailing slash removed", raw: "https://gateway.example/v1/", want: "https://gateway.example/v1"},
		{name: "surrounding whitespace", raw: "  https://gateway.example/v1  ", want: "https://gateway.example/v1"},
		{name: "empty", raw: "", wantErr: true},
		{name: "whitespace only", raw: "   ", wantErr: true},
		{name: "file scheme", raw: "file:///tmp/endpoint", wantErr: true},
		{name: "ftp scheme", raw: "ftp://gateway.example/v1", wantErr: true},
		{name: "query string", raw: "https://gateway.example/v1?key=value", wantErr: true},
		{name: "fragment", raw: "https://gateway.example/v1#frag", wantErr: true},
		{name: "embedded userinfo", raw: "https://user:pass@gateway.example/v1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.raw, "embedding.endpoint")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				if !strings.Contains(err.Error(), "embedding.endpoint") {
					t.Fatalf("error must name the field, got %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOperationURL_SingleOperationPath(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		operation string
		want      string
	}{
		{name: "embeddings", base: "https://host/v1", operation: PathEmbeddings, want: "https://host/v1/embeddings"},
		{name: "models", base: "https://host/v1", operation: PathModels, want: "https://host/v1/models"},
		{name: "no double append embeddings", base: "https://host/v1/embeddings", operation: PathEmbeddings, want: "https://host/v1/embeddings"},
		{name: "operation switch", base: "https://host/v1/embeddings", operation: PathModels, want: "https://host/v1/models"},
		{name: "trailing slash", base: "https://host/v1/", operation: PathEmbeddings, want: "https://host/v1/embeddings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OperationURL(tt.base, tt.operation); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEndpointHost_OpenRouterHostGate(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{endpoint: "https://openrouter.ai/api/v1", want: "openrouter.ai"},
		{endpoint: "https://OpenRouter.AI:443/api/v1", want: "openrouter.ai"},
		{endpoint: "openrouter.ai/api/v1", want: "openrouter.ai"},
		{endpoint: "https://api.openrouter.ai/v1", want: "api.openrouter.ai"},
		{endpoint: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := EndpointHost(tt.endpoint); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsLoopbackHost_LocalAddresses(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "LOCALHOST", want: true},
		{host: "127.0.0.1", want: true},
		{host: "127.10.20.30", want: true},
		{host: "::1", want: true},
		{host: "gateway.example", want: false},
		{host: "10.0.0.5", want: false},
		{host: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsLoopbackHost(tt.host); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInsecureEndpointForCredential_CleartextKey(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		key      string
		want     bool
	}{
		{name: "no key is never insecure", endpoint: "http://gateway.example/v1", key: "", want: false},
		{name: "https non-loopback", endpoint: "https://gateway.example/v1", key: "sk", want: false},
		{name: "http loopback", endpoint: "http://127.0.0.1:8080/v1", key: "sk", want: false},
		{name: "http localhost", endpoint: "http://localhost:8080/v1", key: "sk", want: false},
		{name: "http non-loopback", endpoint: "http://gateway.example/v1", key: "sk", want: true},
		{name: "missing scheme normalizes to http", endpoint: "gateway.example/v1", key: "sk", want: true},
		{name: "invalid endpoint is left to factory errors", endpoint: "ftp://gateway.example/v1", key: "sk", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InsecureEndpointForCredential(tt.endpoint, tt.key); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNormalizeBaseURL_UserinfoNotEchoed pins the credential-hygiene rule
// for validation errors: rejecting an endpoint with embedded userinfo
// must not echo the userinfo back through the error text (code review
// M-1).
func TestNormalizeBaseURL_UserinfoNotEchoed(t *testing.T) {
	_, err := NormalizeBaseURL("https://user:pass@gateway.example/v1", "embedding.endpoint")
	if err == nil {
		t.Fatal("expected error for embedded credentials")
	}
	if strings.Contains(err.Error(), "user:pass") {
		t.Fatalf("error must not echo embedded credentials: %q", err)
	}
	if !strings.Contains(err.Error(), "https://gateway.example/v1") {
		t.Fatalf("error should name the sanitized endpoint: %q", err)
	}
}
