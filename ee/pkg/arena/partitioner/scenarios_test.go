/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package partitioner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListScenariosFromConfig(t *testing.T) {
	dir := t.TempDir()

	// Create scenario files
	writeFile(t, filepath.Join(dir, "billing.scenario.yaml"), `
metadata:
  name: Billing Accuracy
spec:
  id: billing-accuracy
  prompts:
    - text: "What is the cost?"
`)

	writeFile(t, filepath.Join(dir, "auth.scenario.yaml"), `
metadata:
  name: Auth Flow
spec:
  id: auth-flow
  prompts:
    - text: "Login test"
`)

	// Create arena config referencing both scenarios
	configPath := filepath.Join(dir, "config.arena.yaml")
	writeFile(t, configPath, `
apiVersion: arena.altairalabs.ai/v1
kind: ArenaConfig
spec:
  scenarios:
    - file: billing.scenario.yaml
    - file: auth.scenario.yaml
`)

	scenarios, err := ListScenariosFromConfig(configPath)
	if err != nil {
		t.Fatalf("ListScenariosFromConfig() error = %v", err)
	}

	if len(scenarios) != 2 {
		t.Fatalf("len(scenarios) = %d, want 2", len(scenarios))
	}

	// Verify first scenario
	if scenarios[0].ID != "billing-accuracy" {
		t.Errorf("scenarios[0].ID = %s, want billing-accuracy", scenarios[0].ID)
	}
	if scenarios[0].Name != "Billing Accuracy" {
		t.Errorf("scenarios[0].Name = %s, want Billing Accuracy", scenarios[0].Name)
	}
	if scenarios[0].Path != "billing.scenario.yaml" {
		t.Errorf("scenarios[0].Path = %s, want billing.scenario.yaml", scenarios[0].Path)
	}

	// Verify second scenario
	if scenarios[1].ID != "auth-flow" {
		t.Errorf("scenarios[1].ID = %s, want auth-flow", scenarios[1].ID)
	}
}

func TestListScenariosFromConfigNoScenarios(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(dir, "config.arena.yaml")
	writeFile(t, configPath, `
apiVersion: arena.altairalabs.ai/v1
kind: ArenaConfig
spec:
  providers:
    - name: openai
`)

	scenarios, err := ListScenariosFromConfig(configPath)
	if err != nil {
		t.Fatalf("ListScenariosFromConfig() error = %v", err)
	}

	if scenarios != nil {
		t.Errorf("expected nil scenarios, got %v", scenarios)
	}
}

func TestListScenariosFromConfigFileNotFound(t *testing.T) {
	_, err := ListScenariosFromConfig("/nonexistent/path/config.arena.yaml")
	if err == nil {
		t.Error("ListScenariosFromConfig() expected error for missing config file")
	}
}

func TestListScenariosFromConfigMissingScenarioFile(t *testing.T) {
	dir := t.TempDir()

	// Create one valid scenario file
	writeFile(t, filepath.Join(dir, "valid.scenario.yaml"), `
metadata:
  name: Valid Scenario
spec:
  id: valid
`)

	// Reference one valid and one missing scenario file
	configPath := filepath.Join(dir, "config.arena.yaml")
	writeFile(t, configPath, `
spec:
  scenarios:
    - file: valid.scenario.yaml
    - file: missing.scenario.yaml
`)

	scenarios, err := ListScenariosFromConfig(configPath)
	if err != nil {
		t.Fatalf("ListScenariosFromConfig() error = %v", err)
	}

	// Should skip the missing file and return only the valid one
	if len(scenarios) != 1 {
		t.Fatalf("len(scenarios) = %d, want 1", len(scenarios))
	}
	if scenarios[0].ID != "valid" {
		t.Errorf("scenarios[0].ID = %s, want valid", scenarios[0].ID)
	}
}

func TestListScenariosFromConfigIDPriority(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantID   string
		wantName string
	}{
		{
			name: "spec.id takes priority",
			content: `
metadata:
  name: My Scenario
spec:
  id: my-scenario-id
`,
			wantID:   "my-scenario-id",
			wantName: "My Scenario",
		},
		{
			name: "falls back to metadata.name",
			content: `
metadata:
  name: My Scenario Name
spec:
  prompts: []
`,
			wantID:   "My Scenario Name",
			wantName: "My Scenario Name",
		},
		{
			name: "falls back to filename derivation",
			content: `
spec:
  prompts: []
`,
			wantID:   "test-scenario",
			wantName: "test-scenario",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			writeFile(t, filepath.Join(dir, "test-scenario.yaml"), tt.content)

			configPath := filepath.Join(dir, "config.arena.yaml")
			writeFile(t, configPath, `
spec:
  scenarios:
    - file: test-scenario.yaml
`)

			scenarios, err := ListScenariosFromConfig(configPath)
			if err != nil {
				t.Fatalf("ListScenariosFromConfig() error = %v", err)
			}

			if len(scenarios) != 1 {
				t.Fatalf("len(scenarios) = %d, want 1", len(scenarios))
			}
			if scenarios[0].ID != tt.wantID {
				t.Errorf("ID = %s, want %s", scenarios[0].ID, tt.wantID)
			}
			if scenarios[0].Name != tt.wantName {
				t.Errorf("Name = %s, want %s", scenarios[0].Name, tt.wantName)
			}
		})
	}
}

func TestListScenariosFromConfigSubdirectory(t *testing.T) {
	dir := t.TempDir()

	// Create scenario in subdirectory
	scenarioDir := filepath.Join(dir, "scenarios")
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(scenarioDir, "billing.yaml"), `
spec:
  id: billing
`)

	configPath := filepath.Join(dir, "config.arena.yaml")
	writeFile(t, configPath, `
spec:
  scenarios:
    - file: scenarios/billing.yaml
`)

	scenarios, err := ListScenariosFromConfig(configPath)
	if err != nil {
		t.Fatalf("ListScenariosFromConfig() error = %v", err)
	}

	if len(scenarios) != 1 {
		t.Fatalf("len(scenarios) = %d, want 1", len(scenarios))
	}
	if scenarios[0].ID != "billing" {
		t.Errorf("ID = %s, want billing", scenarios[0].ID)
	}
	if scenarios[0].Path != "scenarios/billing.yaml" {
		t.Errorf("Path = %s, want scenarios/billing.yaml", scenarios[0].Path)
	}
}

func TestDeriveIDFromFilename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"billing.yaml", "billing"},
		{"billing.scenario.yaml", "billing"},
		{"my-scenario.yaml", "my-scenario"},
		{"My Scenario.yaml", "My-Scenario"},
		{"scenarios/billing.yaml", "billing"},
		{"test_case_1.yaml", "test-case-1"},
		{".yaml", "scenario"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := deriveIDFromFilename(tt.path)
			if got != tt.want {
				t.Errorf("deriveIDFromFilename(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestListScenariosFromConfigEmptyFileEntry(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(dir, "config.arena.yaml")
	writeFile(t, configPath, `
spec:
  scenarios:
    - file: ""
    - {}
`)

	scenarios, err := ListScenariosFromConfig(configPath)
	if err != nil {
		t.Fatalf("ListScenariosFromConfig() error = %v", err)
	}

	if len(scenarios) != 0 {
		t.Errorf("len(scenarios) = %d, want 0 for empty file entries", len(scenarios))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
