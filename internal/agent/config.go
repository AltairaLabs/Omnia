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

// Package agent provides the agent runtime configuration and initialization.
package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment variable names.
const (
	EnvAgentName           = "OMNIA_AGENT_NAME"
	EnvNamespace           = "OMNIA_NAMESPACE"
	EnvPromptPackName      = "OMNIA_PROMPTPACK_NAME"
	EnvPromptPackVersion   = "OMNIA_PROMPTPACK_VERSION"
	EnvFacadeType          = "OMNIA_FACADE_TYPE"
	EnvFacadePort          = "OMNIA_FACADE_PORT"
	EnvHandlerMode         = "OMNIA_HANDLER_MODE"
	EnvRuntimeAddress      = "OMNIA_RUNTIME_ADDRESS"
	EnvProviderAPIKey      = "OMNIA_PROVIDER_API_KEY"
	EnvToolRegistryName    = "OMNIA_TOOLREGISTRY_NAME"
	EnvToolRegistryNS      = "OMNIA_TOOLREGISTRY_NAMESPACE"
	EnvSessionType         = "OMNIA_SESSION_TYPE"
	EnvSessionTTL          = "OMNIA_SESSION_TTL"
	EnvSessionStoreURL     = "OMNIA_SESSION_STORE_URL"
	EnvPromptPackMountPath = "OMNIA_PROMPTPACK_MOUNT_PATH"
	EnvHealthPort          = "OMNIA_HEALTH_PORT"
	EnvMediaStorageType    = "OMNIA_MEDIA_STORAGE_TYPE"
	EnvMediaStoragePath    = "OMNIA_MEDIA_STORAGE_PATH"
	EnvMediaMaxFileSize    = "OMNIA_MEDIA_MAX_FILE_SIZE"
	EnvMediaDefaultTTL     = "OMNIA_MEDIA_DEFAULT_TTL"

	// S3 storage configuration.
	EnvMediaS3Bucket   = "OMNIA_MEDIA_S3_BUCKET"
	EnvMediaS3Region   = "OMNIA_MEDIA_S3_REGION"
	EnvMediaS3Prefix   = "OMNIA_MEDIA_S3_PREFIX"
	EnvMediaS3Endpoint = "OMNIA_MEDIA_S3_ENDPOINT" // Optional, for S3-compatible services (MinIO, LocalStack)

	// GCS storage configuration.
	EnvMediaGCSBucket = "OMNIA_MEDIA_GCS_BUCKET"
	EnvMediaGCSPrefix = "OMNIA_MEDIA_GCS_PREFIX"

	// Azure Blob Storage configuration.
	EnvMediaAzureAccount   = "OMNIA_MEDIA_AZURE_ACCOUNT"
	EnvMediaAzureContainer = "OMNIA_MEDIA_AZURE_CONTAINER"
	EnvMediaAzurePrefix    = "OMNIA_MEDIA_AZURE_PREFIX"
	EnvMediaAzureKey       = "OMNIA_MEDIA_AZURE_KEY" // Optional, for cross-cloud or explicit credentials

	// Tracing configuration.
	EnvTracingEnabled    = "OMNIA_TRACING_ENABLED"
	EnvTracingEndpoint   = "OMNIA_TRACING_ENDPOINT"
	EnvTracingSampleRate = "OMNIA_TRACING_SAMPLE_RATE"
	EnvTracingInsecure   = "OMNIA_TRACING_INSECURE"
)

// Default values.
const (
	DefaultFacadePort          = 8080
	DefaultHealthPort          = 8081
	DefaultRuntimeAddress      = "localhost:9000"
	DefaultSessionTTL          = 24 * time.Hour
	DefaultPromptPackMountPath = "/etc/promptpack"
	DefaultMediaStoragePath    = "/var/lib/omnia/media"
	DefaultMediaMaxFileSize    = 100 * 1024 * 1024 // 100MB
	DefaultMediaDefaultTTL     = 24 * time.Hour
)

// Error format strings.
const errFmtInvalidEnv = "invalid %s: %w"

// FacadeType represents the type of facade to use.
type FacadeType string

const (
	FacadeTypeWebSocket FacadeType = "websocket"
)

// SessionType represents the type of session store.
type SessionType string

const (
	SessionTypeMemory SessionType = "memory"
	SessionTypeRedis  SessionType = "redis"
)

// MediaStorageType represents the type of media storage backend.
type MediaStorageType string

const (
	// MediaStorageTypeNone disables media storage.
	MediaStorageTypeNone MediaStorageType = "none"
	// MediaStorageTypeLocal uses the local filesystem for media storage.
	MediaStorageTypeLocal MediaStorageType = "local"
	// MediaStorageTypeS3 uses Amazon S3 or S3-compatible storage.
	MediaStorageTypeS3 MediaStorageType = "s3"
	// MediaStorageTypeGCS uses Google Cloud Storage.
	MediaStorageTypeGCS MediaStorageType = "gcs"
	// MediaStorageTypeAzure uses Azure Blob Storage.
	MediaStorageTypeAzure MediaStorageType = "azure"
)

// HandlerMode represents the message handler mode.
type HandlerMode string

const (
	// HandlerModeEcho echoes back the input message (for testing).
	HandlerModeEcho HandlerMode = "echo"
	// HandlerModeDemo provides canned responses with streaming simulation (for demos).
	HandlerModeDemo HandlerMode = "demo"
	// HandlerModeRuntime uses the runtime framework in the container (production).
	HandlerModeRuntime HandlerMode = "runtime"
)

// Config holds the agent runtime configuration.
type Config struct {
	// AgentName is the name of the agent.
	AgentName string

	// Namespace is the Kubernetes namespace.
	Namespace string

	// PromptPack configuration.
	PromptPackName    string
	PromptPackVersion string
	PromptPackPath    string

	// Facade configuration.
	FacadeType     FacadeType
	FacadePort     int
	HandlerMode    HandlerMode
	RuntimeAddress string

	// Provider configuration.
	ProviderAPIKey string

	// ToolRegistry configuration (optional).
	ToolRegistryName      string
	ToolRegistryNamespace string

	// Session configuration.
	SessionType     SessionType
	SessionTTL      time.Duration
	SessionStoreURL string

	// Media storage configuration.
	MediaStorageType MediaStorageType
	MediaStoragePath string
	MediaMaxFileSize int64
	MediaDefaultTTL  time.Duration

	// S3 storage configuration.
	MediaS3Bucket   string
	MediaS3Region   string
	MediaS3Prefix   string
	MediaS3Endpoint string // Optional, for S3-compatible services

	// GCS storage configuration.
	MediaGCSBucket string
	MediaGCSPrefix string

	// Azure Blob Storage configuration.
	MediaAzureAccount   string
	MediaAzureContainer string
	MediaAzurePrefix    string
	MediaAzureKey       string // Optional, for cross-cloud or explicit credentials

	// Tracing configuration.
	TracingEnabled    bool
	TracingEndpoint   string
	TracingSampleRate float64
	TracingInsecure   bool

	// Health check port.
	HealthPort int
}

// Error format for wrapping validation errors with values.
const errWithValueFmt = "%w: %s"

// Validation errors.
var (
	ErrMissingAgentName       = errors.New("OMNIA_AGENT_NAME is required")
	ErrMissingNamespace       = errors.New("OMNIA_NAMESPACE is required")
	ErrMissingPromptPack      = errors.New("OMNIA_PROMPTPACK_NAME is required")
	ErrMissingProviderKey     = errors.New("OMNIA_PROVIDER_API_KEY is required for runtime handler mode")
	ErrInvalidFacadeType      = errors.New("invalid facade type")
	ErrInvalidHandlerMode     = errors.New("invalid handler mode")
	ErrInvalidSessionType     = errors.New("invalid session type")
	ErrMissingSessionStore    = errors.New("OMNIA_SESSION_STORE_URL is required for redis session type")
	ErrInvalidMediaStorageTyp = errors.New("invalid media storage type")
	ErrMissingS3Bucket        = errors.New("OMNIA_MEDIA_S3_BUCKET is required for s3 storage type")
	ErrMissingS3Region        = errors.New("OMNIA_MEDIA_S3_REGION is required for s3 storage type")
	ErrMissingGCSBucket       = errors.New("OMNIA_MEDIA_GCS_BUCKET is required for gcs storage type")
	ErrMissingAzureAccount    = errors.New("OMNIA_MEDIA_AZURE_ACCOUNT is required for azure storage type")
	ErrMissingAzureContainer  = errors.New("OMNIA_MEDIA_AZURE_CONTAINER is required for azure storage type")
)

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		AgentName:             os.Getenv(EnvAgentName),
		Namespace:             os.Getenv(EnvNamespace),
		PromptPackName:        os.Getenv(EnvPromptPackName),
		PromptPackVersion:     os.Getenv(EnvPromptPackVersion),
		PromptPackPath:        getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath),
		ProviderAPIKey:        os.Getenv(EnvProviderAPIKey),
		ToolRegistryName:      os.Getenv(EnvToolRegistryName),
		ToolRegistryNamespace: os.Getenv(EnvToolRegistryNS),
		SessionStoreURL:       os.Getenv(EnvSessionStoreURL),
	}

	// Parse facade type
	facadeType := getEnvOrDefault(EnvFacadeType, string(FacadeTypeWebSocket))
	cfg.FacadeType = FacadeType(facadeType)

	// Parse handler mode
	handlerMode := getEnvOrDefault(EnvHandlerMode, string(HandlerModeRuntime))
	cfg.HandlerMode = HandlerMode(handlerMode)

	// Parse runtime address (for runtime handler mode)
	cfg.RuntimeAddress = getEnvOrDefault(EnvRuntimeAddress, DefaultRuntimeAddress)

	// Parse facade port
	facadePort, err := getEnvAsInt(EnvFacadePort, DefaultFacadePort)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvFacadePort, err)
	}
	cfg.FacadePort = facadePort

	// Parse health port
	healthPort, err := getEnvAsInt(EnvHealthPort, DefaultHealthPort)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvHealthPort, err)
	}
	cfg.HealthPort = healthPort

	// Parse session type
	sessionType := getEnvOrDefault(EnvSessionType, string(SessionTypeMemory))
	cfg.SessionType = SessionType(sessionType)

	// Parse session TTL
	sessionTTL, err := getEnvAsDuration(EnvSessionTTL, DefaultSessionTTL)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvSessionTTL, err)
	}
	cfg.SessionTTL = sessionTTL

	// Parse media storage configuration
	mediaStorageType := getEnvOrDefault(EnvMediaStorageType, string(MediaStorageTypeNone))
	cfg.MediaStorageType = MediaStorageType(mediaStorageType)
	cfg.MediaStoragePath = getEnvOrDefault(EnvMediaStoragePath, DefaultMediaStoragePath)

	mediaMaxFileSize, err := getEnvAsInt64(EnvMediaMaxFileSize, DefaultMediaMaxFileSize)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvMediaMaxFileSize, err)
	}
	cfg.MediaMaxFileSize = mediaMaxFileSize

	mediaDefaultTTL, err := getEnvAsDuration(EnvMediaDefaultTTL, DefaultMediaDefaultTTL)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvMediaDefaultTTL, err)
	}
	cfg.MediaDefaultTTL = mediaDefaultTTL

	// Parse S3 storage configuration
	cfg.MediaS3Bucket = os.Getenv(EnvMediaS3Bucket)
	cfg.MediaS3Region = os.Getenv(EnvMediaS3Region)
	cfg.MediaS3Prefix = os.Getenv(EnvMediaS3Prefix)
	cfg.MediaS3Endpoint = os.Getenv(EnvMediaS3Endpoint)

	// Parse GCS storage configuration
	cfg.MediaGCSBucket = os.Getenv(EnvMediaGCSBucket)
	cfg.MediaGCSPrefix = os.Getenv(EnvMediaGCSPrefix)

	// Parse Azure storage configuration
	cfg.MediaAzureAccount = os.Getenv(EnvMediaAzureAccount)
	cfg.MediaAzureContainer = os.Getenv(EnvMediaAzureContainer)
	cfg.MediaAzurePrefix = os.Getenv(EnvMediaAzurePrefix)
	cfg.MediaAzureKey = os.Getenv(EnvMediaAzureKey)

	// Parse tracing configuration
	cfg.TracingEnabled = os.Getenv(EnvTracingEnabled) == "true"
	cfg.TracingEndpoint = os.Getenv(EnvTracingEndpoint)
	cfg.TracingInsecure = os.Getenv(EnvTracingInsecure) == "true"

	tracingSampleRate, err := getEnvAsFloat64(EnvTracingSampleRate, 1.0)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvTracingSampleRate, err)
	}
	cfg.TracingSampleRate = tracingSampleRate

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.AgentName == "" {
		return ErrMissingAgentName
	}
	if c.Namespace == "" {
		return ErrMissingNamespace
	}
	if c.PromptPackName == "" {
		return ErrMissingPromptPack
	}

	// Validate handler mode
	switch c.HandlerMode {
	case HandlerModeEcho, HandlerModeDemo, HandlerModeRuntime:
		// Valid - runtime mode delegates to sidecar which handles provider keys
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidHandlerMode, c.HandlerMode)
	}

	// Validate facade type
	switch c.FacadeType {
	case FacadeTypeWebSocket:
		// Valid
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidFacadeType, c.FacadeType)
	}

	// Validate session type
	switch c.SessionType {
	case SessionTypeMemory:
		// Valid, no additional config needed
	case SessionTypeRedis:
		if c.SessionStoreURL == "" {
			return ErrMissingSessionStore
		}
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidSessionType, c.SessionType)
	}

	// Validate media storage type
	switch c.MediaStorageType {
	case MediaStorageTypeNone, MediaStorageTypeLocal:
		// Valid, no additional config needed
	case MediaStorageTypeS3:
		if c.MediaS3Bucket == "" {
			return ErrMissingS3Bucket
		}
		if c.MediaS3Region == "" {
			return ErrMissingS3Region
		}
	case MediaStorageTypeGCS:
		if c.MediaGCSBucket == "" {
			return ErrMissingGCSBucket
		}
	case MediaStorageTypeAzure:
		if c.MediaAzureAccount == "" {
			return ErrMissingAzureAccount
		}
		if c.MediaAzureContainer == "" {
			return ErrMissingAzureContainer
		}
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidMediaStorageTyp, c.MediaStorageType)
	}

	return nil
}

// Helper functions for environment variable parsing.

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) (int, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(valueStr)
}

func getEnvAsInt64(key string, defaultValue int64) (int64, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return strconv.ParseInt(valueStr, 10, 64)
}

func getEnvAsDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return time.ParseDuration(valueStr)
}

func getEnvAsFloat64(key string, defaultValue float64) (float64, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return strconv.ParseFloat(valueStr, 64)
}
