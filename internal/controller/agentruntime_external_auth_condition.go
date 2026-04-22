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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// evaluateExternalAuthCondition returns the status, reason, and message
// for ConditionTypeExternalAuth based on spec.externalAuth.
//
// Status breakdown:
//   - True / DashboardOnly        — no externalAuth configured; mgmt-plane is the only admit path
//   - True / DataPlaneConfigured  — at least one data-plane validator is set
//   - False / Unreachable         — allowManagementPlane=false AND no data-plane validator — no caller can admit
//
// The reconciler sets this condition every pass so operators can
// `kubectl describe agentruntime` and immediately see whether the
// configuration they applied actually accepts traffic.
func evaluateExternalAuthCondition(ar *omniav1alpha1.AgentRuntime) metav1.Condition {
	ext := ar.Spec.ExternalAuth
	if ext == nil {
		return metav1.Condition{
			Type:    ConditionTypeExternalAuth,
			Status:  metav1.ConditionTrue,
			Reason:  "DashboardOnly",
			Message: "no spec.externalAuth configured; only dashboard-minted management-plane tokens admit",
		}
	}

	hasDataPlane := ext.SharedToken != nil ||
		ext.APIKeys != nil ||
		ext.OIDC != nil ||
		ext.EdgeTrust != nil

	// allowManagementPlane defaults to true per the CRD's +kubebuilder:default.
	// Treat nil as true so users who omit the field get the permissive default.
	mgmtPlaneAllowed := true
	if ext.AllowManagementPlane != nil {
		mgmtPlaneAllowed = *ext.AllowManagementPlane
	}

	if hasDataPlane {
		return metav1.Condition{
			Type:    ConditionTypeExternalAuth,
			Status:  metav1.ConditionTrue,
			Reason:  "DataPlaneConfigured",
			Message: externalAuthConfiguredMessage(ext, mgmtPlaneAllowed),
		}
	}

	if !mgmtPlaneAllowed {
		return metav1.Condition{
			Type:   ConditionTypeExternalAuth,
			Status: metav1.ConditionFalse,
			Reason: "Unreachable",
			Message: "spec.externalAuth.allowManagementPlane=false but no data-plane validator " +
				"(sharedToken / apiKeys / oidc / edgeTrust) is configured — the facade will reject every request",
		}
	}

	// externalAuth block is set but empty — equivalent to DashboardOnly for
	// admit purposes but distinct state worth surfacing so operators know
	// the block exists.
	return metav1.Condition{
		Type:    ConditionTypeExternalAuth,
		Status:  metav1.ConditionTrue,
		Reason:  "DashboardOnly",
		Message: "spec.externalAuth is set but has no data-plane validator; only management-plane tokens admit",
	}
}

// externalAuthConfiguredMessage summarises which validators are
// configured. Short and grep-friendly — structured-logging friendly.
func externalAuthConfiguredMessage(ext *omniav1alpha1.AgentExternalAuth, mgmtPlaneAllowed bool) string {
	parts := []string{}
	if ext.SharedToken != nil {
		parts = append(parts, "sharedToken")
	}
	if ext.APIKeys != nil {
		parts = append(parts, "apiKeys")
	}
	if ext.OIDC != nil {
		parts = append(parts, "oidc")
	}
	if ext.EdgeTrust != nil {
		parts = append(parts, "edgeTrust")
	}
	if mgmtPlaneAllowed {
		parts = append(parts, "managementPlane")
	}
	return "facade admits: " + joinWithComma(parts)
}

// joinWithComma is strings.Join with ", " but avoids pulling in the
// strings import for a single-purpose helper in a small file.
func joinWithComma(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	out := items[0]
	for _, s := range items[1:] {
		out += ", " + s
	}
	return out
}
