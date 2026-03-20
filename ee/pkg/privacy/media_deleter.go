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

	"github.com/go-logr/logr"
)

// MediaDeleter removes media artifacts associated with sessions.
type MediaDeleter interface {
	// DeleteSessionMedia removes all media artifacts for the given session.
	DeleteSessionMedia(ctx context.Context, sessionID string) error
}

// ObjectStoreClient abstracts the object storage operations needed for deletion.
type ObjectStoreClient interface {
	ListObjects(ctx context.Context, bucket, prefix string) ([]string, error)
	DeleteObjects(ctx context.Context, bucket string, keys []string) error
}

// ObjectStoreMediaDeleter deletes media from object storage (S3/GCS/Azure).
type ObjectStoreMediaDeleter struct {
	bucket string
	prefix string
	client ObjectStoreClient
	log    logr.Logger
}

// NewObjectStoreMediaDeleter creates a MediaDeleter backed by object storage.
func NewObjectStoreMediaDeleter(
	client ObjectStoreClient, bucket, prefix string, log logr.Logger,
) *ObjectStoreMediaDeleter {
	return &ObjectStoreMediaDeleter{
		bucket: bucket,
		prefix: prefix,
		client: client,
		log:    log.WithName("media-deleter"),
	}
}

// DeleteSessionMedia lists and deletes all objects under the session prefix.
func (d *ObjectStoreMediaDeleter) DeleteSessionMedia(ctx context.Context, sessionID string) error {
	objectPrefix := d.prefix + sessionID + "/"

	keys, err := d.client.ListObjects(ctx, d.bucket, objectPrefix)
	if err != nil {
		return fmt.Errorf("listing media objects: %w", err)
	}

	if len(keys) == 0 {
		d.log.V(1).Info("no media artifacts found", "sessionID", sessionID)
		return nil
	}

	if err := d.client.DeleteObjects(ctx, d.bucket, keys); err != nil {
		return fmt.Errorf("deleting media objects: %w", err)
	}

	d.log.V(1).Info("media artifacts deleted",
		"sessionID", sessionID,
		"objectCount", len(keys),
	)
	return nil
}

// NoOpMediaDeleter is used when no media storage is configured.
type NoOpMediaDeleter struct{}

// DeleteSessionMedia is a no-op that always returns nil.
func (NoOpMediaDeleter) DeleteSessionMedia(_ context.Context, _ string) error {
	return nil
}
