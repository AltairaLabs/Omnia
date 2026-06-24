/*
Copyright 2026 Altaira Labs.

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
	"maps"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// resolveWorkspaceRuntimeDefaults returns the RuntimeDefaults of the Workspace
// whose spec.namespace.name matches the given namespace, or nil when there is no
// such Workspace, it declares no runtime defaults, or the client is unset.
//
// This is what lets agents provisioned via the deploy API (which carry no
// cloud-specific SA) inherit the workspace's keyless-provider identity: the
// adapter never sets podOverrides, so without a workspace default the pod runs
// as the operator's per-agent SA with no cloud identity and every LLM call fails
// to acquire a token.
func (r *AgentRuntimeReconciler) resolveWorkspaceRuntimeDefaults(namespace string) *omniav1alpha1.RuntimeDefaults {
	if r.Client == nil {
		return nil
	}
	var list omniav1alpha1.WorkspaceList
	if err := r.List(context.Background(), &list); err != nil {
		return nil
	}
	for i := range list.Items {
		if list.Items[i].Spec.Namespace.Name == namespace {
			return list.Items[i].Spec.Runtime
		}
	}
	return nil
}

// agentOwnsIdentity reports whether the AgentRuntime sets its own pod
// ServiceAccount — in which case it is bringing its own cloud identity (its own
// annotated SA) and opts OUT of the workspace runtime defaults as a unit.
func agentOwnsIdentity(ar *omniav1alpha1.AgentRuntime) bool {
	return ar.Spec.PodOverrides != nil && ar.Spec.PodOverrides.ServiceAccountName != ""
}

// effectiveServiceAccountName returns the SA the agent runtime pod should run as:
//   - the agent's own podOverrides SA when set (it owns its identity), else
//   - the workspace runtime default SA when set, else
//   - the operator-created per-agent <name>-facade SA.
//
// The facade RBAC (Role/RoleBinding + workspace-reader ClusterRoleBinding) must
// target this same SA, or an agent inheriting the workspace SA never receives
// CRD-read permissions and service discovery is denied (cf. #1223).
func effectiveServiceAccountName(ar *omniav1alpha1.AgentRuntime, ws *omniav1alpha1.RuntimeDefaults) string {
	if agentOwnsIdentity(ar) {
		return ar.Spec.PodOverrides.ServiceAccountName
	}
	if ws != nil && ws.ServiceAccountName != "" {
		return ws.ServiceAccountName
	}
	return facadeServiceAccountName(ar)
}

// effectiveFacadeServiceAccountName resolves the workspace defaults for the
// agent's namespace and returns the SA the pod actually runs as. Method form so
// the RBAC reconcilers bind the right SA without threading the Workspace through.
func (r *AgentRuntimeReconciler) effectiveFacadeServiceAccountName(ar *omniav1alpha1.AgentRuntime) string {
	return effectiveServiceAccountName(ar, r.resolveWorkspaceRuntimeDefaults(ar.Namespace))
}

// effectivePodOverrides combines the workspace runtime defaults with the agent's
// own podOverrides into the single PodOverrides the deployment builder applies.
//
//   - agent owns its identity (sets its own SA), or no workspace defaults exist:
//     the agent's own overrides are returned unchanged — no workspace identity is
//     layered on (the SA + its identity labels move as a unit).
//   - otherwise: the workspace SA, podLabels and podAnnotations are layered
//     UNDER the agent's own overrides (agent values win per-key).
func effectivePodOverrides(ar *omniav1alpha1.AgentRuntime, ws *omniav1alpha1.RuntimeDefaults) *omniav1alpha1.PodOverrides {
	agent := ar.Spec.PodOverrides
	if ws == nil || agentOwnsIdentity(ar) ||
		(ws.ServiceAccountName == "" && len(ws.PodLabels) == 0 && len(ws.PodAnnotations) == 0) {
		return agent
	}

	var eff omniav1alpha1.PodOverrides
	if agent != nil {
		eff = *agent.DeepCopy()
	}
	if ws.ServiceAccountName != "" {
		eff.ServiceAccountName = ws.ServiceAccountName
	}
	eff.Labels = mergeAgentWins(ws.PodLabels, eff.Labels)
	eff.Annotations = mergeAgentWins(ws.PodAnnotations, eff.Annotations)
	return &eff
}

// effectivePodOverridesForAgent resolves the workspace defaults for the agent's
// namespace and returns the merged pod-level overrides to apply.
func (r *AgentRuntimeReconciler) effectivePodOverridesForAgent(ar *omniav1alpha1.AgentRuntime) *omniav1alpha1.PodOverrides {
	return effectivePodOverrides(ar, r.resolveWorkspaceRuntimeDefaults(ar.Namespace))
}

// mergeAgentWins returns base ∪ agent where the agent's keys win on collision.
// Returns nil when both are empty so we don't attach an empty map.
func mergeAgentWins(base, agent map[string]string) map[string]string {
	if len(base) == 0 && len(agent) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(agent))
	maps.Copy(out, base)
	maps.Copy(out, agent)
	return out
}
