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
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Manifest is the JSON index stored at {prefix}_manifest.json.
// It enables O(1) session lookups and fast date listing without
// scanning the object store.
type Manifest struct {
	// Version tracks the manifest schema version.
	Version int `json:"version"`
	// UpdatedAt is when the manifest was last written.
	UpdatedAt time.Time `json:"updatedAt"`
	// Dates lists the dates for which archived data exists.
	Dates []DateEntry `json:"dates"`
	// SessionIndex maps session IDs to their file keys for O(1) lookups.
	SessionIndex map[string]string `json:"sessionIndex"`
}

// DateEntry records metadata about a single date partition.
type DateEntry struct {
	// Date is the calendar date (UTC, truncated to midnight).
	Date time.Time `json:"date"`
	// FileCount is the number of Parquet files for this date.
	FileCount int `json:"fileCount"`
	// SessionCount is the number of sessions archived on this date.
	SessionCount int `json:"sessionCount"`
}

// manifestKey returns the object key for the manifest file.
func manifestKey(prefix string) string {
	return prefix + "_manifest.json"
}

// newManifest returns an empty manifest.
func newManifest() *Manifest {
	return &Manifest{
		Version:      1,
		SessionIndex: make(map[string]string),
	}
}

// readManifest loads the manifest from the store. Returns an empty manifest
// if the manifest does not exist yet.
func readManifest(ctx context.Context, store BlobStore, prefix string) (*Manifest, error) {
	data, err := store.Get(ctx, manifestKey(prefix))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return newManifest(), nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	if m.SessionIndex == nil {
		m.SessionIndex = make(map[string]string)
	}
	return &m, nil
}

// writeManifest persists the manifest to the store.
func writeManifest(ctx context.Context, store BlobStore, prefix string, m *Manifest) error {
	m.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return store.Put(ctx, manifestKey(prefix), data, "application/json")
}

// updateManifest performs a read-modify-write on the manifest.
func updateManifest(ctx context.Context, store BlobStore, prefix string, fn func(*Manifest)) error {
	m, err := readManifest(ctx, store, prefix)
	if err != nil {
		return err
	}
	fn(m)
	return writeManifest(ctx, store, prefix, m)
}
