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

package media

import (
	"context"
	"testing"
	"time"
)

func TestBuild_NoneReturnsNilStorage(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{Type: BackendTypeNone})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if store != nil {
		t.Errorf("Build() store = %v, want nil for BackendTypeNone", store)
	}
}

func TestBuild_EmptyTypeReturnsNilStorage(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if store != nil {
		t.Errorf("Build() store = %v, want nil for an empty Type", store)
	}
}

func TestBuild_Local(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{
		Type:         BackendTypeLocal,
		LocalPath:    t.TempDir(),
		LocalBaseURL: "http://localhost:8080",
		DefaultTTL:   24 * time.Hour,
		UploadURLTTL: 15 * time.Minute,
		MaxFileSize:  1024,
	})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if store == nil {
		t.Fatal("Build() store is nil, want a constructed *LocalStorage")
	}
	if _, ok := store.(*LocalStorage); !ok {
		t.Errorf("Build() store type = %T, want *LocalStorage", store)
	}
	if err := store.Close(); err != nil {
		t.Errorf("store.Close() error = %v", err)
	}
}

func TestBuild_S3(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{
		Type:           BackendTypeS3,
		S3Bucket:       "probe-bucket",
		S3Region:       "us-east-1",
		S3Prefix:       "media",
		DefaultTTL:     24 * time.Hour,
		UploadURLTTL:   15 * time.Minute,
		DownloadURLTTL: 1 * time.Hour,
		MaxFileSize:    1024,
	})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if _, ok := store.(*S3Storage); !ok {
		t.Errorf("Build() store type = %T, want *S3Storage", store)
	}
}

func TestBuild_S3_CustomEndpointUsesPathStyle(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{
		Type:       BackendTypeS3,
		S3Bucket:   "probe-bucket",
		S3Region:   "us-east-1",
		S3Endpoint: "http://localhost:9000", // MinIO-style custom endpoint
	})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	s3Store, ok := store.(*S3Storage)
	if !ok {
		t.Fatalf("Build() store type = %T, want *S3Storage", store)
	}
	if !s3Store.config.UsePathStyle {
		t.Error("Build() did not set UsePathStyle for a custom S3 endpoint")
	}
}

func TestBuild_GCS(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{
		Type:      BackendTypeGCS,
		GCSBucket: "probe-bucket",
		GCSPrefix: "media",
	})
	if err != nil {
		// The GCS client requires application-default credentials that are
		// unavailable in CI; skip rather than fail when construction can't happen.
		t.Skipf("GCS backend not constructible in this environment (no credentials): %v", err)
	}
	if _, ok := store.(*GCSStorage); !ok {
		t.Errorf("Build() store type = %T, want *GCSStorage", store)
	}
}

func TestBuild_Azure(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{
		Type:           BackendTypeAzure,
		AzureAccount:   "probeacct",
		AzureContainer: "probecontainer",
		AzurePrefix:    "media",
	})
	if err != nil {
		// The Azure client requires account credentials that may be unavailable
		// in CI; skip rather than fail when construction can't happen.
		t.Skipf("Azure backend not constructible in this environment (no credentials): %v", err)
	}
	if _, ok := store.(*AzureStorage); !ok {
		t.Errorf("Build() store type = %T, want *AzureStorage", store)
	}
}

func TestBuild_UnknownTypeReturnsError(t *testing.T) {
	store, err := Build(context.Background(), BuilderConfig{Type: BackendType("ftp")})
	if err == nil {
		t.Fatal("Build() error = nil, want an error for an unrecognized backend type")
	}
	if store != nil {
		t.Errorf("Build() store = %v, want nil alongside the error", store)
	}
}
