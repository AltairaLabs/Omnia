/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package schemautil

import (
	"strings"
	"testing"
)

func TestCompileSchema_Valid(t *testing.T) {
	got, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a compiled schema, got nil")
	}
}

func TestCompileSchema_Empty(t *testing.T) {
	_, err := CompileSchema(nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want empty-schema error, got %v", err)
	}
}

func TestCompileSchema_NotJSON(t *testing.T) {
	_, err := CompileSchema([]byte(`{not json`))
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("want not-valid-JSON error, got %v", err)
	}
}

func TestCompileSchema_InvalidSchema(t *testing.T) {
	// Mirrors the issue #1116 reproduction: a non-existent draft-2020-12 type.
	_, err := CompileSchema([]byte(`{"type":"not-a-real-type"}`))
	if err == nil {
		t.Fatal("expected compile error for an invalid JSON Schema, got nil")
	}
}
