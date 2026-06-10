/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package sourcesync

import (
	"context"
	"fmt"
	"os"
	"time"
)

// WorkspaceFetcher snapshots a directory that already lives on the workspace
// content volume. Unlike the git/oci/configmap fetchers it performs no external
// fetch and no staging: it points at the live editable directory in place and
// marks the artifact Preserve, so the version flow COPIES (not moves) the
// content into an immutable version dir and the caller skips cleanup of the
// source. This backs the ArenaSource "workspace" type used by dashboard deploy.
type WorkspaceFetcher struct {
	// SourceDir is the absolute path to the editable directory on the volume.
	SourceDir string
}

// NewWorkspaceFetcher creates a WorkspaceFetcher for the given absolute dir.
func NewWorkspaceFetcher(sourceDir string) *WorkspaceFetcher {
	return &WorkspaceFetcher{SourceDir: sourceDir}
}

// Type returns the source type.
func (f *WorkspaceFetcher) Type() string { return "workspace" }

// LatestRevision returns the content hash of the source dir, so that a content
// change is observed as a new revision (and a new version is snapshotted).
func (f *WorkspaceFetcher) LatestRevision(_ context.Context) (string, error) {
	return f.hash()
}

// Fetch returns an in-place artifact pointing at the live source dir. No copy
// is made here — the version flow copies it into an immutable version dir while
// leaving the editable source untouched.
func (f *WorkspaceFetcher) Fetch(_ context.Context, _ string) (*Artifact, error) {
	hash, err := f.hash()
	if err != nil {
		return nil, err
	}
	return &Artifact{
		Path:         f.SourceDir,
		Revision:     hash,
		Checksum:     hash,
		Preserve:     true,
		LastModified: time.Now(),
	}, nil
}

// hash validates the source is an existing directory and returns its content
// hash.
func (f *WorkspaceFetcher) hash() (string, error) {
	info, err := os.Stat(f.SourceDir)
	if err != nil {
		return "", fmt.Errorf("workspace source path not accessible: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace source path is not a directory: %s", f.SourceDir)
	}
	return CalculateDirectoryHash(f.SourceDir)
}
