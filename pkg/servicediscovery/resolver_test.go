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

func TestResolveServiceURLs_NoClient(t *testing.T) {
	r := NewResolver(nil)
	_, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err == nil {
		t.Fatal("expected error when there is no Kubernetes client to resolve with")
	}
}

func TestResolveServiceURLs_FromWorkspaceStatus(t *testing.T) {
	t.Setenv(envOmniaNamespace, "workspace-ns")

	ws := makeWorkspaceWithStatus(testWorkspaceName, "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
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
	urls, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
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
	t.Setenv(envOmniaNamespace, "workspace-ns")

	// Ready=false but URLs populated — should succeed. Callers that only
	// need session-api shouldn't be blocked by memory-api failures.
	ws := makeWorkspaceWithStatus(testWorkspaceName, "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
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
	urls, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://session.workspace-ns.svc.cluster.local" {
		t.Errorf("expected session URL, got %s", urls.SessionURL)
	}
}

func TestResolveServiceURLs_ServiceGroupNoURLs(t *testing.T) {
	t.Setenv(envOmniaNamespace, "workspace-ns")

	// No URLs populated — should fail even if the group exists.
	ws := makeWorkspaceWithStatus(testWorkspaceName, "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
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
	_, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err == nil {
		t.Fatal("expected error when service group has no URLs")
	}
}

func TestResolveServiceURLs_ServiceGroupNotFound(t *testing.T) {
	t.Setenv(envOmniaNamespace, "workspace-ns")

	ws := makeWorkspaceWithStatus(testWorkspaceName, "workspace-ns", []omniav1alpha1.ServiceGroupStatus{
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
	_, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err == nil {
		t.Fatal("expected error when service group not found")
	}
}

func TestResolveServiceURLs_WorkspaceNotFound(t *testing.T) {

	// No Workspace named testWorkspaceName exists.
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err == nil {
		t.Fatal("expected error when the named workspace does not exist")
	}
}

func TestResolveServiceURLs_RequiresAWorkspaceName(t *testing.T) {

	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "", "default")
	if err == nil {
		t.Fatal("expected error when no workspace name is supplied")
	}
}

// The workspace is named "demo" and owns the namespace "omnia-demo". Looking up
// by the namespace must fail: resolution is by the Workspace's metadata.name,
// and the two identifiers are not interchangeable (#1875).
func TestResolveServiceURLs_NamespaceIsNotAWorkspaceName(t *testing.T) {

	ws := makeWorkspaceWithStatus("demo", "omnia-demo", []omniav1alpha1.ServiceGroupStatus{
		{Name: "default", SessionURL: "http://session.svc", MemoryURL: "http://memory.svc", Ready: true},
	})
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()
	r := NewResolver(fakeClient)

	if _, err := r.ResolveServiceURLs(context.Background(), "omnia-demo", "default"); err == nil {
		t.Fatal("expected error: the namespace is not the workspace name")
	}

	urls, err := r.ResolveServiceURLs(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("unexpected error resolving by workspace name: %v", err)
	}
	if urls.SessionURL != "http://session.svc" {
		t.Errorf("unexpected session URL: %s", urls.SessionURL)
	}
}

// GetWorkspace exists so callers wanting more than URLs off the same object —
// the runtime needs metadata.uid to scope memory — reuse one read (#1875).
func TestGetWorkspace_ReturnsTheObjectIncludingUID(t *testing.T) {
	ws := makeWorkspaceWithStatus("demo", "omnia-demo", nil)
	ws.UID = "ws-uid-123"
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	got, err := NewResolver(fakeClient).GetWorkspace(context.Background(), "demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got.UID) != "ws-uid-123" {
		t.Errorf("unexpected UID: %s", got.UID)
	}
}

func TestResolveByWorkspaceName_Success(t *testing.T) {

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

func TestResolveServiceURLs_PrivacyURLFromWorkspaceStatus(t *testing.T) {
	t.Setenv(envOmniaNamespace, "workspace-ns")

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: testWorkspaceName,
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "workspace-ns"},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			PrivacyURL: "http://privacy.workspace-ns.svc.cluster.local",
			Services: []omniav1alpha1.ServiceGroupStatus{
				{
					Name:       "default",
					SessionURL: "http://session.workspace-ns.svc.cluster.local",
					MemoryURL:  "http://memory.workspace-ns.svc.cluster.local",
					Ready:      true,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	r := NewResolver(fakeClient)
	urls, err := r.ResolveServiceURLs(context.Background(), testWorkspaceName, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.PrivacyURL != "http://privacy.workspace-ns.svc.cluster.local" {
		t.Errorf("unexpected privacy URL: %s", urls.PrivacyURL)
	}
}

func TestDetectNamespace(t *testing.T) {
	t.Run("returns OMNIA_NAMESPACE when set", func(t *testing.T) {
		t.Setenv("OMNIA_NAMESPACE", "my-ns")
		if got := DetectNamespace(); got != "my-ns" {
			t.Errorf("DetectNamespace() = %q, want my-ns", got)
		}
	})
	t.Run("falls back to default when env unset and no SA file", func(t *testing.T) {
		t.Setenv("OMNIA_NAMESPACE", "")
		// Test/CI environments have no in-cluster SA namespace file, so
		// currentNamespace errors and DetectNamespace returns the fallback.
		if got := DetectNamespace(); got != "default" {
			t.Errorf("DetectNamespace() = %q, want default", got)
		}
	})
}

// A group with a session URL but no memory URL is legitimate. The old
// all-or-nothing gate could not express this, which is why the facade could
// never use the env path.
func TestSessionURL_ResolvesWithoutAMemoryURL(t *testing.T) {

	ws := makeWorkspaceWithStatus("demo", "omnia-demo", []omniav1alpha1.ServiceGroupStatus{
		{Name: "default", SessionURL: "http://session.svc"},
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(ws).Build()
	r := NewResolver(c)

	got, err := r.SessionURL(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://session.svc" {
		t.Errorf("unexpected session URL: %s", got)
	}

	// Absent memory is not an error — the caller did not require it.
	mem, err := r.MemoryURL(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("unexpected error for absent memory URL: %v", err)
	}
	if mem != "" {
		t.Errorf("expected empty memory URL, got %s", mem)
	}
}

// Session is required: every caller needs it, so its absence is an error
// rather than an empty string that fails later and further away.
func TestSessionURL_ErrorsWhenAbsent(t *testing.T) {

	ws := makeWorkspaceWithStatus("demo", "omnia-demo", []omniav1alpha1.ServiceGroupStatus{
		{Name: "default"},
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(ws).Build()

	if _, err := NewResolver(c).SessionURL(context.Background(), "demo", "default"); err == nil {
		t.Fatal("expected an error when the group has no session URL")
	}
}

// The env vars are no longer a discovery shortcut: with both set and a
// Workspace that disagrees, the Workspace wins.
func TestResolveServiceURLs_IgnoresEnvOverrides(t *testing.T) {
	t.Setenv("SESSION_API_URL", "http://from-env")
	t.Setenv("MEMORY_API_URL", "http://from-env")

	ws := makeWorkspaceWithStatus("demo", "omnia-demo", []omniav1alpha1.ServiceGroupStatus{
		{Name: "default", SessionURL: "http://from-workspace", Ready: true},
	})
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(ws).Build()

	urls, err := NewResolver(c).ResolveServiceURLs(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urls.SessionURL != "http://from-workspace" {
		t.Errorf("env override still in effect: %s", urls.SessionURL)
	}
}
