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

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// secretKeyRedisURL is the key within a session storeRef secret that holds the
// Redis connection URL (consumed by the facade's blip-resume route store).
const secretKeyRedisURL = "url"

// buildFacadeEnvVars creates environment variables for the facade container.
func (r *AgentRuntimeReconciler) buildFacadeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
) []corev1.EnvVar {
	port := primaryFacadePort(agentRuntime)

	envVars := []corev1.EnvVar{
		// Identity from Downward API — facade reads CRD directly using these
		{
			Name: envOmniaAgentName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name: envOmniaNamespace,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathNamespace,
				},
			},
		},
		{
			Name:  "OMNIA_FACADE_PORT",
			Value: fmt.Sprintf("%d", port),
		},
		{
			Name:  "OMNIA_HEALTH_PORT",
			Value: fmt.Sprintf("%d", DefaultFacadeHealthPort),
		},
		{
			// Rollout-semantic variant the facade records on each session when
			// the x-omnia-variant request header is absent (replica-weighted
			// mode has no routing layer to set it). The candidate Deployment
			// overrides this to variantCandidate (#1449).
			Name:  envFacadeVariant,
			Value: variantStable,
		},
	}

	// Determine handler mode - default to runtime if not specified
	handlerMode := omniav1alpha1.HandlerModeRuntime
	if f := primaryFacade(agentRuntime); f != nil && f.Handler != nil {
		handlerMode = *f.Handler
	}

	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_HANDLER_MODE",
		Value: string(handlerMode),
	})

	// Only add runtime address if using runtime handler mode
	if handlerMode == omniav1alpha1.HandlerModeRuntime {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_RUNTIME_ADDRESS",
			Value: fmt.Sprintf("localhost:%d", DefaultRuntimeGRPCPort),
		})
	}

	// Add tracing configuration if enabled
	if r.TracingEnabled && r.TracingEndpoint != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENABLED",
				Value: envValueTrue,
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENDPOINT",
				Value: r.TracingEndpoint,
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_INSECURE",
				Value: envValueTrue,
			},
		)
	}

	// Point the facade at the dashboard's JWKS endpoint so cmd/agent can
	// build a JWKS-backed mgmt-plane validator that fetches signing
	// pubkeys on demand (and refreshes on key rotation). Empty URL means
	// no dashboard is deployed in this install — facade stays mgmt-plane
	// unaware, matching the original behaviour.
	if r.MgmtPlaneJWKSURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  EnvMgmtPlaneJWKSURL,
			Value: r.MgmtPlaneJWKSURL,
		})
	}

	// POD_IP via Downward API — used by the facade's blip-resume route store to
	// record which pod holds a parked realtime session so peers can redirect
	// reconnecting clients.
	envVars = append(envVars, corev1.EnvVar{
		Name: "POD_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	})

	// OMNIA_ROUTE_REDIS_URL — the Redis URL used by the facade's blip-resume
	// route store. Sourced from the same secret as the context store when a
	// Redis-backed context store is configured; omitted otherwise so the facade
	// falls back to the noop route store silently.
	if agentRuntime.Spec.Context != nil &&
		agentRuntime.Spec.Context.Type == omniav1alpha1.ContextStoreTypeRedis &&
		agentRuntime.Spec.Context.StoreRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "OMNIA_ROUTE_REDIS_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *agentRuntime.Spec.Context.StoreRef,
					Key:                  secretKeyRedisURL,
				},
			},
		})
	}

	// Add extra env vars from the primary facade
	if f := primaryFacade(agentRuntime); f != nil && f.ExtraEnv != nil {
		envVars = append(envVars, f.ExtraEnv...)
	}

	return envVars
}

// buildRuntimeEnvVars creates environment variables for the runtime container.
// The runtime reads CRD directly for provider, session, media, eval, and promptpack config.
// Only identity, mount paths, ports, tools, tracing, and mock annotation are injected here.
func (r *AgentRuntimeReconciler) buildRuntimeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	toolRegistry *omniav1alpha1.ToolRegistry,
) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		// Identity from Downward API — runtime reads CRD directly using these
		{
			Name: envOmniaAgentName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name: envOmniaNamespace,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathNamespace,
				},
			},
		},
		// PromptPack path for the runtime to load (mount-path, operator controls)
		{
			Name:  "OMNIA_PROMPTPACK_PATH",
			Value: PromptPackMountPath + "/pack.json",
		},
		// Default prompt name (can be overridden per-request)
		{
			Name:  "OMNIA_PROMPT_NAME",
			Value: promptNameDefault,
		},
		// gRPC port for the runtime server
		{
			Name:  "OMNIA_GRPC_PORT",
			Value: fmt.Sprintf("%d", DefaultRuntimeGRPCPort),
		},
		// Health check port
		{
			Name:  "OMNIA_HEALTH_PORT",
			Value: fmt.Sprintf("%d", DefaultRuntimeHealthPort),
		},
	}

	// Add tools config path if tool registry is present
	if toolRegistry != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLS_CONFIG_PATH",
			Value: ToolsMountPath + "/" + ToolsConfigFileName,
		})
		// The runtime unions the registry's tools into the pack's allowed-tools
		// list so they reach the model, writing the rewritten pack here. The
		// container root filesystem is read-only, so this MUST point at the
		// writable emptyDir the operator mounts (see buildVolumes).
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PACK_CACHE_DIR",
			Value: RuntimePackCacheMountPath,
		})
	}

	// Memory: inject workspace UID so the runtime can scope memory operations.
	// The memory_entities table uses workspace_id as UUID (the Workspace CR's UID).
	if agentRuntime.Spec.Memory != nil && agentRuntime.Spec.Memory.Enabled {
		wsUID := r.resolveWorkspaceUIDForNamespace(agentRuntime.Namespace)
		if wsUID != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_WORKSPACE_UID",
				Value: wsUID,
			})
		}
	}

	// Check for mock provider annotation (for E2E testing)
	if mockProvider, ok := agentRuntime.Annotations[MockProviderAnnotation]; ok && mockProvider == envValueTrue {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_MOCK_PROVIDER",
			Value: envValueTrue,
		})
	}

	// Add tracing configuration if enabled
	if r.TracingEnabled && r.TracingEndpoint != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENABLED",
				Value: envValueTrue,
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENDPOINT",
				Value: r.TracingEndpoint,
			},
			// Use insecure connection for in-cluster communication
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_INSECURE",
				Value: envValueTrue,
			},
		)
	}

	// Skill manifest path. The runtime reads this on startup, parses the
	// manifest, and registers each entry via sdk.WithSkillsDir. Empty
	// when WorkspaceContentPath isn't configured on the reconciler.
	if path := r.skillManifestPath(agentRuntime.Spec.PromptPackRef.Name); path != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_PROMPTPACK_MANIFEST_PATH",
			Value: path,
		})
	}

	// OMNIA_CONTEXT_URL — the Redis URL used by the runtime's durable context
	// store (statestore.NewRedisStore). Sourced from the storeRef secret when a
	// Redis-backed context store is configured; omitted otherwise so the runtime
	// falls back to the in-process memory store.
	if agentRuntime.Spec.Context != nil &&
		agentRuntime.Spec.Context.Type == omniav1alpha1.ContextStoreTypeRedis &&
		agentRuntime.Spec.Context.StoreRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "OMNIA_CONTEXT_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *agentRuntime.Spec.Context.StoreRef,
					Key:                  secretKeyRedisURL,
				},
			},
		})
	}

	// Activate the runtime's PolicyBrokerClient (internal/runtime/tools) by
	// pointing it at the co-located policy-broker sidecar. The client is a
	// no-op unless POLICY_BROKER_URL is set, so this env var is the sole
	// activation switch — gated on the same PolicyBrokerImage condition that
	// injects the sidecar (see buildDeploymentSpec), so pods without the
	// sidecar never set an unreachable URL. FAIL_MODE=closed is stamped
	// explicitly to match the client's own default (fail-closed enforcement).
	if r.PolicyBrokerImage != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "POLICY_BROKER_URL",
				Value: fmt.Sprintf("http://localhost:%d", DefaultPolicyBrokerPort),
			},
			corev1.EnvVar{
				Name:  "POLICY_BROKER_FAIL_MODE",
				Value: "closed",
			},
		)
	}

	// Add extra env vars from CRD
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.ExtraEnv != nil {
		envVars = append(envVars, agentRuntime.Spec.Runtime.ExtraEnv...)
	}

	return envVars
}

// defaultImageForFramework returns the default container image for a framework type.
// resolveWorkspaceUIDForNamespace finds the Workspace CRD whose spec.namespace.name
// matches the given namespace and returns its UID.
func (r *AgentRuntimeReconciler) resolveWorkspaceUIDForNamespace(namespace string) string {
	if r.Client == nil {
		return ""
	}
	var list omniav1alpha1.WorkspaceList
	if err := r.List(context.Background(), &list); err != nil {
		return ""
	}
	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			return string(ws.UID)
		}
	}
	return ""
}
