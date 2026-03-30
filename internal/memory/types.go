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
// memory graphs. Core types (Memory, Store, RetrieveOptions, ListOptions, Extractor, Retriever)
// are re-exported from github.com/AltairaLabs/PromptKit/runtime/memory.
package memory

import (
	"context"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// Re-export PromptKit memory types so existing callers can continue to use memory.Memory, etc.
type (
	Memory          = pkmemory.Memory
	RetrieveOptions = pkmemory.RetrieveOptions
	ListOptions     = pkmemory.ListOptions
)

// Store extends the PromptKit memory.Store interface with Omnia-specific methods.
// ExportAll is needed for DSAR (data subject access request) data export and is
// not part of the PromptKit SDK contract.
type Store interface {
	pkmemory.Store
	ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error)
}
