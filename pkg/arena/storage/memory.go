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

package storage

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/altairalabs/omnia/pkg/arena/aggregator"
)

// MemoryStorage implements ResultStorage using in-memory data structures.
// It is suitable for development, testing, and single-node deployments.
// Data is not persisted and will be lost when the process exits.
type MemoryStorage struct {
	mu      sync.RWMutex
	closed  bool
	results map[string]*storedResult
}

// storedResult wraps JobResults with storage metadata.
type storedResult struct {
	data      *JobResults
	sizeBytes int64
}

// NewMemoryStorage creates a new in-memory result storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		results: make(map[string]*storedResult),
	}
}

// Store persists job results to memory.
func (s *MemoryStorage) Store(ctx context.Context, jobID string, results *JobResults) error {
	if jobID == "" {
		return ErrInvalidJobID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStorageClosed
	}

	// Calculate approximate size by serializing to JSON
	data, err := json.Marshal(results)
	if err != nil {
		return err
	}

	// Store a copy to prevent external modification
	resultsCopy := *results
	if results.Summary != nil {
		summaryCopy := *results.Summary
		resultsCopy.Summary = &summaryCopy
	}
	if results.Results != nil {
		resultsCopy.Results = make([]aggregator.ExecutionResult, len(results.Results))
		copy(resultsCopy.Results, results.Results)
	}
	if results.Metadata != nil {
		resultsCopy.Metadata = make(map[string]string)
		for k, v := range results.Metadata {
			resultsCopy.Metadata[k] = v
		}
	}

	s.results[jobID] = &storedResult{
		data:      &resultsCopy,
		sizeBytes: int64(len(data)),
	}

	return nil
}

// Get retrieves job results from memory.
func (s *MemoryStorage) Get(ctx context.Context, jobID string) (*JobResults, error) {
	if jobID == "" {
		return nil, ErrInvalidJobID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStorageClosed
	}

	stored, exists := s.results[jobID]
	if !exists {
		return nil, ErrResultNotFound
	}

	// Return a copy to prevent external modification
	resultsCopy := *stored.data
	if stored.data.Summary != nil {
		summaryCopy := *stored.data.Summary
		resultsCopy.Summary = &summaryCopy
	}
	if stored.data.Results != nil {
		resultsCopy.Results = make([]aggregator.ExecutionResult, len(stored.data.Results))
		copy(resultsCopy.Results, stored.data.Results)
	}
	if stored.data.Metadata != nil {
		resultsCopy.Metadata = make(map[string]string)
		for k, v := range stored.data.Metadata {
			resultsCopy.Metadata[k] = v
		}
	}

	return &resultsCopy, nil
}

// List returns job IDs that match the given prefix.
func (s *MemoryStorage) List(ctx context.Context, prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStorageClosed
	}

	var jobIDs []string
	for jobID := range s.results {
		if prefix == "" || strings.HasPrefix(jobID, prefix) {
			jobIDs = append(jobIDs, jobID)
		}
	}

	sort.Strings(jobIDs)
	return jobIDs, nil
}

// Delete removes job results from memory.
func (s *MemoryStorage) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return ErrInvalidJobID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStorageClosed
	}

	if _, exists := s.results[jobID]; !exists {
		return ErrResultNotFound
	}

	delete(s.results, jobID)
	return nil
}

// Close releases resources and marks the storage as closed.
func (s *MemoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.results = nil
	return nil
}

// ListWithInfo returns result metadata for jobs matching the prefix.
func (s *MemoryStorage) ListWithInfo(ctx context.Context, prefix string) ([]ResultInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStorageClosed
	}

	var infos []ResultInfo
	for jobID, stored := range s.results {
		if prefix == "" || strings.HasPrefix(jobID, prefix) {
			info := ResultInfo{
				JobID:       jobID,
				Namespace:   stored.data.Namespace,
				CompletedAt: stored.data.CompletedAt,
				SizeBytes:   stored.sizeBytes,
			}
			if stored.data.Summary != nil {
				info.TotalItems = stored.data.Summary.TotalItems
				info.PassedItems = stored.data.Summary.PassedItems
				info.FailedItems = stored.data.Summary.FailedItems
			}
			infos = append(infos, info)
		}
	}

	// Sort by job ID for consistent ordering
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].JobID < infos[j].JobID
	})

	return infos, nil
}

// Ensure MemoryStorage implements both interfaces.
var (
	_ ResultStorage         = (*MemoryStorage)(nil)
	_ ListableResultStorage = (*MemoryStorage)(nil)
)
