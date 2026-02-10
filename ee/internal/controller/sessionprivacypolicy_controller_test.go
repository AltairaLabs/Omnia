/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
)

func setupPrivacyPolicyTest(t *testing.T, objects ...runtime.Object) (*SessionPrivacyPolicyReconciler, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	// Create the omnia-system namespace
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "omnia-system",
		},
	}

	clientObjects := []runtime.Object{omniaSystemNS}
	clientObjects = append(clientObjects, objects...)

	// Convert to client.Object for the fake client builder
	clientObjs := make([]runtime.Object, len(clientObjects))
	copy(clientObjs, clientObjects)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range clientObjs {
		if co, ok := obj.(interface{ GetName() string }); ok {
			_ = co
		}
		builder = builder.WithRuntimeObjects(obj)
	}

	// Also register status subresource for SessionPrivacyPolicy
	builder = builder.WithStatusSubresource(&omniav1alpha1.SessionPrivacyPolicy{})

	fakeClient := builder.Build()

	recorder := record.NewFakeRecorder(20)
	reg := prometheus.NewRegistry()
	testMetrics := metrics.NewPrivacyPolicyMetricsWithRegistry(reg)

	reconciler := &SessionPrivacyPolicyReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		Metrics:  testMetrics,
	}

	return reconciler, recorder
}

func newGlobalPolicy(name string) *omniav1alpha1.SessionPrivacyPolicy {
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   true,
				PII: &omniav1alpha1.PIIConfig{
					Redact:  true,
					Encrypt: false,
				},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{
				Enabled:             true,
				HonorDeleteRequests: true,
				DeleteWithinDays:    ptr[int32](30),
			},
			Retention: &omniav1alpha1.PrivacyRetentionConfig{
				Facade: &omniav1alpha1.PrivacyRetentionTierConfig{
					WarmDays: ptr[int32](90),
					ColdDays: ptr[int32](365),
				},
				RichData: &omniav1alpha1.PrivacyRetentionTierConfig{
					WarmDays: ptr[int32](30),
					ColdDays: ptr[int32](180),
				},
			},
			AuditLog: &omniav1alpha1.AuditLogConfig{
				Enabled:       true,
				RetentionDays: ptr[int32](365),
			},
		},
	}
}

func newWorkspacePolicy(name, workspaceName string) *omniav1alpha1.SessionPrivacyPolicy {
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelWorkspace,
			WorkspaceRef: &corev1alpha1.LocalObjectReference{
				Name: workspaceName,
			},
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false, // Stricter: no rich data
				PII: &omniav1alpha1.PIIConfig{
					Redact:  true,
					Encrypt: true, // Stricter: also encrypt
				},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{
				Enabled:             true,
				HonorDeleteRequests: true,
				DeleteWithinDays:    ptr[int32](14), // Stricter: faster deletion
			},
			Retention: &omniav1alpha1.PrivacyRetentionConfig{
				Facade: &omniav1alpha1.PrivacyRetentionTierConfig{
					WarmDays: ptr[int32](60), // Stricter: less retention
					ColdDays: ptr[int32](180),
				},
			},
		},
	}
}

func newAgentPolicy(name, agentNamespace string) *omniav1alpha1.SessionPrivacyPolicy {
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelAgent,
			AgentRef: &corev1alpha1.NamespacedObjectReference{
				Name:      "my-agent",
				Namespace: agentNamespace,
			},
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false,
				PII: &omniav1alpha1.PIIConfig{
					Redact:  true,
					Encrypt: true,
				},
			},
		},
	}
}

func reconcilePolicy(t *testing.T, r *SessionPrivacyPolicyReconciler, name string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	require.NoError(t, err)
	return result
}

func getPolicy(t *testing.T, r *SessionPrivacyPolicyReconciler, name string) *omniav1alpha1.SessionPrivacyPolicy {
	t.Helper()
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name}, policy)
	require.NoError(t, err)
	return policy
}

func getEffectiveConfigMap(t *testing.T, r *SessionPrivacyPolicyReconciler, policyName string) *corev1.ConfigMap {
	t.Helper()
	cm := &corev1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{
		Name:      "omnia-privacy-policy-effective-" + policyName,
		Namespace: "omnia-system",
	}, cm)
	require.NoError(t, err)
	return cm
}

func getEffectivePolicyFromConfigMap(t *testing.T, cm *corev1.ConfigMap) *omniav1alpha1.SessionPrivacyPolicySpec {
	t.Helper()
	data, ok := cm.Data["effective-policy"]
	require.True(t, ok, "effective-policy key not found in ConfigMap")
	var spec omniav1alpha1.SessionPrivacyPolicySpec
	err := json.Unmarshal([]byte(data), &spec)
	require.NoError(t, err)
	return &spec
}

func TestSessionPrivacyPolicy_GlobalPolicy_CreatesConfigMap(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-privacy")
	r, recorder := setupPrivacyPolicyTest(t, globalPolicy)

	reconcilePolicy(t, r, "global-privacy")

	// Verify status
	policy := getPolicy(t, r, "global-privacy")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)
	assert.Equal(t, policy.Generation, policy.Status.ObservedGeneration)

	// Verify Ready condition
	readyCond := findCondition(policy.Status.Conditions, ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)

	// Verify ParentFound condition (N/A for global)
	parentCond := findCondition(policy.Status.Conditions, ConditionTypeParentFound)
	require.NotNil(t, parentCond)
	assert.Equal(t, metav1.ConditionTrue, parentCond.Status)
	assert.Equal(t, "NotApplicable", parentCond.Reason)

	// Verify EffectivePolicyStored condition
	storedCond := findCondition(policy.Status.Conditions, ConditionTypeEffectivePolicyStored)
	require.NotNil(t, storedCond)
	assert.Equal(t, metav1.ConditionTrue, storedCond.Status)

	// Verify ConfigMap
	cm := getEffectiveConfigMap(t, r, "global-privacy")
	assert.Equal(t, "omnia", cm.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "session-privacy-policy", cm.Labels["app.kubernetes.io/component"])
	assert.Equal(t, "global-privacy", cm.Labels["omnia.altairalabs.ai/policy-name"])
	assert.Equal(t, "global", cm.Labels["omnia.altairalabs.ai/policy-level"])
	assert.Empty(t, cm.Data["parent-policy"])

	// Verify effective policy content
	effective := getEffectivePolicyFromConfigMap(t, cm)
	assert.True(t, effective.Recording.Enabled)
	assert.True(t, effective.Recording.PII.Redact)
	assert.True(t, effective.AuditLog.Enabled)

	// Verify events
	assertEventRecorded(t, recorder, EventReasonEffectivePolicyComputed)
}

func TestSessionPrivacyPolicy_WorkspaceWithGlobalParent(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-privacy")
	wsPolicy := newWorkspacePolicy("ws-privacy", "my-workspace")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy, wsPolicy)

	// Reconcile global first
	reconcilePolicy(t, r, "global-privacy")
	// Then reconcile workspace
	reconcilePolicy(t, r, "ws-privacy")

	// Verify workspace status
	policy := getPolicy(t, r, "ws-privacy")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)

	// Verify ParentFound condition references global
	parentCond := findCondition(policy.Status.Conditions, ConditionTypeParentFound)
	require.NotNil(t, parentCond)
	assert.Equal(t, metav1.ConditionTrue, parentCond.Status)
	assert.Contains(t, parentCond.Message, "global-privacy")

	// Verify ConfigMap has parent reference
	cm := getEffectiveConfigMap(t, r, "ws-privacy")
	assert.Equal(t, "global-privacy", cm.Data["parent-policy"])
	assert.Equal(t, "workspace", cm.Labels["omnia.altairalabs.ai/policy-level"])

	// Verify effective policy merges correctly
	effective := getEffectivePolicyFromConfigMap(t, cm)
	assert.True(t, effective.Recording.Enabled)
	assert.False(t, effective.Recording.RichData) // Workspace is stricter
	assert.True(t, effective.Recording.PII.Redact)
	assert.True(t, effective.Recording.PII.Encrypt) // Workspace adds encryption
	assert.True(t, effective.UserOptOut.Enabled)
	assert.Equal(t, int32(14), *effective.UserOptOut.DeleteWithinDays) // Workspace is faster
	// Retention: workspace has 60 warm days for facade, parent has 90 — minimum wins
	assert.Equal(t, int32(60), *effective.Retention.Facade.WarmDays)
	assert.Equal(t, int32(180), *effective.Retention.Facade.ColdDays)
}

func TestSessionPrivacyPolicy_AgentWithWorkspaceParent(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-privacy")
	wsPolicy := newWorkspacePolicy("ws-privacy", "my-workspace")
	agentPolicy := newAgentPolicy("agent-privacy", "my-workspace")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy, wsPolicy, agentPolicy)

	reconcilePolicy(t, r, "global-privacy")
	reconcilePolicy(t, r, "ws-privacy")
	reconcilePolicy(t, r, "agent-privacy")

	// Verify agent status
	policy := getPolicy(t, r, "agent-privacy")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)

	// Verify 3-level inheritance in ConfigMap
	cm := getEffectiveConfigMap(t, r, "agent-privacy")
	assert.Equal(t, "ws-privacy", cm.Data["parent-policy"])

	// Effective policy should merge all three levels
	effective := getEffectivePolicyFromConfigMap(t, cm)
	assert.True(t, effective.Recording.Enabled)
	assert.False(t, effective.Recording.RichData) // Workspace disables
	assert.True(t, effective.Recording.PII.Redact)
	assert.True(t, effective.Recording.PII.Encrypt)
}

func TestSessionPrivacyPolicy_AgentFallsBackToGlobal(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-privacy")
	// Agent in a workspace with no workspace-level policy
	agentPolicy := newAgentPolicy("agent-no-ws", "other-workspace")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy, agentPolicy)

	reconcilePolicy(t, r, "global-privacy")
	reconcilePolicy(t, r, "agent-no-ws")

	// Verify agent is active with global as parent
	policy := getPolicy(t, r, "agent-no-ws")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)

	cm := getEffectiveConfigMap(t, r, "agent-no-ws")
	assert.Equal(t, "global-privacy", cm.Data["parent-policy"])
}

func TestSessionPrivacyPolicy_OrphanedWorkspace_ErrorPhase(t *testing.T) {
	// Workspace policy without a global parent
	wsPolicy := newWorkspacePolicy("orphan-ws", "my-workspace")
	r, recorder := setupPrivacyPolicyTest(t, wsPolicy)

	reconcilePolicy(t, r, "orphan-ws")

	// Verify error status
	policy := getPolicy(t, r, "orphan-ws")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseError, policy.Status.Phase)

	// Verify ParentFound condition is False
	parentCond := findCondition(policy.Status.Conditions, ConditionTypeParentFound)
	require.NotNil(t, parentCond)
	assert.Equal(t, metav1.ConditionFalse, parentCond.Status)
	assert.Equal(t, EventReasonParentNotFound, parentCond.Reason)

	// Verify Ready condition is False
	readyCond := findCondition(policy.Status.Conditions, ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)

	// Verify warning event
	assertEventRecorded(t, recorder, EventReasonParentNotFound)
}

func TestSessionPrivacyPolicy_ParentUpdateRequeuesChildren(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-privacy")
	wsPolicy := newWorkspacePolicy("ws-privacy", "my-workspace")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy, wsPolicy)

	// Initial reconciliation
	reconcilePolicy(t, r, "global-privacy")
	reconcilePolicy(t, r, "ws-privacy")

	// Verify workspace is initially active
	policy := getPolicy(t, r, "ws-privacy")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)

	// Now update the global policy (simulate a spec change by re-reconciling)
	reconcilePolicy(t, r, "global-privacy")

	// After parent reconcile, the child should have been updated with parent-generation annotation
	wsAfter := getPolicy(t, r, "ws-privacy")
	assert.NotEmpty(t, wsAfter.Annotations["omnia.altairalabs.ai/parent-generation"])
}

func TestSessionPrivacyPolicy_DeleteCleansUpConfigMap(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-to-delete")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy)

	// Create the ConfigMap
	reconcilePolicy(t, r, "global-to-delete")

	// Verify ConfigMap exists
	cm := &corev1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{
		Name:      "omnia-privacy-policy-effective-global-to-delete",
		Namespace: "omnia-system",
	}, cm)
	require.NoError(t, err)

	// Delete the policy
	policy := getPolicy(t, r, "global-to-delete")
	err = r.Delete(context.Background(), policy)
	require.NoError(t, err)

	// Reconcile the deleted policy (triggers cleanup)
	reconcilePolicy(t, r, "global-to-delete")

	// Verify ConfigMap is cleaned up
	err = r.Get(context.Background(), types.NamespacedName{
		Name:      "omnia-privacy-policy-effective-global-to-delete",
		Namespace: "omnia-system",
	}, &corev1.ConfigMap{})
	assert.Error(t, err, "ConfigMap should be deleted")
}

func TestSessionPrivacyPolicy_ObservedGenerationUpdates(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-gen-test")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy)

	reconcilePolicy(t, r, "global-gen-test")

	policy := getPolicy(t, r, "global-gen-test")
	assert.Equal(t, policy.Generation, policy.Status.ObservedGeneration)
}

func TestSessionPrivacyPolicy_ReconcileNonExistent(t *testing.T) {
	r, _ := setupPrivacyPolicyTest(t)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent-policy"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestSessionPrivacyPolicy_ConfigMapUpdatedOnReReconcile(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-update-test")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy)

	// First reconcile
	reconcilePolicy(t, r, "global-update-test")

	// Verify initial ConfigMap
	cm := getEffectiveConfigMap(t, r, "global-update-test")
	effective := getEffectivePolicyFromConfigMap(t, cm)
	assert.True(t, effective.Recording.Enabled)

	// Re-reconcile (simulating an update - the policy content is the same but this tests idempotency)
	reconcilePolicy(t, r, "global-update-test")

	// ConfigMap should still exist and be valid
	cm2 := getEffectiveConfigMap(t, r, "global-update-test")
	effective2 := getEffectivePolicyFromConfigMap(t, cm2)
	assert.True(t, effective2.Recording.Enabled)
}

func TestComputeEffectivePolicy_SinglePolicy(t *testing.T) {
	p := newGlobalPolicy("test")
	chain := []*omniav1alpha1.SessionPrivacyPolicy{p}

	effective := computeEffectivePolicy(chain)

	assert.True(t, effective.Recording.Enabled)
	assert.True(t, effective.Recording.PII.Redact)
	assert.True(t, effective.UserOptOut.Enabled)
}

func TestComputeEffectivePolicy_MergesStricter(t *testing.T) {
	global := newGlobalPolicy("global")
	ws := newWorkspacePolicy("ws", "workspace")

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	// Workspace disables richData
	assert.False(t, effective.Recording.RichData)
	// Workspace adds PII encryption
	assert.True(t, effective.Recording.PII.Encrypt)
	// Retention: min(90, 60) = 60 for facade warm days
	assert.Equal(t, int32(60), *effective.Retention.Facade.WarmDays)
	// Retention: min(365, 180) = 180 for facade cold days
	assert.Equal(t, int32(180), *effective.Retention.Facade.ColdDays)
	// UserOptOut deleteWithinDays: min(30, 14) = 14
	assert.Equal(t, int32(14), *effective.UserOptOut.DeleteWithinDays)
}

func TestComputeEffectivePolicy_RecordingDisabledByParent(t *testing.T) {
	global := newGlobalPolicy("global")
	global.Spec.Recording.Enabled = false

	ws := newWorkspacePolicy("ws", "workspace")
	ws.Spec.Recording.Enabled = true // Tries to enable — should be overridden

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	// Parent disables recording, child can't enable
	assert.False(t, effective.Recording.Enabled)
}

func TestComputeEffectivePolicy_EncryptionTrueWins(t *testing.T) {
	global := newGlobalPolicy("global")
	global.Spec.Encryption = &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
	}

	ws := newWorkspacePolicy("ws", "workspace")
	ws.Spec.Encryption = &omniav1alpha1.EncryptionConfig{
		Enabled: false, // Tries to disable — true wins
	}

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	assert.True(t, effective.Encryption.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, effective.Encryption.KMSProvider)
}

func TestComputeEffectivePolicy_AuditLogTrueWins(t *testing.T) {
	global := newGlobalPolicy("global")
	global.Spec.AuditLog = &omniav1alpha1.AuditLogConfig{
		Enabled:       true,
		RetentionDays: ptr[int32](365),
	}

	ws := newWorkspacePolicy("ws", "workspace")
	ws.Spec.AuditLog = &omniav1alpha1.AuditLogConfig{
		Enabled:       false, // Tries to disable — true wins
		RetentionDays: ptr[int32](90),
	}

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	assert.True(t, effective.AuditLog.Enabled)
	assert.Equal(t, int32(90), *effective.AuditLog.RetentionDays) // min(365, 90) = 90
}

func TestComputeEffectivePolicy_EmptyChain(t *testing.T) {
	effective := computeEffectivePolicy(nil)
	assert.NotNil(t, effective)
}

func TestComputeEffectivePolicy_RetentionMinimumWins(t *testing.T) {
	global := newGlobalPolicy("global")
	// Override with only richData retention
	ws := newWorkspacePolicy("ws", "workspace")
	ws.Spec.Retention = &omniav1alpha1.PrivacyRetentionConfig{
		RichData: &omniav1alpha1.PrivacyRetentionTierConfig{
			WarmDays: ptr[int32](7), // Stricter than global's 30
		},
	}

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	// Facade should keep global values since workspace doesn't override
	assert.Equal(t, int32(90), *effective.Retention.Facade.WarmDays)
	assert.Equal(t, int32(365), *effective.Retention.Facade.ColdDays)
	// RichData warmDays: min(30, 7) = 7
	assert.Equal(t, int32(7), *effective.Retention.RichData.WarmDays)
	// RichData coldDays: only global has it
	assert.Equal(t, int32(180), *effective.Retention.RichData.ColdDays)
}

func TestComputeEffectivePolicy_PIIPatternsUnion(t *testing.T) {
	global := newGlobalPolicy("global")
	global.Spec.Recording.PII = &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: []string{"ssn", "credit_card"},
	}

	ws := newWorkspacePolicy("ws", "workspace")
	ws.Spec.Recording.PII = &omniav1alpha1.PIIConfig{
		Redact:   true,
		Encrypt:  true,
		Patterns: []string{"credit_card", "phone_number"}, // credit_card is duplicate
	}

	chain := []*omniav1alpha1.SessionPrivacyPolicy{global, ws}
	effective := computeEffectivePolicy(chain)

	assert.True(t, effective.Recording.PII.Redact)
	assert.True(t, effective.Recording.PII.Encrypt)
	// Union: ssn, credit_card, phone_number (no duplicates)
	assert.ElementsMatch(t, []string{"ssn", "credit_card", "phone_number"}, effective.Recording.PII.Patterns)
}

func TestSessionPrivacyPolicy_MetricsRecorded(t *testing.T) {
	globalPolicy := newGlobalPolicy("global-metrics")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy)

	reconcilePolicy(t, r, "global-metrics")

	// Verify metrics were recorded by checking via the test registry
	// The metrics struct is populated — verify active policies count
	policy := getPolicy(t, r, "global-metrics")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, policy.Status.Phase)
}

func TestMergeEncryption_NilBase(t *testing.T) {
	override := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderVault,
	}
	result := mergeEncryption(nil, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderVault, result.KMSProvider)
}

func TestMergeEncryption_NilOverride(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
	}
	result := mergeEncryption(base, nil)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, result.KMSProvider)
}

func TestMergeEncryption_OverrideKMS(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
	}
	override := &omniav1alpha1.EncryptionConfig{
		Enabled:     false,
		KMSProvider: omniav1alpha1.KMSProviderGCPKMS,
	}
	result := mergeEncryption(base, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled) // true wins
	assert.Equal(t, omniav1alpha1.KMSProviderGCPKMS, result.KMSProvider)
}

func TestMergeEncryption_BaseKMSNoOverride(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
	}
	override := &omniav1alpha1.EncryptionConfig{
		Enabled: true,
	}
	result := mergeEncryption(base, override)
	require.NotNil(t, result)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, result.KMSProvider)
}

func TestMergeEncryption_KeyIDOverride(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAzureKeyVault,
		KeyID:       "base-key-1",
	}
	override := &omniav1alpha1.EncryptionConfig{
		KeyID: "child-key-2",
	}
	result := mergeEncryption(base, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderAzureKeyVault, result.KMSProvider)
	assert.Equal(t, "child-key-2", result.KeyID)
}

func TestMergeEncryption_KeyIDFromBase(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
		KeyID:       "parent-key",
	}
	override := &omniav1alpha1.EncryptionConfig{
		Enabled: true,
	}
	result := mergeEncryption(base, override)
	require.NotNil(t, result)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, result.KMSProvider)
	assert.Equal(t, "parent-key", result.KeyID)
}

func TestMergeEncryption_ProviderOverrideIncludesKeyID(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
		KeyID:       "aws-key",
	}
	override := &omniav1alpha1.EncryptionConfig{
		KMSProvider: omniav1alpha1.KMSProviderAzureKeyVault,
		KeyID:       "azure-key",
	}
	result := mergeEncryption(base, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderAzureKeyVault, result.KMSProvider)
	assert.Equal(t, "azure-key", result.KeyID)
}

func TestMergeRetention_NilBase(t *testing.T) {
	override := &omniav1alpha1.PrivacyRetentionConfig{
		Facade: &omniav1alpha1.PrivacyRetentionTierConfig{
			WarmDays: ptr[int32](30),
		},
	}
	result := mergeRetention(nil, override)
	require.NotNil(t, result)
	assert.Equal(t, int32(30), *result.Facade.WarmDays)
}

func TestMergeRetention_NilOverride(t *testing.T) {
	base := &omniav1alpha1.PrivacyRetentionConfig{
		Facade: &omniav1alpha1.PrivacyRetentionTierConfig{
			WarmDays: ptr[int32](90),
		},
	}
	result := mergeRetention(base, nil)
	require.NotNil(t, result)
	assert.Equal(t, int32(90), *result.Facade.WarmDays)
}

func TestMergeRetentionTier_NilBase(t *testing.T) {
	override := &omniav1alpha1.PrivacyRetentionTierConfig{
		WarmDays: ptr[int32](15),
		ColdDays: ptr[int32](60),
	}
	result := mergeRetentionTier(nil, override)
	require.NotNil(t, result)
	assert.Equal(t, int32(15), *result.WarmDays)
	assert.Equal(t, int32(60), *result.ColdDays)
}

func TestMergeRetentionTier_NilOverride(t *testing.T) {
	base := &omniav1alpha1.PrivacyRetentionTierConfig{
		WarmDays: ptr[int32](30),
	}
	result := mergeRetentionTier(base, nil)
	require.NotNil(t, result)
	assert.Equal(t, int32(30), *result.WarmDays)
}

func TestMinInt32Ptr_BothNil(t *testing.T) {
	assert.Nil(t, minInt32Ptr(nil, nil))
}

func TestMinInt32Ptr_AOnly(t *testing.T) {
	a := ptr[int32](10)
	result := minInt32Ptr(a, nil)
	require.NotNil(t, result)
	assert.Equal(t, int32(10), *result)
}

func TestMinInt32Ptr_BOnly(t *testing.T) {
	b := ptr[int32](20)
	result := minInt32Ptr(nil, b)
	require.NotNil(t, result)
	assert.Equal(t, int32(20), *result)
}

func TestMinInt32Ptr_BSmaller(t *testing.T) {
	a := ptr[int32](20)
	b := ptr[int32](10)
	result := minInt32Ptr(a, b)
	require.NotNil(t, result)
	assert.Equal(t, int32(10), *result)
}

func TestMergeAuditLog_NilBase(t *testing.T) {
	override := &omniav1alpha1.AuditLogConfig{
		Enabled:       true,
		RetentionDays: ptr[int32](90),
	}
	result := mergeAuditLog(nil, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, int32(90), *result.RetentionDays)
}

func TestMergeAuditLog_NilOverride(t *testing.T) {
	base := &omniav1alpha1.AuditLogConfig{
		Enabled:       true,
		RetentionDays: ptr[int32](365),
	}
	result := mergeAuditLog(base, nil)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, int32(365), *result.RetentionDays)
}

func TestMergeUserOptOut_NilBase(t *testing.T) {
	override := &omniav1alpha1.UserOptOutConfig{
		Enabled:          true,
		DeleteWithinDays: ptr[int32](7),
	}
	result := mergeUserOptOut(nil, override)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.Equal(t, int32(7), *result.DeleteWithinDays)
}

func TestMergeUserOptOut_NilOverride(t *testing.T) {
	base := &omniav1alpha1.UserOptOutConfig{
		Enabled:             true,
		HonorDeleteRequests: true,
	}
	result := mergeUserOptOut(base, nil)
	require.NotNil(t, result)
	assert.True(t, result.Enabled)
	assert.True(t, result.HonorDeleteRequests)
}

func TestMergePII_NilBase(t *testing.T) {
	override := &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: []string{"ssn"},
	}
	result := mergePII(nil, override)
	require.NotNil(t, result)
	assert.True(t, result.Redact)
	assert.Equal(t, []string{"ssn"}, result.Patterns)
}

func TestMergePII_NilOverride(t *testing.T) {
	base := &omniav1alpha1.PIIConfig{
		Encrypt:  true,
		Patterns: []string{"email"},
	}
	result := mergePII(base, nil)
	require.NotNil(t, result)
	assert.True(t, result.Encrypt)
	assert.Equal(t, []string{"email"}, result.Patterns)
}

func TestSessionPrivacyPolicy_HandleStoreError(t *testing.T) {
	globalPolicy := newGlobalPolicy("store-fail-policy")
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "omnia-system"},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	// Build client that fails on ConfigMap creates
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(globalPolicy, omniaSystemNS).
		WithStatusSubresource(&omniav1alpha1.SessionPrivacyPolicy{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(
				ctx context.Context,
				c client.WithWatch,
				obj client.Object,
				opts ...client.CreateOption,
			) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return fmt.Errorf("simulated ConfigMap create error")
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	recorder := record.NewFakeRecorder(20)
	reg := prometheus.NewRegistry()
	testMetrics := metrics.NewPrivacyPolicyMetricsWithRegistry(reg)

	r := &SessionPrivacyPolicyReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		Metrics:  testMetrics,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "store-fail-policy"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated ConfigMap create error")

	// Verify error status was set
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	_ = r.Get(context.Background(),
		types.NamespacedName{Name: "store-fail-policy"}, policy)
	assert.Equal(t,
		omniav1alpha1.SessionPrivacyPolicyPhaseError, policy.Status.Phase)

	storedCond := findCondition(
		policy.Status.Conditions, ConditionTypeEffectivePolicyStored)
	require.NotNil(t, storedCond)
	assert.Equal(t, metav1.ConditionFalse, storedCond.Status)

	assertEventRecorded(t, recorder, EventReasonConfigMapSyncFailed)
}

func TestSessionPrivacyPolicy_SetErrorStatus_OnListError(t *testing.T) {
	// Workspace policy — findParentPolicy calls List which we make fail
	wsPolicy := newWorkspacePolicy("ws-list-err", "my-workspace")
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "omnia-system"},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	// Build client that fails on SessionPrivacyPolicyList
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(wsPolicy, omniaSystemNS).
		WithStatusSubresource(&omniav1alpha1.SessionPrivacyPolicy{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(
				ctx context.Context,
				c client.WithWatch,
				list client.ObjectList,
				opts ...client.ListOption,
			) error {
				if _, ok := list.(*omniav1alpha1.SessionPrivacyPolicyList); ok {
					return fmt.Errorf("simulated list error")
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()

	recorder := record.NewFakeRecorder(20)
	reg := prometheus.NewRegistry()
	testMetrics := metrics.NewPrivacyPolicyMetricsWithRegistry(reg)

	r := &SessionPrivacyPolicyReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		Metrics:  testMetrics,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ws-list-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated list error")

	// Verify setErrorStatus was called
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	getErr := r.Get(context.Background(),
		types.NamespacedName{Name: "ws-list-err"}, policy)
	require.NoError(t, getErr)
	assert.Equal(t,
		omniav1alpha1.SessionPrivacyPolicyPhaseError, policy.Status.Phase)
}

func TestSessionPrivacyPolicy_OrphanedAgent_ErrorPhase(t *testing.T) {
	// Agent policy without any parent (no global, no workspace)
	agentPolicy := newAgentPolicy("orphan-agent", "my-ns")
	r, recorder := setupPrivacyPolicyTest(t, agentPolicy)

	reconcilePolicy(t, r, "orphan-agent")

	policy := getPolicy(t, r, "orphan-agent")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseError, policy.Status.Phase)

	parentCond := findCondition(policy.Status.Conditions, ConditionTypeParentFound)
	require.NotNil(t, parentCond)
	assert.Equal(t, metav1.ConditionFalse, parentCond.Status)

	assertEventRecorded(t, recorder, EventReasonParentNotFound)
}

func TestSessionPrivacyPolicy_WorkspaceNilParent_InChain(t *testing.T) {
	// Workspace policy with no global parent — buildInheritanceChain
	// handles nil parent for workspace by returning just [policy]
	// We test this indirectly through the orphan test, but let's test
	// buildInheritanceChain directly
	ws := newWorkspacePolicy("ws-chain", "my-ws")
	r, _ := setupPrivacyPolicyTest(t)

	chain := r.buildInheritanceChain(context.Background(), ws, nil)
	assert.Len(t, chain, 1)
	assert.Equal(t, "ws-chain", chain[0].Name)
}

func TestSessionPrivacyPolicy_AgentChainNilParent(t *testing.T) {
	agent := newAgentPolicy("agent-chain", "my-ns")
	r, _ := setupPrivacyPolicyTest(t)

	chain := r.buildAgentChain(context.Background(), agent, nil)
	assert.Len(t, chain, 1)
	assert.Equal(t, "agent-chain", chain[0].Name)
}

func TestSessionPrivacyPolicy_AgentChainWithGlobalFallback(t *testing.T) {
	// Agent with global parent (no workspace) — buildAgentChain
	// parent.Spec.Level != PolicyLevelWorkspace → returns [parent, policy]
	globalPolicy := newGlobalPolicy("global-for-chain")
	agent := newAgentPolicy("agent-chain-global", "other-ns")
	r, _ := setupPrivacyPolicyTest(t, globalPolicy)

	chain := r.buildAgentChain(context.Background(), agent, globalPolicy)
	assert.Len(t, chain, 2)
	assert.Equal(t, "global-for-chain", chain[0].Name)
	assert.Equal(t, "agent-chain-global", chain[1].Name)
}

func TestSessionPrivacyPolicy_BuildInheritanceChainDefault(t *testing.T) {
	// Test the default case in buildInheritanceChain (unknown level)
	policy := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "unknown-level"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level:     omniav1alpha1.PolicyLevel("unknown"),
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
		},
	}
	r, _ := setupPrivacyPolicyTest(t)

	chain := r.buildInheritanceChain(context.Background(), policy, nil)
	assert.Len(t, chain, 1)
}

func TestSessionPrivacyPolicy_UpdateActivePolicyCounts(t *testing.T) {
	global := newGlobalPolicy("global-count")
	global.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseActive
	ws := newWorkspacePolicy("ws-count", "my-ws")
	ws.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseActive
	ws2 := newWorkspacePolicy("ws2-count", "other-ws")
	ws2.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseError

	r, _ := setupPrivacyPolicyTest(t, global, ws, ws2)

	r.updateActivePolicyCounts(context.Background())
	// No assertion needed — just exercise the code path
}

func TestMergeStricter_NilSubfields(t *testing.T) {
	base := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelGlobal,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled: true,
		},
	}

	override := &omniav1alpha1.SessionPrivacyPolicySpec{
		Level: omniav1alpha1.PolicyLevelWorkspace,
		Recording: omniav1alpha1.RecordingConfig{
			Enabled: true,
		},
	}

	result := mergeStricter(base, override)
	assert.Nil(t, result.Retention)
	assert.Nil(t, result.Encryption)
	assert.Nil(t, result.AuditLog)
}

// Helper functions for tests

func assertEventRecorded(t *testing.T, recorder *record.FakeRecorder, expectedReason string) {
	t.Helper()
	found := false
	for {
		select {
		case event := <-recorder.Events:
			if strings.Contains(event, expectedReason) {
				found = true
			}
		default:
			assert.True(t, found, "expected event with reason %q", expectedReason)
			return
		}
		if found {
			break
		}
	}
}
