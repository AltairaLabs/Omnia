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
	"fmt"
	"time"
)

// BackendType selects which media storage backend Build constructs.
type BackendType string

const (
	// BackendTypeNone disables media storage. Build returns (nil, nil).
	BackendTypeNone BackendType = "none"
	// BackendTypeLocal uses the local filesystem for media storage.
	BackendTypeLocal BackendType = "local"
	// BackendTypeS3 uses Amazon S3 or an S3-compatible service.
	BackendTypeS3 BackendType = "s3"
	// BackendTypeGCS uses Google Cloud Storage.
	BackendTypeGCS BackendType = "gcs"
	// BackendTypeAzure uses Azure Blob Storage.
	BackendTypeAzure BackendType = "azure"
)

// BuilderConfig carries the storage-backend-agnostic settings needed to
// construct a Storage implementation. Both cmd/agent (the facade binary) and
// cmd/runtime build one of these from their own env/CRD-derived config and
// call Build, so the four backend-construction paths (local/S3/GCS/Azure)
// live in exactly one place instead of being duplicated per binary.
type BuilderConfig struct {
	// Type selects the backend. BackendTypeNone (or "") disables storage —
	// Build returns (nil, nil) rather than an error, so callers can treat a
	// nil Storage as "skip WithMediaStorage" without special-casing errors.
	Type BackendType

	// DefaultTTL is how long stored media is retained before expiry (zero
	// means no expiry). MaxFileSize caps upload size in bytes (zero means no
	// limit). Both apply to every backend.
	DefaultTTL  time.Duration
	MaxFileSize int64

	// UploadURLTTL / DownloadURLTTL bound how long presigned upload/download
	// URLs remain valid. The local backend ignores DownloadURLTTL — it
	// doesn't presign downloads.
	UploadURLTTL   time.Duration
	DownloadURLTTL time.Duration

	// Local backend.
	LocalPath string
	// LocalBaseURL is the base URL used to build download URLs for the local
	// backend (e.g. "http://localhost:<facadePort>"). The facade knows its
	// own port; the runtime does not have a guaranteed way to learn it (see
	// cmd/runtime's localMediaBaseURL) and must supply its best guess here.
	LocalBaseURL string

	// S3 backend.
	S3Bucket   string
	S3Region   string
	S3Prefix   string
	S3Endpoint string // optional, for S3-compatible services (MinIO, LocalStack)

	// GCS backend.
	GCSBucket string
	GCSPrefix string

	// Azure Blob backend.
	AzureAccount   string
	AzureContainer string
	AzurePrefix    string
	AzureKey       string // optional; DefaultAzureCredential is used when empty
}

// Build constructs the Storage backend selected by cfg.Type. It returns
// (nil, nil) when storage is disabled (BackendTypeNone or an empty Type) —
// callers must treat a nil Storage as "don't wire media storage", not as an
// error condition.
func Build(ctx context.Context, cfg BuilderConfig) (Storage, error) {
	switch cfg.Type {
	case BackendTypeNone, "":
		return nil, nil
	case BackendTypeLocal:
		return NewLocalStorage(LocalStorageConfig{
			BasePath:     cfg.LocalPath,
			BaseURL:      cfg.LocalBaseURL,
			DefaultTTL:   cfg.DefaultTTL,
			UploadURLTTL: cfg.UploadURLTTL,
			MaxFileSize:  cfg.MaxFileSize,
		})
	case BackendTypeS3:
		return NewS3Storage(ctx, S3Config{
			Bucket:         cfg.S3Bucket,
			Region:         cfg.S3Region,
			Prefix:         cfg.S3Prefix,
			Endpoint:       cfg.S3Endpoint,
			UsePathStyle:   cfg.S3Endpoint != "", // path-style required for MinIO/custom endpoints
			UploadURLTTL:   cfg.UploadURLTTL,
			DownloadURLTTL: cfg.DownloadURLTTL,
			DefaultTTL:     cfg.DefaultTTL,
			MaxFileSize:    cfg.MaxFileSize,
		})
	case BackendTypeGCS:
		return NewGCSStorage(ctx, GCSConfig{
			Bucket:         cfg.GCSBucket,
			Prefix:         cfg.GCSPrefix,
			UploadURLTTL:   cfg.UploadURLTTL,
			DownloadURLTTL: cfg.DownloadURLTTL,
			DefaultTTL:     cfg.DefaultTTL,
			MaxFileSize:    cfg.MaxFileSize,
		})
	case BackendTypeAzure:
		return NewAzureStorage(ctx, AzureConfig{
			AccountName:    cfg.AzureAccount,
			ContainerName:  cfg.AzureContainer,
			Prefix:         cfg.AzurePrefix,
			AccountKey:     cfg.AzureKey,
			UploadURLTTL:   cfg.UploadURLTTL,
			DownloadURLTTL: cfg.DownloadURLTTL,
			DefaultTTL:     cfg.DefaultTTL,
			MaxFileSize:    cfg.MaxFileSize,
		})
	default:
		return nil, fmt.Errorf("unknown media storage type: %q", cfg.Type)
	}
}
