/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/altairalabs/omnia/ee/pkg/arena/overrides"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestApplyToolOverrides(t *testing.T) {
	t.Run("empty overrides is no-op", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
metadata:
  name: get_weather
spec:
  name: get_weather
  description: Get weather data
  mode: mock
  mock_result: "sunny"
`),
				},
			},
		}

		err := applyToolOverrides(cfg, map[string]ToolOverrideConfig{}, false)
		require.NoError(t, err)

		// Verify tool is unchanged
		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)
		assert.Equal(t, "mock", wrapper.Spec.Mode)
	})

	t.Run("nil overrides is no-op", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
metadata:
  name: get_weather
spec:
  name: get_weather
  mode: mock
`),
				},
			},
		}

		err := applyToolOverrides(cfg, nil, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)
		assert.Equal(t, "mock", wrapper.Spec.Mode)
	})

	t.Run("applies override to matching tool by spec.name", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
metadata:
  name: weather-tool
spec:
  name: get_weather
  description: Original description
  mode: mock
  mock_result: "sunny"
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"get_weather": {
				Name:         "get_weather",
				Endpoint:     "https://api.weather.example.com/v1/weather",
				HandlerName:  "weather-handler",
				RegistryName: "production-tools",
				HandlerType:  "http",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		assert.Equal(t, "http", wrapper.Spec.Mode)
		assert.NotNil(t, wrapper.Spec.HTTP)
		assert.Equal(t, "https://api.weather.example.com/v1/weather", wrapper.Spec.HTTP.URL)
		assert.Equal(t, "POST", wrapper.Spec.HTTP.Method)
		// Original description should be preserved when override doesn't provide one
		assert.Equal(t, "Original description", wrapper.Spec.Description)
	})

	t.Run("applies override to matching tool by metadata.name when spec.name is empty", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/calculate.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
metadata:
  name: calculate
spec:
  description: Do calculations
  mode: mock
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"calculate": {
				Name:         "calculate",
				Endpoint:     "https://api.calc.example.com/compute",
				HandlerName:  "calc-handler",
				RegistryName: "math-tools",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		assert.Equal(t, "http", wrapper.Spec.Mode)
		assert.Equal(t, "https://api.calc.example.com/compute", wrapper.Spec.HTTP.URL)
	})

	t.Run("override with description updates tool description", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/search.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: search
  description: Mock search
  mode: mock
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"search": {
				Name:         "search",
				Description:  "Real production search API",
				Endpoint:     "https://api.search.example.com/v1/search",
				HandlerName:  "search-handler",
				RegistryName: "search-tools",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		assert.Equal(t, "Real production search API", wrapper.Spec.Description)
	})

	t.Run("preserves existing HTTP config when applying override", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/api.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: external_api
  mode: http
  http:
    url: http://old-endpoint:8080/api
    method: GET
    headers:
      Authorization: Bearer old-token
    timeout_ms: 5000
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"external_api": {
				Name:         "external_api",
				Endpoint:     "https://new-endpoint:443/api",
				HandlerName:  "api-handler",
				RegistryName: "api-tools",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		assert.Equal(t, "http", wrapper.Spec.Mode)
		assert.Equal(t, "https://new-endpoint:443/api", wrapper.Spec.HTTP.URL)
		// Existing method should be preserved (not overwritten to POST)
		assert.Equal(t, "GET", wrapper.Spec.HTTP.Method)
		// Headers should be preserved
		assert.Equal(t, "Bearer old-token", wrapper.Spec.HTTP.Headers["Authorization"])
		// Timeout should be preserved
		assert.Equal(t, 5000, wrapper.Spec.HTTP.TimeoutMs)
	})

	t.Run("tool without matching override is unchanged", func(t *testing.T) {
		originalYAML := `apiVersion: promptkit/v1
kind: Tool
spec:
  name: unrelated_tool
  description: Some other tool
  mode: mock
  mock_result: "result"
`
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/unrelated.tool.yaml",
					Data:     []byte(originalYAML),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"different_tool": {
				Name:     "different_tool",
				Endpoint: "https://api.example.com",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		assert.Equal(t, "mock", wrapper.Spec.Mode)
		assert.Nil(t, wrapper.Spec.HTTP)
	})

	t.Run("multiple tools with some overrides", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: get_weather
  mode: mock
`),
				},
				{
					FilePath: "tools/calculate.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: calculate
  mode: mock
`),
				},
				{
					FilePath: "tools/search.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: search
  mode: mock
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"get_weather": {
				Name:     "get_weather",
				Endpoint: "https://weather.api.example.com",
			},
			"search": {
				Name:     "search",
				Endpoint: "https://search.api.example.com",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		// Weather tool should be overridden
		var weatherWrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &weatherWrapper)
		require.NoError(t, err)
		assert.Equal(t, "http", weatherWrapper.Spec.Mode)
		assert.Equal(t, "https://weather.api.example.com", weatherWrapper.Spec.HTTP.URL)

		// Calculate tool should NOT be overridden
		var calcWrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[1].Data, &calcWrapper)
		require.NoError(t, err)
		assert.Equal(t, "mock", calcWrapper.Spec.Mode)
		assert.Nil(t, calcWrapper.Spec.HTTP)

		// Search tool should be overridden
		var searchWrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[2].Data, &searchWrapper)
		require.NoError(t, err)
		assert.Equal(t, "http", searchWrapper.Spec.Mode)
		assert.Equal(t, "https://search.api.example.com", searchWrapper.Spec.HTTP.URL)
	})

	t.Run("invalid YAML tool is skipped", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/invalid.tool.yaml",
					Data:     []byte(`this is not valid yaml: [`),
				},
				{
					FilePath: "tools/valid.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: valid_tool
  mode: mock
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"valid_tool": {
				Name:     "valid_tool",
				Endpoint: "https://api.example.com",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		// Invalid tool should still be invalid (unchanged)
		assert.Equal(t, []byte(`this is not valid yaml: [`), cfg.LoadedTools[0].Data)

		// Valid tool should be overridden
		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[1].Data, &wrapper)
		require.NoError(t, err)
		assert.Equal(t, "http", wrapper.Spec.Mode)
	})

	t.Run("preserves input_schema and other fields", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
metadata:
  name: get_weather
spec:
  name: get_weather
  description: Get current weather
  input_schema:
    type: object
    properties:
      city:
        type: string
        description: City name
    required:
      - city
  output_schema:
    type: object
    properties:
      temperature:
        type: number
  mode: mock
  timeout_ms: 10000
  mock_result:
    temperature: 72
    conditions: sunny
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"get_weather": {
				Name:     "get_weather",
				Endpoint: "https://weather.api.example.com/v1",
			},
		}

		err := applyToolOverrides(cfg, overrides, false)
		require.NoError(t, err)

		var wrapper toolConfigWrapper
		err = yaml.Unmarshal(cfg.LoadedTools[0].Data, &wrapper)
		require.NoError(t, err)

		// Check mode and endpoint changed
		assert.Equal(t, "http", wrapper.Spec.Mode)
		assert.Equal(t, "https://weather.api.example.com/v1", wrapper.Spec.HTTP.URL)

		// Check input_schema preserved
		assert.NotNil(t, wrapper.Spec.InputSchema)
		assert.Equal(t, "object", wrapper.Spec.InputSchema["type"])
		props := wrapper.Spec.InputSchema["properties"].(map[string]interface{})
		assert.NotNil(t, props["city"])

		// Check output_schema preserved
		assert.NotNil(t, wrapper.Spec.OutputSchema)

		// Check timeout_ms preserved
		assert.Equal(t, 10000, wrapper.Spec.TimeoutMs)

		// Check description preserved
		assert.Equal(t, "Get current weather", wrapper.Spec.Description)
	})

	t.Run("verbose mode logs applied overrides", func(t *testing.T) {
		cfg := &config.Config{
			LoadedTools: []config.ToolData{
				{
					FilePath: "tools/weather.tool.yaml",
					Data: []byte(`apiVersion: promptkit/v1
kind: Tool
spec:
  name: get_weather
  mode: mock
`),
				},
			},
		}

		overrides := map[string]ToolOverrideConfig{
			"get_weather": {
				Name:         "get_weather",
				Endpoint:     "https://api.example.com",
				HandlerName:  "weather-handler",
				RegistryName: "prod-tools",
			},
		}

		// Just verify it doesn't error with verbose mode
		err := applyToolOverrides(cfg, overrides, true)
		require.NoError(t, err)
	})
}

func TestToolOverrideConfigParsing(t *testing.T) {
	t.Run("parses complete override config", func(t *testing.T) {
		override := ToolOverrideConfig{
			Name:         "get_weather",
			Description:  "Real weather API",
			Endpoint:     "https://api.weather.example.com/v1",
			HandlerName:  "weather-handler",
			RegistryName: "production-tools",
			HandlerType:  "http",
		}

		assert.Equal(t, "get_weather", override.Name)
		assert.Equal(t, "Real weather API", override.Description)
		assert.Equal(t, "https://api.weather.example.com/v1", override.Endpoint)
		assert.Equal(t, "weather-handler", override.HandlerName)
		assert.Equal(t, "production-tools", override.RegistryName)
		assert.Equal(t, "http", override.HandlerType)
	})

	t.Run("override with minimal fields", func(t *testing.T) {
		override := ToolOverrideConfig{
			Name:     "search",
			Endpoint: "https://api.search.com",
		}

		assert.Equal(t, "search", override.Name)
		assert.Equal(t, "https://api.search.com", override.Endpoint)
		assert.Empty(t, override.Description)
		assert.Empty(t, override.HandlerName)
		assert.Empty(t, override.RegistryName)
	})
}

func TestApplyProviderOverrides(t *testing.T) {
	t.Run("override with SecretEnvVar sets CredentialEnv", func(t *testing.T) {
		cfg := &config.Config{}
		providersByGroup := map[string][]overrides.ProviderOverride{
			"default": {
				{
					ID:           "test-openai",
					Type:         "openai",
					Model:        "gpt-4",
					SecretEnvVar: "OPENAI_API_KEY",
				},
			},
		}

		applyProviderOverrides(cfg, providersByGroup, false)

		require.NotNil(t, cfg.LoadedProviders)
		require.Contains(t, cfg.LoadedProviders, "test-openai")
		provider := cfg.LoadedProviders["test-openai"]
		require.NotNil(t, provider.Credential)
		assert.Equal(t, "OPENAI_API_KEY", provider.Credential.CredentialEnv)
		assert.Empty(t, provider.Credential.CredentialFile)
	})

	t.Run("override with CredentialFile sets CredentialFile", func(t *testing.T) {
		cfg := &config.Config{}
		providersByGroup := map[string][]overrides.ProviderOverride{
			"default": {
				{
					ID:             "test-file-provider",
					Type:           "openai",
					Model:          "gpt-4",
					CredentialFile: "/secrets/openai/api-key",
				},
			},
		}

		applyProviderOverrides(cfg, providersByGroup, false)

		require.NotNil(t, cfg.LoadedProviders)
		require.Contains(t, cfg.LoadedProviders, "test-file-provider")
		provider := cfg.LoadedProviders["test-file-provider"]
		require.NotNil(t, provider.Credential)
		assert.Equal(t, "/secrets/openai/api-key", provider.Credential.CredentialFile)
		assert.Empty(t, provider.Credential.CredentialEnv)
	})

	t.Run("override with neither sets no credential", func(t *testing.T) {
		cfg := &config.Config{}
		providersByGroup := map[string][]overrides.ProviderOverride{
			"default": {
				{
					ID:    "test-mock",
					Type:  "mock",
					Model: "mock-model",
				},
			},
		}

		applyProviderOverrides(cfg, providersByGroup, false)

		require.NotNil(t, cfg.LoadedProviders)
		require.Contains(t, cfg.LoadedProviders, "test-mock")
		provider := cfg.LoadedProviders["test-mock"]
		assert.Nil(t, provider.Credential)
	})

	t.Run("SecretEnvVar takes precedence over CredentialFile", func(t *testing.T) {
		cfg := &config.Config{}
		providersByGroup := map[string][]overrides.ProviderOverride{
			"default": {
				{
					ID:             "test-both",
					Type:           "openai",
					Model:          "gpt-4",
					SecretEnvVar:   "OPENAI_API_KEY",
					CredentialFile: "/secrets/openai/api-key",
				},
			},
		}

		applyProviderOverrides(cfg, providersByGroup, false)

		provider := cfg.LoadedProviders["test-both"]
		require.NotNil(t, provider.Credential)
		assert.Equal(t, "OPENAI_API_KEY", provider.Credential.CredentialEnv)
		assert.Empty(t, provider.Credential.CredentialFile)
	})

	t.Run("sets provider group mapping", func(t *testing.T) {
		cfg := &config.Config{}
		providersByGroup := map[string][]overrides.ProviderOverride{
			"judge": {
				{
					ID:    "judge-claude",
					Type:  "claude",
					Model: "claude-3-opus",
				},
			},
		}

		applyProviderOverrides(cfg, providersByGroup, false)

		require.NotNil(t, cfg.ProviderGroups)
		assert.Equal(t, "judge", cfg.ProviderGroups["judge-claude"])
	})
}

func TestLoadConfigFleet(t *testing.T) {
	t.Run("loads fleet config from env", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/tmp/content")
		t.Setenv("ARENA_EXECUTION_MODE", "fleet")
		t.Setenv("ARENA_FLEET_WS_URL", "ws://agent.default.svc:8080/ws")

		cfg, err := loadConfig()
		require.NoError(t, err)
		assert.Equal(t, "fleet", cfg.ExecutionMode)
		assert.Equal(t, "ws://agent.default.svc:8080/ws", cfg.FleetWSURL)
	})

	t.Run("defaults to direct mode", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/tmp/content")

		cfg, err := loadConfig()
		require.NoError(t, err)
		assert.Equal(t, "direct", cfg.ExecutionMode)
		assert.Empty(t, cfg.FleetWSURL)
	})

	t.Run("requires fleet URL in fleet mode", func(t *testing.T) {
		t.Setenv("ARENA_JOB_NAME", "test-job")
		t.Setenv("ARENA_CONTENT_PATH", "/tmp/content")
		t.Setenv("ARENA_EXECUTION_MODE", "fleet")

		_, err := loadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ARENA_FLEET_WS_URL is required")
	})
}
