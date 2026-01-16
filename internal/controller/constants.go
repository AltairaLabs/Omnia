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
	labelOmniaComp    = "omnia.altairalabs.ai/component"
)

// Label value constants.
const (
	labelValueOmniaAgent    = "omnia-agent"
	labelValueOmniaOperator = "omnia-operator"
)

// KEDA API group constant.
const kedaAPIGroup = "keda.sh"

const (
	// FacadeContainerName is the name of the facade container in the pod.
	FacadeContainerName = "facade"
	// RuntimeContainerName is the name of the runtime container in the pod.
	RuntimeContainerName = "runtime"
	// DefaultFacadeImage is the default image for the facade container.
	DefaultFacadeImage = "ghcr.io/altairalabs/omnia-facade:latest"
	// DefaultFrameworkImage is the default image for the framework container.
	DefaultFrameworkImage = "ghcr.io/altairalabs/omnia-runtime:latest"
	// DefaultFacadePort is the default port for the WebSocket facade.
	DefaultFacadePort = 8080
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
	// PromptPackMountPath is the mount path for PromptPack files.
	PromptPackMountPath = "/etc/omnia/pack"
	// MockProviderAnnotation enables mock provider for testing.
	MockProviderAnnotation = "omnia.altairalabs.ai/mock-provider"
	// healthzPath is the path for health probes.
	healthzPath = "/healthz"
	// toolsConfigVolumeName is the name of the tools config volume.
	toolsConfigVolumeName = "tools-config"
)

// Condition types for AgentRuntime
const (
	ConditionTypeReady             = "Ready"
	ConditionTypeDeploymentReady   = "DeploymentReady"
	ConditionTypeServiceReady      = "ServiceReady"
	ConditionTypePromptPackReady   = "PromptPackReady"
	ConditionTypeToolRegistryReady = "ToolRegistryReady"
	ConditionTypeProviderReady     = "ProviderReady"
)
