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

// Package cold implements the ColdArchiveProvider interface with multi-backend
// object storage support (S3, GCS, Azure Blob) using Parquet format.
package cold

import (
	"context"
	"errors"
)

// ErrObjectNotFound is returned when a requested object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// BlobStore abstracts raw object I/O across cloud storage backends.
type BlobStore interface {
	// Put writes data to the given key with the specified content type.
	Put(ctx context.Context, key string, data []byte, contentType string) error

	// Get retrieves data for the given key.
	// Returns ErrObjectNotFound if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes the object at the given key.
	// Returns ErrObjectNotFound if the key does not exist.
	Delete(ctx context.Context, key string) error

	// List returns all keys matching the given prefix.
	List(ctx context.Context, prefix string) ([]string, error)

	// Exists checks whether an object exists at the given key.
	Exists(ctx context.Context, key string) (bool, error)

	// Ping checks connectivity to the underlying store.
	Ping(ctx context.Context) error

	// Close releases resources held by the store.
	Close() error
}
