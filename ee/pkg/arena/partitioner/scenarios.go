/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package partitioner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// arenaConfigFile is a minimal representation of an arena config YAML file.
// Only the fields needed for scenario enumeration are included.
type arenaConfigFile struct {
	Spec struct {
		Scenarios []struct {
			File string `yaml:"file"`
		} `yaml:"scenarios"`
	} `yaml:"spec"`
}

// scenarioFile is a minimal representation of a scenario YAML file.
type scenarioFile struct {
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		ID string `yaml:"id"`
	} `yaml:"spec"`
}

// ListScenariosFromConfig reads an arena config YAML file and enumerates the
// scenarios it references. For each scenario file entry, it reads the file and
// extracts the scenario ID using the following priority:
//  1. spec.id
//  2. metadata.name
//  3. filename derivation (strip extension, replace non-alnum with hyphens)
//
// Scenario files that cannot be read are skipped gracefully.
func ListScenariosFromConfig(configPath string) ([]Scenario, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read arena config %s: %w", configPath, err)
	}

	var config arenaConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse arena config %s: %w", configPath, err)
	}

	if len(config.Spec.Scenarios) == 0 {
		return nil, nil
	}

	configDir := filepath.Dir(configPath)
	scenarios := make([]Scenario, 0, len(config.Spec.Scenarios))

	for _, entry := range config.Spec.Scenarios {
		if entry.File == "" {
			continue
		}

		scenarioPath := filepath.Join(configDir, entry.File)
		scenario, err := readScenarioFile(scenarioPath, entry.File)
		if err != nil {
			// Skip scenario files that can't be read — don't fail the whole enumeration
			continue
		}

		scenarios = append(scenarios, *scenario)
	}

	return scenarios, nil
}

// readScenarioFile reads a single scenario file and extracts its metadata.
func readScenarioFile(absPath, relativePath string) (*Scenario, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file %s: %w", absPath, err)
	}

	var sf scenarioFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse scenario file %s: %w", absPath, err)
	}

	// Determine ID with fallback chain: spec.id > metadata.name > filename derivation
	id := sf.Spec.ID
	if id == "" {
		id = sf.Metadata.Name
	}
	if id == "" {
		id = deriveIDFromFilename(relativePath)
	}

	name := sf.Metadata.Name
	if name == "" {
		name = id
	}

	return &Scenario{
		ID:   id,
		Name: name,
		Path: relativePath,
	}, nil
}

// deriveIDFromFilename creates a scenario ID from a file path by stripping
// the extension and replacing non-alphanumeric characters with hyphens.
func deriveIDFromFilename(path string) string {
	base := filepath.Base(path)
	// Strip all extensions (e.g., "test.scenario.yaml" -> "test")
	for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
		base = strings.TrimSuffix(base, ext)
	}

	var b strings.Builder
	prevHyphen := false
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}

	result := strings.TrimRight(b.String(), "-")
	if result == "" {
		return "scenario"
	}
	return result
}
