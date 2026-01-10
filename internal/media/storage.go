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

// Package media provides media storage interfaces and implementations for the facade.
package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Storage reference scheme and format constants.
const (
	// StorageRefScheme is the URI scheme for storage references.
	StorageRefScheme = "omnia"
	// StorageRefPrefix is the full prefix for storage references.
	StorageRefPrefix = StorageRefScheme + "://"
)

// Common errors returned by storage implementations.
var (
	// ErrMediaNotFound is returned when media does not exist.
	ErrMediaNotFound = errors.New("media not found")
	// ErrMediaExpired is returned when media has expired.
	ErrMediaExpired = errors.New("media expired")
	// ErrInvalidStorageRef is returned when a storage reference is malformed.
	ErrInvalidStorageRef = errors.New("invalid storage reference")
	// ErrUploadFailed is returned when an upload operation fails.
	ErrUploadFailed = errors.New("upload failed")
	// ErrFileTooLarge is returned when a file exceeds size limits.
	ErrFileTooLarge = errors.New("file too large")
	// ErrUnsupportedMIMEType is returned when a MIME type is not allowed.
	ErrUnsupportedMIMEType = errors.New("unsupported MIME type")
)

// UploadRequest contains parameters for requesting an upload URL.
type UploadRequest struct {
	// SessionID is the session this upload belongs to.
	SessionID string
	// Filename is the original filename.
	Filename string
	// MIMEType is the expected content type.
	MIMEType string
	// SizeBytes is the expected file size in bytes.
	SizeBytes int64
	// TTL is how long the media should be retained (zero means use default).
	TTL time.Duration
}

// UploadCredentials contains the information needed to upload media.
type UploadCredentials struct {
	// UploadID is a unique identifier for this upload.
	UploadID string
	// URL is the presigned URL to upload to.
	URL string
	// StorageRef is the reference to use when accessing the uploaded media.
	StorageRef string
	// ExpiresAt is when the upload URL expires.
	ExpiresAt time.Time
	// Headers contains any required headers for the upload request.
	Headers map[string]string
	// Method is the HTTP method to use (PUT or POST).
	Method string
}

// MediaInfo contains metadata about stored media.
type MediaInfo struct {
	// StorageRef is the storage reference.
	StorageRef string
	// Filename is the original filename.
	Filename string
	// MIMEType is the content type.
	MIMEType string
	// SizeBytes is the file size.
	SizeBytes int64
	// CreatedAt is when the media was uploaded.
	CreatedAt time.Time
	// ExpiresAt is when the media will be deleted (zero means no expiry).
	ExpiresAt time.Time
}

// IsExpired returns true if the media has expired.
func (m *MediaInfo) IsExpired() bool {
	if m.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(m.ExpiresAt)
}

// Storage defines the interface for media storage backends.
type Storage interface {
	// GetUploadURL generates a presigned URL for uploading media.
	// The returned UploadCredentials contains the URL and any required headers.
	GetUploadURL(ctx context.Context, req UploadRequest) (*UploadCredentials, error)

	// GetDownloadURL generates a URL for downloading media.
	// The URL may be presigned with an expiration time.
	GetDownloadURL(ctx context.Context, storageRef string) (string, error)

	// GetMediaInfo retrieves metadata about stored media.
	// Returns ErrMediaNotFound if the media does not exist.
	// Returns ErrMediaExpired if the media has expired.
	GetMediaInfo(ctx context.Context, storageRef string) (*MediaInfo, error)

	// Delete removes media from storage.
	// Returns ErrMediaNotFound if the media does not exist.
	Delete(ctx context.Context, storageRef string) error

	// Close releases any resources held by the storage.
	Close() error
}

// DirectUploadStorage is implemented by storage backends that support
// direct client uploads (S3, GCS). These backends return presigned URLs
// that clients upload to directly, bypassing the facade.
type DirectUploadStorage interface {
	Storage
	// ConfirmUpload verifies that an upload completed and finalizes metadata.
	// This should be called after the client uploads directly to the presigned URL.
	// Returns ErrUploadFailed if the upload ID is invalid or expired.
	// Returns ErrMediaNotFound if the object doesn't exist in storage.
	ConfirmUpload(ctx context.Context, uploadID string) (*MediaInfo, error)
}

// ProxyUploadStorage is implemented by storage backends where uploads
// are proxied through the facade (e.g., LocalStorage).
type ProxyUploadStorage interface {
	Storage
	// GetUploadPath returns the filesystem path for writing an upload.
	// This is used by the handler to write uploaded content.
	GetUploadPath(uploadID string) (string, error)
	// CompleteUpload marks an upload as complete and stores metadata.
	// This should be called after the upload content has been written.
	CompleteUpload(ctx context.Context, uploadID string, actualSize int64) error
	// GetMediaPath returns the filesystem path for reading media.
	// This is used by the handler to serve downloads.
	GetMediaPath(storageRef string) (string, error)
}

// StorageRef represents a parsed storage reference.
type StorageRef struct {
	// SessionID is the session the media belongs to.
	SessionID string
	// MediaID is the unique identifier for the media.
	MediaID string
}

// String returns the storage reference as a URI string.
func (r StorageRef) String() string {
	return fmt.Sprintf("%ssessions/%s/media/%s", StorageRefPrefix, r.SessionID, r.MediaID)
}

// ParseStorageRef parses a storage reference URI.
// The expected format is: omnia://sessions/{session-id}/media/{media-id}
func ParseStorageRef(ref string) (*StorageRef, error) {
	if !strings.HasPrefix(ref, StorageRefPrefix) {
		return nil, fmt.Errorf("%w: missing scheme prefix", ErrInvalidStorageRef)
	}

	path := strings.TrimPrefix(ref, StorageRefPrefix)
	parts := strings.Split(path, "/")

	// Expected: sessions/{session-id}/media/{media-id}
	if len(parts) != 4 || parts[0] != "sessions" || parts[2] != "media" {
		return nil, fmt.Errorf("%w: invalid path format", ErrInvalidStorageRef)
	}

	sessionID := parts[1]
	mediaID := parts[3]

	if sessionID == "" {
		return nil, fmt.Errorf("%w: empty session ID", ErrInvalidStorageRef)
	}
	if mediaID == "" {
		return nil, fmt.Errorf("%w: empty media ID", ErrInvalidStorageRef)
	}

	return &StorageRef{
		SessionID: sessionID,
		MediaID:   mediaID,
	}, nil
}
