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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/podoverrides"
)

// Annotation key for config hash - changes to this trigger pod rollouts
const annotationConfigHash = "omnia.altairalabs.ai/config-hash"

// drainGraceBufferSeconds is added to the DrainTimeout to give the pre-stop
// hook and OS process cleanup headroom after the drain window closes.
const drainGraceBufferSeconds = 15

// gracePeriodFor returns the pod TerminationGracePeriodSeconds for ar.
// When ar.Spec.Facade.DrainTimeout is set to a parseable positive duration,
// the grace period is that duration plus drainGraceBufferSeconds.
// Otherwise the default of 45 seconds is returned.
func gracePeriodFor(ar *omniav1alpha1.AgentRuntime) int64 {
	if ar.Spec.Facade.DrainTimeout != nil {
		if d, err := time.ParseDuration(*ar.Spec.Facade.DrainTimeout); err == nil && d > 0 {
			return int64(d.Seconds()) + drainGraceBufferSeconds
		}
	}
	return 45
}

// agentPodUserID is the uid/gid used by the facade and runtime container images
// (both Dockerfile.agent and Dockerfile.runtime declare USER 65532:65532 on a
// scratch base). Reflecting it in the pod SecurityContext lets PodSecurity
// admission enforce runAsNonRoot and makes fsGroup ownership of mounted
// volumes explicit.
const agentPodUserID int64 = 65532

func (r *AgentRuntimeReconciler) reconcileDeployment(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
) (*appsv1.Deployment, error) {
	log := logf.FromContext(ctx)

	// Calculate config hash for rollout triggering
	configHash := r.getConfigHash(ctx, providers)

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

		// Capture replicas before the builder overwrites them: while a
		// replica-weighted rollout is active, reconcileReplicaWeighting owns
		// .spec.replicas and the builder must not reset it to the canonical total.
		liveReplicas := deployment.Spec.Replicas

		// Build deployment spec
		r.buildDeploymentSpec(ctx, deployment, agentRuntime, promptPack, toolRegistry, configHash, resolvedClients)
		r.preserveWeightedReplicas(ctx, agentRuntime, deployment, liveReplicas)
		return nil
	})

	if err != nil {
		return nil, err
	}

	log.Info("Deployment reconciled", "result", result)
	return deployment, nil
}

func (r *AgentRuntimeReconciler) buildDeploymentSpec(
	ctx context.Context,
	deployment *appsv1.Deployment,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	configHash string,
	resolvedClients []ResolvedA2AClient,
) {
	log := logf.FromContext(ctx)
	// selectorLabels is the immutable subset used for the Deployment's
	// Spec.Selector.MatchLabels (and any Service / PDB / HPA selector).
	// Kubernetes rejects mutations to selector labels after creation, so
	// any value that may evolve (e.g. spec.mode) must live in podLabels
	// only.
	selectorLabels := map[string]string{
		labelAppName:      labelValueOmniaAgent,
		labelAppInstance:  agentRuntime.Name,
		labelAppManagedBy: labelValueOmniaOperator,
		labelOmniaComp:    "agent",
		labelOmniaTrack:   "stable",
	}
	// podLabels = selectorLabels ∪ mutable observability labels. Mode
	// goes here so `kubectl get pods -l omnia.altairalabs.ai/mode=function`
	// works without breaking selector immutability.
	labels := make(map[string]string, len(selectorLabels)+1)
	for k, v := range selectorLabels {
		labels[k] = v
	}
	labels[labelOmniaMode] = string(agentRuntime.EffectiveMode())

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
		facadeContainer := r.buildFacadeContainer(agentRuntime, promptPack, facadePort)

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

		// MCP: add port + env when enabled (function-mode only — CEL enforces).
		applyMCPFacadeOptions(&facadeContainer, agentRuntime)

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
		SecurityContext:    hardenedPodSecurityContext(),
	}

	// Apply hardened container SecurityContext to facade + runtime. The
	// policy-proxy sidecar (injected separately by buildPolicyProxyContainer)
	// sets its own SecurityContext and is skipped here.
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == PolicyProxyContainerName {
			continue
		}
		podSpec.Containers[i].SecurityContext = hardenedContainerSecurityContext()
	}

	// Internal service auth (SEC-1/SEC-5): the facade writes sessions to
	// session-api via httpclient, which reads its bearer token from
	// SESSION_API_TOKEN_PATH. When enabled, mount an audience-bound projected
	// SA token and point the path env at it. No-op when disabled.
	r.ServiceAuth.applyCallerToken(&podSpec)

	// Termination grace period: computed from DrainTimeout when set, otherwise
	// 45s (allows the 30s shutdown timeout plus headroom for the pre-stop hook
	// and connection draining).
	podSpec.TerminationGracePeriodSeconds = ptr.To(gracePeriodFor(agentRuntime))

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
					MatchLabels: selectorLabels,
				},
			},
		}
	}

	// Metrics discovery deliberately does NOT use the single-valued
	// prometheus.io/port annotation. An agent pod serves Prometheus metrics on
	// TWO ports across TWO containers — the facade on its health port (8081)
	// and the runtime on its health port (9001) — and there is no in-pod
	// consolidation of the two. A single prometheus.io/port cannot express
	// both, and the old prometheus.istio.io/merge-metrics assumption only ever
	// worked behind an Istio SIDECAR (it merges one app port + Envoy stats onto
	// :15020 and never covered the runtime's 9001); under ambient mesh or no
	// mesh it resolves to nothing. Instead both containers declare a container
	// port NAMED "metrics" (see below), and scrapers discover every metrics
	// endpoint by that port name via Kubernetes pod service-discovery — the
	// bundled Prometheus "omnia-agents" job and the optional PodMonitor both
	// key on it. This survives swapping the facade/runtime implementation since
	// the contract is the port NAME, not its number.
	//
	// traffic.sidecar.istio.io/excludeInboundPorts lists BOTH metrics ports so
	// that on a sidecar deployment Prometheus can scrape them directly without
	// mTLS (ambient/no-mesh ignore the annotation harmlessly).
	podAnnotations := map[string]string{
		"traffic.sidecar.istio.io/excludeInboundPorts": fmt.Sprintf("%d,%d", DefaultFacadeHealthPort, DefaultRuntimeHealthPort),
	}

	// Add config hash annotation to trigger rollouts when config changes
	if configHash != "" {
		podAnnotations[annotationConfigHash] = configHash
	}

	// Add extra pod annotations from CRD
	for key, value := range agentRuntime.Spec.ExtraPodAnnotations {
		podAnnotations[key] = value
	}

	// Apply user-supplied PodOverrides. Pod-level fields merge onto podSpec +
	// podAnnotations here; container-level fields are applied per-container
	// below to exclude the operator-injected policy-proxy sidecar.
	if agentRuntime.Spec.PodOverrides != nil {
		podMeta := metav1.ObjectMeta{Labels: labels, Annotations: podAnnotations}
		podoverrides.ApplyPod(&podSpec, &podMeta, agentRuntime.Spec.PodOverrides)
		labels = podMeta.Labels
		podAnnotations = podMeta.Annotations

		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name == PolicyProxyContainerName {
				continue
			}
			podoverrides.ApplyContainer(&podSpec.Containers[i], agentRuntime.Spec.PodOverrides)
		}
	}

	deployment.Labels = labels
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		// Selector uses ONLY the immutable label subset — adding labels
		// here later breaks reconcile with `field is immutable`.
		Selector: &metav1.LabelSelector{
			MatchLabels: selectorLabels,
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
		{
			// Rollout-semantic variant the facade records on each session when
			// the x-omnia-variant request header is absent (replica-weighted
			// mode has no routing layer to set it). The candidate Deployment
			// overrides this to variantCandidate (#1449).
			Name:  envFacadeVariant,
			Value: variantStable,
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
