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

package runtime

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPackEvalDefs_WithEvals(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"evals": [
			{
				"id": "tone-check",
				"type": "contains",
				"trigger": "every_turn",
				"params": {"substring": "hello"}
			},
			{
				"id": "json-valid",
				"type": "json_valid",
				"trigger": "every_turn"
			}
		],
		"prompts": {}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	defs, err := LoadPackEvalDefs(packPath)
	require.NoError(t, err)
	assert.Len(t, defs, 2)
	assert.Equal(t, "tone-check", defs[0].ID)
	assert.Equal(t, "json-valid", defs[1].ID)
}

func TestLoadPackEvalDefs_NoEvals(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"

	packContent := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"prompts": {}
	}`
	err := writeTestFile(t, packPath, packContent)
	require.NoError(t, err)

	defs, err := LoadPackEvalDefs(packPath)
	require.NoError(t, err)
	assert.Empty(t, defs)
}

func TestLoadPackEvalDefs_FileNotFound(t *testing.T) {
	_, err := LoadPackEvalDefs("/nonexistent/pack.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read pack file")
}

func TestLoadPackEvalDefs_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"

	err := writeTestFile(t, packPath, "not valid json{{{")
	require.NoError(t, err)

	_, err = LoadPackEvalDefs(packPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse pack file")
}

func TestBuildEvalOptions_NilCollector(t *testing.T) {
	server := NewServer()
	opts := server.buildEvalOptions()
	assert.Nil(t, opts)
}

func TestBuildEvalOptions_WithCollector(t *testing.T) {
	collector := metrics.NewEvalOnlyCollector(metrics.CollectorOpts{
		Registerer: prometheus.NewRegistry(),
		Namespace:  "omnia_eval",
	})
	server := NewServer(
		WithEvalCollector(collector),
		WithEvalDefs([]evals.EvalDef{
			{ID: "test-eval", Type: "contains"},
		}),
	)

	opts := server.buildEvalOptions()
	require.NotNil(t, opts)
	assert.Len(t, opts, 3, "should return WithEvalRunner, WithMetrics, and WithEvalGroups")
}

func TestBuildEvalOptions_EmptyDefs(t *testing.T) {
	collector := metrics.NewEvalOnlyCollector(metrics.CollectorOpts{
		Registerer: prometheus.NewRegistry(),
		Namespace:  "omnia_eval",
	})
	server := NewServer(
		WithEvalCollector(collector),
	)

	opts := server.buildEvalOptions()
	require.NotNil(t, opts)
	assert.Len(t, opts, 3, "should return WithEvalRunner, WithMetrics, and WithEvalGroups")
}

func TestResolveInlineEvalGroups_DefaultWhenUnset(t *testing.T) {
	server := NewServer()
	got := server.resolveInlineEvalGroups()
	assert.Equal(t, DefaultInlineEvalGroups, got)
	assert.Contains(t, got, evals.GroupFastRunning)
	assert.NotContains(t, got, evals.DefaultEvalGroup,
		"default group is deliberately excluded so long-running/external evals don't run inline")
}

func TestResolveInlineEvalGroups_DefaultWhenEmpty(t *testing.T) {
	// Semantic: an empty (non-nil) list also falls back to defaults.
	// This matches PromptKit's FilterByGroups, which treats len==0 as "no
	// filter" — so Omnia promotes the built-in default rather than
	// silently disabling the path. Disabling is done via
	// EvalConfig.Enabled=false.
	server := NewServer(WithInlineEvalGroups([]string{}))
	assert.Equal(t, DefaultInlineEvalGroups, server.resolveInlineEvalGroups())
}

func TestResolveInlineEvalGroups_CustomGroups(t *testing.T) {
	custom := []string{"pii-checks", "brand-voice"}
	server := NewServer(WithInlineEvalGroups(custom))
	assert.Equal(t, custom, server.resolveInlineEvalGroups())
}

func TestBuildEvalOptions_UsesConfiguredInlineGroups(t *testing.T) {
	collector := metrics.NewEvalOnlyCollector(metrics.CollectorOpts{
		Registerer: prometheus.NewRegistry(),
		Namespace:  "omnia_eval",
	})
	server := NewServer(
		WithEvalCollector(collector),
		WithInlineEvalGroups([]string{"custom-group"}),
	)
	opts := server.buildEvalOptions()
	require.Len(t, opts, 3)
	// We can't introspect WithEvalGroups directly without applying it to
	// a config. The earlier resolveInlineEvalGroups tests cover the
	// group-selection logic; here we only assert the options list is
	// shaped right (3 items) when a custom group is configured.
}
