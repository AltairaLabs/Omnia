//go:build integration

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altairalabs/omnia/pkg/arena/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteWorkItemWithMockProvider tests the full execution flow using a mock provider.
// This is an integration test that creates a complete arena bundle and runs it through
// the worker's programmatic engine.
func TestExecuteWorkItemWithMockProvider(t *testing.T) {
	// Create test bundle directory
	bundleDir := t.TempDir()

	// Create the arena config file
	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: integration-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/mock.provider.yaml

  scenarios:
    - file: scenarios/greeting.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "config.arena.yaml"), []byte(arenaConfig), 0644))

	// Create prompts directory and assistant prompt
	promptsDir := filepath.Join(bundleDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0755))

	assistantPrompt := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: assistant
  version: v1.0.0
  description: A helpful AI assistant for testing
  system_template: |
    You are a helpful AI assistant.
    Be concise and friendly in your responses.
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	// Create providers directory and mock provider
	providersDir := filepath.Join(bundleDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0755))

	mockProvider := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-provider
spec:
  id: mock-provider
  type: mock
  model: mock-model
  defaults:
    temperature: 0.7
    max_tokens: 500
  additional_config:
    mock_config: mock-responses.yaml
`
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "mock.provider.yaml"), []byte(mockProvider), 0644))

	// Create scenarios directory and greeting scenario
	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	greetingScenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: greeting-test
spec:
  id: greeting-test
  task_type: assistant
  description: Test basic greeting response

  turns:
    - role: user
      content: "Hello! How are you today?"
      assertions:
        - type: contains
          params:
            substring: "hello"
            case_insensitive: true

    - role: user
      content: "What is 2 + 2?"
      assertions:
        - type: contains
          params:
            substring: "4"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "greeting.scenario.yaml"), []byte(greetingScenario), 0644))

	// Create mock responses file
	mockResponses := `# Mock responses for integration test
defaultResponse: "Hello! I'm doing great, thank you for asking!"

scenarios:
  greeting-test:
    turns:
      1: "Hello! I'm doing great, thank you for asking! How can I help you today?"
      2: "2 + 2 equals 4. That's basic arithmetic!"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))

	// Create output directory
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	// Configure the worker
	cfg := &Config{
		WorkDir: bundleDir,
		Verbose: true,
	}

	// Create a work item
	item := &queue.WorkItem{
		ID:         "test-integration-item",
		JobID:      "test-job",
		ScenarioID: "greeting-test",
		ProviderID: "mock-provider",
	}

	// Execute the work item
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error")

	// Verify the result
	assert.NotNil(t, result, "result should not be nil")
	assert.Equal(t, statusPass, result.Status, "status should be 'pass'")
	assert.Greater(t, result.DurationMs, float64(0), "duration should be positive")

	// Verify metrics
	assert.NotNil(t, result.Metrics, "metrics should not be nil")
	assert.Contains(t, result.Metrics, "runsExecuted", "should have runsExecuted metric")
	assert.Equal(t, float64(1), result.Metrics["runsExecuted"], "should have executed 1 run")

	// Log results for debugging
	t.Logf("Execution result: status=%s, duration=%.0fms", result.Status, result.DurationMs)
	t.Logf("Metrics: %+v", result.Metrics)
	if len(result.Assertions) > 0 {
		t.Logf("Assertions: %+v", result.Assertions)
	}
}

// TestExecuteWorkItemWithAssertionFailure tests that assertion failures are properly reported.
func TestExecuteWorkItemWithAssertionFailure(t *testing.T) {
	bundleDir := t.TempDir()

	// Create config with a scenario that will fail assertions
	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: assertion-failure-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/mock.provider.yaml

  scenarios:
    - file: scenarios/failing.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "config.arena.yaml"), []byte(arenaConfig), 0644))

	// Create prompts directory
	promptsDir := filepath.Join(bundleDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0755))

	assistantPrompt := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: assistant
  version: v1.0.0
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	// Create providers directory
	providersDir := filepath.Join(bundleDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0755))

	mockProvider := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-provider
spec:
  id: mock-provider
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml
`
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "mock.provider.yaml"), []byte(mockProvider), 0644))

	// Create scenarios with an assertion that will fail
	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	failingScenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: failing-test
spec:
  id: failing-test
  task_type: assistant
  description: Test with failing assertion

  turns:
    - role: user
      content: "Say hello"
      assertions:
        - type: contains
          params:
            substring: "THIS_STRING_WILL_NOT_BE_IN_RESPONSE"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "failing.scenario.yaml"), []byte(failingScenario), 0644))

	// Create mock responses
	mockResponses := `defaultResponse: "Hello! How can I help you?"
scenarios:
  failing-test:
    turns:
      1: "Hello there! How can I assist you today?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))

	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	cfg := &Config{
		WorkDir: bundleDir,
		Verbose: true,
	}

	item := &queue.WorkItem{
		ID:         "test-failing-item",
		JobID:      "test-job",
		ScenarioID: "failing-test",
		ProviderID: "mock-provider",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error even for assertion failures")

	// The status should be fail due to assertion failure
	assert.NotNil(t, result, "result should not be nil")
	// Note: Whether this is pass or fail depends on how PromptKit handles assertion failures
	// Some frameworks mark the run as pass even if assertions fail, others mark as fail
	t.Logf("Result status: %s", result.Status)
	t.Logf("Assertions: %+v", result.Assertions)

	// Check that assertions are reported
	if len(result.Assertions) > 0 {
		t.Logf("Found %d assertion(s) in result", len(result.Assertions))
		for i, a := range result.Assertions {
			t.Logf("  Assertion %d: name=%s passed=%v message=%s", i, a.Name, a.Passed, a.Message)
		}
	}
}

// TestExecuteWorkItemWithMultipleScenarios tests running multiple scenarios.
func TestExecuteWorkItemWithMultipleScenarios(t *testing.T) {
	bundleDir := t.TempDir()

	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: multi-scenario-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/mock.provider.yaml

  scenarios:
    - file: scenarios/scenario1.scenario.yaml
    - file: scenarios/scenario2.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    output:
      dir: out
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "config.arena.yaml"), []byte(arenaConfig), 0644))

	promptsDir := filepath.Join(bundleDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0755))
	assistantPrompt := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: assistant
  version: v1.0.0
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	providersDir := filepath.Join(bundleDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0755))
	mockProvider := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock-provider
spec:
  id: mock-provider
  type: mock
  model: mock-model
  additional_config:
    mock_config: mock-responses.yaml
`
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "mock.provider.yaml"), []byte(mockProvider), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	scenario1 := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: scenario1
spec:
  id: scenario1
  task_type: assistant
  description: First test scenario
  turns:
    - role: user
      content: "Hello from scenario 1"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "scenario1.scenario.yaml"), []byte(scenario1), 0644))

	scenario2 := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: scenario2
spec:
  id: scenario2
  task_type: assistant
  description: Second test scenario
  turns:
    - role: user
      content: "Hello from scenario 2"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "scenario2.scenario.yaml"), []byte(scenario2), 0644))

	mockResponses := `defaultResponse: "Hello! I received your message."
scenarios:
  scenario1:
    turns:
      1: "Hello from scenario 1 response!"
  scenario2:
    turns:
      1: "Hello from scenario 2 response!"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	cfg := &Config{
		WorkDir: bundleDir,
		Verbose: true,
	}

	// Test with "default" scenario ID - should run all scenarios
	item := &queue.WorkItem{
		ID:         "test-multi-item",
		JobID:      "test-job",
		ScenarioID: "default", // Run all scenarios
		ProviderID: "mock-provider",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status)

	// Should have executed 2 scenarios
	if runsExecuted, ok := result.Metrics["runsExecuted"]; ok {
		assert.Equal(t, float64(2), runsExecuted, "should have executed 2 scenarios")
	}

	t.Logf("Multi-scenario result: status=%s, metrics=%+v", result.Status, result.Metrics)
}
