/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

// Package api provides the HTTP API layer for the memory-api service.
package api

import (
	"context"
	"errors"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/memory"
)

// Sentinel errors returned by the memory service and handler.
var (
	ErrMissingWorkspace = errors.New("workspace parameter is required")
	ErrMissingQuery     = errors.New("search query parameter is required")
	ErrMissingMemoryID  = errors.New("memory ID is required")
	ErrMissingBody      = errors.New("request body is required")
	ErrBodyTooLarge     = errors.New("request body too large")
)

// MemoryService wraps the memory store with business logic for the HTTP layer.
type MemoryService struct {
	store memory.Store
	log   logr.Logger
}

// NewMemoryService creates a new MemoryService backed by the given store.
func NewMemoryService(store memory.Store, log logr.Logger) *MemoryService {
	return &MemoryService{
		store: store,
		log:   log.WithName("memory-service"),
	}
}

// SaveMemory persists a memory entry.
func (s *MemoryService) SaveMemory(ctx context.Context, mem *memory.Memory) error {
	return s.store.Save(ctx, mem)
}

// SearchMemories retrieves memories matching a query and scope.
func (s *MemoryService) SearchMemories(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	return s.store.Retrieve(ctx, scope, query, opts)
}

// ListMemories returns memories for a given scope with pagination.
func (s *MemoryService) ListMemories(ctx context.Context, scope map[string]string, opts memory.ListOptions) ([]*memory.Memory, error) {
	return s.store.List(ctx, scope, opts)
}

// DeleteMemory performs a soft delete (forget) of a single memory.
func (s *MemoryService) DeleteMemory(ctx context.Context, scope map[string]string, memoryID string) error {
	return s.store.Delete(ctx, scope, memoryID)
}

// DeleteAllMemories hard-deletes all memories for the given scope (DSAR).
func (s *MemoryService) DeleteAllMemories(ctx context.Context, scope map[string]string) error {
	return s.store.DeleteAll(ctx, scope)
}
