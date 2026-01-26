/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/altairalabs/omnia/ee/pkg/license"
)

func setupLicenseActivationTest(t *testing.T) (*LicenseActivationReconciler, *rsa.PrivateKey, *httptest.Server) {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Generate test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create namespaces for fingerprinting
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	// Create mock activation server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/licenses/activate" && r.Method == http.MethodPost:
			resp := license.ActivationResponse{
				Activated:    true,
				ActivationID: "act_test_123",
				Message:      "Activated",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodPost && r.URL.Path[len(r.URL.Path)-9:] == "heartbeat":
			resp := license.HeartbeatResponse{
				Valid:   true,
				Message: "OK",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodDelete:
			resp := license.DeactivationResponse{
				Deactivated: true,
				Message:     "Deactivated",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	return reconciler, privateKey, server
}

//nolint:unparam // tier is used to set features, tests currently only use enterprise
func createTestLicenseJWT(t *testing.T, privateKey *rsa.PrivateKey, tier license.Tier, licenseID string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"lid":      licenseID,
		"tier":     string(tier),
		"customer": "Test Customer",
		"features": map[string]bool{
			"gitSource":          tier == license.TierEnterprise,
			"ociSource":          tier == license.TierEnterprise,
			"s3Source":           tier == license.TierEnterprise,
			"loadTesting":        tier == license.TierEnterprise,
			"dataGeneration":     tier == license.TierEnterprise,
			"scheduling":         tier == license.TierEnterprise,
			"distributedWorkers": tier == license.TierEnterprise,
		},
		"limits": map[string]int{
			"maxScenarios":      100,
			"maxWorkerReplicas": 10,
			"maxActivations":    3,
		},
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(365 * 24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return tokenString
}

func TestLicenseActivationReconciler_Reconcile_OpenCore(t *testing.T) {
	reconciler, _, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// No license secret = open-core
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no activation ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	assert.Error(t, err) // Should not exist
}

func TestLicenseActivationReconciler_Reconcile_Enterprise_NewActivation(t *testing.T) {
	reconciler, privateKey, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err := reconciler.Create(ctx, secret)
	require.NoError(t, err)

	// Invalidate cache to pick up the new license
	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, license.DefaultHeartbeatInterval, result.RequeueAfter)

	// Verify activation ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	require.NoError(t, err)

	// Parse and verify activation state
	var state license.ActivationState
	err = json.Unmarshal([]byte(cm.Data["state"]), &state)
	require.NoError(t, err)
	assert.Equal(t, "act_test_123", state.ActivationID)
	assert.NotEmpty(t, state.ClusterFingerprint)
	assert.False(t, state.ActivatedAt.IsZero())
}

func TestLicenseActivationReconciler_Reconcile_Heartbeat(t *testing.T) {
	reconciler, privateKey, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err := reconciler.Create(ctx, secret)
	require.NoError(t, err)

	// Create existing activation state (heartbeat needed)
	activationState := &license.ActivationState{
		ActivationID:       "act_existing_123",
		ClusterFingerprint: "fp_existing",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-48 * time.Hour),
		LastHeartbeat:      time.Now().Add(-25 * time.Hour), // More than 24h ago
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}
	err = reconciler.Create(ctx, cm)
	require.NoError(t, err)

	// Invalidate cache
	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, license.DefaultHeartbeatInterval, result.RequeueAfter)

	// Verify heartbeat was recorded
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	require.NoError(t, err)

	var updatedState license.ActivationState
	err = json.Unmarshal([]byte(cm.Data["state"]), &updatedState)
	require.NoError(t, err)
	assert.True(t, updatedState.LastHeartbeat.After(activationState.LastHeartbeat))
}

func TestLicenseActivationReconciler_Reconcile_IgnoresOtherSecrets(t *testing.T) {
	reconciler, _, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// Request for a different secret
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "other-secret",
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestLicenseActivationReconciler_Deactivate(t *testing.T) {
	reconciler, _, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// Create existing activation state
	activationState := &license.ActivationState{
		ActivationID:       "act_to_deactivate",
		ClusterFingerprint: "fp_test",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-48 * time.Hour),
		LastHeartbeat:      time.Now().Add(-1 * time.Hour),
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}
	err = reconciler.Create(ctx, cm)
	require.NoError(t, err)

	// Deactivate
	err = reconciler.Deactivate(ctx)
	require.NoError(t, err)

	// Verify ConfigMap was deleted
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, &corev1.ConfigMap{})
	assert.Error(t, err) // Should not exist
}

func TestLicenseActivationReconciler_Deactivate_NotActivated(t *testing.T) {
	reconciler, _, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// No activation state exists
	err := reconciler.Deactivate(ctx)
	require.NoError(t, err) // Should succeed without error
}

func TestLicenseActivationReconciler_HeartbeatNotNeeded(t *testing.T) {
	reconciler, privateKey, server := setupLicenseActivationTest(t)
	defer server.Close()

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err := reconciler.Create(ctx, secret)
	require.NoError(t, err)

	// Create existing activation state with recent heartbeat
	activationState := &license.ActivationState{
		ActivationID:       "act_existing",
		ClusterFingerprint: "fp_existing",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-48 * time.Hour),
		LastHeartbeat:      time.Now().Add(-1 * time.Hour), // Only 1 hour ago
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}
	err = reconciler.Create(ctx, cm)
	require.NoError(t, err)

	// Invalidate cache
	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	// Should requeue for remaining time until next heartbeat
	// (approximately 23 hours from now)
	assert.True(t, result.RequeueAfter > 22*time.Hour)
	assert.True(t, result.RequeueAfter < 24*time.Hour)
}

func TestLicenseActivationReconciler_ActivationRejected(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	// Mock server that rejects activation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := license.ActivationResponse{
			Activated:      false,
			Message:        "Maximum activations reached",
			ActiveClusters: []string{"fp_1", "fp_2", "fp_3"},
			MaxActivations: 3,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Should not requeue when activation is rejected
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no activation ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	assert.Error(t, err)
}

func TestLicenseActivationReconciler_ActivationServerError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	// Mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server error"})
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Should requeue after 5 minutes on error
	assert.Equal(t, 5*time.Minute, result.RequeueAfter)
}

func TestLicenseActivationReconciler_HeartbeatFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	// Mock server that fails heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/licenses/activate" {
			resp := license.ActivationResponse{
				Activated:    true,
				ActivationID: "act_test_123",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "service unavailable"})
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	// Create existing activation state (heartbeat needed)
	activationState := &license.ActivationState{
		ActivationID:       "act_existing",
		ClusterFingerprint: "fp_existing",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-48 * time.Hour),
		LastHeartbeat:      time.Now().Add(-25 * time.Hour),
		HeartbeatFailures:  0,
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}
	err = reconciler.Create(ctx, cm)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Should requeue after 1 hour on heartbeat failure
	assert.Equal(t, time.Hour, result.RequeueAfter)

	// Verify failure count was incremented
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	require.NoError(t, err)

	var updatedState license.ActivationState
	err = json.Unmarshal([]byte(cm.Data["state"]), &updatedState)
	require.NoError(t, err)
	assert.Equal(t, 1, updatedState.HeartbeatFailures)
}

func TestLicenseActivationReconciler_GetActivationStateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	// Create ConfigMap with invalid JSON to cause unmarshal error
	invalidCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": "invalid json",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS, invalidCM).
		Build()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	// Should return error due to invalid activation state
	_, err = reconciler.Reconcile(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestLicenseActivationReconciler_FingerprintError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Don't create kube-system namespace to trigger fingerprint error
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(omniaSystemNS).
		Build()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	// Should requeue after fingerprint error
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, result.RequeueAfter)
}

func TestLicenseActivationReconciler_Deactivate_ServerError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	// Create activation state
	activationState := &license.ActivationState{
		ActivationID:       "act_to_deactivate",
		ClusterFingerprint: "fp_test",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-48 * time.Hour),
		LastHeartbeat:      time.Now().Add(-1 * time.Hour),
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS, cm).
		Build()

	// Mock server that fails deactivation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         record.NewFakeRecorder(10),
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Deactivate - should succeed even if server fails (local cleanup still happens)
	err = reconciler.Deactivate(ctx)
	require.NoError(t, err)

	// Verify ConfigMap was deleted
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, &corev1.ConfigMap{})
	assert.Error(t, err) // Should not exist
}

func TestLicenseActivationReconciler_HeartbeatGracePeriodExpired(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  "kube-system-uid",
		},
	}
	omniaSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: license.LicenseSecretNamespace,
			UID:  "omnia-system-uid",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kubeSystemNS, omniaSystemNS).
		Build()

	// Mock server that fails heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	validator, err := license.NewValidator(client, license.WithPublicKey(&privateKey.PublicKey))
	require.NoError(t, err)

	activationClient := license.NewActivationClient(license.WithServerURL(server.URL))
	recorder := record.NewFakeRecorder(10)

	reconciler := &LicenseActivationReconciler{
		Client:           client,
		Scheme:           scheme,
		Recorder:         recorder,
		LicenseValidator: validator,
		ActivationClient: activationClient,
		ClusterName:      "test-cluster",
	}

	ctx := context.Background()

	// Create enterprise license secret
	licenseJWT := createTestLicenseJWT(t, privateKey, license.TierEnterprise, "lic_test_123")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(licenseJWT),
		},
	}
	err = reconciler.Create(ctx, secret)
	require.NoError(t, err)

	// Create existing activation state with expired grace period
	activationState := &license.ActivationState{
		ActivationID:       "act_existing",
		ClusterFingerprint: "fp_existing",
		LicenseID:          "lic_test_123",
		ActivatedAt:        time.Now().Add(-30 * 24 * time.Hour),
		LastHeartbeat:      time.Now().Add(-8 * 24 * time.Hour), // 8 days ago
		HeartbeatFailures:  5,
	}
	stateData, err := json.Marshal(activationState)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string]string{
			"state": string(stateData),
		},
	}
	err = reconciler.Create(ctx, cm)
	require.NoError(t, err)

	reconciler.LicenseValidator.InvalidateCache()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, time.Hour, result.RequeueAfter)

	// Check that warning event was recorded
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "HeartbeatGracePeriodExpired")
	default:
		t.Error("expected warning event for expired grace period")
	}
}
