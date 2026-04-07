/*
Copyright 2026.

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

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// --- substituteArgs tests ---

func TestSubstituteArgs(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		args     map[string]string
		expected string
	}{
		{
			name:     "single arg",
			query:    `rate(http_errors{service="{{args.service}}"}[5m])`,
			args:     map[string]string{"service": "my-svc"},
			expected: `rate(http_errors{service="my-svc"}[5m])`,
		},
		{
			name:     "multiple args",
			query:    `rate(http_errors{service="{{args.service}}",ns="{{args.namespace}}"}[5m])`,
			args:     map[string]string{"service": "my-svc", "namespace": "prod"},
			expected: `rate(http_errors{service="my-svc",ns="prod"}[5m])`,
		},
		{
			name:     "no args",
			query:    `rate(http_errors[5m])`,
			args:     map[string]string{},
			expected: `rate(http_errors[5m])`,
		},
		{
			name:     "nil args",
			query:    `rate(http_errors[5m])`,
			args:     nil,
			expected: `rate(http_errors[5m])`,
		},
		{
			name:     "repeated arg",
			query:    `{{args.x}} + {{args.x}}`,
			args:     map[string]string{"x": "1"},
			expected: `1 + 1`,
		},
		{
			name:     "unmatched placeholder preserved",
			query:    `rate(errors{svc="{{args.missing}}"}[5m])`,
			args:     map[string]string{"other": "val"},
			expected: `rate(errors{svc="{{args.missing}}"}[5m])`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteArgs(tt.query, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- evaluateCondition tests ---

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		value     float64
		expected  bool
	}{
		{"gte pass", "result[0] >= 0.9", 0.95, true},
		{"gte exact", "result[0] >= 0.9", 0.9, true},
		{"gte fail", "result[0] >= 0.9", 0.85, false},
		{"lte pass", "result[0] <= 0.05", 0.03, true},
		{"lte exact", "result[0] <= 0.05", 0.05, true},
		{"lte fail", "result[0] <= 0.05", 0.06, false},
		{"gt pass", "result[0] > 100", 101, true},
		{"gt fail", "result[0] > 100", 100, false},
		{"lt pass", "result[0] < 5", 4, true},
		{"lt fail", "result[0] < 5", 5, false},
		{"eq pass", "result[0] == 1", 1, true},
		{"eq fail", "result[0] == 1", 2, false},
		{"ne pass", "result[0] != 0", 1, true},
		{"ne fail", "result[0] != 0", 0, false},
		{"negative threshold", "result[0] > -1", 0, true},
		{"whitespace variations", "result[0]>=0.9", 0.95, true},
		{"result index > 0", "result[1] >= 0.5", 0.6, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCondition(tt.condition, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluateCondition_Invalid(t *testing.T) {
	tests := []struct {
		name      string
		condition string
	}{
		{"empty string", ""},
		{"no operator", "result[0] 0.9"},
		{"no result prefix", "value >= 0.9"},
		{"missing threshold", "result[0] >="},
		{"non-numeric threshold", "result[0] >= abc"},
		{"random text", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluateCondition(tt.condition, 1.0)
			assert.Error(t, err)
		})
	}
}

// --- mergeArgs tests ---

func TestMergeArgs(t *testing.T) {
	t.Run("step overrides template defaults", func(t *testing.T) {
		defaults := map[string]string{"threshold": "0.9", "service": "default-svc"}
		stepArgs := []omniav1alpha1.AnalysisArg{
			{Name: "threshold", Value: "0.95"},
		}
		result := mergeArgs(defaults, stepArgs)
		assert.Equal(t, "0.95", result["threshold"])
		assert.Equal(t, "default-svc", result["service"])
	})

	t.Run("step adds new args", func(t *testing.T) {
		defaults := map[string]string{"a": "1"}
		stepArgs := []omniav1alpha1.AnalysisArg{{Name: "b", Value: "2"}}
		result := mergeArgs(defaults, stepArgs)
		assert.Equal(t, "1", result["a"])
		assert.Equal(t, "2", result["b"])
	})

	t.Run("nil defaults", func(t *testing.T) {
		stepArgs := []omniav1alpha1.AnalysisArg{{Name: "x", Value: "1"}}
		result := mergeArgs(nil, stepArgs)
		assert.Equal(t, "1", result["x"])
	})

	t.Run("nil step args", func(t *testing.T) {
		defaults := map[string]string{"x": "1"}
		result := mergeArgs(defaults, nil)
		assert.Equal(t, "1", result["x"])
	})

	t.Run("both nil", func(t *testing.T) {
		result := mergeArgs(nil, nil)
		assert.Empty(t, result)
	})
}

// --- extractValue tests ---

func TestExtractValue_UnsupportedType(t *testing.T) {
	// model.Matrix is not supported
	_, err := extractValue(model.Matrix{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported result type")
}

// --- compareValues tests ---

func TestCompareValues_UnsupportedOperator(t *testing.T) {
	_, err := compareValues(1.0, "~=", 1.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operator")
}

// --- parseAnalysisTemplate tests ---

func TestParseAnalysisTemplate(t *testing.T) {
	t.Run("valid template", func(t *testing.T) {
		ra := makeUnstructuredAnalysis("error-rate-check", map[string]interface{}{
			"args": []interface{}{
				map[string]interface{}{"name": "service", "value": "default-svc"},
			},
			"metrics": []interface{}{
				map[string]interface{}{
					"name":             "error-rate",
					"interval":         "1m",
					"count":            int64(3),
					"failureLimit":     int64(1),
					"successCondition": "result[0] <= 0.05",
					"provider": map[string]interface{}{
						"prometheus": map[string]interface{}{
							"address": "http://prometheus:9090",
							"query":   `rate(errors{svc="{{args.service}}"}[5m])`,
						},
					},
				},
			},
		})

		template, err := parseAnalysisTemplate(ra)
		require.NoError(t, err)
		assert.Equal(t, "default-svc", template.args["service"])
		require.Len(t, template.metrics, 1)
		assert.Equal(t, "error-rate", template.metrics[0].name)
		assert.Equal(t, int32(1), template.metrics[0].failureLimit)
		assert.Equal(t, "result[0] <= 0.05", template.metrics[0].successCondition)
		assert.Equal(t, "http://prometheus:9090", template.metrics[0].prometheusAddr)
	})

	t.Run("no metrics", func(t *testing.T) {
		ra := makeUnstructuredAnalysis("empty", map[string]interface{}{
			"metrics": []interface{}{},
		})
		_, err := parseAnalysisTemplate(ra)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no metrics")
	})

	t.Run("metric missing name", func(t *testing.T) {
		ra := makeUnstructuredAnalysis("bad", map[string]interface{}{
			"metrics": []interface{}{
				map[string]interface{}{
					"successCondition": "result[0] >= 0.9",
					"provider": map[string]interface{}{
						"prometheus": map[string]interface{}{
							"address": "http://prom:9090",
							"query":   "up",
						},
					},
				},
			},
		})
		_, err := parseAnalysisTemplate(ra)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing name")
	})

	t.Run("non-prometheus provider", func(t *testing.T) {
		ra := makeUnstructuredAnalysis("web", map[string]interface{}{
			"metrics": []interface{}{
				map[string]interface{}{
					"name":             "web-metric",
					"successCondition": "result[0] >= 0.9",
					"provider": map[string]interface{}{
						"web": map[string]interface{}{"url": "http://example.com"},
					},
				},
			},
		})
		_, err := parseAnalysisTemplate(ra)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only prometheus provider")
	})
}

// --- runAnalysis integration tests ---

func TestRunAnalysis_TemplateNotFound(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysis"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysisList"},
		&unstructured.UnstructuredList{},
	)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.runAnalysis(context.Background(), "default", &omniav1alpha1.RolloutAnalysisStep{
		TemplateName: "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestRunAnalysis_PrometheusUnavailable(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysis"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysisList"},
		&unstructured.UnstructuredList{},
	)

	ra := makeUnstructuredAnalysis("error-rate-check", map[string]interface{}{
		"metrics": []interface{}{
			map[string]interface{}{
				"name":             "error-rate",
				"interval":         "1m",
				"count":            int64(3),
				"successCondition": "result[0] <= 0.05",
				"provider": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"address": "http://localhost:1", // unreachable
						"query":   "up",
					},
				},
			},
		},
	})
	ra.SetNamespace("default")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ra).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.runAnalysis(ctx, "default", &omniav1alpha1.RolloutAnalysisStep{
		TemplateName: "error-rate-check",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prometheus query")
}

func TestRunAnalysis_AllMetricsPass(t *testing.T) {
	ts := newMockPrometheus(t, "0.02")
	defer ts.Close()

	scheme, fakeClient := setupAnalysisClient(t, ts.URL, "result[0] <= 0.05")
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	result, err := r.runAnalysis(context.Background(), "default", &omniav1alpha1.RolloutAnalysisStep{
		TemplateName: "error-rate-check",
	})
	require.NoError(t, err)
	assert.True(t, result.passed)
	assert.Equal(t, "all metrics passed", result.message)
}

func TestRunAnalysis_MetricFails(t *testing.T) {
	ts := newMockPrometheus(t, "0.15")
	defer ts.Close()

	scheme, fakeClient := setupAnalysisClient(t, ts.URL, "result[0] <= 0.05")
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	result, err := r.runAnalysis(context.Background(), "default", &omniav1alpha1.RolloutAnalysisStep{
		TemplateName: "error-rate-check",
	})
	require.NoError(t, err)
	assert.False(t, result.passed)
	assert.Contains(t, result.message, "error-rate")
}

func TestRunAnalysis_ArgSubstitution(t *testing.T) {
	var capturedQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prometheus client may send query as URL param (GET) or form value (POST).
		if q := r.URL.Query().Get("query"); q != "" {
			capturedQuery = q
		}
		if err := r.ParseForm(); err == nil {
			if q := r.FormValue("query"); q != "" {
				capturedQuery = q
			}
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{},
						"value":  []interface{}{time.Now().Unix(), "0.95"},
					},
				},
			},
		})
		require.NoError(t, err)
	}))
	defer ts.Close()

	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	registerAnalysisScheme(scheme)

	ra := makeUnstructuredAnalysis("latency-check", map[string]interface{}{
		"args": []interface{}{
			map[string]interface{}{"name": "service", "value": "default-svc"},
		},
		"metrics": []interface{}{
			map[string]interface{}{
				"name":             "latency",
				"interval":         "1m",
				"count":            int64(1),
				"successCondition": "result[0] >= 0.9",
				"provider": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"address": ts.URL,
						"query":   `rate(latency{svc="{{args.service}}"}[5m])`,
					},
				},
			},
		},
	})
	ra.SetNamespace("default")

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ra).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.runAnalysis(context.Background(), "default", &omniav1alpha1.RolloutAnalysisStep{
		TemplateName: "latency-check",
		Args: []omniav1alpha1.AnalysisArg{
			{Name: "service", Value: "overridden-svc"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, `rate(latency{svc="overridden-svc"}[5m])`, capturedQuery)
}

// --- toInt32 tests ---

func TestToInt32(t *testing.T) {
	assert.Equal(t, int32(5), toInt32(int64(5)))
	assert.Equal(t, int32(3), toInt32(float64(3.0)))
	assert.Equal(t, int32(7), toInt32(int32(7)))
	assert.Equal(t, int32(0), toInt32("not a number"))
}

// --- helpers ---

func makeUnstructuredAnalysis(name string, spec map[string]interface{}) *unstructured.Unstructured {
	ra := &unstructured.Unstructured{}
	ra.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "omnia.altairalabs.ai",
		Version: "v1alpha1",
		Kind:    "RolloutAnalysis",
	})
	ra.SetName(name)
	ra.SetNamespace("default")
	ra.SetCreationTimestamp(metav1.Now())
	ra.Object["spec"] = spec
	return ra
}

func registerAnalysisScheme(scheme *k8sruntime.Scheme) {
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysis"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "omnia.altairalabs.ai", Version: "v1alpha1", Kind: "RolloutAnalysisList"},
		&unstructured.UnstructuredList{},
	)
}

func newMockPrometheus(t *testing.T, value string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{},
						"value":  []interface{}{time.Now().Unix(), value},
					},
				},
			},
		})
		require.NoError(t, err)
	}))
}

func setupAnalysisClient(t *testing.T, promURL, condition string) (*k8sruntime.Scheme, client.Client) {
	t.Helper()
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	registerAnalysisScheme(scheme)

	ra := makeUnstructuredAnalysis("error-rate-check", map[string]interface{}{
		"metrics": []interface{}{
			map[string]interface{}{
				"name":             "error-rate",
				"interval":         "1m",
				"count":            int64(3),
				"successCondition": condition,
				"provider": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"address": promURL,
						"query":   "rate(http_errors[5m])",
					},
				},
			},
		},
	})
	ra.SetNamespace("default")

	return scheme, fake.NewClientBuilder().WithScheme(scheme).WithObjects(ra).Build()
}
