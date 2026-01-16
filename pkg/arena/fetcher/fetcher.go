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

// Package fetcher provides interfaces and implementations for fetching
// PromptKit bundles from various sources (Git, OCI, ConfigMap).
package fetcher

import (
	"context"
	"time"
)

// Artifact represents a fetched bundle artifact.
type Artifact struct {
	// Path is the local filesystem path to the artifact tarball.
	Path string

	// Revision is the source revision identifier (e.g., commit SHA, tag).
	Revision string

	// Checksum is the SHA256 checksum of the artifact.
	Checksum string

	// Size is the artifact size in bytes.
	Size int64

	// LastModified is when the source was last modified.
	LastModified time.Time
}

// Fetcher defines the interface for fetching PromptKit bundles from sources.
type Fetcher interface {
	// LatestRevision returns the latest available revision from the source.
	// For Git sources, this is the commit SHA at the specified ref.
	// For OCI sources, this is the digest of the specified tag.
	// For ConfigMap sources, this is the resourceVersion.
	LatestRevision(ctx context.Context) (string, error)

	// Fetch downloads the bundle at the specified revision and returns an Artifact.
	// The artifact is stored as a tarball at a temporary path.
	// Callers are responsible for cleaning up the artifact path when done.
	Fetch(ctx context.Context, revision string) (*Artifact, error)

	// Type returns the source type (git, oci, configmap).
	Type() string
}

// Options contains common options for fetcher implementations.
type Options struct {
	// Timeout is the maximum duration for fetch operations.
	Timeout time.Duration

	// WorkDir is the directory for temporary files during fetch.
	WorkDir string
}

// DefaultOptions returns default fetcher options.
func DefaultOptions() Options {
	return Options{
		Timeout: 60 * time.Second,
		WorkDir: "",
	}
}
