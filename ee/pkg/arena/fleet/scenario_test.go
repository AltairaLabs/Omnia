/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fleet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScenarioFile(t *testing.T) {
	t.Run("parses multi-turn scenario", func(t *testing.T) {
		content := `metadata:
  name: greeting-test
spec:
  id: greeting-test
  turns:
    - role: user
      content: "Hello! How are you today?"
    - role: user
      content: "What is 2 + 2?"
`
		path := writeTempFile(t, "scenario.yaml", content)

		turns, err := ParseScenarioFile(path)
		require.NoError(t, err)
		assert.Len(t, turns, 2)
		assert.Equal(t, "user", turns[0].Role)
		assert.Equal(t, "Hello! How are you today?", turns[0].Content)
		assert.Equal(t, "user", turns[1].Role)
		assert.Equal(t, "What is 2 + 2?", turns[1].Content)
	})

	t.Run("parses single-turn scenario", func(t *testing.T) {
		content := `spec:
  turns:
    - role: user
      content: "Just one question"
`
		path := writeTempFile(t, "single.yaml", content)

		turns, err := ParseScenarioFile(path)
		require.NoError(t, err)
		assert.Len(t, turns, 1)
		assert.Equal(t, "Just one question", turns[0].Content)
	})

	t.Run("returns error for empty turns", func(t *testing.T) {
		content := `spec:
  id: empty-test
  turns: []
`
		path := writeTempFile(t, "empty.yaml", content)

		_, err := ParseScenarioFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no turns")
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := ParseScenarioFile("/nonexistent/file.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read")
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		path := writeTempFile(t, "bad.yaml", "{{invalid yaml")

		_, err := ParseScenarioFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})

	t.Run("returns error for missing turns field", func(t *testing.T) {
		content := `spec:
  id: no-turns
  description: "A scenario with no turns field"
`
		path := writeTempFile(t, "no-turns.yaml", content)

		_, err := ParseScenarioFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no turns")
	})
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}
