/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"google.golang.org/api/iterator"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// --- Mock S3 API ---

type mockS3API struct {
	listFunc   func(ctx context.Context, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
	deleteFunc func(ctx context.Context, input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error)
}

func (m *mockS3API) ListObjectsV2(
	ctx context.Context, input *s3.ListObjectsV2Input,
	_ ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, input)
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (m *mockS3API) DeleteObjects(
	ctx context.Context, input *s3.DeleteObjectsInput,
	_ ...func(*s3.Options),
) (*s3.DeleteObjectsOutput, error) {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, input)
	}
	return &s3.DeleteObjectsOutput{}, nil
}

// --- Mock GCS ---

type mockGCSClient struct {
	bucketFunc func(name string) gcsBucketAPI
}

func (m *mockGCSClient) Bucket(name string) gcsBucketAPI {
	return m.bucketFunc(name)
}

type mockGCSBucket struct {
	objectsFunc func(ctx context.Context, q *storage.Query) gcsObjectIterator
	deleteFunc  func(ctx context.Context, name string) error
}

func (m *mockGCSBucket) Objects(
	ctx context.Context, q *storage.Query,
) gcsObjectIterator {
	return m.objectsFunc(ctx, q)
}

func (m *mockGCSBucket) DeleteObject(
	ctx context.Context, name string,
) error {
	return m.deleteFunc(ctx, name)
}

type mockGCSIterator struct {
	items []*storage.ObjectAttrs
	idx   int
	err   error
}

func (m *mockGCSIterator) Next() (*storage.ObjectAttrs, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.idx >= len(m.items) {
		return nil, iterator.Done
	}
	item := m.items[m.idx]
	m.idx++
	return item, nil
}

// --- Mock Azure ---

type mockAzureClient struct {
	pagerFunc func(
		container string, o *azblob.ListBlobsFlatOptions,
	) azureBlobPager
	deleteFunc func(
		ctx context.Context, container, blob string,
		o *azblob.DeleteBlobOptions,
	) (azblob.DeleteBlobResponse, error)
}

func (m *mockAzureClient) NewListBlobsFlatPager(
	containerName string, o *azblob.ListBlobsFlatOptions,
) azureBlobPager {
	return m.pagerFunc(containerName, o)
}

func (m *mockAzureClient) DeleteBlob(
	ctx context.Context, containerName, blobName string,
	o *azblob.DeleteBlobOptions,
) (azblob.DeleteBlobResponse, error) {
	return m.deleteFunc(ctx, containerName, blobName, o)
}

type mockAzurePager struct {
	pages []azblob.ListBlobsFlatResponse
	idx   int
	err   error
}

func (m *mockAzurePager) More() bool {
	return m.idx < len(m.pages)
}

func (m *mockAzurePager) NextPage(
	_ context.Context,
) (azblob.ListBlobsFlatResponse, error) {
	if m.err != nil {
		return azblob.ListBlobsFlatResponse{}, m.err
	}
	page := m.pages[m.idx]
	m.idx++
	return page, nil
}

// --- Mock ObjectStoreClient ---

type adapterMockObjectStore struct {
	ListFunc   func(ctx context.Context, bucket, prefix string) ([]string, error)
	DeleteFunc func(ctx context.Context, bucket string, keys []string) error
}

func (m *adapterMockObjectStore) ListObjects(
	ctx context.Context, bucket, prefix string,
) ([]string, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, bucket, prefix)
	}
	return nil, nil
}

func (m *adapterMockObjectStore) DeleteObjects(
	ctx context.Context, bucket string, keys []string,
) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, bucket, keys)
	}
	return nil
}

// --- S3 Adapter Tests ---

func TestS3ObjectStoreClient_ListObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("returns keys", func(t *testing.T) {
		mock := &mockS3API{
			listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
				return &s3.ListObjectsV2Output{
					Contents: []s3types.Object{
						{Key: aws.String("sessions/abc/file1.png")},
						{Key: aws.String("sessions/abc/file2.jpg")},
					},
				}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		keys, err := client.ListObjects(context.Background(), "bucket", "sessions/abc/")
		assertNoError(t, err)
		assertEqual(t, 2, len(keys))
		assertEqual(t, "sessions/abc/file1.png", keys[0])
	})

	t.Run("returns nil for empty", func(t *testing.T) {
		mock := &mockS3API{
			listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
				return &s3.ListObjectsV2Output{}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		keys, err := client.ListObjects(context.Background(), "bucket", "sessions/none/")
		assertNoError(t, err)
		if keys != nil {
			t.Errorf("expected nil keys, got %v", keys)
		}
	})

	t.Run("paginates", func(t *testing.T) {
		callCount := 0
		mock := &mockS3API{
			listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
				callCount++
				if callCount == 1 {
					return &s3.ListObjectsV2Output{
						Contents:              []s3types.Object{{Key: aws.String("key1")}},
						IsTruncated:           aws.Bool(true),
						NextContinuationToken: aws.String("token1"),
					}, nil
				}
				return &s3.ListObjectsV2Output{
					Contents: []s3types.Object{{Key: aws.String("key2")}},
				}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		keys, err := client.ListObjects(context.Background(), "bucket", "prefix/")
		assertNoError(t, err)
		assertEqual(t, 2, len(keys))
		assertEqual(t, 2, callCount)
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockS3API{
			listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
				return nil, errors.New("access denied")
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		_, err := client.ListObjects(context.Background(), "bucket", "prefix/")
		assertError(t, err)
		assertContains(t, err.Error(), "s3 list objects")
	})
}

func TestS3ObjectStoreClient_DeleteObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("deletes keys", func(t *testing.T) {
		var capturedInput *s3.DeleteObjectsInput
		mock := &mockS3API{
			deleteFunc: func(_ context.Context, input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
				capturedInput = input
				return &s3.DeleteObjectsOutput{}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "bucket", []string{"key1", "key2"})
		assertNoError(t, err)
		assertEqual(t, 2, len(capturedInput.Delete.Objects))
	})

	t.Run("empty keys is no-op", func(t *testing.T) {
		mock := &mockS3API{
			deleteFunc: func(_ context.Context, _ *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
				t.Fatal("should not be called")
				return nil, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "bucket", nil)
		assertNoError(t, err)
	})

	t.Run("batches over 1000 keys", func(t *testing.T) {
		batchCount := 0
		var batchSizes []int
		mock := &mockS3API{
			deleteFunc: func(_ context.Context, input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
				batchCount++
				batchSizes = append(batchSizes, len(input.Delete.Objects))
				return &s3.DeleteObjectsOutput{}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		keys := make([]string, 2500)
		for i := range keys {
			keys[i] = "key" + strconv.Itoa(i)
		}

		err := client.DeleteObjects(context.Background(), "bucket", keys)
		assertNoError(t, err)
		assertEqual(t, 3, batchCount)
		assertEqual(t, 1000, batchSizes[0])
		assertEqual(t, 1000, batchSizes[1])
		assertEqual(t, 500, batchSizes[2])
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockS3API{
			deleteFunc: func(_ context.Context, _ *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
				return nil, errors.New("delete failed")
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "bucket", []string{"key1"})
		assertError(t, err)
		assertContains(t, err.Error(), "s3 delete objects")
	})

	t.Run("stops on batch error", func(t *testing.T) {
		batchCount := 0
		mock := &mockS3API{
			deleteFunc: func(_ context.Context, _ *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
				batchCount++
				if batchCount == 2 {
					return nil, errors.New("batch 2 failed")
				}
				return &s3.DeleteObjectsOutput{}, nil
			},
		}
		client := &S3ObjectStoreClient{client: mock, log: log}

		keys := make([]string, 2500)
		for i := range keys {
			keys[i] = "key" + strconv.Itoa(i)
		}

		err := client.DeleteObjects(context.Background(), "bucket", keys)
		assertError(t, err)
		assertEqual(t, 2, batchCount)
	})
}

// --- GCS Adapter Tests ---

func TestGCSObjectStoreClient_ListObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("returns keys", func(t *testing.T) {
		bucket := &mockGCSBucket{
			objectsFunc: func(_ context.Context, _ *storage.Query) gcsObjectIterator {
				return &mockGCSIterator{items: []*storage.ObjectAttrs{
					{Name: "sessions/abc/file1"},
					{Name: "sessions/abc/file2"},
				}}
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		keys, err := client.ListObjects(context.Background(), "bucket", "sessions/abc/")
		assertNoError(t, err)
		assertEqual(t, 2, len(keys))
		assertEqual(t, "sessions/abc/file1", keys[0])
	})

	t.Run("returns nil for empty", func(t *testing.T) {
		bucket := &mockGCSBucket{
			objectsFunc: func(_ context.Context, _ *storage.Query) gcsObjectIterator {
				return &mockGCSIterator{items: nil}
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		keys, err := client.ListObjects(context.Background(), "bucket", "prefix/")
		assertNoError(t, err)
		if keys != nil {
			t.Errorf("expected nil, got %v", keys)
		}
	})

	t.Run("returns error", func(t *testing.T) {
		bucket := &mockGCSBucket{
			objectsFunc: func(_ context.Context, _ *storage.Query) gcsObjectIterator {
				return &mockGCSIterator{err: errors.New("permission denied")}
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		_, err := client.ListObjects(context.Background(), "bucket", "prefix/")
		assertError(t, err)
		assertContains(t, err.Error(), "gcs list objects")
	})
}

func TestGCSObjectStoreClient_DeleteObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("deletes keys", func(t *testing.T) {
		var deleted []string
		bucket := &mockGCSBucket{
			deleteFunc: func(_ context.Context, name string) error {
				deleted = append(deleted, name)
				return nil
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		err := client.DeleteObjects(context.Background(), "bucket", []string{"k1", "k2"})
		assertNoError(t, err)
		assertEqual(t, 2, len(deleted))
	})

	t.Run("empty keys is no-op", func(t *testing.T) {
		bucket := &mockGCSBucket{
			deleteFunc: func(_ context.Context, _ string) error {
				t.Fatal("should not be called")
				return nil
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		err := client.DeleteObjects(context.Background(), "bucket", nil)
		assertNoError(t, err)
	})

	t.Run("returns error", func(t *testing.T) {
		bucket := &mockGCSBucket{
			deleteFunc: func(_ context.Context, _ string) error {
				return errors.New("delete failed")
			},
		}
		client := &GCSObjectStoreClient{
			client: &mockGCSClient{bucketFunc: func(_ string) gcsBucketAPI { return bucket }},
			log:    log,
		}

		err := client.DeleteObjects(context.Background(), "bucket", []string{"key1"})
		assertError(t, err)
		assertContains(t, err.Error(), "gcs delete object")
	})
}

// --- Azure Adapter Tests ---

func TestAzureObjectStoreClient_ListObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("returns keys", func(t *testing.T) {
		name1, name2 := "blob1", "blob2"
		mock := &mockAzureClient{
			pagerFunc: func(_ string, _ *azblob.ListBlobsFlatOptions) azureBlobPager {
				return &mockAzurePager{pages: []azblob.ListBlobsFlatResponse{{
					ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
						Segment: &container.BlobFlatListSegment{
							BlobItems: []*container.BlobItem{
								{Name: &name1},
								{Name: &name2},
							},
						},
					},
				}}}
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		keys, err := client.ListObjects(context.Background(), "container", "prefix/")
		assertNoError(t, err)
		assertEqual(t, 2, len(keys))
		assertEqual(t, "blob1", keys[0])
	})

	t.Run("returns nil for empty", func(t *testing.T) {
		mock := &mockAzureClient{
			pagerFunc: func(_ string, _ *azblob.ListBlobsFlatOptions) azureBlobPager {
				return &mockAzurePager{pages: []azblob.ListBlobsFlatResponse{{
					ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
						Segment: &container.BlobFlatListSegment{
							BlobItems: []*container.BlobItem{},
						},
					},
				}}}
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		keys, err := client.ListObjects(context.Background(), "container", "prefix/")
		assertNoError(t, err)
		if keys != nil {
			t.Errorf("expected nil, got %v", keys)
		}
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockAzureClient{
			pagerFunc: func(_ string, _ *azblob.ListBlobsFlatOptions) azureBlobPager {
				return &mockAzurePager{
					pages: []azblob.ListBlobsFlatResponse{{}},
					err:   errors.New("network error"),
				}
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		_, err := client.ListObjects(context.Background(), "container", "prefix/")
		assertError(t, err)
		assertContains(t, err.Error(), "azure list blobs")
	})
}

func TestAzureObjectStoreClient_DeleteObjects(t *testing.T) {
	log := testr.New(t)

	t.Run("deletes keys", func(t *testing.T) {
		var deleted []string
		mock := &mockAzureClient{
			deleteFunc: func(_ context.Context, _, blob string, _ *azblob.DeleteBlobOptions) (azblob.DeleteBlobResponse, error) {
				deleted = append(deleted, blob)
				return azblob.DeleteBlobResponse{}, nil
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "container", []string{"b1", "b2"})
		assertNoError(t, err)
		assertEqual(t, 2, len(deleted))
	})

	t.Run("empty keys is no-op", func(t *testing.T) {
		mock := &mockAzureClient{
			deleteFunc: func(_ context.Context, _, _ string, _ *azblob.DeleteBlobOptions) (azblob.DeleteBlobResponse, error) {
				t.Fatal("should not be called")
				return azblob.DeleteBlobResponse{}, nil
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "container", nil)
		assertNoError(t, err)
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockAzureClient{
			deleteFunc: func(_ context.Context, _, _ string, _ *azblob.DeleteBlobOptions) (azblob.DeleteBlobResponse, error) {
				return azblob.DeleteBlobResponse{}, errors.New("delete failed")
			},
		}
		client := &AzureObjectStoreClient{client: mock, log: log}

		err := client.DeleteObjects(context.Background(), "container", []string{"blob1"})
		assertError(t, err)
		assertContains(t, err.Error(), "azure delete blob")
	})
}

// --- ObjectStoreMediaDeleter Tests ---

func TestObjectStoreMediaDeleter_DeleteSessionMedia(t *testing.T) {
	log := testr.New(t)

	t.Run("deletes media artifacts", func(t *testing.T) {
		var capturedBucket, capturedPrefix string
		var capturedKeys []string
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, bucket, prefix string) ([]string, error) {
				capturedBucket = bucket
				capturedPrefix = prefix
				return []string{"sessions/abc/file1", "sessions/abc/file2"}, nil
			},
			DeleteFunc: func(_ context.Context, _ string, keys []string) error {
				capturedKeys = keys
				return nil
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "my-bucket", "sessions/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "abc")
		assertNoError(t, err)
		assertEqual(t, "my-bucket", capturedBucket)
		assertEqual(t, "sessions/abc/", capturedPrefix)
		assertEqual(t, 2, len(capturedKeys))
	})

	t.Run("no artifacts is no-op", func(t *testing.T) {
		deleteCalled := false
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, _, _ string) ([]string, error) {
				return nil, nil
			},
			DeleteFunc: func(_ context.Context, _ string, _ []string) error {
				deleteCalled = true
				return nil
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "bucket", "sessions/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "empty-session")
		assertNoError(t, err)
		if deleteCalled {
			t.Error("delete should not be called for empty list")
		}
	})

	t.Run("empty list is no-op", func(t *testing.T) {
		deleteCalled := false
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, _, _ string) ([]string, error) {
				return []string{}, nil
			},
			DeleteFunc: func(_ context.Context, _ string, _ []string) error {
				deleteCalled = true
				return nil
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "bucket", "sessions/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "empty-session")
		assertNoError(t, err)
		if deleteCalled {
			t.Error("delete should not be called for empty list")
		}
	})

	t.Run("list error propagates", func(t *testing.T) {
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, _, _ string) ([]string, error) {
				return nil, errors.New("list failed")
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "bucket", "sessions/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "abc")
		assertError(t, err)
		assertContains(t, err.Error(), "listing media objects")
	})

	t.Run("delete error propagates", func(t *testing.T) {
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, _, _ string) ([]string, error) {
				return []string{"key1"}, nil
			},
			DeleteFunc: func(_ context.Context, _ string, _ []string) error {
				return errors.New("delete failed")
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "bucket", "sessions/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "abc")
		assertError(t, err)
		assertContains(t, err.Error(), "deleting media objects")
	})

	t.Run("prefix construction", func(t *testing.T) {
		var capturedPrefix string
		mock := &adapterMockObjectStore{
			ListFunc: func(_ context.Context, _, prefix string) ([]string, error) {
				capturedPrefix = prefix
				return nil, nil
			},
		}
		deleter := NewObjectStoreMediaDeleter(mock, "bucket", "media/", log)

		err := deleter.DeleteSessionMedia(context.Background(), "session-123")
		assertNoError(t, err)
		assertEqual(t, "media/session-123/", capturedPrefix)
	})
}

func TestNewS3ObjectStoreClient(t *testing.T) {
	log := logr.Discard()
	client := NewS3ObjectStoreClient(nil, log)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewGCSObjectStoreClient(t *testing.T) {
	log := logr.Discard()
	client := NewGCSObjectStoreClient(nil, log)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewAzureObjectStoreClient(t *testing.T) {
	log := logr.Discard()
	client := NewAzureObjectStoreClient(nil, log)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

// --- Test helpers ---

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func assertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", actual) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if len(s) == 0 || len(substr) == 0 {
		t.Fatalf("expected %q to contain %q", s, substr)
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Fatalf("expected %q to contain %q", s, substr)
}
