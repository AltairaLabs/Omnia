/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package skills parses the PromptPack skill manifest emitted by the operator
// and exposes it to the runtime binary for PromptKit SDK wiring.
package skills

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest mirrors the struct that
// internal/controller/promptpack_skills.go writes to the workspace PVC.
// The struct is duplicated here on purpose: pulling the controller package
// into the runtime binary would drag in controller-runtime.
type Manifest struct {
	Version string          `json:"version"`
	Skills  []ManifestEntry `json:"skills"`
	Config  *Config         `json:"config,omitempty"`
}

// ManifestEntry is one skill the runtime should expose via WithSkillsDir.
type ManifestEntry struct {
	// MountAs is the directory name PromptKit should expose this skill
	// under (e.g. "billing/refund-processing"). Used for workflow scoping.
	MountAs string `json:"mount_as"`
	// ContentPath is the path under the runtime container's workspace
	// content mount where the skill's directory (containing SKILL.md)
	// lives.
	ContentPath string `json:"content_path"`
	// Name is the SKILL.md frontmatter name (for diagnostic logs).
	Name string `json:"name"`
}

// Config carries the PromptPack.spec.skillsConfig block to the runtime.
type Config struct {
	MaxActive int32  `json:"max_active,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

// Read loads the manifest at path. Returns a zero-value manifest (not an
// error) when path is empty or the file doesn't exist — unconfigured skills
// are normal, and the runtime silently skips WithSkillsDir in that case.
func Read(path string) (*Manifest, error) {
	if path == "" {
		return &Manifest{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("read skill manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse skill manifest %s: %w", path, err)
	}
	return &m, nil
}
