/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644))
}

func TestResolveSkills_NoFilter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "a"), "alpha", "first")
	writeSkill(t, filepath.Join(root, "b"), "beta", "second")

	resolved, errs := ResolveSkills(root, nil)
	assert.Empty(t, errs)
	assert.Len(t, resolved, 2)
}

func TestResolveSkills_IncludeGlob(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "safety-a"), "a", "d")
	writeSkill(t, filepath.Join(root, "other"), "b", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Include: []string{"safety-*"},
	})
	assert.Empty(t, errs)
	require.Len(t, resolved, 1)
	assert.Equal(t, "safety-a", resolved[0].RelPath)
}

func TestResolveSkills_Exclude(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "keep"), "keep", "d")
	writeSkill(t, filepath.Join(root, "draft-x"), "dx", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Exclude: []string{"draft-*"},
	})
	assert.Empty(t, errs)
	require.Len(t, resolved, 1)
	assert.Equal(t, "keep", resolved[0].Name)
}

func TestResolveSkills_NamesPin(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "a"), "only-this", "d")
	writeSkill(t, filepath.Join(root, "b"), "and-not-this", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Names: []string{"only-this"},
	})
	assert.Empty(t, errs)
	require.Len(t, resolved, 1)
	assert.Equal(t, "only-this", resolved[0].Name)
}

func TestResolveSkills_ParseErrorsReported(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bad"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "bad", "SKILL.md"),
		[]byte("no frontmatter"), 0644))
	writeSkill(t, filepath.Join(root, "good"), "good", "d")

	resolved, errs := ResolveSkills(root, nil)
	assert.Len(t, resolved, 1)
	assert.NotEmpty(t, errs)
}

func TestResolveSkills_IncludeAndExclude(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "safety-a"), "a", "d")
	writeSkill(t, filepath.Join(root, "safety-draft"), "b", "d")
	writeSkill(t, filepath.Join(root, "other"), "c", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Include: []string{"safety-*"},
		Exclude: []string{"safety-draft"},
	})
	assert.Empty(t, errs)
	require.Len(t, resolved, 1)
	assert.Equal(t, "safety-a", resolved[0].RelPath)
}

func TestResolveSkills_EmptyDir(t *testing.T) {
	root := t.TempDir()
	resolved, errs := ResolveSkills(root, nil)
	assert.Empty(t, resolved)
	assert.Empty(t, errs)
}
