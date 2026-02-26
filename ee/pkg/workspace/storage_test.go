// Copyright (c) 2025 Altaira Labs
// SPDX-License-Identifier: FSL-1.1-Apache-2.0

package workspace

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)
	return scheme
}

func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(objs...).
		Build()
}

func TestStorageManager_EnsureWorkspacePVC(t *testing.T) {
	tests := []struct {
		name             string
		workspace        *omniav1alpha1.Workspace
		existingPVC      *corev1.PersistentVolumeClaim
		defaultStorClass string
		wantErr          bool
		wantErrContains  string
		wantPVCName      string
		wantStorageClass string
	}{
		{
			name: "creates PVC when storage is enabled",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
						Size:    "5Gi",
					},
				},
			},
			wantErr:     false,
			wantPVCName: "workspace-test-ns-content",
		},
		{
			name: "creates PVC with custom storage class",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled:      ptr.To(true),
						StorageClass: "custom-nfs",
					},
				},
			},
			wantErr:          false,
			wantPVCName:      "workspace-test-ns-content",
			wantStorageClass: "custom-nfs",
		},
		{
			name: "uses default storage class when not specified in workspace",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			defaultStorClass: "default-storage",
			wantErr:          false,
			wantPVCName:      "workspace-test-ns-content",
			wantStorageClass: "default-storage",
		},
		{
			name: "returns empty PVC name when storage is disabled",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(false),
					},
				},
			},
			wantErr:     false,
			wantPVCName: "", // Empty PVC name when storage is disabled
		},
		{
			name: "returns empty PVC name when storage is nil",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: nil,
				},
			},
			wantErr:     false,
			wantPVCName: "", // Empty PVC name when storage is nil
		},
		{
			name: "is idempotent when PVC already exists",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			existingPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workspace-test-ns-content",
					Namespace: "test-ns",
					Labels: map[string]string{
						LabelWorkspace:        "test-workspace",
						LabelWorkspaceManaged: LabelValueTrue,
					},
				},
			},
			wantErr:     false,
			wantPVCName: "workspace-test-ns-content",
		},
		{
			name: "enables storage by default when enabled is nil",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						// Enabled is nil - should default to true
					},
				},
			},
			wantErr:     false,
			wantPVCName: "workspace-test-ns-content",
		},
		{
			name: "creates PVC with custom access modes",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled:     ptr.To(true),
						AccessModes: []string{"ReadWriteOnce", "ReadOnlyMany"},
					},
				},
			},
			wantErr:     false,
			wantPVCName: "workspace-test-ns-content",
		},
		{
			name: "returns error when storage size is invalid",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
						Size:    "invalid-size",
					},
				},
			},
			wantErr:         true,
			wantErrContains: "invalid storage size",
		},
		{
			name: "updates labels on existing PVC with nil labels",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			existingPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workspace-test-ns-content",
					Namespace: "test-ns",
					// No labels set - tests nil labels path
				},
			},
			wantErr:     false,
			wantPVCName: "workspace-test-ns-content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build client with workspace and optionally existing PVC
			objs := []client.Object{tt.workspace}
			if tt.existingPVC != nil {
				objs = append(objs, tt.existingPVC)
			}
			// Create namespace for PVC creation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.workspace.Spec.Namespace.Name,
				},
			}
			objs = append(objs, ns)

			c := newFakeClient(objs...)
			sm := NewStorageManager(c, tt.defaultStorClass)

			pvcName, err := sm.EnsureWorkspacePVC(context.Background(), tt.workspace.Name)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.wantErrContains != "" && !contains(err.Error(), tt.wantErrContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if pvcName != tt.wantPVCName {
				t.Errorf("got PVC name %q, want %q", pvcName, tt.wantPVCName)
			}

			// Skip PVC verification if no PVC is expected (storage disabled)
			if tt.wantPVCName == "" {
				return
			}

			// Verify PVC was created
			pvc := &corev1.PersistentVolumeClaim{}
			err = c.Get(context.Background(), client.ObjectKey{
				Name:      tt.wantPVCName,
				Namespace: tt.workspace.Spec.Namespace.Name,
			}, pvc)
			if err != nil {
				t.Errorf("failed to get PVC: %v", err)
				return
			}

			// Verify labels
			if pvc.Labels[LabelWorkspace] != tt.workspace.Name {
				t.Errorf("PVC workspace label = %q, want %q", pvc.Labels[LabelWorkspace], tt.workspace.Name)
			}
			if pvc.Labels[LabelWorkspaceManaged] != LabelValueTrue {
				t.Errorf("PVC managed label = %q, want %q", pvc.Labels[LabelWorkspaceManaged], LabelValueTrue)
			}

			// Verify storage class if expected
			if tt.wantStorageClass != "" {
				if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != tt.wantStorageClass {
					actual := "<nil>"
					if pvc.Spec.StorageClassName != nil {
						actual = *pvc.Spec.StorageClassName
					}
					t.Errorf("PVC storage class = %q, want %q", actual, tt.wantStorageClass)
				}
			}
		})
	}
}

func TestStorageManager_GetWorkspacePVCName(t *testing.T) {
	tests := []struct {
		name      string
		workspace *omniav1alpha1.Workspace
		wantName  string
		wantErr   bool
	}{
		{
			name: "returns PVC name when storage enabled",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			wantName: "workspace-test-ns-content",
		},
		{
			name: "returns empty when storage disabled",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: ptr.To(false),
					},
				},
			},
			wantName: "",
		},
		{
			name: "returns empty when storage nil",
			workspace: &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name: "test-ns",
					},
					Storage: nil,
				},
			},
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeClient(tt.workspace)
			sm := NewStorageManager(c, "")

			name, err := sm.GetWorkspacePVCName(context.Background(), tt.workspace.Name)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if name != tt.wantName {
				t.Errorf("got %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestStorageManager_PVCExists(t *testing.T) {
	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workspace",
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace",
			Namespace: omniav1alpha1.NamespaceConfig{
				Name: "test-ns",
			},
			Storage: &omniav1alpha1.WorkspaceStorageConfig{
				Enabled: ptr.To(true),
			},
		},
	}

	t.Run("returns false when PVC does not exist", func(t *testing.T) {
		c := newFakeClient(workspace)
		sm := NewStorageManager(c, "")

		exists, err := sm.PVCExists(context.Background(), "test-workspace")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if exists {
			t.Errorf("expected PVC to not exist")
		}
	})

	t.Run("returns true when PVC exists", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workspace-test-ns-content",
				Namespace: "test-ns",
			},
		}
		c := newFakeClient(workspace, pvc)
		sm := NewStorageManager(c, "")

		exists, err := sm.PVCExists(context.Background(), "test-workspace")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if !exists {
			t.Errorf("expected PVC to exist")
		}
	})
}

func TestStorageManager_WorkspaceNotFound(t *testing.T) {
	// Create client with no workspace
	c := newFakeClient()
	sm := NewStorageManager(c, "")

	t.Run("EnsureWorkspacePVC returns error when workspace not found", func(t *testing.T) {
		_, err := sm.EnsureWorkspacePVC(context.Background(), "non-existent")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !contains(err.Error(), "failed to get workspace") {
			t.Errorf("expected error to contain 'failed to get workspace', got: %v", err)
		}
	})

	t.Run("GetWorkspacePVCName returns error when workspace not found", func(t *testing.T) {
		_, err := sm.GetWorkspacePVCName(context.Background(), "non-existent")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !contains(err.Error(), "failed to get workspace") {
			t.Errorf("expected error to contain 'failed to get workspace', got: %v", err)
		}
	})

	t.Run("PVCExists returns error when workspace not found", func(t *testing.T) {
		_, err := sm.PVCExists(context.Background(), "non-existent")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !contains(err.Error(), "failed to get workspace") {
			t.Errorf("expected error to contain 'failed to get workspace', got: %v", err)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
