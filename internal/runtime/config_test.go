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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		EnvAgentName, EnvNamespace, EnvSessionType, EnvSessionURL,
		EnvSessionTTL, EnvProviderAPIKey, EnvProviderType, EnvPromptPackPath,
		EnvToolsConfigPath, EnvGRPCPort, EnvHealthPort,
	}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, val)
			}
		}
	}()

	// Clear environment
	for _, key := range envVars {
		_ = os.Unsetenv(key)
	}

	tests := []struct {
		name    string
		envVars map[string]string
		want    *Config
		wantErr error
	}{
		{
			name: "minimal valid config",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvProviderAPIKey: "sk-test-key",
			},
			want: &Config{
				AgentName:       "test-agent",
				Namespace:       "default",
				SessionType:     SessionTypeMemory,
				ProviderType:    ProviderTypeOpenAI,
				ProviderAPIKey:  "sk-test-key",
				PromptPackPath:  DefaultPromptPackPath,
				ToolsConfigPath: DefaultToolsConfigPath,
				GRPCPort:        DefaultGRPCPort,
				HealthPort:      DefaultHealthPort,
				SessionTTL:      DefaultSessionTTL,
			},
		},
		{
			name: "full config with redis",
			envVars: map[string]string{
				EnvAgentName:       "my-agent",
				EnvNamespace:       "production",
				EnvSessionType:     "redis",
				EnvSessionURL:      "redis://localhost:6379",
				EnvSessionTTL:      "1h",
				EnvProviderAPIKey:  "sk-prod-key",
				EnvProviderType:    "openai",
				EnvPromptPackPath:  "/custom/packs",
				EnvToolsConfigPath: "/custom/tools.yaml",
				EnvGRPCPort:        "9090",
				EnvHealthPort:      "9091",
			},
			want: &Config{
				AgentName:       "my-agent",
				Namespace:       "production",
				SessionType:     SessionTypeRedis,
				SessionURL:      "redis://localhost:6379",
				SessionTTL:      time.Hour,
				ProviderType:    ProviderTypeOpenAI,
				ProviderAPIKey:  "sk-prod-key",
				PromptPackPath:  "/custom/packs",
				ToolsConfigPath: "/custom/tools.yaml",
				GRPCPort:        9090,
				HealthPort:      9091,
			},
		},
		{
			name:    "missing agent name",
			envVars: map[string]string{},
			wantErr: ErrMissingAgentName,
		},
		{
			name: "missing namespace",
			envVars: map[string]string{
				EnvAgentName: "test-agent",
			},
			wantErr: ErrMissingNamespace,
		},
		{
			name: "missing provider key",
			envVars: map[string]string{
				EnvAgentName: "test-agent",
				EnvNamespace: "default",
			},
			wantErr: ErrMissingProviderKey,
		},
		{
			name: "invalid session type",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvProviderAPIKey: "sk-test-key",
				EnvSessionType:    "postgres",
			},
			wantErr: ErrInvalidSessionType,
		},
		{
			name: "redis without URL",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvProviderAPIKey: "sk-test-key",
				EnvSessionType:    "redis",
			},
			wantErr: ErrMissingSessionURL,
		},
		{
			name: "invalid provider type",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvProviderAPIKey: "sk-test-key",
				EnvProviderType:   "gemini",
			},
			wantErr: ErrInvalidProviderType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment
			for _, key := range envVars {
				_ = os.Unsetenv(key)
			}
			for key, val := range tt.envVars {
				_ = os.Setenv(key, val)
			}

			got, err := LoadConfig()

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.AgentName, got.AgentName)
			assert.Equal(t, tt.want.Namespace, got.Namespace)
			assert.Equal(t, tt.want.SessionType, got.SessionType)
			assert.Equal(t, tt.want.SessionURL, got.SessionURL)
			assert.Equal(t, tt.want.SessionTTL, got.SessionTTL)
			assert.Equal(t, tt.want.ProviderType, got.ProviderType)
			assert.Equal(t, tt.want.GRPCPort, got.GRPCPort)
			assert.Equal(t, tt.want.HealthPort, got.HealthPort)
		})
	}
}

func TestLoadConfig_InvalidPorts(t *testing.T) {
	// Save and restore environment
	envVars := []string{EnvAgentName, EnvNamespace, EnvProviderAPIKey, EnvGRPCPort, EnvHealthPort}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, val)
			}
		}
	}()

	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{"invalid grpc port", EnvGRPCPort, "not-a-number"},
		{"invalid health port", EnvHealthPort, "not-a-number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set valid base config
			_ = os.Setenv(EnvAgentName, "test")
			_ = os.Setenv(EnvNamespace, "default")
			_ = os.Setenv(EnvProviderAPIKey, "sk-test")
			_ = os.Setenv(tt.envVar, tt.value)
			defer func() { _ = os.Unsetenv(tt.envVar) }()

			_, err := LoadConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestLoadConfig_InvalidSessionTTL(t *testing.T) {
	// Save and restore environment
	saved := os.Getenv(EnvSessionTTL)
	defer func() {
		if saved == "" {
			_ = os.Unsetenv(EnvSessionTTL)
		} else {
			_ = os.Setenv(EnvSessionTTL, saved)
		}
	}()

	_ = os.Setenv(EnvAgentName, "test")
	_ = os.Setenv(EnvNamespace, "default")
	_ = os.Setenv(EnvProviderAPIKey, "sk-test")
	_ = os.Setenv(EnvSessionTTL, "invalid")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid OMNIA_SESSION_TTL")
}
