//go:build livesmoke

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Live smoke test against the real OpenRouter endpoint. Gated behind the
// `livesmoke` build tag so CI never runs it (it costs credits and requires
// a real API key).
//
// Usage:
//
//	env OMNIA_OPENROUTER_API_KEY=sk-or-v1-... \
//	  go test ./internal/runtime/... -run TestOpenRouter_LiveSmoke \
//	  -tags=livesmoke -v -count=1

package runtime

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenRouter_LiveSmoke sends a single one-token completion to
// openrouter.ai using Omnia's runtime Server path. It proves the entire
// pipeline works against the real gateway: auth, attribution headers, and
// response parsing. Skips when OMNIA_OPENROUTER_API_KEY is unset.
func TestOpenRouter_LiveSmoke(t *testing.T) {
	apiKey := os.Getenv("OMNIA_OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OMNIA_OPENROUTER_API_KEY not set — skipping live smoke test")
	}
	// PromptKit's openai provider reads OPENAI_API_KEY (which the runtime
	// normally populates from the Provider CRD's secretRef). Mirror that here
	// so the real flow is exercised end-to-end.
	t.Setenv("OPENAI_API_KEY", apiKey)

	model := os.Getenv("OMNIA_OPENROUTER_MODEL")
	if model == "" {
		// Cheapest usable model for a one-token smoke test.
		model = "meta-llama/llama-3.1-8b-instruct"
	}

	s := NewServer(
		WithLogger(logr.Discard()),
		WithProviderInfo("openai", model),
		WithBaseURL("https://openrouter.ai/api/v1"),
		WithHeaders(map[string]string{
			"HTTP-Referer": "https://omnia.altairalabs.ai",
			"X-Title":      "omnia-livesmoke",
		}),
	)

	provider, err := s.createProviderFromConfig()
	require.NoError(t, err)
	require.NotNil(t, provider)

	resp, err := provider.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "system", Content: "You are a terse test bot. Reply with a single word."},
			{Role: "user", Content: "ping"},
		},
	})
	require.NoError(t, err, "live OpenRouter request failed — check API key, model ID, and account credits")

	t.Logf("openrouter response: %q", resp.Content)
	assert.NotEmpty(t, strings.TrimSpace(resp.Content), "expected a non-empty response from OpenRouter")
}
