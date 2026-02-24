/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fleet

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ScenarioTurn represents a single turn in a scenario conversation.
type ScenarioTurn struct {
	Role    string `yaml:"role"`
	Content string `yaml:"content"`
}

// scenarioFileFormat is a minimal representation of a scenario YAML file,
// extracting only the fields needed for fleet conversation driving.
type scenarioFileFormat struct {
	Spec struct {
		Turns []ScenarioTurn `yaml:"turns"`
	} `yaml:"spec"`
}

// ParseScenarioFile reads a scenario YAML file and extracts the conversation turns.
func ParseScenarioFile(path string) ([]ScenarioTurn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file %s: %w", path, err)
	}

	var sf scenarioFileFormat
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse scenario file %s: %w", path, err)
	}

	if len(sf.Spec.Turns) == 0 {
		return nil, fmt.Errorf("scenario file %s has no turns", path)
	}

	return sf.Spec.Turns, nil
}
