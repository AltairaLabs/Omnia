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

package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalStorageConfig contains configuration for local filesystem storage.
type LocalStorageConfig struct {
	// BasePath is the root directory for storing media files.
	BasePath string
	// BaseURL is the base URL for generating download URLs.
	// For local development, this is typically the facade's HTTP address.
	BaseURL string
	// DefaultTTL is the default time-to-live for media (zero means no expiry).
	DefaultTTL time.Duration
	// UploadURLTTL is how long upload URLs remain valid.
	UploadURLTTL time.Duration
	// MaxFileSize is the maximum allowed file size in bytes (0 means no limit).
	MaxFileSize int64
}

// DefaultLocalStorageConfig returns a configuration with sensible defaults.
func DefaultLocalStorageConfig(basePath, baseURL string) LocalStorageConfig {
	return LocalStorageConfig{
		BasePath:     basePath,
		BaseURL:      baseURL,
		DefaultTTL:   24 * time.Hour,
		UploadURLTTL: 15 * time.Minute,
		MaxFileSize:  100 * 1024 * 1024, // 100MB
	}
}

// LocalStorage implements Storage using the local filesystem.
// This is suitable for development and single-instance deployments.
type LocalStorage struct {
	config LocalStorageConfig
	mu     sync.RWMutex
	// pendingUploads tracks uploads that have been initiated but not completed.
	pendingUploads map[string]*pendingUpload
}

// pendingUpload tracks an initiated upload.
type pendingUpload struct {
	StorageRef string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	ExpiresAt  time.Time
	MediaTTL   time.Duration
}

// mediaMetadata is stored alongside media files.
type mediaMetadata struct {
	Filename  string    `json:"filename"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// NewLocalStorage creates a new local filesystem storage.
func NewLocalStorage(config LocalStorageConfig) (*LocalStorage, error) {
	// Ensure base path exists
	if err := os.MkdirAll(config.BasePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{
		config:         config,
		pendingUploads: make(map[string]*pendingUpload),
	}, nil
}

// GetUploadURL generates a presigned URL for uploading media.
// For local storage, this returns a URL to the facade's upload endpoint.
func (s *LocalStorage) GetUploadURL(ctx context.Context, req UploadRequest) (*UploadCredentials, error) {
	// Validate request
	if req.SessionID == "" {
		return nil, fmt.Errorf("%w: session ID is required", ErrInvalidStorageRef)
	}
	if req.MIMEType == "" {
		return nil, fmt.Errorf("%w: MIME type is required", ErrUnsupportedMIMEType)
	}
	if s.config.MaxFileSize > 0 && req.SizeBytes > s.config.MaxFileSize {
		return nil, fmt.Errorf("%w: max size is %d bytes", ErrFileTooLarge, s.config.MaxFileSize)
	}

	// Generate unique media ID
	mediaID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate media ID: %w", err)
	}

	// Build storage reference
	ref := StorageRef{
		SessionID: req.SessionID,
		MediaID:   mediaID,
	}

	// Calculate expiration
	uploadExpiry := time.Now().Add(s.config.UploadURLTTL)

	// Determine media TTL
	mediaTTL := req.TTL
	if mediaTTL == 0 {
		mediaTTL = s.config.DefaultTTL
	}

	// Track pending upload
	s.mu.Lock()
	s.pendingUploads[mediaID] = &pendingUpload{
		StorageRef: ref.String(),
		Filename:   req.Filename,
		MIMEType:   req.MIMEType,
		SizeBytes:  req.SizeBytes,
		ExpiresAt:  uploadExpiry,
		MediaTTL:   mediaTTL,
	}
	s.mu.Unlock()

	// Generate upload URL
	// For local storage, uploads go to: {baseURL}/media/upload/{uploadID}
	uploadURL := fmt.Sprintf("%s/media/upload/%s", s.config.BaseURL, mediaID)

	return &UploadCredentials{
		UploadID:   mediaID,
		URL:        uploadURL,
		StorageRef: ref.String(),
		ExpiresAt:  uploadExpiry,
		Method:     "PUT",
		Headers: map[string]string{
			"Content-Type": req.MIMEType,
		},
	}, nil
}

// CompleteUpload marks an upload as complete and stores the metadata.
// This should be called by the upload handler after receiving the file.
func (s *LocalStorage) CompleteUpload(ctx context.Context, uploadID string, actualSize int64) error {
	s.mu.Lock()
	pending, ok := s.pendingUploads[uploadID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: upload not found or expired", ErrUploadFailed)
	}
	delete(s.pendingUploads, uploadID)
	s.mu.Unlock()

	// Check if upload URL has expired
	if time.Now().After(pending.ExpiresAt) {
		return fmt.Errorf("%w: upload URL expired", ErrUploadFailed)
	}

	// Parse storage ref to get paths
	ref, err := ParseStorageRef(pending.StorageRef)
	if err != nil {
		return err
	}

	// Calculate media expiration
	var expiresAt time.Time
	if pending.MediaTTL > 0 {
		expiresAt = time.Now().Add(pending.MediaTTL)
	}

	// Save metadata
	meta := mediaMetadata{
		Filename:  pending.Filename,
		MIMEType:  pending.MIMEType,
		SizeBytes: actualSize,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	metaPath := s.metadataPath(ref)
	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, metaData, 0600); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// GetUploadPath returns the filesystem path for an upload.
// This is used by the upload handler to write the file.
func (s *LocalStorage) GetUploadPath(uploadID string) (string, error) {
	s.mu.RLock()
	pending, ok := s.pendingUploads[uploadID]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: upload not found or expired", ErrUploadFailed)
	}

	ref, err := ParseStorageRef(pending.StorageRef)
	if err != nil {
		return "", err
	}

	// Ensure session directory exists
	sessionDir := s.sessionDir(ref.SessionID)
	if err := os.MkdirAll(sessionDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create session directory: %w", err)
	}

	return s.mediaPath(ref), nil
}

// GetDownloadURL generates a URL for downloading media.
func (s *LocalStorage) GetDownloadURL(ctx context.Context, storageRef string) (string, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return "", err
	}

	// Verify the file exists and isn't expired
	info, err := s.GetMediaInfo(ctx, storageRef)
	if err != nil {
		return "", err
	}

	if info.IsExpired() {
		return "", ErrMediaExpired
	}

	// Generate download URL
	// For local storage, downloads come from: {baseURL}/media/download/{session-id}/{media-id}
	return fmt.Sprintf("%s/media/download/%s/%s", s.config.BaseURL, ref.SessionID, ref.MediaID), nil
}

// GetMediaInfo retrieves metadata about stored media.
func (s *LocalStorage) GetMediaInfo(ctx context.Context, storageRef string) (*MediaInfo, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return nil, err
	}

	// Read metadata file
	metaPath := s.metadataPath(ref)
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta mediaMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	info := &MediaInfo{
		StorageRef: storageRef,
		Filename:   meta.Filename,
		MIMEType:   meta.MIMEType,
		SizeBytes:  meta.SizeBytes,
		CreatedAt:  meta.CreatedAt,
		ExpiresAt:  meta.ExpiresAt,
	}

	if info.IsExpired() {
		// Clean up expired media in background
		go func() {
			_ = s.Delete(context.Background(), storageRef)
		}()
		return nil, ErrMediaExpired
	}

	return info, nil
}

// GetMediaPath returns the filesystem path for stored media.
// This is used by the download handler to serve the file.
func (s *LocalStorage) GetMediaPath(storageRef string) (string, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return "", err
	}

	path := s.mediaPath(ref)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", ErrMediaNotFound
		}
		return "", err
	}

	return path, nil
}

// Delete removes media from storage.
func (s *LocalStorage) Delete(ctx context.Context, storageRef string) error {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return err
	}

	mediaPath := s.mediaPath(ref)
	metaPath := s.metadataPath(ref)

	// Check if media exists
	if _, err := os.Stat(mediaPath); err != nil {
		if os.IsNotExist(err) {
			return ErrMediaNotFound
		}
		return err
	}

	// Remove both files
	if err := os.Remove(mediaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete media file: %w", err)
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete metadata file: %w", err)
	}

	// Try to clean up empty session directory
	sessionDir := s.sessionDir(ref.SessionID)
	_ = os.Remove(sessionDir) // Ignore error - directory might not be empty

	return nil
}

// DeleteSessionMedia deletes all media for a session.
func (s *LocalStorage) DeleteSessionMedia(ctx context.Context, sessionID string) error {
	sessionDir := s.sessionDir(sessionID)

	if _, err := os.Stat(sessionDir); err != nil {
		if os.IsNotExist(err) {
			return nil // No media for this session
		}
		return err
	}

	return os.RemoveAll(sessionDir)
}

// Close releases any resources held by the storage.
func (s *LocalStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingUploads = make(map[string]*pendingUpload)
	return nil
}

// CleanupExpired removes all expired media.
// This can be called periodically to free up disk space.
func (s *LocalStorage) CleanupExpired(ctx context.Context) (int, error) {
	var cleaned int

	// Walk through all session directories
	entries, err := os.ReadDir(s.config.BasePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionDir := filepath.Join(s.config.BasePath, entry.Name())
		mediaFiles, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}

		for _, mediaFile := range mediaFiles {
			if filepath.Ext(mediaFile.Name()) == ".meta" {
				continue // Skip metadata files, process media files
			}

			ref := StorageRef{
				SessionID: entry.Name(),
				MediaID:   mediaFile.Name(),
			}

			info, err := s.GetMediaInfo(ctx, ref.String())
			if err == ErrMediaExpired {
				// Already cleaned up by GetMediaInfo
				cleaned++
				continue
			}
			if err != nil {
				continue
			}

			if info.IsExpired() {
				if err := s.Delete(ctx, ref.String()); err == nil {
					cleaned++
				}
			}
		}
	}

	return cleaned, nil
}

// Helper methods

func (s *LocalStorage) sessionDir(sessionID string) string {
	return filepath.Join(s.config.BasePath, sessionID)
}

func (s *LocalStorage) mediaPath(ref *StorageRef) string {
	return filepath.Join(s.config.BasePath, ref.SessionID, ref.MediaID)
}

func (s *LocalStorage) metadataPath(ref *StorageRef) string {
	return filepath.Join(s.config.BasePath, ref.SessionID, ref.MediaID+".meta")
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
