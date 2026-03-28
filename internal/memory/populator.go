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

package memory

import "context"

// MemoryPopulator produces entities, relations, and observations from a source.
// Different agent domains ship different populators. The store and governance are shared.
type MemoryPopulator interface {
	Populate(ctx context.Context, source PopulationSource) (*PopulationResult, error)
	SourceType() string // conversation_extraction | lsp | schema_analysis | operator_curated
	TrustModel() string // deterministic | curated | inferred
}

// PopulationSource carries the input data for a populator.
type PopulationSource struct {
	Scope    map[string]string
	Messages []SimpleMessage
	Data     map[string]any
}

// SimpleMessage is a minimal message representation for population sources.
// Avoids dependency on PromptKit types.
type SimpleMessage struct {
	Role    string
	Content string
}

// PopulationResult holds the extracted entities, relations, and observations.
type PopulationResult struct {
	Entities     []EntityRecord
	Relations    []RelationRecord
	Observations []ObservationRecord
}

// EntityRecord represents an extracted entity.
type EntityRecord struct {
	Name     string
	Kind     string
	Metadata map[string]any
}

// RelationRecord represents a relationship between two entities.
type RelationRecord struct {
	SourceName   string
	TargetName   string
	RelationType string
	Weight       float32
}

// ObservationRecord represents an observation about an entity.
type ObservationRecord struct {
	EntityName string
	Content    string
	Confidence float32
	SessionID  string
}
