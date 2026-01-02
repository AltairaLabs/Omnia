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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_AllFields(t *testing.T) {
	// Set all environment variables (t.Setenv handles cleanup)
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envPromptPackPath, "/custom/pack.json")
	t.Setenv(envPromptName, "assistant")
	t.Setenv(envSessionType, "redis")
	t.Setenv(envSessionURL, "redis://localhost:6379")
	t.Setenv(envSessionTTL, "2h")
	t.Setenv(envProviderType, "claude")
	t.Setenv(envProviderModel, "claude-3-opus-20240229")
	t.Setenv(envProviderBaseURL, "https://api.anthropic.com")
	t.Setenv(envToolsConfigPath, "/custom/tools.yaml")
	t.Setenv(envGRPCPort, "8000")
	t.Setenv(envHealthPort, "8001")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "test-agent", cfg.AgentName)
	assert.Equal(t, "test-ns", cfg.Namespace)
	assert.Equal(t, "/custom/pack.json", cfg.PromptPackPath)
	assert.Equal(t, "assistant", cfg.PromptName)
	assert.Equal(t, SessionTypeRedis, cfg.SessionType)
	assert.Equal(t, "redis://localhost:6379", cfg.SessionURL)
	assert.Equal(t, 2*time.Hour, cfg.SessionTTL)
	assert.Equal(t, ProviderTypeClaude, cfg.ProviderType)
	assert.Equal(t, "claude-3-opus-20240229", cfg.Model)
	assert.Equal(t, "https://api.anthropic.com", cfg.BaseURL)
	assert.Equal(t, "/custom/tools.yaml", cfg.ToolsConfigPath)
	assert.Equal(t, 8000, cfg.GRPCPort)
	assert.Equal(t, 8001, cfg.HealthPort)
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Set only required fields (t.Setenv handles cleanup)
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, defaultPromptPackPath, cfg.PromptPackPath)
	assert.Equal(t, defaultPromptName, cfg.PromptName)
	assert.Equal(t, defaultSessionType, cfg.SessionType)
	assert.Equal(t, defaultSessionTTL, cfg.SessionTTL)
	assert.Equal(t, defaultProviderType, cfg.ProviderType)
	assert.Empty(t, cfg.Model)
	assert.Empty(t, cfg.BaseURL)
	assert.Equal(t, defaultToolsConfigPath, cfg.ToolsConfigPath)
	assert.Equal(t, defaultGRPCPort, cfg.GRPCPort)
	assert.Equal(t, defaultHealthPort, cfg.HealthPort)
}

func TestLoadConfig_MissingAgentName(t *testing.T) {
	t.Setenv(envNamespace, "test-ns")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envAgentName)
}

func TestLoadConfig_MissingNamespace(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envNamespace)
}

func TestLoadConfig_InvalidSessionType(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envSessionType, "invalid")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestLoadConfig_RedisMissingURL(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envSessionType, "redis")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envSessionURL)
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envGRPCPort, "not-a-number")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envGRPCPort)
}

func TestLoadConfig_InvalidTTL(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envSessionTTL, "invalid")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envSessionTTL)
}

func TestLoadConfig_MockProvider(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envMockProvider, "true")
	t.Setenv(envMockConfigPath, "/etc/omnia/mock-config.yaml")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.True(t, cfg.MockProvider)
	assert.Equal(t, "/etc/omnia/mock-config.yaml", cfg.MockConfigPath)
}

func TestLoadConfig_MockProviderDisabled(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	// MockProvider not set

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.False(t, cfg.MockProvider)
	assert.Empty(t, cfg.MockConfigPath)
}

func TestLoadConfig_MockProviderInvalidValue(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envMockProvider, "yes") // Not "true", should be false

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.False(t, cfg.MockProvider, "MockProvider should only be true when value is exactly 'true'")
}

func TestLoadConfig_InvalidProviderType(t *testing.T) {
	t.Setenv(envAgentName, "test-agent")
	t.Setenv(envNamespace, "test-ns")
	t.Setenv(envProviderType, "invalid")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
	assert.Contains(t, err.Error(), envProviderType)
}

func TestLoadConfig_ProviderTypes(t *testing.T) {
	testCases := []struct {
		name         string
		providerType string
		expected     string
	}{
		{"auto", "auto", ProviderTypeAuto},
		{"claude", "claude", ProviderTypeClaude},
		{"openai", "openai", ProviderTypeOpenAI},
		{"gemini", "gemini", ProviderTypeGemini},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envAgentName, "test-agent")
			t.Setenv(envNamespace, "test-ns")
			t.Setenv(envProviderType, tc.providerType)

			cfg, err := LoadConfig()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cfg.ProviderType)
		})
	}
}
