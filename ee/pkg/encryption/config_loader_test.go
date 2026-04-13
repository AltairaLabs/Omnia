/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

const testSecretNamespace = "omnia-system"

func newFakeClientWithSecret(t *testing.T, name string, data map[string][]byte) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1: %v", err)
	}
	b := fake.NewClientBuilder().WithScheme(scheme)
	if data != nil {
		b = b.WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testSecretNamespace},
			Data:       data,
		})
	}
	return b
}

func TestProviderConfigFromEncryptionSpec_Nil(t *testing.T) {
	b := newFakeClientWithSecret(t, "", nil)
	cfg, err := ProviderConfigFromEncryptionSpec(context.Background(), b.Build(), "omnia-system", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProviderType != "" || cfg.KeyID != "" {
		t.Fatalf("expected zero config, got %+v", cfg)
	}
}

func TestProviderConfigFromEncryptionSpec_NoSecret(t *testing.T) {
	b := newFakeClientWithSecret(t, "", nil)
	enc := &omniav1alpha1.EncryptionConfig{
		KMSProvider: "aws-kms",
		KeyID:       "alias/my-key",
	}
	cfg, err := ProviderConfigFromEncryptionSpec(context.Background(), b.Build(), "omnia-system", enc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProviderType != ProviderAWSKMS || cfg.KeyID != "alias/my-key" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.Credentials != nil {
		t.Fatalf("expected nil credentials, got %v", cfg.Credentials)
	}
}

func TestProviderConfigFromEncryptionSpec_WithSecret_SetsVaultURL(t *testing.T) {
	b := newFakeClientWithSecret(t, "kms-creds", map[string][]byte{
		"vault-url": []byte("https://vault.example.com"),
		"token":     []byte("s.abc123"),
	})
	enc := &omniav1alpha1.EncryptionConfig{
		KMSProvider: "vault",
		KeyID:       "omnia",
		SecretRef:   &corev1alpha1.LocalObjectReference{Name: "kms-creds"},
	}
	cfg, err := ProviderConfigFromEncryptionSpec(context.Background(), b.Build(), "omnia-system", enc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultURL != "https://vault.example.com" {
		t.Fatalf("expected VaultURL set from secret, got %q", cfg.VaultURL)
	}
	if cfg.Credentials["token"] != "s.abc123" {
		t.Fatalf("expected credentials populated, got %v", cfg.Credentials)
	}
}

func TestProviderConfigFromEncryptionSpec_SecretNotFound(t *testing.T) {
	b := newFakeClientWithSecret(t, "", nil)
	enc := &omniav1alpha1.EncryptionConfig{
		KMSProvider: "aws-kms",
		KeyID:       "k",
		SecretRef:   &corev1alpha1.LocalObjectReference{Name: "missing"},
	}
	_, err := ProviderConfigFromEncryptionSpec(context.Background(), b.Build(), "omnia-system", enc)
	if err == nil {
		t.Fatal("expected error loading missing secret")
	}
}
