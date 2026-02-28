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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func TestReconcile_PermissiveMode(t *testing.T) {
	scheme := newTestScheme(t)

	policy := &omniav1alpha1.AgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "permissive-policy",
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
		NamespacedName: types.NamespacedName{Name: "permissive-policy", Namespace: "default"},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &omniav1alpha1.AgentPolicy{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "permissive-policy", Namespace: "default"}, updated)
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
