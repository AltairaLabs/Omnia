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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/internal/session"
)

// mockProvider implements encryption.Provider for testing.
type mockProvider struct {
	EncryptFn      func(ctx context.Context, plaintext []byte) (*encryption.EncryptOutput, error)
	DecryptFn      func(ctx context.Context, ciphertext []byte) ([]byte, error)
	GetKeyMetaFn   func(ctx context.Context) (*encryption.KeyMetadata, error)
	RotateKeyFn    func(ctx context.Context) (*encryption.KeyRotationResult, error)
	CloseFn        func() error
	rotateKeyCalls int
}

func (m *mockProvider) Encrypt(ctx context.Context, plaintext []byte) (*encryption.EncryptOutput, error) {
	if m.EncryptFn != nil {
		return m.EncryptFn(ctx, plaintext)
	}
	return &encryption.EncryptOutput{Ciphertext: plaintext}, nil
}

func (m *mockProvider) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if m.DecryptFn != nil {
		return m.DecryptFn(ctx, ciphertext)
	}
	return ciphertext, nil
}

func (m *mockProvider) GetKeyMetadata(ctx context.Context) (*encryption.KeyMetadata, error) {
	if m.GetKeyMetaFn != nil {
		return m.GetKeyMetaFn(ctx)
	}
	return &encryption.KeyMetadata{KeyID: "test-key", KeyVersion: "1"}, nil
}

func (m *mockProvider) RotateKey(ctx context.Context) (*encryption.KeyRotationResult, error) {
	m.rotateKeyCalls++
	if m.RotateKeyFn != nil {
		return m.RotateKeyFn(ctx)
	}
	return &encryption.KeyRotationResult{
		PreviousKeyVersion: "1",
		NewKeyVersion:      "2",
		RotatedAt:          time.Now(),
	}, nil
}

func (m *mockProvider) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

// mockReEncryptionStore implements encryption.ReEncryptionStore for testing.
type mockReEncryptionStore struct {
	GetBatchFn func(ctx context.Context, keyID, notKeyVersion string, batchSize int, afterID string) ([]*encryption.EncryptedMessage, error)
	UpdateFn   func(ctx context.Context, sessionID string, msg *session.Message) error
}

func (m *mockReEncryptionStore) GetEncryptedMessageBatch(
	ctx context.Context, keyID, notKeyVersion string, batchSize int, afterID string,
) ([]*encryption.EncryptedMessage, error) {
	if m.GetBatchFn != nil {
		return m.GetBatchFn(ctx, keyID, notKeyVersion, batchSize, afterID)
	}
	return nil, nil
}

func (m *mockReEncryptionStore) UpdateMessageContent(
	ctx context.Context, sessionID string, msg *session.Message,
) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, sessionID, msg)
	}
	return nil
}

func setupKeyRotationTest(t *testing.T, objects ...runtime.Object) (*KeyRotationReconciler, *record.FakeRecorder, *mockProvider) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)

	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "omnia-system"},
	}

	clientObjects := []runtime.Object{omniaSystemNS}
	clientObjects = append(clientObjects, objects...)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range clientObjects {
		builder = builder.WithRuntimeObjects(obj)
	}
	builder = builder.WithStatusSubresource(&omniav1alpha1.SessionPrivacyPolicy{})

	fakeClient := builder.Build()
	recorder := record.NewFakeRecorder(20)

	provider := &mockProvider{}
	reconciler := &KeyRotationReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		ProviderFactory: func(_ encryption.ProviderConfig) (encryption.Provider, error) {
			return provider, nil
		},
		StoreFactory: func() (encryption.ReEncryptionStore, error) {
			return &mockReEncryptionStore{}, nil
		},
	}

	return reconciler, recorder, provider
}

func newKeyRotationPolicy() *omniav1alpha1.SessionPrivacyPolicy {
	return &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
			},
			Encryption: &omniav1alpha1.EncryptionConfig{
				Enabled:     true,
				KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
				KeyID:       "arn:aws:kms:us-east-1:123456:key/test-key",
				SecretRef:   &corev1alpha1.LocalObjectReference{Name: "encryption-secret"},
				KeyRotation: &omniav1alpha1.KeyRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
				},
			},
		},
	}
}

func newEncryptionSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encryption-secret",
			Namespace: "omnia-system",
		},
		Data: map[string][]byte{
			"access-key-id":     []byte("AKIAIOSFODNN7EXAMPLE"),
			"secret-access-key": []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
		},
	}
}

func TestKeyRotation_AnnotationTriggered(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Annotations = map[string]string{
		rotateKeyAnnotation: "true",
	}

	reconciler, recorder, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 1, provider.rotateKeyCalls)

	// Verify annotation was removed.
	updated := &omniav1alpha1.SessionPrivacyPolicy{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-policy"}, updated)
	require.NoError(t, err)
	assert.Empty(t, updated.Annotations[rotateKeyAnnotation])

	// Verify status was updated.
	assert.NotNil(t, updated.Status.KeyRotation)
	assert.Equal(t, "2", updated.Status.KeyRotation.CurrentKeyVersion)
	assert.NotNil(t, updated.Status.KeyRotation.LastRotatedAt)

	// Verify event was emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, eventReasonKeyRotated)
	default:
		t.Error("expected KeyRotated event")
	}
}

func TestKeyRotation_AnnotationTriggered_WithReEncryption(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Annotations = map[string]string{
		rotateKeyAnnotation: "true",
	}
	policy.Spec.Encryption.KeyRotation.ReEncryptExisting = true

	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, reEncryptionRequeueDelay, result.RequeueAfter)

	// Verify re-encryption was started.
	updated := &omniav1alpha1.SessionPrivacyPolicy{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-policy"}, updated)
	require.NoError(t, err)
	assert.NotNil(t, updated.Status.KeyRotation.ReEncryptionProgress)
	assert.Equal(t, "InProgress", updated.Status.KeyRotation.ReEncryptionProgress.Status)
}

func TestKeyRotation_ScheduledRotation_NotDue(t *testing.T) {
	policy := newKeyRotationPolicy()
	// Set last rotation to now so next rotation is a month away.
	now := metav1.Now()
	policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{
		LastRotatedAt:     &now,
		CurrentKeyVersion: "1",
	}

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "should requeue at next scheduled time")
	assert.Equal(t, 0, provider.rotateKeyCalls, "should not rotate when not due")
}

func TestKeyRotation_ScheduledRotation_NeverRotated(t *testing.T) {
	policy := newKeyRotationPolicy()
	// No status.keyRotation set — never rotated before.

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 1, provider.rotateKeyCalls, "should rotate immediately when never rotated")
}

func TestKeyRotation_ScheduledRotation_PastDue(t *testing.T) {
	policy := newKeyRotationPolicy()
	// Set last rotation to 2 months ago.
	pastTime := metav1.NewTime(time.Now().Add(-60 * 24 * time.Hour))
	policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{
		LastRotatedAt:     &pastTime,
		CurrentKeyVersion: "1",
	}

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 1, provider.rotateKeyCalls, "should rotate when past due")
}

func TestKeyRotation_Disabled(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KeyRotation.Enabled = false

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, provider.rotateKeyCalls)
}

func TestKeyRotation_NoEncryption(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption = nil

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, provider.rotateKeyCalls)
}

func TestKeyRotation_NotFound(t *testing.T) {
	reconciler, _, provider := setupKeyRotationTest(t)

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, provider.rotateKeyCalls)
}

func TestKeyRotation_ProviderError(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Annotations = map[string]string{rotateKeyAnnotation: "true"}

	reconciler, recorder, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())
	provider.RotateKeyFn = func(_ context.Context) (*encryption.KeyRotationResult, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "KMS unavailable")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify failure event.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, eventReasonKeyRotationFailed)
	default:
		t.Error("expected KeyRotationFailed event")
	}
}

func TestKeyRotation_ProviderFactoryError(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Annotations = map[string]string{rotateKeyAnnotation: "true"}

	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())
	reconciler.ProviderFactory = func(_ encryption.ProviderConfig) (encryption.Provider, error) {
		return nil, fmt.Errorf("invalid credentials")
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid credentials")
}

func TestKeyRotation_MissingSecret(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Annotations = map[string]string{rotateKeyAnnotation: "true"}
	// No secret object passed.

	reconciler, _, _ := setupKeyRotationTest(t, policy)

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading secret")
}

func TestKeyRotation_ReEncryptionBatchCompleted(t *testing.T) {
	policy := newKeyRotationPolicy()
	now := metav1.Now()
	policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{
		LastRotatedAt:     &now,
		CurrentKeyVersion: "2",
		ReEncryptionProgress: &omniav1alpha1.ReEncryptionProgress{
			Status:    "InProgress",
			StartedAt: &now,
		},
	}

	mockStore := &mockReEncryptionStore{
		GetBatchFn: func(_ context.Context, _, _ string, _ int, _ string) ([]*encryption.EncryptedMessage, error) {
			// Return empty batch to signal completion.
			return nil, nil
		},
	}

	reconciler, recorder, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())
	reconciler.StoreFactory = func() (encryption.ReEncryptionStore, error) {
		return mockStore, nil
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "should not requeue when re-encryption is complete")

	// Verify progress was updated to Completed.
	updated := &omniav1alpha1.SessionPrivacyPolicy{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-policy"}, updated)
	require.NoError(t, err)
	assert.Equal(t, "Completed", updated.Status.KeyRotation.ReEncryptionProgress.Status)
	assert.NotNil(t, updated.Status.KeyRotation.ReEncryptionProgress.CompletedAt)

	// Verify completion event.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Re-encryption completed")
	default:
		t.Error("expected re-encryption completion event")
	}
}

func TestKeyRotation_ReEncryptionStoreFactoryNil(t *testing.T) {
	policy := newKeyRotationPolicy()
	now := metav1.Now()
	policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{
		LastRotatedAt:     &now,
		CurrentKeyVersion: "2",
		ReEncryptionProgress: &omniav1alpha1.ReEncryptionProgress{
			Status:    "InProgress",
			StartedAt: &now,
		},
	}

	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())
	reconciler.StoreFactory = nil

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify progress was marked Failed.
	updated := &omniav1alpha1.SessionPrivacyPolicy{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: "test-policy"}, updated)
	require.NoError(t, err)
	assert.Equal(t, "Failed", updated.Status.KeyRotation.ReEncryptionProgress.Status)
}

func TestKeyRotation_NoSchedule(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KeyRotation.Schedule = ""

	reconciler, _, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, provider.rotateKeyCalls, "should not rotate without a schedule")
}

func TestKeyRotation_InvalidCronSchedule(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KeyRotation.Schedule = "invalid-cron"

	reconciler, recorder, provider := setupKeyRotationTest(t, policy, newEncryptionSecret())

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, 0, provider.rotateKeyCalls)

	// Verify warning event for invalid schedule.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "invalid cron schedule")
	default:
		t.Error("expected invalid cron schedule event")
	}
}

func TestKeyRotation_CustomBatchSize(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KeyRotation.BatchSize = ptr.To(int32(50))

	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())
	assert.Equal(t, 50, reconciler.getBatchSize(policy))
}

func TestKeyRotation_DefaultBatchSize(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KeyRotation.BatchSize = nil

	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())
	assert.Equal(t, defaultBatchSize, reconciler.getBatchSize(policy))
}

func TestKeyRotation_BuildProviderConfig(t *testing.T) {
	policy := newKeyRotationPolicy()
	reconciler, _, _ := setupKeyRotationTest(t, policy, newEncryptionSecret())

	cfg, err := reconciler.buildProviderConfig(context.Background(), policy)
	require.NoError(t, err)
	assert.Equal(t, encryption.ProviderType("aws-kms"), cfg.ProviderType)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456:key/test-key", cfg.KeyID)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cfg.Credentials["access-key-id"])
}

func TestKeyRotation_BuildProviderConfig_VaultURL(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.KMSProvider = omniav1alpha1.KMSProviderVault

	vaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encryption-secret",
			Namespace: "omnia-system",
		},
		Data: map[string][]byte{
			"vault-url":   []byte("https://vault.example.com"),
			"vault-token": []byte("hvs.testtoken"),
		},
	}

	reconciler, _, _ := setupKeyRotationTest(t, policy, vaultSecret)

	cfg, err := reconciler.buildProviderConfig(context.Background(), policy)
	require.NoError(t, err)
	assert.Equal(t, "https://vault.example.com", cfg.VaultURL)
}

func TestKeyRotation_BuildProviderConfig_NoSecretRef(t *testing.T) {
	policy := newKeyRotationPolicy()
	policy.Spec.Encryption.SecretRef = nil

	reconciler, _, _ := setupKeyRotationTest(t, policy)

	cfg, err := reconciler.buildProviderConfig(context.Background(), policy)
	require.NoError(t, err)
	assert.Empty(t, cfg.Credentials)
}
