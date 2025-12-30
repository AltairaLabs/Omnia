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

package agent

import (
	"os"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	// Save original env and restore after test
	origEnv := map[string]string{}
	for _, key := range []string{
		EnvAgentName, EnvNamespace, EnvPromptPackName, EnvPromptPackVersion,
		EnvFacadeType, EnvFacadePort, EnvHandlerMode, EnvProviderAPIKey, EnvToolRegistryName,
		EnvToolRegistryNS, EnvSessionType, EnvSessionTTL, EnvSessionStoreURL,
		EnvPromptPackMountPath, EnvHealthPort,
	} {
		origEnv[key] = os.Getenv(key)
	}
	defer func() {
		for key, value := range origEnv {
			if value == "" {
				_ = os.Unsetenv(key) // Ignore error in test cleanup
			} else {
				_ = os.Setenv(key, value) // Ignore error in test cleanup
			}
		}
	}()

	// Clear all env vars
	for key := range origEnv {
		_ = os.Unsetenv(key) // Ignore error in test setup
	}

	tests := []struct {
		name    string
		envVars map[string]string
		want    *Config
		wantErr bool
	}{
		{
			name: "minimal valid config",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvPromptPackName: "my-pack",
				EnvProviderAPIKey: "sk-test-key",
			},
			want: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				ProviderAPIKey: "sk-test-key",
				PromptPackPath: DefaultPromptPackMountPath,
				FacadeType:     FacadeTypeWebSocket,
				FacadePort:     DefaultFacadePort,
				HealthPort:     DefaultHealthPort,
				HandlerMode:    HandlerModeRuntime,
				SessionType:    SessionTypeMemory,
				SessionTTL:     DefaultSessionTTL,
			},
			wantErr: false,
		},
		{
			name: "full config with redis",
			envVars: map[string]string{
				EnvAgentName:           "my-agent",
				EnvNamespace:           "production",
				EnvPromptPackName:      "customer-support",
				EnvPromptPackVersion:   "v1.2.0",
				EnvFacadeType:          "websocket",
				EnvFacadePort:          "9090",
				EnvHandlerMode:         "demo",
				EnvProviderAPIKey:      "sk-prod-key",
				EnvToolRegistryName:    "tools",
				EnvToolRegistryNS:      "shared",
				EnvSessionType:         "redis",
				EnvSessionTTL:          "1h",
				EnvSessionStoreURL:     "redis://localhost:6379",
				EnvPromptPackMountPath: "/custom/path",
				EnvHealthPort:          "9091",
			},
			want: &Config{
				AgentName:             "my-agent",
				Namespace:             "production",
				PromptPackName:        "customer-support",
				PromptPackVersion:     "v1.2.0",
				PromptPackPath:        "/custom/path",
				FacadeType:            FacadeTypeWebSocket,
				FacadePort:            9090,
				HealthPort:            9091,
				HandlerMode:           HandlerModeDemo,
				ProviderAPIKey:        "sk-prod-key",
				ToolRegistryName:      "tools",
				ToolRegistryNamespace: "shared",
				SessionType:           SessionTypeRedis,
				SessionTTL:            time.Hour,
				SessionStoreURL:       "redis://localhost:6379",
			},
			wantErr: false,
		},
		{
			name: "invalid facade port",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvPromptPackName: "my-pack",
				EnvProviderAPIKey: "sk-test-key",
				EnvFacadePort:     "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "invalid session TTL",
			envVars: map[string]string{
				EnvAgentName:      "test-agent",
				EnvNamespace:      "default",
				EnvPromptPackName: "my-pack",
				EnvProviderAPIKey: "sk-test-key",
				EnvSessionTTL:     "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env
			for key := range origEnv {
				_ = os.Unsetenv(key)
			}

			// Set test env vars
			for key, value := range tt.envVars {
				_ = os.Setenv(key, value)
			}

			got, err := LoadFromEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Compare relevant fields
			if got.AgentName != tt.want.AgentName {
				t.Errorf("AgentName = %v, want %v", got.AgentName, tt.want.AgentName)
			}
			if got.Namespace != tt.want.Namespace {
				t.Errorf("Namespace = %v, want %v", got.Namespace, tt.want.Namespace)
			}
			if got.PromptPackName != tt.want.PromptPackName {
				t.Errorf("PromptPackName = %v, want %v", got.PromptPackName, tt.want.PromptPackName)
			}
			if got.FacadePort != tt.want.FacadePort {
				t.Errorf("FacadePort = %v, want %v", got.FacadePort, tt.want.FacadePort)
			}
			if got.SessionType != tt.want.SessionType {
				t.Errorf("SessionType = %v, want %v", got.SessionType, tt.want.SessionType)
			}
			if got.SessionTTL != tt.want.SessionTTL {
				t.Errorf("SessionTTL = %v, want %v", got.SessionTTL, tt.want.SessionTTL)
			}
			if got.HandlerMode != tt.want.HandlerMode {
				t.Errorf("HandlerMode = %v, want %v", got.HandlerMode, tt.want.HandlerMode)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name: "valid config with memory session",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				ProviderAPIKey: "sk-test",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeRuntime,
				SessionType:    SessionTypeMemory,
			},
			wantErr: nil,
		},
		{
			name: "valid config with redis session",
			config: &Config{
				AgentName:       "test-agent",
				Namespace:       "default",
				PromptPackName:  "my-pack",
				ProviderAPIKey:  "sk-test",
				FacadeType:      FacadeTypeWebSocket,
				HandlerMode:     HandlerModeRuntime,
				SessionType:     SessionTypeRedis,
				SessionStoreURL: "redis://localhost:6379",
			},
			wantErr: nil,
		},
		{
			name: "valid config with echo handler (no provider key needed)",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeEcho,
				SessionType:    SessionTypeMemory,
			},
			wantErr: nil,
		},
		{
			name: "valid config with demo handler (no provider key needed)",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeDemo,
				SessionType:    SessionTypeMemory,
			},
			wantErr: nil,
		},
		{
			name: "missing agent name",
			config: &Config{
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeEcho,
				SessionType:    SessionTypeMemory,
			},
			wantErr: ErrMissingAgentName,
		},
		{
			name: "missing namespace",
			config: &Config{
				AgentName:      "test-agent",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeEcho,
				SessionType:    SessionTypeMemory,
			},
			wantErr: ErrMissingNamespace,
		},
		{
			name: "missing promptpack",
			config: &Config{
				AgentName:   "test-agent",
				Namespace:   "default",
				FacadeType:  FacadeTypeWebSocket,
				HandlerMode: HandlerModeEcho,
				SessionType: SessionTypeMemory,
			},
			wantErr: ErrMissingPromptPack,
		},
		{
			name: "missing provider key for runtime handler",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeRuntime,
				SessionType:    SessionTypeMemory,
			},
			wantErr: ErrMissingProviderKey,
		},
		{
			name: "invalid handler mode",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    "invalid",
				SessionType:    SessionTypeMemory,
			},
			wantErr: ErrInvalidHandlerMode,
		},
		{
			name: "invalid facade type",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     "grpc",
				HandlerMode:    HandlerModeEcho,
				SessionType:    SessionTypeMemory,
			},
			wantErr: ErrInvalidFacadeType,
		},
		{
			name: "invalid session type",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeEcho,
				SessionType:    "postgres",
			},
			wantErr: ErrInvalidSessionType,
		},
		{
			name: "redis session without store URL",
			config: &Config{
				AgentName:      "test-agent",
				Namespace:      "default",
				PromptPackName: "my-pack",
				FacadeType:     FacadeTypeWebSocket,
				HandlerMode:    HandlerModeEcho,
				SessionType:    SessionTypeRedis,
			},
			wantErr: ErrMissingSessionStore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Errorf("Validate() expected error %v, got nil", tt.wantErr)
				return
			}

			// Check if error matches or wraps expected error
			if err != tt.wantErr && !containsError(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func containsError(err, target error) bool {
	if err == nil || target == nil {
		return false
	}
	return err.Error() == target.Error() ||
		(len(err.Error()) > len(target.Error()) &&
			err.Error()[:len(target.Error())] == target.Error())
}
