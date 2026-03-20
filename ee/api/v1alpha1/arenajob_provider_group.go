/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import "encoding/json"

// ArenaProviderGroup is a polymorphic provider group value.
// Array mode: []ArenaProviderEntry (test provider pools).
// Map mode: map[string]ArenaProviderEntry (1:1 config-provider-ID → CRD mappings).
// JSON auto-detects: arrays → Entries, objects → Mapping.
//
// +kubebuilder:validation:Type=""
// +kubebuilder:pruning:PreserveUnknownFields
type ArenaProviderGroup struct {
	Entries []ArenaProviderEntry          `json:"-"`
	Mapping map[string]ArenaProviderEntry `json:"-"`
}

// UnmarshalJSON detects array vs object and populates the correct field.
func (g *ArenaProviderGroup) UnmarshalJSON(data []byte) error {
	// Peek at the first non-whitespace byte
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '[':
			return json.Unmarshal(data, &g.Entries)
		case '{':
			return json.Unmarshal(data, &g.Mapping)
		default:
			return json.Unmarshal(data, &g.Entries)
		}
	}
	return nil
}

// MarshalJSON serialises as object (map mode) or array (entries mode).
func (g ArenaProviderGroup) MarshalJSON() ([]byte, error) {
	if g.Mapping != nil {
		return json.Marshal(g.Mapping)
	}
	return json.Marshal(g.Entries)
}

// IsMapMode returns true when the group uses 1:1 config-provider-ID mapping.
func (g *ArenaProviderGroup) IsMapMode() bool {
	return g.Mapping != nil
}

// AllEntries returns every ArenaProviderEntry regardless of mode.
func (g *ArenaProviderGroup) AllEntries() []ArenaProviderEntry {
	if g.Mapping != nil {
		entries := make([]ArenaProviderEntry, 0, len(g.Mapping))
		for _, e := range g.Mapping {
			entries = append(entries, e)
		}
		return entries
	}
	return g.Entries
}

// Len returns the number of entries regardless of mode.
func (g *ArenaProviderGroup) Len() int {
	if g.Mapping != nil {
		return len(g.Mapping)
	}
	return len(g.Entries)
}

// DeepCopyInto copies the receiver into out. Manual implementation because
// controller-gen cannot handle json:"-" fields.
func (g *ArenaProviderGroup) DeepCopyInto(out *ArenaProviderGroup) {
	if g.Entries != nil {
		out.Entries = make([]ArenaProviderEntry, len(g.Entries))
		for i := range g.Entries {
			g.Entries[i].DeepCopyInto(&out.Entries[i])
		}
	}
	if g.Mapping != nil {
		out.Mapping = make(map[string]ArenaProviderEntry, len(g.Mapping))
		for k, v := range g.Mapping {
			var entry ArenaProviderEntry
			v.DeepCopyInto(&entry)
			out.Mapping[k] = entry
		}
	}
}

// DeepCopy creates a deep copy of ArenaProviderGroup.
func (g *ArenaProviderGroup) DeepCopy() *ArenaProviderGroup {
	if g == nil {
		return nil
	}
	out := new(ArenaProviderGroup)
	g.DeepCopyInto(out)
	return out
}
