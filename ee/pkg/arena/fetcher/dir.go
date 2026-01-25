/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher

import (
	"io"
	"os"
	"path/filepath"
)

// CopyDirectory recursively copies a directory from src to dst.
// All file permissions are preserved.
func CopyDirectory(src, dst string) error {
	return CopyDirectoryExcluding(src, dst, nil)
}

// CopyDirectoryExcluding recursively copies a directory from src to dst,
// excluding any paths matching the provided patterns.
// Patterns are matched against the relative path from src.
func CopyDirectoryExcluding(src, dst string, exclude []string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Check exclusions
		for _, pattern := range exclude {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Also check if the name matches (for directory exclusions like ".git")
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
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

		// Copy regular file
		return copyFileWithMode(path, targetPath, info.Mode())
	})
}

// CalculateDirectorySize calculates the total size of all files in a directory.
func CalculateDirectorySize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// copyFileWithMode copies a file preserving its mode.
func copyFileWithMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		_ = destFile.Close()
		return err
	}

	if err := destFile.Sync(); err != nil {
		_ = destFile.Close()
		return err
	}

	return destFile.Close()
}
