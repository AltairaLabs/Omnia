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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers" // Register default eval handlers
	"github.com/AltairaLabs/PromptKit/runtime/events"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// --- Test helpers ---

// gatherMetricNames returns metric family names from a Gatherer.
func gatherMetricNames(t *testing.T, g prometheus.Gatherer) map[string]bool {
	t.Helper()
	gathered, err := g.Gather()
	require.NoError(t, err)
	names := make(map[string]bool, len(gathered))
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}
	return names
}

// assertMetricExists fails if the named metric is not in the gathered output.
func assertMetricExists(t *testing.T, g prometheus.Gatherer, name string) {
	t.Helper()
	names := gatherMetricNames(t, g)
	assert.True(t, names[name], "expected metric %q to exist, got: %v", name, names)
}

// assertMetricHasLabels fails if no metric in the family has all specified labels.
func assertMetricHasLabels(t *testing.T, g prometheus.Gatherer, name string, labels map[string]string) {
	t.Helper()
	gathered, err := g.Gather()
	require.NoError(t, err)

	var family *dto.MetricFamily
	for _, mf := range gathered {
		if mf.GetName() == name {
			family = mf
			break
		}
	}
	require.NotNilf(t, family, "metric family %q not found", name)

	for _, m := range family.GetMetric() {
		if metricHasAllLabels(m, labels) {
			return // found a match
		}
	}
	t.Errorf("metric %q has no series with labels %v", name, labels)
}

// metricHasAllLabels checks if a metric has all the specified label key-value pairs.
func metricHasAllLabels(m *dto.Metric, labels map[string]string) bool {
	labelMap := make(map[string]string)
	for _, lp := range m.GetLabel() {
		labelMap[lp.GetName()] = lp.GetValue()
	}
	for k, v := range labels {
		if labelMap[k] != v {
			return false
		}
	}
	return true
}

// --- Tests ---

// TestCollectorIntegration_PipelineMetrics verifies that the PromptKit Collector
// registers all pipeline metrics and records them when events are fired.
// This validates the contract between Omnia's runtime wiring (cmd/runtime/main.go)
// and the SDK's metric output.
func TestCollectorIntegration_PipelineMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := sdkmetrics.NewCollector(sdkmetrics.CollectorOpts{
		Registerer: reg,
		Namespace:  "omnia",
		ConstLabels: prometheus.Labels{
			"agent":           "test-agent",
			"namespace":       "test-ns",
			"promptpack_name": "test-pack",
		},
	})

	// Bind with nil — no instance labels, only const labels
	ctx := collector.Bind(nil)

	// Fire one event per type to trigger all pipeline metrics
	now := time.Now()

	ctx.OnEvent(&events.Event{
		Type: events.EventPipelineCompleted, Timestamp: now,
		Data: &events.PipelineCompletedData{
			Duration: 2 * time.Second, InputTokens: 100, OutputTokens: 50,
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventProviderCallCompleted, Timestamp: now,
		Data: &events.ProviderCallCompletedData{
			Provider: "openai", Model: "gpt-4", Duration: time.Second,
			InputTokens: 100, OutputTokens: 50, CachedTokens: 20, Cost: 0.01,
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventProviderCallFailed, Timestamp: now,
		Data: &events.ProviderCallFailedData{
			Provider: "openai", Model: "gpt-4", Duration: 500 * time.Millisecond,
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventToolCallCompleted, Timestamp: now,
		Data: &events.ToolCallCompletedData{
			ToolName: "get_weather", Duration: 200 * time.Millisecond, Status: "success",
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventToolCallFailed, Timestamp: now,
		Data: &events.ToolCallFailedData{
			ToolName: "calculate", Duration: 100 * time.Millisecond, Status: "error",
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventValidationPassed, Timestamp: now,
		Data: &events.ValidationPassedData{
			ValidatorName: "max_length", ValidatorType: "output", Duration: 5 * time.Millisecond,
		},
	})

	ctx.OnEvent(&events.Event{
		Type: events.EventValidationFailed, Timestamp: now,
		Data: &events.ValidationFailedData{
			ValidatorName: "content_filter", ValidatorType: "output", Duration: 10 * time.Millisecond,
		},
	})

	// Assert all 11 pipeline metric names appear with omnia_ prefix
	expectedMetrics := []string{
		"omnia_pipeline_duration_seconds",
		"omnia_provider_request_duration_seconds",
		"omnia_provider_requests_total",
		"omnia_provider_input_tokens_total",
		"omnia_provider_output_tokens_total",
		"omnia_provider_cached_tokens_total",
		"omnia_provider_cost_total",
		"omnia_tool_call_duration_seconds",
		"omnia_tool_calls_total",
		"omnia_validation_duration_seconds",
		"omnia_validations_total",
	}

	names := gatherMetricNames(t, reg)
	for _, name := range expectedMetrics {
		assert.True(t, names[name], "expected metric %q to be registered", name)
	}

	// Assert const labels present on a representative metric
	assertMetricHasLabels(t, reg, "omnia_provider_requests_total", map[string]string{
		"agent":           "test-agent",
		"namespace":       "test-ns",
		"promptpack_name": "test-pack",
	})

	// Assert variable labels present on provider metrics
	assertMetricHasLabels(t, reg, "omnia_provider_requests_total", map[string]string{
		"provider": "openai",
		"model":    "gpt-4",
		"status":   "success",
	})

	// Assert variable labels present on tool metrics
	assertMetricHasLabels(t, reg, "omnia_tool_calls_total", map[string]string{
		"tool":   "get_weather",
		"status": "success",
	})

	// Assert variable labels present on validation metrics
	assertMetricHasLabels(t, reg, "omnia_validations_total", map[string]string{
		"validator":      "max_length",
		"validator_type": "output",
		"status":         "passed",
	})
}

// TestCollectorIntegration_EvalMetrics_WithInstanceLabels verifies that the
// eval-only collector (used by arena-eval-worker) correctly records eval metrics
// with bound instance labels and produces NO pipeline metrics.
func TestCollectorIntegration_EvalMetrics_WithInstanceLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := sdkmetrics.NewEvalOnlyCollector(sdkmetrics.CollectorOpts{
		Registerer:     reg,
		Namespace:      "omnia",
		InstanceLabels: []string{"agent", "namespace", "promptpack_name"},
	})

	// Bind with instance label values — mirrors eval worker per-evaluation binding
	ctx := collector.Bind(map[string]string{
		"agent":           "test-agent",
		"namespace":       "prod",
		"promptpack_name": "my-pack",
	})

	// Record eval results via the MetricRecorder interface
	err := ctx.Record(evalResultForTest(0.85), gaugeMetricDef("helpfulness_score", 0, 1))
	require.NoError(t, err)

	err = ctx.Record(evalResultForTest(1.0), booleanMetricDef("response_contains_expected"))
	require.NoError(t, err)

	// Assert eval metrics appear with omnia_eval_ prefix and bound instance labels.
	// The Collector prefixes eval metrics with {namespace}_eval_{name}.
	assertMetricExists(t, reg, "omnia_eval_helpfulness_score")
	assertMetricHasLabels(t, reg, "omnia_eval_helpfulness_score", map[string]string{
		"agent":           "test-agent",
		"namespace":       "prod",
		"promptpack_name": "my-pack",
	})

	assertMetricExists(t, reg, "omnia_eval_response_contains_expected")
	assertMetricHasLabels(t, reg, "omnia_eval_response_contains_expected", map[string]string{
		"agent":           "test-agent",
		"namespace":       "prod",
		"promptpack_name": "my-pack",
	})

	// Assert NO pipeline metrics present (eval-only collector)
	names := gatherMetricNames(t, reg)
	pipelineMetrics := []string{
		"omnia_pipeline_duration_seconds",
		"omnia_provider_requests_total",
		"omnia_tool_calls_total",
		"omnia_validations_total",
	}
	for _, name := range pipelineMetrics {
		assert.False(t, names[name], "eval-only collector should NOT have pipeline metric %q", name)
	}
}

// TestCollectorIntegration_RegistryMerging verifies that prometheus.Gatherers
// correctly merges an Omnia-owned registry with a Collector registry, mirroring
// the actual /metrics endpoint configuration in cmd/runtime/main.go.
func TestCollectorIntegration_RegistryMerging(t *testing.T) {
	// Registry 1: Omnia-owned metrics (simulate omnia_runtime_info gauge)
	omniaReg := prometheus.NewRegistry()
	testGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_runtime_info",
		Help: "Runtime information gauge (always 1)",
		ConstLabels: prometheus.Labels{
			"agent":     "test-agent",
			"namespace": "test-ns",
		},
	})
	require.NoError(t, omniaReg.Register(testGauge))
	testGauge.Set(1)

	// Registry 2: Collector metrics
	collectorReg := prometheus.NewRegistry()
	collector := sdkmetrics.NewCollector(sdkmetrics.CollectorOpts{
		Registerer: collectorReg,
		Namespace:  "omnia",
		ConstLabels: prometheus.Labels{
			"agent":           "test-agent",
			"namespace":       "test-ns",
			"promptpack_name": "test-pack",
		},
	})

	// Fire an event so the collector has data
	ctx := collector.Bind(nil)
	ctx.OnEvent(&events.Event{
		Type: events.EventProviderCallCompleted, Timestamp: time.Now(),
		Data: &events.ProviderCallCompletedData{
			Provider: "openai", Model: "gpt-4", Duration: time.Second,
			InputTokens: 100, OutputTokens: 50,
		},
	})

	// Merge registries — this mirrors the actual /metrics handler setup
	gatherers := prometheus.Gatherers{omniaReg, collectorReg}

	// Assert both metric families appear in gathered output
	names := gatherMetricNames(t, gatherers)
	assert.True(t, names["omnia_runtime_info"], "Omnia-owned metric should appear in merged output")
	assert.True(t, names["omnia_provider_requests_total"], "Collector metric should appear in merged output")

	// Verify HTTP handler output mirrors actual /metrics path
	handler := promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "omnia_runtime_info")
	assert.Contains(t, body, "omnia_provider_requests_total")
}

// TestCollectorIntegration_FullPipeline exercises the complete
// Server → Conversation → Collector pipeline, verifying that both
// pipeline metrics (from mock provider) and eval metrics appear in the
// same registry. This mirrors the production wiring in cmd/runtime/main.go
// where a full NewCollector (not eval-only) is used.
func TestCollectorIntegration_FullPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.json"
	err := writeTestFile(t, packPath, demoToolsPackJSON)
	require.NoError(t, err)

	evalDefs, err := LoadAllEvalDefs(packPath)
	require.NoError(t, err)
	require.Len(t, evalDefs, 3, "should load 1 pack + 2 prompt evals")

	// Use full NewCollector (not eval-only) — mirrors cmd/runtime/main.go
	collectorReg := prometheus.NewRegistry()
	collector := sdkmetrics.NewCollector(sdkmetrics.CollectorOpts{
		Registerer: collectorReg,
		Namespace:  "omnia",
		ConstLabels: prometheus.Labels{
			"agent":           "test-agent",
			"namespace":       "test-ns",
			"promptpack_name": "test-pack",
		},
	})

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
		{SessionId: "metrics-integration-test", Content: "What's the weather in London?"},
	})

	err = server.Converse(stream)
	assert.Error(t, err) // Stream ends after processing

	assert.NotEmpty(t, stream.sentMessages, "should have sent response messages")

	// Give async eval dispatch time to complete
	time.Sleep(500 * time.Millisecond)

	// Verify PIPELINE metrics appear (from mock provider call)
	names := gatherMetricNames(t, collectorReg)

	// The mock provider fires provider.call.completed events
	assert.True(t, names["omnia_provider_requests_total"],
		"pipeline metric omnia_provider_requests_total should appear from mock provider")

	// Verify EVAL metrics also appear (from turn-level evals).
	// The Collector prefixes eval metrics with {namespace}_eval_{name}.
	assert.True(t, names["omnia_eval_no_hallucinated_urls"],
		"eval metric should appear from regex URL eval")
	assert.True(t, names["omnia_eval_response_contains_expected"],
		"eval metric should appear from contains eval")

	// Verify const labels are present on pipeline metrics
	assertMetricHasLabels(t, collectorReg, "omnia_provider_requests_total", map[string]string{
		"agent":           "test-agent",
		"namespace":       "test-ns",
		"promptpack_name": "test-pack",
	})
}

// --- Eval result helpers ---

func evalResultForTest(value float64) evals.EvalResult {
	return evals.EvalResult{
		EvalID: "test-eval",
		Type:   "test",
		Score:  &value,
		Value:  value,
	}
}

func gaugeMetricDef(name string, min, max float64) *evals.MetricDef {
	return &evals.MetricDef{
		Name: name,
		Type: evals.MetricGauge,
		Range: &evals.Range{
			Min: &min,
			Max: &max,
		},
	}
}

func booleanMetricDef(name string) *evals.MetricDef {
	return &evals.MetricDef{
		Name: name,
		Type: evals.MetricBoolean,
	}
}
