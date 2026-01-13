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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
)

// AzureConfig contains configuration for Azure Blob Storage.
type AzureConfig struct {
	// AccountName is the Azure Storage account name.
	AccountName string
	// ContainerName is the blob container name.
	ContainerName string
	// Prefix is the blob name prefix for all media objects.
	Prefix string
	// AccountKey is the storage account key (optional - use for cross-cloud or non-Azure environments).
	// If not provided, uses DefaultAzureCredential (workload identity, managed identity, etc.).
	AccountKey string
	// UploadURLTTL is how long upload URLs remain valid.
	UploadURLTTL time.Duration
	// DownloadURLTTL is how long download URLs remain valid.
	DownloadURLTTL time.Duration
	// DefaultTTL is the default time-to-live for media (zero means no expiry).
	DefaultTTL time.Duration
	// MaxFileSize is the maximum allowed file size in bytes (0 means no limit).
	MaxFileSize int64
}

// DefaultAzureConfig returns a configuration with sensible defaults.
func DefaultAzureConfig(accountName, containerName string) AzureConfig {
	return AzureConfig{
		AccountName:    accountName,
		ContainerName:  containerName,
		UploadURLTTL:   15 * time.Minute,
		DownloadURLTTL: 1 * time.Hour,
		DefaultTTL:     24 * time.Hour,
		MaxFileSize:    100 * 1024 * 1024, // 100MB
	}
}

// AzureStorage implements Storage using Azure Blob Storage.
type AzureStorage struct {
	client     *azblob.Client
	config     AzureConfig
	credential azcore.TokenCredential
	// sharedKeyCredential is used when AccountKey is provided (for SAS generation)
	sharedKeyCredential *azblob.SharedKeyCredential
	mu                  sync.RWMutex
	// pendingUploads tracks uploads that have been initiated but not confirmed.
	pendingUploads map[string]*azurePendingUpload
}

// Compile-time check that AzureStorage implements DirectUploadStorage.
var _ DirectUploadStorage = (*AzureStorage)(nil)

// azurePendingUpload tracks an initiated upload.
type azurePendingUpload struct {
	StorageRef string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	ExpiresAt  time.Time
	MediaTTL   time.Duration
}

// NewAzureStorage creates a new Azure Blob Storage backend.
func NewAzureStorage(ctx context.Context, cfg AzureConfig) (*AzureStorage, error) {
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AccountName)

	storage := &AzureStorage{
		config:         cfg,
		pendingUploads: make(map[string]*azurePendingUpload),
	}

	var err error

	if cfg.AccountKey != "" {
		// Use shared key credential (for cross-cloud or explicit credentials)
		storage.sharedKeyCredential, err = azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create shared key credential: %w", err)
		}

		storage.client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, storage.sharedKeyCredential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client with shared key: %w", err)
		}
	} else {
		// Use DefaultAzureCredential (workload identity, managed identity, etc.)
		storage.credential, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure credential: %w", err)
		}

		storage.client, err = azblob.NewClient(serviceURL, storage.credential, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure client: %w", err)
		}
	}

	return storage, nil
}

// GetUploadURL generates a presigned URL for uploading media directly to Azure Blob Storage.
func (a *AzureStorage) GetUploadURL(ctx context.Context, req UploadRequest) (*UploadCredentials, error) {
	// Validate request
	if req.SessionID == "" {
		return nil, fmt.Errorf("%w: session ID is required", ErrInvalidStorageRef)
	}
	if req.MIMEType == "" {
		return nil, fmt.Errorf("%w: MIME type is required", ErrUnsupportedMIMEType)
	}
	if a.config.MaxFileSize > 0 && req.SizeBytes > a.config.MaxFileSize {
		return nil, fmt.Errorf("%w: max size is %d bytes", ErrFileTooLarge, a.config.MaxFileSize)
	}

	// Generate unique media ID
	mediaID, err := a.generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate media ID: %w", err)
	}

	// Build storage reference
	ref := StorageRef{
		SessionID: req.SessionID,
		MediaID:   mediaID,
	}

	// Calculate expiration
	uploadExpiry := time.Now().Add(a.config.UploadURLTTL)

	// Determine media TTL
	mediaTTL := req.TTL
	if mediaTTL == 0 {
		mediaTTL = a.config.DefaultTTL
	}

	// Track pending upload
	a.mu.Lock()
	a.pendingUploads[mediaID] = &azurePendingUpload{
		StorageRef: ref.String(),
		Filename:   req.Filename,
		MIMEType:   req.MIMEType,
		SizeBytes:  req.SizeBytes,
		ExpiresAt:  uploadExpiry,
		MediaTTL:   mediaTTL,
	}
	a.mu.Unlock()

	// Generate SAS URL for upload
	blobName := a.blobName(&ref)
	sasURL, err := a.generateSASURL(blobName, sas.BlobPermissions{Write: true, Create: true}, uploadExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SAS URL: %w", err)
	}

	return &UploadCredentials{
		UploadID:   mediaID,
		URL:        sasURL,
		StorageRef: ref.String(),
		ExpiresAt:  uploadExpiry,
		Method:     "PUT",
		Headers: map[string]string{
			"Content-Type":            req.MIMEType,
			"x-ms-blob-type":          "BlockBlob",
			"x-ms-blob-cache-control": "max-age=3600",
		},
	}, nil
}

// ConfirmUpload verifies that an upload completed and stores metadata.
func (a *AzureStorage) ConfirmUpload(ctx context.Context, uploadID string) (*MediaInfo, error) {
	a.mu.Lock()
	pending, ok := a.pendingUploads[uploadID]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: upload not found or expired", ErrUploadFailed)
	}
	delete(a.pendingUploads, uploadID)
	a.mu.Unlock()

	// Check if upload URL has expired
	if time.Now().After(pending.ExpiresAt) {
		return nil, fmt.Errorf("%w: upload URL expired", ErrUploadFailed)
	}

	// Parse storage ref to get the blob name
	ref, err := ParseStorageRef(pending.StorageRef)
	if err != nil {
		return nil, err
	}

	// Verify the blob exists
	blobName := a.blobName(ref)
	blobClient := a.client.ServiceClient().NewContainerClient(a.config.ContainerName).NewBlobClient(blobName)
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if errors.As(err, &storageErr) && storageErr.StatusCode == 404 {
			return nil, fmt.Errorf("%w: blob not found in Azure", ErrUploadFailed)
		}
		return nil, fmt.Errorf("failed to verify upload: %w", err)
	}

	// Calculate media expiration
	var expiresAt time.Time
	if pending.MediaTTL > 0 {
		expiresAt = time.Now().Add(pending.MediaTTL)
	}

	// Store metadata as a separate blob
	meta := mediaMetadata{
		Filename:  pending.Filename,
		MIMEType:  pending.MIMEType,
		SizeBytes: *props.ContentLength,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaBlobName := a.metadataBlobName(ref)
	_, err = a.client.UploadBuffer(ctx, a.config.ContainerName, metaBlobName, metaData, nil)
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
func (a *AzureStorage) GetDownloadURL(ctx context.Context, storageRef string) (string, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return "", err
	}

	// Verify the media exists and isn't expired
	info, err := a.GetMediaInfo(ctx, storageRef)
	if err != nil {
		return "", err
	}

	if info.IsExpired() {
		return "", ErrMediaExpired
	}

	// Generate SAS URL for download
	blobName := a.blobName(ref)
	sasURL, err := a.generateSASURL(blobName, sas.BlobPermissions{Read: true}, time.Now().Add(a.config.DownloadURLTTL))
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS URL: %w", err)
	}

	return sasURL, nil
}

// GetMediaInfo retrieves metadata about stored media.
func (a *AzureStorage) GetMediaInfo(ctx context.Context, storageRef string) (*MediaInfo, error) {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return nil, err
	}

	// Get metadata blob
	metaBlobName := a.metadataBlobName(ref)
	downloadResp, err := a.client.DownloadStream(ctx, a.config.ContainerName, metaBlobName, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if errors.As(err, &storageErr) && storageErr.StatusCode == 404 {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer func() { _ = downloadResp.Body.Close() }()

	metaData, err := io.ReadAll(downloadResp.Body)
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
		// Clean up expired media in background.
		// Using context.Background() intentionally: cleanup must complete
		// independently of the parent request's lifecycle.
		go func() {
			_ = a.Delete(context.Background(), storageRef) //nolint:contextcheck // intentional background cleanup
		}()
		return nil, ErrMediaExpired
	}

	return info, nil
}

// Delete removes media from Azure Blob Storage.
func (a *AzureStorage) Delete(ctx context.Context, storageRef string) error {
	ref, err := ParseStorageRef(storageRef)
	if err != nil {
		return err
	}

	mediaBlobName := a.blobName(ref)
	metaBlobName := a.metadataBlobName(ref)

	// Delete media blob
	_, err = a.client.DeleteBlob(ctx, a.config.ContainerName, mediaBlobName, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if errors.As(err, &storageErr) && storageErr.StatusCode != 404 {
			return fmt.Errorf("failed to delete media blob: %w", err)
		}
	}

	// Delete metadata blob
	_, err = a.client.DeleteBlob(ctx, a.config.ContainerName, metaBlobName, nil)
	if err != nil {
		var storageErr *azcore.ResponseError
		if errors.As(err, &storageErr) && storageErr.StatusCode != 404 {
			return fmt.Errorf("failed to delete metadata blob: %w", err)
		}
	}

	return nil
}

// DeleteSessionMedia deletes all media for a session.
func (a *AzureStorage) DeleteSessionMedia(ctx context.Context, sessionID string) error {
	prefix := a.sessionPrefix(sessionID)

	// List all blobs with the session prefix
	pager := a.client.NewListBlobsFlatPager(a.config.ContainerName, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list blobs: %w", err)
		}

		for _, item := range page.Segment.BlobItems {
			if item.Name == nil {
				continue
			}
			_, err := a.client.DeleteBlob(ctx, a.config.ContainerName, *item.Name, nil)
			if err != nil {
				var storageErr *azcore.ResponseError
				if errors.As(err, &storageErr) && storageErr.StatusCode != 404 {
					return fmt.Errorf("failed to delete blob %s: %w", *item.Name, err)
				}
			}
		}
	}

	return nil
}

// Close releases any resources held by the storage.
func (a *AzureStorage) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingUploads = make(map[string]*azurePendingUpload)
	return nil
}

// Helper methods

func (a *AzureStorage) sessionPrefix(sessionID string) string {
	if a.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/", a.config.Prefix, sessionID)
	}
	return fmt.Sprintf("%s/", sessionID)
}

func (a *AzureStorage) blobName(ref *StorageRef) string {
	if a.config.Prefix != "" {
		return fmt.Sprintf("%s/%s/%s", a.config.Prefix, ref.SessionID, ref.MediaID)
	}
	return fmt.Sprintf("%s/%s", ref.SessionID, ref.MediaID)
}

func (a *AzureStorage) metadataBlobName(ref *StorageRef) string {
	return a.blobName(ref) + ".meta"
}

func (a *AzureStorage) generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateSASURL generates a SAS URL for a blob.
func (a *AzureStorage) generateSASURL(blobName string, permissions sas.BlobPermissions, expiry time.Time) (string, error) {
	if a.sharedKeyCredential == nil {
		return "", fmt.Errorf("SAS URL generation requires shared key credential; set AccountKey in config")
	}

	// Generate SAS token
	sasQueryParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     time.Now().Add(-5 * time.Minute), // Start 5 minutes ago to handle clock skew
		ExpiryTime:    expiry,
		Permissions:   permissions.String(),
		ContainerName: a.config.ContainerName,
		BlobName:      blobName,
	}.SignWithSharedKey(a.sharedKeyCredential)
	if err != nil {
		return "", fmt.Errorf("failed to sign SAS: %w", err)
	}

	// Build full URL
	blobURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s?%s",
		a.config.AccountName,
		a.config.ContainerName,
		blobName,
		sasQueryParams.Encode())

	return blobURL, nil
}

// Note: Azure with workload identity uses User Delegation SAS which requires additional API calls.
// For production use with workload identity, consider using the following approach:
//
// 1. Use Azure's User Delegation Key API to get a delegation key
// 2. Generate User Delegation SAS with the delegation key
// 3. This requires the identity to have "Storage Blob Delegator" RBAC role
//
// The current implementation requires AccountKey for SAS generation. In Kubernetes environments,
// you can either:
// - Store the account key in a Kubernetes Secret and reference it via env var
// - Implement User Delegation SAS for full workload identity support
