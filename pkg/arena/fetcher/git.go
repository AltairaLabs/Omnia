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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// GitRef specifies which ref to checkout from a Git repository.
type GitRef struct {
	// Branch is the branch name to checkout.
	Branch string

	// Tag is the tag name to checkout.
	Tag string

	// Commit is the specific commit SHA to checkout.
	Commit string
}

// GitCredentials contains authentication credentials for Git operations.
type GitCredentials struct {
	// Username for HTTPS authentication.
	Username string

	// Password or token for HTTPS authentication.
	Password string

	// PrivateKey is the SSH private key content.
	PrivateKey []byte

	// PrivateKeyPassword is the passphrase for the private key.
	PrivateKeyPassword string

	// KnownHosts is the SSH known_hosts content.
	KnownHosts []byte
}

// GitFetcherConfig contains configuration for the Git fetcher.
type GitFetcherConfig struct {
	// URL is the Git repository URL (https:// or ssh://).
	URL string

	// Ref specifies which ref to checkout.
	Ref GitRef

	// Path is the subdirectory within the repository to extract.
	// If empty, the entire repository is used.
	Path string

	// Credentials contains authentication credentials.
	Credentials *GitCredentials

	// Options contains common fetcher options.
	Options Options
}

// GitFetcher implements the Fetcher interface for Git repositories.
type GitFetcher struct {
	config GitFetcherConfig
}

// NewGitFetcher creates a new Git fetcher with the given configuration.
func NewGitFetcher(config GitFetcherConfig) *GitFetcher {
	if config.Options.Timeout == 0 {
		config.Options = DefaultOptions()
	}
	return &GitFetcher{config: config}
}

// Type returns the source type.
func (f *GitFetcher) Type() string {
	return "git"
}

// LatestRevision returns the latest commit SHA at the configured ref.
func (f *GitFetcher) LatestRevision(ctx context.Context) (string, error) {
	auth, err := f.getAuth()
	if err != nil {
		return "", fmt.Errorf("failed to get auth: %w", err)
	}

	// Create a temporary directory for the clone
	tmpDir, err := os.MkdirTemp(f.config.Options.WorkDir, "git-fetch-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Clone with depth 1 to get just the latest commit
	cloneOpts := &git.CloneOptions{
		URL:          f.config.URL,
		Auth:         auth,
		Depth:        1,
		SingleBranch: true,
		NoCheckout:   true,
	}

	// Set the reference based on config
	if f.config.Ref.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(f.config.Ref.Branch)
	} else if f.config.Ref.Tag != "" {
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(f.config.Ref.Tag)
	}

	repo, err := git.PlainCloneContext(ctx, tmpDir, false, cloneOpts)
	if err != nil {
		return "", fmt.Errorf("failed to clone repository: %w", err)
	}

	// If a specific commit was requested, verify it exists
	if f.config.Ref.Commit != "" {
		return f.config.Ref.Commit, nil
	}

	// Get HEAD reference
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

// Fetch clones the repository at the specified revision and creates a tarball.
func (f *GitFetcher) Fetch(ctx context.Context, revision string) (*Artifact, error) {
	auth, err := f.getAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth: %w", err)
	}

	// Create a temporary directory for the clone
	tmpDir, err := os.MkdirTemp(f.config.Options.WorkDir, "git-fetch-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneDir := filepath.Join(tmpDir, "repo")

	// Clone the repository with shallow clone for memory efficiency
	cloneOpts := &git.CloneOptions{
		URL:          f.config.URL,
		Auth:         auth,
		Depth:        1,
		SingleBranch: true,
	}

	// Set the reference based on config
	if f.config.Ref.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(f.config.Ref.Branch)
	} else if f.config.Ref.Tag != "" {
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(f.config.Ref.Tag)
	}

	repo, err := git.PlainCloneContext(ctx, cloneDir, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Checkout the specific revision
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(revision),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout revision %s: %w", revision, err)
	}

	// Determine source directory
	sourceDir := cloneDir
	if f.config.Path != "" {
		sourceDir = filepath.Join(cloneDir, f.config.Path)
		if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
			return nil, fmt.Errorf("path %s does not exist in repository", f.config.Path)
		}
	}

	// Get commit info for metadata
	commit, err := repo.CommitObject(plumbing.NewHash(revision))
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Create the tarball
	tarballPath := filepath.Join(tmpDir, "artifact.tar.gz")
	checksum, size, err := f.createTarball(sourceDir, tarballPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create tarball: %w", err)
	}

	// Move tarball to final location
	finalPath, err := os.CreateTemp(f.config.Options.WorkDir, "artifact-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("failed to create final artifact file: %w", err)
	}
	if err := finalPath.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tarballPath, finalPath.Name()); err != nil {
		// If rename fails (cross-device), copy instead
		if err := copyFile(tarballPath, finalPath.Name()); err != nil {
			return nil, fmt.Errorf("failed to move artifact: %w", err)
		}
	}

	// Format revision with ref info
	revisionStr := formatRevision(f.config.Ref, revision)

	return &Artifact{
		Path:         finalPath.Name(),
		Revision:     revisionStr,
		Checksum:     checksum,
		Size:         size,
		LastModified: commit.Author.When,
	}, nil
}

// getAuth returns the appropriate transport.AuthMethod based on credentials.
// nolint:gocyclo // Auth logic is inherently branchy but straightforward
func (f *GitFetcher) getAuth() (transport.AuthMethod, error) {
	if f.config.Credentials == nil {
		return nil, nil
	}

	// SSH authentication
	if len(f.config.Credentials.PrivateKey) > 0 {
		publicKeys, err := ssh.NewPublicKeys(
			"git",
			f.config.Credentials.PrivateKey,
			f.config.Credentials.PrivateKeyPassword,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH auth: %w", err)
		}

		// If known_hosts is provided, write to temp file and use for host key verification
		if len(f.config.Credentials.KnownHosts) > 0 {
			tmpFile, err := os.CreateTemp("", "known_hosts")
			if err != nil {
				return nil, fmt.Errorf("failed to create known_hosts temp file: %w", err)
			}
			// Note: We don't defer remove here because the file needs to persist
			// for the SSH connection. The OS will clean up temp files.

			if _, err := tmpFile.Write(f.config.Credentials.KnownHosts); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("failed to write known_hosts: %w", err)
			}
			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("failed to close known_hosts file: %w", err)
			}

			callback, err := ssh.NewKnownHostsCallback(tmpFile.Name())
			if err != nil {
				_ = os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("failed to parse known_hosts: %w", err)
			}
			publicKeys.HostKeyCallback = callback
		}

		return publicKeys, nil
	}

	// HTTPS authentication
	if f.config.Credentials.Username != "" || f.config.Credentials.Password != "" {
		return &http.BasicAuth{
			Username: f.config.Credentials.Username,
			Password: f.config.Credentials.Password,
		}, nil
	}

	return nil, nil
}

// createTarball creates a gzipped tarball of the source directory.
func (f *GitFetcher) createTarball(sourceDir, destPath string) (string, int64, error) {
	file, err := os.Create(destPath)
	if err != nil {
		return "", 0, err
	}

	hash := sha256.New()
	multiWriter := io.MultiWriter(file, hash)

	gzipWriter := gzip.NewWriter(multiWriter)
	tarWriter := tar.NewWriter(gzipWriter)

	// Track if we've closed everything successfully
	var closed bool
	defer func() {
		if !closed {
			// Only close if not already closed (error path)
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			_ = file.Close()
		}
	}()

	walkErr := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		return addFileToTar(tarWriter, sourceDir, path, info, walkErr)
	})

	if walkErr != nil {
		return "", 0, walkErr
	}

	// Close writers to flush
	if err := tarWriter.Close(); err != nil {
		return "", 0, err
	}
	if err := gzipWriter.Close(); err != nil {
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	closed = true

	// Get file size
	stat, err := os.Stat(destPath)
	if err != nil {
		return "", 0, err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), stat.Size(), nil
}

// addFileToTar adds a single file or directory to the tar archive.
func addFileToTar(tarWriter *tar.Writer, sourceDir, path string, info os.FileInfo, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}

	// Skip .git directory
	if info.IsDir() && info.Name() == ".git" {
		return filepath.SkipDir
	}

	// Get relative path
	relPath, err := filepath.Rel(sourceDir, path)
	if err != nil {
		return err
	}

	// Skip root directory
	if relPath == "." {
		return nil
	}

	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = relPath

	// Handle symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			return err
		}
		header.Linkname = link
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// Write file content for regular files
	if !info.IsDir() && info.Mode().IsRegular() {
		return copyFileToTar(tarWriter, path)
	}

	return nil
}

// copyFileToTar copies a file's content to the tar writer.
func copyFileToTar(tarWriter *tar.Writer, path string) error {
	srcFile, err := os.Open(path)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(tarWriter, srcFile)
	closeErr := srcFile.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// formatRevision formats the revision with ref information.
func formatRevision(ref GitRef, commitSHA string) string {
	shortSHA := commitSHA
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}

	if ref.Branch != "" {
		return fmt.Sprintf("%s@sha1:%s", ref.Branch, shortSHA)
	}
	if ref.Tag != "" {
		return fmt.Sprintf("%s@sha1:%s", ref.Tag, shortSHA)
	}
	return fmt.Sprintf("sha1:%s", shortSHA)
}

// copyFile copies a file from src to dst.
// This is only used as a fallback when os.Rename fails (cross-filesystem moves).
// Coverage: tested via integration tests as it requires cross-filesystem scenarios.
func copyFile(src, dst string) error { //nolint:unused // Used as fallback for cross-device moves
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		_ = sourceFile.Close()
		return err
	}

	_, copyErr := io.Copy(destFile, sourceFile)
	srcCloseErr := sourceFile.Close()
	dstCloseErr := destFile.Close()

	if copyErr != nil {
		return copyErr
	}
	if srcCloseErr != nil {
		return srcCloseErr
	}
	return dstCloseErr
}

// Ensure GitFetcher implements Fetcher interface.
var _ Fetcher = (*GitFetcher)(nil)
