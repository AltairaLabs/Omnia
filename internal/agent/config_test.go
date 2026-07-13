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
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name: "valid config",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeRuntime,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "valid config minimal",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeRuntime,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "valid config with echo handler (no provider key needed)",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "valid config with demo handler (no provider key needed)",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeDemo,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "missing agent name",
			config: &Config{
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: ErrMissingAgentName,
		},
		{
			name: "missing namespace",
			config: &Config{
				AgentName:        "test-agent",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: ErrMissingNamespace,
		},
		{
			name: "missing promptpack",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: ErrMissingPromptPack,
		},
		{
			name: "runtime handler mode valid without provider key",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeRuntime,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: nil, // Facade delegates to runtime sidecar which handles provider keys
		},
		{
			name: "invalid handler mode",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      "invalid",
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: ErrInvalidHandlerMode,
		},
		{
			// "grpc" became a valid facade type in #1108 (Functions PR 4);
			// pick an actually-unknown value to assert the negative case.
			name: "invalid facade type",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       "unknown-facade",
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeNone,
			},
			wantErr: ErrInvalidFacadeType,
		},
		{
			name: "invalid media storage type",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: "ftp", // Invalid - not implemented
			},
			wantErr: ErrInvalidMediaStorageTyp,
		},
		{
			name: "valid config with local media storage",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeLocal,
			},
			wantErr: nil,
		},
		{
			name: "s3 storage without bucket",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeS3,
				MediaS3Region:    "us-west-2",
			},
			wantErr: ErrMissingS3Bucket,
		},
		{
			name: "s3 storage without region",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeS3,
				MediaS3Bucket:    "my-bucket",
			},
			wantErr: ErrMissingS3Region,
		},
		{
			name: "valid s3 storage config",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeS3,
				MediaS3Bucket:    "my-bucket",
				MediaS3Region:    "us-west-2",
			},
			wantErr: nil,
		},
		{
			name: "gcs storage without bucket",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeGCS,
			},
			wantErr: ErrMissingGCSBucket,
		},
		{
			name: "valid gcs storage config",
			config: &Config{
				AgentName:        "test-agent",
				Namespace:        "default",
				PromptPackName:   "my-pack",
				FacadeType:       FacadeTypeWebSocket,
				HandlerMode:      HandlerModeEcho,
				MediaStorageType: MediaStorageTypeGCS,
				MediaGCSBucket:   "my-gcs-bucket",
			},
			wantErr: nil,
		},
		{
			name: "azure storage without account",
			config: &Config{
				AgentName:           "test-agent",
				Namespace:           "default",
				PromptPackName:      "my-pack",
				FacadeType:          FacadeTypeWebSocket,
				HandlerMode:         HandlerModeEcho,
				MediaStorageType:    MediaStorageTypeAzure,
				MediaAzureContainer: "my-container",
			},
			wantErr: ErrMissingAzureAccount,
		},
		{
			name: "azure storage without container",
			config: &Config{
				AgentName:         "test-agent",
				Namespace:         "default",
				PromptPackName:    "my-pack",
				FacadeType:        FacadeTypeWebSocket,
				HandlerMode:       HandlerModeEcho,
				MediaStorageType:  MediaStorageTypeAzure,
				MediaAzureAccount: "myaccount",
			},
			wantErr: ErrMissingAzureContainer,
		},
		{
			name: "valid azure storage config",
			config: &Config{
				AgentName:           "test-agent",
				Namespace:           "default",
				PromptPackName:      "my-pack",
				FacadeType:          FacadeTypeWebSocket,
				HandlerMode:         HandlerModeEcho,
				MediaStorageType:    MediaStorageTypeAzure,
				MediaAzureAccount:   "myaccount",
				MediaAzureContainer: "my-container",
			},
			wantErr: nil,
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

func TestGetEnvDuration(t *testing.T) {
	t.Setenv("OMNIA_TEST_ENV_DURATION", "")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() = %v, want 1h for an unset var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_DURATION", "not-a-duration")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() = %v, want 1h for an unparseable var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_DURATION", "30m")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != 30*time.Minute {
		t.Errorf("getEnvDuration() = %v, want 30m", got)
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
