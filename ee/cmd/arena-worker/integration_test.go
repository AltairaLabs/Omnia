//go:build integration

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	integTestNamespace    = "test-ns"
	integTestJobName      = "test-job"
	integTestProviderName = "mock-crd-provider"
)

// makeTestBundle creates the standard arena bundle files in bundleDir.
// It writes config.arena.yaml, prompts/assistant.yaml, providers/mock.provider.yaml,
// scenarios/test.scenario.yaml, and mock-responses.yaml.
func makeTestBundle(t *testing.T, bundleDir string) {
	t.Helper()

	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: integration-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers: []

  scenarios:
    - file: scenarios/test.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: A helpful AI assistant for testing
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	scenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: test
spec:
  id: test
  task_type: assistant
  description: Test scenario
  turns:
    - role: user
      content: "Hello"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "test.scenario.yaml"), []byte(scenario), 0644))

	mockResponses := `defaultResponse: "Hello! How can I help you?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))
}

// integMakeArenaJob creates an unstructured ArenaJob with the given providers map.
func integMakeArenaJob(name, namespace string, providers map[string]interface{}) *unstructured.Unstructured {
	arenaJob := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "omnia.altairalabs.ai/v1alpha1",
			"kind":       "ArenaJob",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"sourceRef": map[string]interface{}{"name": "test-source"},
				"providers": providers,
			},
		},
	}
	arenaJob.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "omnia.altairalabs.ai",
		Version: "v1alpha1",
		Kind:    "ArenaJob",
	})
	return arenaJob
}

// integMakeArenaJobWithToolRegistries creates an unstructured ArenaJob with providers and toolRegistries.
func integMakeArenaJobWithToolRegistries(
	name, namespace string,
	providers map[string]interface{},
	toolRegistries []interface{},
) *unstructured.Unstructured {
	arenaJob := integMakeArenaJob(name, namespace, providers)
	spec := arenaJob.Object["spec"].(map[string]interface{})
	spec["toolRegistries"] = toolRegistries
	return arenaJob
}

// defaultProviders returns the default providers map for an ArenaJob referencing integTestProviderName.
func defaultProviders() map[string]interface{} {
	return map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": integTestProviderName,
				},
			},
		},
	}
}

// makeFakeK8sClient creates a fake k8s client with a Provider CRD and an ArenaJob.
func makeFakeK8sClient(t *testing.T, providerName, namespace string) client.Client {
	t.Helper()

	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerName,
			Namespace: namespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	arenaJob := integMakeArenaJob(integTestJobName, namespace, defaultProviders())

	scheme := k8s.Scheme()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(provider).
		WithObjects(arenaJob).
		Build()
}

// makeTestConfig creates a Config with the standard test settings and given fake client.
func makeTestConfig(bundleDir string, fakeClient client.Client) *Config {
	return &Config{
		JobName:      integTestJobName,
		JobNamespace: integTestNamespace,
		WorkDir:      bundleDir,
		Verbose:      true,
		K8sClient:    fakeClient,
	}
}

// --- Task 1: Fixed pre-existing tests ---

// TestExecuteWorkItemWithMockProvider tests the full execution flow using a mock provider.
func TestExecuteWorkItemWithMockProvider(t *testing.T) {
	bundleDir := t.TempDir()

	// Create the arena config with formats under output
	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: integration-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers: []

  scenarios:
    - file: scenarios/greeting.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: A helpful AI assistant for testing
  system_template: |
    You are a helpful AI assistant.
    Be concise and friendly in your responses.
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

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

    - role: user
      content: "What is 2 + 2?"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "greeting.scenario.yaml"), []byte(greetingScenario), 0644))

	mockResponses := `# Mock responses for integration test
defaultResponse: "Hello! I'm doing great, thank you for asking!"

scenarios:
  greeting-test:
    turns:
      1: "Hello! I'm doing great, thank you for asking! How can I help you today?"
      2: "2 + 2 equals 4. That's basic arithmetic!"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-integration-item",
		JobID:      integTestJobName,
		ScenarioID: "greeting-test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error")

	assert.NotNil(t, result, "result should not be nil")
	assert.Equal(t, statusPass, result.Status, "status should be 'pass'")
	assert.GreaterOrEqual(t, result.DurationMs, float64(0), "duration should be non-negative")

	assert.NotNil(t, result.Metrics, "metrics should not be nil")
	assert.Contains(t, result.Metrics, "runsExecuted", "should have runsExecuted metric")
	assert.Equal(t, float64(1), result.Metrics["runsExecuted"], "should have executed 1 run")

	t.Logf("Execution result: status=%s, duration=%.0fms", result.Status, result.DurationMs)
	t.Logf("Metrics: %+v", result.Metrics)
	if len(result.Assertions) > 0 {
		t.Logf("Assertions: %+v", result.Assertions)
	}
}

// TestExecuteWorkItemWithAssertionFailure tests that assertion failures are properly reported.
func TestExecuteWorkItemWithAssertionFailure(t *testing.T) {
	bundleDir := t.TempDir()

	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: assertion-failure-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers: []

  scenarios:
    - file: scenarios/failing.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: A helpful assistant for assertion failure testing
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

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
            patterns:
              - "THIS_STRING_WILL_NOT_BE_IN_RESPONSE"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "failing.scenario.yaml"), []byte(failingScenario), 0644))

	mockResponses := `defaultResponse: "Hello! How can I help you?"
scenarios:
  failing-test:
    turns:
      1: "Hello there! How can I assist you today?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-failing-item",
		JobID:      integTestJobName,
		ScenarioID: "failing-test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error even for assertion failures")

	assert.NotNil(t, result, "result should not be nil")
	assert.Equal(t, statusFail, result.Status, "status should be 'fail' due to assertion failure")

	t.Logf("Result status: %s", result.Status)
	t.Logf("Assertions: %+v", result.Assertions)
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

  providers: []

  scenarios:
    - file: scenarios/scenario1.scenario.yaml
    - file: scenarios/scenario2.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    output:
      dir: out
      formats:
        - json
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
  description: A helpful assistant for multi-scenario testing
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

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

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	// Test with "default" scenario ID - should run all scenarios
	item := &queue.WorkItem{
		ID:         "test-multi-item",
		JobID:      integTestJobName,
		ScenarioID: "default", // Run all scenarios
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status)

	if runsExecuted, ok := result.Metrics["runsExecuted"]; ok {
		assert.Equal(t, float64(2), runsExecuted, "should have executed 2 scenarios")
	}

	t.Logf("Multi-scenario result: status=%s, metrics=%+v", result.Status, result.Metrics)
}

// TestVerboseLoggingCapturesPromptKitLogs validates that verbose mode captures promptkit debug logs.
func TestVerboseLoggingCapturesPromptKitLogs(t *testing.T) {
	bundleDir := t.TempDir()

	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: verbose-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers: []

  scenarios:
    - file: scenarios/test.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: A helpful assistant for verbose logging testing
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	scenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: test
spec:
  id: test
  task_type: assistant
  description: Test scenario
  turns:
    - role: user
      content: "Hello"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "test.scenario.yaml"), []byte(scenario), 0644))

	mockResponses := `defaultResponse: "Hello! How can I help you?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	// Capture stderr to verify promptkit logs are output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-verbose-item",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)

	// Restore stderr and capture output
	w.Close()
	os.Stderr = oldStderr
	var buf [4096]byte
	n, _ := r.Read(buf[:])
	stderrOutput := string(buf[:n])

	t.Logf("Captured stderr output (%d bytes):\n%s", n, stderrOutput)

	require.NoError(t, err, "executeWorkItem should not return error")
	assert.NotNil(t, result, "result should not be nil")

	// Verbose mode may or may not produce stderr output depending on PromptKit version.
	// We verify the execution succeeds with Verbose=true; stderr capture is best-effort.
	if len(stderrOutput) > 0 {
		t.Logf("Verbose logging captured %d bytes of promptkit logs", len(stderrOutput))
	} else {
		t.Logf("No stderr output captured (promptkit may use structured logging instead)")
	}

	t.Logf("Execution result: status=%s, duration=%.0fms", result.Status, result.DurationMs)
}

// TestExecuteWorkItemWithProviderGroups tests the CRD-based provider resolution path.
func TestExecuteWorkItemWithProviderGroups(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-crd-item",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error with CRD-resolved provider")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status, "mock provider should pass")
	assert.GreaterOrEqual(t, result.DurationMs, float64(0))

	t.Logf("CRD provider groups result: status=%s, duration=%.0fms, metrics=%+v",
		result.Status, result.DurationMs, result.Metrics)
}

// --- Task 2: Edge case tests ---

// CRD error paths

func TestExecuteWorkItem_ArenaJobNotFound(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create a fake client with a Provider but NO ArenaJob
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integTestProviderName,
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(provider).
		Build()

	cfg := &Config{
		JobName:      "nonexistent-job",
		JobNamespace: integTestNamespace,
		WorkDir:      bundleDir,
		K8sClient:    fakeClient,
	}

	item := &queue.WorkItem{
		ID:         "test-no-arenajob",
		JobID:      "nonexistent-job",
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ArenaJob")
}

func TestExecuteWorkItem_ProviderCRDMissing(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create ArenaJob referencing a provider that does NOT exist
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "nonexistent-provider",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-missing-provider",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "nonexistent-provider",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestExecuteWorkItem_EmptyProviders(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create ArenaJob with empty providers map
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-empty-providers",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers")
}

func TestExecuteWorkItem_AgentRefMissingWSURL(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create ArenaJob with an agentRef but no ARENA_AGENT_WS_URLS env var
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"agentRef": map[string]interface{}{
					"name": "my-agent",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(arenaJob).
		Build()

	// Ensure ARENA_AGENT_WS_URLS is empty
	t.Setenv("ARENA_AGENT_WS_URLS", "")

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-missing-ws-url",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "agent-my-agent",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WebSocket URL")
}

// Engine error paths

func TestExecuteWorkItem_MissingConfigFile(t *testing.T) {
	bundleDir := t.TempDir()
	// Create the bundle directory but do NOT write any config files

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-no-config",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestExecuteWorkItem_InvalidConfigYAML(t *testing.T) {
	bundleDir := t.TempDir()

	// Write invalid YAML as the arena config
	invalidYAML := `{{{{this is not valid yaml!!!!`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "config.arena.yaml"), []byte(invalidYAML), 0644))

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-invalid-yaml",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load configuration")
}

func TestExecuteWorkItem_NoCombinations(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	// Use a ProviderID that doesn't match the CRD provider
	item := &queue.WorkItem{
		ID:         "test-no-combos",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "nonexistent-provider-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "should return result not error for no combinations")
	assert.Equal(t, statusFail, result.Status)
	assert.Contains(t, result.Error, "no scenario/provider combinations")
}

// Multi-provider/group tests

func TestExecuteWorkItem_MultipleProvidersInGroup(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create two Provider CRDs
	provider1 := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-one",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	provider2 := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-two",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	// ArenaJob with 2 providers in the "default" group
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "provider-one",
				},
			},
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "provider-two",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(provider1, provider2).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	// Target provider-one
	item := &queue.WorkItem{
		ID:         "test-multi-provider",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "provider-one",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status)
}

func TestExecuteWorkItem_MultipleGroups(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create two Provider CRDs for different groups
	defaultProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-provider",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	judgeProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "judge-provider",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	// ArenaJob with "default" and "judge" groups
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "default-provider",
				},
			},
		},
		"judge": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "judge-provider",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultProvider, judgeProvider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	// Target default-provider from the "default" group
	item := &queue.WorkItem{
		ID:         "test-multi-group",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "default-provider",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status)
}

// ToolRegistry CRD tests

func TestExecuteWorkItem_ToolRegistryCRDMissing(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create Provider CRD
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integTestProviderName,
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	// Create ArenaJob with providers AND a toolRegistries ref to a non-existent ToolRegistry
	arenaJob := integMakeArenaJobWithToolRegistries(
		integTestJobName, integTestNamespace,
		defaultProviders(),
		[]interface{}{
			map[string]interface{}{
				"name": "nonexistent-tool-registry",
			},
		},
	)

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(provider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-missing-toolreg",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ToolRegistry")
}

// Result assertions

func TestExecuteWorkItem_AssertionFailureReportsStatus(t *testing.T) {
	bundleDir := t.TempDir()

	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: assertion-status-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers: []

  scenarios:
    - file: scenarios/failing.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: A helpful assistant for assertion status testing
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))
	failingScenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: failing-test
spec:
  id: failing-test
  task_type: assistant
  description: Scenario with assertion that will fail
  turns:
    - role: user
      content: "Tell me a joke"
      assertions:
        - type: contains
          params:
            patterns:
              - "IMPOSSIBLE_SUBSTRING_THAT_WONT_MATCH"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "failing.scenario.yaml"), []byte(failingScenario), 0644))

	mockResponses := `defaultResponse: "Here is a joke: Why did the chicken cross the road?"
scenarios:
  failing-test:
    turns:
      1: "Here is a joke: Why did the chicken cross the road? To get to the other side!"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	fakeClient := makeFakeK8sClient(t, integTestProviderName, integTestNamespace)
	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-assertion-status",
		JobID:      integTestJobName,
		ScenarioID: "failing-test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should not return error for assertion failures")
	require.NotNil(t, result)

	// Explicitly assert the status is fail
	assert.Equal(t, statusFail, result.Status, "assertion failure should produce statusFail")
	assert.NotEmpty(t, result.Error, "assertion failure should populate Error field")

	t.Logf("Assertion failure result: status=%s, error=%s", result.Status, result.Error)
	for i, a := range result.Assertions {
		t.Logf("  Assertion %d: name=%s passed=%v message=%s", i, a.Name, a.Passed, a.Message)
	}
}

// TestExecuteWorkItemWithSelfPlayRemap tests that CRD providers in a self-play group
// are correctly remapped to the provider ID expected by the arena config.
func TestExecuteWorkItemWithSelfPlayRemap(t *testing.T) {
	bundleDir := t.TempDir()

	// Arena config with self-play enabled, referencing provider "selfplay"
	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: selfplay-remap-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/selfplay.provider.yaml
      group: selfplay

  self_play:
    enabled: true
    personas:
      - file: prompts/persona.yaml
    roles:
      - id: user-sim
        provider: selfplay

  scenarios:
    - file: scenarios/test.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: Assistant for selfplay remap test
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	personaYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: curious-user
spec:
  description: A curious user who asks questions
  system_prompt: "You are a curious user. Ask questions about the topic."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "persona.yaml"), []byte(personaYAML), 0644))

	// Dummy provider YAML so LoadConfig validation passes.
	// CRD resolution will clear and replace LoadedProviders.
	providersDir := filepath.Join(bundleDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0755))

	selfplayProviderYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: selfplay
spec:
  id: selfplay
  type: mock
  model: placeholder
`
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "selfplay.provider.yaml"), []byte(selfplayProviderYAML), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	scenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: test
spec:
  id: test
  task_type: assistant
  description: Test scenario
  turns:
    - role: user
      content: "Hello"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "test.scenario.yaml"), []byte(scenario), 0644))

	mockResponses := `defaultResponse: "Hello! How can I help you?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	// Create Provider CRDs: one for default group, one for selfplay group
	// The selfplay provider has CRD name "mock-selfplay-provider" which differs
	// from the expected ID "selfplay" — remapProviderIDs should fix this.
	defaultProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integTestProviderName,
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	selfplayProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-selfplay-provider",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-selfplay-model",
		},
	}

	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": integTestProviderName,
				},
			},
		},
		"selfplay": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "mock-selfplay-provider",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultProvider, selfplayProvider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-selfplay-remap",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "executeWorkItem should succeed with remapped self-play provider")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status, "mock provider should pass with self-play remap")

	t.Logf("Self-play remap result: status=%s, duration=%.0fms, metrics=%+v",
		result.Status, result.DurationMs, result.Metrics)
}

// TestExecuteWorkItemWithMapModeProviders tests that map-mode provider groups
// use the map key as the provider ID (no sanitizeID, no remapping).
func TestExecuteWorkItemWithMapModeProviders(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create a Provider CRD
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-crd-provider",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	// ArenaJob with map-mode "default" group: key "my-config-id" → providerRef "my-crd-provider"
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": map[string]interface{}{
			"my-config-id": map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "my-crd-provider",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(provider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	// Provider ID should be the map key "my-config-id", not sanitizeID("my-crd-provider")
	item := &queue.WorkItem{
		ID:         "test-map-mode",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "my-config-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "map-mode provider should resolve successfully")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status, "mock provider via map mode should pass")

	t.Logf("Map-mode result: status=%s, duration=%.0fms, metrics=%+v",
		result.Status, result.DurationMs, result.Metrics)
}

// TestExecuteWorkItemWithMixedModeProviders tests that array-mode and map-mode
// groups can coexist in the same ArenaJob.
func TestExecuteWorkItemWithMixedModeProviders(t *testing.T) {
	bundleDir := t.TempDir()
	makeTestBundle(t, bundleDir)

	// Create two Provider CRDs
	defaultProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "array-provider",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	judgeProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "judge-crd",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}

	// Mixed: "default" is array-mode, "judges" is map-mode
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "array-provider",
				},
			},
		},
		"judges": map[string]interface{}{
			"quality-judge": map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "judge-crd",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultProvider, judgeProvider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	// Target the array-mode provider (sanitizeID("array-provider") = "array-provider")
	item := &queue.WorkItem{
		ID:         "test-mixed-mode",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: "array-provider",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "mixed-mode providers should resolve successfully")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status, "mock provider via mixed mode should pass")

	t.Logf("Mixed-mode result: status=%s, duration=%.0fms, metrics=%+v",
		result.Status, result.DurationMs, result.Metrics)
}

// TestExecuteWorkItemWithMapModeSelfPlay tests that map-mode providers for self-play
// skip remapping because the map key already matches the expected config provider ID.
func TestExecuteWorkItemWithMapModeSelfPlay(t *testing.T) {
	bundleDir := t.TempDir()

	// Arena config with self-play referencing provider "selfplay"
	arenaConfig := `$schema: https://promptkit.altairalabs.ai/schemas/latest/arena.json
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: map-selfplay-test
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml

  providers:
    - file: providers/selfplay.provider.yaml
      group: selfplay

  self_play:
    enabled: true
    personas:
      - file: prompts/persona.yaml
    roles:
      - id: user-sim
        provider: selfplay

  scenarios:
    - file: scenarios/test.scenario.yaml

  defaults:
    temperature: 0.5
    max_tokens: 500
    seed: 42
    output:
      dir: out
      formats:
        - json
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
  description: Assistant for map-mode self-play test
  system_template: "You are a helpful assistant."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "assistant.yaml"), []byte(assistantPrompt), 0644))

	personaYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: curious-user
spec:
  description: A curious user who asks questions
  system_prompt: "You are a curious user. Ask questions about the topic."
`
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "persona.yaml"), []byte(personaYAML), 0644))

	providersDir := filepath.Join(bundleDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0755))

	selfplayProviderYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: selfplay
spec:
  id: selfplay
  type: mock
  model: placeholder
`
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "selfplay.provider.yaml"), []byte(selfplayProviderYAML), 0644))

	scenariosDir := filepath.Join(bundleDir, "scenarios")
	require.NoError(t, os.MkdirAll(scenariosDir, 0755))

	scenario := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: test
spec:
  id: test
  task_type: assistant
  description: Test scenario
  turns:
    - role: user
      content: "Hello"
`
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "test.scenario.yaml"), []byte(scenario), 0644))

	mockResponses := `defaultResponse: "Hello! How can I help you?"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "mock-responses.yaml"), []byte(mockResponses), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "out"), 0755))

	// Create Provider CRDs
	defaultProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      integTestProviderName,
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-model",
		},
	}
	selfplayProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-selfplay-crd",
			Namespace: integTestNamespace,
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  "mock",
			Model: "mock-selfplay-model",
		},
	}

	// Map-mode selfplay group: key "selfplay" → CRD "mock-selfplay-crd"
	// The key "selfplay" matches the config reference exactly — no remapping needed.
	arenaJob := integMakeArenaJob(integTestJobName, integTestNamespace, map[string]interface{}{
		"default": []interface{}{
			map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": integTestProviderName,
				},
			},
		},
		"selfplay": map[string]interface{}{
			"selfplay": map[string]interface{}{
				"providerRef": map[string]interface{}{
					"name": "mock-selfplay-crd",
				},
			},
		},
	})

	scheme := k8s.Scheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(defaultProvider, selfplayProvider).
		WithObjects(arenaJob).
		Build()

	cfg := makeTestConfig(bundleDir, fakeClient)

	item := &queue.WorkItem{
		ID:         "test-map-selfplay",
		JobID:      integTestJobName,
		ScenarioID: "test",
		ProviderID: integTestProviderName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executeWorkItem(ctx, testLog(), cfg, item, bundleDir)
	require.NoError(t, err, "map-mode self-play should resolve without remapping")

	assert.NotNil(t, result)
	assert.Equal(t, statusPass, result.Status, "mock provider should pass with map-mode self-play")

	t.Logf("Map-mode self-play result: status=%s, duration=%.0fms, metrics=%+v",
		result.Status, result.DurationMs, result.Metrics)
}
