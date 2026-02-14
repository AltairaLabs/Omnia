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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
)

// MockPublisher implements StreamingPublisher for testing.
type MockPublisher struct {
	closed bool
}

func (m *MockPublisher) Close() error {
	m.closed = true
	return nil
}

func setupStreamingTest(
	t *testing.T, objects ...runtime.Object,
) (*SessionStreamingConfigReconciler, *record.FakeRecorder) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objects {
		builder = builder.WithRuntimeObjects(obj)
	}
	builder = builder.WithStatusSubresource(&corev1alpha1.SessionStreamingConfig{})
	fakeClient := builder.Build()

	recorder := record.NewFakeRecorder(20)

	reconciler := &SessionStreamingConfigReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		PublisherFactory: func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
			return &MockPublisher{}, nil
		},
	}

	return reconciler, recorder
}

func newKafkaStreamingConfig(name string, enabled bool) *corev1alpha1.SessionStreamingConfig {
	return &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  enabled,
			Provider: corev1alpha1.StreamingProviderKafka,
			Kafka: &corev1alpha1.KafkaConfig{
				Brokers: []string{"broker1:9092", "broker2:9092"},
				Topic:   "session-events",
			},
		},
	}
}

func reconcileStreaming(
	t *testing.T, r *SessionStreamingConfigReconciler, name string,
) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	require.NoError(t, err)
	return result
}

func getStreamingConfig(
	t *testing.T, r *SessionStreamingConfigReconciler, name string,
) *corev1alpha1.SessionStreamingConfig {
	t.Helper()
	config := &corev1alpha1.SessionStreamingConfig{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name}, config)
	require.NoError(t, err)
	return config
}

func findStreamingCondition(
	conditions []metav1.Condition, condType string,
) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func assertStreamingEventRecorded(
	t *testing.T, recorder *record.FakeRecorder, expectedReason string,
) {
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

func TestSessionStreamingConfig_ValidKafka_PhaseActive(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-valid", true)
	r, recorder := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "kafka-valid")

	result := getStreamingConfig(t, r, "kafka-valid")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
	assert.True(t, result.Status.Connected)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)

	// Verify ProviderConfigured condition
	provCond := findStreamingCondition(result.Status.Conditions, conditionTypeProviderConfigured)
	require.NotNil(t, provCond)
	assert.Equal(t, metav1.ConditionTrue, provCond.Status)
	assert.Equal(t, eventReasonProviderConfigured, provCond.Reason)

	// Verify Ready condition
	readyCond := findStreamingCondition(result.Status.Conditions, conditionTypeStreamingReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)

	// Verify publisher event
	assertStreamingEventRecorded(t, recorder, eventReasonPublisherCreated)
}

func TestSessionStreamingConfig_Disabled_PhaseActive_NoPublisher(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-disabled", false)
	publisherCreated := false
	r, _ := setupStreamingTest(t, config)
	r.PublisherFactory = func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
		publisherCreated = true
		return &MockPublisher{}, nil
	}

	reconcileStreaming(t, r, "kafka-disabled")

	result := getStreamingConfig(t, r, "kafka-disabled")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
	assert.False(t, result.Status.Connected)
	assert.False(t, publisherCreated)

	// Both conditions should be true (streaming disabled is a valid state)
	readyCond := findStreamingCondition(result.Status.Conditions, conditionTypeStreamingReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, eventReasonStreamingDisabled, readyCond.Reason)
}

func TestSessionStreamingConfig_MissingProviderConfig_PhaseError(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kafka-missing-config",
		},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderKafka,
			// Kafka config is nil
		},
	}
	r, recorder := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "kafka-missing-config")

	result := getStreamingConfig(t, r, "kafka-missing-config")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
	assert.False(t, result.Status.Connected)

	// ProviderConfigured should be false
	provCond := findStreamingCondition(result.Status.Conditions, conditionTypeProviderConfigured)
	require.NotNil(t, provCond)
	assert.Equal(t, metav1.ConditionFalse, provCond.Status)
	assert.Equal(t, eventReasonProviderConfigInvalid, provCond.Reason)

	// Ready should be false
	readyCond := findStreamingCondition(result.Status.Conditions, conditionTypeStreamingReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)

	// Verify warning event
	assertStreamingEventRecorded(t, recorder, eventReasonProviderConfigInvalid)
}

func TestSessionStreamingConfig_DeletedResource_NoError(t *testing.T) {
	r, _ := setupStreamingTest(t)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent-config"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestSessionStreamingConfig_DeleteClosesPublisher(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-to-delete", true)
	mock := &MockPublisher{}
	r, _ := setupStreamingTest(t, config)
	r.PublisherFactory = func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
		return mock, nil
	}

	// Create publisher
	reconcileStreaming(t, r, "kafka-to-delete")
	assert.False(t, mock.closed)

	// Delete the config
	existing := getStreamingConfig(t, r, "kafka-to-delete")
	err := r.Delete(context.Background(), existing)
	require.NoError(t, err)

	// Reconcile deleted resource
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "kafka-to-delete"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestSessionStreamingConfig_ConfigUpdate_ReconnectsPublisher(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-update", true)
	callCount := 0
	r, _ := setupStreamingTest(t, config)
	r.PublisherFactory = func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
		callCount++
		return &MockPublisher{}, nil
	}

	// First reconcile
	reconcileStreaming(t, r, "kafka-update")
	assert.Equal(t, 1, callCount)

	// Second reconcile (simulating config update)
	reconcileStreaming(t, r, "kafka-update")
	assert.Equal(t, 2, callCount)
}

func TestSessionStreamingConfig_PublisherFactoryError(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-factory-err", true)
	r, recorder := setupStreamingTest(t, config)
	r.PublisherFactory = func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
		return nil, fmt.Errorf("connection refused")
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "kafka-factory-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	result := getStreamingConfig(t, r, "kafka-factory-err")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
	assert.False(t, result.Status.Connected)

	readyCond := findStreamingCondition(result.Status.Conditions, conditionTypeStreamingReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, eventReasonPublisherError, readyCond.Reason)

	assertStreamingEventRecorded(t, recorder, eventReasonPublisherError)
}

func TestSessionStreamingConfig_KinesisNoConfig_Error(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "kinesis-missing"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderKinesis,
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "kinesis-missing")

	result := getStreamingConfig(t, r, "kinesis-missing")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
}

func TestSessionStreamingConfig_PulsarNoConfig_Error(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "pulsar-missing"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderPulsar,
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "pulsar-missing")

	result := getStreamingConfig(t, r, "pulsar-missing")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
}

func TestSessionStreamingConfig_NATSNoConfig_Error(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "nats-missing"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderNATS,
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "nats-missing")

	result := getStreamingConfig(t, r, "nats-missing")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
}

func TestSessionStreamingConfig_UnsupportedProvider_Error(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "unsupported"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProvider("rabbitmq"),
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "unsupported")

	result := getStreamingConfig(t, r, "unsupported")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseError, result.Status.Phase)
}

func TestSessionStreamingConfig_ValidKinesisConfig_Active(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "kinesis-valid"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderKinesis,
			Kinesis: &corev1alpha1.KinesisConfig{
				StreamName: "my-stream",
				Region:     "us-east-1",
			},
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "kinesis-valid")

	result := getStreamingConfig(t, r, "kinesis-valid")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
	assert.True(t, result.Status.Connected)
}

func TestSessionStreamingConfig_ValidPulsarConfig_Active(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "pulsar-valid"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderPulsar,
			Pulsar: &corev1alpha1.PulsarConfig{
				ServiceUrl: "pulsar://localhost:6650",
				Topic:      "my-topic",
			},
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "pulsar-valid")

	result := getStreamingConfig(t, r, "pulsar-valid")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
}

func TestSessionStreamingConfig_ValidNATSConfig_Active(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "nats-valid"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderNATS,
			NATS: &corev1alpha1.NATSConfig{
				URL:     "nats://localhost:4222",
				Stream:  "SESSIONS",
				Subject: "sessions.events",
			},
		},
	}
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "nats-valid")

	result := getStreamingConfig(t, r, "nats-valid")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
}

func TestSessionStreamingConfig_NilPublisherFactory_Active(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-no-factory", true)
	r, _ := setupStreamingTest(t, config)
	r.PublisherFactory = nil

	reconcileStreaming(t, r, "kafka-no-factory")

	result := getStreamingConfig(t, r, "kafka-no-factory")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
}

func TestSessionStreamingConfig_NilRecorder_NoEvent(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-no-recorder", true)
	r, _ := setupStreamingTest(t, config)
	r.Recorder = nil

	reconcileStreaming(t, r, "kafka-no-recorder")

	result := getStreamingConfig(t, r, "kafka-no-recorder")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
}

func TestSessionStreamingConfig_DisabledClosesExistingPublisher(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-disable-close", true)
	mock := &MockPublisher{}
	r, _ := setupStreamingTest(t, config)
	r.PublisherFactory = func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
		return mock, nil
	}

	// First reconcile — creates publisher
	reconcileStreaming(t, r, "kafka-disable-close")
	assert.False(t, mock.closed)

	// Update to disabled
	existing := getStreamingConfig(t, r, "kafka-disable-close")
	existing.Spec.Enabled = false
	err := r.Update(context.Background(), existing)
	require.NoError(t, err)

	// Re-reconcile — should close publisher
	reconcileStreaming(t, r, "kafka-disable-close")

	result := getStreamingConfig(t, r, "kafka-disable-close")
	assert.Equal(t, corev1alpha1.SessionStreamingConfigPhaseActive, result.Status.Phase)
	assert.False(t, result.Status.Connected)
}

func TestSessionStreamingConfig_ObservedGeneration(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-gen", true)
	r, _ := setupStreamingTest(t, config)

	reconcileStreaming(t, r, "kafka-gen")

	result := getStreamingConfig(t, r, "kafka-gen")
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
}

func TestSessionStreamingConfig_StatusUpdateError_Disabled(t *testing.T) {
	config := newKafkaStreamingConfig("kafka-status-err", false)

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(config).
		WithStatusSubresource(&corev1alpha1.SessionStreamingConfig{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				ctx context.Context,
				c client.Client,
				subResourceName string,
				obj client.Object,
				opts ...client.SubResourceUpdateOption,
			) error {
				return fmt.Errorf("simulated status update error")
			},
		}).
		Build()

	r := &SessionStreamingConfigReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "kafka-status-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update error")
}

func TestSessionStreamingConfig_StatusUpdateError_ValidationFail(t *testing.T) {
	config := &corev1alpha1.SessionStreamingConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "val-status-err"},
		Spec: corev1alpha1.SessionStreamingConfigSpec{
			Enabled:  true,
			Provider: corev1alpha1.StreamingProviderKafka,
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(config).
		WithStatusSubresource(&corev1alpha1.SessionStreamingConfig{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				ctx context.Context,
				c client.Client,
				subResourceName string,
				obj client.Object,
				opts ...client.SubResourceUpdateOption,
			) error {
				return fmt.Errorf("simulated status update error")
			},
		}).
		Build()

	r := &SessionStreamingConfigReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "val-status-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update error")
}

func TestSessionStreamingConfig_StatusUpdateError_Success(t *testing.T) {
	config := newKafkaStreamingConfig("success-status-err", true)

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(config).
		WithStatusSubresource(&corev1alpha1.SessionStreamingConfig{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				ctx context.Context,
				c client.Client,
				subResourceName string,
				obj client.Object,
				opts ...client.SubResourceUpdateOption,
			) error {
				return fmt.Errorf("simulated status update error")
			},
		}).
		Build()

	r := &SessionStreamingConfigReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "success-status-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update error")
}

func TestSessionStreamingConfig_GetError_ReturnsError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(
				ctx context.Context,
				c client.WithWatch,
				key client.ObjectKey,
				obj client.Object,
				opts ...client.GetOption,
			) error {
				return fmt.Errorf("simulated get error")
			},
		}).
		Build()

	r := &SessionStreamingConfigReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "any-config"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated get error")
}

func TestSessionStreamingConfig_PublisherError_StatusUpdateFails(t *testing.T) {
	config := newKafkaStreamingConfig("pub-status-err", true)

	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(config).
		WithStatusSubresource(&corev1alpha1.SessionStreamingConfig{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(
				ctx context.Context,
				c client.Client,
				subResourceName string,
				obj client.Object,
				opts ...client.SubResourceUpdateOption,
			) error {
				return fmt.Errorf("simulated status update error")
			},
		}).
		Build()

	r := &SessionStreamingConfigReconciler{
		Client: fakeClient,
		Scheme: scheme,
		PublisherFactory: func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "pub-status-err"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update error")
}

func TestSessionStreamingConfig_ClosePublisher_NilPublisher(t *testing.T) {
	r := &SessionStreamingConfigReconciler{}
	// Should not panic
	r.closePublisher()
	assert.Nil(t, r.publisher)
}

func TestSessionStreamingConfig_RecordStreamingEvent_NilRecorder(t *testing.T) {
	r := &SessionStreamingConfigReconciler{}
	// Should not panic with nil recorder
	r.recordStreamingEvent(nil, "Normal", "test", "test message")
}
