/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


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
	// Path is the local filesystem path to the artifact directory.
	// The fetcher writes content directly to this directory (not as a tarball).
	// Callers are responsible for cleaning up this directory when done.
	Path string

	// Revision is the source revision identifier (e.g., commit SHA, tag).
	Revision string

	// Checksum is the SHA256 checksum of the directory contents.
	// Calculated using CalculateDirectoryHash for content-addressable versioning.
	Checksum string

	// Size is the total size of all files in the directory in bytes.
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
	// The artifact is stored as a directory at a temporary path.
	// Callers are responsible for cleaning up the artifact directory when done.
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
