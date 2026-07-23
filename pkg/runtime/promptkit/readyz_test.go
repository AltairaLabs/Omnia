/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package promptkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/schema"
)

const promptID = "default"

// writePackFile marshals a minimal pack and writes it to a temp file. When
// withTemplateEngine is false the pack is missing the required root
// template_engine, so it fails schema validation (the #1299 repro).
func writePackFile(t *testing.T, withTemplateEngine bool) string {
	t.Helper()
	pack := map[string]any{
		"id":      "test",
		"name":    "Test Pack",
		"version": "1.0.0",
		"prompts": map[string]any{
			promptID: map[string]any{
				"id":              promptID,
				"name":            "Default",
				"version":         "1.0.0",
				"system_template": "Test",
			},
		},
	}
	if withTemplateEngine {
		pack["template_engine"] = map[string]any{"version": "v1", "syntax": "{{variable}}"}
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "pack.json")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPackReadyError(t *testing.T) {
	v := schema.NewSchemaValidatorWithOptions(zap.New(zap.UseDevMode(true)), nil, 0)

	t.Run("valid pack is ready", func(t *testing.T) {
		if err := packReadyError(v, writePackFile(t, true)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("schema-invalid pack is not ready", func(t *testing.T) {
		if err := packReadyError(v, writePackFile(t, false)); err == nil {
			t.Fatal("expected error for schema-invalid pack")
		}
	})

	t.Run("missing pack file is not ready", func(t *testing.T) {
		if err := packReadyError(v, filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("expected error for missing pack file")
		}
	})

	t.Run("nil validator skips schema check (fail-open) when file is readable", func(t *testing.T) {
		if err := packReadyError(nil, writePackFile(t, false)); err != nil {
			t.Fatalf("nil validator should not fail readiness: %v", err)
		}
	})
}
