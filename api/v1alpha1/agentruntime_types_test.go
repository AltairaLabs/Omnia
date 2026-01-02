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

const (
	testVersion     = "1.0.0"
	testPromptPack  = "my-prompts"
	testCredentials = "llm-credentials"
)

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
				Name:    testPromptPack,
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
			Provider: &ProviderConfig{
				SecretRef: &corev1.LocalObjectReference{
					Name: testCredentials,
				},
			},
		},
	}

	// Verify spec fields
	if ar.Spec.PromptPackRef.Name != testPromptPack {
		t.Errorf("PromptPackRef.Name = %q, want %q", ar.Spec.PromptPackRef.Name, testPromptPack)
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
	if ar.Spec.Provider.SecretRef.Name != testCredentials {
		t.Errorf("Provider.SecretRef.Name = %q, want %q", ar.Spec.Provider.SecretRef.Name, testCredentials)
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
					Provider: &ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "secret-1",
						},
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
					Provider: &ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "secret-2",
						},
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
				Name: testPromptPack,
			},
			Facade: FacadeConfig{
				Type: FacadeTypeWebSocket,
			},
			Provider: &ProviderConfig{
				SecretRef: &corev1.LocalObjectReference{
					Name: testCredentials,
				},
			},
		},
	}

	// Verify required fields are set
	if ar.Spec.PromptPackRef.Name != testPromptPack {
		t.Errorf("PromptPackRef.Name = %q, want %q", ar.Spec.PromptPackRef.Name, testPromptPack)
	}
	if ar.Spec.Facade.Type != FacadeTypeWebSocket {
		t.Errorf("Facade.Type = %q, want %q", ar.Spec.Facade.Type, FacadeTypeWebSocket)
	}
	if ar.Spec.Provider.SecretRef.Name != testCredentials {
		t.Errorf("Provider.SecretRef.Name = %q, want %q", ar.Spec.Provider.SecretRef.Name, testCredentials)
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

func TestPromptPackRefWithTrackOnly(t *testing.T) {
	track := "canary"
	ref := PromptPackRef{
		Name:  testPromptPack,
		Track: &track,
	}

	if ref.Name != testPromptPack {
		t.Errorf("Name = %q, want %q", ref.Name, testPromptPack)
	}
	if ref.Version != nil {
		t.Error("Version should be nil when using track")
	}
	if *ref.Track != "canary" {
		t.Errorf("Track = %q, want %q", *ref.Track, "canary")
	}
}

func TestFacadeConfigWithGRPC(t *testing.T) {
	port := int32(9090)
	config := FacadeConfig{
		Type: FacadeTypeGRPC,
		Port: &port,
	}

	if config.Type != FacadeTypeGRPC {
		t.Errorf("Type = %q, want %q", config.Type, FacadeTypeGRPC)
	}
	if *config.Port != 9090 {
		t.Errorf("Port = %d, want %d", *config.Port, 9090)
	}
}

func TestSessionConfigMemory(t *testing.T) {
	ttl := "1h"
	config := SessionConfig{
		Type: SessionStoreTypeMemory,
		TTL:  &ttl,
	}

	if config.Type != SessionStoreTypeMemory {
		t.Errorf("Type = %q, want %q", config.Type, SessionStoreTypeMemory)
	}
	if config.StoreRef != nil {
		t.Error("StoreRef should be nil for memory store")
	}
	if *config.TTL != "1h" {
		t.Errorf("TTL = %q, want %q", *config.TTL, "1h")
	}
}

func TestSessionConfigPostgres(t *testing.T) {
	ttl := "48h"
	config := SessionConfig{
		Type: SessionStoreTypePostgres,
		StoreRef: &corev1.LocalObjectReference{
			Name: "postgres-connection",
		},
		TTL: &ttl,
	}

	if config.Type != SessionStoreTypePostgres {
		t.Errorf("Type = %q, want %q", config.Type, SessionStoreTypePostgres)
	}
	if config.StoreRef.Name != "postgres-connection" {
		t.Errorf("StoreRef.Name = %q, want %q", config.StoreRef.Name, "postgres-connection")
	}
}

func TestRuntimeConfigWithTolerations(t *testing.T) {
	replicas := int32(2)
	config := RuntimeConfig{
		Replicas: &replicas,
		Tolerations: []corev1.Toleration{
			{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "agent",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}

	if *config.Replicas != 2 {
		t.Errorf("Replicas = %d, want %d", *config.Replicas, 2)
	}
	if len(config.Tolerations) != 1 {
		t.Errorf("len(Tolerations) = %d, want %d", len(config.Tolerations), 1)
	}
	if config.Tolerations[0].Key != "dedicated" {
		t.Errorf("Tolerations[0].Key = %q, want %q", config.Tolerations[0].Key, "dedicated")
	}
	if config.Tolerations[0].Effect != corev1.TaintEffectNoSchedule {
		t.Errorf("Tolerations[0].Effect = %q, want %q", config.Tolerations[0].Effect, corev1.TaintEffectNoSchedule)
	}
}

func TestRuntimeConfigWithAffinity(t *testing.T) {
	config := RuntimeConfig{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/arch",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"amd64", "arm64"},
								},
							},
						},
					},
				},
			},
		},
	}

	if config.Affinity == nil {
		t.Fatal("Affinity should not be nil")
	}
	if config.Affinity.NodeAffinity == nil {
		t.Fatal("NodeAffinity should not be nil")
	}
	nodeSelector := config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if nodeSelector == nil {
		t.Fatal("RequiredDuringSchedulingIgnoredDuringExecution should not be nil")
	}
	if len(nodeSelector.NodeSelectorTerms) != 1 {
		t.Errorf("len(NodeSelectorTerms) = %d, want %d", len(nodeSelector.NodeSelectorTerms), 1)
	}
}

func TestToolRegistryRefSameNamespace(t *testing.T) {
	// When namespace is nil, it should use the same namespace as AgentRuntime
	ref := ToolRegistryRef{
		Name: "shared-tools",
	}

	if ref.Name != "shared-tools" {
		t.Errorf("Name = %q, want %q", ref.Name, "shared-tools")
	}
	if ref.Namespace != nil {
		t.Error("Namespace should be nil for same-namespace reference")
	}
}

func TestReplicaStatusZeroValues(t *testing.T) {
	status := ReplicaStatus{
		Desired:   0,
		Ready:     0,
		Available: 0,
	}

	if status.Desired != 0 {
		t.Errorf("Desired = %d, want %d", status.Desired, 0)
	}
	if status.Ready != 0 {
		t.Errorf("Ready = %d, want %d", status.Ready, 0)
	}
	if status.Available != 0 {
		t.Errorf("Available = %d, want %d", status.Available, 0)
	}
}

func TestAgentRuntimeStatusEmpty(t *testing.T) {
	status := AgentRuntimeStatus{}

	if status.Phase != "" {
		t.Errorf("Phase = %q, want empty string", status.Phase)
	}
	if status.Replicas != nil {
		t.Error("Replicas should be nil")
	}
	if status.ActiveVersion != nil {
		t.Error("ActiveVersion should be nil")
	}
	if status.Conditions != nil {
		t.Error("Conditions should be nil")
	}
	if status.ObservedGeneration != 0 {
		t.Errorf("ObservedGeneration = %d, want %d", status.ObservedGeneration, 0)
	}
}

func TestAgentRuntimeStatusPending(t *testing.T) {
	status := AgentRuntimeStatus{
		Phase: AgentRuntimePhasePending,
		Replicas: &ReplicaStatus{
			Desired:   3,
			Ready:     0,
			Available: 0,
		},
		Conditions: []metav1.Condition{
			{
				Type:               "Progressing",
				Status:             metav1.ConditionTrue,
				Reason:             "NewReplicaSetCreated",
				Message:            "Created new replica set",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	if status.Phase != AgentRuntimePhasePending {
		t.Errorf("Phase = %q, want %q", status.Phase, AgentRuntimePhasePending)
	}
	if status.Replicas.Ready != 0 {
		t.Errorf("Replicas.Ready = %d, want %d", status.Replicas.Ready, 0)
	}
	if status.Conditions[0].Type != "Progressing" {
		t.Errorf("Conditions[0].Type = %q, want %q", status.Conditions[0].Type, "Progressing")
	}
}

func TestAgentRuntimeStatusFailed(t *testing.T) {
	status := AgentRuntimeStatus{
		Phase: AgentRuntimePhaseFailed,
		Conditions: []metav1.Condition{
			{
				Type:               "Available",
				Status:             metav1.ConditionFalse,
				Reason:             "MinimumReplicasUnavailable",
				Message:            "Deployment does not have minimum availability",
				LastTransitionTime: metav1.Now(),
			},
			{
				Type:               "Degraded",
				Status:             metav1.ConditionTrue,
				Reason:             "PodCrashLooping",
				Message:            "Pod is crash looping",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	if status.Phase != AgentRuntimePhaseFailed {
		t.Errorf("Phase = %q, want %q", status.Phase, AgentRuntimePhaseFailed)
	}
	if len(status.Conditions) != 2 {
		t.Errorf("len(Conditions) = %d, want %d", len(status.Conditions), 2)
	}
	if status.Conditions[1].Type != "Degraded" {
		t.Errorf("Conditions[1].Type = %q, want %q", status.Conditions[1].Type, "Degraded")
	}
}

func TestAgentRuntimeDeepCopy(t *testing.T) {
	port := int32(8080)
	replicas := int32(3)
	version := testVersion

	original := &AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: AgentRuntimeSpec{
			PromptPackRef: PromptPackRef{
				Name:    "my-prompts",
				Version: &version,
			},
			Facade: FacadeConfig{
				Type: FacadeTypeWebSocket,
				Port: &port,
			},
			Runtime: &RuntimeConfig{
				Replicas: &replicas,
			},
			Provider: &ProviderConfig{
				SecretRef: &corev1.LocalObjectReference{
					Name: "llm-credentials",
				},
			},
		},
		Status: AgentRuntimeStatus{
			Phase: AgentRuntimePhaseRunning,
			Replicas: &ReplicaStatus{
				Desired:   3,
				Ready:     3,
				Available: 3,
			},
		},
	}

	// Test DeepCopy
	copied := original.DeepCopy()

	// Verify the copy is equal
	if copied.Name != original.Name {
		t.Errorf("copied.Name = %q, want %q", copied.Name, original.Name)
	}
	if copied.Spec.PromptPackRef.Name != original.Spec.PromptPackRef.Name {
		t.Errorf("copied.Spec.PromptPackRef.Name = %q, want %q", copied.Spec.PromptPackRef.Name, original.Spec.PromptPackRef.Name)
	}

	// Verify it's a deep copy (modifying copy doesn't affect original)
	newVersion := "2.0.0"
	copied.Spec.PromptPackRef.Version = &newVersion
	if *original.Spec.PromptPackRef.Version != testVersion {
		t.Errorf("original.Spec.PromptPackRef.Version was modified, got %q, want %q", *original.Spec.PromptPackRef.Version, testVersion)
	}

	newReplicas := int32(5)
	copied.Spec.Runtime.Replicas = &newReplicas
	if *original.Spec.Runtime.Replicas != 3 {
		t.Errorf("original.Spec.Runtime.Replicas was modified, got %d, want %d", *original.Spec.Runtime.Replicas, 3)
	}
}

func TestAgentRuntimeListDeepCopy(t *testing.T) {
	original := &AgentRuntimeList{
		Items: []AgentRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-1",
					Namespace: "default",
				},
				Spec: AgentRuntimeSpec{
					PromptPackRef: PromptPackRef{Name: "pack-1"},
					Facade:        FacadeConfig{Type: FacadeTypeWebSocket},
					Provider: &ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{Name: "secret-1"},
					},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if len(copied.Items) != len(original.Items) {
		t.Errorf("len(copied.Items) = %d, want %d", len(copied.Items), len(original.Items))
	}

	// Modify the copy and verify original is unchanged
	copied.Items[0].Name = "modified-agent"
	if original.Items[0].Name != "agent-1" {
		t.Errorf("original.Items[0].Name was modified, got %q, want %q", original.Items[0].Name, "agent-1")
	}
}

func TestAgentRuntimeSpecDeepCopy(t *testing.T) {
	port := int32(8080)
	ttl := "24h"

	original := AgentRuntimeSpec{
		PromptPackRef: PromptPackRef{Name: "prompts"},
		Facade: FacadeConfig{
			Type: FacadeTypeWebSocket,
			Port: &port,
		},
		Session: &SessionConfig{
			Type: SessionStoreTypeRedis,
			TTL:  &ttl,
		},
		Provider: &ProviderConfig{
			SecretRef: &corev1.LocalObjectReference{Name: "secret"},
		},
	}

	copied := original.DeepCopy()

	// Modify the copy
	newPort := int32(9090)
	copied.Facade.Port = &newPort

	// Verify original is unchanged
	if *original.Facade.Port != 8080 {
		t.Errorf("original.Facade.Port was modified, got %d, want %d", *original.Facade.Port, 8080)
	}
}

func TestGroupVersionInfo(t *testing.T) {
	if GroupVersion.Group != "omnia.altairalabs.ai" {
		t.Errorf("GroupVersion.Group = %q, want %q", GroupVersion.Group, "omnia.altairalabs.ai")
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("GroupVersion.Version = %q, want %q", GroupVersion.Version, "v1alpha1")
	}
}
