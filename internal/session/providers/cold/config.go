/*
Copyright 2025.

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

package cold

// BackendType identifies the object storage backend.
type BackendType string

const (
	// BackendS3 uses Amazon S3 or S3-compatible storage (e.g. MinIO).
	BackendS3 BackendType = "s3"
	// BackendGCS uses Google Cloud Storage.
	BackendGCS BackendType = "gcs"
	// BackendAzure uses Azure Blob Storage.
	BackendAzure BackendType = "azure"
)

// Config configures a cold archive Provider.
type Config struct {
	// Backend selects the object storage implementation.
	Backend BackendType
	// Bucket is the bucket (S3/GCS) or container (Azure) name.
	Bucket string
	// Prefix is the base path prefix for all objects (default "sessions/").
	Prefix string
	// DefaultCompression is the Parquet compression codec (default "snappy").
	DefaultCompression string
	// DefaultMaxFileSize is the maximum Parquet file size in bytes (default 128MB).
	DefaultMaxFileSize int64
	// S3 contains S3-specific configuration. Required when Backend == BackendS3.
	S3 *S3Config
	// GCS contains GCS-specific configuration. Required when Backend == BackendGCS.
	GCS *GCSConfig
	// Azure contains Azure-specific configuration. Required when Backend == BackendAzure.
	Azure *AzureConfig
}

// S3Config contains S3-specific settings.
type S3Config struct {
	// Region is the AWS region.
	Region string
	// Endpoint is an optional custom endpoint (for MinIO / S3-compatible).
	Endpoint string
	// AccessKeyID is the AWS access key (optional, uses IAM if not set).
	AccessKeyID string
	// SecretAccessKey is the AWS secret key (optional, uses IAM if not set).
	SecretAccessKey string
	// UsePathStyle forces path-style addressing (required for MinIO).
	UsePathStyle bool
}

// GCSConfig contains GCS-specific settings.
type GCSConfig struct {
	// CredentialsJSON contains the service account key JSON (optional, uses ADC if not set).
	CredentialsJSON []byte
}

// AzureConfig contains Azure Blob Storage-specific settings.
type AzureConfig struct {
	// AccountName is the Azure Storage account name.
	AccountName string
	// AccountKey is the storage account key (optional, uses DefaultAzureCredential if not set).
	AccountKey string
}

// Options configures a Provider when wrapping an existing BlobStore via NewFromBlobStore.
type Options struct {
	// Prefix is the base path prefix for all objects.
	Prefix string
	// DefaultCompression is the Parquet compression codec.
	DefaultCompression string
	// DefaultMaxFileSize is the maximum Parquet file size in bytes.
	DefaultMaxFileSize int64
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Prefix:             "sessions/",
		DefaultCompression: "snappy",
		DefaultMaxFileSize: 128 * 1024 * 1024, // 128MB
	}
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Prefix:             "sessions/",
		DefaultCompression: "snappy",
		DefaultMaxFileSize: 128 * 1024 * 1024, // 128MB
	}
}
