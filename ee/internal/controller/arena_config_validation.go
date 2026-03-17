/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// arenaConfigSpec is a minimal representation of the arena config YAML
// for extracting required provider groups and tool references.
type arenaConfigSpec struct {
	Spec struct {
		Providers []struct {
			File  string `yaml:"file"`
			Group string `yaml:"group"`
		} `yaml:"providers"`
		SelfPlay *struct {
			Enabled bool `yaml:"enabled"`
			Roles   []struct {
				ID       string `yaml:"id"`
				Provider string `yaml:"provider"`
			} `yaml:"roles"`
		} `yaml:"self_play"`
		Tools      []struct{ File string }    `yaml:"tools"`
		MCPServers []struct{ Command string } `yaml:"mcp_servers"`
	} `yaml:"spec"`
}

// extractRequiredGroups parses the arena config and returns the unique
// provider group names it references. Sources:
//   - spec.providers[].group (defaults to "default" when omitted)
//   - spec.self_play.roles[].provider (when self_play is enabled)
func extractRequiredGroups(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read arena config: %w", err)
	}

	var cfg arenaConfigSpec
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse arena config: %w", err)
	}

	groups := make(map[string]bool)
	for _, p := range cfg.Spec.Providers {
		g := p.Group
		if g == "" {
			g = "default"
		}
		groups[g] = true
	}

	if cfg.Spec.SelfPlay != nil && cfg.Spec.SelfPlay.Enabled {
		for _, role := range cfg.Spec.SelfPlay.Roles {
			if role.Provider != "" {
				groups[role.Provider] = true
			}
		}
	}

	result := make([]string, 0, len(groups))
	for g := range groups {
		result = append(result, g)
	}
	return result, nil
}

// validateProviderGroups checks that spec.providers covers all groups required by the arena config.
// It also validates that provider ID references (self-play, judges) can be unambiguously mapped
// to a single CRD provider in their group.
// Returns a human-readable error message if validation fails, or empty string on success.
func (r *ArenaJobReconciler) validateProviderGroups(
	arenaJob *omniav1alpha1.ArenaJob,
	configPath string,
) string {
	requiredGroups, err := extractRequiredGroups(configPath)
	if err != nil {
		// Can't parse config — skip validation, let the worker report the error
		return ""
	}

	if len(requiredGroups) == 0 {
		return ""
	}

	var missing []string
	for _, group := range requiredGroups {
		entries, exists := arenaJob.Spec.Providers[group]
		if !exists || len(entries) == 0 {
			missing = append(missing, group)
		}
	}

	if len(missing) > 0 {
		return fmt.Sprintf(
			"arena config requires provider groups [%s] but spec.providers is missing: [%s]",
			strings.Join(requiredGroups, ", "),
			strings.Join(missing, ", "),
		)
	}

	// Check for ambiguous provider ID mappings in self-play/judge references
	return validateProviderIDMappings(arenaJob, configPath)
}

// validateProviderIDMappings checks that each provider ID referenced by self-play roles,
// judges, and judge specs maps to exactly one CRD provider in the corresponding group.
// Returns a human-readable error message on failure, or empty string on success.
func validateProviderIDMappings(
	arenaJob *omniav1alpha1.ArenaJob,
	configPath string,
) string {
	refIDs, err := extractProviderIDRefs(configPath)
	if err != nil || len(refIDs) == 0 {
		return ""
	}

	var ambiguous []string
	for _, id := range refIDs {
		entries, exists := arenaJob.Spec.Providers[id]
		if !exists || len(entries) == 0 {
			// Already caught by missing group check above
			continue
		}
		if len(entries) > 1 {
			ambiguous = append(ambiguous, fmt.Sprintf("%s(%d providers)", id, len(entries)))
		}
	}

	if len(ambiguous) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"provider groups referenced by self-play/judges have multiple providers (ambiguous mapping): [%s]",
		strings.Join(ambiguous, ", "),
	)
}

// extractProviderIDRefs parses the arena config YAML and returns all provider IDs
// referenced by self-play roles, judges, and judge specs.
func extractProviderIDRefs(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read arena config: %w", err)
	}

	var cfg arenaConfigProviderIDRefs
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse arena config: %w", err)
	}

	seen := make(map[string]bool)
	var ids []string

	addID := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if cfg.Spec.SelfPlay != nil && cfg.Spec.SelfPlay.Enabled {
		for _, role := range cfg.Spec.SelfPlay.Roles {
			addID(role.Provider)
		}
	}

	for _, judge := range cfg.Spec.Judges {
		addID(judge.Provider)
	}

	for _, spec := range cfg.Spec.JudgeSpecs {
		addID(spec.Provider)
	}

	return ids, nil
}

// arenaConfigProviderIDRefs is a minimal representation of the arena config YAML
// for extracting provider ID references from self-play roles, judges, and judge specs.
type arenaConfigProviderIDRefs struct {
	Spec struct {
		SelfPlay *struct {
			Enabled bool `yaml:"enabled"`
			Roles   []struct {
				Provider string `yaml:"provider"`
			} `yaml:"roles"`
		} `yaml:"self_play"`
		Judges []struct {
			Provider string `yaml:"provider"`
		} `yaml:"judges"`
		JudgeSpecs map[string]struct {
			Provider string `yaml:"provider"`
		} `yaml:"judge_specs"`
	} `yaml:"spec"`
}

// getArenaConfigPath returns the full filesystem path to the arena config file.
// Returns empty string if filesystem access is not available.
func (r *ArenaJobReconciler) getArenaConfigPath(
	arenaJob *omniav1alpha1.ArenaJob,
	basePath string,
) string {
	if basePath == "" {
		return ""
	}

	arenaFile := arenaJob.Spec.ArenaFile
	if arenaFile == "" {
		arenaFile = "config.arena.yaml"
	}
	arenaFileName := filepath.Base(arenaFile)
	return filepath.Join(basePath, arenaFileName)
}
