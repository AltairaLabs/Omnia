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

package servicediscovery

import (
	"context"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

//nolint:unparam
func makeWorkspaceWithStatus(
	name, namespace string, groups []omniav1alpha1.ServiceGroupStatus,
) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: name,
			Namespace:   omniav1alpha1.NamespaceConfig{Name: namespace},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: groups,
		},
	}
}

func TestResolveServiceURLs_EnvVarOverride(t *testing.T) {
	t.Setenv(envSessionAPIURL, "http://session-override.example.com")
	t.Setenv(envMemoryAPIURL, "http://memory-override.example.com")

	r := NewResolver(nil)
	urls, err := r.ResolveServiceURLs(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session-override.example.com" {
		t.Errorf("expected session override, got %s", urls.SessionURL)
	}
	if urls.MemoryURL != "http://memory-override.example.com" {
		t.Errorf("expected memory override, got %s", urls.MemoryURL)
	}
}

func TestResolveServiceURLs_NoEnvNoClient(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")

	r := NewResolver(nil)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error when no env vars and nil client")
	}
}

func TestResolveServiceURLs_FromWorkspaceStatus(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "workspace-ns")

	ws := makeWorkspaceWithStatus("my-workspace", "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "default",
			SessionURL: "http://session.workspace-ns.svc.cluster.local",
			MemoryURL:  "http://memory.workspace-ns.svc.cluster.local",
			Ready:      true,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	urls, err := r.ResolveServiceURLs(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session.workspace-ns.svc.cluster.local" {
		t.Errorf("unexpected session URL: %s", urls.SessionURL)
	}
	if urls.MemoryURL != "http://memory.workspace-ns.svc.cluster.local" {
		t.Errorf("unexpected memory URL: %s", urls.MemoryURL)
	}
}

func TestResolveServiceURLs_ServiceGroupNotReady(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "workspace-ns")

	// Ready=false but URLs populated — should succeed. Callers that only
	// need session-api shouldn't be blocked by memory-api failures.
	ws := makeWorkspaceWithStatus("my-workspace", "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "default",
			SessionURL: "http://session.workspace-ns.svc.cluster.local",
			MemoryURL:  "http://memory.workspace-ns.svc.cluster.local",
			Ready:      false,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	urls, err := r.ResolveServiceURLs(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session.workspace-ns.svc.cluster.local" {
		t.Errorf("expected session URL, got %s", urls.SessionURL)
	}
}

func TestResolveServiceURLs_ServiceGroupNoURLs(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "workspace-ns")

	// No URLs populated — should fail even if the group exists.
	ws := makeWorkspaceWithStatus("my-workspace", "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:  "default",
			Ready: false,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error when service group has no URLs")
	}
}

func TestResolveServiceURLs_ServiceGroupNotFound(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "workspace-ns")

	ws := makeWorkspaceWithStatus("my-workspace", "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "other-group",
			SessionURL: "http://session.workspace-ns.svc.cluster.local",
			MemoryURL:  "http://memory.workspace-ns.svc.cluster.local",
			Ready:      true,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error when service group not found")
	}
}

func TestResolveServiceURLs_NoWorkspaceForNamespace(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "some-namespace")

	// No workspace whose spec.namespace.name == "some-namespace"
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error when no workspace found for namespace")
	}
}

func TestResolveServiceURLs_NamespaceFromFile(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "") // no env var; must fall back to file

	// Write a temp namespace file.
	nsFile := t.TempDir() + "/namespace"
	if err := os.WriteFile(nsFile, []byte("file-ns"), 0600); err != nil {
		t.Fatalf("write temp namespace file: %v", err)
	}

	// Temporarily override the namespace file path.
	origPath := namespaceFilePath
	namespaceFilePath = nsFile
	t.Cleanup(func() { namespaceFilePath = origPath })

	ws := makeWorkspaceWithStatus("my-workspace", "file-ns", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "default",
			SessionURL: "http://session.svc",
			MemoryURL:  "http://memory.svc",
			Ready:      true,
		},
	})
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	urls, err := r.ResolveServiceURLs(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session.svc" {
		t.Errorf("unexpected session URL: %s", urls.SessionURL)
	}
}

func TestResolveByWorkspaceName_Success(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")

	ws := makeWorkspaceWithStatus("dev-agents", "dev-agents", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "default",
			SessionURL: "http://session-dev-agents-default.dev-agents:8080",
			MemoryURL:  "http://memory-dev-agents-default.dev-agents:8080",
			Ready:      true,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	urls, err := r.ResolveByWorkspaceName(context.Background(), "dev-agents", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session-dev-agents-default.dev-agents:8080" {
		t.Errorf("unexpected session URL: %s", urls.SessionURL)
	}
	if urls.MemoryURL != "http://memory-dev-agents-default.dev-agents:8080" {
		t.Errorf("unexpected memory URL: %s", urls.MemoryURL)
	}
}

func TestResolveByWorkspaceName_NotFound(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveByWorkspaceName(context.Background(), "nonexistent", "default")
	if err == nil {
		t.Fatal("expected error when workspace not found")
	}
}

func TestResolveByWorkspaceName_ServiceGroupNotFound(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")

	ws := makeWorkspaceWithStatus("dev-agents", "dev-agents", []omniav1alpha1.ServiceGroupStatus{
		{
			Name:       "premium",
			SessionURL: "http://session.svc",
			MemoryURL:  "http://memory.svc",
			Ready:      true,
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveByWorkspaceName(context.Background(), "dev-agents", "default")
	if err == nil {
		t.Fatal("expected error when service group not found")
	}
}

func TestResolveByWorkspaceName_EnvVarOverride(t *testing.T) {
	t.Setenv(envSessionAPIURL, "http://override-session")
	t.Setenv(envMemoryAPIURL, "http://override-memory")

	r := NewResolver(nil)
	urls, err := r.ResolveByWorkspaceName(context.Background(), "any", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://override-session" {
		t.Errorf("unexpected session URL: %s", urls.SessionURL)
	}
}

func TestResolveServiceURLs_NamespaceFileNotFound(t *testing.T) {
	t.Setenv(envSessionAPIURL, "")
	t.Setenv(envMemoryAPIURL, "")
	t.Setenv(envOmniaNamespace, "")

	origPath := namespaceFilePath
	namespaceFilePath = "/nonexistent/namespace"
	t.Cleanup(func() { namespaceFilePath = origPath })

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error when namespace file not found")
	}
}
