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

package cold

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
)

// AzureBlobStore implements BlobStore using Azure Blob Storage.
type AzureBlobStore struct {
	client    *azblob.Client
	container string
}

// NewAzureBlobStore creates a new Azure Blob Storage-backed BlobStore.
func NewAzureBlobStore(ctx context.Context, container string, cfg AzureConfig) (*AzureBlobStore, error) {
	if container == "" {
		return nil, errors.New("container is required")
	}
	if cfg.AccountName == "" {
		return nil, errors.New("account name is required")
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AccountName)

	var client *azblob.Client
	var err error

	if cfg.AccountKey != "" {
		cred, credErr := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
		if credErr != nil {
			return nil, fmt.Errorf("failed to create shared key credential: %w", credErr)
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	} else {
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return nil, fmt.Errorf("failed to create Azure credential: %w", credErr)
		}
		client, err = azblob.NewClient(serviceURL, cred, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	return &AzureBlobStore{client: client, container: container}, nil
}

func (a *AzureBlobStore) Put(ctx context.Context, key string, data []byte, _ string) error {
	_, err := a.client.UploadBuffer(ctx, a.container, key, data, nil)
	if err != nil {
		return fmt.Errorf("azure put: %w", err)
	}
	return nil
}

func (a *AzureBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := a.client.DownloadStream(ctx, a.container, key, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("azure get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure read body: %w", err)
	}
	return data, nil
}

func (a *AzureBlobStore) Delete(ctx context.Context, key string) error {
	_, err := a.client.DeleteBlob(ctx, a.container, key, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return ErrObjectNotFound
		}
		return fmt.Errorf("azure delete: %w", err)
	}
	return nil
}

func (a *AzureBlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	pager := a.client.NewListBlobsFlatPager(a.container, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure list: %w", err)
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name != nil {
				keys = append(keys, *item.Name)
			}
		}
	}
	return keys, nil
}

func (a *AzureBlobStore) Exists(ctx context.Context, key string) (bool, error) {
	blobClient := a.client.ServiceClient().NewContainerClient(a.container).NewBlobClient(key)
	_, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("azure exists: %w", err)
	}
	return true, nil
}

func (a *AzureBlobStore) Ping(ctx context.Context) error {
	// Verify the container is accessible by listing zero blobs.
	pager := a.client.NewListBlobsFlatPager(a.container, &azblob.ListBlobsFlatOptions{
		MaxResults: int32Ptr(1),
	})
	if pager.More() {
		if _, err := pager.NextPage(ctx); err != nil {
			return fmt.Errorf("azure ping: %w", err)
		}
	}
	return nil
}

func (a *AzureBlobStore) Close() error {
	return nil
}

func int32Ptr(v int32) *int32 { return &v }

var _ BlobStore = (*AzureBlobStore)(nil)
