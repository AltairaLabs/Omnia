/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func discardLogger() logr.Logger { return logr.Discard() }

func unmarshalForTest(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

func newLocalObjectRef(name string) *corev1.LocalObjectReference {
	return &corev1.LocalObjectReference{Name: name}
}

func TestSanitizeEnvName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-agent", "MY_AGENT"},
		{"agent123", "AGENT123"},
		{"foo.bar", "FOO_BAR"},
		{"ALREADY_UPPER", "ALREADY_UPPER"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeEnvName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeEnvName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestA2AClientTokenEnvName(t *testing.T) {
	got := a2aClientTokenEnvName("my-agent")
	want := "OMNIA_A2A_CLIENT_TOKEN_MY_AGENT"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalA2AClients(t *testing.T) {
	clients := []ResolvedA2AClient{
		{Name: "agent-a", URL: "http://agent-a:8080", ExposeAsTools: true, AuthTokenEnv: "OMNIA_A2A_CLIENT_TOKEN_AGENT_A"},
		{Name: "agent-b", URL: "http://external.example.com/a2a"},
	}

	data, err := marshalA2AClients(clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data == "" {
		t.Fatal("expected non-empty JSON")
	}

	// Round-trip verify.
	var parsed []ResolvedA2AClient
	if err := unmarshalForTest(data, &parsed); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(parsed))
	}
	if parsed[0].Name != "agent-a" {
		t.Errorf("first client name = %q, want %q", parsed[0].Name, "agent-a")
	}
	if !parsed[0].ExposeAsTools {
		t.Error("first client should have exposeAsTools=true")
	}
}

func TestResolveOneClient_ExternalURL(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "my-agent"
	ar.Namespace = "test-ns"

	client := omniav1alpha1.A2AClientSpec{
		Name: "external",
		URL:  "https://external.example.com/a2a",
	}

	rc, status := r.resolveOneClient(context.TODO(), discardLogger(), ar, client)
	if rc == nil {
		t.Fatal("expected resolved client")
	}
	if rc.URL != "https://external.example.com/a2a" {
		t.Errorf("URL = %q, want %q", rc.URL, "https://external.example.com/a2a")
	}
	if !status.Ready {
		t.Error("expected status.Ready = true")
	}
	if status.ResolvedURL != rc.URL {
		t.Errorf("status.ResolvedURL = %q, want %q", status.ResolvedURL, rc.URL)
	}
}

func TestResolveOneClient_ExternalURL_WithExposeAsTools(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}

	client := omniav1alpha1.A2AClientSpec{
		Name:          "tools-agent",
		URL:           "http://tools.example.com",
		ExposeAsTools: true,
	}

	rc, status := r.resolveOneClient(context.TODO(), discardLogger(), ar, client)
	if rc == nil {
		t.Fatal("expected resolved client")
	}
	if !rc.ExposeAsTools {
		t.Error("expected ExposeAsTools = true")
	}
	if !status.Ready {
		t.Error("expected status.Ready = true")
	}
}

func TestResolveOneClient_MissingURLAndRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}

	client := omniav1alpha1.A2AClientSpec{
		Name: "broken",
	}

	rc, status := r.resolveOneClient(context.TODO(), discardLogger(), ar, client)
	if rc != nil {
		t.Error("expected nil resolved client")
	}
	if status.Ready {
		t.Error("expected status.Ready = false")
	}
	if status.Error == "" {
		t.Error("expected error message")
	}
}

func TestResolveOneClient_WithAuth(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}

	secretRef := newLocalObjectRef("my-secret")
	client := omniav1alpha1.A2AClientSpec{
		Name: "authed-agent",
		URL:  "http://authed.example.com",
		Authentication: &omniav1alpha1.A2AClientAuthConfig{
			SecretRef: secretRef,
		},
	}

	rc, status := r.resolveOneClient(context.TODO(), discardLogger(), ar, client)
	if rc == nil {
		t.Fatal("expected resolved client")
	}
	if rc.AuthTokenEnv != "OMNIA_A2A_CLIENT_TOKEN_AUTHED_AGENT" {
		t.Errorf("AuthTokenEnv = %q, want %q", rc.AuthTokenEnv, "OMNIA_A2A_CLIENT_TOKEN_AUTHED_AGENT")
	}
	if !status.Ready {
		t.Error("expected ready")
	}
}

func TestResolveA2AClients_NilA2A(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}

	clients, statuses := r.resolveA2AClients(context.TODO(), discardLogger(), ar)
	if clients != nil {
		t.Error("expected nil clients")
	}
	if statuses != nil {
		t.Error("expected nil statuses")
	}
}

func TestResolveA2AClients_EmptyClients(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{}

	clients, statuses := r.resolveA2AClients(context.TODO(), discardLogger(), ar)
	if clients != nil {
		t.Error("expected nil clients")
	}
	if statuses != nil {
		t.Error("expected nil statuses")
	}
}

func TestResolveA2AClients_MultipleClients(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		Clients: []omniav1alpha1.A2AClientSpec{
			{Name: "ext-1", URL: "http://ext-1.example.com"},
			{Name: "ext-2", URL: "http://ext-2.example.com", ExposeAsTools: true},
			{Name: "broken"}, // no URL or ref
		},
	}

	clients, statuses := r.resolveA2AClients(context.TODO(), discardLogger(), ar)
	// 2 resolved (ext-1, ext-2), 1 failed (broken)
	if len(clients) != 2 {
		t.Fatalf("expected 2 resolved clients, got %d", len(clients))
	}
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}
	if statuses[2].Ready {
		t.Error("broken client should not be ready")
	}
}
