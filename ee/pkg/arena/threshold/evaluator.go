/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package threshold evaluates SLO thresholds against accumulated Arena load test statistics.
package threshold

import (
	"fmt"
	"math"
	"strconv"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// Result contains the evaluation outcome for a single threshold.
type Result struct {
	// Metric is the threshold metric name (e.g., "latency_avg").
	Metric string
	// Operator is the comparison operator (e.g., "<").
	Operator string
	// Target is the original target value string (e.g., "3s").
	Target string
	// Actual is the computed metric value.
	Actual float64
	// ActualStr is the human-readable actual value (e.g., "4.2s").
	ActualStr string
	// Passed indicates whether the threshold was met.
	Passed bool
}

// String returns a human-readable summary like "4.2s < 3s PASS".
func (r Result) String() string {
	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	return fmt.Sprintf("%s %s %s %s", r.ActualStr, r.Operator, r.Target, verdict)
}

// Evaluate checks all thresholds against the accumulated job stats.
// Returns per-threshold results and whether all thresholds passed.
func Evaluate(thresholds []omniav1alpha1.LoadThreshold, stats *queue.JobStats) ([]Result, bool) {
	if len(thresholds) == 0 {
		return nil, true
	}

	results := make([]Result, 0, len(thresholds))
	allPassed := true

	for _, t := range thresholds {
		r := evaluateOne(t, stats)
		results = append(results, r)
		if !r.Passed {
			allPassed = false
		}
	}

	return results, allPassed
}

// evaluateOne evaluates a single threshold against the stats.
func evaluateOne(t omniav1alpha1.LoadThreshold, stats *queue.JobStats) Result {
	r := Result{
		Metric:   string(t.Metric),
		Operator: string(t.Operator),
		Target:   t.Value,
	}

	actual, available := extractMetric(t.Metric, stats)
	if !available {
		r.Actual = math.NaN()
		r.ActualStr = "unavailable"
		r.Passed = true // don't fail on metrics we can't compute
		return r
	}

	r.Actual = actual
	r.ActualStr = formatMetricValue(t.Metric, actual)

	target, err := parseTargetValue(t.Metric, t.Value)
	if err != nil {
		r.ActualStr = "parse error"
		r.Passed = true // don't fail on unparseable targets
		return r
	}

	r.Passed = compare(actual, string(t.Operator), target)
	return r
}

// extractMetric returns the metric value from stats and whether it is available.
func extractMetric(metric omniav1alpha1.LoadThresholdMetric, stats *queue.JobStats) (float64, bool) {
	switch metric {
	case omniav1alpha1.LoadThresholdMetricLatencyAvg:
		return computeAvgLatencySeconds(stats)
	case omniav1alpha1.LoadThresholdMetricErrorRate:
		return computeRate(stats.Failed, stats.Total)
	case omniav1alpha1.LoadThresholdMetricPassRate:
		return computeRate(stats.Passed, stats.Total)
	case omniav1alpha1.LoadThresholdMetricTotalCost:
		return stats.TotalCost, true
	default:
		// Percentile and TTFT metrics are not computable from counters alone.
		return 0, false
	}
}

// computeAvgLatencySeconds returns the average latency in seconds.
func computeAvgLatencySeconds(stats *queue.JobStats) (float64, bool) {
	if stats.Total == 0 {
		return 0, false
	}
	avgMs := stats.TotalDurationMs / float64(stats.Total)
	return avgMs / 1000.0, true
}

// computeRate returns numerator/denominator as a ratio (0..1).
func computeRate(numerator, denominator int64) (float64, bool) {
	if denominator == 0 {
		return 0, false
	}
	return float64(numerator) / float64(denominator), true
}

// parseTargetValue parses the target string into a float64 appropriate for the metric.
// For latency metrics, duration strings are parsed and converted to seconds.
// For all other metrics, the string is parsed as a float.
func parseTargetValue(metric omniav1alpha1.LoadThresholdMetric, value string) (float64, error) {
	if isLatencyMetric(metric) || isTTFTMetric(metric) {
		return parseDurationOrFloat(value)
	}
	return strconv.ParseFloat(value, 64)
}

// parseDurationOrFloat tries time.ParseDuration first, then falls back to float parsing.
// Duration values are returned in seconds.
func parseDurationOrFloat(value string) (float64, error) {
	d, err := time.ParseDuration(value)
	if err == nil {
		return d.Seconds(), nil
	}
	// Fall back to plain float (assumed seconds)
	return strconv.ParseFloat(value, 64)
}

// compare applies the operator to actual and target.
func compare(actual float64, op string, target float64) bool {
	switch op {
	case "<":
		return actual < target
	case ">":
		return actual > target
	case "<=":
		return actual <= target
	case ">=":
		return actual >= target
	default:
		return false
	}
}

// formatMetricValue returns a human-readable string for the metric value.
func formatMetricValue(metric omniav1alpha1.LoadThresholdMetric, value float64) string {
	if isLatencyMetric(metric) || isTTFTMetric(metric) {
		return formatDuration(value)
	}
	if isRateMetric(metric) {
		return fmt.Sprintf("%.4f", value)
	}
	// cost
	return fmt.Sprintf("%.2f", value)
}

// formatDuration formats seconds into a human-readable duration string.
func formatDuration(seconds float64) string {
	if seconds < 1.0 {
		return fmt.Sprintf("%.0fms", seconds*1000)
	}
	return fmt.Sprintf("%.2fs", seconds)
}

func isLatencyMetric(m omniav1alpha1.LoadThresholdMetric) bool {
	switch m {
	case omniav1alpha1.LoadThresholdMetricLatencyAvg,
		omniav1alpha1.LoadThresholdMetricLatencyP50,
		omniav1alpha1.LoadThresholdMetricLatencyP90,
		omniav1alpha1.LoadThresholdMetricLatencyP95,
		omniav1alpha1.LoadThresholdMetricLatencyP99:
		return true
	}
	return false
}

func isTTFTMetric(m omniav1alpha1.LoadThresholdMetric) bool {
	switch m {
	case omniav1alpha1.LoadThresholdMetricTTFTAvg,
		omniav1alpha1.LoadThresholdMetricTTFTP50,
		omniav1alpha1.LoadThresholdMetricTTFTP90,
		omniav1alpha1.LoadThresholdMetricTTFTP95,
		omniav1alpha1.LoadThresholdMetricTTFTP99:
		return true
	}
	return false
}

func isRateMetric(m omniav1alpha1.LoadThresholdMetric) bool {
	switch m {
	case omniav1alpha1.LoadThresholdMetricErrorRate,
		omniav1alpha1.LoadThresholdMetricPassRate,
		omniav1alpha1.LoadThresholdMetricRateLimit:
		return true
	}
	return false
}

// SummaryLine returns a one-line summary of threshold evaluation results,
// e.g., "3/4 passed".
func SummaryLine(results []Result) string {
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d passed", passed, len(results))
}
