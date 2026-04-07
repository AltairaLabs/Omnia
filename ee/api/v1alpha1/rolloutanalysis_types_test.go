/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ptr[T any](v T) *T { return &v }

func TestRolloutAnalysisRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	gvk := GroupVersion.WithKind("RolloutAnalysis")
	if !scheme.Recognizes(gvk) {
		t.Errorf("scheme does not recognize %v", gvk)
	}

	gvkList := GroupVersion.WithKind("RolloutAnalysisList")
	if !scheme.Recognizes(gvkList) {
		t.Errorf("scheme does not recognize %v", gvkList)
	}
}

func TestRolloutAnalysisPrometheusProvider(t *testing.T) {
	timeout := int32(60)
	ra := &RolloutAnalysis{
		ObjectMeta: metav1.ObjectMeta{Name: "prom-analysis", Namespace: "default"},
		Spec: RolloutAnalysisSpec{
			Metrics: []AnalysisMetric{
				{
					Name:             "error-rate",
					Interval:         "1m",
					Count:            5,
					FailureLimit:     1,
					SuccessCondition: "result[0] <= 0.05",
					Provider: MetricProvider{
						Prometheus: &PrometheusProvider{
							Address: "http://prometheus:9090",
							Query:   `sum(rate(http_errors_total[5m])) / sum(rate(http_requests_total[5m]))`,
							Timeout: &timeout,
						},
					},
				},
			},
		},
	}

	if ra.Spec.Metrics[0].Provider.Prometheus == nil {
		t.Fatal("expected Prometheus provider to be set")
	}
	p := ra.Spec.Metrics[0].Provider.Prometheus
	if p.Address != "http://prometheus:9090" {
		t.Errorf("address = %q, want %q", p.Address, "http://prometheus:9090")
	}
	if p.Timeout == nil || *p.Timeout != 60 {
		t.Errorf("timeout = %v, want 60", p.Timeout)
	}
	if ra.Spec.Metrics[0].Provider.ArenaEval != nil {
		t.Error("expected ArenaEval provider to be nil")
	}
	if ra.Spec.Metrics[0].Provider.Web != nil {
		t.Error("expected Web provider to be nil")
	}
}

func TestRolloutAnalysisArenaEvalProvider(t *testing.T) {
	ra := &RolloutAnalysis{
		ObjectMeta: metav1.ObjectMeta{Name: "arena-analysis", Namespace: "default"},
		Spec: RolloutAnalysisSpec{
			Metrics: []AnalysisMetric{
				{
					Name:             "hallucination-rate",
					Interval:         "5m",
					Count:            3,
					SuccessCondition: "result <= 0.1",
					FailureCondition: "result > 0.5",
					Provider: MetricProvider{
						ArenaEval: &ArenaEvalProvider{
							Workspace: "production",
							EvalDef:   "hallucination-check",
						},
					},
				},
			},
		},
	}

	if ra.Spec.Metrics[0].Provider.ArenaEval == nil {
		t.Fatal("expected ArenaEval provider to be set")
	}
	a := ra.Spec.Metrics[0].Provider.ArenaEval
	if a.Workspace != "production" {
		t.Errorf("workspace = %q, want %q", a.Workspace, "production")
	}
	if a.EvalDef != "hallucination-check" {
		t.Errorf("evalDef = %q, want %q", a.EvalDef, "hallucination-check")
	}
	if ra.Spec.Metrics[0].FailureCondition != "result > 0.5" {
		t.Errorf("failureCondition = %q, want %q", ra.Spec.Metrics[0].FailureCondition, "result > 0.5")
	}
}

func TestRolloutAnalysisWebProvider(t *testing.T) {
	timeout := int32(10)
	ra := &RolloutAnalysis{
		ObjectMeta: metav1.ObjectMeta{Name: "web-analysis", Namespace: "default"},
		Spec: RolloutAnalysisSpec{
			Metrics: []AnalysisMetric{
				{
					Name:             "latency-p99",
					Interval:         "2m",
					Count:            10,
					SuccessCondition: "result <= 500",
					Provider: MetricProvider{
						Web: &WebProvider{
							URL:      "http://metrics-api/latency?env={{args.env}}",
							Method:   "GET",
							Headers:  map[string]string{"Authorization": "Bearer token"},
							JSONPath: "$.p99",
							Timeout:  &timeout,
						},
					},
				},
			},
		},
	}

	if ra.Spec.Metrics[0].Provider.Web == nil {
		t.Fatal("expected Web provider to be set")
	}
	w := ra.Spec.Metrics[0].Provider.Web
	if w.Method != "GET" {
		t.Errorf("method = %q, want %q", w.Method, "GET")
	}
	if w.Headers["Authorization"] != "Bearer token" {
		t.Errorf("Authorization header = %q, want %q", w.Headers["Authorization"], "Bearer token")
	}
	if w.JSONPath != "$.p99" {
		t.Errorf("jsonPath = %q, want %q", w.JSONPath, "$.p99")
	}
	if w.Timeout == nil || *w.Timeout != 10 {
		t.Errorf("timeout = %v, want 10", w.Timeout)
	}
}

func TestRolloutAnalysisArgs(t *testing.T) {
	t.Run("arg with default value", func(t *testing.T) {
		arg := RolloutAnalysisArg{
			Name:  "threshold",
			Value: ptr("0.05"),
		}
		if arg.Name != "threshold" {
			t.Errorf("name = %q, want %q", arg.Name, "threshold")
		}
		if arg.Value == nil || *arg.Value != "0.05" {
			t.Errorf("value = %v, want \"0.05\"", arg.Value)
		}
	})

	t.Run("arg without default value", func(t *testing.T) {
		arg := RolloutAnalysisArg{
			Name: "env",
		}
		if arg.Value != nil {
			t.Errorf("expected nil value, got %v", arg.Value)
		}
	})
}

func TestRolloutAnalysisFullStructure(t *testing.T) {
	ra := &RolloutAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "full-analysis",
			Namespace: "production",
		},
		Spec: RolloutAnalysisSpec{
			Args: []RolloutAnalysisArg{
				{Name: "env", Value: ptr("prod")},
				{Name: "threshold"},
			},
			Metrics: []AnalysisMetric{
				{
					Name:             "error-rate",
					Interval:         "1m",
					Count:            5,
					FailureLimit:     2,
					SuccessCondition: "result[0] <= 0.05",
					Provider: MetricProvider{
						Prometheus: &PrometheusProvider{
							Address: "http://prometheus:9090",
							Query:   `rate(errors_total[5m])`,
						},
					},
				},
				{
					Name:             "eval-score",
					Interval:         "5m",
					Count:            3,
					SuccessCondition: "result >= 0.9",
					Provider: MetricProvider{
						ArenaEval: &ArenaEvalProvider{
							Workspace: "production",
							EvalDef:   "quality-check",
						},
					},
				},
			},
		},
	}

	if len(ra.Spec.Args) != 2 {
		t.Errorf("args count = %d, want 2", len(ra.Spec.Args))
	}
	if len(ra.Spec.Metrics) != 2 {
		t.Errorf("metrics count = %d, want 2", len(ra.Spec.Metrics))
	}

	firstArg := ra.Spec.Args[0]
	if firstArg.Name != "env" || firstArg.Value == nil || *firstArg.Value != "prod" {
		t.Errorf("first arg: name=%q value=%v, want name=env value=prod", firstArg.Name, firstArg.Value)
	}

	secondArg := ra.Spec.Args[1]
	if secondArg.Value != nil {
		t.Errorf("second arg value should be nil, got %v", secondArg.Value)
	}

	if ra.Spec.Metrics[0].FailureLimit != 2 {
		t.Errorf("first metric failureLimit = %d, want 2", ra.Spec.Metrics[0].FailureLimit)
	}
	if ra.Spec.Metrics[1].Provider.ArenaEval == nil {
		t.Error("second metric should have ArenaEval provider")
	}
}
