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

package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestScheme_RegistersOmniaTypes(t *testing.T) {
	s := Scheme()

	// Check that Omnia CRD types are registered
	gvks, _, err := s.ObjectKinds(&omniav1alpha1.AgentRuntime{})
	if err != nil {
		t.Fatalf("AgentRuntime not registered: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("no GVKs found for AgentRuntime")
	}

	gvks, _, err = s.ObjectKinds(&omniav1alpha1.Provider{})
	if err != nil {
		t.Fatalf("Provider not registered: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("no GVKs found for Provider")
	}

	// Check corev1 types
	gvks, _, err = s.ObjectKinds(&corev1.Secret{})
	if err != nil {
		t.Fatalf("Secret not registered: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("no GVKs found for Secret")
	}
}

func TestGetAgentRuntime_Found(t *testing.T) {
	s := Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(ar).Build()

	got, err := GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", got.Name)
	}
}

func TestGetAgentRuntime_NotFound(t *testing.T) {
	s := Scheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	_, err := GetAgentRuntime(context.Background(), c, "nonexistent", "default")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetProvider_Found(t *testing.T) {
	s := Scheme()
	p := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{Type: "claude"},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(p).Build()

	ref := omniav1alpha1.ProviderRef{Name: "test-provider"}
	got, err := GetProvider(context.Background(), c, ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "test-provider" {
		t.Errorf("expected name test-provider, got %s", got.Name)
	}
}

func TestGetProvider_CrossNamespace(t *testing.T) {
	s := Scheme()
	p := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-provider",
			Namespace: "shared",
		},
		Spec: omniav1alpha1.ProviderSpec{Type: "openai"},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(p).Build()

	ns := "shared"
	ref := omniav1alpha1.ProviderRef{Name: "shared-provider", Namespace: &ns}
	got, err := GetProvider(context.Background(), c, ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Namespace != "shared" {
		t.Errorf("expected namespace shared, got %s", got.Namespace)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	s := Scheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	ref := omniav1alpha1.ProviderRef{Name: "nonexistent"}
	_, err := GetProvider(context.Background(), c, ref, "default")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetProviderSecret_Found(t *testing.T) {
	s := Scheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-key",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ANTHROPIC_API_KEY": []byte("test-key"),
		},
	}
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{
			Type:      "claude",
			SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-key"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(secret, provider).Build()

	got, err := GetProviderSecret(context.Background(), c, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got.Data["ANTHROPIC_API_KEY"]) != "test-key" {
		t.Errorf("unexpected secret data")
	}
}

func TestGetProviderSecret_NoSecretRef(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{Type: "mock"},
	}

	c := fake.NewClientBuilder().WithScheme(Scheme()).Build()

	_, err := GetProviderSecret(context.Background(), c, provider)
	if err == nil {
		t.Fatal("expected error for no secretRef")
	}
}

func TestGetProviderSecret_SecretNotFound(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{
			Type:      "claude",
			SecretRef: &omniav1alpha1.SecretKeyRef{Name: "nonexistent-secret"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(Scheme()).Build()

	_, err := GetProviderSecret(context.Background(), c, provider)
	if err == nil {
		t.Fatal("expected error for secret not found")
	}
}

func TestNewClient_NoClusterConfig(t *testing.T) {
	// Unset KUBECONFIG and HOME to ensure no K8s config is available.
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	t.Setenv("HOME", "/nonexistent")

	// Outside a K8s cluster, NewClient should return an error (not panic).
	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error when no K8s config available")
	}
}

func TestNewClientWithConfig_Success(t *testing.T) {
	// Start a minimal HTTPS server to act as a fake API server.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
	}))
	defer srv.Close()

	cfg := &rest.Config{
		Host: srv.URL,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}

	c, err := NewClientWithConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClientWithConfig_InvalidConfig(t *testing.T) {
	// A config with an invalid host scheme should cause client creation to fail.
	cfg := &rest.Config{
		Host: "://invalid",
	}

	_, err := NewClientWithConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func strPtr(s string) *string { return &s }

func TestDetermineSecretKey_ExplicitKey(t *testing.T) {
	ref := &omniav1alpha1.SecretKeyRef{Name: "s", Key: strPtr("custom-key")}
	got := DetermineSecretKey(ref, omniav1alpha1.ProviderTypeClaude)
	if got != "custom-key" {
		t.Errorf("expected custom-key, got %s", got)
	}
}

func TestDetermineSecretKey_ClaudeDefault(t *testing.T) {
	ref := &omniav1alpha1.SecretKeyRef{Name: "s"}
	got := DetermineSecretKey(ref, omniav1alpha1.ProviderTypeClaude)
	if got != "ANTHROPIC_API_KEY" {
		t.Errorf("expected ANTHROPIC_API_KEY, got %s", got)
	}
}

func TestDetermineSecretKey_OpenAIDefault(t *testing.T) {
	ref := &omniav1alpha1.SecretKeyRef{Name: "s"}
	got := DetermineSecretKey(ref, omniav1alpha1.ProviderTypeOpenAI)
	if got != "OPENAI_API_KEY" {
		t.Errorf("expected OPENAI_API_KEY, got %s", got)
	}
}

func TestDetermineSecretKey_GeminiDefault(t *testing.T) {
	ref := &omniav1alpha1.SecretKeyRef{Name: "s"}
	got := DetermineSecretKey(ref, omniav1alpha1.ProviderTypeGemini)
	if got != "GEMINI_API_KEY" {
		t.Errorf("expected GEMINI_API_KEY, got %s", got)
	}
}

func TestDetermineSecretKey_UnknownFallback(t *testing.T) {
	ref := &omniav1alpha1.SecretKeyRef{Name: "s"}
	got := DetermineSecretKey(ref, "unknown")
	if got != "api-key" {
		t.Errorf("expected api-key, got %s", got)
	}
}

func TestEffectiveSecretRef_CredentialPreferred(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		Spec: omniav1alpha1.ProviderSpec{
			SecretRef: &omniav1alpha1.SecretKeyRef{Name: "legacy"},
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "preferred"},
			},
		},
	}
	ref := EffectiveSecretRef(provider)
	if ref == nil || ref.Name != "preferred" {
		t.Errorf("expected preferred, got %v", ref)
	}
}

func TestEffectiveSecretRef_LegacyFallback(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		Spec: omniav1alpha1.ProviderSpec{
			SecretRef: &omniav1alpha1.SecretKeyRef{Name: "legacy"},
		},
	}
	ref := EffectiveSecretRef(provider)
	if ref == nil || ref.Name != "legacy" {
		t.Errorf("expected legacy, got %v", ref)
	}
}

func TestEffectiveSecretRef_None(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		Spec: omniav1alpha1.ProviderSpec{Type: "mock"},
	}
	ref := EffectiveSecretRef(provider)
	if ref != nil {
		t.Errorf("expected nil, got %v", ref)
	}
}
