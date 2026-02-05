/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildEngineComponentsOutputDirectory tests that BuildEngineComponents
// respects the output directory configuration and doesn't try to create
// directories in the current working directory.
func TestBuildEngineComponentsOutputDirectory(t *testing.T) {
	// Create a temporary directory for output
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	// Create a minimal config with output directory set
	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err, "BuildEngineComponents should succeed with valid output directory")
	require.NotNil(t, registry, "Registry should not be nil")

	// Verify the output directory was created
	_, err = os.Stat(outputDir)
	assert.NoError(t, err, "Output directory should be created")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestBuildEngineComponentsWithWorkdirChange tests that changing the working
// directory before calling BuildEngineComponents allows it to create relative
// paths in a writable location.
func TestBuildEngineComponentsWithWorkdirChange(t *testing.T) {
	// Create a temporary directory to use as working directory
	tmpDir := t.TempDir()

	// Save original working directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// Change to temp directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Create config with empty output directory (will use default "out")
	cfg := &config.Config{
		Defaults: config.Defaults{
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components - should create "out" in tmpDir
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	require.NoError(t, err, "BuildEngineComponents should succeed when working directory is writable")
	require.NotNil(t, registry, "Registry should not be nil")

	// Verify the "out" directory was created in tmpDir
	outDir := filepath.Join(tmpDir, "out")
	_, err = os.Stat(outDir)
	assert.NoError(t, err, "out directory should be created in temp directory")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestBuildEngineComponentsFailsWithReadOnlyDir tests that BuildEngineComponents
// fails when trying to create directories in a read-only location.
func TestBuildEngineComponentsFailsWithReadOnlyDir(t *testing.T) {
	// Skip if running as root (root can write to read-only dirs)
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	// Create a read-only directory
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)
	defer func() {
		// Make it writable again for cleanup
		_ = os.Chmod(readOnlyDir, 0755)
	}()

	// Save original working directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// Change to read-only directory
	err = os.Chdir(readOnlyDir)
	require.NoError(t, err)

	// Create config with empty output directory
	cfg := &config.Config{
		Defaults: config.Defaults{
			ConfigDir: readOnlyDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	// Build engine components - should fail
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	assert.Error(t, err, "BuildEngineComponents should fail when working directory is read-only")
	assert.Contains(t, err.Error(), "permission denied", "Error should mention permission denied")

	// Clean up
	if registry != nil {
		_ = registry.Close()
	}
}

// TestPromptKitHandlerBuildComponents tests that the handler's buildComponents
// method properly sets up output directories.
func TestPromptKitHandlerBuildComponents(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := &config.Config{
		Defaults: config.Defaults{
			Output: config.OutputConfig{
				Dir: outputDir,
			},
			OutDir:    outputDir,
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err := handler.buildComponents()
	require.NoError(t, err, "buildComponents should succeed")
	require.NotNil(t, handler.providerRegistry, "Provider registry should be set")

	// Verify output directory exists
	_, err = os.Stat(outputDir)
	assert.NoError(t, err, "Output directory should exist")

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestPromptKitHandlerBuildComponentsWithEmptyOutputDir tests that the handler
// sets a default output directory when none is configured.
func TestPromptKitHandlerBuildComponentsWithEmptyOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and change working directory to temp
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	cfg := &config.Config{
		Defaults: config.Defaults{
			// Empty Output.Dir - handler should set a default
			ConfigDir: tmpDir,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock": {
				ID:    "mock",
				Type:  "mock",
				Model: "mock-model",
			},
		},
	}

	handler := &PromptKitHandler{
		config:       cfg,
		log:          logr.Discard(),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	err = handler.buildComponents()
	require.NoError(t, err, "buildComponents should succeed with empty output dir")

	// Verify the handler set a default output directory
	assert.NotEmpty(t, cfg.Defaults.Output.Dir, "Output directory should be set")
	assert.Contains(t, cfg.Defaults.Output.Dir, "/tmp", "Output directory should be in /tmp")

	// Clean up
	if handler.providerRegistry != nil {
		_ = handler.providerRegistry.Close()
	}
}

// TestBuildConfigFromProviders tests that BuildConfigFromProviders creates
// a properly configured config with writable output directories.
func TestBuildConfigFromProviders(t *testing.T) {
	testProviders := map[string]*config.Provider{
		"test-provider": {
			ID:    "test-provider",
			Type:  "mock",
			Model: "test-model",
		},
	}

	cfg := BuildConfigFromProviders(testProviders)

	assert.NotNil(t, cfg)
	assert.Equal(t, "/tmp/arena-dev-console-output", cfg.Defaults.Output.Dir)
	assert.Equal(t, "/tmp/arena-dev-console-output", cfg.Defaults.OutDir)
	assert.Equal(t, "/tmp/arena-dev-console", cfg.Defaults.ConfigDir)
	assert.Len(t, cfg.LoadedProviders, 1)
	assert.Contains(t, cfg.LoadedProviders, "test-provider")
}
