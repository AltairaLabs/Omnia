/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package sourcesync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// arenaDirName is the hidden directory under a workspace where Arena source
// artifacts are materialized (versions/HEAD layout). Extracted to satisfy
// go:S1192 (duplicated 5x across syncer path construction).
const arenaDirName = ".arena"

// StorageManager is the minimal interface FilesystemSyncer needs to ensure a
// workspace PVC exists before writing artifacts. The ee/pkg/workspace
// StorageManager type satisfies this interface.
//
// May be nil — when nil, the syncer skips lazy storage provisioning and
// assumes the PVC is already mounted.
type StorageManager interface {
	EnsureWorkspacePVC(ctx context.Context, workspaceName string) (string, error)
}

// FilesystemSyncer manages content-addressable filesystem sync for source content.
// It provides the shared pipeline: hash calculation, version storage, HEAD pointer update,
// and garbage collection of old versions.
type FilesystemSyncer struct {
	// WorkspaceContentPath is the base path for workspace content volumes.
	WorkspaceContentPath string

	// MaxVersionsPerSource is the maximum number of versions to retain per source.
	// Default is 10 if not set.
	MaxVersionsPerSource int

	// StorageManager optionally ensures workspace PVCs exist before writes.
	// When nil, the syncer assumes the PVC is already mounted. The
	// ee/pkg/workspace StorageManager satisfies this interface.
	StorageManager StorageManager
}

// SyncParams contains the parameters for a filesystem sync operation.
type SyncParams struct {
	// WorkspaceName is the resolved workspace name for the namespace.
	WorkspaceName string

	// Namespace is the Kubernetes namespace of the source.
	Namespace string

	// TargetPath is the relative path within the workspace content directory.
	TargetPath string

	// Artifact is the fetched artifact to sync.
	Artifact *Artifact
}

// SyncToFilesystem copies the artifact directory to the workspace content filesystem
// and creates a content-addressable version. It returns the relative content path and the version hash.
func (s *FilesystemSyncer) SyncToFilesystem(ctx context.Context, params SyncParams) (contentPath, version string, err error) {
	log := logf.FromContext(ctx).WithValues(
		"workspace", params.WorkspaceName,
		"namespace", params.Namespace,
		"targetPath", params.TargetPath,
	)

	if err := s.ensureWorkspacePVC(ctx, params.WorkspaceName); err != nil {
		return "", "", err
	}

	// Workspace content structure: {base}/{workspace}/{namespace}/{targetPath}
	workspacePath := filepath.Join(s.WorkspaceContentPath, params.WorkspaceName, params.Namespace, params.TargetPath)

	version, err = calculateVersion(params.Artifact)
	if err != nil {
		return "", "", err
	}

	// Check if this version already exists
	versionDir := filepath.Join(workspacePath, arenaDirName, "versions", version)
	if _, statErr := os.Stat(versionDir); statErr == nil {
		log.V(1).Info("Version already exists, skipping sync", "version", version)
		contentPath = filepath.Join(params.TargetPath, arenaDirName, "versions", version)
		if headErr := UpdateHEAD(workspacePath, version); headErr != nil {
			return "", "", fmt.Errorf("failed to update HEAD: %w", headErr)
		}
		return contentPath, version, nil
	}

	if err := storeVersion(params.Artifact.Path, versionDir); err != nil {
		return "", "", err
	}

	// Update HEAD pointer atomically
	if err := UpdateHEAD(workspacePath, version); err != nil {
		return "", "", fmt.Errorf("failed to update HEAD: %w", err)
	}

	// Garbage collect old versions
	if err := GCOldVersions(workspacePath, s.MaxVersionsPerSource); err != nil {
		// Log but don't fail on GC errors
		log.Error(err, "Failed to garbage collect old versions")
	}

	log.Info("Successfully synced content to filesystem",
		"version", version,
		"path", versionDir,
	)

	contentPath = filepath.Join(params.TargetPath, arenaDirName, "versions", version)
	return contentPath, version, nil
}

// ensureWorkspacePVC ensures the workspace PVC exists if a StorageManager is configured.
func (s *FilesystemSyncer) ensureWorkspacePVC(ctx context.Context, workspaceName string) error {
	if s.StorageManager == nil {
		return nil
	}
	log := logf.FromContext(ctx).WithValues("workspace", workspaceName)
	if _, err := s.StorageManager.EnsureWorkspacePVC(ctx, workspaceName); err != nil {
		log.Error(err, "failed to ensure workspace PVC exists")
		return fmt.Errorf("failed to ensure workspace PVC: %w", err)
	}
	log.V(1).Info("workspace PVC ensured")
	return nil
}

// calculateVersion computes a short content-addressable version string from the artifact checksum.
func calculateVersion(artifact *Artifact) (string, error) {
	contentHash := strings.TrimPrefix(artifact.Checksum, "sha256:")
	if contentHash == "" || contentHash == artifact.Checksum || len(contentHash) < 12 {
		var err error
		contentHash, err = CalculateDirectoryHash(artifact.Path)
		if err != nil {
			return "", fmt.Errorf("failed to calculate content hash: %w", err)
		}
	}
	// Short version for display (first 12 chars of SHA256)
	return contentHash[:12], nil
}

// storeVersion creates the version directory and moves/copies the artifact content into it.
func storeVersion(artifactPath, versionDir string) error {
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("failed to create version directory: %w", err)
	}

	// Try os.Rename first (atomic, same filesystem), fallback to copy
	if err := os.Rename(artifactPath, versionDir); err != nil {
		// Rename failed (likely cross-filesystem), copy instead
		_ = os.RemoveAll(versionDir)
		if mkErr := os.MkdirAll(versionDir, 0755); mkErr != nil {
			return fmt.Errorf("failed to create version directory: %w", mkErr)
		}
		if cpErr := CopyDirectory(artifactPath, versionDir); cpErr != nil {
			_ = os.RemoveAll(versionDir)
			return fmt.Errorf("failed to copy content to version directory: %w", cpErr)
		}
	}
	return nil
}

// UpdateHEAD atomically updates the HEAD pointer to the given version.
func UpdateHEAD(workspacePath, version string) error {
	arenaDir := filepath.Join(workspacePath, arenaDirName)
	if err := os.MkdirAll(arenaDir, 0755); err != nil {
		return err
	}

	headPath := filepath.Join(arenaDir, "HEAD")
	tempPath := headPath + ".tmp"

	// Write to temp file first
	if err := os.WriteFile(tempPath, []byte(version), 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tempPath, headPath)
}

// GCOldVersions removes old versions exceeding maxVersions.
// If maxVersions is <= 0, defaults to 10.
func GCOldVersions(workspacePath string, maxVersions int) error {
	versionsDir := filepath.Join(workspacePath, arenaDirName, "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if maxVersions <= 0 {
		maxVersions = 10 // Default
	}

	if len(entries) <= maxVersions {
		return nil
	}

	versions := collectVersionInfos(entries)

	// Sort by mod time (oldest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].modTime.Before(versions[j].modTime)
	})

	// Remove oldest versions
	for i := 0; i < len(versions)-maxVersions; i++ {
		versionPath := filepath.Join(versionsDir, versions[i].name)
		if err := os.RemoveAll(versionPath); err != nil {
			return fmt.Errorf("failed to remove old version %s: %w", versions[i].name, err)
		}
	}

	return nil
}

// versionInfo holds directory name and modification time for GC sorting.
type versionInfo struct {
	name    string
	modTime time.Time
}

// collectVersionInfos reads directory entries and returns versionInfo for directories only.
func collectVersionInfos(entries []os.DirEntry) []versionInfo {
	versions := make([]versionInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		versions = append(versions, versionInfo{
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}
	return versions
}

// copyDirectory / copyFileWithMode live in dir.go (exported as CopyDirectory,
// CopyDirectoryExcluding, and the internal copyFileWithMode helper).
