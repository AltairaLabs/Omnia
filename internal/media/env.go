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

import "time"

// Environment variable names for the OMNIA_MEDIA_* backend-selection
// contract. Both the facade (cmd/agent) and runtime (cmd/runtime) binaries
// read these to build a BuilderConfig for Build. spec.media.storage (the
// AgentRuntime CRD) is the source of truth for these settings: the operator
// (internal/controller/media_storage_env.go) renders the CRD fields into this
// exact env contract on both containers. They remain pod-env rather than a
// direct CRD read in each binary so the same contract also works when a
// binary runs outside the operator's control (demo mode, E2E, manually via
// spec.podOverrides).
const (
	EnvStorageType = "OMNIA_MEDIA_STORAGE_TYPE"
	EnvStoragePath = "OMNIA_MEDIA_STORAGE_PATH"
	EnvMaxFileSize = "OMNIA_MEDIA_MAX_FILE_SIZE"
	EnvDefaultTTL  = "OMNIA_MEDIA_DEFAULT_TTL"

	// EnvUploadURLTTL / EnvDownloadURLTTL configure how long presigned
	// upload/download URLs remain valid. Rendered from
	// spec.media.storage.uploadURLTTL / downloadURLTTL by
	// appendMediaLimits (internal/controller/media_storage_env.go).
	EnvUploadURLTTL   = "OMNIA_MEDIA_UPLOAD_URL_TTL"
	EnvDownloadURLTTL = "OMNIA_MEDIA_DOWNLOAD_URL_TTL"

	// S3 storage configuration.
	EnvS3Bucket   = "OMNIA_MEDIA_S3_BUCKET"
	EnvS3Region   = "OMNIA_MEDIA_S3_REGION"
	EnvS3Prefix   = "OMNIA_MEDIA_S3_PREFIX"
	EnvS3Endpoint = "OMNIA_MEDIA_S3_ENDPOINT" // Optional, for S3-compatible services (MinIO, LocalStack)

	// GCS storage configuration.
	EnvGCSBucket = "OMNIA_MEDIA_GCS_BUCKET"
	EnvGCSPrefix = "OMNIA_MEDIA_GCS_PREFIX"

	// Azure Blob Storage configuration.
	EnvAzureAccount   = "OMNIA_MEDIA_AZURE_ACCOUNT"
	EnvAzureContainer = "OMNIA_MEDIA_AZURE_CONTAINER"
	EnvAzurePrefix    = "OMNIA_MEDIA_AZURE_PREFIX"
	EnvAzureKey       = "OMNIA_MEDIA_AZURE_KEY" // Optional, for cross-cloud or explicit credentials
)

// Default values shared by both binaries' env-var loading.
const (
	DefaultStoragePath    = "/var/lib/omnia/media"
	DefaultMaxFileSize    = 100 * 1024 * 1024 // 100MB
	DefaultDefaultTTL     = 24 * time.Hour
	DefaultUploadURLTTL   = 15 * time.Minute
	DefaultDownloadURLTTL = 1 * time.Hour
)
