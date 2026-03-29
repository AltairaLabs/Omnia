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

// Package memory provides the PostgreSQL-backed memory store for entity-relation-observation
// memory graphs. Types here mirror the PromptKit memory.Store interface and will be replaced
// with direct imports once the PromptKit memory package is published.
package memory

import (
	"context"
	"time"
)

// Memory represents a single memory entry composed of an entity and its latest observation.
// TODO: replace with github.com/AltairaLabs/PromptKit/runtime/memory.Memory when published.
type Memory struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Content    string            `json:"content"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Confidence float64           `json:"confidence"`
	Scope      map[string]string `json:"scope"`
	SessionID  string            `json:"session_id,omitempty"`
	TurnRange  [2]int            `json:"turn_range,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	AccessedAt time.Time         `json:"accessed_at"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
}

// RetrieveOptions controls filtering and pagination for Retrieve calls.
type RetrieveOptions struct {
	Types         []string
	Limit         int
	MinConfidence float64
	Purpose       string
}

// ListOptions controls filtering and pagination for List calls.
type ListOptions struct {
	Types   []string
	Limit   int
	Offset  int
	Purpose string
}

// Store defines the interface for memory persistence.
// TODO: replace with github.com/AltairaLabs/PromptKit/runtime/memory.Store when published.
type Store interface {
	Save(ctx context.Context, mem *Memory) error
	Retrieve(ctx context.Context, scope map[string]string, query string, opts RetrieveOptions) ([]*Memory, error)
	List(ctx context.Context, scope map[string]string, opts ListOptions) ([]*Memory, error)
	Delete(ctx context.Context, scope map[string]string, memoryID string) error
	DeleteAll(ctx context.Context, scope map[string]string) error
	ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error)
}
