/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// FilesystemSyncer manages content-addressable filesystem sync for arena sources.
// It provides the shared pipeline: hash calculation, version storage, HEAD pointer update,
// and garbage collection of old versions.
type FilesystemSyncer struct {
	// WorkspaceContentPath is the base path for workspace content volumes.
	WorkspaceContentPath string

	// MaxVersionsPerSource is the maximum number of versions to retain per source.
	// Default is 10 if not set.
	MaxVersionsPerSource int

	// StorageManager handles lazy workspace PVC creation.
	StorageManager *workspace.StorageManager
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
	Artifact *fetcher.Artifact
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
	versionDir := filepath.Join(workspacePath, ".arena", "versions", version)
	if _, statErr := os.Stat(versionDir); statErr == nil {
		log.V(1).Info("Version already exists, skipping sync", "version", version)
		contentPath = filepath.Join(params.TargetPath, ".arena", "versions", version)
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

	contentPath = filepath.Join(params.TargetPath, ".arena", "versions", version)
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
func calculateVersion(artifact *fetcher.Artifact) (string, error) {
	contentHash := strings.TrimPrefix(artifact.Checksum, "sha256:")
	if contentHash == "" || contentHash == artifact.Checksum || len(contentHash) < 12 {
		var err error
		contentHash, err = fetcher.CalculateDirectoryHash(artifact.Path)
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
		if cpErr := copyDirectory(artifactPath, versionDir); cpErr != nil {
			_ = os.RemoveAll(versionDir)
			return fmt.Errorf("failed to copy content to version directory: %w", cpErr)
		}
	}
	return nil
}

// UpdateHEAD atomically updates the HEAD pointer to the given version.
func UpdateHEAD(workspacePath, version string) error {
	arenaDir := filepath.Join(workspacePath, ".arena")
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
	versionsDir := filepath.Join(workspacePath, ".arena", "versions")

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

// copyDirectory recursively copies a directory.
func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, targetPath)
		}

		// Copy file
		return copyFileWithMode(path, targetPath, info.Mode())
	})
}

// copyFileWithMode copies a file preserving its mode.
func copyFileWithMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close source file %s: %v\n", src, err)
		}
	}()

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		if closeErr := destFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close dest file %s after copy error: %v\n", dst, closeErr)
		}
		return err
	}

	if err := destFile.Sync(); err != nil {
		if closeErr := destFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close dest file %s after sync error: %v\n", dst, closeErr)
		}
		return err
	}

	return destFile.Close()
}
