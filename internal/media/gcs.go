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
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

// GCSConfig contains configuration for Google Cloud Storage.
type GCSConfig struct {
	// Bucket is the GCS bucket name.
	Bucket string
	// Prefix is the key prefix for all media objects.
	Prefix string
	// UploadURLTTL is how long upload URLs remain valid.
	UploadURLTTL time.Duration
	// DownloadURLTTL is how long download URLs remain valid.
	DownloadURLTTL time.Duration
	// DefaultTTL is the default time-to-live for media (zero means no expiry).
	DefaultTTL time.Duration
	// MaxFileSize is the maximum allowed file size in bytes (0 means no limit).
	MaxFileSize int64
}

// DefaultGCSConfig returns a configuration with sensible defaults.
func DefaultGCSConfig(bucket string) GCSConfig {
	return GCSConfig{
		Bucket:         bucket,
		UploadURLTTL:   15 * time.Minute,
		DownloadURLTTL: 1 * time.Hour,
		DefaultTTL:     24 * time.Hour,
		MaxFileSize:    100 * 1024 * 1024, // 100MB
	}
}

// GCSStorage implements Storage using Google Cloud Storage.
type GCSStorage struct {
	client *storage.Client
	bucket *storage.BucketHandle
	config GCSConfig
	mu     sync.RWMutex
	// pendingUploads tracks uploads that have been initiated but not confirmed.
	pendingUploads map[string]*gcsPendingUpload
}

// Compile-time check that GCSStorage implements DirectUploadStorage.
var _ DirectUploadStorage = (*GCSStorage)(nil)

// gcsPendingUpload tracks an initiated upload.
type gcsPendingUpload struct {
	StorageRef string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	ExpiresAt  time.Time
	MediaTTL   time.Duration
}

// NewGCSStorage creates a new GCS storage backend.
func NewGCSStorage(ctx context.Context, cfg GCSConfig) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSStorage{
		client:         client,
		bucket:         client.Bucket(cfg.Bucket),
		config:         cfg,
		pendingUploads: make(map[string]*gcsPendingUpload),
	}, nil
}

// GetUploadURL generates a presigned URL for uploading media directly to GCS.
func (g *GCSStorage) GetUploadURL(ctx context.Context, req UploadRequest) (*UploadCredentials, error) {
	// Validate request
	if req.SessionID == "" {
		return nil, fmt.Errorf("%w: session ID is required", ErrInvalidStorageRef)
	}
	if req.MIMEType == "" {
		return nil, fmt.Errorf("%w: MIME type is required", ErrUnsupportedMIMEType)
	}
	if g.config.MaxFileSize > 0 && req.SizeBytes > g.config.MaxFileSize {
		return nil, fmt.Errorf("%w: max size is %d bytes", ErrFileTooLarge, g.config.MaxFileSize)
	}

	// Generate unique media ID
	mediaID, err := g.generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate media ID: %w", err)
	}

	// Build storage reference
	ref := StorageRef{
		SessionID: req.SessionID,
		MediaID:   mediaID,
	}

	// Calculate expiration
	uploadExpiry := time.Now().Add(g.config.UploadURLTTL)

	// Determine media TTL
	mediaTTL := req.TTL
	if mediaTTL == 0 {
		mediaTTL = g.config.DefaultTTL
	}

	// Track pending upload
	g.mu.Lock()
	g.pendingUploads[mediaID] = &gcsPendingUpload{
		StorageRef: ref.String(),
		Filename:   req.Filename,
		MIMEType:   req.MIMEType,
		SizeBytes:  req.SizeBytes,
		ExpiresAt:  uploadExpiry,
		MediaTTL:   mediaTTL,
	}
	g.mu.Unlock()

	// Generate presigned PUT URL
	key := g.objectKey(&ref)
	url, err := g.bucket.SignedURL(key, &storage.SignedURLOptions{
		Method:      "PUT",
		Expires:     uploadExpiry,
		ContentType: req.MIMEType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return &UploadCredentials{
		UploadID:   mediaID,
		URL:        url,
		StorageRef: ref.String(),
		ExpiresAt:  uploadExpiry,
		Method:     "PUT",
		Headers: map[string]string{
			"Content-Type": req.MIMEType,
		},
	}, nil
}

// ConfirmUpload verifies that an upload completed and stores metadata.
func (g *GCSStorage) ConfirmUpload(ctx context.Context, uploadID string) (*MediaInfo, error) {
	g.mu.Lock()
	pending, ok := g.pendingUploads[uploadID]
	if !ok {
		g.mu.Unlock()
		return nil, fmt.Errorf("%w: upload not found or expired", ErrUploadFailed)
	}
	delete(g.pendingUploads, uploadID)
	g.mu.Unlock()

	// Check if upload URL has expired
	if time.Now().After(pending.ExpiresAt) {
		return nil, fmt.Errorf("%w: upload URL expired", ErrUploadFailed)
	}

	// Parse storage ref to get the key
	ref, err := ParseStorageRef(pending.StorageRef)
	if err != nil {
		return nil, err
	}

	// Verify the object exists in GCS
	key := g.objectKey(ref)
	obj := g.bucket.Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, fmt.Errorf("%w: object not found in GCS", ErrUploadFailed)
		}
		return nil, fmt.Errorf("failed to verify upload: %w", err)
	}

	// Calculate media expiration
	var expiresAt time.Time
	if pending.MediaTTL > 0 {
		expiresAt = time.Now().Add(pending.MediaTTL)
	}

	// Store metadata as a separate object
	meta := mediaMetadata{
		Filename:  pending.Filename,
		MIMEType:  pending.MIMEType,
		SizeBytes: attrs.Size,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaKey := g.metadataKey(ref)
	metaWriter := g.bucket.Object(metaKey).NewWriter(ctx)
	metaWriter.ContentType = "application/json"
	if _, err := metaWriter.Write(metaData); err != nil {
		_ = metaWriter.Close()
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}
	if err := metaWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close metadata writer: %w", err)
	}

	return &MediaInfo{
		StorageRef: pending.StorageRef,
		Filename:   meta.Filename,
		MIMEType:   meta.MIMEType,
		SizeBytes:  meta.SizeBytes,
		CreatedAt:  meta.CreatedAt,
		ExpiresAt:  meta.ExpiresAt,
	}, nil
}

// GetDownloadURL generates a presigned URL for downloading media.
func (g *GCSStorage) GetDownloadURL(ctx context.Context, storageRef string) (string, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return "", err
	}

	// Verify the media exists and isn't expired
	info, err := g.GetMediaInfo(ctx, storageRef)
	if err != nil {
		return "", err
	}

	if info.IsExpired() {
		return "", ErrMediaExpired
	}

	// Generate signed GET URL
	key := g.objectKey(ref)
	url, err := g.bucket.SignedURL(key, &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(g.config.DownloadURLTTL),
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return url, nil
}

// GetMediaInfo retrieves metadata about stored media.
func (g *GCSStorage) GetMediaInfo(ctx context.Context, storageRef string) (*MediaInfo, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return nil, err
	}

	// Get metadata object
	metaKey := g.metadataKey(ref)
	reader, err := g.bucket.Object(metaKey).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer func() { _ = reader.Close() }()

	metaData, err := io.ReadAll(reader)
	if err != nil {
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
			_ = g.Delete(context.Background(), storageRef)
		}()
		return nil, ErrMediaExpired
	}

	return info, nil
}

// Delete removes media from GCS.
func (g *GCSStorage) Delete(ctx context.Context, storageRef string) error {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return err
	}

	mediaKey := g.objectKey(ref)
	metaKey := g.metadataKey(ref)

	// Delete media object
	if err := g.bucket.Object(mediaKey).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("failed to delete media object: %w", err)
	}

	// Delete metadata object
	if err := g.bucket.Object(metaKey).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("failed to delete metadata object: %w", err)
	}

	return nil
}

// DeleteSessionMedia deletes all media for a session.
func (g *GCSStorage) DeleteSessionMedia(ctx context.Context, sessionID string) error {
	prefix := g.sessionPrefix(sessionID)

	// List all objects with the session prefix
	it := g.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	var objectNames []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, storage.ErrObjectNotExist) {
			break
		}
		if err != nil {
			// Check for iterator done
			if err.Error() == "no more items in iterator" {
				break
			}
			return fmt.Errorf("failed to list objects: %w", err)
		}
		if attrs == nil {
			break
		}
		objectNames = append(objectNames, attrs.Name)
	}

	// Delete all objects
	for _, name := range objectNames {
		if err := g.bucket.Object(name).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return fmt.Errorf("failed to delete object %s: %w", name, err)
		}
	}

	return nil
}

// Close releases any resources held by the storage.
func (g *GCSStorage) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pendingUploads = make(map[string]*gcsPendingUpload)
	return g.client.Close()
}

// Helper methods

func (g *GCSStorage) sessionPrefix(sessionID string) string {
	if g.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/", g.config.Prefix, sessionID)
	}
	return fmt.Sprintf("%s/", sessionID)
}

func (g *GCSStorage) objectKey(ref *StorageRef) string {
	if g.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/%s", g.config.Prefix, ref.SessionID, ref.MediaID)
	}
	return fmt.Sprintf("%s/%s", ref.SessionID, ref.MediaID)
}

func (g *GCSStorage) metadataKey(ref *StorageRef) string {
	return g.objectKey(ref) + ".meta"
}

func (g *GCSStorage) generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
