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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
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

	opts := f.getRemoteOptions(ctx)

	desc, err := f.client.Head(ref, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to get image manifest: %w", err)
	}

	return desc.Digest.String(), nil
}

// Fetch downloads the OCI artifact and extracts it to a directory.
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

	opts := f.getRemoteOptions(ctx)

	img, err := f.client.Image(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Create temporary file for the OCI tarball (required by go-containerregistry API)
	tmpFile, err := os.CreateTemp(f.config.Options.WorkDir, "oci-artifact-*.tar")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpTarPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpTarPath) }() // Clean up temp tarball

	// Write image as tarball (go-containerregistry API constraint)
	if err := tarball.Write(ref, img, tmpFile); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to write tarball: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Extract tarball to output directory
	outputDir, err := os.MkdirTemp(f.config.Options.WorkDir, "artifact-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := f.extractOCITarToDir(tmpTarPath, outputDir); err != nil {
		_ = os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to extract OCI tarball: %w", err)
	}

	// Calculate checksum of output directory
	checksum, err := CalculateDirectoryHash(outputDir)
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Calculate total size
	size, err := CalculateDirectorySize(outputDir)
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to calculate size: %w", err)
	}

	// Get the digest for the revision string
	digest, err := img.Digest()
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to get image digest: %w", err)
	}

	// Format revision
	revisionStr := f.formatRevision(ref, digest.String())

	return &Artifact{
		Path:         outputDir,
		Revision:     revisionStr,
		Checksum:     "sha256:" + checksum,
		Size:         size,
		LastModified: time.Now(), // OCI doesn't provide creation time easily
	}, nil
}

// extractOCITarToDir extracts an OCI image tarball to the destination directory.
// OCI tarballs have a specific structure with manifest.json and layer blobs.
func (f *OCIFetcher) extractOCITarToDir(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Security: use SecureJoin to prevent directory traversal attacks
		target, err := securejoin.SecureJoin(destDir, header.Name)
		if err != nil {
			return fmt.Errorf("invalid tar path %q: %w", header.Name, err)
		}

		// Skip macOS resource fork files (AppleDouble format)
		if strings.HasPrefix(filepath.Base(header.Name), "._") {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := f.extractRegularFile(tr, target, header); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := f.extractSymlink(header, destDir); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractRegularFile extracts a regular file from the tar archive.
func (f *OCIFetcher) extractRegularFile(tr *tar.Reader, target string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	outFile, err := os.Create(target)
	if err != nil {
		return err
	}

	if _, err := io.CopyN(outFile, tr, header.Size); err != nil && err != io.EOF {
		_ = outFile.Close()
		return err
	}

	if err := outFile.Close(); err != nil {
		return err
	}

	return os.Chmod(target, os.FileMode(header.Mode))
}

// extractSymlink extracts a symlink, validating it doesn't escape the destination.
// The symlink path is validated using SecureJoin, and the link destination is
// manually validated to ensure it resolves within destDir.
func (f *OCIFetcher) extractSymlink(header *tar.Header, destDir string) error {
	// Securely resolve the symlink path within destDir
	target, err := securejoin.SecureJoin(destDir, header.Name)
	if err != nil {
		return fmt.Errorf("invalid symlink path %q: %w", header.Name, err)
	}

	// Compute where the symlink would resolve to when followed.
	// We use filepath.Join (not SecureJoin) because we need to see
	// where the OS would actually resolve the symlink, not a sanitized version.
	linkTarget := header.Linkname
	linkDir := filepath.Dir(target)
	resolvedPath := filepath.Clean(filepath.Join(linkDir, linkTarget))

	// Validate the resolved path is within destDir
	cleanDestDir := filepath.Clean(destDir)
	if !strings.HasPrefix(resolvedPath, cleanDestDir+string(filepath.Separator)) &&
		resolvedPath != cleanDestDir {
		return fmt.Errorf("symlink escape attempt: %s -> %s resolves outside destDir",
			header.Name, linkTarget)
	}

	return os.Symlink(linkTarget, target)
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
func (f *OCIFetcher) getRemoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	auth := f.getAuth()
	if auth != nil {
		opts = append(opts, remote.WithAuth(auth))
	}

	return opts
}

// getAuth returns the appropriate authenticator based on credentials.
func (f *OCIFetcher) getAuth() authn.Authenticator {
	if f.config.Credentials == nil {
		return authn.Anonymous
	}

	// Basic authentication
	if f.config.Credentials.Username != "" || f.config.Credentials.Password != "" {
		return &authn.Basic{
			Username: f.config.Credentials.Username,
			Password: f.config.Credentials.Password,
		}
	}

	// Docker config authentication
	if len(f.config.Credentials.DockerConfig) > 0 {
		// Parse the docker config and extract credentials for the registry
		// For now, return anonymous if docker config is provided but not parsed
		// A full implementation would parse the JSON and match the registry
		return authn.Anonymous
	}

	return authn.Anonymous
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
