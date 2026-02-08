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

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GCSBlobStore implements BlobStore using Google Cloud Storage.
type GCSBlobStore struct {
	client *storage.Client
	bucket *storage.BucketHandle
}

// NewGCSBlobStore creates a new GCS-backed BlobStore.
func NewGCSBlobStore(ctx context.Context, bucket string, cfg GCSConfig) (*GCSBlobStore, error) {
	if bucket == "" {
		return nil, errors.New("bucket is required")
	}

	var opts []option.ClientOption
	if len(cfg.CredentialsJSON) > 0 {
		opts = append(opts, option.WithCredentialsJSON(cfg.CredentialsJSON))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSBlobStore{
		client: client,
		bucket: client.Bucket(bucket),
	}, nil
}

func (g *GCSBlobStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	w := g.bucket.Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("gcs put write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gcs put close: %w", err)
	}
	return nil
}

func (g *GCSBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	r, err := g.bucket.Object(key).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("gcs get: %w", err)
	}
	defer func() { _ = r.Close() }()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gcs read body: %w", err)
	}
	return data, nil
}

func (g *GCSBlobStore) Delete(ctx context.Context, key string) error {
	err := g.bucket.Object(key).Delete(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return ErrObjectNotFound
		}
		return fmt.Errorf("gcs delete: %w", err)
	}
	return nil
}

func (g *GCSBlobStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	it := g.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcs list: %w", err)
		}
		keys = append(keys, attrs.Name)
	}
	return keys, nil
}

func (g *GCSBlobStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := g.bucket.Object(key).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("gcs exists: %w", err)
	}
	return true, nil
}

func (g *GCSBlobStore) Ping(ctx context.Context) error {
	_, err := g.bucket.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("gcs ping: %w", err)
	}
	return nil
}

func (g *GCSBlobStore) Close() error {
	return g.client.Close()
}

var _ BlobStore = (*GCSBlobStore)(nil)
