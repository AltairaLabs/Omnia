/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SkillFrontmatter is the YAML frontmatter at the top of a SKILL.md file.
// Matches the AgentSkills.io specification subset used by PromptKit.
type SkillFrontmatter struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	AllowedTools []string          `yaml:"allowed-tools,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
}

// ParseSkillFile reads a SKILL.md path and returns its parsed frontmatter.
func ParseSkillFile(path string) (*SkillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	return parseSkillBytes(data)
}

const skillFrontmatterOpen = "---\n"

func parseSkillBytes(data []byte) (*SkillFrontmatter, error) {
	if !bytes.HasPrefix(data, []byte(skillFrontmatterOpen)) {
		return nil, fmt.Errorf("SKILL.md missing opening '---' frontmatter marker")
	}
	rest := data[len(skillFrontmatterOpen):]
	// Closing marker is either "\n---\n" (body follows) or "\n---" at end of file.
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		end = bytes.Index(rest, []byte("\n---"))
		if end < 0 {
			return nil, fmt.Errorf("SKILL.md missing closing '---' frontmatter marker")
		}
	}
	frontmatterBytes := rest[:end]

	var fm SkillFrontmatter
	if err := yaml.Unmarshal(frontmatterBytes, &fm); err != nil {
		return nil, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if fm.Name == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required 'name' field")
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required 'description' field")
	}
	return &fm, nil
}
