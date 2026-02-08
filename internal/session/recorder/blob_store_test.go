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
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session/providers/cold"
)

func TestOmniaBlobStore_Store(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	data := []byte("hello world")
	payload, err := bs.Store(context.Background(), "sess-1", data, "text/plain")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if payload.StorageRef == "" {
		t.Error("expected non-empty StorageRef")
	}
	if payload.MIMEType != "text/plain" {
		t.Errorf("expected MIMEType text/plain, got %s", payload.MIMEType)
	}
	if payload.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), payload.Size)
	}
	if !strings.HasPrefix(payload.Checksum, "sha256:") {
		t.Errorf("expected sha256 checksum prefix, got %s", payload.Checksum)
	}

	// Verify data was stored in cold storage
	stored, err := coldBlob.Get(context.Background(), payload.StorageRef)
	if err != nil {
		t.Fatalf("cold store Get failed: %v", err)
	}
	if !bytes.Equal(stored, data) {
		t.Error("stored data doesn't match original")
	}

	// Verify artifact was saved to warm store
	if len(warmStore.artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(warmStore.artifacts))
	}
	artifact := warmStore.artifacts[0]
	if artifact.SessionID != "sess-1" {
		t.Errorf("expected session ID sess-1, got %s", artifact.SessionID)
	}
	if artifact.StorageURI != payload.StorageRef {
		t.Errorf("expected StorageURI %s, got %s", payload.StorageRef, artifact.StorageURI)
	}
}

func TestOmniaBlobStore_Store_KeyFormat(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	tests := []struct {
		mimeType     string
		wantCategory string
		wantExt      string
	}{
		{"image/png", "image", ".png"},
		{"audio/wav", "audio", ".wav"},
		{"video/mp4", "video", ".mp4"},
		{"application/octet-stream", "file", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			payload, err := bs.Store(context.Background(), "sess-1", []byte("test"), tt.mimeType)
			if err != nil {
				t.Fatalf("Store failed: %v", err)
			}

			// Key format: sessions/{sessionID}/artifacts/{category}/{hash}.{ext}
			if !strings.HasPrefix(payload.StorageRef, "sessions/sess-1/artifacts/"+tt.wantCategory+"/") {
				t.Errorf("key %s doesn't have expected prefix for category %s", payload.StorageRef, tt.wantCategory)
			}
			if !strings.HasSuffix(payload.StorageRef, tt.wantExt) {
				t.Errorf("key %s doesn't have expected extension %s", payload.StorageRef, tt.wantExt)
			}
		})
	}
}

func TestOmniaBlobStore_Load(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	data := []byte("binary data")
	payload, err := bs.Store(context.Background(), "sess-1", data, "application/octet-stream")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	loaded, err := bs.Load(context.Background(), payload.StorageRef)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !bytes.Equal(loaded, data) {
		t.Error("loaded data doesn't match original")
	}
}

func TestOmniaBlobStore_Delete(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	data := []byte("to be deleted")
	payload, err := bs.Store(context.Background(), "sess-1", data, "text/plain")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if err := bs.Delete(context.Background(), payload.StorageRef); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = bs.Load(context.Background(), payload.StorageRef)
	if err == nil {
		t.Error("expected error loading deleted blob")
	}
}

func TestOmniaBlobStore_StoreReader(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	data := []byte("from reader")
	r := bytes.NewReader(data)

	payload, err := bs.StoreReader(context.Background(), "sess-1", r, "text/plain")
	if err != nil {
		t.Fatalf("StoreReader failed: %v", err)
	}
	if payload.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), payload.Size)
	}

	loaded, err := bs.Load(context.Background(), payload.StorageRef)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !bytes.Equal(loaded, data) {
		t.Error("loaded data doesn't match original")
	}
}

func TestOmniaBlobStore_ColdFailure(t *testing.T) {
	coldBlob := &failingBlobStore{}
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	_, err := bs.Store(context.Background(), "sess-1", []byte("data"), "text/plain")
	if err == nil {
		t.Error("expected error from cold store failure")
	}
}

func TestOmniaBlobStore_DeleteFailure(t *testing.T) {
	coldBlob := &failingBlobStore{}
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	err := bs.Delete(context.Background(), "nonexistent-key")
	if err == nil {
		t.Error("expected error from cold store delete failure")
	}
}

func TestOmniaBlobStore_LoadFailure(t *testing.T) {
	coldBlob := &failingBlobStore{}
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	_, err := bs.Load(context.Background(), "nonexistent-key")
	if err == nil {
		t.Error("expected error from cold store load failure")
	}
}

func TestOmniaBlobStore_LoadReaderFailure(t *testing.T) {
	coldBlob := &failingBlobStore{}
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	_, err := bs.LoadReader(context.Background(), "nonexistent-key")
	if err == nil {
		t.Error("expected error from cold store load reader failure")
	}
}

func TestOmniaBlobStore_StoreReaderFailure(t *testing.T) {
	coldBlob := &failingBlobStore{}
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	_, err := bs.StoreReader(context.Background(), "sess-1", strings.NewReader("data"), "text/plain")
	if err == nil {
		t.Error("expected error from cold store failure via StoreReader")
	}
}

func TestOmniaBlobStore_Close(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())
	if err := bs.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestExtensionFromMIME(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"audio/wav", ".wav"},
		{"audio/wave", ".wav"},
		{"audio/x-wav", ".wav"},
		{"audio/mpeg", ".mp3"},
		{"audio/mp3", ".mp3"},
		{"audio/ogg", ".ogg"},
		{"audio/opus", ".opus"},
		{"audio/webm", ".webm"},
		{"audio/pcm", ".pcm"},
		{"audio/L16", ".pcm"},
		{"video/mp4", ".mp4"},
		{"video/webm", ".webm"},
		{"video/quicktime", ".mov"},
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"application/pdf", ".bin"},
		{"text/plain", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := extensionFromMIME(tt.mimeType)
			if got != tt.want {
				t.Errorf("extensionFromMIME(%s) = %s, want %s", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestOmniaBlobStore_LoadReader(t *testing.T) {
	coldBlob := cold.NewMemoryBlobStore()
	warmStore := newMockWarmStoreForTest()
	bs := NewOmniaBlobStore(coldBlob, warmStore, logr.Discard())

	data := []byte("reader test data")
	payload, err := bs.Store(context.Background(), "sess-1", data, "text/plain")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	rc, err := bs.LoadReader(context.Background(), payload.StorageRef)
	if err != nil {
		t.Fatalf("LoadReader failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("reader data doesn't match original")
	}
}
