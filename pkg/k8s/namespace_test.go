/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorNamespace(t *testing.T) {
	const fallback = "omnia-system"

	t.Run("prefers POD_NAMESPACE env", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "omnia")
		// Even with a SA file present, env wins.
		withNamespaceFile(t, "from-file")
		if got := OperatorNamespace(fallback); got != "omnia" {
			t.Fatalf("got %q, want %q", got, "omnia")
		}
	})

	t.Run("trims whitespace from env", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "  omnia\n")
		if got := OperatorNamespace(fallback); got != "omnia" {
			t.Fatalf("got %q, want %q", got, "omnia")
		}
	})

	t.Run("falls back to SA namespace file when env unset", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "")
		withNamespaceFile(t, "omnia-from-file\n")
		if got := OperatorNamespace(fallback); got != "omnia-from-file" {
			t.Fatalf("got %q, want %q", got, "omnia-from-file")
		}
	})

	t.Run("uses fallback when env unset and no SA file", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "")
		withNamespaceFile(t, "") // points at a non-existent path
		_ = os.Remove(podNamespaceFile)
		if got := OperatorNamespace(fallback); got != fallback {
			t.Fatalf("got %q, want %q", got, fallback)
		}
	})

	t.Run("uses fallback when SA file is empty", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "")
		withNamespaceFile(t, "   \n")
		if got := OperatorNamespace(fallback); got != fallback {
			t.Fatalf("got %q, want %q", got, fallback)
		}
	})
}

// withNamespaceFile points podNamespaceFile at a temp file containing content
// (or, when content is "", a path that does not exist) for the duration of the
// test, restoring the original afterwards.
func withNamespaceFile(t *testing.T, content string) {
	t.Helper()
	orig := podNamespaceFile
	t.Cleanup(func() { podNamespaceFile = orig })

	path := filepath.Join(t.TempDir(), "namespace")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write temp namespace file: %v", err)
		}
	}
	podNamespaceFile = path
}
