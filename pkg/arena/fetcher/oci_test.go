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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
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

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_UsernameOnly(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			Username: "user",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_PasswordOnly(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{
			Password: "token",
		},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
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

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestOCIFetcher_GetAuth_EmptyCredentials(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL:         "oci://ghcr.io/example/repo:latest",
		Credentials: &OCICredentials{},
	})

	auth, err := fetcher.getAuth()
	require.NoError(t, err)
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
	digest := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
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
		name        string
		config      OCIFetcherConfig
		expectError bool
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
			opts, err := fetcher.getRemoteOptions(context.Background())

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, opts)
		})
	}
}

func TestOCIFetcher_CalculateChecksum(t *testing.T) {
	// Create a temporary file with known content
	tmpDir, err := os.MkdirTemp("", "oci-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	testContent := "test content for checksum"
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte(testContent), 0644))

	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	checksum, size, err := fetcher.calculateChecksum(testFile)
	require.NoError(t, err)
	assert.Equal(t, int64(len(testContent)), size)
	assert.True(t, strings.HasPrefix(checksum, "sha256:"))
	assert.Len(t, checksum, 7+64) // "sha256:" + 64 hex chars
}

func TestOCIFetcher_CalculateChecksum_FileNotFound(t *testing.T) {
	fetcher := NewOCIFetcher(OCIFetcherConfig{
		URL: "oci://ghcr.io/example/repo:latest",
	})

	_, _, err := fetcher.calculateChecksum("/nonexistent/file.txt")
	assert.Error(t, err)
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
	_, err := fetcher.Fetch(ctx, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
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

	opts, err := fetcher.getRemoteOptions(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}
