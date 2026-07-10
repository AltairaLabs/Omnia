/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestReconcile_EmptyModel_ForcesError proves the reconcile path is wired: a
// pre-existing Provider with valid credentials but no model (one that predates
// the admission CEL rule, so it lives in the store) is driven to phase=Error
// with ModelValid=False, gating any AgentRuntime bound to it (#1819). A fake
// client is used deliberately — it bypasses the CEL rule that would otherwise
// reject creating such a Provider, which is exactly the pre-upgrade state we
// need to exercise.
func TestReconcile_EmptyModel_ForcesError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-secret", Namespace: "default"},
		Data:       map[string][]byte{"ANTHROPIC_API_KEY": []byte("sk-real-key")},
	}
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-no-model", Namespace: "default"},
		Spec: omniav1alpha1.ProviderSpec{
			Type: omniav1alpha1.ProviderTypeClaude,
			// Model intentionally empty.
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "claude-secret"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, provider).
		WithStatusSubresource(provider).
		Build()

	r := &ProviderReconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		HTTPClient: alwaysHealthyClient(),
		// Credential probes valid so we reach the model check rather than
		// short-circuiting on the credential.
		CredentialValidatorFactory: func(*omniav1alpha1.Provider, *http.Client) CredentialValidator {
			return fakeCredentialValidator{err: nil}
		},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "claude-no-model", Namespace: "default"},
	})
	require.NoError(t, err)

	updated := &omniav1alpha1.Provider{}
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "claude-no-model", Namespace: "default"}, updated))

	assert.Equal(t, omniav1alpha1.ProviderPhaseError, updated.Status.Phase,
		"a provider with valid creds but no model must not be Ready")

	cond := meta.FindStatusCondition(updated.Status.Conditions, ProviderConditionTypeModelValid)
	require.NotNil(t, cond, "ModelValid condition must be set")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "ModelMissing", cond.Reason)
}

// TestReconcile_ModelSet_StaysReady is the positive control: the same provider
// with a model set reconciles to Ready.
func TestReconcile_ModelSet_StaysReady(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-secret", Namespace: "default"},
		Data:       map[string][]byte{"ANTHROPIC_API_KEY": []byte("sk-real-key")},
	}
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-with-model", Namespace: "default"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeClaude,
			Model: "claude-sonnet-4-20250514",
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "claude-secret"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, provider).
		WithStatusSubresource(provider).
		Build()

	r := &ProviderReconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		HTTPClient: alwaysHealthyClient(),
		CredentialValidatorFactory: func(*omniav1alpha1.Provider, *http.Client) CredentialValidator {
			return fakeCredentialValidator{err: nil}
		},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "claude-with-model", Namespace: "default"},
	})
	require.NoError(t, err)

	updated := &omniav1alpha1.Provider{}
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "claude-with-model", Namespace: "default"}, updated))

	assert.Equal(t, omniav1alpha1.ProviderPhaseReady, updated.Status.Phase)
	cond := meta.FindStatusCondition(updated.Status.Conditions, ProviderConditionTypeModelValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}
