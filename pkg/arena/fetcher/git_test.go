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
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitFetcher(t *testing.T) {
	tests := []struct {
		name   string
		config GitFetcherConfig
	}{
		{
			name: "basic config",
			config: GitFetcherConfig{
				URL: "https://github.com/example/repo",
				Ref: GitRef{Branch: "main"},
			},
		},
		{
			name: "with credentials",
			config: GitFetcherConfig{
				URL: "https://github.com/example/repo",
				Ref: GitRef{Tag: "v1.0.0"},
				Credentials: &GitCredentials{
					Username: "user",
					Password: "token",
				},
			},
		},
		{
			name: "with custom timeout",
			config: GitFetcherConfig{
				URL: "https://github.com/example/repo",
				Ref: GitRef{Commit: "abc123"},
				Options: Options{
					Timeout: 120 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewGitFetcher(tt.config)
			assert.NotNil(t, fetcher)
			assert.Equal(t, "git", fetcher.Type())
		})
	}
}

func TestGitFetcher_Type(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "https://github.com/example/repo",
	})
	assert.Equal(t, "git", fetcher.Type())
}

func TestFormatRevision(t *testing.T) {
	tests := []struct {
		name      string
		ref       GitRef
		commitSHA string
		expected  string
	}{
		{
			name:      "branch ref",
			ref:       GitRef{Branch: "main"},
			commitSHA: "abc123def456789",
			expected:  "main@sha1:abc123def456",
		},
		{
			name:      "tag ref",
			ref:       GitRef{Tag: "v1.0.0"},
			commitSHA: "def456abc123789",
			expected:  "v1.0.0@sha1:def456abc123",
		},
		{
			name:      "commit ref",
			ref:       GitRef{Commit: "abc123def456"},
			commitSHA: "abc123def456789",
			expected:  "sha1:abc123def456",
		},
		{
			name:      "short commit",
			ref:       GitRef{},
			commitSHA: "abc123",
			expected:  "sha1:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRevision(tt.ref, tt.commitSHA)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitFetcher_LocalRepo(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "pack.json")
	testContent := `{"name": "test-bundle", "version": "1.0.0"}`
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add("pack.json")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher for local repo
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Branch: "master"}, // git init creates 'master' by default
	})

	ctx := context.Background()

	// Test LatestRevision
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, commit.String(), revision)

	// Test Fetch
	artifact, err := fetcher.Fetch(ctx, revision)
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.Contains(t, artifact.Revision, "master@sha1:")
	assert.True(t, strings.HasPrefix(artifact.Checksum, "sha256:"))
	assert.Greater(t, artifact.Size, int64(0))

	// Verify tarball contents
	verifyTarballContents(t, artifact.Path, []string{"pack.json"})
}

func TestGitFetcher_Subdirectory(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a subdirectory with test files
	subDir := filepath.Join(tmpDir, "prompts")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "pack.json")
	err = os.WriteFile(testFile, []byte(`{"name": "sub-bundle"}`), 0644)
	require.NoError(t, err)

	// Create a file outside the subdirectory
	rootFile := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(rootFile, []byte("# Test"), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher for subdirectory
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:  tmpDir,
		Ref:  GitRef{Branch: "master"},
		Path: "prompts",
	})

	ctx := context.Background()

	// Test Fetch
	artifact, err := fetcher.Fetch(ctx, commit.String())
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	// Verify tarball only contains files from subdirectory
	verifyTarballContents(t, artifact.Path, []string{"pack.json"})
	verifyTarballNotContains(t, artifact.Path, []string{"README.md"})
}

func TestGitFetcher_TagRef(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "pack.json")
	testContent := `{"name": "test-bundle", "version": "1.0.0"}`
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add("pack.json")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create a tag
	_, err = repo.CreateTag("v1.0.0", commit, nil)
	require.NoError(t, err)

	// Create fetcher for tag
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Tag: "v1.0.0"},
	})

	ctx := context.Background()

	// Test LatestRevision
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, commit.String(), revision)

	// Test Fetch
	artifact, err := fetcher.Fetch(ctx, revision)
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	assert.Contains(t, artifact.Revision, "v1.0.0@sha1:")
}

func TestGitFetcher_CommitRef(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "pack.json")
	err = os.WriteFile(testFile, []byte(`{}`), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add("pack.json")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher with explicit commit ref
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Commit: commit.String()},
	})

	ctx := context.Background()

	// Test LatestRevision - should return the specified commit
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, commit.String(), revision)
}

func TestGitFetcher_HTTPSAuth(t *testing.T) {
	// Test that HTTPS auth is properly configured
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "https://github.com/example/repo",
		Ref: GitRef{Branch: "main"},
		Credentials: &GitCredentials{
			Username: "user",
			Password: "token123",
		},
	})

	// getAuth is private, so we test indirectly by checking config
	assert.NotNil(t, fetcher.config.Credentials)
	assert.Equal(t, "user", fetcher.config.Credentials.Username)
	assert.Equal(t, "token123", fetcher.config.Credentials.Password)
}

func TestGitFetcher_NoCredentials(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add("test.txt")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher with no credentials (nil)
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:         tmpDir,
		Ref:         GitRef{Branch: "master"},
		Credentials: nil,
	})

	ctx := context.Background()

	// Test LatestRevision works without credentials for local repos
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, revision)
}

func TestGitFetcher_EmptyCredentials(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add("test.txt")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher with empty credentials struct (no username/password/key)
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:         tmpDir,
		Ref:         GitRef{Branch: "master"},
		Credentials: &GitCredentials{},
	})

	ctx := context.Background()

	// Test LatestRevision works with empty credentials
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, revision)
}

func TestGitFetcher_GetAuth_NilCredentials(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:         "https://github.com/example/repo",
		Credentials: nil,
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.Nil(t, auth)
}

func TestGitFetcher_GetAuth_EmptyCredentials(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:         "https://github.com/example/repo",
		Credentials: &GitCredentials{},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.Nil(t, auth)
}

func TestGitFetcher_GetAuth_HTTPSCredentials(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "https://github.com/example/repo",
		Credentials: &GitCredentials{
			Username: "user",
			Password: "token",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_UsernameOnly(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "https://github.com/example/repo",
		Credentials: &GitCredentials{
			Username: "user",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_PasswordOnly(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "https://github.com/example/repo",
		Credentials: &GitCredentials{
			Password: "token",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_InvalidSSHKey(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "ssh://git@github.com/example/repo.git",
		Credentials: &GitCredentials{
			PrivateKey: []byte("invalid-key-data"),
		},
	})

	_, err := fetcher.getAuth()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SSH auth")
}

func TestGitFetcher_InvalidPath(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file and commit
	testFile := filepath.Join(tmpDir, "pack.json")
	err = os.WriteFile(testFile, []byte(`{}`), 0644)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher for non-existent subdirectory
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL:  tmpDir,
		Ref:  GitRef{Branch: "master"},
		Path: "nonexistent",
	})

	ctx := context.Background()

	// Test Fetch - should fail
	_, err = fetcher.Fetch(ctx, commit.String())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// verifyTarballContents checks that the tarball contains the expected files.
func verifyTarballContents(t *testing.T, tarballPath string, expectedFiles []string) {
	t.Helper()

	file, err := os.Open(tarballPath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	foundFiles := make(map[string]bool)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		foundFiles[header.Name] = true
	}

	for _, expected := range expectedFiles {
		assert.True(t, foundFiles[expected], "expected file %s not found in tarball", expected)
	}
}

// verifyTarballNotContains checks that the tarball does not contain certain files.
func verifyTarballNotContains(t *testing.T, tarballPath string, unexpectedFiles []string) {
	t.Helper()

	file, err := os.Open(tarballPath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	gzReader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)

	foundFiles := make(map[string]bool)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		foundFiles[header.Name] = true
	}

	for _, unexpected := range unexpectedFiles {
		assert.False(t, foundFiles[unexpected], "unexpected file %s found in tarball", unexpected)
	}
}

// testSSHPrivateKey is a valid ED25519 private key for testing.
// This is a throwaway key generated specifically for tests - DO NOT use in production.
const testSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCHJt9HySBGQ56zepndK2UAOkjwQrECBSLZHxLXVTIapgAAAJglsAe1JbAH
tQAAAAtzc2gtZWQyNTUxOQAAACCHJt9HySBGQ56zepndK2UAOkjwQrECBSLZHxLXVTIapg
AAAEB7+BqMTeku1LmezL5nS/c5jvTkRoECBMMVaG9CbDnN4Icm30fJIEZDnrN6md0rZQA6
SPBCsQIFItkfEtdVMhqmAAAAEHRlc3RAZXhhbXBsZS5jb20BAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`

// testKnownHosts is a sample known_hosts entry for testing.
const testKnownHosts = `github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl`

func TestGitFetcher_GetAuth_ValidSSHKey(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "ssh://git@github.com/example/repo.git",
		Credentials: &GitCredentials{
			PrivateKey: []byte(testSSHPrivateKey),
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_SSHKeyWithPassword(t *testing.T) {
	// Using an unencrypted key with an empty password should work
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "ssh://git@github.com/example/repo.git",
		Credentials: &GitCredentials{
			PrivateKey:         []byte(testSSHPrivateKey),
			PrivateKeyPassword: "",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_SSHKeyWithKnownHosts(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "ssh://git@github.com/example/repo.git",
		Credentials: &GitCredentials{
			PrivateKey: []byte(testSSHPrivateKey),
			KnownHosts: []byte(testKnownHosts),
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestGitFetcher_GetAuth_SSHKeyWithInvalidKnownHosts(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "ssh://git@github.com/example/repo.git",
		Credentials: &GitCredentials{
			PrivateKey: []byte(testSSHPrivateKey),
			KnownHosts: []byte("invalid known_hosts content"),
		},
	})

	_, err := fetcher.getAuth()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse known_hosts")
}

func TestCopyFile(t *testing.T) {
	// Create a temp source file
	srcFile, err := os.CreateTemp("", "copy-src-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(srcFile.Name()) }()

	testContent := []byte("test content for copy")
	_, err = srcFile.Write(testContent)
	require.NoError(t, err)
	require.NoError(t, srcFile.Close())

	// Create a temp destination path
	dstFile, err := os.CreateTemp("", "copy-dst-*")
	require.NoError(t, err)
	dstPath := dstFile.Name()
	require.NoError(t, dstFile.Close())
	_ = os.Remove(dstPath) // Remove so copyFile can create it
	defer func() { _ = os.Remove(dstPath) }()

	// Test copyFile
	err = copyFile(srcFile.Name(), dstPath)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, content)
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/path/file.txt", "/tmp/dest.txt")
	assert.Error(t, err)
}

func TestCopyFile_DestinationError(t *testing.T) {
	// Create a temp source file
	srcFile, err := os.CreateTemp("", "copy-src-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(srcFile.Name()) }()
	require.NoError(t, srcFile.Close())

	// Try to copy to an invalid destination
	err = copyFile(srcFile.Name(), "/nonexistent/directory/file.txt")
	assert.Error(t, err)
}

func TestAddFileToTar_WalkError(t *testing.T) {
	tarWriter := tar.NewWriter(io.Discard)
	defer func() { _ = tarWriter.Close() }()

	// Test with a walk error
	walkErr := fmt.Errorf("simulated walk error")
	err := addFileToTar(tarWriter, "/tmp", "/tmp/test", nil, walkErr)
	assert.Error(t, err)
	assert.Equal(t, walkErr, err)
}

func TestGitFetcher_MultipleFiles(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create multiple test files
	files := map[string]string{
		"file1.txt":        "content1",
		"file2.txt":        "content2",
		"subdir/file3.txt": "content3",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if dir != tmpDir {
			require.NoError(t, os.MkdirAll(dir, 0755))
		}
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Branch: "master"},
	})

	ctx := context.Background()

	// Test Fetch
	artifact, err := fetcher.Fetch(ctx, commit.String())
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	// Verify tarball contains all files
	verifyTarballContents(t, artifact.Path, []string{"file1.txt", "file2.txt", "subdir/file3.txt"})
}

func TestGitFetcher_WithSymlink(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(tmpDir, "original.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Create a symlink
	symlinkPath := filepath.Join(tmpDir, "link.txt")
	require.NoError(t, os.Symlink("original.txt", symlinkPath))

	// Add and commit
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	commit, err := worktree.Commit("Initial commit with symlink", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create fetcher
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Branch: "master"},
	})

	ctx := context.Background()

	// Test Fetch
	artifact, err := fetcher.Fetch(ctx, commit.String())
	require.NoError(t, err)
	defer func() { _ = os.Remove(artifact.Path) }()

	// Verify tarball contains both files
	verifyTarballContents(t, artifact.Path, []string{"original.txt", "link.txt"})
}

func TestAddFileToTar_SkipGitDirectory(t *testing.T) {
	tarWriter := tar.NewWriter(io.Discard)
	defer func() { _ = tarWriter.Close() }()

	// Create a mock directory info for .git
	gitDirInfo := mockDirInfo{name: ".git", isDir: true}
	err := addFileToTar(tarWriter, "/tmp/repo", "/tmp/repo/.git", gitDirInfo, nil)
	assert.Equal(t, filepath.SkipDir, err)
}

func TestAddFileToTar_SkipRootDirectory(t *testing.T) {
	tarWriter := tar.NewWriter(io.Discard)
	defer func() { _ = tarWriter.Close() }()

	// Create a mock directory info for root
	rootInfo := mockDirInfo{name: "repo", isDir: true}
	err := addFileToTar(tarWriter, "/tmp/repo", "/tmp/repo", rootInfo, nil)
	assert.NoError(t, err)
}

func TestAddFileToTar_RegularDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tar-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	tarWriter := tar.NewWriter(io.Discard)
	defer func() { _ = tarWriter.Close() }()

	// Get real directory info
	info, err := os.Stat(subDir)
	require.NoError(t, err)

	err = addFileToTar(tarWriter, tmpDir, subDir, info, nil)
	assert.NoError(t, err)
}

// mockDirInfo implements os.FileInfo for testing
type mockDirInfo struct {
	name  string
	isDir bool
}

func (m mockDirInfo) Name() string       { return m.name }
func (m mockDirInfo) Size() int64        { return 0 }
func (m mockDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (m mockDirInfo) ModTime() time.Time { return time.Now() }
func (m mockDirInfo) IsDir() bool        { return m.isDir }
func (m mockDirInfo) Sys() any           { return nil }

func TestGitFetcher_LatestRevision_InvalidURL(t *testing.T) {
	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: "/nonexistent/repo",
		Ref: GitRef{Branch: "main"},
	})

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clone repository")
}

func TestGitFetcher_Fetch_InvalidRevision(t *testing.T) {
	// Create a temporary directory for the test repo
	tmpDir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Initialize a local git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	// Create test file and commit
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644))
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(".")
	require.NoError(t, err)
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	fetcher := NewGitFetcher(GitFetcherConfig{
		URL: tmpDir,
		Ref: GitRef{Branch: "master"},
	})

	ctx := context.Background()

	// Try to fetch with an invalid commit hash
	_, err = fetcher.Fetch(ctx, "0000000000000000000000000000000000000000")
	assert.Error(t, err)
}

func TestCopyFileToTar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "copy-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a test file
	testContent := "test content"
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte(testContent), 0644))

	// Create a tar writer to a buffer
	var buf strings.Builder
	tarWriter := tar.NewWriter(&buf)

	// Write header first
	header := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(testContent)),
	}
	require.NoError(t, tarWriter.WriteHeader(header))

	err = copyFileToTar(tarWriter, testFile)
	assert.NoError(t, err)

	_ = tarWriter.Close()
}

func TestCopyFileToTar_FileNotFound(t *testing.T) {
	var buf strings.Builder
	tarWriter := tar.NewWriter(&buf)

	err := copyFileToTar(tarWriter, "/nonexistent/file.txt")
	assert.Error(t, err)

	_ = tarWriter.Close()
}
