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

	"github.com/altairalabs/omnia/internal/media"
)

// Environment variable names.
const (
	EnvAgentName           = "OMNIA_AGENT_NAME"
	EnvNamespace           = "OMNIA_NAMESPACE"
	EnvPromptPackName      = "OMNIA_PROMPTPACK_NAME"
	EnvFacadeType          = "OMNIA_FACADE_TYPE"
	EnvFacadePort          = "OMNIA_FACADE_PORT"
	EnvHandlerMode         = "OMNIA_HANDLER_MODE"
	EnvRuntimeAddress      = "OMNIA_RUNTIME_ADDRESS"
	EnvPromptPackMountPath = "OMNIA_PROMPTPACK_PATH"
	EnvHealthPort          = "OMNIA_HEALTH_PORT"
	// EnvPromptPackVersion carries the operator-resolved PromptPack's
	// concrete version. Used as a fallback when spec.promptPackRef.Version is
	// nil (a `track:`-selected AgentRuntime), so the facade — which writes
	// the session record — stamps the same concrete version as the runtime
	// instead of an empty string (#1847).
	EnvPromptPackVersion = "OMNIA_PROMPTPACK_VERSION"
	// The Env* / Default* media constants alias internal/media's shared
	// OMNIA_MEDIA_* contract (internal/media/env.go) so the facade
	// (cmd/agent) and runtime (cmd/runtime) binaries read the identical env
	// vars when each builds a media.BuilderConfig for media.Build. See
	// cmd/runtime/media.go for the runtime side of this contract.
	EnvMediaStorageType    = media.EnvStorageType
	EnvMediaStoragePath    = media.EnvStoragePath
	EnvMediaMaxFileSize    = media.EnvMaxFileSize
	EnvMediaDefaultTTL     = media.EnvDefaultTTL
	EnvMediaUploadURLTTL   = media.EnvUploadURLTTL
	EnvMediaDownloadURLTTL = media.EnvDownloadURLTTL

	// S3 storage configuration.
	EnvMediaS3Bucket   = media.EnvS3Bucket
	EnvMediaS3Region   = media.EnvS3Region
	EnvMediaS3Prefix   = media.EnvS3Prefix
	EnvMediaS3Endpoint = media.EnvS3Endpoint // Optional, for S3-compatible services (MinIO, LocalStack)

	// GCS storage configuration.
	EnvMediaGCSBucket = media.EnvGCSBucket
	EnvMediaGCSPrefix = media.EnvGCSPrefix

	// Azure Blob Storage configuration.
	EnvMediaAzureAccount   = media.EnvAzureAccount
	EnvMediaAzureContainer = media.EnvAzureContainer
	EnvMediaAzurePrefix    = media.EnvAzurePrefix
	EnvMediaAzureKey       = media.EnvAzureKey // Optional, for cross-cloud or explicit credentials

	// Tracing configuration.
	EnvTracingEnabled    = "OMNIA_TRACING_ENABLED"
	EnvTracingEndpoint   = "OMNIA_TRACING_ENDPOINT"
	EnvTracingSampleRate = "OMNIA_TRACING_SAMPLE_RATE"
	EnvTracingInsecure   = "OMNIA_TRACING_INSECURE"

	// A2A configuration.
	EnvA2ATaskTTL         = "OMNIA_A2A_TASK_TTL"
	EnvA2AConversationTTL = "OMNIA_A2A_CONVERSATION_TTL"
	EnvA2ATaskStoreType   = "OMNIA_A2A_TASK_STORE_TYPE"
	EnvA2ARedisURL        = "OMNIA_A2A_REDIS_URL"
	EnvA2AEnabled         = "OMNIA_A2A_ENABLED"
	EnvA2APort            = "OMNIA_A2A_PORT"
	EnvA2AClients         = "OMNIA_A2A_CLIENTS"

	// MCP configuration.
	EnvMCPEnabled = "OMNIA_MCP_ENABLED"
	EnvMCPPort    = "OMNIA_MCP_PORT"

	// Internal management-plane twin-listener ports. The facade serves each
	// surface a second time on these ports behind a mgmt-plane-only auth chain
	// (see facade plane-isolation design). In-cluster these are derived from the
	// CRD (gated per-facade on facades[].managementPlane); these env vars are the
	// demo/E2E fallback. Zero means "no internal listener for that surface".
	EnvInternalFacadePort = "OMNIA_INTERNAL_FACADE_PORT"
	EnvInternalA2APort    = "OMNIA_INTERNAL_A2A_PORT"
	EnvInternalMCPPort    = "OMNIA_INTERNAL_MCP_PORT"
)

// Default values.
const (
	DefaultFacadePort          = 8080
	DefaultHealthPort          = 8081
	DefaultRuntimeAddress      = "localhost:9000"
	DefaultSessionTTL          = 24 * time.Hour
	DefaultPromptPackMountPath = "/etc/omnia/pack"
	// DefaultMediaStoragePath / DefaultMediaMaxFileSize / DefaultMediaDefaultTTL /
	// DefaultMediaUploadURLTTL / DefaultMediaDownloadURLTTL alias internal/media's
	// shared defaults (see EnvMediaStorageType above).
	DefaultMediaStoragePath    = media.DefaultStoragePath
	DefaultMediaMaxFileSize    = media.DefaultMaxFileSize
	DefaultMediaDefaultTTL     = media.DefaultDefaultTTL
	DefaultMediaUploadURLTTL   = media.DefaultUploadURLTTL
	DefaultMediaDownloadURLTTL = media.DefaultDownloadURLTTL
	DefaultA2ATaskTTL          = 1 * time.Hour
	DefaultA2AConversationTTL  = 30 * time.Minute
	DefaultA2APort             = 9999
	DefaultMCPPort             = 9998

	// Internal management-plane twin-listener port defaults. Independently
	// declared (not derived from the external port by an offset). Used per
	// facade whose managementPlane is enabled (the default).
	DefaultInternalFacadePort = 18080
	DefaultInternalA2APort    = 19999
	DefaultInternalMCPPort    = 19998
)

// Error format strings.
const errFmtInvalidEnv = "invalid %s: %w"

// envValueTrue is the string value representing true in environment variables.
const envValueTrue = "true"

// FacadeType represents the type of facade to use.
type FacadeType string

const (
	FacadeTypeWebSocket FacadeType = "websocket"
	FacadeTypeA2A       FacadeType = "a2a"
	// FacadeTypeREST is the primary facade type for function-mode
	// AgentRuntimes (#1464). The route is HTTP (POST /functions/{name}) — an
	// honest label for the one-shot request/response surface.
	FacadeTypeREST FacadeType = "rest"
)

// Mode values for Config.Mode. Mirrors AgentRuntime.spec.mode.
const (
	ModeAgent    = "agent"
	ModeFunction = "function"
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

	// WorkspaceName is the workspace this agent belongs to.
	WorkspaceName string

	// PromptPack configuration.
	PromptPackName    string
	PromptPackVersion string
	PromptPackPath    string

	// Facade configuration.
	FacadeType     FacadeType
	FacadePort     int
	HandlerMode    HandlerMode
	RuntimeAddress string

	// Mode is the AgentRuntime.spec.mode discriminator (agent | function).
	// Defaults to "agent" for back-compat with pre-mode CRDs. The agent
	// binary branches on this at startup to decide whether to mount the
	// WebSocket facade or the Functions HTTP handler.
	Mode string

	// FunctionInputSchemaJSON / FunctionOutputSchemaJSON carry the raw
	// JSON-Schema bytes from spec.inputSchema / spec.outputSchema.
	// Populated only when Mode=="function". The function-mode startup
	// path compiles these once via facade.CompileSchema.
	FunctionInputSchemaJSON  []byte
	FunctionOutputSchemaJSON []byte

	// ToolRegistry configuration (optional).
	ToolRegistryName      string
	ToolRegistryNamespace string

	// Session configuration.
	SessionTTL time.Duration

	// ClientToolTimeout overrides the default 60s timeout for client tool
	// responses. Sourced from the primary facade's clientToolTimeout.
	// Zero means "use RuntimeHandler default".
	ClientToolTimeout time.Duration

	// DrainTimeout is how long the facade keeps serving active realtime calls
	// after SIGTERM before tearing down remaining connections.
	// Sourced from the primary facade's drainTimeout. Zero means "use
	// facade.DefaultServerConfig default (30s)".
	DrainTimeout time.Duration

	// Media storage configuration.
	MediaStorageType    MediaStorageType
	MediaStoragePath    string
	MediaMaxFileSize    int64
	MediaDefaultTTL     time.Duration
	MediaUploadURLTTL   time.Duration
	MediaDownloadURLTTL time.Duration

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

	// A2A configuration.
	A2ATaskTTL         time.Duration
	A2AConversationTTL time.Duration
	A2ATaskStoreType   string // "memory" or "redis"
	A2ARedisURL        string // Redis URL for A2A task store
	A2AEnabled         bool   // true when A2A is an additional endpoint (dual-protocol)
	A2APort            int    // port for A2A in dual-protocol mode (default 9999)
	A2AClientsJSON     string // JSON-encoded resolved client list from controller

	// MCP configuration. Function-mode only; the operator's CEL
	// validation rejects MCPEnabled=true on agent-mode runtimes.
	MCPEnabled bool
	MCPPort    int

	// Internal management-plane twin-listener ports. Each surface (WS/A2A/MCP)
	// is served a second time on its internal port behind a mgmt-plane-only
	// auth chain. Zero means "no internal listener for that surface" (mgmt plane
	// disabled, or the surface itself disabled). See plane-isolation design.
	InternalFacadePort int
	InternalA2APort    int
	InternalMCPPort    int

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
	ErrInvalidFacadeType      = errors.New("invalid facade type")
	ErrInvalidHandlerMode     = errors.New("invalid handler mode")
	ErrInvalidMediaStorageTyp = errors.New("invalid media storage type")
	ErrMissingS3Bucket        = errors.New("OMNIA_MEDIA_S3_BUCKET is required for s3 storage type")
	ErrMissingS3Region        = errors.New("OMNIA_MEDIA_S3_REGION is required for s3 storage type")
	ErrMissingGCSBucket       = errors.New("OMNIA_MEDIA_GCS_BUCKET is required for gcs storage type")
	ErrMissingAzureAccount    = errors.New("OMNIA_MEDIA_AZURE_ACCOUNT is required for azure storage type")
	ErrMissingAzureContainer  = errors.New("OMNIA_MEDIA_AZURE_CONTAINER is required for azure storage type")

	// Function-mode validation errors.
	ErrMissingFunctionInputSchema  = errors.New("function mode requires spec.inputSchema")
	ErrMissingFunctionOutputSchema = errors.New("function mode requires spec.outputSchema")
)

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

	// Validate the primary facade type. Agent-mode pods serve websocket or a2a;
	// function-mode pods serve rest (the CRD's CEL gate enforces the
	// mode↔facade-type split — see spec.facades validations).
	switch c.FacadeType {
	case FacadeTypeWebSocket, FacadeTypeA2A, FacadeTypeREST:
		// Valid
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidFacadeType, c.FacadeType)
	}

	if c.Mode == ModeFunction {
		if len(c.FunctionInputSchemaJSON) == 0 {
			return ErrMissingFunctionInputSchema
		}
		if len(c.FunctionOutputSchemaJSON) == 0 {
			return ErrMissingFunctionOutputSchema
		}
		// Function mode serves a one-shot HTTP route; the primary facade is rest.
		if c.FacadeType != FacadeTypeREST {
			return fmt.Errorf("%w: function mode requires a rest facade, got %q",
				ErrInvalidFacadeType, c.FacadeType)
		}
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

func getEnvAsFloat64(key string, defaultValue float64) (float64, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return strconv.ParseFloat(valueStr, 64)
}

// getEnvDuration parses the env var as a time.Duration, returning def when
// unset or unparseable. Mirrors cmd/runtime/media.go's getEnvDuration; the
// facade doesn't import the runtime binary's package, so this is
// intentionally a separate copy of the same tiny helper.
func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return parsed
}
