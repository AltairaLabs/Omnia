/*
Copyright 2026.

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

package main

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

const testRawKey = "omk_some_random_value"

func sha256Bytes(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func newAPIKeySecret(name, agent string, hash []byte, role string, expiresAt string) *corev1.Secret {
	data := map[string][]byte{
		APIKeyDataKeyHash: hash,
	}
	if role != "" {
		data[APIKeyDataKeyScopes] = []byte(`{"role":"` + role + `"}`)
	}
	if expiresAt != "" {
		data[APIKeyDataKeyExpiresAt] = []byte(expiresAt)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "ns",
			Labels: map[string]string{
				LabelCredentialKind: LabelCredentialKindAgentAPIKey,
				LabelAgent:          agent,
			},
		},
		Data: data,
	}
}

func TestSecretBackedKeyStore_LoadsMatchingSecrets(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	hash := sha256Bytes(testRawKey)
	secret := newAPIKeySecret("agent-myagent-apikey-001", "myagent", hash, "editor", "")
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	hashHex := auth.HashToken(testRawKey)
	key, ok := store.Lookup(hashHex)
	if !ok {
		t.Fatalf("expected key in store, available hashes were not exposed")
	}
	if got, want := key.ID, "001"; got != want {
		t.Errorf("ID = %q, want %q", got, want)
	}
	if got, want := key.Role, "editor"; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
}

func TestSecretBackedKeyStore_FiltersOtherAgents(t *testing.T) {
	// A Secret with the right credential-kind label but a DIFFERENT
	// agent label must not appear in this agent's store — the per-agent
	// isolation is the whole point of the label scheme.
	t.Parallel()
	scheme := newTestScheme(t)
	hash := sha256Bytes(testRawKey)
	other := newAPIKeySecret("agent-other-apikey-001", "other-agent", hash, "editor", "")
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup(auth.HashToken(testRawKey)); ok {
		t.Error("expected miss — Secret belongs to a different agent")
	}
}

func TestSecretBackedKeyStore_FiltersOtherCredentialKinds(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	hash := sha256Bytes(testRawKey)
	wrong := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-myagent-apikey-001",
			Namespace: "ns",
			Labels: map[string]string{
				LabelCredentialKind: "agent-shared-token", // wrong kind
				LabelAgent:          "myagent",
			},
		},
		Data: map[string][]byte{APIKeyDataKeyHash: hash},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wrong).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup(auth.HashToken(testRawKey)); ok {
		t.Error("expected miss — Secret has wrong credential-kind")
	}
}

func TestSecretBackedKeyStore_ParsesExpiry(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	hash := sha256Bytes(testRawKey)
	expires := "2030-12-31T23:59:59Z"
	secret := newAPIKeySecret("agent-myagent-apikey-x", "myagent", hash, "viewer", expires)
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	key, ok := store.Lookup(auth.HashToken(testRawKey))
	if !ok {
		t.Fatal("expected key present")
	}
	want := time.Date(2030, time.December, 31, 23, 59, 59, 0, time.UTC)
	if !key.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", key.ExpiresAt, want)
	}
}

func TestSecretBackedKeyStore_SkipsMalformedSecrets(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	good := newAPIKeySecret("agent-myagent-apikey-good", "myagent", sha256Bytes("good-key"), "editor", "")
	missingHash := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-myagent-apikey-bad",
			Namespace: "ns",
			Labels: map[string]string{
				LabelCredentialKind: LabelCredentialKindAgentAPIKey,
				LabelAgent:          "myagent",
			},
		},
		Data: map[string][]byte{}, // missing keyHash
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(good, missingHash).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup(auth.HashToken("good-key")); !ok {
		t.Error("good key should still load alongside the malformed sibling")
	}
}

func TestSecretBackedKeyStore_NoMatchingSecretsIsValid(t *testing.T) {
	// An agent with apiKeys configured but no Secrets yet should still
	// init successfully — operators may enable the feature before
	// minting any keys via the dashboard.
	t.Parallel()
	scheme := newTestScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()

	store, err := NewSecretBackedKeyStore(context.Background(), fc, "ns", "myagent", logr.Discard(),
		WithKeyStoreRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup(auth.HashToken(testRawKey)); ok {
		t.Error("empty store should miss every lookup")
	}
}

func TestParseAPIKeySecret_MalformedScopesErrors(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-x-apikey-y",
			Labels: map[string]string{
				LabelCredentialKind: LabelCredentialKindAgentAPIKey,
				LabelAgent:          "x",
			},
		},
		Data: map[string][]byte{
			APIKeyDataKeyHash:   sha256Bytes("k"),
			APIKeyDataKeyScopes: []byte("not-json"),
		},
	}
	if _, err := parseAPIKeySecret(secret); err == nil {
		t.Error("expected error on malformed scopes JSON")
	}
}

func TestParseAPIKeySecret_MalformedExpiresAtErrors(t *testing.T) {
	t.Parallel()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-x-apikey-y",
			Labels: map[string]string{
				LabelCredentialKind: LabelCredentialKindAgentAPIKey,
				LabelAgent:          "x",
			},
		},
		Data: map[string][]byte{
			APIKeyDataKeyHash:      sha256Bytes("k"),
			APIKeyDataKeyExpiresAt: []byte("yesterday"),
		},
	}
	if _, err := parseAPIKeySecret(secret); err == nil {
		t.Error("expected error on malformed expiresAt")
	}
}
