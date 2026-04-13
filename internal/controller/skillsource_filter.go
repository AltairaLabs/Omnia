/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ResolvedSkill is a skill that passed the filter: its SKILL.md was parsed
// and the containing directory is retained under the target path.
type ResolvedSkill struct {
	// Name is the frontmatter name.
	Name string
	// Description is the frontmatter description.
	Description string
	// AllowedTools is the frontmatter allowed-tools list.
	AllowedTools []string
	// RelPath is the skill's directory path relative to the synced root.
	RelPath string
}

// ResolveSkills walks syncRoot finding every SKILL.md, parses its frontmatter,
// and applies the optional filter. Returns one ResolvedSkill per retained
// skill plus any errors encountered (non-fatal — callers surface them via
// status conditions).
func ResolveSkills(syncRoot string, filter *corev1alpha1.SkillFilter) ([]ResolvedSkill, []error) {
	var resolved []ResolvedSkill
	var errs []error

	_ = filepath.Walk(syncRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}
		dir := filepath.Dir(path)
		relDir, relErr := filepath.Rel(syncRoot, dir)
		if relErr != nil {
			errs = append(errs, relErr)
			return nil
		}

		fm, parseErr := ParseSkillFile(path)
		if parseErr != nil {
			errs = append(errs, parseErr)
			return nil
		}

		if filter != nil && !matchesSkillFilter(relDir, fm.Name, filter) {
			return nil
		}

		resolved = append(resolved, ResolvedSkill{
			Name:         fm.Name,
			Description:  fm.Description,
			AllowedTools: fm.AllowedTools,
			RelPath:      relDir,
		})
		return nil
	})

	return resolved, errs
}

func matchesSkillFilter(relPath, name string, f *corev1alpha1.SkillFilter) bool {
	if len(f.Include) > 0 && !matchesAny(f.Include, relPath) {
		return false
	}
	if matchesAny(f.Exclude, relPath) {
		return false
	}
	if len(f.Names) > 0 && !containsString(f.Names, name) {
		return false
	}
	return true
}

func matchesAny(patterns []string, s string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, s); ok {
			return true
		}
	}
	return false
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
