/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
)

// s3PutFunc is a function type for PutObject operations.
type s3PutFunc func(
	ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error)

// s3GetFunc is a function type for GetObject operations.
type s3GetFunc func(
	ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
) (*s3.GetObjectOutput, error)

// s3HeadFunc is a function type for HeadObject operations.
type s3HeadFunc func(
	ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
) (*s3.HeadObjectOutput, error)

// s3DeleteFunc is a function type for DeleteObject operations.
type s3DeleteFunc func(
	ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error)

// s3ListFunc is a function type for ListObjectsV2 operations.
type s3ListFunc func(
	ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error)

// mockS3Client implements a mock S3 client for testing.
type mockS3Client struct {
	objects           map[string][]byte
	putObjectFunc     s3PutFunc
	getObjectFunc     s3GetFunc
	headObjectFunc    s3HeadFunc
	deleteObjectFunc  s3DeleteFunc
	listObjectsV2Func s3ListFunc
}

func newMockS3Client() *mockS3Client {
	m := &mockS3Client{
		objects: make(map[string][]byte),
	}

	// Default implementations
	m.putObjectFunc = func(
		ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options),
	) (*s3.PutObjectOutput, error) {
		data, err := io.ReadAll(params.Body)
		if err != nil {
			return nil, err
		}
		m.objects[*params.Key] = data
		return &s3.PutObjectOutput{}, nil
	}

	m.getObjectFunc = func(
		ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
	) (*s3.GetObjectOutput, error) {
		data, exists := m.objects[*params.Key]
		if !exists {
			return nil, &types.NoSuchKey{}
		}
		return &s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader(data)),
		}, nil
	}

	m.headObjectFunc = func(
		ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
	) (*s3.HeadObjectOutput, error) {
		_, exists := m.objects[*params.Key]
		if !exists {
			return nil, &types.NotFound{}
		}
		return &s3.HeadObjectOutput{}, nil
	}

	m.deleteObjectFunc = func(
		ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options),
	) (*s3.DeleteObjectOutput, error) {
		delete(m.objects, *params.Key)
		return &s3.DeleteObjectOutput{}, nil
	}

	m.listObjectsV2Func = func(
		ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
	) (*s3.ListObjectsV2Output, error) {
		var contents []types.Object
		prefix := ""
		if params.Prefix != nil {
			prefix = *params.Prefix
		}
		for key := range m.objects {
			if prefix == "" || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				keyCopy := key
				size := int64(len(m.objects[key]))
				modTime := time.Now()
				contents = append(contents, types.Object{
					Key:          &keyCopy,
					Size:         &size,
					LastModified: &modTime,
				})
			}
		}
		return &s3.ListObjectsV2Output{
			Contents: contents,
		}, nil
	}

	return m
}

func (m *mockS3Client) PutObject(
	ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	return m.putObjectFunc(ctx, params, optFns...)
}

func (m *mockS3Client) GetObject(
	ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	return m.getObjectFunc(ctx, params, optFns...)
}

func (m *mockS3Client) HeadObject(
	ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	return m.headObjectFunc(ctx, params, optFns...)
}

func (m *mockS3Client) DeleteObject(
	ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	return m.deleteObjectFunc(ctx, params, optFns...)
}

func (m *mockS3Client) ListObjectsV2(
	ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	return m.listObjectsV2Func(ctx, params, optFns...)
}

// Verify mockS3Client implements s3ListClient interface.
var _ s3ListClient = (*mockS3Client)(nil)

func TestDefaultS3Config(t *testing.T) {
	cfg := DefaultS3Config("my-bucket", "us-west-2")

	if cfg.Bucket != "my-bucket" {
		t.Errorf("Bucket = %v, want my-bucket", cfg.Bucket)
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("Region = %v, want us-west-2", cfg.Region)
	}
	if cfg.Prefix != "arena/results" {
		t.Errorf("Prefix = %v, want arena/results", cfg.Prefix)
	}
}

func TestNewS3Storage_Validation(t *testing.T) {
	t.Run("returns error for empty bucket", func(t *testing.T) {
		_, err := NewS3Storage(context.Background(), S3Config{
			Region: "us-west-2",
		})
		if err == nil || err.Error() != "bucket is required" {
			t.Errorf("NewS3Storage() error = %v, want bucket is required", err)
		}
	})

	t.Run("returns error for empty region", func(t *testing.T) {
		_, err := NewS3Storage(context.Background(), S3Config{
			Bucket: "my-bucket",
		})
		if err == nil || err.Error() != "region is required" {
			t.Errorf("NewS3Storage() error = %v, want region is required", err)
		}
	})
}

func TestS3Storage_Store(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	results := &JobResults{
		JobID:       "test-job-1",
		Namespace:   "default",
		ConfigName:  "test-config",
		StartedAt:   time.Now().Add(-time.Minute),
		CompletedAt: time.Now(),
		Summary: &aggregator.AggregatedResult{
			TotalItems:  10,
			PassedItems: 8,
			FailedItems: 2,
			PassRate:    80.0,
		},
		Results: []aggregator.ExecutionResult{
			{WorkItemID: "item-1", Status: "pass"},
		},
		Metadata: map[string]string{
			"version": "1.0",
		},
	}

	t.Run("stores results successfully", func(t *testing.T) {
		err := storage.Store(context.Background(), "test-job-1", results)
		if err != nil {
			t.Errorf("Store() error = %v", err)
		}

		// Verify object was stored
		if _, exists := mock.objects["arena/results/test-job-1.json"]; !exists {
			t.Error("Store() did not create object in S3")
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		err := storage.Store(context.Background(), "", results)
		if err != ErrInvalidJobID {
			t.Errorf("Store() error = %v, want %v", err, ErrInvalidJobID)
		}
	})

	t.Run("returns error after close", func(t *testing.T) {
		closedStorage := &S3Storage{
			client: mock,
			config: storage.config,
			closed: true,
		}
		err := closedStorage.Store(context.Background(), "test", results)
		if err != ErrStorageClosed {
			t.Errorf("Store() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("returns error on S3 failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.putObjectFunc = func(
			ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options),
		) (*s3.PutObjectOutput, error) {
			return nil, errors.New("S3 error")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		err := failingStorage.Store(context.Background(), "test", results)
		if err == nil {
			t.Error("Store() expected error on S3 failure")
		}
	})
}

func TestS3Storage_Get(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	results := &JobResults{
		JobID:     "test-job-1",
		Namespace: "default",
		Summary: &aggregator.AggregatedResult{
			TotalItems: 10,
		},
	}

	// Store results first
	_ = storage.Store(context.Background(), "test-job-1", results)

	t.Run("retrieves stored results", func(t *testing.T) {
		got, err := storage.Get(context.Background(), "test-job-1")
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
		if got.JobID != "test-job-1" {
			t.Errorf("JobID = %v, want test-job-1", got.JobID)
		}
		if got.Summary.TotalItems != 10 {
			t.Errorf("TotalItems = %v, want 10", got.Summary.TotalItems)
		}
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		_, err := storage.Get(context.Background(), "non-existent")
		if err != ErrResultNotFound {
			t.Errorf("Get() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		_, err := storage.Get(context.Background(), "")
		if err != ErrInvalidJobID {
			t.Errorf("Get() error = %v, want %v", err, ErrInvalidJobID)
		}
	})

	t.Run("returns error after close", func(t *testing.T) {
		closedStorage := &S3Storage{
			client: mock,
			config: storage.config,
			closed: true,
		}
		_, err := closedStorage.Get(context.Background(), "test-job-1")
		if err != ErrStorageClosed {
			t.Errorf("Get() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("handles NoSuchKey error string", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.getObjectFunc = func(
			ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
		) (*s3.GetObjectOutput, error) {
			return nil, errors.New("NoSuchKey: the specified key does not exist")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		_, err := failingStorage.Get(context.Background(), "test")
		if err != ErrResultNotFound {
			t.Errorf("Get() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("handles not found error string", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.getObjectFunc = func(
			ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
		) (*s3.GetObjectOutput, error) {
			return nil, errors.New("key not found")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		_, err := failingStorage.Get(context.Background(), "test")
		if err != ErrResultNotFound {
			t.Errorf("Get() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error on read failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.getObjectFunc = func(
			ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
		) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(&failingReader{}),
			}, nil
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		_, err := failingStorage.Get(context.Background(), "test")
		if err == nil {
			t.Error("Get() expected error on read failure")
		}
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		invalidMock := newMockS3Client()
		invalidMock.objects["arena/results/invalid.json"] = []byte("not valid json")
		invalidStorage := &S3Storage{
			client: invalidMock,
			config: storage.config,
		}
		_, err := invalidStorage.Get(context.Background(), "invalid")
		if err == nil {
			t.Error("Get() expected error on invalid JSON")
		}
	})
}

type failingReader struct{}

func (r *failingReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestS3Storage_List(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	// Store multiple results
	for _, jobID := range []string{"job-a-1", "job-a-2", "job-b-1", "job-c-1"} {
		_ = storage.Store(context.Background(), jobID, &JobResults{JobID: jobID})
	}

	t.Run("lists all jobs with empty prefix", func(t *testing.T) {
		jobs, err := storage.List(context.Background(), "")
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(jobs) != 4 {
			t.Errorf("List() returned %d jobs, want 4", len(jobs))
		}
	})

	t.Run("lists jobs with prefix", func(t *testing.T) {
		jobs, err := storage.List(context.Background(), "job-a")
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(jobs) != 2 {
			t.Errorf("List() returned %d jobs, want 2", len(jobs))
		}
	})

	t.Run("returns sorted results", func(t *testing.T) {
		jobs, _ := storage.List(context.Background(), "")
		for i := 1; i < len(jobs); i++ {
			if jobs[i-1] > jobs[i] {
				t.Errorf("List() not sorted: %v > %v", jobs[i-1], jobs[i])
			}
		}
	})

	t.Run("returns error after close", func(t *testing.T) {
		closedStorage := &S3Storage{
			client: mock,
			config: storage.config,
			closed: true,
		}
		_, err := closedStorage.List(context.Background(), "")
		if err != ErrStorageClosed {
			t.Errorf("List() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("returns error on S3 failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.listObjectsV2Func = func(
			ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
		) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("S3 error")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		_, err := failingStorage.List(context.Background(), "")
		if err == nil {
			t.Error("List() expected error on S3 failure")
		}
	})
}

func TestS3Storage_Delete(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	_ = storage.Store(context.Background(), "test-job-1", &JobResults{JobID: "test-job-1"})

	t.Run("deletes existing results", func(t *testing.T) {
		err := storage.Delete(context.Background(), "test-job-1")
		if err != nil {
			t.Errorf("Delete() error = %v", err)
		}

		_, err = storage.Get(context.Background(), "test-job-1")
		if err != ErrResultNotFound {
			t.Errorf("Get() after Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		err := storage.Delete(context.Background(), "non-existent")
		if err != ErrResultNotFound {
			t.Errorf("Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error for empty jobID", func(t *testing.T) {
		err := storage.Delete(context.Background(), "")
		if err != ErrInvalidJobID {
			t.Errorf("Delete() error = %v, want %v", err, ErrInvalidJobID)
		}
	})

	t.Run("returns error after close", func(t *testing.T) {
		closedStorage := &S3Storage{
			client: mock,
			config: storage.config,
			closed: true,
		}
		err := closedStorage.Delete(context.Background(), "test")
		if err != ErrStorageClosed {
			t.Errorf("Delete() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("handles NotFound error string", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.headObjectFunc = func(
			ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
		) (*s3.HeadObjectOutput, error) {
			return nil, errors.New("NotFound: the specified key does not exist")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		err := failingStorage.Delete(context.Background(), "test")
		if err != ErrResultNotFound {
			t.Errorf("Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("handles not found error string", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.headObjectFunc = func(
			ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
		) (*s3.HeadObjectOutput, error) {
			return nil, errors.New("key not found")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		err := failingStorage.Delete(context.Background(), "test")
		if err != ErrResultNotFound {
			t.Errorf("Delete() error = %v, want %v", err, ErrResultNotFound)
		}
	})

	t.Run("returns error on head failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.headObjectFunc = func(
			ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
		) (*s3.HeadObjectOutput, error) {
			return nil, errors.New("permission denied")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		err := failingStorage.Delete(context.Background(), "test")
		if err == nil {
			t.Error("Delete() expected error on head failure")
		}
	})

	t.Run("returns error on delete failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.objects["arena/results/test.json"] = []byte("{}")
		failingMock.deleteObjectFunc = func(
			ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options),
		) (*s3.DeleteObjectOutput, error) {
			return nil, errors.New("S3 error")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		err := failingStorage.Delete(context.Background(), "test")
		if err == nil {
			t.Error("Delete() expected error on delete failure")
		}
	})
}

func TestS3Storage_Close(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	err := storage.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify operations fail after close
	err = storage.Store(context.Background(), "test", &JobResults{})
	if err != ErrStorageClosed {
		t.Errorf("Store() after Close() error = %v, want %v", err, ErrStorageClosed)
	}
}

func TestS3Storage_ListWithInfo(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "arena/results",
		},
	}

	now := time.Now()
	results := []*JobResults{
		{
			JobID:       "job-1",
			Namespace:   "ns-1",
			CompletedAt: now,
			Summary: &aggregator.AggregatedResult{
				TotalItems:  10,
				PassedItems: 8,
				FailedItems: 2,
			},
		},
		{
			JobID:       "job-2",
			Namespace:   "ns-2",
			CompletedAt: now.Add(-time.Hour),
			Summary: &aggregator.AggregatedResult{
				TotalItems:  5,
				PassedItems: 5,
				FailedItems: 0,
			},
		},
	}
	for _, r := range results {
		_ = storage.Store(context.Background(), r.JobID, r)
	}

	t.Run("lists with metadata", func(t *testing.T) {
		infos, err := storage.ListWithInfo(context.Background(), "")
		if err != nil {
			t.Errorf("ListWithInfo() error = %v", err)
		}
		if len(infos) != 2 {
			t.Errorf("ListWithInfo() returned %d infos, want 2", len(infos))
		}
	})

	t.Run("includes size information", func(t *testing.T) {
		infos, _ := storage.ListWithInfo(context.Background(), "")
		for _, info := range infos {
			if info.SizeBytes <= 0 {
				t.Errorf("info.SizeBytes = %v, want > 0", info.SizeBytes)
			}
		}
	})

	t.Run("returns error after close", func(t *testing.T) {
		closedStorage := &S3Storage{
			client: mock,
			config: storage.config,
			closed: true,
		}
		_, err := closedStorage.ListWithInfo(context.Background(), "")
		if err != ErrStorageClosed {
			t.Errorf("ListWithInfo() error = %v, want %v", err, ErrStorageClosed)
		}
	})

	t.Run("returns error on S3 failure", func(t *testing.T) {
		failingMock := newMockS3Client()
		failingMock.listObjectsV2Func = func(
			ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
		) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("S3 error")
		}
		failingStorage := &S3Storage{
			client: failingMock,
			config: storage.config,
		}
		_, err := failingStorage.ListWithInfo(context.Background(), "")
		if err == nil {
			t.Error("ListWithInfo() expected error on S3 failure")
		}
	})

	t.Run("handles nil key in listing", func(t *testing.T) {
		nilKeyMock := newMockS3Client()
		nilKeyMock.listObjectsV2Func = func(
			ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
		) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: nil},
				},
			}, nil
		}
		nilKeyStorage := &S3Storage{
			client: nilKeyMock,
			config: storage.config,
		}
		infos, err := nilKeyStorage.ListWithInfo(context.Background(), "")
		if err != nil {
			t.Errorf("ListWithInfo() error = %v", err)
		}
		if len(infos) != 0 {
			t.Errorf("ListWithInfo() returned %d infos, want 0", len(infos))
		}
	})
}

func TestS3Storage_keyForJob(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		storage := &S3Storage{
			config: S3Config{
				Prefix: "arena/results",
			},
		}
		key := storage.keyForJob("test-job")
		if key != "arena/results/test-job.json" {
			t.Errorf("keyForJob() = %v, want arena/results/test-job.json", key)
		}
	})

	t.Run("without prefix", func(t *testing.T) {
		storage := &S3Storage{
			config: S3Config{
				Prefix: "",
			},
		}
		key := storage.keyForJob("test-job")
		if key != "test-job.json" {
			t.Errorf("keyForJob() = %v, want test-job.json", key)
		}
	})
}

func TestS3Storage_jobIDFromKey(t *testing.T) {
	storage := &S3Storage{
		config: S3Config{
			Prefix: "arena/results",
		},
	}

	t.Run("extracts job ID from key", func(t *testing.T) {
		jobID := storage.jobIDFromKey("arena/results/test-job.json")
		if jobID != "test-job" {
			t.Errorf("jobIDFromKey() = %v, want test-job", jobID)
		}
	})

	t.Run("returns empty for non-json key", func(t *testing.T) {
		jobID := storage.jobIDFromKey("arena/results/test-job.txt")
		if jobID != "" {
			t.Errorf("jobIDFromKey() = %v, want empty", jobID)
		}
	})

	t.Run("handles prefix without trailing slash", func(t *testing.T) {
		jobID := storage.jobIDFromKey("arena/results/my-job.json")
		if jobID != "my-job" {
			t.Errorf("jobIDFromKey() = %v, want my-job", jobID)
		}
	})
}

func TestS3Storage_List_EmptyPrefix(t *testing.T) {
	mock := newMockS3Client()
	storage := &S3Storage{
		client: mock,
		config: S3Config{
			Bucket: "test-bucket",
			Region: "us-west-2",
			Prefix: "", // No prefix
		},
	}

	// Store a result directly
	data, _ := json.Marshal(&JobResults{JobID: "direct-job"})
	mock.objects["direct-job.json"] = data

	jobs, err := storage.List(context.Background(), "")
	if err != nil {
		t.Errorf("List() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0] != "direct-job" {
		t.Errorf("List() = %v, want [direct-job]", jobs)
	}
}

// Verify S3Storage implements both interfaces
var (
	_ ResultStorage         = (*S3Storage)(nil)
	_ ListableResultStorage = (*S3Storage)(nil)
)
