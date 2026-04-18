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
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/credentials"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
)

// httpTimeoutSetter is satisfied by providers whose BaseProvider exposes
// SetHTTPTimeout (available in PromptKit since the request_timeout feature).
// Applied via type assertion so Omnia builds against both current and older
// PromptKit versions.
type httpTimeoutSetter interface {
	SetHTTPTimeout(time.Duration)
}

// streamIdleTimeoutSetter is satisfied by providers whose BaseProvider
// exposes SetStreamIdleTimeout. Not yet in the published PromptKit SDK;
// this assertion is forward-compatible — it silently does nothing today
// and will start taking effect once PromptKit publishes the setter.
// TODO: drop the type assertion once SetStreamIdleTimeout is released.
type streamIdleTimeoutSetter interface {
	SetStreamIdleTimeout(time.Duration)
}

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
// Returns nil, nil if provider type is empty (no provider configured).
func (s *Server) createProviderFromConfig() (providers.Provider, error) {
	// Skip if no explicit provider type
	if s.providerType == "" {
		return nil, nil
	}

	spec := s.buildProviderSpec()

	// Resolve platform credential lazily at provider-creation time (not during
	// spec assembly) so tests that exercise spec wiring don't need cloud
	// credentials in their environment.
	if spec.Platform != "" {
		cred, err := credentials.Resolve(context.Background(), credentials.ResolverConfig{
			ProviderType:   s.providerType,
			PlatformConfig: spec.PlatformConfig,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve platform credential: %w", err)
		}
		spec.Credential = cred
	}

	s.log.Info("creating provider from config",
		"type", s.providerType,
		"model", spec.Model,
		"baseURL", s.baseURL,
		"platform", s.platformType,
		"authType", s.authType,
		"hasHeaders", len(s.headers) > 0,
		"requestTimeout", s.providerRequestTimeout,
		"streamIdleTimeout", s.providerStreamIdleTimeout)

	provider, err := providers.CreateProviderFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider from spec: %w", err)
	}

	s.applyProviderTimeouts(provider)
	return provider, nil
}

// buildProviderSpec assembles the PromptKit ProviderSpec from Server fields.
// It fills Platform/PlatformConfig/Headers/Pricing and auto-maps claude
// release names to Bedrock model IDs when bedrock hosting is used. It does
// NOT resolve credentials — createProviderFromConfig does that separately so
// unit tests can exercise spec wiring without cloud credentials.
func (s *Server) buildProviderSpec() providers.ProviderSpec {
	spec := providers.ProviderSpec{
		ID:      s.providerType,
		Type:    s.providerType,
		Model:   s.model,
		BaseURL: s.baseURL,
		Headers: s.headers,
	}

	if s.inputCostPer1K > 0 && s.outputCostPer1K > 0 {
		spec.Defaults.Pricing = providers.Pricing{
			InputCostPer1K:  s.inputCostPer1K,
			OutputCostPer1K: s.outputCostPer1K,
		}
	}

	if s.platformType == "" {
		return spec
	}

	spec.Platform = s.platformType
	spec.PlatformConfig = &providers.PlatformConfig{
		Type:     s.platformType,
		Region:   s.platformRegion,
		Project:  s.platformProject,
		Endpoint: s.platformEndpoint,
	}

	// Auto-map claude release names to Bedrock model IDs.
	if s.platformType == "bedrock" && s.model != "" {
		if bedrockID, ok := credentials.BedrockModelMapping[s.model]; ok {
			spec.Model = bedrockID
		}
	}

	return spec
}

// applyProviderTimeouts sets HTTP and stream-idle timeouts via the setter
// interfaces exposed by BaseProvider, without requiring those fields on the
// published ProviderSpec.
func (s *Server) applyProviderTimeouts(provider providers.Provider) {
	if s.providerRequestTimeout > 0 {
		if p, ok := provider.(httpTimeoutSetter); ok {
			p.SetHTTPTimeout(s.providerRequestTimeout)
		}
	}
	if s.providerStreamIdleTimeout > 0 {
		if p, ok := provider.(streamIdleTimeoutSetter); ok {
			p.SetStreamIdleTimeout(s.providerStreamIdleTimeout)
		}
	}
}
