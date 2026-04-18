/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const openAIChatCompletionResponse = `{"id":"x","object":"chat.completion","created":1,` +
	`"model":"anthropic/claude-sonnet-4","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

// TestOpenRouter_CustomHeadersReachTheWire proves that custom headers set via
// Omnia's WithHeaders option (the path cmd/runtime/main.go uses when wiring
// cfg.Headers from a Provider CRD) are present on the HTTP request when the
// runtime calls its LLM provider.
//
// This is the Omnia-side end-to-end for the #909 headers feature: PromptKit's
// own custom_headers_test.go covers the SDK layer. This test covers the
// Omnia wrapper layer (Server → buildProviderSpec → providers.CreateProviderFromSpec),
// which is the surface a user of the Provider CRD actually depends on.
func TestOpenRouter_CustomHeadersReachTheWire(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, openAIChatCompletionResponse)
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "sk-or-v1-test-openrouter-key")

	s := NewServer(
		WithLogger(logr.Discard()),
		WithProviderInfo("openai", "anthropic/claude-sonnet-4"),
		WithBaseURL(server.URL),
		WithHeaders(map[string]string{
			"HTTP-Referer": "https://my-app.example",
			"X-Title":      "omnia",
		}),
	)

	provider, err := s.createProviderFromConfig()
	require.NoError(t, err)
	require.NotNil(t, provider)

	_, err = provider.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)

	assert.Equal(t, "https://my-app.example", receivedHeaders.Get("HTTP-Referer"),
		"HTTP-Referer header should reach the LLM endpoint when set via WithHeaders")
	assert.Equal(t, "omnia", receivedHeaders.Get("X-Title"),
		"X-Title header should reach the LLM endpoint when set via WithHeaders")
	// Authorization is set by the provider itself — confirm custom headers did
	// not overwrite it.
	assert.NotEmpty(t, receivedHeaders.Get("Authorization"),
		"built-in Authorization header must still be present alongside custom headers")
}

// TestOpenRouter_HeaderCollisionRejected verifies that a custom header that
// collides with a provider-controlled header is rejected at request time
// rather than silently breaking authentication. This mirrors PromptKit's
// collision behavior and is the behavior the how-to doc tells users about.
func TestOpenRouter_HeaderCollisionRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("request should not have been sent; Authorization collision must be rejected before send")
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "sk-or-v1-test-key")

	s := NewServer(
		WithLogger(logr.Discard()),
		WithProviderInfo("openai", "anthropic/claude-sonnet-4"),
		WithBaseURL(server.URL),
		WithHeaders(map[string]string{
			"Authorization": "Bearer conflict",
		}),
	)

	provider, err := s.createProviderFromConfig()
	require.NoError(t, err)
	require.NotNil(t, provider)

	_, err = provider.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err, "colliding custom header must fail the request")
}
