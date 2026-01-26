/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyDirectory(t *testing.T) {
	// Create source directory with files
	srcDir, err := os.MkdirTemp("", "copy-src-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create files in source
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644))

	// Create destination
	dstDir, err := os.MkdirTemp("", "copy-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Copy
	err = CopyDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// Verify
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(content2))
}

func TestCopyDirectoryExcluding(t *testing.T) {
	tests := []struct {
		name     string
		exclude  []string
		expected map[string]bool
	}{
		{
			name:    "exclude single file",
			exclude: []string{"excluded.txt"},
			expected: map[string]bool{
				"keep.txt": true,
			},
		},
		{
			name:    "exclude directory",
			exclude: []string{".git"},
			expected: map[string]bool{
				"keep.txt":         true,
				"subdir/nested.md": true,
			},
		},
		{
			name:    "exclude pattern",
			exclude: []string{"*.log"},
			expected: map[string]bool{
				"keep.txt": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create source directory
			srcDir, err := os.MkdirTemp("", "exclude-src-*")
			require.NoError(t, err)
			defer func() { _ = os.RemoveAll(srcDir) }()

			// Create test files based on test case
			require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644))

			switch tt.name {
			case "exclude single file":
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, "excluded.txt"), []byte("excluded"), 0644))
			case "exclude directory":
				require.NoError(t, os.MkdirAll(filepath.Join(srcDir, ".git", "objects"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, ".git", "HEAD"), []byte("ref"), 0644))
				require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "nested.md"), []byte("nested"), 0644))
			case "exclude pattern":
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, "debug.log"), []byte("log"), 0644))
			}

			// Create destination
			dstDir, err := os.MkdirTemp("", "exclude-dst-*")
			require.NoError(t, err)
			defer func() { _ = os.RemoveAll(dstDir) }()

			// Copy with exclusions
			err = CopyDirectoryExcluding(srcDir, dstDir, tt.exclude)
			require.NoError(t, err)

			// Verify expected files exist
			for relPath := range tt.expected {
				_, err := os.Stat(filepath.Join(dstDir, relPath))
				assert.NoError(t, err, "expected file %s to exist", relPath)
			}

			// Verify excluded files don't exist
			switch tt.name {
			case "exclude single file":
				_, err := os.Stat(filepath.Join(dstDir, "excluded.txt"))
				assert.True(t, os.IsNotExist(err))
			case "exclude directory":
				_, err := os.Stat(filepath.Join(dstDir, ".git"))
				assert.True(t, os.IsNotExist(err))
			case "exclude pattern":
				_, err := os.Stat(filepath.Join(dstDir, "debug.log"))
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}

func TestCopyDirectoryExcluding_WithSymlink(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "symlink-src-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create a file and a symlink to it
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "target.txt"), []byte("target"), 0644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(srcDir, "link.txt")))

	dstDir, err := os.MkdirTemp("", "symlink-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = CopyDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// Verify symlink was copied
	linkInfo, err := os.Lstat(filepath.Join(dstDir, "link.txt"))
	require.NoError(t, err)
	assert.True(t, linkInfo.Mode()&os.ModeSymlink != 0)

	// Verify symlink target
	target, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestCalculateDirectorySize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "size-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create files with known sizes
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("12345"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "file2.txt"), []byte("1234567890"), 0644))

	size, err := CalculateDirectorySize(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, int64(15), size) // 5 + 10 bytes
}

func TestCalculateDirectorySize_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "empty-size-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	size, err := CalculateDirectorySize(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size)
}

func TestCopyFileWithMode(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "mode-src-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	srcFile := filepath.Join(srcDir, "source.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0644))

	dstDir, err := os.MkdirTemp("", "mode-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	dstFile := filepath.Join(dstDir, "dest.txt")
	err = copyFileWithMode(srcFile, dstFile, 0755)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	// Verify mode
	info, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestCopyFileWithMode_NonexistentSource(t *testing.T) {
	dstDir, err := os.MkdirTemp("", "mode-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = copyFileWithMode("/nonexistent/file.txt", filepath.Join(dstDir, "dest.txt"), 0644)
	assert.Error(t, err)
}

func TestCopyFileWithMode_CreatesParentDir(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "mode-src-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	srcFile := filepath.Join(srcDir, "source.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0644))

	dstDir, err := os.MkdirTemp("", "mode-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Destination is in a subdirectory that doesn't exist
	dstFile := filepath.Join(dstDir, "nested", "subdir", "dest.txt")
	err = copyFileWithMode(srcFile, dstFile, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestCopyDirectory_NonexistentSource(t *testing.T) {
	dstDir, err := os.MkdirTemp("", "copy-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = CopyDirectory("/nonexistent/directory", dstDir)
	assert.Error(t, err)
}

func TestCopyDirectoryExcluding_NonexistentSource(t *testing.T) {
	dstDir, err := os.MkdirTemp("", "exclude-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = CopyDirectoryExcluding("/nonexistent/directory", dstDir, []string{".git"})
	assert.Error(t, err)
}

func TestCalculateDirectorySize_NonexistentDir(t *testing.T) {
	_, err := CalculateDirectorySize("/nonexistent/directory")
	assert.Error(t, err)
}

func TestCopyDirectoryExcluding_ExcludeFile(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "exclude-file-src-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create files - one to keep, one to exclude
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "excluded.txt"), []byte("excluded"), 0644))

	dstDir, err := os.MkdirTemp("", "exclude-file-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Exclude by exact file name match
	err = CopyDirectoryExcluding(srcDir, dstDir, []string{"excluded.txt"})
	require.NoError(t, err)

	// Verify keep.txt exists
	_, err = os.Stat(filepath.Join(dstDir, "keep.txt"))
	assert.NoError(t, err)

	// Verify excluded.txt doesn't exist
	_, err = os.Stat(filepath.Join(dstDir, "excluded.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestCalculateDirectorySize_WithSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "size-symlink-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a regular file
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("12345"), 0644))
	// Create a symlink (should not be counted in size)
	require.NoError(t, os.Symlink("file.txt", filepath.Join(tmpDir, "link.txt")))

	size, err := CalculateDirectorySize(tmpDir)
	require.NoError(t, err)
	// Only the regular file should be counted
	assert.Equal(t, int64(5), size)
}

func TestCopyDirectoryExcluding_MultiplePatterns(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "multi-exclude-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create various files
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.log"), []byte("log1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b.log"), []byte("log2"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, ".git", "config"), []byte("git"), 0644))

	dstDir, err := os.MkdirTemp("", "multi-exclude-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Exclude both *.log and .git
	err = CopyDirectoryExcluding(srcDir, dstDir, []string{"*.log", ".git"})
	require.NoError(t, err)

	// Verify keep.txt exists
	_, err = os.Stat(filepath.Join(dstDir, "keep.txt"))
	assert.NoError(t, err)

	// Verify .log files don't exist
	_, err = os.Stat(filepath.Join(dstDir, "a.log"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dstDir, "b.log"))
	assert.True(t, os.IsNotExist(err))

	// Verify .git doesn't exist
	_, err = os.Stat(filepath.Join(dstDir, ".git"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyDirectoryExcluding_PreservesMode(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "mode-preserve-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create an executable file
	execFile := filepath.Join(srcDir, "script.sh")
	require.NoError(t, os.WriteFile(execFile, []byte("#!/bin/bash"), 0755))

	dstDir, err := os.MkdirTemp("", "mode-preserve-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = CopyDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// Verify the mode is preserved
	info, err := os.Stat(filepath.Join(dstDir, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestCopyDirectoryExcluding_ExcludeByBaseName(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "basename-exclude-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create files with different names
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
	// This file should be excluded by its basename, not path
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "exclude-me.txt"), []byte("excluded"), 0644))

	dstDir, err := os.MkdirTemp("", "basename-exclude-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Exclude by file name
	err = CopyDirectoryExcluding(srcDir, dstDir, []string{"exclude-me.txt"})
	require.NoError(t, err)

	// Verify keep.txt exists
	_, err = os.Stat(filepath.Join(dstDir, "keep.txt"))
	assert.NoError(t, err)

	// Verify subdir exists
	_, err = os.Stat(filepath.Join(dstDir, "subdir"))
	assert.NoError(t, err)

	// Verify excluded file doesn't exist
	_, err = os.Stat(filepath.Join(dstDir, "subdir", "exclude-me.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyDirectoryExcluding_ExcludeSubdirectory(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "subdir-exclude-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create directory structure
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "node_modules", "pkg"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "node_modules", "pkg", "file.js"), []byte("js"), 0644))

	dstDir, err := os.MkdirTemp("", "subdir-exclude-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	// Exclude node_modules directory
	err = CopyDirectoryExcluding(srcDir, dstDir, []string{"node_modules"})
	require.NoError(t, err)

	// Verify keep.txt exists
	_, err = os.Stat(filepath.Join(dstDir, "keep.txt"))
	assert.NoError(t, err)

	// Verify node_modules doesn't exist
	_, err = os.Stat(filepath.Join(dstDir, "node_modules"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyDirectory_WithNestedSymlink(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "nested-symlink-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(srcDir) }()

	// Create nested structure with symlink
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "target.txt"), []byte("target"), 0644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(srcDir, "subdir", "link.txt")))

	dstDir, err := os.MkdirTemp("", "nested-symlink-dst-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dstDir) }()

	err = CopyDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// Verify symlink was copied
	linkPath := filepath.Join(dstDir, "subdir", "link.txt")
	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)
}
