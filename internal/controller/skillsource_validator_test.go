/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkillBytes_Valid(t *testing.T) {
	content := `---
name: ai-safety
description: Safety guardrails for AI-generated content.
allowed-tools:
  - redact
  - escalate
metadata:
  tags: "safety,compliance"
---

# AI Safety

Follow these rules when generating content.
`
	fm, err := parseSkillBytes([]byte(content))
	require.NoError(t, err)
	assert.Equal(t, "ai-safety", fm.Name)
	assert.Equal(t, "Safety guardrails for AI-generated content.", fm.Description)
	assert.Equal(t, []string{"redact", "escalate"}, fm.AllowedTools)
	assert.Equal(t, "safety,compliance", fm.Metadata["tags"])
}

func TestParseSkillBytes_MissingOpeningMarker(t *testing.T) {
	_, err := parseSkillBytes([]byte("name: foo\ndescription: bar\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening")
}

func TestParseSkillBytes_MissingClosingMarker(t *testing.T) {
	// Has opening but no closing "---".
	_, err := parseSkillBytes([]byte("---\nname: foo\ndescription: bar\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closing")
}

func TestParseSkillBytes_ClosingAtEOF(t *testing.T) {
	// Valid: frontmatter ends with "\n---" at EOF (no body).
	content := strings.Join([]string{
		"---",
		"name: eof",
		"description: ends here",
		"---",
	}, "\n")
	fm, err := parseSkillBytes([]byte(content))
	require.NoError(t, err)
	assert.Equal(t, "eof", fm.Name)
}

func TestParseSkillBytes_MissingName(t *testing.T) {
	_, err := parseSkillBytes([]byte(strings.Join([]string{
		"---",
		"description: No name here",
		"---",
		"body",
	}, "\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestParseSkillBytes_MissingDescription(t *testing.T) {
	_, err := parseSkillBytes([]byte(strings.Join([]string{
		"---",
		"name: nodesc",
		"---",
		"body",
	}, "\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}

func TestParseSkillBytes_MalformedYAML(t *testing.T) {
	_, err := parseSkillBytes([]byte(strings.Join([]string{
		"---",
		"name: [unclosed",
		"---",
		"body",
	}, "\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestParseSkillFile_ReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := "---\nname: fromdisk\ndescription: yes\n---\nbody"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	fm, err := ParseSkillFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fromdisk", fm.Name)
}

func TestParseSkillFile_MissingFile(t *testing.T) {
	_, err := ParseSkillFile("/nonexistent/SKILL.md")
	require.Error(t, err)
}
