package openaicompat

import (
	"strings"
	"testing"
)

// clearKeyEnv blanks every credential variable so a test starts from a
// known state.
func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"DEWEY_EMBEDDING_API_KEY",
		"DEWEY_SYNTHESIS_API_KEY",
		"DEWEY_API_KEY",
		"OPENROUTER_API_KEY",
	} {
		t.Setenv(name, "")
	}
}

func TestResolveAPIKey_Precedence(t *testing.T) {
	tests := []struct {
		name       string
		capability Capability
		endpoint   string
		env        map[string]string
		want       string
	}{
		{
			name:       "capability scoped wins",
			capability: CapabilityEmbedding,
			endpoint:   "https://gateway.example/v1",
			env: map[string]string{
				"DEWEY_EMBEDDING_API_KEY": "scoped",
				"DEWEY_API_KEY":           "shared",
			},
			want: "scoped",
		},
		{
			name:       "shared key is second",
			capability: CapabilitySynthesis,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{"DEWEY_API_KEY": "shared"},
			want:       "shared",
		},
		{
			name:       "embedding does not read synthesis scoped key",
			capability: CapabilityEmbedding,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{"DEWEY_SYNTHESIS_API_KEY": "synth"},
			want:       "",
		},
		{
			name:       "synthesis does not read embedding scoped key",
			capability: CapabilitySynthesis,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{"DEWEY_EMBEDDING_API_KEY": "embed"},
			want:       "",
		},
		{
			name:       "openrouter exact host",
			capability: CapabilitySynthesis,
			endpoint:   "https://openrouter.ai/api/v1",
			env:        map[string]string{"OPENROUTER_API_KEY": "router"},
			want:       "router",
		},
		{
			name:       "openrouter host with port and mixed case",
			capability: CapabilitySynthesis,
			endpoint:   "https://OpenRouter.AI:443/api/v1",
			env:        map[string]string{"OPENROUTER_API_KEY": "router"},
			want:       "router",
		},
		{
			name:       "openrouter subdomain does not match",
			capability: CapabilitySynthesis,
			endpoint:   "https://api.openrouter.ai/api/v1",
			env:        map[string]string{"OPENROUTER_API_KEY": "router"},
			want:       "",
		},
		{
			name:       "openrouter key not used on other hosts",
			capability: CapabilitySynthesis,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{"OPENROUTER_API_KEY": "router"},
			want:       "",
		},
		{
			name:       "empty value counts as unset",
			capability: CapabilityEmbedding,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{"DEWEY_EMBEDDING_API_KEY": "", "DEWEY_API_KEY": "shared"},
			want:       "shared",
		},
		{
			name:       "no key resolves to empty string",
			capability: CapabilityEmbedding,
			endpoint:   "https://gateway.example/v1",
			env:        map[string]string{},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearKeyEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := ResolveAPIKey(tt.capability, tt.endpoint); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthReasons_EnvVarNaming(t *testing.T) {
	embedding := AuthRequiredReason(CapabilityEmbedding)
	if !strings.Contains(embedding, "DEWEY_EMBEDDING_API_KEY") || !strings.Contains(embedding, "DEWEY_API_KEY") {
		t.Fatalf("embedding auth reason must name both variables: %q", embedding)
	}
	if strings.Contains(embedding, "config.yaml") {
		t.Fatalf("auth reason must not suggest storing the key in config: %q", embedding)
	}

	synthesis := AuthRequiredReason(CapabilitySynthesis)
	if !strings.Contains(synthesis, "DEWEY_SYNTHESIS_API_KEY") {
		t.Fatalf("synthesis auth reason must name its scoped variable: %q", synthesis)
	}

	verify := VerifyKeyReason(CapabilitySynthesis, 401)
	if !strings.Contains(verify, "verify the key") || !strings.Contains(verify, "401") {
		t.Fatalf("verify-key reason malformed: %q", verify)
	}
}

func TestCapabilityFields_Names(t *testing.T) {
	if got := CapabilityEmbedding.endpointField(); got != "embedding.endpoint" {
		t.Fatalf("got %q", got)
	}
	if got := CapabilitySynthesis.endpointField(); got != "synthesis.endpoint" {
		t.Fatalf("got %q", got)
	}
	if got := CapabilityEmbedding.scopedKeyVar(); got != "DEWEY_EMBEDDING_API_KEY" {
		t.Fatalf("got %q", got)
	}
	if got := CapabilitySynthesis.scopedKeyVar(); got != "DEWEY_SYNTHESIS_API_KEY" {
		t.Fatalf("got %q", got)
	}
}
