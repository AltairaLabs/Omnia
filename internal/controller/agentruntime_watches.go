/*
Copyright 2026.

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
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SetupWithManager sets up the controller with the Manager.
//
// Lives apart from agentruntime_controller.go so the reconcile logic and the
// watch wiring stay separately readable (and the controller file stays under
// the file-length guardrail, #1325). The map functions these watches use are in
// watch_handlers.go.
func (r *AgentRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.gatewayAPIPresent = gatewayAPIAvailable(mgr.GetRESTMapper())

	b := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.AgentRuntime{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		// Watch Provider changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.Provider{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForProvider),
		).
		// Watch PromptPack changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.PromptPack{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForPromptPack),
		).
		// Watch ToolRegistry changes and reconcile AgentRuntimes that reference them.
		// Without this, an agent never recovers when its ToolRegistry appears/changes (#1491).
		Watches(
			&omniav1alpha1.ToolRegistry{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForToolRegistry),
		).
		// Watch Secret changes and reconcile AgentRuntimes that use them for credentials
		// This triggers pod rollouts when API keys are rotated
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForSecret),
		).
		// Watch Workspace changes and reconcile the AgentRuntimes in its namespace.
		// Without this, an agent reconciled before its Workspace exists never gets
		// its workspace-reader binding or OMNIA_WORKSPACE_NAME, and never recovers
		// (#1875) — the pod no longer discovers the workspace for itself.
		Watches(
			&omniav1alpha1.Workspace{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForWorkspace),
		)

	b = r.registerFacadeWatches(b)

	return b.Named("agentruntime").Complete(r)
}
