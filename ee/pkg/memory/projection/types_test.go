/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPointJSONTags(t *testing.T) {
	p := Point{ID: "e1", X: 0.5, Y: -0.5, Tier: "user", Preview: "hi"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"id":"e1"`, `"x":0.5`, `"y":-0.5`, `"tier":"user"`, `"preview":"hi"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	// omitempty: empty User key must be absent (note: "user" also appears as the
	// tier value, so match the key form `"user":`).
	if strings.Contains(s, `"user":`) {
		t.Errorf("empty user should be omitted: %s", s)
	}
}

func TestPoint_MaskedOmitsIdentifyingFields(t *testing.T) {
	// A masked point has its identifying/content fields zeroed; the JSON must
	// omit them entirely (strip-before-serialize), keeping only safe metadata.
	masked := Point{X: 0.4, Y: -0.1, Tier: "user", Confidence: 0.9, Masked: true}
	b, err := json.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	// Match the key form `"x":` — the bare value "user" also appears as the tier.
	for _, key := range []string{`"id":`, `"preview":`, `"title":`, `"user":`, `"userRef":`, `"category":`, `"type":`} {
		if strings.Contains(got, key) {
			t.Errorf("masked JSON must not contain %s, got %s", key, got)
		}
	}
	if !strings.Contains(got, `"masked":true`) {
		t.Errorf("masked JSON must contain masked:true, got %s", got)
	}
}

func TestPoint_UnmaskedKeepsFields(t *testing.T) {
	// A normal point still serializes id and preview.
	p := Point{ID: "e1", X: 0.1, Y: 0.2, Tier: "user", Preview: "hello", Confidence: 0.5}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"id":"e1"`) || !strings.Contains(got, `"preview":"hello"`) {
		t.Errorf("unmasked JSON must contain id and preview, got %s", got)
	}
	if strings.Contains(got, `"masked"`) {
		t.Errorf("unmasked JSON must omit masked, got %s", got)
	}
}
