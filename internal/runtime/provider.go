/*
Copyright 2025.

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
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
)

// createMockProvider creates a mock provider based on configuration.
// Returns a ToolProvider (not the basic Provider) so that PredictWithTools
// is available — without this, the ProviderStage falls back to plain Predict
// and mock tool_calls from the config are never emitted. See #734.
func (s *Server) createMockProvider() (*mock.ToolProvider, error) {
	if s.mockConfigPath != "" {
		repo, err := mock.NewFileMockRepository(s.mockConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load mock config: %w", err)
		}
		// Wrap the repo so that the Omnia runtime's default scenario ("default")
		// is used when the PromptKit mock provider receives an empty ScenarioID.
		// Without this, GetTurn rejects empty IDs and falls through to the
		// text-only defaultResponse, bypassing tool_calls turns. See #734.
		wrapped := &defaultScenarioRepo{inner: repo}
		return mock.NewToolProviderWithRepository("mock", "mock-model", false, wrapped), nil
	}
	return mock.NewToolProvider("mock", "mock-model", false, nil), nil
}

// defaultScenarioRepo wraps a ResponseRepository and substitutes "default"
// for empty ScenarioIDs. This bridges the gap between Omnia's scenario
// handling (which returns ScenarioDefault="default" but doesn't inject it
// into the SDK's request metadata) and PromptKit's mock provider (which
// treats empty ScenarioID as "no scenario" and skips scenario turns).
type defaultScenarioRepo struct {
	inner mock.ResponseRepository
}

func (r *defaultScenarioRepo) GetResponse(
	ctx context.Context, params mock.ResponseParams,
) (string, error) {
	if params.ScenarioID == "" {
		params.ScenarioID = ScenarioDefault
	}
	return r.inner.GetResponse(ctx, params)
}

func (r *defaultScenarioRepo) GetTurn(
	ctx context.Context, params mock.ResponseParams,
) (*mock.Turn, error) {
	if params.ScenarioID == "" {
		params.ScenarioID = ScenarioDefault
	}
	return r.inner.GetTurn(ctx, params)
}

// createProviderFromConfig creates a PromptKit provider based on runtime configuration.
// This is used for explicit provider types (ollama, claude, openai, gemini).
// Returns nil, nil if provider type is empty (no provider configured).
func (s *Server) createProviderFromConfig() (providers.Provider, error) {
	// Skip if no explicit provider type
	if s.providerType == "" {
		return nil, nil
	}

	// Create provider from spec
	spec := providers.ProviderSpec{
		ID:      s.providerType,
		Type:    s.providerType,
		Model:   s.model,
		BaseURL: s.baseURL,
	}

	// Pass CRD pricing to PromptKit so providers use it for cost calculation
	if s.inputCostPer1K > 0 && s.outputCostPer1K > 0 {
		spec.Defaults.Pricing = providers.Pricing{
			InputCostPer1K:  s.inputCostPer1K,
			OutputCostPer1K: s.outputCostPer1K,
		}
	}

	s.log.Info("creating explicit provider from config",
		"type", s.providerType,
		"model", s.model,
		"baseURL", s.baseURL)

	provider, err := providers.CreateProviderFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider from spec: %w", err)
	}

	return provider, nil
}
