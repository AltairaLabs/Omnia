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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// OCICredentials contains authentication credentials for OCI registries.
type OCICredentials struct {
	// Username for basic authentication.
	Username string

	// Password for basic authentication.
	Password string

	// DockerConfig is the raw docker config JSON for authentication.
	// This is the content of ~/.docker/config.json or a Kubernetes
	// docker-registry secret.
	DockerConfig []byte
}

// OCIFetcherConfig contains configuration for the OCI fetcher.
type OCIFetcherConfig struct {
	// URL is the OCI artifact URL (e.g., "oci://ghcr.io/org/repo:tag").
	URL string

	// Insecure allows connections to registries without TLS.
	Insecure bool

	// Credentials contains authentication credentials.
	Credentials *OCICredentials

	// Options contains common fetcher options.
	Options Options
}

// remoteClient abstracts OCI registry operations for testing.
type remoteClient interface {
	Head(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error)
	Image(ref name.Reference, opts ...remote.Option) (v1.Image, error)
}

// defaultRemoteClient uses the real go-containerregistry remote package.
type defaultRemoteClient struct{}

func (d *defaultRemoteClient) Head(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
	return remote.Head(ref, opts...)
}

func (d *defaultRemoteClient) Image(ref name.Reference, opts ...remote.Option) (v1.Image, error) {
	return remote.Image(ref, opts...)
}

// OCIFetcher implements the Fetcher interface for OCI registries.
type OCIFetcher struct {
	config OCIFetcherConfig
	client remoteClient // For testing; defaults to real client
}

// NewOCIFetcher creates a new OCI fetcher with the given configuration.
func NewOCIFetcher(config OCIFetcherConfig) *OCIFetcher {
	if config.Options.Timeout == 0 {
		config.Options = DefaultOptions()
	}
	return &OCIFetcher{
		config: config,
		client: &defaultRemoteClient{},
	}
}

// Type returns the source type.
func (f *OCIFetcher) Type() string {
	return "oci"
}

// LatestRevision returns the digest of the OCI artifact at the configured reference.
func (f *OCIFetcher) LatestRevision(ctx context.Context) (string, error) {
	ref, err := f.parseReference()
	if err != nil {
		return "", err
	}

	opts, err := f.getRemoteOptions(ctx)
	if err != nil {
		return "", err
	}

	desc, err := f.client.Head(ref, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to get image manifest: %w", err)
	}

	return desc.Digest.String(), nil
}

// Fetch downloads the OCI artifact and returns it as a tarball.
func (f *OCIFetcher) Fetch(ctx context.Context, revision string) (*Artifact, error) {
	ref, err := f.parseReference()
	if err != nil {
		return nil, err
	}

	// If revision is a digest, use it directly
	if strings.HasPrefix(revision, "sha256:") {
		digestRef, err := name.NewDigest(ref.Context().String() + "@" + revision)
		if err != nil {
			return nil, fmt.Errorf("failed to parse digest reference: %w", err)
		}
		ref = digestRef
	}

	opts, err := f.getRemoteOptions(ctx)
	if err != nil {
		return nil, err
	}

	img, err := f.client.Image(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Create temporary file for the tarball
	tmpFile, err := os.CreateTemp(f.config.Options.WorkDir, "oci-artifact-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write image as tarball
	if err := tarball.Write(ref, img, tmpFile); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to write tarball: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Calculate checksum
	checksum, size, err := f.calculateChecksum(tmpFile.Name())
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Get the digest for the revision string
	digest, err := img.Digest()
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to get image digest: %w", err)
	}

	// Format revision
	revisionStr := f.formatRevision(ref, digest.String())

	return &Artifact{
		Path:         tmpFile.Name(),
		Revision:     revisionStr,
		Checksum:     checksum,
		Size:         size,
		LastModified: time.Now(), // OCI doesn't provide creation time easily
	}, nil
}

// parseReference parses the OCI URL into a name.Reference.
func (f *OCIFetcher) parseReference() (name.Reference, error) {
	// Strip oci:// prefix if present
	url := f.config.URL
	url = strings.TrimPrefix(url, "oci://")

	var opts []name.Option
	if f.config.Insecure {
		opts = append(opts, name.Insecure)
	}

	ref, err := name.ParseReference(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference %q: %w", url, err)
	}

	return ref, nil
}

// getRemoteOptions returns the remote options for OCI operations.
func (f *OCIFetcher) getRemoteOptions(ctx context.Context) ([]remote.Option, error) {
	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	auth, err := f.getAuth()
	if err != nil {
		return nil, err
	}
	if auth != nil {
		opts = append(opts, remote.WithAuth(auth))
	}

	return opts, nil
}

// getAuth returns the appropriate authenticator based on credentials.
func (f *OCIFetcher) getAuth() (authn.Authenticator, error) {
	if f.config.Credentials == nil {
		return authn.Anonymous, nil
	}

	// Basic authentication
	if f.config.Credentials.Username != "" || f.config.Credentials.Password != "" {
		return &authn.Basic{
			Username: f.config.Credentials.Username,
			Password: f.config.Credentials.Password,
		}, nil
	}

	// Docker config authentication
	if len(f.config.Credentials.DockerConfig) > 0 {
		// Parse the docker config and extract credentials for the registry
		// For now, return anonymous if docker config is provided but not parsed
		// A full implementation would parse the JSON and match the registry
		return authn.Anonymous, nil
	}

	return authn.Anonymous, nil
}

// calculateChecksum calculates the SHA256 checksum and size of a file.
func (f *OCIFetcher) calculateChecksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), size, nil
}

// formatRevision formats the revision with tag/digest information.
func (f *OCIFetcher) formatRevision(ref name.Reference, digest string) string {
	// If the reference is a tag, include it
	if tag, ok := ref.(name.Tag); ok {
		return fmt.Sprintf("%s@%s", tag.TagStr(), digest)
	}
	// If it's already a digest reference, just return the digest
	return digest
}

// Ensure OCIFetcher implements Fetcher interface.
var _ Fetcher = (*OCIFetcher)(nil)
