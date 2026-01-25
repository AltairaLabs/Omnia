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
