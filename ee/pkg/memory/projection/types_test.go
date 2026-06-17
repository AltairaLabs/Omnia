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
