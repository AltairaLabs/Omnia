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
	"k8s.io/utils/ptr"
)

// hardenedPodSecurityContext returns a restricted-profile-compliant PodSecurityContext
// for workspace agent pods (facade + runtime). Matches the controller and dashboard
// hardening so agent pods are not the soft spot in a restricted namespace.
func hardenedPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		RunAsUser:    ptr.To(agentPodUserID),
		RunAsGroup:   ptr.To(agentPodUserID),
		FSGroup:      ptr.To(agentPodUserID),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// hardenedContainerSecurityContext returns a restricted-profile-compliant
// container SecurityContext: no privilege escalation, read-only root, all
// capabilities dropped, seccomp RuntimeDefault. Applied to facade + runtime
// containers; the policy-proxy sidecar (injected separately) configures its
// own SecurityContext.
func hardenedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		RunAsNonRoot:             ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{capabilityAll},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
