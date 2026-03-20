/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-logr/logr"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// s3DeleteBatchSize is the maximum number of objects per S3 DeleteObjects request.
const s3DeleteBatchSize = 1000

// --- S3 Adapter ---

// s3API is the subset of the S3 client API used by the adapter.
type s3API interface {
	ListObjectsV2(
		ctx context.Context, params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options),
	) (*s3.ListObjectsV2Output, error)
	DeleteObjects(
		ctx context.Context, params *s3.DeleteObjectsInput,
		optFns ...func(*s3.Options),
	) (*s3.DeleteObjectsOutput, error)
}

// S3ObjectStoreClient adapts an AWS S3 client to the ObjectStoreClient interface.
type S3ObjectStoreClient struct {
	client s3API
	log    logr.Logger
}

// NewS3ObjectStoreClient creates an ObjectStoreClient backed by AWS S3.
func NewS3ObjectStoreClient(
	client *s3.Client, log logr.Logger,
) *S3ObjectStoreClient {
	return &S3ObjectStoreClient{
		client: client,
		log:    log.WithName("s3-object-store"),
	}
}

// ListObjects lists all object keys under the given prefix
// using paginated ListObjectsV2 calls.
func (c *S3ObjectStoreClient) ListObjects(
	ctx context.Context, bucket, prefix string,
) ([]string, error) {
	var keys []string
	var continuationToken *string

	for {
		output, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("s3 list objects: %w", err)
		}
		for _, obj := range output.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if output.IsTruncated == nil || !*output.IsTruncated {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	return keys, nil
}

// DeleteObjects deletes keys from an S3 bucket in batches of 1000.
func (c *S3ObjectStoreClient) DeleteObjects(
	ctx context.Context, bucket string, keys []string,
) error {
	if len(keys) == 0 {
		return nil
	}

	for i := 0; i < len(keys); i += s3DeleteBatchSize {
		batch := keys[i:min(i+s3DeleteBatchSize, len(keys))]
		if err := c.deleteBatch(ctx, bucket, batch); err != nil {
			return err
		}
	}

	return nil
}

// deleteBatch deletes a single batch of keys (up to 1000).
func (c *S3ObjectStoreClient) deleteBatch(
	ctx context.Context, bucket string, keys []string,
) error {
	objects := make([]s3types.ObjectIdentifier, len(keys))
	for i, key := range keys {
		objects[i] = s3types.ObjectIdentifier{Key: aws.String(key)}
	}

	_, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &s3types.Delete{
			Objects: objects,
			Quiet:   aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("s3 delete objects: %w", err)
	}

	return nil
}

// Compile-time interface check.
var _ ObjectStoreClient = (*S3ObjectStoreClient)(nil)

// --- GCS Adapter ---

// gcsObjectIterator abstracts the GCS object listing iterator.
type gcsObjectIterator interface {
	Next() (*storage.ObjectAttrs, error)
}

// gcsBucketAPI abstracts GCS bucket operations for listing and deleting.
type gcsBucketAPI interface {
	Objects(ctx context.Context, q *storage.Query) gcsObjectIterator
	DeleteObject(ctx context.Context, name string) error
}

// gcsClientAPI abstracts the GCS client to return a bucket handle.
type gcsClientAPI interface {
	Bucket(name string) gcsBucketAPI
}

// realGCSClient wraps *storage.Client to satisfy gcsClientAPI.
type realGCSClient struct {
	client *storage.Client
}

func (r *realGCSClient) Bucket(name string) gcsBucketAPI {
	return &realGCSBucket{bh: r.client.Bucket(name)}
}

// realGCSBucket wraps *storage.BucketHandle to satisfy gcsBucketAPI.
type realGCSBucket struct {
	bh *storage.BucketHandle
}

func (b *realGCSBucket) Objects(
	ctx context.Context, q *storage.Query,
) gcsObjectIterator {
	return b.bh.Objects(ctx, q)
}

func (b *realGCSBucket) DeleteObject(
	ctx context.Context, name string,
) error {
	return b.bh.Object(name).Delete(ctx)
}

// GCSObjectStoreClient adapts a Google Cloud Storage client to the
// ObjectStoreClient interface.
type GCSObjectStoreClient struct {
	client gcsClientAPI
	log    logr.Logger
}

// NewGCSObjectStoreClient creates an ObjectStoreClient backed by
// Google Cloud Storage.
func NewGCSObjectStoreClient(
	client *storage.Client, log logr.Logger,
) *GCSObjectStoreClient {
	return &GCSObjectStoreClient{
		client: &realGCSClient{client: client},
		log:    log.WithName("gcs-object-store"),
	}
}

// ListObjects lists all object keys under the given prefix
// using the GCS Objects iterator.
func (c *GCSObjectStoreClient) ListObjects(
	ctx context.Context, bucket, prefix string,
) ([]string, error) {
	bh := c.client.Bucket(bucket)
	it := bh.Objects(ctx, &storage.Query{Prefix: prefix})

	var keys []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcs list objects: %w", err)
		}
		keys = append(keys, attrs.Name)
	}

	return keys, nil
}

// DeleteObjects deletes keys from a GCS bucket sequentially.
func (c *GCSObjectStoreClient) DeleteObjects(
	ctx context.Context, bucket string, keys []string,
) error {
	bh := c.client.Bucket(bucket)
	for _, key := range keys {
		if err := bh.DeleteObject(ctx, key); err != nil {
			return fmt.Errorf("gcs delete object %q: %w", key, err)
		}
	}
	return nil
}

// Compile-time interface check.
var _ ObjectStoreClient = (*GCSObjectStoreClient)(nil)

// --- Azure Adapter ---

// azureBlobPager abstracts Azure blob listing pagination.
type azureBlobPager interface {
	More() bool
	NextPage(ctx context.Context) (azblob.ListBlobsFlatResponse, error)
}

// azureBlobAPI abstracts the Azure blob client operations used by the adapter.
type azureBlobAPI interface {
	NewListBlobsFlatPager(
		containerName string,
		o *azblob.ListBlobsFlatOptions,
	) azureBlobPager
	DeleteBlob(
		ctx context.Context, containerName, blobName string,
		o *azblob.DeleteBlobOptions,
	) (azblob.DeleteBlobResponse, error)
}

// realAzureClient wraps *azblob.Client to satisfy azureBlobAPI.
type realAzureClient struct {
	client *azblob.Client
}

func (r *realAzureClient) NewListBlobsFlatPager(
	containerName string, o *azblob.ListBlobsFlatOptions,
) azureBlobPager {
	return r.client.NewListBlobsFlatPager(containerName, o)
}

func (r *realAzureClient) DeleteBlob(
	ctx context.Context, containerName, blobName string,
	o *azblob.DeleteBlobOptions,
) (azblob.DeleteBlobResponse, error) {
	return r.client.DeleteBlob(ctx, containerName, blobName, o)
}

// AzureObjectStoreClient adapts an Azure Blob Storage client to the
// ObjectStoreClient interface.
type AzureObjectStoreClient struct {
	client azureBlobAPI
	log    logr.Logger
}

// NewAzureObjectStoreClient creates an ObjectStoreClient backed by
// Azure Blob Storage.
func NewAzureObjectStoreClient(
	client *azblob.Client, log logr.Logger,
) *AzureObjectStoreClient {
	return &AzureObjectStoreClient{
		client: &realAzureClient{client: client},
		log:    log.WithName("azure-object-store"),
	}
}

// ListObjects lists all blob keys under the given prefix using the
// Azure flat pager. The bucket parameter maps to the Azure container.
func (c *AzureObjectStoreClient) ListObjects(
	ctx context.Context, bucket, prefix string,
) ([]string, error) {
	pager := c.client.NewListBlobsFlatPager(
		bucket, &azblob.ListBlobsFlatOptions{Prefix: &prefix},
	)

	var keys []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure list blobs: %w", err)
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name != nil {
				keys = append(keys, *item.Name)
			}
		}
	}

	return keys, nil
}

// DeleteObjects deletes blobs from an Azure container sequentially.
// The bucket parameter maps to the Azure container name.
func (c *AzureObjectStoreClient) DeleteObjects(
	ctx context.Context, bucket string, keys []string,
) error {
	for _, key := range keys {
		if _, err := c.client.DeleteBlob(
			ctx, bucket, key, nil,
		); err != nil {
			return fmt.Errorf("azure delete blob %q: %w", key, err)
		}
	}
	return nil
}

// Compile-time interface check.
var _ ObjectStoreClient = (*AzureObjectStoreClient)(nil)
