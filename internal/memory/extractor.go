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

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
)

// Redactor strips PII from text before memory storage.
// Implemented by ee/pkg/redaction.Redactor in production.
type Redactor interface {
	RedactText(ctx context.Context, text string) (string, error)
}

// OmniaExtractor bridges the flat Extractor interface to the richer MemoryPopulator.
// Called by PromptKit's extraction pipeline stage.
type OmniaExtractor struct {
	store     Store
	populator MemoryPopulator
	redactor  Redactor
	log       logr.Logger
}

// NewOmniaExtractor creates a new OmniaExtractor.
// If redactor is nil, no PII redaction is applied.
func NewOmniaExtractor(store Store, populator MemoryPopulator, redactor Redactor, log logr.Logger) *OmniaExtractor {
	return &OmniaExtractor{
		store:     store,
		populator: populator,
		redactor:  redactor,
		log:       log,
	}
}

// Extract derives memories from conversation messages.
// Internally delegates to MemoryPopulator.Populate() and saves results to the store.
// Returns flat []*Memory summary for pipeline telemetry.
func (e *OmniaExtractor) Extract(ctx context.Context, scope map[string]string, messages []types.Message) ([]*Memory, error) {
	source := PopulationSource{
		Scope:    scope,
		Messages: messages,
	}

	e.log.V(1).Info("extraction starting",
		"messageCount", len(messages),
		"sourceType", e.populator.SourceType())

	result, err := e.populator.Populate(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("memory: populate: %w", err)
	}

	if e.redactor != nil {
		e.redactResult(ctx, result)
	}

	entities := buildEntityIndex(result.Entities)
	saved, err := e.saveObservations(ctx, scope, result.Observations, entities)
	if err != nil {
		return nil, err
	}

	e.log.V(1).Info("extraction complete",
		"savedCount", len(saved),
		"observationCount", len(result.Observations))

	return saved, nil
}

// buildEntityIndex returns a map from entity name to EntityRecord.
func buildEntityIndex(entities []EntityRecord) map[string]EntityRecord {
	idx := make(map[string]EntityRecord, len(entities))
	for _, ent := range entities {
		idx[ent.Name] = ent
	}
	return idx
}

// saveObservations persists each observation as a Memory and returns the saved slice.
func (e *OmniaExtractor) saveObservations(
	ctx context.Context,
	scope map[string]string,
	observations []ObservationRecord,
	entities map[string]EntityRecord,
) ([]*Memory, error) {
	saved := make([]*Memory, 0, len(observations))
	for _, obs := range observations {
		mem := buildMemory(scope, obs, entities[obs.EntityName])
		if err := e.store.Save(ctx, mem); err != nil {
			return nil, fmt.Errorf("memory: save observation for %q: %w", obs.EntityName, err)
		}
		saved = append(saved, mem)
	}
	return saved, nil
}

// redactResult applies PII redaction to all entity names and observation contents in-place.
func (e *OmniaExtractor) redactResult(ctx context.Context, result *PopulationResult) {
	for i := range result.Entities {
		redacted, err := e.redactor.RedactText(ctx, result.Entities[i].Name)
		if err != nil {
			e.log.Error(err, "redaction failed for entity name",
				"entityName", result.Entities[i].Name)
			continue
		}
		result.Entities[i].Name = redacted
	}

	for i := range result.Observations {
		redacted, err := e.redactor.RedactText(ctx, result.Observations[i].Content)
		if err != nil {
			e.log.Error(err, "redaction failed for observation content",
				"entityName", result.Observations[i].EntityName)
			continue
		}
		result.Observations[i].Content = redacted
	}

	e.log.V(1).Info("redaction applied",
		"entityCount", len(result.Entities),
		"observationCount", len(result.Observations))
}

// buildMemory constructs a Memory from an observation and its parent entity.
func buildMemory(scope map[string]string, obs ObservationRecord, ent EntityRecord) *Memory {
	return &Memory{
		Type:       ent.Kind,
		Content:    obs.Content,
		Confidence: float64(obs.Confidence),
		Scope:      scope,
		SessionID:  obs.SessionID,
		Metadata:   ent.Metadata,
	}
}
