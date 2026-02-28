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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	return scheme
}

func newTestSchemeWithIstio(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := newTestScheme(t)
	// Register Istio AuthorizationPolicy as an unstructured type
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "security.istio.io", Version: "v1", Kind: "AuthorizationPolicy"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "security.istio.io", Version: "v1", Kind: "AuthorizationPolicyList"},
		&unstructured.UnstructuredList{},
	)
	return scheme
}

// --- Claim Mapping Validation Tests ---

func TestValidateClaimMappings_Valid(t *testing.T) {
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "team", Header: "X-Omnia-Claim-Team"},
		{Claim: "region", Header: "X-Omnia-Claim-Region"},
	}
	err := validateClaimMappings(entries)
	assert.NoError(t, err)
}

func TestValidateClaimMappings_EmptyClaim(t *testing.T) {
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "", Header: "X-Omnia-Claim-Team"},
	}
	err := validateClaimMappings(entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "claim name must not be empty")
}

func TestValidateClaimMappings_EmptyHeader(t *testing.T) {
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "team", Header: ""},
	}
	err := validateClaimMappings(entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "header name must not be empty")
}

func TestValidateClaimMappings_WrongPrefix(t *testing.T) {
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "team", Header: "X-Custom-Team"},
	}
	err := validateClaimMappings(entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must start with")
}

func TestValidateClaimMappings_DuplicateHeader(t *testing.T) {
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "team", Header: "X-Omnia-Claim-Team"},
		{Claim: "group", Header: "X-Omnia-Claim-Team"},
	}
	err := validateClaimMappings(entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate header")
}

func TestValidateClaimMappings_DuplicateHeaderCaseInsensitive(t *testing.T) {
	// Both entries have valid prefix; duplicate detection is case-insensitive
	entries := []omniav1alpha1.ClaimMappingEntry{
		{Claim: "team", Header: "X-Omnia-Claim-Team"},
		{Claim: "group", Header: "X-Omnia-Claim-team"},
	}
	err := validateClaimMappings(entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate header")
}

func TestValidateClaimMappings_Empty(t *testing.T) {
	err := validateClaimMappings(nil)
	assert.NoError(t, err)
}

func TestValidateClaimEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   omniav1alpha1.ClaimMappingEntry
		wantErr bool
	}{
		{
			name:    "valid entry",
			entry:   omniav1alpha1.ClaimMappingEntry{Claim: "team", Header: "X-Omnia-Claim-Team"},
			wantErr: false,
		},
		{
			name:    "empty claim",
			entry:   omniav1alpha1.ClaimMappingEntry{Claim: "", Header: "X-Omnia-Claim-Team"},
			wantErr: true,
		},
		{
			name:    "empty header",
			entry:   omniav1alpha1.ClaimMappingEntry{Claim: "team", Header: ""},
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			entry:   omniav1alpha1.ClaimMappingEntry{Claim: "team", Header: "X-Custom-Header"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClaimEntry(tt.entry, make(map[string]bool))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- ToolAccess Validation Tests ---

func TestValidateToolAccess_Valid(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode: omniav1alpha1.ToolAccessModeAllowlist,
		Rules: []omniav1alpha1.ToolAccessRule{
			{Registry: "my-registry", Tools: []string{"tool-a", "tool-b"}},
		},
	}
	err := validateToolAccess(cfg)
	assert.NoError(t, err)
}

func TestValidateToolAccess_EmptyRules(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode:  omniav1alpha1.ToolAccessModeDenylist,
		Rules: []omniav1alpha1.ToolAccessRule{},
	}
	err := validateToolAccess(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rules must not be empty")
}

func TestValidateToolAccess_EmptyRegistry(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode: omniav1alpha1.ToolAccessModeAllowlist,
		Rules: []omniav1alpha1.ToolAccessRule{
			{Registry: "", Tools: []string{"tool-a"}},
		},
	}
	err := validateToolAccess(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry must not be empty")
}

func TestValidateToolAccess_EmptyTools(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode: omniav1alpha1.ToolAccessModeAllowlist,
		Rules: []omniav1alpha1.ToolAccessRule{
			{Registry: "my-registry", Tools: []string{}},
		},
	}
	err := validateToolAccess(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tools must not be empty")
}

func TestValidateToolAccess_EmptyToolName(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode: omniav1alpha1.ToolAccessModeAllowlist,
		Rules: []omniav1alpha1.ToolAccessRule{
			{Registry: "my-registry", Tools: []string{"tool-a", ""}},
		},
	}
	err := validateToolAccess(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool name must not be empty")
}

func TestValidateToolAccessRule_Valid(t *testing.T) {
	rule := omniav1alpha1.ToolAccessRule{
		Registry: "my-registry",
		Tools:    []string{"tool-a"},
	}
	err := validateToolAccessRule(rule)
	assert.NoError(t, err)
}

func TestValidateToolAccess_MultipleRules(t *testing.T) {
	cfg := &omniav1alpha1.ToolAccessConfig{
		Mode: omniav1alpha1.ToolAccessModeDenylist,
		Rules: []omniav1alpha1.ToolAccessRule{
			{Registry: "reg-1", Tools: []string{"tool-a"}},
			{Registry: "reg-2", Tools: []string{"tool-b", "tool-c"}},
		},
	}
	err := validateToolAccess(cfg)
	assert.NoError(t, err)
}

// --- Policy Validation Tests ---

func TestValidatePolicy_NilClaimMapping(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{},
	}
	err := r.validatePolicy(policy)
	assert.NoError(t, err)
}

func TestValidatePolicy_ValidClaimMapping(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "X-Omnia-Claim-Team"},
				},
			},
		},
	}
	err := r.validatePolicy(policy)
	assert.NoError(t, err)
}

func TestValidatePolicy_InvalidClaimMapping(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "Invalid-Header"},
				},
			},
		},
	}
	err := r.validatePolicy(policy)
	assert.Error(t, err)
}

func TestValidatePolicy_ValidToolAccess(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "my-registry", Tools: []string{"tool-a"}},
				},
			},
		},
	}
	err := r.validatePolicy(policy)
	assert.NoError(t, err)
}

func TestValidatePolicy_InvalidToolAccess(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode:  omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{},
			},
		},
	}
	err := r.validatePolicy(policy)
	assert.Error(t, err)
}

func TestValidatePolicy_BothClaimMappingAndToolAccess(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "X-Omnia-Claim-Team"},
				},
			},
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"tool"}},
				},
			},
		},
	}
	err := r.validatePolicy(policy)
	assert.NoError(t, err)
}

// --- PolicyMatchesAgent Tests ---

func TestPolicyMatchesAgent(t *testing.T) {
	r := &AgentPolicyReconciler{}

	// No selector matches all
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{},
	}
	assert.True(t, r.policyMatchesAgent(policy, "any-agent"))

	// Empty agents list matches all
	policy.Spec.Selector = &omniav1alpha1.AgentPolicySelector{
		Agents: []string{},
	}
	assert.True(t, r.policyMatchesAgent(policy, "any-agent"))

	// Specific agents list
	policy.Spec.Selector.Agents = []string{"agent-a", "agent-b"}
	assert.True(t, r.policyMatchesAgent(policy, "agent-a"))
	assert.True(t, r.policyMatchesAgent(policy, "agent-b"))
	assert.False(t, r.policyMatchesAgent(policy, "agent-c"))
}

// --- SetErrorStatus Tests ---

func TestSetErrorStatus(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{}
	policy.Generation = 2

	r.setErrorStatus(policy, assert.AnError)

	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseError, policy.Status.Phase)
	assert.Equal(t, int64(2), policy.Status.ObservedGeneration)
	assert.Len(t, policy.Status.Conditions, 1)
	assert.Equal(t, AgentPolicyConditionTypeValid, policy.Status.Conditions[0].Type)
	assert.Equal(t, "False", string(policy.Status.Conditions[0].Status))
}

// --- SetActiveStatus Tests ---

func TestSetActiveStatus(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{}
	policy.Generation = 3

	r.setActiveStatus(policy, 5)

	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseActive, policy.Status.Phase)
	assert.Equal(t, int32(5), policy.Status.MatchedAgents)
	assert.Equal(t, int64(3), policy.Status.ObservedGeneration)
	assert.Len(t, policy.Status.Conditions, 2)
}

// --- Reconcile Tests ---

func TestReconcile_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_ValidPolicy(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "X-Omnia-Claim-Team"},
				},
			},
		},
	}

	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-a",
			Namespace: "default",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, agent).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify status was updated
	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseActive, updated.Status.Phase)
	assert.Equal(t, int32(1), updated.Status.MatchedAgents)
}

func TestReconcile_InvalidPolicy(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "bad-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "Bad-Header"},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-policy", Namespace: "default"},
	})

	// Validation errors return nil error (no retry)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify status was updated to Error
	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "bad-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseError, updated.Status.Phase)
}

func TestReconcile_InvalidToolAccess(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "bad-tool-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode:  omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-tool-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "bad-tool-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseError, updated.Status.Phase)
}

func TestReconcile_WithSelector(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "selective-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Selector: &omniav1alpha1.AgentPolicySelector{
				Agents: []string{"agent-a"},
			},
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "X-Omnia-Claim-Team"},
				},
			},
		},
	}

	agentA := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}
	agentB := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-b", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, agentA, agentB).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "selective-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "selective-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, int32(1), updated.Status.MatchedAgents) // Only agent-a matches
}

// --- CountMatchedAgents Tests ---

func TestCountMatchedAgents_AllAgents(t *testing.T) {
	scheme := newTestScheme(t)

	agentA := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}
	agentB := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-b", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(agentA, agentB).
		Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	// No selector -> match all
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	count, err := r.countMatchedAgents(context.Background(), policy)
	require.NoError(t, err)
	assert.Equal(t, int32(2), count)
}

func TestCountMatchedAgents_Filtered(t *testing.T) {
	scheme := newTestScheme(t)

	agentA := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}
	agentB := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-b", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(agentA, agentB).
		Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: omniav1alpha1.AgentPolicySpec{
			Selector: &omniav1alpha1.AgentPolicySelector{
				Agents: []string{"agent-b"},
			},
		},
	}
	count, err := r.countMatchedAgents(context.Background(), policy)
	require.NoError(t, err)
	assert.Equal(t, int32(1), count)
}

func TestReconcile_PermissiveModeClaimMapping(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "permissive-claim-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModePermissive,
			ClaimMapping: &omniav1alpha1.ClaimMapping{
				ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
					{Claim: "team", Header: "X-Omnia-Claim-Team"},
				},
			},
		},
	}

	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test"},
			Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, agent).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "permissive-claim-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "permissive-claim-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseActive, updated.Status.Phase)
	assert.Equal(t, int32(1), updated.Status.MatchedAgents)

	// Verify permissive mode is reflected in conditions
	var appliedCondition *metav1.Condition
	for i := range updated.Status.Conditions {
		if updated.Status.Conditions[i].Type == AgentPolicyConditionTypeApplied {
			appliedCondition = &updated.Status.Conditions[i]
			break
		}
	}
	require.NotNil(t, appliedCondition)
	assert.Contains(t, appliedCondition.Message, "permissive mode")
}

func TestBuildAppliedMessage(t *testing.T) {
	tests := []struct {
		name     string
		mode     omniav1alpha1.AgentPolicyMode
		count    int32
		contains string
	}{
		{
			name:     "enforce mode",
			mode:     omniav1alpha1.AgentPolicyModeEnforce,
			count:    3,
			contains: "Policy applied to 3 agent(s)",
		},
		{
			name:     "permissive mode",
			mode:     omniav1alpha1.AgentPolicyModePermissive,
			count:    2,
			contains: "permissive mode",
		},
		{
			name:     "empty mode defaults to enforce behavior",
			mode:     "",
			count:    1,
			contains: "Policy applied to 1 agent(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildAppliedMessage(tt.mode, tt.count)
			assert.Contains(t, msg, tt.contains)
		})
	}
}

// --- FindPoliciesForAgent Tests ---

func TestFindPoliciesForAgent(t *testing.T) {
	scheme := newTestScheme(t)

	policyAll := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-all", Namespace: "default"},
		Spec:       omniav1alpha1.AgentPolicySpec{}, // no selector = match all
	}
	policySpecific := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-specific", Namespace: "default"},
		Spec: omniav1alpha1.AgentPolicySpec{
			Selector: &omniav1alpha1.AgentPolicySelector{
				Agents: []string{"agent-a"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policyAll, policySpecific).
		Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
	}
	requests := r.findPoliciesForAgent(context.Background(), agent)
	assert.Len(t, requests, 2) // Both policies match agent-a

	agentOther := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-other", Namespace: "default"},
	}
	requests = r.findPoliciesForAgent(context.Background(), agentOther)
	assert.Len(t, requests, 1) // Only policy-all matches
}

// --- Istio AuthorizationPolicy Generation Tests ---

func TestBuildDesiredAuthPolicies_NilToolAccess(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		Spec: omniav1alpha1.AgentPolicySpec{},
	}
	result := r.buildDesiredAuthPolicies(policy)
	assert.Nil(t, result)
}

func TestBuildDesiredAuthPolicies_AllowlistEnforce(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "my-registry", Tools: []string{"tool-a", "tool-b"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	// Allowlist enforce: 1 ALLOW + 1 DENY catch-all
	require.Len(t, result, 2)

	// Verify ALLOW policy
	allowPolicy := result[0]
	assert.Equal(t, "test-policy-allow", allowPolicy.GetName())
	assert.Equal(t, istioSecurityAPIVersion, allowPolicy.GetAPIVersion())
	assert.Equal(t, istioAuthPolicyKind, allowPolicy.GetKind())

	spec := allowPolicy.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionAllow, spec["action"])

	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	when := rule["when"].([]interface{})
	require.Len(t, when, 1) // No selector, so only tool condition
	toolCondition := when[0].(map[string]interface{})
	assert.Equal(t, "request.headers[X-Omnia-Tool-Name]", toolCondition["key"])
	values := toolCondition["values"].([]interface{})
	assert.Contains(t, values, "my-registry/tool-a")
	assert.Contains(t, values, "my-registry/tool-b")

	// Verify DENY catch-all policy
	denyPolicy := result[1]
	assert.Equal(t, "test-policy-deny-all", denyPolicy.GetName())
	denySpec := denyPolicy.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionDeny, denySpec["action"])

	// Verify owner references
	ownerRefs := allowPolicy.GetOwnerReferences()
	require.Len(t, ownerRefs, 1)
	assert.Equal(t, "test-policy", ownerRefs[0].Name)
	assert.Equal(t, "AgentPolicy", ownerRefs[0].Kind)

	// Verify labels
	labels := allowPolicy.GetLabels()
	assert.Equal(t, managedByLabelValue, labels[labelAppManagedBy])
	assert.Equal(t, "test-policy", labels[ownerPolicyLabel])
}

func TestBuildDesiredAuthPolicies_DenylistEnforce(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deny-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "bad-registry", Tools: []string{"dangerous-tool"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	// Denylist: only 1 DENY policy, no catch-all
	require.Len(t, result, 1)

	denyPolicy := result[0]
	assert.Equal(t, "deny-policy-deny", denyPolicy.GetName())

	spec := denyPolicy.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionDeny, spec["action"])

	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	when := rule["when"].([]interface{})
	toolCondition := when[0].(map[string]interface{})
	values := toolCondition["values"].([]interface{})
	assert.Contains(t, values, "bad-registry/dangerous-tool")
}

func TestBuildDesiredAuthPolicies_PermissiveMode(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audit-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModePermissive,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"tool-a"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	// Permissive mode: only AUDIT policy, no DENY catch-all
	require.Len(t, result, 1)

	auditPolicy := result[0]
	spec := auditPolicy.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionAudit, spec["action"])
}

func TestBuildDesiredAuthPolicies_PermissiveDenylist(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audit-deny-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModePermissive,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"tool-a"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	require.Len(t, result, 1)
	spec := result[0].Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionAudit, spec["action"])
}

func TestBuildDesiredAuthPolicies_WithAgentSelector(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scoped-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Selector: &omniav1alpha1.AgentPolicySelector{
				Agents: []string{"agent-a", "agent-b"},
			},
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"tool-x"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	require.Len(t, result, 1)
	spec := result[0].Object["spec"].(map[string]interface{})
	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 1)

	rule := rules[0].(map[string]interface{})
	when := rule["when"].([]interface{})
	// Should have both tool condition and agent condition
	require.Len(t, when, 2)

	agentCondition := when[1].(map[string]interface{})
	assert.Equal(t, "request.headers[X-Omnia-Agent-Name]", agentCondition["key"])
	agentValues := agentCondition["values"].([]interface{})
	assert.Contains(t, agentValues, "agent-a")
	assert.Contains(t, agentValues, "agent-b")
}

func TestBuildDesiredAuthPolicies_MultipleRules(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-rule-policy",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg-1", Tools: []string{"tool-a"}},
					{Registry: "reg-2", Tools: []string{"tool-b", "tool-c"}},
				},
			},
		},
	}

	result := r.buildDesiredAuthPolicies(policy)

	require.Len(t, result, 1)
	spec := result[0].Object["spec"].(map[string]interface{})
	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 2)

	// First rule
	rule1 := rules[0].(map[string]interface{})
	when1 := rule1["when"].([]interface{})
	cond1 := when1[0].(map[string]interface{})
	vals1 := cond1["values"].([]interface{})
	assert.Equal(t, []interface{}{"reg-1/tool-a"}, vals1)

	// Second rule
	rule2 := rules[1].(map[string]interface{})
	when2 := rule2["when"].([]interface{})
	cond2 := when2[0].(map[string]interface{})
	vals2 := cond2["values"].([]interface{})
	assert.Contains(t, vals2, "reg-2/tool-b")
	assert.Contains(t, vals2, "reg-2/tool-c")
}

// --- ResolveIstioAction Tests ---

func TestResolveIstioAction(t *testing.T) {
	r := &AgentPolicyReconciler{}

	tests := []struct {
		name     string
		mode     omniav1alpha1.AgentPolicyMode
		taMode   omniav1alpha1.ToolAccessMode
		expected string
	}{
		{
			name:     "permissive always returns AUDIT",
			mode:     omniav1alpha1.AgentPolicyModePermissive,
			taMode:   omniav1alpha1.ToolAccessModeAllowlist,
			expected: istioActionAudit,
		},
		{
			name:     "permissive denylist returns AUDIT",
			mode:     omniav1alpha1.AgentPolicyModePermissive,
			taMode:   omniav1alpha1.ToolAccessModeDenylist,
			expected: istioActionAudit,
		},
		{
			name:     "enforce allowlist returns ALLOW",
			mode:     omniav1alpha1.AgentPolicyModeEnforce,
			taMode:   omniav1alpha1.ToolAccessModeAllowlist,
			expected: istioActionAllow,
		},
		{
			name:     "enforce denylist returns DENY",
			mode:     omniav1alpha1.AgentPolicyModeEnforce,
			taMode:   omniav1alpha1.ToolAccessModeDenylist,
			expected: istioActionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &omniav1alpha1.AgentPolicy{
				Spec: omniav1alpha1.AgentPolicySpec{
					Mode: tt.mode,
					ToolAccess: &omniav1alpha1.ToolAccessConfig{
						Mode: tt.taMode,
					},
				},
			}
			assert.Equal(t, tt.expected, r.resolveIstioAction(policy))
		})
	}
}

// --- AuthPolicy Apply Tests ---

func TestReconcileAuthorizationPolicies_NoToolAccess(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       omniav1alpha1.AgentPolicySpec{},
	}

	err := r.reconcileAuthorizationPolicies(context.Background(), policy)
	assert.NoError(t, err)
}

func TestCreateOrUpdateAuthPolicy_Create(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	desired := &unstructured.Unstructured{}
	desired.SetAPIVersion(istioSecurityAPIVersion)
	desired.SetKind(istioAuthPolicyKind)
	desired.SetName("test-auth-policy")
	desired.SetNamespace("default")
	desired.SetLabels(map[string]string{ownerPolicyLabel: "test"})
	desired.Object["spec"] = map[string]interface{}{"action": istioActionDeny}

	err := r.createOrUpdateAuthPolicy(context.Background(), desired)
	assert.NoError(t, err)

	// Verify it was created
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(istioSecurityAPIVersion)
	existing.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-auth-policy", Namespace: "default"}, existing)
	assert.NoError(t, err)
	assert.Equal(t, istioActionDeny, existing.Object["spec"].(map[string]interface{})["action"])
}

func TestCreateOrUpdateAuthPolicy_Update(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(istioSecurityAPIVersion)
	existing.SetKind(istioAuthPolicyKind)
	existing.SetName("test-auth-policy")
	existing.SetNamespace("default")
	existing.SetLabels(map[string]string{ownerPolicyLabel: "test"})
	existing.Object["spec"] = map[string]interface{}{"action": istioActionDeny}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	desired := &unstructured.Unstructured{}
	desired.SetAPIVersion(istioSecurityAPIVersion)
	desired.SetKind(istioAuthPolicyKind)
	desired.SetName("test-auth-policy")
	desired.SetNamespace("default")
	desired.SetLabels(map[string]string{ownerPolicyLabel: "test", "new-label": "value"})
	desired.Object["spec"] = map[string]interface{}{"action": istioActionAllow}

	err := r.createOrUpdateAuthPolicy(context.Background(), desired)
	assert.NoError(t, err)

	// Verify it was updated
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioSecurityAPIVersion)
	updated.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-auth-policy", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.Equal(t, istioActionAllow, updated.Object["spec"].(map[string]interface{})["action"])
	assert.Equal(t, "value", updated.GetLabels()["new-label"])
}

func TestDeleteStaleAuthPolicies(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	stale := &unstructured.Unstructured{}
	stale.SetAPIVersion(istioSecurityAPIVersion)
	stale.SetKind(istioAuthPolicyKind)
	stale.SetName("old-policy")
	stale.SetNamespace("default")
	stale.SetLabels(map[string]string{ownerPolicyLabel: "my-policy"})

	keep := &unstructured.Unstructured{}
	keep.SetAPIVersion(istioSecurityAPIVersion)
	keep.SetKind(istioAuthPolicyKind)
	keep.SetName("keep-policy")
	keep.SetNamespace("default")
	keep.SetLabels(map[string]string{ownerPolicyLabel: "my-policy"})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale, keep).Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default"},
	}

	desiredNames := map[string]bool{"keep-policy": true}

	err := r.deleteStaleAuthPolicies(context.Background(), policy, desiredNames)
	assert.NoError(t, err)

	// Verify stale was deleted
	check := &unstructured.Unstructured{}
	check.SetAPIVersion(istioSecurityAPIVersion)
	check.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "old-policy", Namespace: "default"}, check)
	assert.True(t, apierrors.IsNotFound(err))

	// Verify keep still exists
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "keep-policy", Namespace: "default"}, check)
	assert.NoError(t, err)
}

// --- Helper Function Tests ---

func TestBuildToolHeaderValues(t *testing.T) {
	rule := omniav1alpha1.ToolAccessRule{
		Registry: "my-reg",
		Tools:    []string{"tool-a", "tool-b"},
	}
	values := buildToolHeaderValues(rule)
	assert.Equal(t, []interface{}{"my-reg/tool-a", "my-reg/tool-b"}, values)
}

func TestBuildAgentCondition(t *testing.T) {
	condition := buildAgentCondition([]string{"agent-1", "agent-2"})
	assert.Equal(t, "request.headers[X-Omnia-Agent-Name]", condition["key"])
	values := condition["values"].([]interface{})
	assert.Contains(t, values, "agent-1")
	assert.Contains(t, values, "agent-2")
}

func TestBuildCatchAllRules_NoSelector(t *testing.T) {
	rules := buildCatchAllRules(nil)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	_, hasWhen := rule["when"]
	assert.False(t, hasWhen)
}

func TestBuildCatchAllRules_WithSelector(t *testing.T) {
	selector := &omniav1alpha1.AgentPolicySelector{
		Agents: []string{"agent-a"},
	}
	rules := buildCatchAllRules(selector)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	when := rule["when"].([]interface{})
	require.Len(t, when, 1)
	cond := when[0].(map[string]interface{})
	assert.Equal(t, "request.headers[X-Omnia-Agent-Name]", cond["key"])
}

func TestBuildCatchAllRules_EmptySelector(t *testing.T) {
	selector := &omniav1alpha1.AgentPolicySelector{
		Agents: []string{},
	}
	rules := buildCatchAllRules(selector)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	_, hasWhen := rule["when"]
	assert.False(t, hasWhen)
}

func TestBuildSingleToolRule_NoSelector(t *testing.T) {
	rule := omniav1alpha1.ToolAccessRule{Registry: "reg", Tools: []string{"tool"}}
	result := buildSingleToolRule(rule, nil)
	when := result["when"].([]interface{})
	assert.Len(t, when, 1) // Only tool condition
}

func TestBuildSingleToolRule_WithSelector(t *testing.T) {
	rule := omniav1alpha1.ToolAccessRule{Registry: "reg", Tools: []string{"tool"}}
	selector := &omniav1alpha1.AgentPolicySelector{Agents: []string{"agent-a"}}
	result := buildSingleToolRule(rule, selector)
	when := result["when"].([]interface{})
	assert.Len(t, when, 2) // Tool + agent conditions
}

func TestBuildRulesFromToolAccess(t *testing.T) {
	rules := []omniav1alpha1.ToolAccessRule{
		{Registry: "reg-1", Tools: []string{"tool-a"}},
		{Registry: "reg-2", Tools: []string{"tool-b"}},
	}
	result := buildRulesFromToolAccess(rules, nil)
	assert.Len(t, result, 2)
}

func TestIsNoMatchError(t *testing.T) {
	assert.True(t, isNoMatchError(fmt.Errorf("no matches for kind \"AuthorizationPolicy\" in group \"security.istio.io\"")))
	assert.False(t, isNoMatchError(fmt.Errorf("some other error")))
}

func TestSetAuthPolicySpec(t *testing.T) {
	ap := &unstructured.Unstructured{Object: map[string]interface{}{}}
	ap.SetAPIVersion(istioSecurityAPIVersion)
	ap.SetKind(istioAuthPolicyKind)

	rules := []interface{}{map[string]interface{}{"when": []interface{}{}}}
	setAuthPolicySpec(ap, istioActionDeny, rules)

	spec, found, err := unstructured.NestedMap(ap.Object, "spec")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, istioActionDeny, spec["action"])
}

func TestNewAuthPolicyBase(t *testing.T) {
	r := &AgentPolicyReconciler{}
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-policy",
			Namespace: "test-ns",
			UID:       "uid-123",
		},
	}

	ap := r.newAuthPolicyBase(policy, "my-policy-allow")

	assert.Equal(t, "my-policy-allow", ap.GetName())
	assert.Equal(t, "test-ns", ap.GetNamespace())
	assert.Equal(t, istioSecurityAPIVersion, ap.GetAPIVersion())
	assert.Equal(t, istioAuthPolicyKind, ap.GetKind())
	assert.Equal(t, managedByLabelValue, ap.GetLabels()[labelAppManagedBy])
	assert.Equal(t, "my-policy", ap.GetLabels()[ownerPolicyLabel])

	ownerRefs := ap.GetOwnerReferences()
	require.Len(t, ownerRefs, 1)
	assert.Equal(t, "my-policy", ownerRefs[0].Name)
}

// --- Integration-style Tests ---

func TestReconcile_WithToolAccessAllowlist(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "tool-policy",
			Namespace:  "default",
			Generation: 1,
			UID:        "uid-abc",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "my-reg", Tools: []string{"tool-a"}},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "tool-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify AuthorizationPolicies were created
	allowAP := &unstructured.Unstructured{}
	allowAP.SetAPIVersion(istioSecurityAPIVersion)
	allowAP.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "tool-policy-allow", Namespace: "default"}, allowAP)
	assert.NoError(t, err)

	denyAP := &unstructured.Unstructured{}
	denyAP.SetAPIVersion(istioSecurityAPIVersion)
	denyAP.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "tool-policy-deny-all", Namespace: "default"}, denyAP)
	assert.NoError(t, err)

	// Verify status is active
	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "tool-policy", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseActive, updated.Status.Phase)
}

func TestReconcile_WithToolAccessDenylist(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "deny-tool-policy",
			Namespace:  "default",
			Generation: 1,
			UID:        "uid-def",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "bad-reg", Tools: []string{"bad-tool"}},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "deny-tool-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify single DENY AuthorizationPolicy was created
	denyAP := &unstructured.Unstructured{}
	denyAP.SetAPIVersion(istioSecurityAPIVersion)
	denyAP.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "deny-tool-policy-deny", Namespace: "default"}, denyAP)
	assert.NoError(t, err)

	spec := denyAP.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionDeny, spec["action"])
}

func TestReconcile_PermissiveMode(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "permissive-policy",
			Namespace:  "default",
			Generation: 1,
			UID:        "uid-perm",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModePermissive,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeAllowlist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"tool"}},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "permissive-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify AUDIT policy was created (not ALLOW + DENY)
	auditAP := &unstructured.Unstructured{}
	auditAP.SetAPIVersion(istioSecurityAPIVersion)
	auditAP.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "permissive-policy-allow", Namespace: "default"}, auditAP)
	assert.NoError(t, err)
	spec := auditAP.Object["spec"].(map[string]interface{})
	assert.Equal(t, istioActionAudit, spec["action"])

	// Verify no deny-all policy (permissive mode skips catch-all)
	denyAP := &unstructured.Unstructured{}
	denyAP.SetAPIVersion(istioSecurityAPIVersion)
	denyAP.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "permissive-policy-deny-all", Namespace: "default"}, denyAP)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_CleanupOnToolAccessRemoval(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	// Pre-existing AuthorizationPolicy from a previous reconcile
	existingAP := &unstructured.Unstructured{}
	existingAP.SetAPIVersion(istioSecurityAPIVersion)
	existingAP.SetKind(istioAuthPolicyKind)
	existingAP.SetName("cleanup-policy-deny")
	existingAP.SetNamespace("default")
	existingAP.SetLabels(map[string]string{ownerPolicyLabel: "cleanup-policy"})

	// Policy without toolAccess (removed)
	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cleanup-policy",
			Namespace:  "default",
			Generation: 2,
			UID:        "uid-clean",
		},
		Spec: omniav1alpha1.AgentPolicySpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, existingAP).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cleanup-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify stale AP was deleted
	check := &unstructured.Unstructured{}
	check.SetAPIVersion(istioSecurityAPIVersion)
	check.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "cleanup-policy-deny", Namespace: "default"}, check)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_UpdatePropagation(t *testing.T) {
	scheme := newTestSchemeWithIstio(t)

	// Pre-existing AuthorizationPolicy with old action
	existingAP := &unstructured.Unstructured{}
	existingAP.SetAPIVersion(istioSecurityAPIVersion)
	existingAP.SetKind(istioAuthPolicyKind)
	existingAP.SetName("update-policy-deny")
	existingAP.SetNamespace("default")
	existingAP.SetLabels(map[string]string{ownerPolicyLabel: "update-policy"})
	existingAP.Object["spec"] = map[string]interface{}{
		"action": istioActionDeny,
		"rules":  []interface{}{},
	}

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "update-policy",
			Namespace:  "default",
			Generation: 2,
			UID:        "uid-update",
		},
		Spec: omniav1alpha1.AgentPolicySpec{
			Mode: omniav1alpha1.AgentPolicyModeEnforce,
			ToolAccess: &omniav1alpha1.ToolAccessConfig{
				Mode: omniav1alpha1.ToolAccessModeDenylist,
				Rules: []omniav1alpha1.ToolAccessRule{
					{Registry: "reg", Tools: []string{"new-tool"}},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, existingAP).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "update-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the AuthorizationPolicy was updated with new tool
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioSecurityAPIVersion)
	updated.SetKind(istioAuthPolicyKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "update-policy-deny", Namespace: "default"}, updated)
	require.NoError(t, err)

	spec := updated.Object["spec"].(map[string]interface{})
	rules := spec["rules"].([]interface{})
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	when := rule["when"].([]interface{})
	cond := when[0].(map[string]interface{})
	vals := cond["values"].([]interface{})
	assert.Contains(t, vals, "reg/new-tool")
}

// --- HandleValidationError Tests ---

func TestHandleValidationError(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "err-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentPolicySpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := &AgentPolicyReconciler{Client: fakeClient, Scheme: scheme}

	result, err := r.handleValidationError(context.Background(), policy, fmt.Errorf("test error"))
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, omniav1alpha1.AgentPolicyPhaseError, policy.Status.Phase)
}
