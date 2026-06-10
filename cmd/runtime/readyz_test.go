/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/schema"
)

const (
	validPackJSON   = `{"id":"test","name":"Test Pack","version":"1.0.0","template_engine":{"version":"v1","syntax":"{{variable}}"},"prompts":{"default":{"id":"default","name":"Default","version":"1.0.0","system_template":"Test"}}}`
	invalidPackJSON = `{"id":"test","name":"Test Pack","version":"1.0.0","prompts":{"default":{"id":"default","name":"Default","version":"1.0.0","system_template":"Test"}}}` // missing template_engine
)

func writePack(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.json")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPackReadyError(t *testing.T) {
	v := schema.NewSchemaValidatorWithOptions(zap.New(zap.UseDevMode(true)), nil, 0)

	t.Run("valid pack is ready", func(t *testing.T) {
		if err := packReadyError(v, writePack(t, validPackJSON)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("schema-invalid pack is not ready", func(t *testing.T) {
		if err := packReadyError(v, writePack(t, invalidPackJSON)); err == nil {
			t.Fatal("expected error for schema-invalid pack")
		}
	})

	t.Run("missing pack file is not ready", func(t *testing.T) {
		if err := packReadyError(v, filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("expected error for missing pack file")
		}
	})

	t.Run("nil validator skips schema check (fail-open) when file is readable", func(t *testing.T) {
		if err := packReadyError(nil, writePack(t, invalidPackJSON)); err != nil {
			t.Fatalf("nil validator should not fail readiness: %v", err)
		}
	})
}
