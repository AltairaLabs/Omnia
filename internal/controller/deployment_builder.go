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
		r.buildDeploymentSpec(deployment, agentRuntime, promptPack, toolRegistry, providers, secretHash)
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
		if provider.Spec.SecretRef != nil {
			r.hashSecretData(ctx, hasher, provider.Spec.SecretRef.Name, provider.Namespace)
		}
	}

	// Include inline provider secret if present (legacy)
	if agentRuntime.Spec.Provider != nil && agentRuntime.Spec.Provider.SecretRef != nil {
		r.hashSecretData(ctx, hasher, agentRuntime.Spec.Provider.SecretRef.Name, agentRuntime.Namespace)
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
	deployment *appsv1.Deployment,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
	secretHash string,
) {
	labels := map[string]string{
		labelAppName:      labelValueOmniaAgent,
		labelAppInstance:  agentRuntime.Name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    "agent",
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

	// Build facade container
	facadeContainer := r.buildFacadeContainer(agentRuntime, facadePort)

	// Build runtime container — select primary provider for runtime env vars
	primaryProvider := primaryProviderFromMap(providers)
	runtimeContainer := r.buildRuntimeContainer(agentRuntime, promptPack, toolRegistry, primaryProvider)

	// Build pod spec with both containers
	podSpec := corev1.PodSpec{
		ServiceAccountName: facadeServiceAccountName(agentRuntime),
		Containers:         []corev1.Container{facadeContainer, runtimeContainer},
		Volumes:            volumes,
	}

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
	}

	return container
}

// buildRuntimeContainer creates the runtime container spec.
func (r *AgentRuntimeReconciler) buildRuntimeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
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
		Env:          r.buildRuntimeEnvVars(agentRuntime, promptPack, toolRegistry, provider),
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
					FieldPath: "metadata.labels['app.kubernetes.io/instance']",
				},
			},
		},
		{
			Name: "OMNIA_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
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

	// Add session config (facade needs this for session management)
	envVars = append(envVars, buildSessionEnvVars(agentRuntime.Spec.Session, "OMNIA_SESSION_STORE_URL")...)

	// Inject session-api URL so the facade uses httpclient.Store for session recording
	if r.SessionAPIURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SESSION_API_URL",
			Value: r.SessionAPIURL,
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

	// Add eval env vars for PromptKit agents with evals enabled
	if hasEvalsEnabled(&agentRuntime.Spec) && isPromptKit(&agentRuntime.Spec) {
		envVars = append(envVars, buildEvalEnvVars(agentRuntime.Spec.Evals)...)
	}

	// Add extra env vars from CRD
	if agentRuntime.Spec.Facade.ExtraEnv != nil {
		envVars = append(envVars, agentRuntime.Spec.Facade.ExtraEnv...)
	}

	return envVars
}

// buildRuntimeEnvVars creates environment variables for the runtime container.
func (r *AgentRuntimeReconciler) buildRuntimeEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	provider *omniav1alpha1.Provider,
) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "OMNIA_AGENT_NAME",
			Value: agentRuntime.Name,
		},
		{
			Name:  "OMNIA_NAMESPACE",
			Value: agentRuntime.Namespace,
		},
		{
			Name:  "OMNIA_PROMPTPACK_NAME",
			Value: promptPack.Name,
		},
		{
			Name:  "OMNIA_PROMPTPACK_NAMESPACE",
			Value: promptPack.Namespace,
		},
		{
			Name:  "OMNIA_PROMPTPACK_VERSION",
			Value: promptPack.Spec.Version,
		},
		// PromptPack path for the runtime to load
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

	// Add provider configuration
	// Provider CRD takes precedence over inline provider config
	if provider != nil {
		envVars = append(envVars, buildProviderEnvVarsFromCRD(provider)...)
	} else {
		envVars = append(envVars, buildProviderEnvVars(agentRuntime.Spec.Provider)...)
	}

	// Add tool registry info if present
	if toolRegistry != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLREGISTRY_NAME",
			Value: toolRegistry.Name,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLREGISTRY_NAMESPACE",
			Value: toolRegistry.Namespace,
		})
		// Tools config path
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_TOOLS_CONFIG_PATH",
			Value: ToolsMountPath + "/" + ToolsConfigFileName,
		})
	}

	// Add session config for conversation persistence
	envVars = append(envVars, buildSessionEnvVars(agentRuntime.Spec.Session, "OMNIA_SESSION_URL")...)

	// Add media config for mock provider responses
	if agentRuntime.Spec.Media != nil && agentRuntime.Spec.Media.BasePath != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OMNIA_MEDIA_BASE_PATH",
			Value: agentRuntime.Spec.Media.BasePath,
		})
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

	// Enable real-time evals — promptPack is always non-nil when this function is called
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_EVAL_ENABLED",
		Value: "true",
	})

	// Add extra env vars from CRD
	if agentRuntime.Spec.Runtime != nil && agentRuntime.Spec.Runtime.ExtraEnv != nil {
		envVars = append(envVars, agentRuntime.Spec.Runtime.ExtraEnv...)
	}

	return envVars
}

// defaultImageForFramework returns the default container image for a framework type.
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

// primaryProviderFromMap selects the primary provider from a providers map.
// The "default" key is preferred; otherwise the first entry in sorted key order is used.
func primaryProviderFromMap(providers map[string]*omniav1alpha1.Provider) *omniav1alpha1.Provider {
	if len(providers) == 0 {
		return nil
	}
	if p, ok := providers["default"]; ok {
		return p
	}
	// Fall back to first key in sorted order
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return providers[names[0]]
}
