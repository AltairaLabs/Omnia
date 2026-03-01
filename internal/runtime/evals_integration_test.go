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

package runtime

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // Register default eval handlers
	"github.com/altairalabs/omnia/pkg/metrics"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// testResultWriter captures eval results for assertion.
type testResultWriter struct {
	mu      sync.Mutex
	results []evals.EvalResult
	written chan struct{}
}

func newTestResultWriter() *testResultWriter {
	return &testResultWriter{written: make(chan struct{}, 100)}
}

func (w *testResultWriter) WriteResults(_ context.Context, results []evals.EvalResult) error {
	w.mu.Lock()
	w.results = append(w.results, results...)
	w.mu.Unlock()
	w.written <- struct{}{}
	return nil
}

func (w *testResultWriter) Results() []evals.EvalResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	r := make([]evals.EvalResult, len(w.results))
	copy(r, w.results)
	return r
}

// demoToolsPackJSON mirrors the pack.json used by the demo-prompts agent in the cluster.
// All eval types and params must match what PromptKit's handler registry supports.
// NOTE: The regex handler uses Go's regexp (RE2) — no lookaheads/lookbehinds.
// NOTE: The regex handler param is "expect_match", NOT "should_match".
const demoToolsPackJSON = `{
  "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
  "id": "demo-prompts",
  "name": "Demo Prompts",
  "version": "2.0.0",
  "description": "Demo pack showcasing evals, validators, tools, and workflow features.",
  "template_engine": {
    "version": "v1",
    "syntax": "{{variable}}",
    "features": ["basic_substitution"]
  },
  "prompts": {
    "default": {
      "id": "default",
      "name": "General Assistant",
      "description": "Friendly general-purpose assistant with tool access.",
      "version": "2.0.0",
      "system_template": "You are a helpful AI assistant for demo purposes. Be concise and friendly in your responses. You can use tools to look up weather and perform calculations.",
      "tools": ["get_weather", "calculate"],
      "tool_policy": {
        "tool_choice": "auto",
        "max_rounds": 3
      },
      "parameters": {
        "temperature": 0.7,
        "max_tokens": 1024
      },
      "validators": [
        {
          "type": "max_length",
          "enabled": true,
          "fail_on_violation": false,
          "params": {
            "max_characters": 2000
          }
        }
      ],
      "evals": [
        {
          "id": "helpfulness",
          "type": "llm_judge",
          "trigger": "sample_turns",
          "sample_percentage": 20,
          "description": "Assess response helpfulness",
          "params": {
            "judge_prompt": "Rate the response for helpfulness and accuracy on a 1-5 scale.",
            "passing_score": 3
          },
          "metric": {
            "name": "helpfulness_score",
            "type": "gauge",
            "range": { "min": 0, "max": 1 }
          }
        },
        {
          "id": "contains-mock",
          "type": "contains",
          "trigger": "every_turn",
          "description": "Verify response contains expected content",
          "params": {
            "patterns": ["response"]
          },
          "metric": {
            "name": "response_contains_expected",
            "type": "boolean"
          }
        }
      ]
    }
  },
  "tools": {
    "get_weather": {
      "name": "get_weather",
      "description": "Get current weather for a location",
      "parameters": {
        "type": "object",
        "properties": {
          "location": {
            "type": "string",
            "description": "City name or coordinates"
          }
        },
        "required": ["location"]
      }
    },
    "calculate": {
      "name": "calculate",
      "description": "Perform mathematical calculations",
      "parameters": {
        "type": "object",
        "properties": {
          "expression": {
            "type": "string",
            "description": "Mathematical expression to evaluate"
          }
        },
        "required": ["expression"]
      }
    }
  },
  "evals": [
    {
      "id": "no-hallucination-urls",
      "type": "regex",
      "trigger": "every_turn",
      "description": "Flag responses that fabricate URLs",
      "params": {
        "pattern": "https?://[a-zA-Z0-9.-]+\\.[a-z]{2,}",
        "expect_match": false
      },
      "metric": {
        "name": "no_hallucinated_urls",
        "type": "boolean"
      }
    }
  ],
  "skills": [
    {
      "name": "demo-guidelines",
      "description": "Guidelines for demo conversations",
      "instructions": "This is a demo environment. Keep responses brief and showcase tool usage when relevant. Avoid making real API calls or referencing production systems."
    }
  ],
  "metadata": {
    "domain": "demo",
    "language": "en",
    "tags": ["demo", "general", "tools"]
  }
}`

// TestEvalIntegration_DemoToolsPack verifies pack-level and all-level eval loading
// from the demo tools pack.
func TestEvalIntegration_DemoToolsPack(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	t.Run("pack-level evals", func(t *testing.T) {
		packEvals, loadErr := LoadPackEvalDefs(packPath)
		require.NoError(t, loadErr)
		assert.Len(t, packEvals, 1, "should load 1 pack-level eval")
		assert.Equal(t, "no-hallucination-urls", packEvals[0].ID)
		assert.Equal(t, "regex", packEvals[0].Type)
		assert.Equal(t, evals.TriggerEveryTurn, packEvals[0].Trigger)
	})

	t.Run("all evals including prompt-level", func(t *testing.T) {
		allEvals, loadErr := LoadAllEvalDefs(packPath)
		require.NoError(t, loadErr)
		assert.Len(t, allEvals, 3, "should load 1 pack + 2 prompt evals")
	})
}

// TestEvalIntegration_ValidateEvalDefs verifies that startup validation catches
// unregistered eval types and passes for valid ones.
func TestEvalIntegration_ValidateEvalDefs(t *testing.T) {
	t.Run("all demo pack types are registered", func(t *testing.T) {
		tmpDir := t.TempDir()
		packPath := tmpDir + "/pack.json"
		err := writeTestFile(t, packPath, demoToolsPackJSON)
		require.NoError(t, err)

		allDefs, loadErr := LoadAllEvalDefs(packPath)
		require.NoError(t, loadErr)

		missing := ValidateEvalDefs(allDefs)
		assert.Empty(t, missing, "all eval types in demo pack should be registered")
	})

	t.Run("catches unregistered types", func(t *testing.T) {
		defs := []evals.EvalDef{
			{ID: "ok", Type: "contains"},
			{ID: "bad1", Type: "nonexistent_type"},
			{ID: "bad2", Type: "also_fake"},
		}
		missing := ValidateEvalDefs(defs)
		assert.Len(t, missing, 2)
		assert.Contains(t, missing, "nonexistent_type")
		assert.Contains(t, missing, "also_fake")
	})

	t.Run("deduplicates types", func(t *testing.T) {
		defs := []evals.EvalDef{
			{ID: "a", Type: "fake"},
			{ID: "b", Type: "fake"},
		}
		missing := ValidateEvalDefs(defs)
		assert.Len(t, missing, 1, "should deduplicate missing types")
	})
}

// TestEvalIntegration_RegistryHasRequiredHandlers checks that all eval types used
// by the demo pack are registered in the handler registry.
func TestEvalIntegration_RegistryHasRequiredHandlers(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()

	requiredTypes := []string{"regex", "llm_judge", "contains"}

	for _, evalType := range requiredTypes {
		t.Run(evalType, func(t *testing.T) {
			assert.True(t, registry.Has(evalType),
				"eval type %q must be registered", evalType)
		})
	}
}

// TestEvalIntegration_ContainsEval tests the contains handler with the patterns
// param format used by the demo pack's contains-mock eval.
func TestEvalIntegration_ContainsEval(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()
	runner := evals.NewEvalRunner(registry)

	evalDefs := []evals.EvalDef{
		{
			ID:      "contains-mock",
			Type:    "contains",
			Trigger: evals.TriggerEveryTurn,
			Params: map[string]any{
				"patterns": []any{"response"},
			},
		},
	}

	t.Run("passes when pattern found", func(t *testing.T) {
		evalCtx := &evals.EvalContext{
			TurnIndex:     1,
			CurrentOutput: "Here is my response to your question about the weather.",
		}
		results := runner.RunTurnEvals(context.Background(), evalDefs, evalCtx)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
		assert.Empty(t, results[0].Error)
	})

	t.Run("fails when pattern missing", func(t *testing.T) {
		evalCtx := &evals.EvalContext{
			TurnIndex:     1,
			CurrentOutput: "Hello! How can I help you today?",
		}
		results := runner.RunTurnEvals(context.Background(), evalDefs, evalCtx)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})
}

// TestEvalIntegration_RegexEval tests the regex handler with expect_match=false
// as used by the demo pack's no-hallucination-urls eval.
func TestEvalIntegration_RegexEval(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()
	runner := evals.NewEvalRunner(registry)

	// RE2-compatible URL pattern — must NOT use lookaheads (Go's regexp is RE2).
	evalDefs := []evals.EvalDef{
		{
			ID:      "no-hallucination-urls",
			Type:    "regex",
			Trigger: evals.TriggerEveryTurn,
			Params: map[string]any{
				"pattern":      `https?://[a-zA-Z0-9.-]+\.[a-z]{2,}`,
				"expect_match": false,
			},
		},
	}

	t.Run("passes when no URLs present", func(t *testing.T) {
		evalCtx := &evals.EvalContext{
			TurnIndex:     1,
			CurrentOutput: "The weather in London is 72°F and sunny.",
		}
		results := runner.RunTurnEvals(context.Background(), evalDefs, evalCtx)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed, "no URLs = no match = pass (expect_match=false)")
	})

	t.Run("fails for any URL", func(t *testing.T) {
		evalCtx := &evals.EvalContext{
			TurnIndex:     1,
			CurrentOutput: "Check out https://fake-site.com/api for more info.",
		}
		results := runner.RunTurnEvals(context.Background(), evalDefs, evalCtx)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed, "URL matches = fail (expect_match=false)")
	})
}

// TestEvalIntegration_UnknownTypeReturnsError verifies that unknown eval types
// produce an error result (not a panic or silent skip).
func TestEvalIntegration_UnknownTypeReturnsError(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()
	runner := evals.NewEvalRunner(registry)

	evalDefs := []evals.EvalDef{
		{
			ID:      "bad-eval",
			Type:    "nonexistent_type",
			Trigger: evals.TriggerEveryTurn,
		},
	}

	evalCtx := &evals.EvalContext{
		TurnIndex:     1,
		CurrentOutput: "test",
	}

	results := runner.RunTurnEvals(context.Background(), evalDefs, evalCtx)

	require.Len(t, results, 1)
	assert.Equal(t, "bad-eval", results[0].EvalID)
	assert.Contains(t, results[0].Error, "handler not found")
	assert.Contains(t, results[0].Error, "nonexistent_type")
}

// TestEvalIntegration_FullPipelineWithMockProvider exercises the complete
// Server → Conversation → EvalMiddleware → Dispatcher → Results pipeline.
func TestEvalIntegration_FullPipelineWithMockProvider(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	// Load ALL eval defs (pack + prompt level) — mirrors what cmd/runtime/main.go does.
	evalDefs, err := LoadAllEvalDefs(packPath)
	require.NoError(t, err)
	require.Len(t, evalDefs, 3, "should load 1 pack + 2 prompt evals")

	collector := evals.NewMetricCollector(evals.WithNamespace("test_eval"))

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithEvalCollector(collector),
		WithEvalDefs(evalDefs),
	)
	defer func() { _ = server.Close() }()

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "eval-test-session", Content: "What's the weather in London?"},
	})

	err = server.Converse(stream)
	assert.Error(t, err) // Stream ends after processing

	assert.NotEmpty(t, stream.sentMessages, "should have sent response messages")

	// Give async eval dispatch time to complete
	time.Sleep(500 * time.Millisecond)

	// Verify eval pipeline was wired correctly
	opts := server.buildEvalOptions()
	assert.Len(t, opts, 2, "should have WithEvalDispatcher and WithResultWriters options")

	// Verify eval metrics were actually recorded in the collector.
	// This proves the full pipeline: SDK middleware → dispatcher → runner → writer → collector.
	var buf bytes.Buffer
	err = collector.WritePrometheus(&buf)
	require.NoError(t, err)
	metricsOutput := buf.String()

	// Turn-level evals (regex, contains) should have produced boolean metrics
	assert.Contains(t, metricsOutput, "no_hallucinated_urls",
		"regex URL hallucination eval should have recorded a metric")
	assert.Contains(t, metricsOutput, "response_contains_expected",
		"contains eval should have recorded a metric")
}

// TestEvalIntegration_HandlersRegistered verifies that the blank import of
// evals/handlers registers all handler types used by the demo pack.
// This catches the bug where cmd/runtime/main.go was missing the import.
func TestEvalIntegration_HandlersRegistered(t *testing.T) {
	registry := evals.NewEvalTypeRegistry()
	types := registry.Types()
	assert.NotEmpty(t, types, "handlers init() should have registered types via blank import")

	// All demo pack eval types must be present
	for _, evalType := range []string{"regex", "contains", "llm_judge"} {
		assert.True(t, registry.Has(evalType),
			"eval type %q must be registered by handlers init()", evalType)
	}
}

// TestEvalIntegration_ResolveEvalsFromPack verifies that pack-level and prompt-level
// evals are correctly loaded and merged from the pack JSON.
func TestEvalIntegration_ResolveEvalsFromPack(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	packEvals, err := LoadPackEvalDefs(packPath)
	require.NoError(t, err)

	allEvals, err := LoadAllEvalDefs(packPath)
	require.NoError(t, err)

	// Extract prompt-level evals (all minus pack-level)
	packIDs := make(map[string]bool)
	for _, e := range packEvals {
		packIDs[e.ID] = true
	}
	var promptEvals []evals.EvalDef
	for _, e := range allEvals {
		if !packIDs[e.ID] {
			promptEvals = append(promptEvals, e)
		}
	}

	resolved := evals.ResolveEvals(packEvals, promptEvals)

	assert.Len(t, resolved, 3, "should merge 1 pack + 2 prompt evals")
	assert.Equal(t, "no-hallucination-urls", resolved[0].ID)
	assert.Equal(t, "helpfulness", resolved[1].ID)
	assert.Equal(t, "contains-mock", resolved[2].ID)
}

// TestEvalIntegration_LoadAllEvalDefs verifies that LoadAllEvalDefs extracts
// both pack-level and prompt-level eval definitions from a pack file.
func TestEvalIntegration_LoadAllEvalDefs(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	allDefs, err := LoadAllEvalDefs(packPath)
	require.NoError(t, err)

	// 1 pack-level (no-hallucination-urls) + 2 prompt-level
	assert.Len(t, allDefs, 3)

	// Verify we got all evals
	ids := make([]string, len(allDefs))
	for i, d := range allDefs {
		ids[i] = d.ID
	}
	assert.Contains(t, ids, "no-hallucination-urls")
	assert.Contains(t, ids, "helpfulness")
	assert.Contains(t, ids, "contains-mock")
}

// TestEvalIntegration_ResultWriterCapture verifies the result writer correctly
// captures eval results for downstream processing.
func TestEvalIntegration_ResultWriterCapture(t *testing.T) {
	writer := newTestResultWriter()

	results := []evals.EvalResult{
		{EvalID: "e1", Type: "contains", Passed: true},
		{EvalID: "e2", Type: "regex", Passed: false},
	}

	err := writer.WriteResults(context.Background(), results)
	require.NoError(t, err)

	captured := writer.Results()
	assert.Len(t, captured, 2)
	assert.True(t, captured[0].Passed)
	assert.False(t, captured[1].Passed)
}

// TestEvalIntegration_PrometheusMetrics exercises the full pipeline and verifies
// that PrometheusResultWriter records eval results to Prometheus metrics.
// Mirrors TestEvalIntegration_FullPipelineWithMockProvider but checks Prometheus output.
func TestEvalIntegration_PrometheusMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	evalDefs, err := LoadAllEvalDefs(packPath)
	require.NoError(t, err)
	require.Len(t, evalDefs, 3)

	collector := evals.NewMetricCollector(evals.WithNamespace("test_prom_eval"))

	// Create Prometheus eval metrics with isolated registry
	reg := prometheus.NewRegistry()
	evalM := metrics.NewEvalMetricsWithRegisterer(reg, metrics.EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithEvalCollector(collector),
		WithEvalDefs(evalDefs),
		WithEvalMetrics(evalM),
	)
	defer func() { _ = server.Close() }()

	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "prom-eval-test", Content: "What's the weather in London?"},
	})

	err = server.Converse(stream)
	assert.Error(t, err) // Stream ends after messages consumed

	assert.NotEmpty(t, stream.sentMessages, "should have sent response messages")

	// Give async eval dispatch time to complete
	time.Sleep(500 * time.Millisecond)

	// Gather Prometheus metrics from our isolated registry
	gathered, err := reg.Gather()
	require.NoError(t, err)

	metricNames := make(map[string]bool)
	for _, mf := range gathered {
		metricNames[mf.GetName()] = true
	}

	// Turn-level evals (regex, contains) should have produced Prometheus metrics
	assert.True(t, metricNames["omnia_eval_executed_total"],
		"omnia_eval_executed_total should be recorded")
	assert.True(t, metricNames["omnia_eval_duration_seconds"],
		"omnia_eval_duration_seconds should be recorded")

	// At least one eval should have passed or failed
	hasPassed := metricNames["omnia_eval_passed_total"]
	hasFailed := metricNames["omnia_eval_failed_total"]
	assert.True(t, hasPassed || hasFailed,
		"should have at least one passed or failed eval metric")
}
