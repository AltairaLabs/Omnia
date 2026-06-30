/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projection

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// tierUser is the memory tier used across these fixtures.
const tierUser = "user"

func inputs(n int) []Input {
	out := make([]Input, n)
	for i := 0; i < n; i++ {
		out[i] = Input{
			EntityID:   fmt.Sprintf("e%04d", i),
			Content:    strings.Repeat("word ", 40),
			Tier:       tierUser,
			User:       "u1",
			Kind:       "profile",
			Confidence: 0.5,
			ObservedAt: time.Unix(int64(i), 0).UTC(),
		}
	}
	return out
}

func TestPointJSONTags(t *testing.T) {
	p := Point{ID: "e1", X: 0.5, Y: -0.5, Tier: tierUser, Preview: "hi"}
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
	masked := Point{X: 0.4, Y: -0.1, Tier: tierUser, Confidence: 0.9, Masked: true}
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
	p := Point{ID: "e1", X: 0.1, Y: 0.2, Tier: tierUser, Preview: "hello", Confidence: 0.5}
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

func TestFromStored_ReusesCoordsRefreshesMeta(t *testing.T) {
	in := inputs(4)
	stored := map[string][2]float64{
		in[0].EntityID: {0.1, 0.2},
		in[1].EntityID: {0.3, 0.4},
		in[2].EntityID: {0.5, 0.6},
		// in[3] has no stored coord → dropped
	}
	res := FromStored(in, stored, Options{})
	if res.Total != 3 || len(res.Points) != 3 {
		t.Fatalf("total/points = %d/%d, want 3/3", res.Total, len(res.Points))
	}
	if res.Points[0].X != 0.1 || res.Points[0].Y != 0.2 {
		t.Errorf("coords not reused: %v", res.Points[0])
	}
	if res.Points[0].Preview == "" {
		t.Error("preview should be refreshed from input content")
	}
}

func TestFromStored_EmptyWhenNoStoredCoords(t *testing.T) {
	res := FromStored(inputs(3), map[string][2]float64{}, Options{})
	if res.Total != 0 || len(res.Points) != 0 || res.Points == nil {
		t.Errorf("empty stored: total=%d points=%v", res.Total, res.Points)
	}
}

func TestFromStored_TruncatesPreview(t *testing.T) {
	in := inputs(1)
	in[0].Content = strings.Repeat("x", 500)
	res := FromStored(in, map[string][2]float64{in[0].EntityID: {0, 0}}, Options{PreviewChars: 10})
	if got := len([]rune(res.Points[0].Preview)); got != 10 {
		t.Errorf("preview length = %d, want 10", got)
	}
}

func TestOptions_WithDefaults(t *testing.T) {
	o := (Options{}).WithDefaults()
	if o.Cap != defaultRenderCap {
		t.Errorf("Cap = %d, want %d", o.Cap, defaultRenderCap)
	}
	if o.DenseThreshold != DefaultDenseThreshold {
		t.Errorf("DenseThreshold = %v, want %v", o.DenseThreshold, DefaultDenseThreshold)
	}
	if o.PCADims != 50 || o.LSADims != 50 || o.PreviewChars != 120 || o.TinySet != 30 {
		t.Errorf("unexpected defaults: %+v", o)
	}
	// Explicit values are preserved.
	custom := Options{Cap: 5, DenseThreshold: 0.1, PCADims: 7, LSADims: 8, PreviewChars: 9, TinySet: 3}
	if custom.WithDefaults() != custom {
		t.Errorf("WithDefaults overwrote explicit values: %+v", custom.WithDefaults())
	}
}

// TestOptions_DefaultRenderCapIsBounded guards the cap that keeps the EXACT
// O(n²) t-SNE backend tractable: a regression back to a large default (e.g.
// 8000) reintroduces multi-minute, ~GB renders.
func TestOptions_DefaultRenderCapIsBounded(t *testing.T) {
	got := (Options{}).WithDefaults().Cap
	if got != defaultRenderCap {
		t.Errorf("default Cap = %d, want %d", got, defaultRenderCap)
	}
	if got > 3000 {
		t.Errorf("default Cap %d is too large for exact O(n^2) t-SNE", got)
	}
}

// stubProjector confirms the Projector interface is satisfiable by a pure
// implementation (no gonum dependency in core).
type stubProjector struct{}

func (stubProjector) Project(in []Input, _ map[string][2]float64, _ Options) (Result, error) {
	return Result{Total: len(in), Points: []Point{}}, nil
}

func TestProjector_InterfaceSatisfied(t *testing.T) {
	var p Projector = stubProjector{}
	res, err := p.Project(inputs(2), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Errorf("Total = %d, want 2", res.Total)
	}
}
