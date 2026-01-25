/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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

// Fetch clones the repository at the specified revision and returns the directory.
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

	// Clone and checkout the repository
	repo, cloneDir, err := f.cloneAndCheckout(ctx, tmpDir, revision, auth)
	if err != nil {
		return nil, err
	}

	// Determine and validate source directory
	sourceDir, err := f.getSourceDirectory(cloneDir)
	if err != nil {
		return nil, err
	}

	// Get commit info for metadata
	commit, err := repo.CommitObject(plumbing.NewHash(revision))
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Create output directory and copy contents (excluding .git)
	return f.createArtifact(sourceDir, revision, commit)
}

// cloneAndCheckout clones the repository and checks out the specified revision.
func (f *GitFetcher) cloneAndCheckout(
	ctx context.Context, tmpDir, revision string, auth transport.AuthMethod,
) (*git.Repository, string, error) {
	cloneDir := filepath.Join(tmpDir, "repo")

	cloneOpts := f.buildCloneOptions(auth)

	repo, err := git.PlainCloneContext(ctx, cloneDir, false, cloneOpts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to clone repository: %w", err)
	}

	// Checkout the specific revision
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get worktree: %w", err)
	}

	if err := worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(revision)}); err != nil {
		return nil, "", fmt.Errorf("failed to checkout revision %s: %w", revision, err)
	}

	return repo, cloneDir, nil
}

// buildCloneOptions creates git clone options based on config.
func (f *GitFetcher) buildCloneOptions(auth transport.AuthMethod) *git.CloneOptions {
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

	return cloneOpts
}

// getSourceDirectory returns the source directory for the artifact.
func (f *GitFetcher) getSourceDirectory(cloneDir string) (string, error) {
	if f.config.Path == "" {
		return cloneDir, nil
	}

	sourceDir := filepath.Join(cloneDir, f.config.Path)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return "", fmt.Errorf("path %s does not exist in repository", f.config.Path)
	}
	return sourceDir, nil
}

// createArtifact copies the source directory to a new location and returns the artifact.
func (f *GitFetcher) createArtifact(sourceDir, revision string, commit *object.Commit) (*Artifact, error) {
	// Create output directory
	outputDir, err := os.MkdirTemp(f.config.Options.WorkDir, "artifact-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Copy contents excluding .git directory
	if err := CopyDirectoryExcluding(sourceDir, outputDir, []string{".git"}); err != nil {
		_ = os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to copy directory: %w", err)
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

	// Format revision with ref info
	revisionStr := formatRevision(f.config.Ref, revision)

	return &Artifact{
		Path:         outputDir,
		Revision:     revisionStr,
		Checksum:     "sha256:" + checksum,
		Size:         size,
		LastModified: commit.Author.When,
	}, nil
}

// getAuth returns the appropriate transport.AuthMethod based on credentials.
func (f *GitFetcher) getAuth() (transport.AuthMethod, error) {
	if f.config.Credentials == nil {
		return nil, nil
	}

	// SSH authentication
	if len(f.config.Credentials.PrivateKey) > 0 {
		return f.getSSHAuth()
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

// getSSHAuth creates SSH authentication from credentials.
func (f *GitFetcher) getSSHAuth() (transport.AuthMethod, error) {
	publicKeys, err := ssh.NewPublicKeys(
		"git",
		f.config.Credentials.PrivateKey,
		f.config.Credentials.PrivateKeyPassword,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH auth: %w", err)
	}

	// If known_hosts is provided, configure host key verification
	if len(f.config.Credentials.KnownHosts) > 0 {
		if err := f.configureKnownHosts(publicKeys); err != nil {
			return nil, err
		}
	}

	return publicKeys, nil
}

// configureKnownHosts sets up host key verification from known_hosts data.
func (f *GitFetcher) configureKnownHosts(publicKeys *ssh.PublicKeys) error {
	tmpFile, err := os.CreateTemp("", "known_hosts")
	if err != nil {
		return fmt.Errorf("failed to create known_hosts temp file: %w", err)
	}
	// Note: We don't defer remove here because the file needs to persist
	// for the SSH connection. The OS will clean up temp files.

	if _, err := tmpFile.Write(f.config.Credentials.KnownHosts); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to write known_hosts: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to close known_hosts file: %w", err)
	}

	callback, err := ssh.NewKnownHostsCallback(tmpFile.Name())
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("failed to parse known_hosts: %w", err)
	}
	publicKeys.HostKeyCallback = callback

	return nil
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

// Ensure GitFetcher implements Fetcher interface.
var _ Fetcher = (*GitFetcher)(nil)
