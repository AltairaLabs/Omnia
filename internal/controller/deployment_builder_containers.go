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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// buildFacadeContainer creates the facade container spec.
func (r *AgentRuntimeReconciler) buildFacadeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	facadePort int32,
) corev1.Container {
	// Check for CRD image override first, then operator default, then hardcoded default
	facadeImage := ""
	if f := primaryFacade(agentRuntime); f != nil && f.Image != "" {
		facadeImage = f.Image
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
				Name:          portNameFacade,
				ContainerPort: facadePort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				// Serves /healthz, /readyz AND /metrics. Named "metrics" so
				// pod service-discovery finds it by the fleet-wide port-name
				// contract (memory-api/eval-worker use the same name). Probes
				// reference this port by number, so the name is free to be the
				// scrape contract.
				Name:          metricsPortName,
				ContainerPort: DefaultFacadeHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          r.buildFacadeEnvVars(agentRuntime),
		VolumeMounts: r.buildFacadeVolumeMounts(promptPack),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: readyzPath,
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
	if f := primaryFacade(agentRuntime); f != nil && f.Image != "" {
		facadeImage = f.Image
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
				Name:          portNameFacade,
				ContainerPort: facadePort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          r.buildA2AEnvVars(agentRuntime, resolvedClients),
		VolumeMounts: r.buildRuntimeVolumeMounts(agentRuntime, promptPack, toolRegistry),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: readyzPath,
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

// buildRuntimeContainer creates the runtime container spec.
// promptPack is only needed for volume mounts (the pack file mount path).
func (r *AgentRuntimeReconciler) buildRuntimeContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
) corev1.Container {
	// Resolve the runtime image by framework type. The reconcile validated
	// resolvability (FrameworkReady) before building the Deployment, so ok is
	// true here; the blank fallback is defensive only.
	frameworkImage, _ := r.resolveFrameworkImage(agentRuntime)

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
				// Serves /healthz, /readyz AND /metrics. Named "metrics" for the
				// same port-name discovery contract as the facade container.
				Name:          metricsPortName,
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
