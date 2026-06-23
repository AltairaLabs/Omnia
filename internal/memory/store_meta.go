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
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// stringFromMeta returns the trimmed lowercased value of meta[key]
// for the about_* keys (where consistent normalisation prevents
// silent dedup misses across casing or whitespace), and the raw
// trimmed value otherwise. Empty / missing → "".
func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, _ := meta[key].(string)
	v = strings.TrimSpace(v)
	if key == MetaKeyAboutKind || key == MetaKeyAboutKey {
		return strings.ToLower(v)
	}
	return v
}

// Trust-model and source-type values persisted on memory_entities by
// trustFromProvenance. Extracted as constants (SonarCloud/goconst S1192);
// agentScopedSourceType ("operator_curated") is reused from agent_scoped.go.
const (
	trustModelInferred        = "inferred"
	sourceTypeSystemGenerated = "system_generated"
	sourceTypeUserRequested   = "user_requested"
)

// trustFromProvenance maps a PromptKit provenance value to the
// (trust_model, source_type) pair persisted on memory_entities.
// Returns (nil, nil) when the caller didn't set a provenance so the
// schema-level defaults apply.
func trustFromProvenance(meta map[string]any) (trustModel, sourceType *string) {
	if meta == nil {
		return nil, nil
	}
	prov, _ := meta[pkmemory.MetaKeyProvenance].(string)
	switch prov {
	case string(pkmemory.ProvenanceUserRequested):
		tm, st := "explicit", sourceTypeUserRequested
		return &tm, &st
	case string(pkmemory.ProvenanceOperatorCurated):
		tm, st := "curated", agentScopedSourceType
		return &tm, &st
	case string(pkmemory.ProvenanceAgentExtracted):
		tm, st := trustModelInferred, "conversation_extraction"
		return &tm, &st
	case string(pkmemory.ProvenanceSystemGenerated):
		tm, st := trustModelInferred, sourceTypeSystemGenerated
		return &tm, &st
	default:
		return nil, nil
	}
}

// purposeFromMetadata returns a pointer to the Metadata[MetaKeyPurpose] value
// when set, or nil so the INSERT falls through to the schema default.
func purposeFromMetadata(meta map[string]any) *string {
	if meta == nil {
		return nil
	}
	v, ok := meta[MetaKeyPurpose].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

// consentCategoryFromMetadata reads MetaKeyConsentCategory from the
// memory metadata, returning nil when absent so the column stays NULL
// and the row falls under the default retention policy rather than a
// per-category override.
func consentCategoryFromMetadata(meta map[string]any) *string {
	if meta == nil {
		return nil
	}
	v, ok := meta[MetaKeyConsentCategory].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

// copyScope returns a shallow copy of the scope map.
func copyScope(scope map[string]string) map[string]string {
	out := make(map[string]string, len(scope))
	maps.Copy(out, scope)
	return out
}

// scopeOrNil returns a *string for the given scope key, or nil if absent.
func scopeOrNil(scope map[string]string, key string) *string {
	if v, ok := scope[key]; ok && v != "" {
		return &v
	}
	return nil
}

// marshalMetadata serializes metadata to JSON, defaulting to "{}".
func marshalMetadata(meta map[string]any) ([]byte, error) {
	if len(meta) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("memory: marshal metadata: %w", err)
	}
	return b, nil
}
