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

// Package recorder bridges PromptKit's event/blob system with Omnia's tiered
// session storage (warm store for text, cold archive for binary artifacts).
package recorder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
)

// OmniaBlobStore implements the PromptKit events.BlobStore interface backed by
// Omnia's cold.BlobStore for binary data and WarmStoreProvider for artifact
// reference tracking.
type OmniaBlobStore struct {
	coldBlob  cold.BlobStore
	warmStore providers.WarmStoreProvider
	log       logr.Logger
}

// NewOmniaBlobStore creates a new blob store bridge.
func NewOmniaBlobStore(
	coldBlob cold.BlobStore,
	warmStore providers.WarmStoreProvider,
	log logr.Logger,
) *OmniaBlobStore {
	return &OmniaBlobStore{
		coldBlob:  coldBlob,
		warmStore: warmStore,
		log:       log.WithName("omnia-blob-store"),
	}
}

// Store saves binary data to cold storage and records an artifact reference
// in the warm store. It returns a BinaryPayload with the storage reference.
func (s *OmniaBlobStore) Store(
	ctx context.Context,
	sessionID string,
	data []byte,
	mimeType string,
) (*events.BinaryPayload, error) {
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])
	ext := extensionFromMIME(mimeType)
	category := categoryFromMIME(mimeType)

	key := fmt.Sprintf("sessions/%s/artifacts/%s/%s%s", sessionID, category, hashStr, ext)

	if err := s.coldBlob.Put(ctx, key, data, mimeType); err != nil {
		return nil, fmt.Errorf("cold store put: %w", err)
	}

	artifact := &session.Artifact{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		MessageID:  sessionID, // Will be updated by caller when message ID is known
		Type:       category,
		MIMEType:   mimeType,
		StorageURI: key,
		SizeBytes:  int64(len(data)),
		Checksum:   "sha256:" + hashStr,
		CreatedAt:  time.Now(),
	}
	if err := s.warmStore.SaveArtifact(ctx, artifact); err != nil {
		s.log.Error(err, "failed to save artifact reference", "key", key)
		// Non-fatal: the binary data is already stored
	}

	return &events.BinaryPayload{
		StorageRef: key,
		MIMEType:   mimeType,
		Size:       int64(len(data)),
		Checksum:   "sha256:" + hashStr,
	}, nil
}

// StoreReader reads all data from the reader and delegates to Store.
func (s *OmniaBlobStore) StoreReader(
	ctx context.Context,
	sessionID string,
	r io.Reader,
	mimeType string,
) (*events.BinaryPayload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read blob data: %w", err)
	}
	return s.Store(ctx, sessionID, data, mimeType)
}

// Load retrieves binary data from cold storage by key.
func (s *OmniaBlobStore) Load(ctx context.Context, ref string) ([]byte, error) {
	data, err := s.coldBlob.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("cold store get: %w", err)
	}
	return data, nil
}

// LoadReader returns an io.ReadCloser wrapping the data from cold storage.
func (s *OmniaBlobStore) LoadReader(ctx context.Context, ref string) (io.ReadCloser, error) {
	data, err := s.Load(ctx, ref)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(io.LimitReader(
		&bytesReader{data: data},
		int64(len(data)),
	)), nil
}

// Delete removes binary data from cold storage.
func (s *OmniaBlobStore) Delete(ctx context.Context, ref string) error {
	if err := s.coldBlob.Delete(ctx, ref); err != nil {
		return fmt.Errorf("cold store delete: %w", err)
	}
	return nil
}

// Close is a no-op; the underlying stores are managed by the Registry.
func (s *OmniaBlobStore) Close() error {
	return nil
}

// bytesReader is a simple io.Reader backed by a byte slice.
type bytesReader struct {
	data []byte
	off  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

// extensionFromMIME returns a file extension for common MIME types.
func extensionFromMIME(mimeType string) string {
	switch mimeType {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "audio/opus":
		return ".opus"
	case "audio/webm":
		return ".webm"
	case "audio/pcm", "audio/L16":
		return ".pcm"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

// Artifact category constants.
const (
	categoryAudio = "audio"
	categoryVideo = "video"
	categoryImage = "image"
	categoryFile  = "file"
)

// categoryFromMIME returns an artifact category from the MIME type.
func categoryFromMIME(mimeType string) string {
	if len(mimeType) > 6 && mimeType[:6] == "audio/" {
		return categoryAudio
	}
	if len(mimeType) > 6 && mimeType[:6] == "video/" {
		return categoryVideo
	}
	if len(mimeType) > 6 && mimeType[:6] == "image/" {
		return categoryImage
	}
	return categoryFile
}

// Ensure OmniaBlobStore implements events.BlobStore.
var _ events.BlobStore = (*OmniaBlobStore)(nil)
