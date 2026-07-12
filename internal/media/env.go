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
// read these to build a BuilderConfig for Build. There is no CRD field for
// backend selection today (AgentRuntime's spec.media only carries BasePath,
// used for mock:// resolution, a different concern) — these are pod-env-only
// on whichever container needs media storage, typically set via
// spec.podOverrides.
const (
	EnvStorageType = "OMNIA_MEDIA_STORAGE_TYPE"
	EnvStoragePath = "OMNIA_MEDIA_STORAGE_PATH"
	EnvMaxFileSize = "OMNIA_MEDIA_MAX_FILE_SIZE"
	EnvDefaultTTL  = "OMNIA_MEDIA_DEFAULT_TTL"

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
	DefaultStoragePath = "/var/lib/omnia/media"
	DefaultMaxFileSize = 100 * 1024 * 1024 // 100MB
	DefaultDefaultTTL  = 24 * time.Hour
)
