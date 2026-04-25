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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func int32Ptr(v int32) *int32 { return &v }

//nolint:unparam
func makeWorkspaceWithServices(name string, services []omniav1alpha1.WorkspaceServiceGroup) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: name,
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "test-ns"},
			Services:    services,
		},
	}
}

//nolint:unparam
func makeSecret(name, namespace, connStr string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			secretKeyPostgresConn: []byte(connStr),
		},
	}
}

func TestResolveSessionConfig(t *testing.T) {
	warmDays := int32Ptr(7)
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "session-db-secret"},
				},
				Retention: &omniav1alpha1.SessionRetentionConfig{
					WarmDays: warmDays,
				},
			},
		},
	})
	secret := makeSecret("session-db-secret", "test-ns", "postgres://host/db")

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws, secret).
		Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PostgresConn != "postgres://host/db" {
		t.Errorf("unexpected postgres conn: %s", cfg.PostgresConn)
	}
	if cfg.WarmDays == nil || *cfg.WarmDays != 7 {
		t.Errorf("expected warmDays=7, got %v", cfg.WarmDays)
	}
}

func TestResolveSessionConfig_NoRetention(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "session-db-secret"},
				},
			},
		},
	})
	secret := makeSecret("session-db-secret", "test-ns", "postgres://host/db")

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws, secret).
		Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WarmDays != nil {
		t.Errorf("expected nil warmDays, got %v", cfg.WarmDays)
	}
}

func TestResolveSessionConfig_WorkspaceNotFound(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveSessionConfig(context.Background(), "nonexistent", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when workspace not found")
	}
}

func TestResolveSessionConfig_ServiceGroupNotFound(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "other-group",
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "session-db-secret"},
				},
			},
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when service group not found")
	}
}

func TestResolveSessionConfig_NoSessionConfig(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			// Session is nil (external mode, no session config)
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when session config is nil")
	}
}

func TestResolveSessionConfig_SecretNotFound(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "missing-secret"},
				},
			},
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when secret not found")
	}
}

func TestResolveMemoryConfig(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "memory-db-secret"},
				},
				ProviderRef: &corev1.LocalObjectReference{Name: "my-embedding-provider"},
				PolicyRef:   &corev1.LocalObjectReference{Name: "my-memory-policy"},
			},
		},
	})
	secret := makeSecret("memory-db-secret", "test-ns", "postgres://memory-host/db")

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws, secret).
		Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveMemoryConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PostgresConn != "postgres://memory-host/db" {
		t.Errorf("unexpected postgres conn: %s", cfg.PostgresConn)
	}
	if cfg.EmbeddingProviderName != "my-embedding-provider" {
		t.Errorf("unexpected provider name: %s", cfg.EmbeddingProviderName)
	}
}

func TestResolveMemoryConfig_NoProvider(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "memory-db-secret"},
				},
				// ProviderRef is nil
			},
		},
	})
	secret := makeSecret("memory-db-secret", "test-ns", "postgres://memory-host/db")

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws, secret).
		Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveMemoryConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EmbeddingProviderName != "" {
		t.Errorf("expected empty provider name, got %s", cfg.EmbeddingProviderName)
	}
}

func TestResolveMemoryConfig_WorkspaceNotFound(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveMemoryConfig(context.Background(), "nonexistent", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when workspace not found")
	}
}

func TestResolveMemoryConfig_ServiceGroupNotFound(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "other-group",
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "memory-db-secret"},
				},
			},
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveMemoryConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when service group not found")
	}
}

func TestResolveMemoryConfig_NoMemoryConfig(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			// Memory is nil
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveMemoryConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when memory config is nil")
	}
}

func TestResolveMemoryConfig_SecretNotFound(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "missing-secret"},
				},
			},
		},
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveMemoryConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when secret not found")
	}
}

func TestResolveSessionConfig_SecretMissingKey(t *testing.T) {
	ws := makeWorkspaceWithServices("my-workspace", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "empty-secret"},
				},
			},
		},
	})
	// Secret exists but has no POSTGRES_CONN key.
	emptySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(ws, emptySecret).
		Build()

	cr := NewConfigResolver(fakeClient)
	_, err := cr.ResolveSessionConfig(context.Background(), "my-workspace", "default", "test-ns")
	if err == nil {
		t.Fatal("expected error when secret has no POSTGRES_CONN key")
	}
}
