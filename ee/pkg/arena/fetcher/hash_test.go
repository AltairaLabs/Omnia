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

func TestCalculateDirectoryHash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0644))

	hash, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex = 64 chars
}

func TestCalculateDirectoryHash_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash-empty-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	hash, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestCalculateDirectoryHash_DifferentContent(t *testing.T) {
	dir1, err := os.MkdirTemp("", "hash-test1-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir1) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("content1"), 0644))

	dir2, err := os.MkdirTemp("", "hash-test2-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir2) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("content2"), 0644))

	hash1, err := CalculateDirectoryHash(dir1)
	require.NoError(t, err)

	hash2, err := CalculateDirectoryHash(dir2)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)
}

func TestCalculateDirectoryHash_SameContent(t *testing.T) {
	dir1, err := os.MkdirTemp("", "hash-same1-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir1) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("same content"), 0644))

	dir2, err := os.MkdirTemp("", "hash-same2-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir2) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("same content"), 0644))

	hash1, err := CalculateDirectoryHash(dir1)
	require.NoError(t, err)

	hash2, err := CalculateDirectoryHash(dir2)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

func TestCalculateDirectoryHash_WithSubdirectories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash-subdir-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir1", "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir1", "nested", "file.txt"), []byte("nested"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "subdir2"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("root"), 0644))

	hash, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64)
}

func TestCalculateDirectoryHash_DifferentModes(t *testing.T) {
	dir1, err := os.MkdirTemp("", "hash-mode1-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir1) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("content"), 0644))

	dir2, err := os.MkdirTemp("", "hash-mode2-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir2) }()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("content"), 0755))

	hash1, err := CalculateDirectoryHash(dir1)
	require.NoError(t, err)

	hash2, err := CalculateDirectoryHash(dir2)
	require.NoError(t, err)

	// Different modes should produce different hashes
	assert.NotEqual(t, hash1, hash2)
}

func TestCalculateDirectoryHash_NonexistentDir(t *testing.T) {
	_, err := CalculateDirectoryHash("/nonexistent/directory")
	assert.Error(t, err)
}

func TestCalculateDirectoryHash_Deterministic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash-determ-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("aaa"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("bbb"), 0644))

	// Calculate hash multiple times
	hash1, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)

	hash2, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)

	hash3, err := CalculateDirectoryHash(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, hash2, hash3)
}
