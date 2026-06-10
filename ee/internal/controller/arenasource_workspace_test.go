/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestResolveWorkspaceSourceDir(t *testing.T) {
	base := filepath.Join("/workspace-content", "ws", "ns")
	tests := []struct {
		name    string
		rel     string
		target  string
		wantErr bool
		want    string
	}{
		{"valid", "arena/projects/p1", "arena/deployed/p1", false, filepath.Join(base, "arena/projects/p1")},
		{"absolute rejected", "/etc/passwd", "arena/x", true, ""},
		{"dotdot rejected", "../../etc", "arena/x", true, ""},
		{"escaping after clean rejected", "a/../../b", "arena/x", true, ""},
		{"empty rejected", "", "arena/x", true, ""},
		{"target equals source rejected", "arena/p", "arena/p", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveWorkspaceSourceDir(base, tt.rel, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCreateWorkspaceFetcher(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	content := t.TempDir()
	const ns = "default"
	// No Namespace label → GetWorkspaceForNamespace falls back to the
	// namespace name, so base is {content}/{ns}/{ns}.
	srcDir := filepath.Join(content, ns, ns, "arena", "projects", "p1")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "pack.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	r := &ArenaSourceReconciler{Client: cl, WorkspaceContentPath: content}
	newSource := func(path string) *omniav1alpha1.ArenaSource {
		return &omniav1alpha1.ArenaSource{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: ns},
			Spec: omniav1alpha1.ArenaSourceSpec{
				Type:       omniav1alpha1.ArenaSourceTypeWorkspace,
				Workspace:  &corev1alpha1.WorkspaceSource{Path: path},
				TargetPath: "arena/deployed/p1",
			},
		}
	}

	t.Run("resolves an existing dir", func(t *testing.T) {
		f, err := r.createWorkspaceFetcher(context.Background(), newSource("arena/projects/p1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.Type() != "workspace" {
			t.Fatalf("type = %q, want workspace", f.Type())
		}
	})

	t.Run("missing path errors", func(t *testing.T) {
		if _, err := r.createWorkspaceFetcher(context.Background(), newSource("arena/projects/missing")); err == nil {
			t.Fatal("expected error for missing path")
		}
	})

	t.Run("nil workspace block errors", func(t *testing.T) {
		src := newSource("arena/projects/p1")
		src.Spec.Workspace = nil
		if _, err := r.createWorkspaceFetcher(context.Background(), src); err == nil {
			t.Fatal("expected error for nil workspace block")
		}
	})

	t.Run("missing WorkspaceContentPath errors", func(t *testing.T) {
		r2 := &ArenaSourceReconciler{Client: cl}
		if _, err := r2.createWorkspaceFetcher(context.Background(), newSource("arena/projects/p1")); err == nil {
			t.Fatal("expected error when WorkspaceContentPath is empty")
		}
	})
}
