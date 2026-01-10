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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config contains configuration for S3 storage.
type S3Config struct {
	// Bucket is the S3 bucket name.
	Bucket string
	// Region is the AWS region.
	Region string
	// Prefix is the key prefix for all media objects.
	Prefix string
	// Endpoint is an optional custom endpoint (for S3-compatible services like MinIO).
	Endpoint string
	// UsePathStyle forces path-style addressing (required for MinIO).
	UsePathStyle bool
	// UploadURLTTL is how long upload URLs remain valid.
	UploadURLTTL time.Duration
	// DownloadURLTTL is how long download URLs remain valid.
	DownloadURLTTL time.Duration
	// DefaultTTL is the default time-to-live for media (zero means no expiry).
	DefaultTTL time.Duration
	// MaxFileSize is the maximum allowed file size in bytes (0 means no limit).
	MaxFileSize int64
}

// DefaultS3Config returns a configuration with sensible defaults.
func DefaultS3Config(bucket, region string) S3Config {
	return S3Config{
		Bucket:         bucket,
		Region:         region,
		UploadURLTTL:   15 * time.Minute,
		DownloadURLTTL: 1 * time.Hour,
		DefaultTTL:     24 * time.Hour,
		MaxFileSize:    100 * 1024 * 1024, // 100MB
	}
}

// S3Storage implements Storage using Amazon S3.
type S3Storage struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	config        S3Config
	mu            sync.RWMutex
	// pendingUploads tracks uploads that have been initiated but not confirmed.
	pendingUploads map[string]*s3PendingUpload
}

// Compile-time check that S3Storage implements DirectUploadStorage.
var _ DirectUploadStorage = (*S3Storage)(nil)

// s3PendingUpload tracks an initiated upload.
type s3PendingUpload struct {
	StorageRef string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	ExpiresAt  time.Time
	MediaTTL   time.Duration
}

// NewS3Storage creates a new S3 storage backend.
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	// Load AWS config
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with optional custom endpoint
	s3Opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = cfg.UsePathStyle
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	presignClient := s3.NewPresignClient(client)

	return &S3Storage{
		client:         client,
		presignClient:  presignClient,
		config:         cfg,
		pendingUploads: make(map[string]*s3PendingUpload),
	}, nil
}

// GetUploadURL generates a presigned URL for uploading media directly to S3.
func (s *S3Storage) GetUploadURL(ctx context.Context, req UploadRequest) (*UploadCredentials, error) {
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
	mediaID, err := s.generateID()
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
	s.pendingUploads[mediaID] = &s3PendingUpload{
		StorageRef: ref.String(),
		Filename:   req.Filename,
		MIMEType:   req.MIMEType,
		SizeBytes:  req.SizeBytes,
		ExpiresAt:  uploadExpiry,
		MediaTTL:   mediaTTL,
	}
	s.mu.Unlock()

	// Generate presigned PUT URL
	key := s.mediaKey(&ref)
	presignReq, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.config.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(req.MIMEType),
	}, s3.WithPresignExpires(s.config.UploadURLTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return &UploadCredentials{
		UploadID:   mediaID,
		URL:        presignReq.URL,
		StorageRef: ref.String(),
		ExpiresAt:  uploadExpiry,
		Method:     presignReq.Method,
		Headers: map[string]string{
			"Content-Type": req.MIMEType,
		},
	}, nil
}

// ConfirmUpload verifies that an upload completed and stores metadata.
func (s *S3Storage) ConfirmUpload(ctx context.Context, uploadID string) (*MediaInfo, error) {
	s.mu.Lock()
	pending, ok := s.pendingUploads[uploadID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: upload not found or expired", ErrUploadFailed)
	}
	delete(s.pendingUploads, uploadID)
	s.mu.Unlock()

	// Check if upload URL has expired
	if time.Now().After(pending.ExpiresAt) {
		return nil, fmt.Errorf("%w: upload URL expired", ErrUploadFailed)
	}

	// Parse storage ref to get the key
	ref, err := ParseStorageRef(pending.StorageRef)
	if err != nil {
		return nil, err
	}

	// Verify the object exists in S3
	key := s.mediaKey(ref)
	headResp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%w: object not found in S3", ErrUploadFailed)
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
		SizeBytes: aws.ToInt64(headResp.ContentLength),
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaKey := s.metadataKey(ref)
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.config.Bucket),
		Key:         aws.String(metaKey),
		Body:        bytes.NewReader(metaData),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store metadata: %w", err)
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
func (s *S3Storage) GetDownloadURL(ctx context.Context, storageRef string) (string, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return "", err
	}

	// Verify the media exists and isn't expired
	info, err := s.GetMediaInfo(ctx, storageRef)
	if err != nil {
		return "", err
	}

	if info.IsExpired() {
		return "", ErrMediaExpired
	}

	// Generate presigned GET URL
	key := s.mediaKey(ref)
	presignReq, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.config.Bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(fmt.Sprintf("inline; filename=\"%s\"", info.Filename)),
		ResponseContentType:        aws.String(info.MIMEType),
	}, s3.WithPresignExpires(s.config.DownloadURLTTL))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignReq.URL, nil
}

// GetMediaInfo retrieves metadata about stored media.
func (s *S3Storage) GetMediaInfo(ctx context.Context, storageRef string) (*MediaInfo, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return nil, err
	}

	// Get metadata object
	metaKey := s.metadataKey(ref)
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(metaKey),
	})
	if err != nil {
		var notFound *types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var meta mediaMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
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

// Delete removes media from S3.
func (s *S3Storage) Delete(ctx context.Context, storageRef string) error {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return err
	}

	mediaKey := s.mediaKey(ref)
	metaKey := s.metadataKey(ref)

	// Delete both media and metadata objects
	_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.config.Bucket),
		Delete: &types.Delete{
			Objects: []types.ObjectIdentifier{
				{Key: aws.String(mediaKey)},
				{Key: aws.String(metaKey)},
			},
			Quiet: aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}

	return nil
}

// DeleteSessionMedia deletes all media for a session.
func (s *S3Storage) DeleteSessionMedia(ctx context.Context, sessionID string) error {
	prefix := s.sessionPrefix(sessionID)

	// List all objects with the session prefix
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.config.Bucket),
		Prefix: aws.String(prefix),
	})

	var objectsToDelete []types.ObjectIdentifier
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key: obj.Key,
			})
		}
	}

	if len(objectsToDelete) == 0 {
		return nil
	}

	// Delete in batches of 1000 (S3 limit)
	for i := 0; i < len(objectsToDelete); i += 1000 {
		end := i + 1000
		if end > len(objectsToDelete) {
			end = len(objectsToDelete)
		}

		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.config.Bucket),
			Delete: &types.Delete{
				Objects: objectsToDelete[i:end],
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects: %w", err)
		}
	}

	return nil
}

// Close releases any resources held by the storage.
func (s *S3Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingUploads = make(map[string]*s3PendingUpload)
	return nil
}

// Helper methods

func (s *S3Storage) sessionPrefix(sessionID string) string {
	if s.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/", s.config.Prefix, sessionID)
	}
	return fmt.Sprintf("%s/", sessionID)
}

func (s *S3Storage) mediaKey(ref *StorageRef) string {
	if s.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/%s", s.config.Prefix, ref.SessionID, ref.MediaID)
	}
	return fmt.Sprintf("%s/%s", ref.SessionID, ref.MediaID)
}

func (s *S3Storage) metadataKey(ref *StorageRef) string {
	return s.mediaKey(ref) + ".meta"
}

func (s *S3Storage) generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
