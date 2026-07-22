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

package controller

// Log message constants.
const (
	logMsgFailedToUpdateStatus = "Failed to update status"
)

// Kubernetes label constants.
const (
	labelAppName      = "app.kubernetes.io/name"
	labelAppInstance  = "app.kubernetes.io/instance"
	labelAppManagedBy = "app.kubernetes.io/managed-by"
	labelAppComponent = "app.kubernetes.io/component"
	labelOmniaComp    = "omnia.altairalabs.ai/component"
	labelOmniaTrack   = "omnia.altairalabs.ai/track"
	// labelOmniaMode mirrors AgentRuntime.spec.mode on the pod for ops
	// visibility (kubectl filtering, dashboards). Value matches the
	// CRD enum: "agent" or "function".
	labelOmniaMode = "omnia.altairalabs.ai/mode"
)

// Label value constants.
const (
	labelValueOmniaAgent    = "omnia-agent"
	labelValueOmniaOperator = "omnia-operator"
)

// KEDA API group constant.
const kedaAPIGroup = "keda.sh"

// DefaultKEDAConnectionThreshold is the default target connections per pod for KEDA scaling.
// Set to 200 which is appropriate for text chat workloads. Audio workloads should use ~20.
const DefaultKEDAConnectionThreshold = 200

const (
	// FacadeContainerName is the name of the facade container in the pod.
	FacadeContainerName = "facade"
	// RuntimeContainerName is the name of the runtime container in the pod.
	RuntimeContainerName = "runtime"
	// DefaultFacadeImage is the default image for the facade container.
	DefaultFacadeImage = "ghcr.io/altairalabs/omnia-facade:latest"
	// DefaultFrameworkImage is the default image for the framework container (PromptKit).
	DefaultFrameworkImage = "ghcr.io/altairalabs/omnia-runtime:latest"
	// DefaultFacadePort is the default port for the WebSocket facade.
	DefaultFacadePort = 8080
	// DefaultA2APort is the default port for the A2A endpoint in dual-protocol mode.
	DefaultA2APort = 9999
	// DefaultMCPPort is the default port for the MCP Streamable HTTP server
	// on function-mode pods (alongside the HTTP POST /functions/{name} route).
	DefaultMCPPort = 9998
	// portNameMCP is the container/service port name for the MCP endpoint.
	portNameMCP = "mcp"

	// Internal management-plane twin-listener ports. The facade serves each
	// surface a second time on these ports behind a mgmt-plane-only auth chain
	// when a facade's managementPlane is enabled. Independently declared
	// (not an offset of the external port); must match the agent package's
	// Default Internal*Port constants.
	DefaultInternalFacadePort = 18080
	DefaultInternalA2APort    = 19999
	DefaultInternalMCPPort    = 19998
	// portNameFacadeMgmt / portNameA2AMgmt / portNameMCPMgmt are the
	// container/service port names for the internal management-plane listeners.
	portNameFacadeMgmt = "facade-mgmt"
	portNameA2AMgmt    = "a2a-mgmt"
	portNameMCPMgmt    = "mcp-mgmt"

	// appProtocolHTTP is the Istio Service-port appProtocol value stamped on
	// every agent facade port. Setting appProtocol is what lets an Istio
	// waypoint (or sidecar) do L7 on a port — required for mode=mesh weighted
	// routing AND for the facade's WebSocket upgrade to traverse the waypoint.
	// Without it Istio treats the port as opaque TCP and the waypoint silently
	// bypasses/breaks L7. Every facade protocol (websocket/a2a/rest/mcp) is HTTP.
	appProtocolHTTP = "http"
	// DefaultFacadeHealthPort is the health port for the facade container.
	DefaultFacadeHealthPort = 8081
	// DefaultRuntimeGRPCPort is the gRPC port for the runtime container.
	DefaultRuntimeGRPCPort = 9000
	// DefaultRuntimeHealthPort is the health port for the runtime container.
	DefaultRuntimeHealthPort = 9001
	// FinalizerName is the finalizer for AgentRuntime resources.
	FinalizerName = "agentruntime.omnia.altairalabs.ai/finalizer"
	// ToolsConfigMapSuffix is the suffix for the tools ConfigMap name.
	ToolsConfigMapSuffix = "-tools"
	// ToolsConfigFileName is the filename for tools configuration.
	ToolsConfigFileName = "tools.yaml"
	// ToolsMountPath is the mount path for tools configuration.
	ToolsMountPath = "/etc/omnia/tools"
	// ToolSecretsSecretSuffix is appended to the AgentRuntime name for the
	// operator-managed Secret holding resolved tool auth tokens.
	ToolSecretsSecretSuffix = "-tool-secrets"
	// ToolSecretsMountPath is where the tool-secrets Secret is mounted read-only
	// on the runtime container.
	ToolSecretsMountPath = "/etc/omnia/tool-secrets"
	// RuntimePackCacheMountPath is a writable emptyDir the runtime uses to stage
	// the tool-surfaced pack. The container root filesystem is read-only, so the
	// rewritten pack cannot go to /tmp; this emptyDir mount is writable.
	RuntimePackCacheMountPath = "/var/run/omnia/pack-cache"
	// runtimePackCacheVolumeName is the emptyDir volume backing the pack cache.
	runtimePackCacheVolumeName = "pack-cache"
	// CanaryConfigMapSuffix is the suffix for the per-agent canary override
	// ConfigMap name (<agent>-canary-config). Mounted only into candidate pods.
	CanaryConfigMapSuffix = "-canary-config"
	// CanaryOverrideFileName is the key/filename for the canary override JSON.
	CanaryOverrideFileName = "override.json"
	// CanaryOverrideMountPath is the mount path for the canary override; the
	// runtime reads <CanaryOverrideMountPath>/<CanaryOverrideFileName>. It MUST
	// match the runtime's defaultCanaryOverridePath.
	CanaryOverrideMountPath = "/etc/omnia/canary"
	// PromptPackMountPath is the mount path for PromptPack files.
	PromptPackMountPath = "/etc/omnia/pack"
	// MockProviderAnnotation enables mock provider for testing.
	MockProviderAnnotation = "omnia.altairalabs.ai/mock-provider"
	// healthzPath is the path for health probes.
	healthzPath = "/healthz"
	// fieldPathInstanceLabel is the downward API field path for the instance label.
	fieldPathInstanceLabel = "metadata.labels['app.kubernetes.io/instance']"
	// fieldPathNamespace is the downward API field path for the namespace.
	fieldPathNamespace = "metadata.namespace"
	// promptpackConfigVolumeName is the name of the PromptPack config volume.
	promptpackConfigVolumeName = "promptpack-config"
	// toolsConfigVolumeName is the name of the tools config volume.
	toolsConfigVolumeName = "tools-config"
	// toolSecretsVolumeName is the name of the tool-secrets volume.
	toolSecretsVolumeName = "tool-secrets"
	// canaryOverrideVolumeName is the name of the canary override volume.
	canaryOverrideVolumeName = "canary-override"
	// EnvMgmtPlaneJWKSURL is the env var the operator sets on the facade
	// container pointing at the dashboard's JWKS endpoint, e.g.
	// http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks.
	// cmd/agent reads this and, when present, constructs a JWKS-backed
	// mgmt-plane validator that fetches the dashboard's signing public
	// key on demand. Replaces the old per-workspace ConfigMap mirror
	// that silently went stale on key rotation.
	EnvMgmtPlaneJWKSURL = "OMNIA_MGMT_PLANE_JWKS_URL"
)

// Deployment-builder string constants. Extracted so the builder helper files
// reference named values instead of repeated literals (goconst / SonarCloud
// S1192). Values are unchanged from the inline literals they replace.
const (
	// portNameFacade is the container port name for the primary facade port.
	portNameFacade = "facade"
	// readyzPath is the path for readiness probes.
	readyzPath = "/readyz"
	// capabilityAll is the Linux capability set dropped by hardened containers.
	capabilityAll = "ALL"
	// promptNameDefault is the default prompt name injected into the runtime.
	promptNameDefault = "default"
	// envValueTrue is the string value for boolean-true environment variables.
	envValueTrue = "true"
	// envOmniaAgentName is the env var carrying the agent name (downward API).
	envOmniaAgentName = "OMNIA_AGENT_NAME"
	// envOmniaNamespace is the env var carrying the namespace (downward API).
	envOmniaNamespace = "OMNIA_NAMESPACE"
	// envPromptPackVersion carries the RESOLVED PromptPack's concrete
	// semver (see appendPromptPackVersionEnv). A `track:`-selected
	// AgentRuntime has spec.promptPackRef.Version == nil, so without this
	// the runtime/facade would stamp an empty version on sessions — this
	// keeps the eval-path version stamp concrete (#1847).
	envPromptPackVersion = "OMNIA_PROMPTPACK_VERSION"
)

// Eval-related constants.
const (
	// DefaultEvalWorkerImage is the default image for the arena-eval-worker container.
	DefaultEvalWorkerImage = "ghcr.io/altairalabs/omnia-eval-worker:latest"
	// EvalWorkerContainerName is the container name inside the eval worker Deployment.
	EvalWorkerContainerName = "eval-worker"
)

// Eval environment variable name constants.
const (
	envRedisURL      = "REDIS_URL"
	envSessionAPIURL = "SESSION_API_URL"

	// envWorkspaceName carries the Workspace CR's metadata.name to pods that
	// must read their own Workspace. Must match pkg/k8s.EnvWorkspaceName. It is
	// the workspace NAME, never the namespace that workspace owns (#1875).
	envWorkspaceName = "OMNIA_WORKSPACE_NAME"
	envNamespace     = "NAMESPACE"
	envServiceGroup  = "OMNIA_SERVICE_GROUP"
)

// Eval label constants.
const (
	labelEvalWorkerComp  = "eval-worker"
	labelValueEvalWorker = "arena-eval-worker"
)

// Condition types for AgentRuntime
const (
	ConditionTypeReady             = "Ready"
	ConditionTypeDeploymentReady   = "DeploymentReady"
	ConditionTypeServiceReady      = "ServiceReady"
	ConditionTypePromptPackReady   = "PromptPackReady"
	ConditionTypeToolRegistryReady = "ToolRegistryReady"
	ConditionTypeProviderReady     = "ProviderReady"
	ConditionTypePackContentValid  = "PackContentValid"
	// ConditionTypeFrameworkReady is False when the AgentRuntime's declared
	// framework.type has no resolvable runtime image (issue #1206) — the
	// operator blocks rather than silently running the PromptKit image.
	ConditionTypeFrameworkReady = "FrameworkReady"
	ConditionTypeRolloutActive  = "RolloutActive"

	// ConditionTypeTrafficRouting surfaces rollout traffic-routing health —
	// False with a reason when the resolved mode degraded from what was
	// requested (e.g. mesh unavailable) or weight is approximated.
	ConditionTypeTrafficRouting = "TrafficRouting"

	// ConditionTypeExternalAuth surfaces the effective facade auth
	// configuration. False/Unreachable means the operator explicitly
	// set a facade's managementPlane=false without
	// configuring any data-plane validator — the agent cannot accept
	// any callers. Always emitted so operators can assert on it from
	// kubectl describe / helm unittest.
	ConditionTypeExternalAuth = "ExternalAuth"

	// ConditionTypeAutoscalingReady surfaces whether the agent's effective
	// autoscaling policy (its own spec.runtime.autoscaling, or the inherited
	// WorkspaceServiceGroup.autoscaling default) is in effect. True/Scaling
	// means an HPA or KEDA ScaledObject is reconciled; True/Disabled means no
	// policy applies and the agent uses static replicas; False/KEDANotInstalled
	// means the policy requested type "keda" but the KEDA CRDs are absent — the
	// agent stays at static replicas (non-blocking).
	ConditionTypeAutoscalingReady = "AutoscalingReady"
)

// Autoscaling condition reasons.
const (
	reasonAutoscalingScaling     = "Scaling"
	reasonAutoscalingDisabled    = "Disabled"
	reasonAutoscalingError       = "Error"
	reasonAutoscalingKEDAMissing = "KEDANotInstalled"
)
