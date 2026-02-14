/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// mockAnalyticsProviderFactory implements AnalyticsProviderFactory for testing.
type mockAnalyticsProviderFactory struct {
	pingErr error
}

func (m *mockAnalyticsProviderFactory) Ping(_ context.Context, _ corev1alpha1.SessionAnalyticsSyncSpec) error {
	return m.pingErr
}

func setupAnalyticsSyncTest(
	t *testing.T, factory AnalyticsProviderFactory, objects ...runtime.Object,
) (*SessionAnalyticsSyncReconciler, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objects {
		builder = builder.WithRuntimeObjects(obj)
	}
	builder = builder.WithStatusSubresource(&corev1alpha1.SessionAnalyticsSync{})
	fakeClient := builder.Build()

	recorder := record.NewFakeRecorder(20)
	reconciler := &SessionAnalyticsSyncReconciler{
		Client:          fakeClient,
		Scheme:          scheme,
		Recorder:        recorder,
		ProviderFactory: factory,
	}
	return reconciler, recorder
}

func newSnowflakeSync(name string, enabled *bool) *corev1alpha1.SessionAnalyticsSync {
	return &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Enabled:  enabled,
			Provider: corev1alpha1.AnalyticsProviderSnowflake,
			Snowflake: &corev1alpha1.SnowflakeConfig{
				Account:   "xy12345.us-east-1",
				Database:  "analytics",
				Warehouse: "COMPUTE_WH",
				SecretRef: corev1.LocalObjectReference{Name: "snowflake-creds"},
			},
			Sync: corev1alpha1.SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}
}

func newBigQuerySync(name string) *corev1alpha1.SessionAnalyticsSync {
	return &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: corev1alpha1.AnalyticsProviderBigQuery,
			BigQuery: &corev1alpha1.BigQueryConfig{
				ProjectID: "my-project",
				Dataset:   "omnia_sessions",
				SecretRef: corev1.LocalObjectReference{Name: "gcp-creds"},
			},
			Sync: corev1alpha1.SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}
}

func newClickHouseSync(name string) *corev1alpha1.SessionAnalyticsSync {
	return &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: corev1alpha1.AnalyticsProviderClickHouse,
			ClickHouse: &corev1alpha1.ClickHouseConfig{
				Hosts:    []string{"clickhouse:9000"},
				Database: "omnia",
				Auth: corev1alpha1.ClickHouseAuth{
					SecretRef: corev1.LocalObjectReference{Name: "ch-creds"},
				},
			},
			Sync: corev1alpha1.SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}
}

func TestAnalyticsSyncReconcile_SnowflakeValid(t *testing.T) {
	syncObj := newSnowflakeSync("test-snowflake", nil)
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-snowflake"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-snowflake"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)
	assert.Equal(t, int64(1), updated.Status.ObservedGeneration)

	readyCond := findAnalyticsSyncCondition(updated.Status.Conditions, conditionTypeAnalyticsReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)

	connCond := findAnalyticsSyncCondition(updated.Status.Conditions, conditionTypeAnalyticsConnected)
	require.NotNil(t, connCond)
	assert.Equal(t, metav1.ConditionTrue, connCond.Status)
}

func TestAnalyticsSyncReconcile_BigQueryValid(t *testing.T) {
	syncObj := newBigQuerySync("test-bigquery")
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-bigquery"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-bigquery"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_ClickHouseValid(t *testing.T) {
	syncObj := newClickHouseSync("test-clickhouse")
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-clickhouse"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-clickhouse"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_Disabled(t *testing.T) {
	enabled := false
	syncObj := newSnowflakeSync("test-disabled", &enabled)
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-disabled"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-disabled"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)

	connCond := findAnalyticsSyncCondition(updated.Status.Conditions, conditionTypeAnalyticsConnected)
	require.NotNil(t, connCond)
	assert.Equal(t, metav1.ConditionFalse, connCond.Status)
}

func TestAnalyticsSyncReconcile_MissingProviderConfig(t *testing.T) {
	syncObj := &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-missing-config",
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: corev1alpha1.AnalyticsProviderSnowflake,
			Sync: corev1alpha1.SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-missing-config"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-missing-config"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseError, updated.Status.Phase)

	provCond := findAnalyticsSyncCondition(updated.Status.Conditions, conditionTypeAnalyticsProviderConfigured)
	require.NotNil(t, provCond)
	assert.Equal(t, metav1.ConditionFalse, provCond.Status)
}

func TestAnalyticsSyncReconcile_MissingBigQueryConfig(t *testing.T) {
	syncObj := &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-missing-bq",
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: corev1alpha1.AnalyticsProviderBigQuery,
			Sync:     corev1alpha1.SyncConfig{Schedule: "0 3 * * *"},
		},
	}
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-missing-bq"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-missing-bq"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseError, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_MissingClickHouseConfig(t *testing.T) {
	syncObj := &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-missing-ch",
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: corev1alpha1.AnalyticsProviderClickHouse,
			Sync:     corev1alpha1.SyncConfig{Schedule: "0 3 * * *"},
		},
	}
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-missing-ch"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-missing-ch"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseError, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_UnsupportedProvider(t *testing.T) {
	syncObj := &corev1alpha1.SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-unsupported",
			Generation: 1,
		},
		Spec: corev1alpha1.SessionAnalyticsSyncSpec{
			Provider: "unknown",
			Sync:     corev1alpha1.SyncConfig{Schedule: "0 3 * * *"},
		},
	}
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-unsupported"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-unsupported"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseError, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_NotFound(t *testing.T) {
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestAnalyticsSyncReconcile_ConnectivityFailure(t *testing.T) {
	syncObj := newSnowflakeSync("test-conn-fail", nil)
	factory := &mockAnalyticsProviderFactory{
		pingErr: fmt.Errorf("connection refused"),
	}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-conn-fail"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-conn-fail"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseError, updated.Status.Phase)

	connCond := findAnalyticsSyncCondition(updated.Status.Conditions, conditionTypeAnalyticsConnected)
	require.NotNil(t, connCond)
	assert.Equal(t, metav1.ConditionFalse, connCond.Status)
	assert.Contains(t, connCond.Message, "connection refused")
}

func TestAnalyticsSyncReconcile_NilProviderFactory(t *testing.T) {
	syncObj := newSnowflakeSync("test-nil-factory", nil)
	reconciler, _ := setupAnalyticsSyncTest(t, nil, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-nil-factory"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-nil-factory"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_EnabledExplicitlyTrue(t *testing.T) {
	enabled := true
	syncObj := newSnowflakeSync("test-explicit-true", &enabled)
	factory := &mockAnalyticsProviderFactory{}
	reconciler, _ := setupAnalyticsSyncTest(t, factory, syncObj)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-explicit-true"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &corev1alpha1.SessionAnalyticsSync{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-explicit-true"}, updated)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.SessionAnalyticsSyncPhaseActive, updated.Status.Phase)
}

func TestAnalyticsSyncReconcile_EventsEmitted(t *testing.T) {
	syncObj := newSnowflakeSync("test-events", nil)
	factory := &mockAnalyticsProviderFactory{}
	reconciler, recorder := setupAnalyticsSyncTest(t, factory, syncObj)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-events"},
	})
	require.NoError(t, err)

	// Check that an event was emitted
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, eventReasonAnalyticsConfigValidated)
	default:
		t.Error("expected an event to be emitted")
	}
}

func TestIsSyncEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil means enabled", nil, true},
		{"explicit true", ptr(true), true},
		{"explicit false", ptr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncObj := &corev1alpha1.SessionAnalyticsSync{
				Spec: corev1alpha1.SessionAnalyticsSyncSpec{Enabled: tt.enabled},
			}
			assert.Equal(t, tt.want, isSyncEnabled(syncObj))
		})
	}
}

// findAnalyticsSyncCondition finds a condition by type in a conditions slice.
func findAnalyticsSyncCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
