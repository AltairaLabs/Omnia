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

	if len(missing) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"arena config requires provider groups [%s] but spec.providers is missing: [%s]",
		strings.Join(requiredGroups, ", "),
		strings.Join(missing, ", "),
	)
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
