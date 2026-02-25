/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewConfigMapFetcher(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name   string
		config ConfigMapFetcherConfig
	}{
		{
			name: "basic config",
			config: ConfigMapFetcherConfig{
				Name:      "my-config",
				Namespace: "default",
			},
		},
		{
			name: "with custom timeout",
			config: ConfigMapFetcherConfig{
				Name:      "my-config",
				Namespace: "default",
				Options: Options{
					Timeout: 120 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := NewConfigMapFetcher(tt.config, fakeClient)
			assert.NotNil(t, fetcher)
			assert.Equal(t, "configmap", fetcher.Type())
		})
	}
}

func TestConfigMapFetcher_Type(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test",
		Namespace: "default",
	}, fakeClient)

	assert.Equal(t, "configmap", fetcher.Type())
}

func TestConfigMapFetcher_LatestRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-config",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	revision, err := fetcher.LatestRevision(ctx)
	require.NoError(t, err)
	assert.Equal(t, "12345", revision)
}

func TestConfigMapFetcher_LatestRevision_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "nonexistent",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.LatestRevision(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ConfigMap")
}

func TestConfigMapFetcher_Fetch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "key: value\n",
			"prompt.txt":  "Hello, world!",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "12345")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
	assert.Equal(t, "12345", artifact.Revision)
	assert.True(t, strings.HasPrefix(artifact.Checksum, "sha256:"))
	assert.Greater(t, artifact.Size, int64(0))

	// Verify directory contents
	verifyDirectoryContents(t, artifact.Path, map[string]string{
		"config.yaml": "key: value\n",
		"prompt.txt":  "Hello, world!",
	})
}

func TestConfigMapFetcher_Fetch_WithBinaryData(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
		BinaryData: map[string][]byte{
			"binary.bin": binaryContent,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)

	// Verify binary data is in directory
	binaryPath := filepath.Join(artifact.Path, "binary.bin")
	content, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, content)
}

func TestConfigMapFetcher_Fetch_RevisionMismatch(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-config",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "99999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "revision mismatch")
}

func TestConfigMapFetcher_Fetch_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "nonexistent",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	_, err := fetcher.Fetch(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get ConfigMap")
}

func TestConfigMapFetcher_Fetch_EmptyConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "empty-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		// No Data or BinaryData
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "empty-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Should create a valid (empty) directory
	assert.NotEmpty(t, artifact.Path)
	assert.Equal(t, "12345", artifact.Revision)

	// Verify directory exists but is empty
	entries, err := os.ReadDir(artifact.Path)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestConfigMapFetcher_Fetch_DataPrecedenceOverBinary(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Same key in both Data and BinaryData - Data should take precedence
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.yaml": "from-data",
		},
		BinaryData: map[string][]byte{
			"config.yaml": []byte("from-binary"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Data should take precedence
	configPath := filepath.Join(artifact.Path, "config.yaml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("from-data"), content)
}

func TestConfigMapFetcher_DeterministicChecksum(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"a.txt": "content a",
			"b.txt": "content b",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()

	// Fetch twice and verify same checksum
	artifact1, err := fetcher.Fetch(ctx, "12345")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact1.Path) }()

	artifact2, err := fetcher.Fetch(ctx, "12345")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact2.Path) }()

	assert.Equal(t, artifact1.Checksum, artifact2.Checksum)
}

func TestConfigMapFetcher_Fetch_BinaryDataOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "binary-only-config",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		BinaryData: map[string][]byte{
			"data.bin": binaryContent,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "binary-only-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Verify binary data
	content, err := os.ReadFile(filepath.Join(artifact.Path, "data.bin"))
	require.NoError(t, err)
	assert.Equal(t, binaryContent, content)
}

func TestConfigMapFetcher_Fetch_WithZeroTimestamp(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-config",
			Namespace:       "default",
			ResourceVersion: "12345",
			// No CreationTimestamp set (zero value)
		},
		Data: map[string]string{
			"config.yaml": "key: value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "test-config",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	assert.NotEmpty(t, artifact.Path)
}

func TestConfigMapFetcher_Fetch_DecodesDoubleUnderscoreToNestedDirs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// The dashboard deploy route encodes "/" as "__" because K8s ConfigMap keys
	// only allow [-._a-zA-Z0-9]+. The fetcher must decode them back.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "arena-project-test",
			Namespace:         "default",
			ResourceVersion:   "12345",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.arena.yaml":                     "apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: ArenaConfig\n",
			"scenarios__greeting.scenario.yaml":     "scenario: greeting",
			"scenarios__deep__nested.scenario.yaml": "scenario: nested",
			"prompts__main.yaml":                    "prompt: main",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "arena-project-test",
		Namespace: "default",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// config.arena.yaml should remain at root (no __ to decode)
	content, err := os.ReadFile(filepath.Join(artifact.Path, "config.arena.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "ArenaConfig")

	// scenarios__greeting.scenario.yaml should become scenarios/greeting.scenario.yaml
	content, err = os.ReadFile(filepath.Join(artifact.Path, "scenarios", "greeting.scenario.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "scenario: greeting", string(content))

	// scenarios__deep__nested.scenario.yaml should become scenarios/deep/nested.scenario.yaml
	content, err = os.ReadFile(filepath.Join(artifact.Path, "scenarios", "deep", "nested.scenario.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "scenario: nested", string(content))

	// prompts__main.yaml should become prompts/main.yaml
	content, err = os.ReadFile(filepath.Join(artifact.Path, "prompts", "main.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "prompt: main", string(content))

	// The flat __-encoded filenames should NOT exist
	_, err = os.Stat(filepath.Join(artifact.Path, "scenarios__greeting.scenario.yaml"))
	assert.True(t, os.IsNotExist(err), "flat __-encoded file should not exist")
}

func TestConfigMapFetcher_Fetch_FleetProjectLayout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Simulate the exact ConfigMap layout from the arena-fleet-sample.yaml
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "arena-project-fleet-echo-test",
			Namespace:         "dev-agents",
			ResourceVersion:   "99999",
			CreationTimestamp: metav1.Now(),
		},
		Data: map[string]string{
			"config.arena.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: fleet-echo-test
spec:
  providers: []
  scenarios:
    - file: scenarios/echo-greeting.scenario.yaml
    - file: scenarios/echo-math.scenario.yaml
  defaults:
    temperature: 0
    max_tokens: 256
`,
			"scenarios__echo-greeting.scenario.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: echo-greeting
spec:
  id: echo-greeting
  task_type: assistant
  turns:
    - role: user
      content: "Hello!"
`,
			"scenarios__echo-math.scenario.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: echo-math
spec:
  id: echo-math
  task_type: assistant
  turns:
    - role: user
      content: "What is 2+2?"
`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	fetcher := NewConfigMapFetcher(ConfigMapFetcherConfig{
		Name:      "arena-project-fleet-echo-test",
		Namespace: "dev-agents",
	}, fakeClient)

	ctx := context.Background()
	artifact, err := fetcher.Fetch(ctx, "")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	// Verify the directory structure matches what the arena worker expects
	// The worker reads config.arena.yaml which references scenarios/echo-greeting.scenario.yaml

	// 1. config.arena.yaml at root
	configContent, err := os.ReadFile(filepath.Join(artifact.Path, "config.arena.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(configContent), "scenarios/echo-greeting.scenario.yaml",
		"arena config should reference scenario files with / paths")

	// 2. Scenario files in scenarios/ subdirectory (decoded from scenarios__ keys)
	greetingContent, err := os.ReadFile(filepath.Join(artifact.Path, "scenarios", "echo-greeting.scenario.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(greetingContent), "echo-greeting")

	mathContent, err := os.ReadFile(filepath.Join(artifact.Path, "scenarios", "echo-math.scenario.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(mathContent), "echo-math")

	// 3. Verify the scenarios directory was actually created
	entries, err := os.ReadDir(filepath.Join(artifact.Path, "scenarios"))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "scenarios/ should contain exactly 2 files")
}

func TestDecodeConfigMapKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "no encoding",
			key:      "config.arena.yaml",
			expected: "config.arena.yaml",
		},
		{
			name:     "single level",
			key:      "scenarios__greeting.yaml",
			expected: "scenarios/greeting.yaml",
		},
		{
			name:     "multiple levels",
			key:      "a__b__c__file.yaml",
			expected: "a/b/c/file.yaml",
		},
		{
			name:     "empty string",
			key:      "",
			expected: "",
		},
		{
			name:     "trailing double underscore",
			key:      "dir__",
			expected: "dir/",
		},
		{
			name:     "single underscore preserved",
			key:      "my_file.yaml",
			expected: "my_file.yaml",
		},
		{
			name:     "mixed single and double underscores",
			key:      "my_dir__my_file.yaml",
			expected: "my_dir/my_file.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, decodeConfigMapKey(tt.key))
		})
	}
}

// Helper function to verify directory contents
func verifyDirectoryContents(t *testing.T, dirPath string, expected map[string]string) {
	t.Helper()

	for key, expectedValue := range expected {
		filePath := filepath.Join(dirPath, key)
		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "expected file %s not found in directory", key)
		assert.Equal(t, expectedValue, string(content), "content mismatch for %s", key)
	}
}
