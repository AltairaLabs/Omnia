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
		// Use file-based mock repository
		repo, err := mock.NewFileMockRepository(s.mockConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load mock config: %w", err)
		}
		return mock.NewToolProviderWithRepository("mock", "mock-model", false, repo), nil
	}
	// Use in-memory mock provider with default responses
	return mock.NewToolProvider("mock", "mock-model", false, nil), nil
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
