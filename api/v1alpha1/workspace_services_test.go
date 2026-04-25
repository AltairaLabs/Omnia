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
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestWorkspaceServiceGroupManagedJSON(t *testing.T) {
	warmDays := int32(7)
	group := WorkspaceServiceGroup{
		Name: "default",
		Mode: ServiceModeManaged,
		Memory: &MemoryServiceConfig{
			Database: DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "memory-db-secret"},
			},
			ProviderRef: &corev1.LocalObjectReference{Name: "openai-provider"},
			PolicyRef:   &corev1.LocalObjectReference{Name: "default-memory-policy"},
		},
		Session: &SessionServiceConfig{
			Database: DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "session-db-secret"},
			},
			Retention: &SessionRetentionConfig{WarmDays: &warmDays},
		},
	}

	data, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got WorkspaceServiceGroup
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Name != group.Name {
		t.Errorf("Name = %q, want %q", got.Name, group.Name)
	}
	if got.Mode != ServiceModeManaged {
		t.Errorf("Mode = %q, want %q", got.Mode, ServiceModeManaged)
	}
	if got.Memory == nil {
		t.Fatal("Memory is nil after round-trip")
	}
	if got.Memory.Database.SecretRef.Name != "memory-db-secret" {
		t.Errorf("Memory.Database.SecretRef.Name = %q, want %q", got.Memory.Database.SecretRef.Name, "memory-db-secret")
	}
	if got.Memory.ProviderRef == nil || got.Memory.ProviderRef.Name != "openai-provider" {
		t.Errorf("Memory.ProviderRef.Name = %v, want openai-provider", got.Memory.ProviderRef)
	}
	if got.Memory.PolicyRef == nil || got.Memory.PolicyRef.Name != "default-memory-policy" {
		t.Errorf("Memory.PolicyRef = %v, want default-memory-policy", got.Memory.PolicyRef)
	}
	if got.Session == nil {
		t.Fatal("Session is nil after round-trip")
	}
	if got.Session.Database.SecretRef.Name != "session-db-secret" {
		t.Errorf("Session.Database.SecretRef.Name = %q, want %q", got.Session.Database.SecretRef.Name, "session-db-secret")
	}
	if got.Session.Retention == nil || got.Session.Retention.WarmDays == nil || *got.Session.Retention.WarmDays != warmDays {
		t.Errorf("Session.Retention.WarmDays = %v, want %d", got.Session.Retention, warmDays)
	}
	if got.External != nil {
		t.Errorf("External should be nil for managed group, got %+v", got.External)
	}
}

func TestWorkspaceServiceGroupExternalJSON(t *testing.T) {
	group := WorkspaceServiceGroup{
		Name: "external-group",
		Mode: ServiceModeExternal,
		External: &ExternalEndpoints{
			SessionURL: "https://session.example.com",
			MemoryURL:  "https://memory.example.com",
		},
	}

	data, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got WorkspaceServiceGroup
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Name != group.Name {
		t.Errorf("Name = %q, want %q", got.Name, group.Name)
	}
	if got.Mode != ServiceModeExternal {
		t.Errorf("Mode = %q, want %q", got.Mode, ServiceModeExternal)
	}
	if got.External == nil {
		t.Fatal("External is nil after round-trip")
	}
	if got.External.SessionURL != "https://session.example.com" {
		t.Errorf("External.SessionURL = %q, want https://session.example.com", got.External.SessionURL)
	}
	if got.External.MemoryURL != "https://memory.example.com" {
		t.Errorf("External.MemoryURL = %q, want https://memory.example.com", got.External.MemoryURL)
	}
	if got.Memory != nil {
		t.Errorf("Memory should be nil for external group, got %+v", got.Memory)
	}
	if got.Session != nil {
		t.Errorf("Session should be nil for external group, got %+v", got.Session)
	}
}

func TestServiceGroupStatusJSON(t *testing.T) {
	status := ServiceGroupStatus{
		Name:       "default",
		SessionURL: "https://session-svc.omnia-system.svc.cluster.local",
		MemoryURL:  "https://memory-svc.omnia-system.svc.cluster.local",
		Ready:      true,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got ServiceGroupStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got.Name != status.Name {
		t.Errorf("Name = %q, want %q", got.Name, status.Name)
	}
	if got.SessionURL != status.SessionURL {
		t.Errorf("SessionURL = %q, want %q", got.SessionURL, status.SessionURL)
	}
	if got.MemoryURL != status.MemoryURL {
		t.Errorf("MemoryURL = %q, want %q", got.MemoryURL, status.MemoryURL)
	}
	if !got.Ready {
		t.Error("Ready = false, want true")
	}
}

func TestWorkspaceServiceGroupDefaultMode(t *testing.T) {
	// A zero-value WorkspaceServiceGroup should have empty Mode, not a default.
	// The kubebuilder default applies only at the API server level (admission webhook),
	// not at the Go struct level.
	group := WorkspaceServiceGroup{Name: "test"}
	if group.Mode != "" {
		t.Errorf("zero-value Mode = %q, want empty string", group.Mode)
	}
}

func TestServiceModeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant ServiceMode
		expected string
	}{
		{"Managed", ServiceModeManaged, "managed"},
		{"External", ServiceModeExternal, "external"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("ServiceMode %s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
