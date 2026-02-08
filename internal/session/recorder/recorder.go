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
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
)

// SessionRecorder orchestrates event and blob storage for a single session.
// It creates the underlying OmniaEventStore and OmniaBlobStore and provides
// access to them for integration with PromptKit's EventBus.
type SessionRecorder struct {
	eventStore *OmniaEventStore
	blobStore  *OmniaBlobStore
	sessionID  string
	warmStore  providers.WarmStoreProvider
	log        logr.Logger
}

// NewSessionRecorder creates a recorder for the given session. If coldBlob is
// nil, binary artifact storage is disabled (events with binary payloads will
// only record metadata).
func NewSessionRecorder(
	sessionID string,
	warmStore providers.WarmStoreProvider,
	coldBlob cold.BlobStore,
	log logr.Logger,
) *SessionRecorder {
	recLog := log.WithName("session-recorder").WithValues("sessionID", sessionID)

	var blobStore *OmniaBlobStore
	if coldBlob != nil {
		blobStore = NewOmniaBlobStore(coldBlob, warmStore, recLog)
	}

	eventStore := NewOmniaEventStore(warmStore, blobStore, recLog)

	return &SessionRecorder{
		eventStore: eventStore,
		blobStore:  blobStore,
		sessionID:  sessionID,
		warmStore:  warmStore,
		log:        recLog,
	}
}

// EventStore returns the PromptKit EventStore implementation for use with
// EventBus.WithStore().
func (r *SessionRecorder) EventStore() events.EventStore {
	return r.eventStore
}

// BlobStore returns the PromptKit BlobStore implementation, or nil if cold
// storage is not configured.
func (r *SessionRecorder) BlobStore() events.BlobStore {
	if r.blobStore == nil {
		return nil
	}
	return r.blobStore
}

// Cleanup removes all artifacts for the session from both cold storage and
// the warm store artifact references. Use this when aborting a session.
func (r *SessionRecorder) Cleanup(ctx context.Context) error {
	if r.warmStore == nil {
		return nil
	}

	artifacts, err := r.warmStore.GetSessionArtifacts(ctx, r.sessionID)
	if err != nil {
		return fmt.Errorf("get session artifacts: %w", err)
	}

	var lastErr error
	for _, artifact := range artifacts {
		if r.blobStore != nil {
			if err := r.blobStore.Delete(ctx, artifact.StorageURI); err != nil {
				r.log.Error(err, "failed to delete artifact from cold storage",
					"artifactID", artifact.ID, "storageURI", artifact.StorageURI)
				lastErr = err
			}
		}
	}

	if err := r.warmStore.DeleteSessionArtifacts(ctx, r.sessionID); err != nil {
		r.log.Error(err, "failed to delete artifact references")
		lastErr = err
	}

	return lastErr
}

// Close releases resources. Currently a no-op since the underlying stores
// are managed by the provider Registry.
func (r *SessionRecorder) Close() error {
	return nil
}
