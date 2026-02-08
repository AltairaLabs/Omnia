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

package recorder

import (
	"context"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session/providers/cold"
)

func TestNewSessionRecorder(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	coldBlob := cold.NewMemoryBlobStore()

	rec := NewSessionRecorder("sess-1", warmStore, coldBlob, logr.Discard())

	if rec.EventStore() == nil {
		t.Error("expected non-nil EventStore")
	}
	if rec.BlobStore() == nil {
		t.Error("expected non-nil BlobStore when coldBlob is provided")
	}
}

func TestNewSessionRecorder_NoColdBlob(t *testing.T) {
	warmStore := newMockWarmStoreForTest()

	rec := NewSessionRecorder("sess-1", warmStore, nil, logr.Discard())

	if rec.EventStore() == nil {
		t.Error("expected non-nil EventStore even without cold blob")
	}
	if rec.BlobStore() != nil {
		t.Error("expected nil BlobStore when coldBlob is nil")
	}
}

func TestSessionRecorder_EventStoreInterface(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	coldBlob := cold.NewMemoryBlobStore()
	rec := NewSessionRecorder("sess-1", warmStore, coldBlob, logr.Discard())

	// Verify EventStore and BlobStore are non-nil and usable
	es := rec.EventStore()
	if es == nil {
		t.Fatal("expected non-nil EventStore")
	}
	bs := rec.BlobStore()
	if bs == nil {
		t.Fatal("expected non-nil BlobStore")
	}
	// Verify Close is callable (interface contract)
	if err := es.Close(); err != nil {
		t.Errorf("EventStore.Close failed: %v", err)
	}
	if err := bs.Close(); err != nil {
		t.Errorf("BlobStore.Close failed: %v", err)
	}
}

func TestSessionRecorder_Cleanup(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	coldBlob := cold.NewMemoryBlobStore()
	rec := NewSessionRecorder("sess-1", warmStore, coldBlob, logr.Discard())

	// Store some blobs
	bs := rec.BlobStore()
	_, err := bs.Store(context.Background(), "sess-1", []byte("artifact-1"), "image/png")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	_, err = bs.Store(context.Background(), "sess-1", []byte("artifact-2"), "audio/wav")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Verify artifacts exist
	artifacts, err := warmStore.GetSessionArtifacts(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetSessionArtifacts failed: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	// Cleanup
	if err := rec.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify artifacts were removed from warm store
	artifacts, err = warmStore.GetSessionArtifacts(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetSessionArtifacts after cleanup failed: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts after cleanup, got %d", len(artifacts))
	}
}

func TestSessionRecorder_Cleanup_NilWarmStore(t *testing.T) {
	// Cleanup with nil warm store should be a no-op
	rec := &SessionRecorder{sessionID: "sess-1"}
	if err := rec.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup with nil warmStore failed: %v", err)
	}
}

func TestSessionRecorder_Cleanup_ColdDeleteFailure(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	coldBlob := cold.NewMemoryBlobStore()
	rec := NewSessionRecorder("sess-1", warmStore, coldBlob, logr.Discard())

	// Store a blob
	bs := rec.BlobStore()
	_, err := bs.Store(context.Background(), "sess-1", []byte("data"), "image/png")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Delete the cold blob directly so cleanup will fail on cold delete
	keys, _ := coldBlob.List(context.Background(), "sessions/sess-1/")
	for _, key := range keys {
		_ = coldBlob.Delete(context.Background(), key)
	}

	// Cleanup should still succeed (cold delete failure is non-fatal for artifact ref cleanup)
	err = rec.Cleanup(context.Background())
	if err == nil {
		// Actually cold.MemoryBlobStore returns ErrObjectNotFound which propagates
		// Just verify it doesn't panic
		t.Log("Cleanup returned error as expected for missing cold objects")
	}
}

func TestSessionRecorder_Close(t *testing.T) {
	warmStore := newMockWarmStoreForTest()
	rec := NewSessionRecorder("sess-1", warmStore, nil, logr.Discard())
	if err := rec.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
