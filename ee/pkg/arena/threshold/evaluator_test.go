/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package threshold

import (
	"math"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

func TestEvaluate_EmptyThresholds(t *testing.T) {
	results, allPassed := Evaluate(nil, &queue.JobStats{})
	if !allPassed {
		t.Error("expected allPassed=true for empty thresholds")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestEvaluate_LatencyAvg_Pass(t *testing.T) {
	stats := &queue.JobStats{
		Total:           10,
		TotalDurationMs: 20000, // 20s total → 2s avg
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricLatencyAvg,
			Operator: omniav1alpha1.LoadThresholdOperatorLT,
			Value:    "3s",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Passed {
		t.Error("expected threshold to pass")
	}
	if r.ActualStr != "2.00s" {
		t.Errorf("expected ActualStr=2.00s, got %s", r.ActualStr)
	}
}

func TestEvaluate_LatencyAvg_Fail(t *testing.T) {
	stats := &queue.JobStats{
		Total:           10,
		TotalDurationMs: 50000, // 50s total → 5s avg
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricLatencyAvg,
			Operator: omniav1alpha1.LoadThresholdOperatorLT,
			Value:    "3s",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if allPassed {
		t.Error("expected allPassed=false")
	}
	if results[0].Passed {
		t.Error("expected threshold to fail")
	}
}

func TestEvaluate_LatencyAvg_MillisecondTarget(t *testing.T) {
	stats := &queue.JobStats{
		Total:           10,
		TotalDurationMs: 3000, // 3s total → 300ms avg
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricLatencyAvg,
			Operator: omniav1alpha1.LoadThresholdOperatorLT,
			Value:    "500ms",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	r := results[0]
	if r.ActualStr != "300ms" {
		t.Errorf("expected ActualStr=300ms, got %s", r.ActualStr)
	}
}

func TestEvaluate_ErrorRate_Pass(t *testing.T) {
	stats := &queue.JobStats{
		Total:  100,
		Passed: 99,
		Failed: 1,
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricErrorRate,
			Operator: omniav1alpha1.LoadThresholdOperatorLT,
			Value:    "0.05",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	if results[0].Actual != 0.01 {
		t.Errorf("expected actual=0.01, got %f", results[0].Actual)
	}
}

func TestEvaluate_ErrorRate_Fail(t *testing.T) {
	stats := &queue.JobStats{
		Total:  100,
		Passed: 80,
		Failed: 20,
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricErrorRate,
			Operator: omniav1alpha1.LoadThresholdOperatorLT,
			Value:    "0.05",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if allPassed {
		t.Error("expected allPassed=false")
	}
	if results[0].Actual != 0.2 {
		t.Errorf("expected actual=0.2, got %f", results[0].Actual)
	}
}

func TestEvaluate_PassRate_Pass(t *testing.T) {
	stats := &queue.JobStats{
		Total:  100,
		Passed: 96,
		Failed: 4,
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricPassRate,
			Operator: omniav1alpha1.LoadThresholdOperatorGTE,
			Value:    "0.95",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	if results[0].Actual != 0.96 {
		t.Errorf("expected actual=0.96, got %f", results[0].Actual)
	}
}

func TestEvaluate_TotalCost_Pass(t *testing.T) {
	stats := &queue.JobStats{
		Total:     50,
		TotalCost: 42.50,
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricTotalCost,
			Operator: omniav1alpha1.LoadThresholdOperatorLTE,
			Value:    "50.00",
		},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	if results[0].Actual != 42.50 {
		t.Errorf("expected actual=42.50, got %f", results[0].Actual)
	}
}

func TestEvaluate_TotalCost_Fail(t *testing.T) {
	stats := &queue.JobStats{
		Total:     50,
		TotalCost: 75.00,
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{
			Metric:   omniav1alpha1.LoadThresholdMetricTotalCost,
			Operator: omniav1alpha1.LoadThresholdOperatorLTE,
			Value:    "50.00",
		},
	}

	_, allPassed := Evaluate(thresholds, stats)
	if allPassed {
		t.Error("expected allPassed=false")
	}
}

func TestEvaluate_UnavailableMetrics(t *testing.T) {
	stats := &queue.JobStats{Total: 10}
	unavailableMetrics := []omniav1alpha1.LoadThresholdMetric{
		omniav1alpha1.LoadThresholdMetricLatencyP50,
		omniav1alpha1.LoadThresholdMetricLatencyP90,
		omniav1alpha1.LoadThresholdMetricLatencyP95,
		omniav1alpha1.LoadThresholdMetricLatencyP99,
		omniav1alpha1.LoadThresholdMetricTTFTAvg,
		omniav1alpha1.LoadThresholdMetricTTFTP50,
		omniav1alpha1.LoadThresholdMetricTTFTP90,
		omniav1alpha1.LoadThresholdMetricTTFTP95,
		omniav1alpha1.LoadThresholdMetricTTFTP99,
		omniav1alpha1.LoadThresholdMetricRateLimit,
	}

	for _, metric := range unavailableMetrics {
		thresholds := []omniav1alpha1.LoadThreshold{
			{Metric: metric, Operator: omniav1alpha1.LoadThresholdOperatorLT, Value: "1.0"},
		}
		results, allPassed := Evaluate(thresholds, stats)
		if !allPassed {
			t.Errorf("metric %s: expected allPassed=true for unavailable metric", metric)
		}
		if results[0].ActualStr != "unavailable" {
			t.Errorf("metric %s: expected ActualStr=unavailable, got %s", metric, results[0].ActualStr)
		}
		if !math.IsNaN(results[0].Actual) {
			t.Errorf("metric %s: expected NaN actual, got %f", metric, results[0].Actual)
		}
	}
}

func TestEvaluate_ZeroStats(t *testing.T) {
	stats := &queue.JobStats{}

	// latency_avg with zero total → unavailable
	thresholds := []omniav1alpha1.LoadThreshold{
		{Metric: omniav1alpha1.LoadThresholdMetricLatencyAvg, Operator: omniav1alpha1.LoadThresholdOperatorLT, Value: "3s"},
		{Metric: omniav1alpha1.LoadThresholdMetricErrorRate, Operator: omniav1alpha1.LoadThresholdOperatorLT, Value: "0.05"},
		{Metric: omniav1alpha1.LoadThresholdMetricPassRate, Operator: omniav1alpha1.LoadThresholdOperatorGTE, Value: "0.95"},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true for zero stats (all unavailable)")
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("metric %s: expected passed for zero stats", r.Metric)
		}
	}
}

func TestEvaluate_MixedPassFail(t *testing.T) {
	stats := &queue.JobStats{
		Total:           100,
		Passed:          90,
		Failed:          10,
		TotalDurationMs: 500000, // 5s avg
		TotalCost:       30.00,
	}

	lt := omniav1alpha1.LoadThresholdOperatorLT
	gte := omniav1alpha1.LoadThresholdOperatorGTE
	lte := omniav1alpha1.LoadThresholdOperatorLTE
	thresholds := []omniav1alpha1.LoadThreshold{
		{Metric: omniav1alpha1.LoadThresholdMetricLatencyAvg, Operator: lt, Value: "3s"},
		{Metric: omniav1alpha1.LoadThresholdMetricErrorRate, Operator: lt, Value: "0.15"},
		{Metric: omniav1alpha1.LoadThresholdMetricPassRate, Operator: gte, Value: "0.95"},
		{Metric: omniav1alpha1.LoadThresholdMetricTotalCost, Operator: lte, Value: "50.00"},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if allPassed {
		t.Error("expected allPassed=false")
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	expected := []bool{false, true, false, true}
	for i, r := range results {
		if r.Passed != expected[i] {
			t.Errorf("result[%d] (%s): expected passed=%v, got %v", i, r.Metric, expected[i], r.Passed)
		}
	}
}

func TestEvaluate_AllOperators(t *testing.T) {
	stats := &queue.JobStats{
		Total:     100,
		Passed:    50,
		Failed:    50,
		TotalCost: 10.0,
	}

	tests := []struct {
		op     omniav1alpha1.LoadThresholdOperator
		value  string
		passed bool
	}{
		{omniav1alpha1.LoadThresholdOperatorLT, "20.00", true},   // 10 < 20
		{omniav1alpha1.LoadThresholdOperatorLT, "5.00", false},   // 10 < 5
		{omniav1alpha1.LoadThresholdOperatorGT, "5.00", true},    // 10 > 5
		{omniav1alpha1.LoadThresholdOperatorGT, "20.00", false},  // 10 > 20
		{omniav1alpha1.LoadThresholdOperatorLTE, "10.00", true},  // 10 <= 10
		{omniav1alpha1.LoadThresholdOperatorLTE, "5.00", false},  // 10 <= 5
		{omniav1alpha1.LoadThresholdOperatorGTE, "10.00", true},  // 10 >= 10
		{omniav1alpha1.LoadThresholdOperatorGTE, "20.00", false}, // 10 >= 20
	}

	for _, tt := range tests {
		thresholds := []omniav1alpha1.LoadThreshold{
			{Metric: omniav1alpha1.LoadThresholdMetricTotalCost, Operator: tt.op, Value: tt.value},
		}
		results, _ := Evaluate(thresholds, stats)
		if results[0].Passed != tt.passed {
			t.Errorf("op=%s value=%s: expected passed=%v, got %v", tt.op, tt.value, tt.passed, results[0].Passed)
		}
	}
}

func TestEvaluate_FloatTargetForLatency(t *testing.T) {
	// Plain float target (assumed seconds) for latency metric
	stats := &queue.JobStats{
		Total:           10,
		TotalDurationMs: 20000, // 2s avg
	}
	thresholds := []omniav1alpha1.LoadThreshold{
		{Metric: omniav1alpha1.LoadThresholdMetricLatencyAvg, Operator: omniav1alpha1.LoadThresholdOperatorLT, Value: "3.0"},
	}

	results, allPassed := Evaluate(thresholds, stats)
	if !allPassed {
		t.Error("expected allPassed=true")
	}
	if !results[0].Passed {
		t.Error("expected threshold to pass")
	}
}

func TestResult_String(t *testing.T) {
	r := Result{
		Metric:    "latency_avg",
		Operator:  "<",
		Target:    "3s",
		Actual:    4.2,
		ActualStr: "4.20s",
		Passed:    false,
	}
	got := r.String()
	expected := "4.20s < 3s FAIL"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	r.Passed = true
	got = r.String()
	expected = "4.20s < 3s PASS"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSummaryLine(t *testing.T) {
	results := []Result{
		{Passed: true},
		{Passed: false},
		{Passed: true},
		{Passed: true},
	}
	got := SummaryLine(results)
	expected := "3/4 passed"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCompare_UnknownOperator(t *testing.T) {
	if compare(1.0, "==", 1.0) {
		t.Error("expected false for unknown operator")
	}
}

func TestFormatDuration_SubSecond(t *testing.T) {
	got := formatDuration(0.250)
	if got != "250ms" {
		t.Errorf("expected 250ms, got %s", got)
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	got := formatDuration(2.5)
	if got != "2.50s" {
		t.Errorf("expected 2.50s, got %s", got)
	}
}

func TestParseDurationOrFloat_Duration(t *testing.T) {
	v, err := parseDurationOrFloat("500ms")
	if err != nil {
		t.Fatal(err)
	}
	if v != 0.5 {
		t.Errorf("expected 0.5, got %f", v)
	}
}

func TestParseDurationOrFloat_Float(t *testing.T) {
	v, err := parseDurationOrFloat("2.5")
	if err != nil {
		t.Fatal(err)
	}
	if v != 2.5 {
		t.Errorf("expected 2.5, got %f", v)
	}
}

func TestParseDurationOrFloat_Invalid(t *testing.T) {
	_, err := parseDurationOrFloat("not-a-number")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestParseTargetValue_RateMetric(t *testing.T) {
	v, err := parseTargetValue(omniav1alpha1.LoadThresholdMetricErrorRate, "0.05")
	if err != nil {
		t.Fatal(err)
	}
	if v != 0.05 {
		t.Errorf("expected 0.05, got %f", v)
	}
}

func TestParseTargetValue_InvalidRate(t *testing.T) {
	_, err := parseTargetValue(omniav1alpha1.LoadThresholdMetricErrorRate, "not-a-number")
	if err == nil {
		t.Error("expected error for invalid rate")
	}
}

func TestEvaluateOne_ParseError(t *testing.T) {
	stats := &queue.JobStats{Total: 10, TotalCost: 5.0}
	threshold := omniav1alpha1.LoadThreshold{
		Metric:   omniav1alpha1.LoadThresholdMetricTotalCost,
		Operator: omniav1alpha1.LoadThresholdOperatorLT,
		Value:    "not-a-number",
	}

	r := evaluateOne(threshold, stats)
	if !r.Passed {
		t.Error("expected passed=true for parse error")
	}
	if r.ActualStr != "parse error" {
		t.Errorf("expected ActualStr='parse error', got %s", r.ActualStr)
	}
}

func TestIsLatencyMetric(t *testing.T) {
	if !isLatencyMetric(omniav1alpha1.LoadThresholdMetricLatencyAvg) {
		t.Error("expected latency_avg to be latency metric")
	}
	if isLatencyMetric(omniav1alpha1.LoadThresholdMetricErrorRate) {
		t.Error("expected error_rate to not be latency metric")
	}
}

func TestIsTTFTMetric(t *testing.T) {
	if !isTTFTMetric(omniav1alpha1.LoadThresholdMetricTTFTAvg) {
		t.Error("expected ttft_avg to be TTFT metric")
	}
	if isTTFTMetric(omniav1alpha1.LoadThresholdMetricLatencyAvg) {
		t.Error("expected latency_avg to not be TTFT metric")
	}
}

func TestIsRateMetric(t *testing.T) {
	if !isRateMetric(omniav1alpha1.LoadThresholdMetricErrorRate) {
		t.Error("expected error_rate to be rate metric")
	}
	if !isRateMetric(omniav1alpha1.LoadThresholdMetricRateLimit) {
		t.Error("expected rate_limit_rate to be rate metric")
	}
	if isRateMetric(omniav1alpha1.LoadThresholdMetricTotalCost) {
		t.Error("expected total_cost to not be rate metric")
	}
}

func TestFormatMetricValue_Rate(t *testing.T) {
	got := formatMetricValue(omniav1alpha1.LoadThresholdMetricErrorRate, 0.05)
	if got != "0.0500" {
		t.Errorf("expected 0.0500, got %s", got)
	}
}

func TestFormatMetricValue_Cost(t *testing.T) {
	got := formatMetricValue(omniav1alpha1.LoadThresholdMetricTotalCost, 42.5)
	if got != "42.50" {
		t.Errorf("expected 42.50, got %s", got)
	}
}
