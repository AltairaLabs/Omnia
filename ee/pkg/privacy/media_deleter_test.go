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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockObjectStoreClient is a test double for ObjectStoreClient.
type MockObjectStoreClient struct {
	ListKeys  []string
	ListErr   error
	DeleteErr error
	// Captured calls for assertions.
	DeletedBucket string
	DeletedKeys   []string
	ListedBucket  string
	ListedPrefix  string
}

func (m *MockObjectStoreClient) ListObjects(_ context.Context, bucket, prefix string) ([]string, error) {
	m.ListedBucket = bucket
	m.ListedPrefix = prefix
	return m.ListKeys, m.ListErr
}

func (m *MockObjectStoreClient) DeleteObjects(_ context.Context, bucket string, keys []string) error {
	m.DeletedBucket = bucket
	m.DeletedKeys = keys
	return m.DeleteErr
}

func TestObjectStoreMediaDeleter_Success(t *testing.T) {
	client := &MockObjectStoreClient{
		ListKeys: []string{"sessions/sess-1/image.png", "sessions/sess-1/audio.wav"},
	}
	deleter := NewObjectStoreMediaDeleter(client, "my-bucket", "sessions/", logr.Discard())

	err := deleter.DeleteSessionMedia(context.Background(), "sess-1")
	require.NoError(t, err)

	assert.Equal(t, "my-bucket", client.ListedBucket)
	assert.Equal(t, "sessions/sess-1/", client.ListedPrefix)
	assert.Equal(t, "my-bucket", client.DeletedBucket)
	assert.Equal(t, []string{"sessions/sess-1/image.png", "sessions/sess-1/audio.wav"}, client.DeletedKeys)
}

func TestObjectStoreMediaDeleter_NoArtifacts(t *testing.T) {
	client := &MockObjectStoreClient{
		ListKeys: []string{},
	}
	deleter := NewObjectStoreMediaDeleter(client, "my-bucket", "sessions/", logr.Discard())

	err := deleter.DeleteSessionMedia(context.Background(), "sess-1")
	require.NoError(t, err)

	// DeleteObjects should not be called when there are no keys.
	assert.Empty(t, client.DeletedKeys)
}

func TestObjectStoreMediaDeleter_NilKeys(t *testing.T) {
	client := &MockObjectStoreClient{
		ListKeys: nil,
	}
	deleter := NewObjectStoreMediaDeleter(client, "my-bucket", "sessions/", logr.Discard())

	err := deleter.DeleteSessionMedia(context.Background(), "sess-1")
	require.NoError(t, err)
	assert.Empty(t, client.DeletedKeys)
}

func TestObjectStoreMediaDeleter_ListError(t *testing.T) {
	client := &MockObjectStoreClient{
		ListErr: errors.New("access denied"),
	}
	deleter := NewObjectStoreMediaDeleter(client, "my-bucket", "sessions/", logr.Discard())

	err := deleter.DeleteSessionMedia(context.Background(), "sess-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listing media objects")
}

func TestObjectStoreMediaDeleter_DeleteError(t *testing.T) {
	client := &MockObjectStoreClient{
		ListKeys:  []string{"sessions/sess-1/file.txt"},
		DeleteErr: errors.New("storage unavailable"),
	}
	deleter := NewObjectStoreMediaDeleter(client, "my-bucket", "sessions/", logr.Discard())

	err := deleter.DeleteSessionMedia(context.Background(), "sess-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deleting media objects")
}

func TestObjectStoreMediaDeleter_PrefixConstruction(t *testing.T) {
	client := &MockObjectStoreClient{ListKeys: nil}
	deleter := NewObjectStoreMediaDeleter(client, "bucket", "media/", logr.Discard())

	_ = deleter.DeleteSessionMedia(context.Background(), "abc-123")
	assert.Equal(t, "media/abc-123/", client.ListedPrefix)
}

func TestNoOpMediaDeleter_AlwaysReturnsNil(t *testing.T) {
	deleter := NoOpMediaDeleter{}
	err := deleter.DeleteSessionMedia(context.Background(), "any-session")
	assert.NoError(t, err)
}

func TestNewObjectStoreMediaDeleter(t *testing.T) {
	client := &MockObjectStoreClient{}
	deleter := NewObjectStoreMediaDeleter(client, "bucket", "prefix/", logr.Discard())
	assert.NotNil(t, deleter)
	assert.Equal(t, "bucket", deleter.bucket)
	assert.Equal(t, "prefix/", deleter.prefix)
}
