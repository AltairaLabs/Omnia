/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/media"
)

// startMediaValidationEnv boots a standalone envtest API server with the
// AgentRuntime CRD installed (CEL rules and all) so
// spec.media.storage per-type validation can be exercised against the real
// apiserver, independent of the package's shared Ginkgo envtest suite (see
// suite_test.go). It is self-contained so it also works when this test is
// selected on its own via `go test -run MediaValidation`, which does not
// trigger the Ginkgo suite's BeforeSuite.
func startMediaValidationEnv(t *testing.T) (client.Client, func()) {
	t.Helper()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := firstMediaValidationEnvTestBinaryDir(); dir != "" {
		env.BinaryAssetsDirectory = dir
	}

	cfg, err := env.Start()
	if err != nil {
		t.Skipf("envtest unavailable (run 'make setup-envtest' or set KUBEBUILDER_ASSETS): %v", err)
	}

	if err := omniav1alpha1.AddToScheme(scheme.Scheme); err != nil {
		_ = env.Stop()
		t.Fatalf("add scheme: %v", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		_ = env.Stop()
		t.Fatalf("client: %v", err)
	}

	return c, func() { _ = env.Stop() }
}

func firstMediaValidationEnvTestBinaryDir() string {
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

// mediaValidationAR builds a minimal, otherwise-valid AgentRuntime carrying
// the given media storage config, so each case exercises only the
// spec.media.storage CEL rules.
func mediaValidationAR(name string, storage *omniav1alpha1.MediaStorageConfig) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Mode:          omniav1alpha1.AgentRuntimeModeAgent,
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "pack"},
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeWebSocket},
			},
			Media: &omniav1alpha1.MediaConfig{Storage: storage},
		},
	}
}

// TestMediaValidation covers the spec.media.storage.type per-backend
// XValidation CEL rules added to MediaStorageConfig: each type requires its
// matching backend sub-config to be present.
func TestMediaValidation(t *testing.T) {
	c, stop := startMediaValidationEnv(t)
	defer stop()

	tests := []struct {
		name        string
		crName      string
		storage     *omniav1alpha1.MediaStorageConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "s3 type without s3 block is rejected",
			crName:      "media-s3-no-block",
			storage:     &omniav1alpha1.MediaStorageConfig{Type: "s3"},
			wantErr:     true,
			errContains: "requires spec.media.storage.s3",
		},
		{
			name:        "gcs type without gcs block is rejected",
			crName:      "media-gcs-no-block",
			storage:     &omniav1alpha1.MediaStorageConfig{Type: "gcs"},
			wantErr:     true,
			errContains: "requires spec.media.storage.gcs",
		},
		{
			name:        "azure type without azure block is rejected",
			crName:      "media-azure-no-block",
			storage:     &omniav1alpha1.MediaStorageConfig{Type: "azure"},
			wantErr:     true,
			errContains: "requires spec.media.storage.azure",
		},
		{
			name:        "local type without local block is rejected",
			crName:      "media-local-no-block",
			storage:     &omniav1alpha1.MediaStorageConfig{Type: string(media.BackendTypeLocal)},
			wantErr:     true,
			errContains: "requires spec.media.storage.local",
		},
		{
			name:   "valid s3 block is accepted",
			crName: "media-s3-valid",
			storage: &omniav1alpha1.MediaStorageConfig{
				Type: "s3",
				S3:   &omniav1alpha1.S3MediaBackend{Bucket: "my-bucket"},
			},
		},
		{
			name:   "valid gcs block is accepted",
			crName: "media-gcs-valid",
			storage: &omniav1alpha1.MediaStorageConfig{
				Type: "gcs",
				GCS:  &omniav1alpha1.GCSMediaBackend{Bucket: "my-bucket"},
			},
		},
		{
			name:   "valid azure block is accepted",
			crName: "media-azure-valid",
			storage: &omniav1alpha1.MediaStorageConfig{
				Type:  "azure",
				Azure: &omniav1alpha1.AzureMediaBackend{Account: "acct", Container: "c"},
			},
		},
		{
			name:   "valid local block is accepted",
			crName: "media-local-valid",
			storage: &omniav1alpha1.MediaStorageConfig{
				Type:  string(media.BackendTypeLocal),
				Local: &omniav1alpha1.LocalMediaBackend{BasePath: "/data/media"},
			},
		},
		{
			name:    "type none with no sub-config is accepted",
			crName:  "media-none-valid",
			storage: &omniav1alpha1.MediaStorageConfig{Type: "none"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := mediaValidationAR(tt.crName, tt.storage)
			err := c.Create(context.Background(), ar)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected Create to be rejected, got no error")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error to contain %q, got: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected Create to succeed, got: %v", err)
			}
		})
	}
}
