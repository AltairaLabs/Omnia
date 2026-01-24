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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEmptyDigest is the SHA256 digest of an empty blob, used in tests.
const testEmptyDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestNewOCIFetcher(t *testing.T) {
	tests := []struct {
		name   string
		config OCIFetcherConfig
	}{
		{
			name: "basic config",
			config: OCIFetcherConfig{
				URL: "oci://ghcr.io/example/repo:latest",
			},
		},
		{
			name: "with credentials",
			config: OCIFetcherConfig{
				URL: "oci://ghcr.io/example/repo:v1.0.0",
				Credentials: &OCICredentials{
					Username: "user",
					Password: "token",
				},
			},
		},
		{
			name: "insecure registry",
			config: OCIFetcherConfig{
				URL:      "oci://localhost:5000/repo:latest",
				Insecure: true,
			},
		},
		{
			name: "with custom timeout",
			config: OCIFetcherConfig{
				URL: "oci://gcr.io/project/image:tag",
				Options: Options{
					Timeout: 120 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewOCIFetcher(tt.config)
			assert.NotNil(t, fetcher)
			assert.Equal(t, "oci", fetcher.Type())
		})
	}
}

func TestOCIFetcher_Type(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})
	assert.Equal(t, "oci", fetcher.Type())
}

func TestOCIFetcher_ParseReference(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		insecure    bool
		expectError bool
		expectTag   string
	}{
		{
			name:      "standard tag reference",
			url:       "oci://ghcr.io/org/repo:v1.0.0",
			expectTag: "v1.0.0",
		},
		{
			name:      "latest tag",
			url:       "oci://docker.io/library/alpine:latest",
			expectTag: "latest",
		},
		{
			name:      "without oci prefix",
			url:       "ghcr.io/org/repo:tag",
			expectTag: "tag",
		},
		{
			name:     "insecure localhost",
			url:      "oci://localhost:5000/repo:latest",
			insecure: true,
		},
		{
			name:        "invalid reference",
			url:         "oci://",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewOCIFetcher(OCIFetcherConfig{
				URL:      tt.url,
				Insecure: tt.insecure,
			})

			ref, err := fetcher.parseReference()
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, ref)
		})
	}
}

func TestOCIFetcher_GetAuth_NilCredentials(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL:         "oci://ghcr.io/example/repo:latest",
		Credentials: nil,
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth) // Returns Anonymous authenticator
}

func TestOCIFetcher_GetAuth_BasicCredentials(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			Username: "user",
			Password: "token",
		},
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_UsernameOnly(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			Username: "user",
		},
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_PasswordOnly(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			Password: "token",
		},
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_DockerConfig(t *testing.T) {
	dockerConfig := []byte(`{"auths":{"ghcr.io":{"auth":"dXNlcjp0b2tlbg=="}}}`)

	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			DockerConfig: dockerConfig,
		},
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_EmptyCredentials(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL:         "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{},
	})

	auth := fetcher.getAuth()
	assert.NotNil(t, auth) // Returns Anonymous authenticator
}

func TestOCIFetcher_FormatRevision(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:v1.0.0",
	})

	ref, err := fetcher.parseReference()
	require.NoError(t, err)

	revision := fetcher.formatRevision(ref, "sha256:abc123def456")
	assert.Contains(t, revision, "v1.0.0")
	assert.Contains(t, revision, "sha256:abc123def456")
}

func TestOCIFetcher_FormatRevision_DigestRef(t *testing.T) {
	// Use a valid 64-character hex digest
	digest := testEmptyDigest
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo@" + digest,
	})

	ref, err := fetcher.parseReference()
	require.NoError(t, err)

	revision := fetcher.formatRevision(ref, digest)
	// For digest references, should just return the digest
	assert.Equal(t, digest, revision)
}

func TestOCIFetcher_GetRemoteOptions(t *testing.T) {
	tests := []struct {
		name   string
		config OCIFetcherConfig
	}{
		{
			name: "no credentials",
			config: OCIFetcherConfig{
				URL: "oci://ghcr.io/example/repo:latest",
			},
		},
		{
			name: "with basic auth",
			config: OCIFetcherConfig{
				URL: "oci://ghcr.io/example/repo:latest",
				Credentials: &OCICredentials{
					Username: "user",
					Password: "token",
				},
			},
		},
		{
			name: "with docker config",
			config: OCIFetcherConfig{
				URL: "oci://ghcr.io/example/repo:latest",
				Credentials: &OCICredentials{
					DockerConfig: []byte(`{"auths":{}}`),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewOCIFetcher(tt.config)
			opts := fetcher.getRemoteOptions(context.Background())
			assert.NotEmpty(t, opts)
		})
	}
}

func TestOCIFetcher_LatestRevision_InvalidReference(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://",
	})

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse OCI reference")
}

func TestOCIFetcher_Fetch_InvalidReference(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://",
	})

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "sha256:abc123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse OCI reference")
}

func TestOCIFetcher_Fetch_WithDigestRevision(t *testing.T) {
	// Test that a valid digest revision is parsed correctly
	// This won't connect to registry but tests the digest parsing path
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	ctx := context.Background()
	// Use a properly formatted sha256 digest
	_, err := fetcher.Fetch(ctx, testEmptyDigest)
	// This will fail at the remote.Image call but we're testing the digest parsing path
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull image")
}

func TestOCIFetcher_LatestRevision_NetworkError(t *testing.T) {
	// Test with a valid reference but unreachable registry
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://localhost:59999/nonexistent/repo:latest",
	})

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get image manifest")
}

func TestOCIFetcher_Fetch_NetworkError(t *testing.T) {
	// Test with a valid reference but unreachable registry
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://localhost:59999/nonexistent/repo:latest",
	})

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "v1.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull image")
}

func TestOCIFetcher_Fetch_InvalidDigestFormat(t *testing.T) {
	// Test with a valid URL but malformed digest that will fail name.NewDigest
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	ctx := context.Background()
	// Use sha256: prefix but with invalid content (not valid hex)
	_, err := fetcher.Fetch(ctx, "sha256:invalid!digest@content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse digest reference")
}

func TestOCIFetcher_LatestRevision_WithInsecureRegistry(t *testing.T) {
	// Test insecure flag is properly passed through
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL:      "oci://localhost:59999/repo:latest",
		Insecure: true,
	})

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	// Will fail at network level, but tests the insecure path
	assert.Error(t, err)
}

func TestOCIFetcher_GetRemoteOptions_WithContext(t *testing.T) {
	// Test that context is properly included in options
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := fetcher.getRemoteOptions(ctx)
	assert.NotEmpty(t, opts)
}

// mockRemoteClient implements remoteClient for testing.
type mockRemoteClient struct {
	headFunc  func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error)
	imageFunc func(ref name.Reference, opts ...remote.Option) (v1.Image, error)
}

func (m *mockRemoteClient) Head(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
	if m.headFunc != nil {
		return m.headFunc(ref, opts...)
	}
	return nil, nil
}

func (m *mockRemoteClient) Image(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
	if m.imageFunc != nil {
		return m.imageFunc(ref, opts...)
	}
	return nil, nil
}

func TestOCIFetcher_LatestRevision_WithMock(t *testing.T) {
	expectedDigest := testEmptyDigest

	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	// Replace with mock client
	fetcher.client = &mockRemoteClient{
		headFunc: func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
			hash, _ := v1.NewHash(expectedDigest)
			return &v1.Descriptor{
				Digest:    hash,
				MediaType: types.OCIManifestSchema1,
				Size:      1234,
			}, nil
		},
	}

	ctx := context.Background()
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, expectedDigest, revision)
}

func TestOCIFetcher_Fetch_WithMock(t *testing.T) {
	// Create a minimal valid image for testing
	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)

	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:v1.0.0",
	})

	// Replace with mock client
	fetcher.client = &mockRemoteClient{
		imageFunc: func(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
			return img, nil
		},
	}

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "v1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.NotEmpty(t, artifact.Revision)
	assert.True(t, strings.HasPrefix(artifact.Checksum, "sha256:"))
	assert.Greater(t, artifact.Size, int64(0))
}

func TestOCIFetcher_Fetch_WithDigestRevision_Mock(t *testing.T) {
	// Create a minimal valid image for testing
	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)

	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	// Replace with mock client
	fetcher.client = &mockRemoteClient{
		imageFunc: func(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
			return img, nil
		},
	}

	ctx := context.Background()
	// Use a valid digest revision
	revision := testEmptyDigest
	artifact, err := fetcher.Fetch(ctx, revision)
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.NotEmpty(t, artifact.Revision)
}

// Helper function to create a test tarball
func createTestTar(t *testing.T, entries map[string]testTarEntry) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.tar")
	require.NoError(t, err)
	defer func() { _ = tmpFile.Close() }()

	tw := tar.NewWriter(tmpFile)
	defer func() { _ = tw.Close() }()

	for entryName, entry := range entries {
		hdr := &tar.Header{
			Name:     entryName,
			Mode:     entry.Mode,
			Size:     int64(len(entry.Content)),
			Typeflag: entry.Typeflag,
			Linkname: entry.Linkname,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if entry.Content != "" {
			_, err := tw.Write([]byte(entry.Content))
			require.NoError(t, err)
		}
	}

	return tmpFile.Name()
}

type testTarEntry struct {
	Content  string
	Mode     int64
	Typeflag byte
	Linkname string
}

func TestOCIFetcher_ExtractOCITarToDir_RegularFile(t *testing.T) {
	tarPath := createTestTar(t, map[string]testTarEntry{
		"file.txt": {Content: "hello", Mode: 0644, Typeflag: tar.TypeReg},
	})
	defer func() { _ = os.Remove(tarPath) }()

	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir(tarPath, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestOCIFetcher_ExtractOCITarToDir_Directory(t *testing.T) {
	tarPath := createTestTar(t, map[string]testTarEntry{
		"subdir/": {Mode: 0755, Typeflag: tar.TypeDir},
	})
	defer func() { _ = os.Remove(tarPath) }()

	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir(tarPath, destDir)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(destDir, "subdir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestOCIFetcher_ExtractOCITarToDir_Symlink(t *testing.T) {
	tarPath := createTestTar(t, map[string]testTarEntry{
		"target.txt": {Content: "target", Mode: 0644, Typeflag: tar.TypeReg},
		"link.txt":   {Mode: 0777, Typeflag: tar.TypeSymlink, Linkname: "target.txt"},
	})
	defer func() { _ = os.Remove(tarPath) }()

	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir(tarPath, destDir)
	require.NoError(t, err)

	// Verify symlink exists
	linkPath := filepath.Join(destDir, "link.txt")
	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0)

	// Verify symlink target
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestOCIFetcher_ExtractOCITarToDir_SkipsAppleDouble(t *testing.T) {
	tarPath := createTestTar(t, map[string]testTarEntry{
		"file.txt":   {Content: "real", Mode: 0644, Typeflag: tar.TypeReg},
		"._file.txt": {Content: "resource fork", Mode: 0644, Typeflag: tar.TypeReg},
	})
	defer func() { _ = os.Remove(tarPath) }()

	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir(tarPath, destDir)
	require.NoError(t, err)

	// file.txt should exist
	_, err = os.Stat(filepath.Join(destDir, "file.txt"))
	assert.NoError(t, err)

	// ._file.txt should not exist (skipped)
	_, err = os.Stat(filepath.Join(destDir, "._file.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestOCIFetcher_ExtractOCITarToDir_InvalidTarPath(t *testing.T) {
	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir("/nonexistent/file.tar", destDir)
	assert.Error(t, err)
}

func TestOCIFetcher_ExtractOCITarToDir_DirectoryTraversal(t *testing.T) {
	// Test that directory traversal attempts are safely handled by SecureJoin
	// The file should be extracted safely within destDir, not outside
	tarPath := createTestTar(t, map[string]testTarEntry{
		"../escape.txt": {Content: "escape", Mode: 0644, Typeflag: tar.TypeReg},
	})
	defer func() { _ = os.Remove(tarPath) }()

	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	// Create parent dir to verify escape didn't happen
	parentDir := filepath.Dir(destDir)

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})
	err = fetcher.extractOCITarToDir(tarPath, destDir)
	// SecureJoin prevents escape by safely resolving paths within destDir
	assert.NoError(t, err)

	// Verify file was NOT created outside destDir
	escapedPath := filepath.Join(parentDir, "escape.txt")
	_, err = os.Stat(escapedPath)
	assert.True(t, os.IsNotExist(err), "file should not have escaped to parent directory")

	// Verify file was created safely inside destDir
	safePath := filepath.Join(destDir, "escape.txt")
	_, err = os.Stat(safePath)
	assert.NoError(t, err, "file should be created safely inside destDir")
}

func TestOCIFetcher_ExtractSymlink_EscapeAttempt(t *testing.T) {
	destDir, err := os.MkdirTemp("", "extract-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(destDir) }()

	fetcher := NewOCIFetcher(OCIFetcherConfig{URL: "oci://test:latest"})

	header := &tar.Header{
		Name:     "link.txt",
		Linkname: "../../../etc/passwd",
	}
	target := filepath.Join(destDir, "link.txt")

	err = fetcher.extractSymlink(header, target, destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "symlink escape attempt")
}
