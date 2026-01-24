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
	"context"
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
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.Contains(t, artifact.Revision, "master@sha1:")
	assert.True(t, strings.HasPrefix(artifact.Checksum, "sha256:"))
	assert.Greater(t, artifact.Size, int64(0))

	// Verify directory contents
	verifyGitDirectoryContents(t, artifact.Path, []string{"pack.json"})
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
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Verify directory only contains files from subdirectory
	verifyGitDirectoryContents(t, artifact.Path, []string{"pack.json"})
	verifyGitDirectoryNotContains(t, artifact.Path, []string{"README.md"})
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
	defer func() { _ = os.RemoveAll(artifact.Path) }()

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
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Verify directory contains all files
	verifyGitDirectoryContents(t, artifact.Path, []string{"file1.txt", "file2.txt", "subdir/file3.txt"})
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
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Verify directory contains both files
	verifyGitDirectoryContents(t, artifact.Path, []string{"original.txt", "link.txt"})
}

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

// verifyGitDirectoryContents checks that the directory contains the expected files.
func verifyGitDirectoryContents(t *testing.T, dirPath string, expectedFiles []string) {
	t.Helper()

	for _, expected := range expectedFiles {
		filePath := filepath.Join(dirPath, expected)
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "expected file %s not found in directory", expected)
	}
}

// verifyGitDirectoryNotContains checks that the directory does not contain certain files.
func verifyGitDirectoryNotContains(t *testing.T, dirPath string, unexpectedFiles []string) {
	t.Helper()

	for _, unexpected := range unexpectedFiles {
		filePath := filepath.Join(dirPath, unexpected)
		_, err := os.Stat(filePath)
		assert.True(t, os.IsNotExist(err), "unexpected file %s found in directory", unexpected)
	}
}
