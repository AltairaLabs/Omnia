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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Annotation key for secret hash - changes to this trigger pod rollouts
const annotationSecretHash = "omnia.altairalabs.ai/secret-hash"

func (r *AgentRuntimeReconciler) reconcileDeployment(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
) (*appsv1.Deployment, error) {
	log := logf.FromContext(ctx)

	// Calculate secret hash for rollout triggering
	secretHash := r.getSecretHash(ctx, agentRuntime, providers)

	// Resolve A2A clients for env injection.
	resolvedClients, _ := r.resolveA2AClients(ctx, log, agentRuntime)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, deployment, r.Scheme); err != nil {
			return err
		}

		// Build deployment spec
		r.buildDeploymentSpec(ctx, deployment, agentRuntime, promptPack, toolRegistry, secretHash, resolvedClients)
		return nil
	})

	if err != nil {
		return nil, err
	}

	log.Info("Deployment reconciled", "result", result)
	return deployment, nil
}

// getSecretHash calculates a hash of all secrets referenced by the agent.
// This is used to trigger pod rollouts when secrets change.
func (r *AgentRuntimeReconciler) getSecretHash(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	providers map[string]*omniav1alpha1.Provider,
) string {
	hasher := sha256.New()

	// Include all providers' secrets in sorted key order for determinism
	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, name := range providerNames {
		provider := providers[name]
		if ref := effectiveSecretRef(provider); ref != nil {
			r.hashSecretData(ctx, hasher, ref.Name, provider.Namespace)
		}
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))
	// Use first 16 chars for brevity
	if len(hashStr) > 16 {
		hashStr = hashStr[:16]
	}
	return hashStr
}

// hashSecretData reads a secret and writes its data to the hasher in deterministic order.
func (r *AgentRuntimeReconciler) hashSecretData(ctx context.Context, hasher hash.Hash, secretName, namespace string) {
	log := logf.FromContext(ctx)
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
	if err := r.Get(ctx, secretKey, secret); err == nil {
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hasher.Write([]byte(k))
			hasher.Write(secret.Data[k])
		}
		log.V(1).Info("Included secret in hash", "secret", secretKey.String())
	} else {
		log.V(1).Info("Could not get secret for hash", "secret", secretKey.String(), "error", err)
	}
}

func (r *AgentRuntimeReconciler) buildDeploymentSpec(
	ctx context.Context,
	deployment *appsv1.Deployment,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	secretHash string,
	resolvedClients []ResolvedA2AClient,
) {
	log := logf.FromContext(ctx)
	labels := map[string]string{
		labelAppName:      labelValueOmniaAgent,
		labelAppInstance:  agentRuntime.Name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    "agent",
		labelOmniaTrack:   "stable",
	}

	replicas := int32(1)
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Replicas != nil {
		replicas = *agentRuntime.Spec.Runtime.Replicas
	}

	facadePort := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		facadePort = *agentRuntime.Spec.Facade.Port
	}

	// Build volumes (shared between containers)
	volumes := r.buildVolumes(agentRuntime, promptPack, toolRegistry)

	// A2A facade runs the SDK in-process (single container), while WebSocket/gRPC
	// uses the traditional facade + runtime sidecar architecture.
	var containers []corev1.Container
	if agentRuntime.Spec.Facade.Type == omniav1alpha1.FacadeTypeA2A {
		a2aContainer := r.buildA2AContainer(agentRuntime, promptPack, toolRegistry, facadePort, resolvedClients)
		containers = []corev1.Container{a2aContainer}
	} else {
		facadeContainer := r.buildFacadeContainer(agentRuntime, facadePort)

		// Dual-protocol: add A2A port and env vars to the facade container.
		if isDualProtocol(agentRuntime) {
			a2aPort := int32(DefaultA2APort)
			if agentRuntime.Spec.A2A.Port != nil {
				a2aPort = *agentRuntime.Spec.A2A.Port
			}
			facadeContainer.Ports = append(facadeContainer.Ports, corev1.ContainerPort{
				Name:          "a2a",
				ContainerPort: a2aPort,
				Protocol:      corev1.ProtocolTCP,
			})
			facadeContainer.Env = append(facadeContainer.Env,
				corev1.EnvVar{Name: "OMNIA_A2A_ENABLED", Value: "true"},
				corev1.EnvVar{Name: "OMNIA_A2A_PORT", Value: fmt.Sprintf("%d", a2aPort)},
			)
			facadeContainer.Env = append(facadeContainer.Env, r.buildA2ADualProtocolEnvVars(agentRuntime)...)
		}

		runtimeContainer := r.buildRuntimeContainer(agentRuntime, promptPack, toolRegistry)
		containers = []corev1.Container{facadeContainer, runtimeContainer}
	}

	// Inject policy-proxy sidecar when enterprise edition is enabled.
	// The sidecar intercepts tool calls and evaluates ToolPolicy CEL rules
	// before they reach the runtime. PolicyProxyImage is only set when
	// the --enterprise flag is active.
	if r.PolicyProxyImage != "" {
		policyContainer := buildPolicyProxyContainer(agentRuntime, r.PolicyProxyImage)
		containers = append(containers, policyContainer)
		log.Info("injecting policy-proxy sidecar", "agent", agentRuntime.Name)
	}

	// Build pod spec
	podSpec := corev1.PodSpec{
		ServiceAccountName: facadeServiceAccountName(agentRuntime),
		Containers:         containers,
		Volumes:            volumes,
	}

	// Termination grace period: 45s allows the 30s shutdown timeout to complete
	// plus headroom for the pre-stop hook and connection draining.
	podSpec.TerminationGracePeriodSeconds = ptr.To(int64(45))

	// Add scheduling constraints if specified
	if agentRuntime.Spec.Runtime != nil {
		if agentRuntime.Spec.Runtime.NodeSelector != nil {
			podSpec.NodeSelector = agentRuntime.Spec.Runtime.NodeSelector
		}
		if agentRuntime.Spec.Runtime.Tolerations != nil {
			podSpec.Tolerations = agentRuntime.Spec.Runtime.Tolerations
		}
		if agentRuntime.Spec.Runtime.Affinity != nil {
			podSpec.Affinity = agentRuntime.Spec.Runtime.Affinity
		}
	}

	// Default topology spread: distribute agent pods across zones when replicas > 1.
	// Users can override via CRD affinity rules.
	if replicas > 1 && podSpec.Affinity == nil {
		podSpec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{
			{
				MaxSkew:           1,
				TopologyKey:       "topology.kubernetes.io/zone",
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		}
	}

	// Prometheus scrape annotations for metrics collection
	// - prometheus.io/* annotations tell Prometheus where to scrape (non-Istio pods use these directly)
	// - prometheus.istio.io/merge-metrics tells Istio to merge app metrics with Envoy stats
	//   Istio reads prometheus.io/port and prometheus.io/path BEFORE overwriting them,
	//   then merges app metrics into port 15020 alongside Envoy metrics
	// - traffic.sidecar.istio.io/excludeInboundPorts excludes runtime metrics port from Istio
	//   so Prometheus can directly scrape port 9001 without mTLS
	podAnnotations := map[string]string{
		"prometheus.io/scrape":                         "true",
		"prometheus.io/port":                           fmt.Sprintf("%d", facadePort),
		"prometheus.io/path":                           "/metrics",
		"prometheus.istio.io/merge-metrics":            "true",
		"traffic.sidecar.istio.io/excludeInboundPorts": fmt.Sprintf("%d", DefaultRuntimeHealthPort),
	}

	// Add secret hash annotation to trigger rollouts when secrets change
	if secretHash != "" {
		podAnnotations[annotationSecretHash] = secretHash
	}

	// Add extra pod annotations from CRD
	for key, value := range agentRuntime.Spec.ExtraPodAnnotations {
		podAnnotations[key] = value
	}

	deployment.Labels = labels
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: podAnnotations,
			},
			Spec: podSpec,
		},
	}
}

// buildFacadeContainer creates the facade container spec.
func (r *AgentRuntimeReconciler) buildFacadeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	facadePort int32,
) corev1.Container {
	// Check for CRD image override first, then operator default, then hardcoded default
	facadeImage := ""
	if agentRuntime.Spec.Facade.Image != "" {
		facadeImage = agentRuntime.Spec.Facade.Image
	} else if r.FacadeImage != "" {
		facadeImage = r.FacadeImage
	} else {
		facadeImage = DefaultFacadeImage
	}

	// Use configured pull policy, or default to IfNotPresent
	pullPolicy := r.FacadeImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}

	container := corev1.Container{
		Name:            FacadeContainerName,
		Image:           facadeImage,
		ImagePullPolicy: pullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "facade",
				ContainerPort: facadePort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "facade-health",
				ContainerPort: DefaultFacadeHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: r.buildFacadeEnvVars(agentRuntime),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(DefaultFacadeHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(DefaultFacadeHealthPort),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
		// Pre-stop hook: sleep 5s to let the load balancer stop routing traffic
		// before SIGTERM triggers the 30s graceful shutdown in the facade process.
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "sleep 5"},
				},
			},
		},
	}

	return container
}

// buildA2AContainer creates a single container that combines the facade and runtime
// for A2A protocol agents. The SDK runs in-process — no runtime sidecar needed.
func (r *AgentRuntimeReconciler) buildA2AContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	facadePort int32,
	resolvedClients []ResolvedA2AClient,
) corev1.Container {
	// A2A uses the facade image (which includes the SDK)
	facadeImage := ""
	if agentRuntime.Spec.Facade.Image != "" {
		facadeImage = agentRuntime.Spec.Facade.Image
	} else if r.FacadeImage != "" {
		facadeImage = r.FacadeImage
	} else {
		facadeImage = DefaultFacadeImage
	}

	pullPolicy := r.FacadeImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}

	container := corev1.Container{
		Name:            FacadeContainerName,
		Image:           facadeImage,
		ImagePullPolicy: pullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "facade",
				ContainerPort: facadePort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          r.buildA2AEnvVars(agentRuntime, resolvedClients),
		VolumeMounts: r.buildRuntimeVolumeMounts(agentRuntime, promptPack, toolRegistry),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(facadePort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(facadePort),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "sleep 5"},
				},
			},
		},
	}

	// Add resources if specified
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Resources != nil {
		container.Resources = *agentRuntime.Spec.Runtime.Resources
	}

	return container
}

// buildA2AEnvVars creates environment variables for the A2A container.
func (r *AgentRuntimeReconciler) buildA2AEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	resolvedClients []ResolvedA2AClient,
) []corev1.EnvVar {
	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

	envVars := []corev1.EnvVar{
		{
			Name: "OMNIA_AGENT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name: "OMNIA_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathNamespace,
				},
			},
		},
		{
			Name:  "OMNIA_FACADE_TYPE",
			Value: string(omniav1alpha1.FacadeTypeA2A),
		},
		{
			Name:  "OMNIA_FACADE_PORT",
			Value: fmt.Sprintf("%d", port),
		},
		{
			Name:  "OMNIA_PROMPTPACK_PATH",
			Value: PromptPackMountPath,
		},
	}

	// Handler mode
	handlerMode := omniav1alpha1.HandlerModeRuntime
	if agentRuntime.Spec.Facade.Handler != nil {
		handlerMode = *agentRuntime.Spec.Facade.Handler
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_HANDLER_MODE",
		Value: string(handlerMode),
	})

	// A2A-specific config (TTLs, auth, task store).
	envVars = append(envVars, buildA2AConfigEnvVars(agentRuntime.Spec.A2A)...)

	// Tracing
	if r.TracingEnabled && r.TracingEndpoint != "" {
		envVars = append(envVars,
			corev1.EnvVar{Name: "OMNIA_TRACING_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "OMNIA_TRACING_ENDPOINT", Value: r.TracingEndpoint},
			corev1.EnvVar{Name: "OMNIA_TRACING_INSECURE", Value: "true"},
		)
	}

	// Resolved A2A clients (JSON-encoded list + per-client secret refs).
	envVars = append(envVars, buildA2AClientEnvVars(agentRuntime, resolvedClients)...)

	// Extra env vars from CRD
	if agentRuntime.Spec.Facade.ExtraEnv != nil {
		envVars = append(envVars, agentRuntime.Spec.Facade.ExtraEnv...)
	}

	return envVars
}

// buildA2AConfigEnvVars creates env vars for A2A TTLs, auth, and task store config.
func buildA2AConfigEnvVars(a2a *omniav1alpha1.A2AConfig) []corev1.EnvVar {
	if a2a == nil {
		return nil
	}

	var envVars []corev1.EnvVar

	if a2a.TaskTTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_TASK_TTL",
			Value: *a2a.TaskTTL,
		})
	}
	if a2a.ConversationTTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_CONVERSATION_TTL",
			Value: *a2a.ConversationTTL,
		})
	}
	if a2a.Authentication != nil && a2a.Authentication.SecretRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "OMNIA_A2A_AUTH_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *a2a.Authentication.SecretRef,
					Key:                  "token",
				},
			},
		})
	}
	if a2a.TaskStore != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_TASK_STORE_TYPE",
			Value: string(a2a.TaskStore.Type),
		})
		if a2a.TaskStore.RedisSecretRef != nil {
			// Secret ref takes precedence over plain-text URL.
			envVars = append(envVars, corev1.EnvVar{
				Name: "OMNIA_A2A_REDIS_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: *a2a.TaskStore.RedisSecretRef,
						Key:                  "url",
					},
				},
			})
		} else if a2a.TaskStore.RedisURL != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_A2A_REDIS_URL",
				Value: a2a.TaskStore.RedisURL,
			})
		}
	}

	return envVars
}

// buildA2AClientEnvVars creates env vars for resolved A2A clients and their auth secrets.
func buildA2AClientEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	resolvedClients []ResolvedA2AClient,
) []corev1.EnvVar {
	if len(resolvedClients) == 0 {
		return nil
	}

	var envVars []corev1.EnvVar

	clientsJSON, err := marshalA2AClients(resolvedClients)
	if err == nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_CLIENTS",
			Value: clientsJSON,
		})
	}

	// Per-client auth tokens from secrets.
	for _, rc := range resolvedClients {
		if rc.AuthTokenEnv == "" {
			continue
		}
		for _, cs := range agentRuntime.Spec.A2A.Clients {
			if cs.Name == rc.Name && cs.Authentication != nil && cs.Authentication.SecretRef != nil {
				envVars = append(envVars, corev1.EnvVar{
					Name: rc.AuthTokenEnv,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: *cs.Authentication.SecretRef,
							Key:                  "token",
						},
					},
				})
				break
			}
		}
	}

	return envVars
}

// buildRuntimeContainer creates the runtime container spec.
// promptPack is only needed for volume mounts (the pack file mount path).
func (r *AgentRuntimeReconciler) buildRuntimeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) corev1.Container {
	// Check for CRD image override first, then operator default, then framework-specific default
	frameworkImage := ""
	if agentRuntime.Spec.Framework != nil && agentRuntime.Spec.Framework.Image != "" {
		frameworkImage = agentRuntime.Spec.Framework.Image
	} else if r.FrameworkImage != "" {
		frameworkImage = r.FrameworkImage
	} else {
		frameworkImage = defaultImageForFramework(agentRuntime.Spec.Framework)
	}

	// Use configured pull policy, or default to IfNotPresent
	runtimePullPolicy := r.FrameworkImagePullPolicy
	if runtimePullPolicy == "" {
		runtimePullPolicy = corev1.PullIfNotPresent
	}

	container := corev1.Container{
		Name:            RuntimeContainerName,
		Image:           frameworkImage,
		ImagePullPolicy: runtimePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: DefaultRuntimeGRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "runtime-health",
				ContainerPort: DefaultRuntimeHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          r.buildRuntimeEnvVars(agentRuntime, toolRegistry),
		VolumeMounts: r.buildRuntimeVolumeMounts(agentRuntime, promptPack, toolRegistry),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(DefaultRuntimeHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(DefaultRuntimeHealthPort),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
		},
	}

	// Add resources if specified
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.Resources != nil {
		container.Resources = *agentRuntime.Spec.Runtime.Resources
	}

	return container
}

// buildFacadeEnvVars creates environment variables for the facade container.
func (r *AgentRuntimeReconciler) buildFacadeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
) []corev1.EnvVar {
	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

	envVars := []corev1.EnvVar{
		// Identity from Downward API — facade reads CRD directly using these
		{
			Name: "OMNIA_AGENT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name: "OMNIA_NAMESPACE",
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
	}

	// Determine handler mode - default to runtime if not specified
	handlerMode := omniav1alpha1.HandlerModeRuntime
	if agentRuntime.Spec.Facade.Handler != nil {
		handlerMode = *agentRuntime.Spec.Facade.Handler
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
				Value: "true",
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENDPOINT",
				Value: r.TracingEndpoint,
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_INSECURE",
				Value: "true",
			},
		)
	}

	// Add extra env vars from CRD
	if agentRuntime.Spec.Facade.ExtraEnv != nil {
		envVars = append(envVars, agentRuntime.Spec.Facade.ExtraEnv...)
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
			Name: "OMNIA_AGENT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name: "OMNIA_NAMESPACE",
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
			Value: "default",
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
	if mockProvider, ok := agentRuntime.Annotations[MockProviderAnnotation]; ok && mockProvider == "true" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_MOCK_PROVIDER",
			Value: "true",
		})
	}

	// Add tracing configuration if enabled
	if r.TracingEnabled && r.TracingEndpoint != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENABLED",
				Value: "true",
			},
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_ENDPOINT",
				Value: r.TracingEndpoint,
			},
			// Use insecure connection for in-cluster communication
			corev1.EnvVar{
				Name:  "OMNIA_TRACING_INSECURE",
				Value: "true",
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

func defaultImageForFramework(framework *omniav1alpha1.FrameworkConfig) string {
	if framework == nil {
		return DefaultFrameworkImage
	}

	switch framework.Type {
	case omniav1alpha1.FrameworkTypeLangChain:
		return DefaultLangChainImage
	case omniav1alpha1.FrameworkTypePromptKit:
		return DefaultFrameworkImage
	case omniav1alpha1.FrameworkTypeAutoGen:
		// AutoGen doesn't have a default image yet; use PromptKit as fallback
		// Users must specify an image override for this framework
		return DefaultFrameworkImage
	default:
		return DefaultFrameworkImage
	}
}

// isDualProtocol returns true when the AgentRuntime has A2A enabled as an
// additional endpoint alongside a non-A2A primary facade (websocket or grpc).
func isDualProtocol(ar *omniav1alpha1.AgentRuntime) bool {
	return ar.Spec.Facade.Type != omniav1alpha1.FacadeTypeA2A &&
		ar.Spec.A2A != nil &&
		ar.Spec.A2A.Enabled
}

// buildA2ADualProtocolEnvVars returns extra env vars needed when A2A runs
// alongside the primary facade. These are appended to the facade container's env.
func (r *AgentRuntimeReconciler) buildA2ADualProtocolEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	if agentRuntime.Spec.A2A == nil {
		return envVars
	}

	// A2A TTLs
	if agentRuntime.Spec.A2A.TaskTTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_TASK_TTL",
			Value: *agentRuntime.Spec.A2A.TaskTTL,
		})
	}
	if agentRuntime.Spec.A2A.ConversationTTL != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_CONVERSATION_TTL",
			Value: *agentRuntime.Spec.A2A.ConversationTTL,
		})
	}

	// Auth token from secret
	if agentRuntime.Spec.A2A.Authentication != nil && agentRuntime.Spec.A2A.Authentication.SecretRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "OMNIA_A2A_AUTH_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *agentRuntime.Spec.A2A.Authentication.SecretRef,
					Key:                  "token",
				},
			},
		})
	}

	// Task store configuration
	if agentRuntime.Spec.A2A.TaskStore != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_A2A_TASK_STORE_TYPE",
			Value: string(agentRuntime.Spec.A2A.TaskStore.Type),
		})
		if agentRuntime.Spec.A2A.TaskStore.RedisSecretRef != nil {
			// Secret ref takes precedence over plain-text URL.
			envVars = append(envVars, corev1.EnvVar{
				Name: "OMNIA_A2A_REDIS_URL",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: *agentRuntime.Spec.A2A.TaskStore.RedisSecretRef,
						Key:                  "url",
					},
				},
			})
		} else if agentRuntime.Spec.A2A.TaskStore.RedisURL != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "OMNIA_A2A_REDIS_URL",
				Value: agentRuntime.Spec.A2A.TaskStore.RedisURL,
			})
		}
	}

	return envVars
}
