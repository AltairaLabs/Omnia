/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package pgutil

import (
	"testing"
	"time"
)

func TestNullString(t *testing.T) {
	if got := NullString(""); got != nil {
		t.Errorf("expected nil for empty string, got %v", got)
	}
	if got := NullString("hello"); got == nil || *got != "hello" {
		t.Errorf("expected pointer to %q, got %v", "hello", got)
	}
}

func TestDerefString(t *testing.T) {
	if got := DerefString(nil); got != "" {
		t.Errorf("expected empty string for nil, got %q", got)
	}
	s := "world"
	if got := DerefString(&s); got != "world" {
		t.Errorf("expected %q, got %q", "world", got)
	}
}

func TestNullInt(t *testing.T) {
	if got := NullInt(0); got != nil {
		t.Errorf("expected nil for zero, got %v", got)
	}
	if got := NullInt(42); got == nil || *got != 42 {
		t.Errorf("expected pointer to 42, got %v", got)
	}
}

func TestNullInt32(t *testing.T) {
	if got := NullInt32(0); got != nil {
		t.Errorf("expected nil for zero, got %v", got)
	}
	if got := NullInt32(7); got == nil || *got != 7 {
		t.Errorf("expected pointer to 7, got %v", got)
	}
}

func TestNullInt64(t *testing.T) {
	if got := NullInt64(0); got != nil {
		t.Errorf("expected nil for zero, got %v", got)
	}
	if got := NullInt64(99); got == nil || *got != 99 {
		t.Errorf("expected pointer to 99, got %v", got)
	}
}

func TestNullTime(t *testing.T) {
	if got := NullTime(time.Time{}); got != nil {
		t.Errorf("expected nil for zero time, got %v", got)
	}
	now := time.Now()
	if got := NullTime(now); got == nil || !got.Equal(now) {
		t.Errorf("expected pointer to %v, got %v", now, got)
	}
}

func TestTimeOrZero(t *testing.T) {
	if got := TimeOrZero(nil); !got.IsZero() {
		t.Errorf("expected zero time for nil, got %v", got)
	}
	now := time.Now()
	if got := TimeOrZero(&now); !got.Equal(now) {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestMarshalJSONB_Nil(t *testing.T) {
	got := MarshalJSONB(nil)
	if string(got) != "{}" {
		t.Errorf("expected %q, got %q", "{}", string(got))
	}
}

func TestMarshalJSONB_NonNil(t *testing.T) {
	m := map[string]string{"key": "value"}
	got := MarshalJSONB(m)
	if string(got) != `{"key":"value"}` {
		t.Errorf("expected %q, got %q", `{"key":"value"}`, string(got))
	}
}

func TestUnmarshalJSONB_Empty(t *testing.T) {
	if got := UnmarshalJSONB(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	if got := UnmarshalJSONB([]byte{}); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestUnmarshalJSONB_EmptyObject(t *testing.T) {
	if got := UnmarshalJSONB([]byte("{}")); got != nil {
		t.Errorf("expected nil for empty JSON object, got %v", got)
	}
}

func TestUnmarshalJSONB_InvalidJSON(t *testing.T) {
	if got := UnmarshalJSONB([]byte("not json")); got != nil {
		t.Errorf("expected nil for invalid JSON, got %v", got)
	}
}

func TestUnmarshalJSONB_Valid(t *testing.T) {
	got := UnmarshalJSONB([]byte(`{"a":"1","b":"2"}`))
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("unexpected map: %v", got)
	}
}
