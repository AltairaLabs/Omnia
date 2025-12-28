/*
Copyright 2025.

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

package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAgentRuntimeTypesRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	// Verify AgentRuntime is registered
	gvk := GroupVersion.WithKind("AgentRuntime")
	if !scheme.Recognizes(gvk) {
		t.Errorf("scheme does not recognize %v", gvk)
	}

	// Verify AgentRuntimeList is registered
	gvkList := GroupVersion.WithKind("AgentRuntimeList")
	if !scheme.Recognizes(gvkList) {
		t.Errorf("scheme does not recognize %v", gvkList)
	}
}

func TestFacadeTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      FacadeType
		expected string
	}{
		{"WebSocket", FacadeTypeWebSocket, "websocket"},
		{"gRPC", FacadeTypeGRPC, "grpc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("FacadeType %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestSessionStoreTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      SessionStoreType
		expected string
	}{
		{"Memory", SessionStoreTypeMemory, "memory"},
		{"Redis", SessionStoreTypeRedis, "redis"},
		{"Postgres", SessionStoreTypePostgres, "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("SessionStoreType %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestAgentRuntimePhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      AgentRuntimePhase
		expected string
	}{
		{"Pending", AgentRuntimePhasePending, "Pending"},
		{"Running", AgentRuntimePhaseRunning, "Running"},
		{"Failed", AgentRuntimePhaseFailed, "Failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("AgentRuntimePhase %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

const testVersion = "1.0.0"

func TestAgentRuntimeCreation(t *testing.T) {
	port := int32(8080)
	replicas := int32(3)
	version := testVersion
	track := "stable"
	ttl := "24h"
	namespace := "tools"

	ar := &AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: AgentRuntimeSpec{
			PromptPackRef: PromptPackRef{
				Name:    "my-prompts",
				Version: &version,
				Track:   &track,
			},
			Facade: FacadeConfig{
				Type: FacadeTypeWebSocket,
				Port: &port,
			},
			ToolRegistryRef: &ToolRegistryRef{
				Name:      "my-tools",
				Namespace: &namespace,
			},
			Session: &SessionConfig{
				Type: SessionStoreTypeRedis,
				StoreRef: &corev1.LocalObjectReference{
					Name: "redis-secret",
				},
				TTL: &ttl,
			},
			Runtime: &RuntimeConfig{
				Replicas: &replicas,
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				NodeSelector: map[string]string{
					"node-type": "agent",
				},
			},
			ProviderSecretRef: corev1.LocalObjectReference{
				Name: "llm-credentials",
			},
		},
	}

	// Verify spec fields
	if ar.Spec.PromptPackRef.Name != "my-prompts" {
		t.Errorf("PromptPackRef.Name = %q, want %q", ar.Spec.PromptPackRef.Name, "my-prompts")
	}
	if *ar.Spec.PromptPackRef.Version != testVersion {
		t.Errorf("PromptPackRef.Version = %q, want %q", *ar.Spec.PromptPackRef.Version, testVersion)
	}
	if ar.Spec.Facade.Type != FacadeTypeWebSocket {
		t.Errorf("Facade.Type = %q, want %q", ar.Spec.Facade.Type, FacadeTypeWebSocket)
	}
	if *ar.Spec.Facade.Port != 8080 {
		t.Errorf("Facade.Port = %d, want %d", *ar.Spec.Facade.Port, 8080)
	}
	if ar.Spec.ToolRegistryRef.Name != "my-tools" {
		t.Errorf("ToolRegistryRef.Name = %q, want %q", ar.Spec.ToolRegistryRef.Name, "my-tools")
	}
	if *ar.Spec.ToolRegistryRef.Namespace != "tools" {
		t.Errorf("ToolRegistryRef.Namespace = %q, want %q", *ar.Spec.ToolRegistryRef.Namespace, "tools")
	}
	if ar.Spec.Session.Type != SessionStoreTypeRedis {
		t.Errorf("Session.Type = %q, want %q", ar.Spec.Session.Type, SessionStoreTypeRedis)
	}
	if ar.Spec.Session.StoreRef.Name != "redis-secret" {
		t.Errorf("Session.StoreRef.Name = %q, want %q", ar.Spec.Session.StoreRef.Name, "redis-secret")
	}
	if *ar.Spec.Runtime.Replicas != 3 {
		t.Errorf("Runtime.Replicas = %d, want %d", *ar.Spec.Runtime.Replicas, 3)
	}
	if ar.Spec.ProviderSecretRef.Name != "llm-credentials" {
		t.Errorf("ProviderSecretRef.Name = %q, want %q", ar.Spec.ProviderSecretRef.Name, "llm-credentials")
	}
}

func TestAgentRuntimeStatus(t *testing.T) {
	version := testVersion

	status := AgentRuntimeStatus{
		Phase: AgentRuntimePhaseRunning,
		Replicas: &ReplicaStatus{
			Desired:   3,
			Ready:     2,
			Available: 2,
		},
		ActiveVersion:      &version,
		ObservedGeneration: 5,
		Conditions: []metav1.Condition{
			{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "MinimumReplicasAvailable",
				Message:            "Deployment has minimum availability",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	if status.Phase != AgentRuntimePhaseRunning {
		t.Errorf("Phase = %q, want %q", status.Phase, AgentRuntimePhaseRunning)
	}
	if status.Replicas.Desired != 3 {
		t.Errorf("Replicas.Desired = %d, want %d", status.Replicas.Desired, 3)
	}
	if status.Replicas.Ready != 2 {
		t.Errorf("Replicas.Ready = %d, want %d", status.Replicas.Ready, 2)
	}
	if status.Replicas.Available != 2 {
		t.Errorf("Replicas.Available = %d, want %d", status.Replicas.Available, 2)
	}
	if *status.ActiveVersion != testVersion {
		t.Errorf("ActiveVersion = %q, want %q", *status.ActiveVersion, testVersion)
	}
	if status.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want %d", status.ObservedGeneration, 5)
	}
	if len(status.Conditions) != 1 {
		t.Errorf("len(Conditions) = %d, want %d", len(status.Conditions), 1)
	}
	if status.Conditions[0].Type != "Available" {
		t.Errorf("Conditions[0].Type = %q, want %q", status.Conditions[0].Type, "Available")
	}
}

func TestAgentRuntimeListCreation(t *testing.T) {
	list := &AgentRuntimeList{
		Items: []AgentRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-1",
					Namespace: "default",
				},
				Spec: AgentRuntimeSpec{
					PromptPackRef: PromptPackRef{Name: "pack-1"},
					Facade:        FacadeConfig{Type: FacadeTypeWebSocket},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "secret-1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-2",
					Namespace: "default",
				},
				Spec: AgentRuntimeSpec{
					PromptPackRef: PromptPackRef{Name: "pack-2"},
					Facade:        FacadeConfig{Type: FacadeTypeGRPC},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "secret-2",
					},
				},
			},
		},
	}

	if len(list.Items) != 2 {
		t.Errorf("len(Items) = %d, want %d", len(list.Items), 2)
	}
	if list.Items[0].Name != "agent-1" {
		t.Errorf("Items[0].Name = %q, want %q", list.Items[0].Name, "agent-1")
	}
	if list.Items[1].Spec.Facade.Type != FacadeTypeGRPC {
		t.Errorf("Items[1].Spec.Facade.Type = %q, want %q", list.Items[1].Spec.Facade.Type, FacadeTypeGRPC)
	}
}

func TestMinimalAgentRuntime(t *testing.T) {
	// Test creating an AgentRuntime with only required fields
	ar := &AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-agent",
			Namespace: "default",
		},
		Spec: AgentRuntimeSpec{
			PromptPackRef: PromptPackRef{
				Name: "my-prompts",
			},
			Facade: FacadeConfig{
				Type: FacadeTypeWebSocket,
			},
			ProviderSecretRef: corev1.LocalObjectReference{
				Name: "llm-credentials",
			},
		},
	}

	// Verify required fields are set
	if ar.Spec.PromptPackRef.Name != "my-prompts" {
		t.Errorf("PromptPackRef.Name = %q, want %q", ar.Spec.PromptPackRef.Name, "my-prompts")
	}
	if ar.Spec.Facade.Type != FacadeTypeWebSocket {
		t.Errorf("Facade.Type = %q, want %q", ar.Spec.Facade.Type, FacadeTypeWebSocket)
	}
	if ar.Spec.ProviderSecretRef.Name != "llm-credentials" {
		t.Errorf("ProviderSecretRef.Name = %q, want %q", ar.Spec.ProviderSecretRef.Name, "llm-credentials")
	}

	// Verify optional fields are nil
	if ar.Spec.PromptPackRef.Version != nil {
		t.Error("PromptPackRef.Version should be nil")
	}
	if ar.Spec.Facade.Port != nil {
		t.Error("Facade.Port should be nil")
	}
	if ar.Spec.ToolRegistryRef != nil {
		t.Error("ToolRegistryRef should be nil")
	}
	if ar.Spec.Session != nil {
		t.Error("Session should be nil")
	}
	if ar.Spec.Runtime != nil {
		t.Error("Runtime should be nil")
	}
}
