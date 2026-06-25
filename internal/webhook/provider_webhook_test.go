/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func embeddingProvider(dim int32) *corev1alpha1.Provider {
	p := &corev1alpha1.Provider{}
	p.Spec.Role = corev1alpha1.ProviderRoleEmbedding
	if dim > 0 {
		p.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: dim}
	}
	return p
}

func TestProviderValidator_WarnsOnDimensionChange(t *testing.T) {
	v := &ProviderValidator{}
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(1536), embeddingProvider(768))
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "1536→768")
	assert.Contains(t, warns[0], "consent")
}

func TestProviderValidator_NoWarnSameDimension(t *testing.T) {
	v := &ProviderValidator{}
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(768), embeddingProvider(768))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_NoWarnWhenDimensionUnsetOrFirstDeclared(t *testing.T) {
	v := &ProviderValidator{}
	// unset -> set is model-driven / a first declaration, not a destructive change.
	warns, err := v.ValidateUpdate(context.Background(), embeddingProvider(0), embeddingProvider(768))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_NoWarnForNonEmbeddingRole(t *testing.T) {
	v := &ProviderValidator{}
	oldP := &corev1alpha1.Provider{}
	oldP.Spec.Role = corev1alpha1.ProviderRoleLLM
	oldP.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: 1536}
	newP := &corev1alpha1.Provider{}
	newP.Spec.Role = corev1alpha1.ProviderRoleLLM
	newP.Spec.Embedding = &corev1alpha1.EmbeddingConfig{Dimensions: 768}

	warns, err := v.ValidateUpdate(context.Background(), oldP, newP)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

func TestProviderValidator_CreateAndDeleteNeverWarn(t *testing.T) {
	v := &ProviderValidator{}
	w1, err := v.ValidateCreate(context.Background(), embeddingProvider(1536))
	require.NoError(t, err)
	assert.Empty(t, w1)
	w2, err := v.ValidateDelete(context.Background(), embeddingProvider(1536))
	require.NoError(t, err)
	assert.Empty(t, w2)
}

// newWebhookScheme returns a scheme with corev1 and corev1alpha1 registered.
func newWebhookScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, corev1alpha1.AddToScheme(s))
	return s
}

// claudeProvider returns a Provider with spec.credential.secretRef set.
func claudeProvider(ns, secretName string, key *string) *corev1alpha1.Provider {
	p := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude", Namespace: ns},
		Spec: corev1alpha1.ProviderSpec{
			Type: corev1alpha1.ProviderTypeClaude,
			Credential: &corev1alpha1.CredentialConfig{
				SecretRef: &corev1alpha1.SecretKeyRef{Name: secretName, Key: key},
			},
		},
	}
	return p
}

// authProvider returns a Provider with spec.auth.credentialsSecretRef set.
func authProvider(ns, secretName string, key *string) *corev1alpha1.Provider {
	return &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "bedrock-claude", Namespace: ns},
		Spec: corev1alpha1.ProviderSpec{
			Type: corev1alpha1.ProviderTypeClaude,
			Auth: &corev1alpha1.AuthConfig{
				Type:                 corev1alpha1.AuthMethodAccessKey,
				CredentialsSecretRef: &corev1alpha1.SecretKeyRef{Name: secretName, Key: key},
			},
		},
	}
}

func strPtr(s string) *string { return &s }

// TestProviderValidator_WarnsOnMissingSecret: no secret in cluster → warning on create.
func TestProviderValidator_WarnsOnMissingSecret(t *testing.T) {
	scheme := newWebhookScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &ProviderValidator{Client: cl}
	p := claudeProvider("ns", "missing-secret", nil)

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "missing-secret")
}

// TestProviderValidator_WarnsOnMissingSecretUpdate: same check fires on update too.
func TestProviderValidator_WarnsOnMissingSecretUpdate(t *testing.T) {
	scheme := newWebhookScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &ProviderValidator{Client: cl}
	p := claudeProvider("ns", "missing-secret", nil)

	warns, err := v.ValidateUpdate(context.Background(), p, p)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "missing-secret")
}

// TestProviderValidator_WarnsOnMissingExplicitKey: secret exists but the explicit key is absent.
func TestProviderValidator_WarnsOnMissingExplicitKey(t *testing.T) {
	scheme := newWebhookScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "ns"},
		Data:       map[string][]byte{"OTHER_KEY": []byte("val")},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	v := &ProviderValidator{Client: cl}
	p := claudeProvider("ns", "my-secret", strPtr("MISSING_KEY"))

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "MISSING_KEY")
}

// TestProviderValidator_NoWarnWhenDefaultKeyPresent: secret has a default key and no key is set.
func TestProviderValidator_NoWarnWhenDefaultKeyPresent(t *testing.T) {
	scheme := newWebhookScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "ns"},
		Data:       map[string][]byte{"ANTHROPIC_API_KEY": []byte("sk-test")},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	v := &ProviderValidator{Client: cl}
	p := claudeProvider("ns", "my-secret", nil)

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestProviderValidator_NoWarnWhenExplicitKeyPresent: secret has the exact explicit key.
func TestProviderValidator_NoWarnWhenExplicitKeyPresent(t *testing.T) {
	scheme := newWebhookScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "ns"},
		Data:       map[string][]byte{"MY_CUSTOM_KEY": []byte("sk-test")},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	v := &ProviderValidator{Client: cl}
	p := claudeProvider("ns", "my-secret", strPtr("MY_CUSTOM_KEY"))

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestProviderValidator_WarnsOnMissingAuthSecret: auth.credentialsSecretRef missing.
func TestProviderValidator_WarnsOnMissingAuthSecret(t *testing.T) {
	scheme := newWebhookScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &ProviderValidator{Client: cl}
	p := authProvider("ns", "missing-auth-secret", nil)

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "missing-auth-secret")
}

// TestProviderValidator_NoWarnNilClient: nil client is safe (no-op).
func TestProviderValidator_NoWarnNilClient(t *testing.T) {
	v := &ProviderValidator{Client: nil}
	p := claudeProvider("ns", "some-secret", nil)

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestProviderValidator_BothRefWarnsCombined: both refs missing → two warnings on create.
func TestProviderValidator_BothRefWarnsCombined(t *testing.T) {
	scheme := newWebhookScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &ProviderValidator{Client: cl}
	p := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude", Namespace: "ns"},
		Spec: corev1alpha1.ProviderSpec{
			Type: corev1alpha1.ProviderTypeClaude,
			Credential: &corev1alpha1.CredentialConfig{
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "missing-cred"},
			},
			Auth: &corev1alpha1.AuthConfig{
				Type:                 corev1alpha1.AuthMethodAccessKey,
				CredentialsSecretRef: &corev1alpha1.SecretKeyRef{Name: "missing-auth"},
			},
		},
	}

	warns, err := v.ValidateCreate(context.Background(), p)
	require.NoError(t, err)
	require.Len(t, warns, 2)
}

// TestProviderValidator_UpdateBothEmbeddingAndSecretWarns: dimension change + missing secret = two warnings.
func TestProviderValidator_UpdateBothEmbeddingAndSecretWarns(t *testing.T) {
	scheme := newWebhookScheme(t)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &ProviderValidator{Client: cl}

	oldP := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "emb", Namespace: "ns"},
		Spec: corev1alpha1.ProviderSpec{
			Type:      corev1alpha1.ProviderTypeOpenAI,
			Role:      corev1alpha1.ProviderRoleEmbedding,
			Embedding: &corev1alpha1.EmbeddingConfig{Dimensions: 1536},
			Credential: &corev1alpha1.CredentialConfig{
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "gone"},
			},
		},
	}
	newP := oldP.DeepCopy()
	newP.Spec.Embedding.Dimensions = 768

	warns, err := v.ValidateUpdate(context.Background(), oldP, newP)
	require.NoError(t, err)
	require.Len(t, warns, 2)
}
