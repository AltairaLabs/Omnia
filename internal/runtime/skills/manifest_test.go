/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_EmptyPath(t *testing.T) {
	m, err := Read("")
	require.NoError(t, err)
	assert.NotNil(t, m)
	assert.Empty(t, m.Skills)
}

func TestRead_FileNotExists(t *testing.T) {
	m, err := Read("/nonexistent/manifest.json")
	require.NoError(t, err)
	assert.NotNil(t, m)
	assert.Empty(t, m.Skills)
}

func TestRead_ValidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	body := `{
		"version": "vabc123",
		"skills": [
			{"mount_as": "billing/refund", "content_path": "skills/internal/refund", "name": "refund"},
			{"mount_as": "compliance/ai-safety", "content_path": "skills/anthropic/ai-safety", "name": "ai-safety"}
		],
		"config": {"max_active": 3, "selector": "tag"}
	}`
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))

	m, err := Read(path)
	require.NoError(t, err)
	assert.Equal(t, "vabc123", m.Version)
	assert.Len(t, m.Skills, 2)
	require.NotNil(t, m.Config)
	assert.Equal(t, int32(3), m.Config.MaxActive)
	assert.Equal(t, "tag", m.Config.Selector)
}

func TestRead_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))

	_, err := Read(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestRead_EmptySkillsArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"v","skills":[]}`), 0644))

	m, err := Read(path)
	require.NoError(t, err)
	assert.Empty(t, m.Skills)
}
