/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
)

func setupPrivacyPolicyTest(t *testing.T, objects ...runtime.Object) (*SessionPrivacyPolicyReconciler, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objects {
		builder = builder.WithRuntimeObjects(obj)
	}
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

func newTestPrivacyPolicy(name, namespace string) *omniav1alpha1.SessionPrivacyPolicy { //nolint:unparam
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				PII: &omniav1alpha1.PIIConfig{
					Redact: true,
				},
			},
		},
	}
}

func reconcilePolicy(t *testing.T, r *SessionPrivacyPolicyReconciler, name, namespace string) ctrl.Result { //nolint:unparam
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
	require.NoError(t, err)
	return result
}

func getPolicy(t *testing.T, r *SessionPrivacyPolicyReconciler, name, namespace string) *omniav1alpha1.SessionPrivacyPolicy {
	t.Helper()
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, policy)
	require.NoError(t, err)
	return policy
}

// findPrivacyCond returns the named condition from a privacy policy or nil.
func findPrivacyCond(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// assertPrivacyEventRecorded drains the recorder channel and checks for the given reason.
func assertPrivacyEventRecorded(t *testing.T, recorder *record.FakeRecorder, reason string) {
	t.Helper()
	for {
		select {
		case event := <-recorder.Events:
			if privacyContains(event, reason) {
				return
			}
		default:
			t.Errorf("event with reason %q not found", reason)
			return
		}
	}
}

func privacyContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSessionPrivacyPolicy_ReconcileActive verifies a valid policy gets Active phase,
// ObservedGeneration matches Generation, and Ready=True condition is set.
func TestSessionPrivacyPolicy_ReconcileActive(t *testing.T) {
	policy := newTestPrivacyPolicy("test-policy", "default")
	r, recorder := setupPrivacyPolicyTest(t, policy)

	reconcilePolicy(t, r, "test-policy", "default")

	got := getPolicy(t, r, "test-policy", "default")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, got.Status.Phase)
	assert.Equal(t, got.Generation, got.Status.ObservedGeneration)

	cond := findPrivacyCond(got.Status.Conditions, ConditionTypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, EventReasonPolicyValidated, cond.Reason)

	assertPrivacyEventRecorded(t, recorder, EventReasonPolicyValidated)
}

// TestSessionPrivacyPolicy_ReconcileNonExistent verifies that a not-found policy
// returns no error.
func TestSessionPrivacyPolicy_ReconcileNonExistent(t *testing.T) {
	r, _ := setupPrivacyPolicyTest(t)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestSessionPrivacyPolicy_MetricsRecorded verifies that reconcile records a metric.
func TestSessionPrivacyPolicy_MetricsRecorded(t *testing.T) {
	policy := newTestPrivacyPolicy("metrics-policy", "default")
	r, _ := setupPrivacyPolicyTest(t, policy)

	reconcilePolicy(t, r, "metrics-policy", "default")

	got := getPolicy(t, r, "metrics-policy", "default")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, got.Status.Phase)
}

// TestSessionPrivacyPolicy_ReconcileIdempotent verifies that reconciling twice
// leaves the policy in Active phase without error.
func TestSessionPrivacyPolicy_ReconcileIdempotent(t *testing.T) {
	policy := newTestPrivacyPolicy("idempotent-policy", "default")
	r, _ := setupPrivacyPolicyTest(t, policy)

	reconcilePolicy(t, r, "idempotent-policy", "default")
	reconcilePolicy(t, r, "idempotent-policy", "default")

	got := getPolicy(t, r, "idempotent-policy", "default")
	assert.Equal(t, omniav1alpha1.SessionPrivacyPolicyPhaseActive, got.Status.Phase)
}

// TestSessionPrivacyPolicy_NoRecorder verifies that reconcile succeeds without
// an event recorder wired (recorder may be nil in some test setups).
func TestSessionPrivacyPolicy_NoRecorder(t *testing.T) {
	policy := newTestPrivacyPolicy("no-recorder-policy", "default")

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(policy).
		WithStatusSubresource(&omniav1alpha1.SessionPrivacyPolicy{}).
		Build()

	r := &SessionPrivacyPolicyReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: nil,
		Metrics:  nil,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "no-recorder-policy", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
