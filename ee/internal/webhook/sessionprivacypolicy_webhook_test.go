/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package webhook

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(s)
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

func newPolicy(name, namespace string) *omniav1alpha1.SessionPrivacyPolicy { //nolint:unparam
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
		},
	}
}

func newWorkspace(name, namespace string, groups ...corev1alpha1.WorkspaceServiceGroup) *corev1alpha1.Workspace {
	return &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: namespace},
			Services:  groups,
		},
	}
}

func newAgentRuntime(name, namespace string, policyRef *corev1.LocalObjectReference) *corev1alpha1.AgentRuntime {
	ar := &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	ar.Spec.PrivacyPolicyRef = policyRef
	return ar
}

// TestValidateCreate verifies that create always passes.
func TestValidateCreate(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("test-policy", "default")
	warnings, err := v.ValidateCreate(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

// TestValidateUpdate verifies that update always passes.
func TestValidateUpdate(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	oldPolicy := newPolicy("test-policy", "default")
	newPolicyObj := newPolicy("test-policy", "default")
	_, err := v.ValidateUpdate(context.Background(), oldPolicy, newPolicyObj)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateDelete_AllowsWhenNoConsumers verifies deletion succeeds with no references.
func TestValidateDelete_AllowsWhenNoConsumers(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateDelete_BlocksWhenServiceGroupReferences verifies deletion is blocked
// when a Workspace service group references the policy.
func TestValidateDelete_BlocksWhenServiceGroupReferences(t *testing.T) {
	ws := newWorkspace("my-workspace", "default",
		corev1alpha1.WorkspaceServiceGroup{
			Name: "prod",
			PrivacyPolicyRef: &corev1.LocalObjectReference{
				Name: "my-policy",
			},
		},
	)
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err == nil {
		t.Error("expected error blocking deletion but got none")
	}
}

// TestValidateDelete_AllowsWhenServiceGroupDifferentNamespace verifies deletion is
// allowed when the Workspace service group references a policy in a different namespace.
func TestValidateDelete_AllowsWhenServiceGroupDifferentNamespace(t *testing.T) {
	// Workspace's namespace is "other-ns", policy is in "default" → no match
	ws := newWorkspace("my-workspace", "other-ns",
		corev1alpha1.WorkspaceServiceGroup{
			Name: "prod",
			PrivacyPolicyRef: &corev1.LocalObjectReference{
				Name: "my-policy",
			},
		},
	)
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateDelete_AllowsWhenServiceGroupDifferentPolicyName verifies deletion is
// allowed when the service group references a different policy by name.
func TestValidateDelete_AllowsWhenServiceGroupDifferentPolicyName(t *testing.T) {
	ws := newWorkspace("my-workspace", "default",
		corev1alpha1.WorkspaceServiceGroup{
			Name: "prod",
			PrivacyPolicyRef: &corev1.LocalObjectReference{
				Name: "other-policy",
			},
		},
	)
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateDelete_BlocksWhenAgentRuntimeReferences verifies deletion is blocked
// when an AgentRuntime in the same namespace references the policy.
func TestValidateDelete_BlocksWhenAgentRuntimeReferences(t *testing.T) {
	ar := newAgentRuntime("my-agent", "default", &corev1.LocalObjectReference{Name: "my-policy"})
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ar).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err == nil {
		t.Error("expected error blocking deletion but got none")
	}
}

// TestValidateDelete_AllowsWhenAgentRuntimeNoPolicyRef verifies deletion is allowed
// when an AgentRuntime exists but has no privacyPolicyRef.
func TestValidateDelete_AllowsWhenAgentRuntimeNoPolicyRef(t *testing.T) {
	ar := newAgentRuntime("my-agent", "default", nil)
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ar).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateDelete_AllowsWhenAgentRuntimeDifferentPolicyName verifies deletion is
// allowed when an AgentRuntime references a different policy.
func TestValidateDelete_AllowsWhenAgentRuntimeDifferentPolicyName(t *testing.T) {
	ar := newAgentRuntime("my-agent", "default", &corev1.LocalObjectReference{Name: "other-policy"})
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ar).
		Build()
	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	policy := newPolicy("my-policy", "default")
	_, err := v.ValidateDelete(context.Background(), policy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
