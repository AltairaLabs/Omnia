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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func ptrBool(b bool) *bool { return &b }

func arWithExternalAuth(ext *omniav1alpha1.AgentExternalAuth) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{ExternalAuth: ext},
	}
}

func TestEvaluateExternalAuthCondition_NoExternalAuth(t *testing.T) {
	t.Parallel()
	cond := evaluateExternalAuthCondition(arWithExternalAuth(nil))
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("status = %v, want True", cond.Status)
	}
	if cond.Reason != "DashboardOnly" {
		t.Errorf("reason = %q, want DashboardOnly", cond.Reason)
	}
}

func TestEvaluateExternalAuthCondition_EmptyExternalAuth(t *testing.T) {
	// externalAuth block set but no validator populated — still admits
	// via management-plane, emit DashboardOnly with a hint that the
	// block is set so operators aren't surprised.
	t.Parallel()
	cond := evaluateExternalAuthCondition(arWithExternalAuth(&omniav1alpha1.AgentExternalAuth{}))
	if cond.Status != metav1.ConditionTrue || cond.Reason != "DashboardOnly" {
		t.Errorf("status=%v reason=%q, want True/DashboardOnly", cond.Status, cond.Reason)
	}
	if !strings.Contains(cond.Message, "no data-plane validator") {
		t.Errorf("message = %q, want hint about empty block", cond.Message)
	}
}

func TestEvaluateExternalAuthCondition_DataPlaneConfigured(t *testing.T) {
	t.Parallel()
	cases := map[string]*omniav1alpha1.AgentExternalAuth{
		"sharedToken": {SharedToken: &omniav1alpha1.SharedTokenAuth{SecretRef: corev1.LocalObjectReference{Name: "t"}}},
		"apiKeys":     {APIKeys: &omniav1alpha1.APIKeysAuth{}},
		"oidc":        {OIDC: &omniav1alpha1.OIDCAuth{Issuer: "x", Audience: "y"}},
		"edgeTrust":   {EdgeTrust: &omniav1alpha1.EdgeTrustAuth{}},
	}
	for name, ext := range cases {
		t.Run(name, func(t *testing.T) {
			cond := evaluateExternalAuthCondition(arWithExternalAuth(ext))
			if cond.Status != metav1.ConditionTrue || cond.Reason != "DataPlaneConfigured" {
				t.Errorf("status=%v reason=%q, want True/DataPlaneConfigured", cond.Status, cond.Reason)
			}
			if !strings.Contains(cond.Message, name) {
				t.Errorf("message = %q should mention %q", cond.Message, name)
			}
		})
	}
}

// TestEvaluateExternalAuthCondition_Unreachable proves T11 surfaces
// the "dark agent" foot-gun: allowManagementPlane=false AND no
// data-plane validator means the facade 401s every request. Operators
// need to see this as a False condition on kubectl describe.
func TestEvaluateExternalAuthCondition_Unreachable(t *testing.T) {
	t.Parallel()
	ext := &omniav1alpha1.AgentExternalAuth{AllowManagementPlane: ptrBool(false)}
	cond := evaluateExternalAuthCondition(arWithExternalAuth(ext))
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("status = %v, want False", cond.Status)
	}
	if cond.Reason != "Unreachable" {
		t.Errorf("reason = %q, want Unreachable", cond.Reason)
	}
	if !strings.Contains(cond.Message, "reject every request") {
		t.Errorf("message = %q should call out the effect", cond.Message)
	}
}

func TestEvaluateExternalAuthCondition_AllowManagementPlanePointerNil(t *testing.T) {
	// Pointer nil should be treated as the kubebuilder default (true)
	// so a block with only oidc set is still True/DataPlaneConfigured.
	t.Parallel()
	ext := &omniav1alpha1.AgentExternalAuth{
		OIDC: &omniav1alpha1.OIDCAuth{Issuer: "x", Audience: "y"},
	}
	cond := evaluateExternalAuthCondition(arWithExternalAuth(ext))
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("nil allowManagementPlane should default to true: status=%v", cond.Status)
	}
	if !strings.Contains(cond.Message, "managementPlane") {
		t.Errorf("message should mention managementPlane when default is true: %q", cond.Message)
	}
}

func TestEvaluateExternalAuthCondition_AllowManagementPlaneExplicitFalseWithDataPlane(t *testing.T) {
	// allowManagementPlane=false is fine as long as a data-plane
	// validator is configured — condition still True, and the message
	// must NOT include managementPlane since that path is disabled.
	t.Parallel()
	ext := &omniav1alpha1.AgentExternalAuth{
		AllowManagementPlane: ptrBool(false),
		SharedToken: &omniav1alpha1.SharedTokenAuth{
			SecretRef: corev1.LocalObjectReference{Name: "t"},
		},
	}
	cond := evaluateExternalAuthCondition(arWithExternalAuth(ext))
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("status = %v, want True", cond.Status)
	}
	if strings.Contains(cond.Message, "managementPlane") {
		t.Errorf("message must not advertise managementPlane when disabled: %q", cond.Message)
	}
	if !strings.Contains(cond.Message, "sharedToken") {
		t.Errorf("message should mention sharedToken: %q", cond.Message)
	}
}

func TestJoinWithComma(t *testing.T) {
	t.Parallel()
	if got := joinWithComma(nil); got != "(none)" {
		t.Errorf("nil: got %q, want (none)", got)
	}
	if got := joinWithComma([]string{}); got != "(none)" {
		t.Errorf("empty: got %q, want (none)", got)
	}
	if got := joinWithComma([]string{"a"}); got != "a" {
		t.Errorf("single: got %q, want a", got)
	}
	if got := joinWithComma([]string{"a", "b", "c"}); got != "a, b, c" {
		t.Errorf("multi: got %q, want a, b, c", got)
	}
}
