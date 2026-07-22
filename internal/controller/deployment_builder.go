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

// drainGraceBufferSeconds is added to the effective drain timeout to size the
// pod's TerminationGracePeriodSeconds. It must cover the facade's post-drain
// graceful Shutdown (cmd/agent shutdownTimeout, 30s) plus OS process cleanup,
// so a pod that drains for its full window is not SIGKILLed mid-Shutdown.
const drainGraceBufferSeconds = 30

// defaultDrainTimeoutSeconds mirrors facade DefaultServerConfig().DrainTimeout
// (30s); used when the primary facade's drainTimeout is unset or unparseable.
const defaultDrainTimeoutSeconds = 30

// maxDrainTimeoutSeconds caps the drain window so a misconfigured large
// drainTimeout cannot stall rollout/teardown indefinitely (10 minutes).
const maxDrainTimeoutSeconds = 600

// gracePeriodFor returns the pod TerminationGracePeriodSeconds for ar: the
// effective drain timeout (the primary facade's drainTimeout, or the 30s
// default, clamped to maxDrainTimeoutSeconds) plus drainGraceBufferSeconds.
func gracePeriodFor(ar *omniav1alpha1.AgentRuntime) int64 {
	drainSecs := int64(defaultDrainTimeoutSeconds)
	if f := primaryFacade(ar); f != nil && f.DrainTimeout != nil {
		if d, err := time.ParseDuration(*f.DrainTimeout); err == nil && d >= time.Second {
			drainSecs = int64(d.Seconds())
		}
	}
	if drainSecs > maxDrainTimeoutSeconds {
		drainSecs = maxDrainTimeoutSeconds
	}
	return drainSecs + drainGraceBufferSeconds
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

	// Calculate config hash for rollout triggering — covers provider config +
	// secrets AND the PromptPack / ToolRegistry the runtime loads, so a tool or
	// prompt change actually rolls the pod instead of silently leaving it stale.
	configHash := r.getConfigHash(ctx, providers, promptPack, toolRegistry)

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
	if capabilitiesMismatchForCurrentGen(agentRuntime) {
		// The runtime lacks a required capability for this generation: stop
		// wasting resources on an agent that can't serve its facade. Fixing the
		// runtime image bumps the generation and lifts this (§4.4 / 3a-enforce).
		replicas = 0
	}

	facadePort := primaryFacadePort(agentRuntime)

	// Build volumes (shared between containers)
	volumes := r.buildVolumes(agentRuntime, promptPack, toolRegistry)

	// A standalone A2A facade runs the SDK in-process (single container), while
	// WebSocket/REST uses the traditional facade + runtime sidecar architecture.
	var containers []corev1.Container
	if isStandaloneA2A(agentRuntime) {
		a2aContainer := r.buildA2AContainer(agentRuntime, promptPack, toolRegistry, facadePort, resolvedClients)
		containers = []corev1.Container{a2aContainer}
	} else {
		facadeContainer := r.buildFacadeContainer(agentRuntime, promptPack, facadePort)

		// Dual-protocol: add A2A port and env vars to the facade container.
		if isDualProtocol(agentRuntime) {
			a2aPort := a2aSecondaryPort(agentRuntime)
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

	// Inject policy-broker sidecar when configured. The broker watches
	// ToolPolicy CRDs in the agent's namespace and serves CEL decisions to the
	// runtime's PolicyBrokerClient over localhost (see buildRuntimeEnvVars,
	// which points the runtime at it via POLICY_BROKER_URL).
	if r.PolicyBrokerImage != "" {
		brokerContainer := buildPolicyBrokerContainer(agentRuntime, r.PolicyBrokerImage, r.LicenseAPIURL)
		containers = append(containers, brokerContainer)
		log.Info("injecting policy-broker sidecar", "agent", agentRuntime.Name)
	}

	// Build pod spec
	podSpec := corev1.PodSpec{
		ServiceAccountName: facadeServiceAccountName(agentRuntime),
		Containers:         containers,
		Volumes:            volumes,
		SecurityContext:    hardenedPodSecurityContext(),
	}

	// Apply hardened container SecurityContext to facade + runtime. The
	// policy-broker sidecar (injected separately by buildPolicyBrokerContainer)
	// sets its own SecurityContext and is skipped here.
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == PolicyBrokerContainerName {
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
	// traffic.sidecar.istio.io/excludeInboundPorts lists all THREE metrics
	// ports (facade, runtime, and the policy-broker sidecar's health/metrics
	// port) so that on a sidecar deployment Prometheus can scrape them
	// directly without mTLS (ambient/no-mesh ignore the annotation
	// harmlessly). Listing the broker port here is harmless even on pods
	// without the sidecar (PolicyBrokerImage unset) since it's just an unused
	// port number in the exclusion list.
	podAnnotations := map[string]string{
		"traffic.sidecar.istio.io/excludeInboundPorts": fmt.Sprintf(
			"%d,%d,%d", DefaultFacadeHealthPort, DefaultRuntimeHealthPort, DefaultPolicyBrokerHealthPort,
		),
	}

	// Add config hash annotation to trigger rollouts when config changes
	if configHash != "" {
		podAnnotations[annotationConfigHash] = configHash
	}

	// Add extra pod annotations from CRD
	for key, value := range agentRuntime.Spec.ExtraPodAnnotations {
		podAnnotations[key] = value
	}

	// Apply pod-level PodOverrides. The Workspace runtime defaults (cloud
	// workload-identity SA + pod labels) are layered UNDER the agent's own
	// podOverrides, so agents provisioned via the deploy API — which carry no
	// podOverrides — still inherit the workspace's keyless-provider identity
	// (#1598). An agent that sets its own SA opts out of the workspace identity
	// as a unit. Container-level fields come only from the agent's own overrides
	// (the workspace supplies none) and are applied per-container below to
	// exclude the operator-injected policy-broker sidecar.
	if effOverrides := r.effectivePodOverridesForAgent(agentRuntime); effOverrides != nil {
		podMeta := metav1.ObjectMeta{Labels: labels, Annotations: podAnnotations}
		podoverrides.ApplyPod(&podSpec, &podMeta, effOverrides)
		labels = podMeta.Labels
		podAnnotations = podMeta.Annotations
	}
	if agentRuntime.Spec.PodOverrides != nil {
		for i := range podSpec.Containers {
			if podSpec.Containers[i].Name == PolicyBrokerContainerName {
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
	promptPack *omniav1alpha1.PromptPack,
	resolvedClients []ResolvedA2AClient,
) []corev1.EnvVar {
	port := primaryFacadePort(agentRuntime)

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
	if f := primaryFacade(agentRuntime); f != nil && f.Handler != nil {
		handlerMode = *f.Handler
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "OMNIA_HANDLER_MODE",
		Value: string(handlerMode),
	})

	// A2A-specific config (TTLs, task store).
	envVars = append(envVars, buildA2AConfigEnvVars(a2aConfig(agentRuntime))...)

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
	if f := primaryFacade(agentRuntime); f != nil && f.ExtraEnv != nil {
		envVars = append(envVars, f.ExtraEnv...)
	}

	// Standalone A2A combines facade+runtime and writes the session record, so it
	// needs the resolved PromptPack version stamp for track: agents too (#1847).
	envVars = r.appendPromptPackVersionEnv(envVars, promptPack)

	return envVars
}

// buildA2AConfigEnvVars creates env vars for A2A TTLs and task store config.
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
		a2a := a2aConfig(agentRuntime)
		if a2a == nil {
			continue
		}
		for _, cs := range a2a.Clients {
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

// buildA2ADualProtocolEnvVars returns extra env vars needed when A2A runs as a
// secondary listener alongside the primary websocket facade. These are appended
// to the facade container's env. Shares buildA2AConfigEnvVars with the
// standalone-A2A path so both emit the same TTL/task-store env.
func (r *AgentRuntimeReconciler) buildA2ADualProtocolEnvVars(
	agentRuntime *omniav1alpha1.AgentRuntime,
) []corev1.EnvVar {
	return buildA2AConfigEnvVars(a2aConfig(agentRuntime))
}
